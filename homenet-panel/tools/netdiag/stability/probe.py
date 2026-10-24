#!/usr/bin/env python3
"""分层网络稳定性探针（单文件、python3 标准库、无第三方依赖）。

用途：在两台软路由上各自跑同一套目标，比较「谁的上游更稳」，并定位断在哪一层。
    L0 icmp   : ping 计数（可选，失败不影响其它层）
    L1 tcp    : 建连成功率 / 建连毫秒
    L2 tls    : 握手成功率 / 握手毫秒
    L3 http   : 状态码 / 首字节 / 总时长 / 内容断言
    L4 dns    : 指定解析器的应答时间与结果（UDP 直连 53）

输出：每行一条 JSONL（UTC epoch 秒），便于断点续跑与事后聚合。
用法：
    probe.py --tag a85 --duration 600 --interval 10 --out /tmp/probe-a85.jsonl
    probe.py --tag b11 --duration 3600 --interval 10 --layers l1,l2,l3,l4
"""

import argparse
import errno
import json
import os
import random
import socket
import ssl
import struct
import subprocess
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed

# 默认目标集合：类别 -> [(host, port, 断言子串或 None)]
# 第 5 个字段是该目标允许的层（空 = 全部）。223.5.5.5:443 是 DoH 端点，
# 普通的 GET / 会返回 404，不能用 HTTP 层判定可达性，因此只测 l0/l1/l2。
DEFAULT_TARGETS = [
    ("cn-baidu", "www.baidu.com", 443, None, ""),
    ("cn-ali", "223.5.5.5", 443, None, "l0,l1,l2"),
    ("global-cf", "www.cloudflare.com", 443, None, ""),
    ("global-1111", "1.1.1.1", 443, None, ""),
    ("global-google", "www.google.com", 443, None, ""),
    ("global-ytb", "www.youtube.com", 443, None, ""),
]

# 机场/订阅域名单独低频测量（避免触发限流）
SLOW_TARGETS = [
    ("sub-s3", "s3.dlpublicsub.com", 443, None, ""),
    ("sub-link", "link.sumno.xyz", 443, None, ""),
]

DEFAULT_RESOLVERS = ["127.0.0.1", "192.168.1.1", "223.5.5.5"]
DNS_NAMES = ["www.baidu.com", "www.cloudflare.com", "www.youtube.com"]

WRITE_LOCK = threading.Lock()
OUT = None


def emit(rec):
    line = json.dumps(rec, ensure_ascii=False, separators=(",", ":"))
    with WRITE_LOCK:
        OUT.write(line + "\n")
        OUT.flush()


def now():
    return time.time()


def ms(d):
    return round(d * 1000.0, 1)


# ---------------- L0: icmp ----------------

def icmp_once(host):
    """用系统的 ping 做一次探测（raw socket 需要 root，这里不假设权限）。"""
    try:
        out = subprocess.run(["ping", "-c", "1", "-W", "1", host],
                             capture_output=True, text=True, timeout=5)
        for line in out.stdout.splitlines():
            if "time=" in line:
                frag = line.split("time=")[1].split()[0]
                return True, float(frag)
        return False, None
    except Exception:
        return False, None


# ---------------- L1/L2/L3 ----------------

def tcp_connect(host, port, timeout=5.0):
    t0 = time.monotonic()
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(timeout)
    try:
        s.connect((host, port))
        return True, ms(time.monotonic() - t0), None
    except Exception as e:
        return False, ms(time.monotonic() - t0), describe(e)
    finally:
        s.close()


def describe(e):
    name = type(e).__name__
    text = str(e) or name
    return "%s: %s" % (name, text[:120])


def tls_handshake(host, port, timeout=6.0):
    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    t0 = time.monotonic()
    try:
        with socket.create_connection((host, port), timeout=timeout) as raw:
            with ctx.wrap_socket(raw, server_hostname=host) as ss:
                ver = ss.version()
        return True, ms(time.monotonic() - t0), ver, None
    except Exception as e:
        return False, ms(time.monotonic() - t0), None, describe(e)


def http_get(host, port, path="/", timeout=20.0, expect=None):
    """极简 HTTP/1.1：不用 http.client 的 DNS 解析，直接对已解析 IP 发请求。"""
    t0 = time.monotonic()
    try:
        ip = socket.gethostbyname(host)
    except Exception as e:
        return False, 0.0, 0.0, 0, describe(e)
    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    try:
        raw = socket.create_connection((ip, port), timeout=timeout)
        with ctx.wrap_socket(raw, server_hostname=host) as ss:
            t_conn = time.monotonic()
            req = ("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: hnp-probe/1\r\n"
                   "Accept: */*\r\nConnection: close\r\n\r\n" % (path, host))
            ss.settimeout(timeout)
            ss.sendall(req.encode())
            buf = b""
            first_byte_at = None
            while len(buf) < 65536:
                chunk = ss.recv(4096)
                if not chunk:
                    if first_byte_at is None:
                        # 服务端/代理在我们发出请求后直接关连接，没有任何响应字节
                        return False, ms(t_conn - t0), 0.0, 0, "EOFNoResponse"
                    break
                if first_byte_at is None:
                    first_byte_at = time.monotonic()
                buf += chunk
                if b"\r\n\r\n" in buf and len(buf) > 512:
                    break
            t_done = time.monotonic()
        head = buf.split(b"\r\n", 1)[0].decode("latin-1", "replace")
        code = 0
        parts = head.split()
        if len(parts) >= 2 and parts[1].isdigit():
            code = int(parts[1])
        ok = 200 <= code < 400
        if expect and expect.encode() not in buf:
            ok = False
        return ok, ms(t_conn - t0), ms((first_byte_at or t_done) - t_conn), code, None
    except Exception as e:
        return False, 0.0, 0.0, 0, describe(e)


# ---------------- L4: dns ----------------

def dns_query(server, name, timeout=4.0):
    """最小的 A 记录查询：返回 (ok, ms, ips, err)。"""
    tid = random.randint(0, 0xFFFF)
    q = struct.pack("!HHHHHH", tid, 0x0100, 1, 0, 0, 0)
    for part in name.split("."):
        q += bytes([len(part)]) + part.encode()
    q += b"\x00" + struct.pack("!HH", 1, 1)
    t0 = time.monotonic()
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.settimeout(timeout)
    try:
        s.sendto(q, (server, 53))
        data, _ = s.recvfrom(2048)
        el = ms(time.monotonic() - t0)
        if len(data) < 12 or struct.unpack("!H", data[0:2])[0] != tid:
            return False, el, [], "bad-response"
        ancount = struct.unpack("!H", data[6:8])[0]
        if ancount == 0:
            return False, el, [], "no-answer"
        # 跳过问题段
        i = 12
        while i < len(data) and data[i] != 0:
            i += data[i] + 1
        i += 1 + 4
        ips = []
        for _ in range(ancount):
            if i >= len(data):
                break
            while data[i] & 0xC0 != 0xC0 and data[i] != 0:
                i += data[i] + 1
            if data[i] & 0xC0 == 0xC0:
                i += 2
            else:
                i += 1
            if i + 10 > len(data):
                break
            rtype, _rclass, _ttl, rdlen = struct.unpack("!HHIH", data[i:i + 10])
            i += 10
            rdata = data[i:i + rdlen]
            i += rdlen
            if rtype == 1 and rdlen == 4:
                ips.append(".".join(str(b) for b in rdata))
        return bool(ips), el, ips, None
    except Exception as e:
        return False, ms(time.monotonic() - t0), [], describe(e)
    finally:
        s.close()


# ---------------- 系统侧样本 ----------------

def system_sample():
    rec = {"ts": now(), "kind": "system"}
    try:
        rec["load"] = [round(x, 2) for x in os.getloadavg()]
    except Exception:
        pass
    try:
        mem = {}
        with open("/proc/meminfo") as f:
            for line in f:
                k, v = line.split(":", 1)
                mem[k] = int(v.strip().split()[0])
        rec["memAvailMB"] = mem.get("MemAvailable", 0) // 1024
        rec["memTotalMB"] = mem.get("MemTotal", 0) // 1024
    except Exception:
        pass
    try:
        with open("/proc/net/sockstat") as f:
            for line in f:
                if line.startswith("TCP:"):
                    rec["tcpInUse"] = int(line.split("inuse")[1].split()[0])
    except Exception:
        pass
    try:
        out = subprocess.run(["pgrep", "-x", "dae"], capture_output=True, text=True)
        pids = out.stdout.split()
        rec["daeRunning"] = bool(pids)
        rss = 0
        for pid in pids:
            with open("/proc/%s/status" % pid) as f:
                for line in f:
                    if line.startswith("VmRSS:"):
                        rss += int(line.split()[1])
        rec["daeRssMB"] = rss // 1024
    except Exception:
        pass
    rec["kind"] = "system"
    return rec


# ---------------- 主循环 ----------------

def probe_target(tag, name, host, port, expect, layers, allow=""):
    out = []
    if allow:
        layers = layers & set(x for x in allow.split(",") if x)
    if "l0" in layers:
        ok, t = icmp_once(host)
        out.append({"ts": now(), "tag": tag, "target": name, "layer": "l0",
                    "ok": ok, "ms": t})
    if "l1" in layers:
        ok, el, err = tcp_connect(host, port)
        out.append({"ts": now(), "tag": tag, "target": name, "layer": "l1",
                    "ok": ok, "ms": el, "err": err})
        if not ok:
            return out  # 后面几层必然失败，别制造噪声
    if "l2" in layers:
        ok, el, ver, err = tls_handshake(host, port)
        out.append({"ts": now(), "tag": tag, "target": name, "layer": "l2",
                    "ok": ok, "ms": el, "ver": ver, "err": err})
    if "l3" in layers:
        ok, conn_ms, fb_ms, code, err = http_get(host, port, expect=expect)
        out.append({"ts": now(), "tag": tag, "target": name, "layer": "l3",
                    "ok": ok, "connMs": conn_ms, "firstByteMs": fb_ms,
                    "code": code, "err": err})
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--tag", required=True, help="探针标签，例如 a85 / b11")
    ap.add_argument("--duration", type=int, default=600, help="总时长秒")
    ap.add_argument("--interval", type=int, default=10, help="采样间隔秒")
    ap.add_argument("--out", required=True, help="JSONL 输出路径")
    ap.add_argument("--layers", default="l0,l1,l2,l3,l4")
    ap.add_argument("--slow-every", type=int, default=6,
                    help="机场域名每 N 轮才测一次（默认 6 轮 ≈ 60s）")
    ap.add_argument("--resolvers", default=",".join(DEFAULT_RESOLVERS))
    ap.add_argument("--workers", type=int, default=3,
                    help="并发线程数（机场对并发敏感，默认 3）")
    args = ap.parse_args()

    global OUT
    OUT = open(args.out, "a", buffering=1)
    layers = set(x.strip() for x in args.layers.split(",") if x.strip())
    resolvers = [x for x in args.resolvers.split(",") if x]
    targets = list(DEFAULT_TARGETS) + list(SLOW_TARGETS)

    alive_resolvers = []
    for rv in resolvers:
        ok, el, ips, err = dns_query(rv, "www.baidu.com", timeout=2.0)
        emit({"ts": now(), "kind": "info", "tag": args.tag, "resolver": rv,
              "alive": ok, "ms": el, "err": err})
        if ok:
            alive_resolvers.append(rv)
    resolvers = alive_resolvers

    emit({"ts": now(), "kind": "start", "tag": args.tag, "host": socket.gethostname(),
          "layers": sorted(layers), "interval": args.interval,
          "duration": args.duration, "pid": os.getpid()})

    t_end = time.time() + args.duration
    rnd = 0
    with ThreadPoolExecutor(max_workers=max(1, args.workers)) as pool:
        while time.time() < t_end:
            rnd += 1
            r0 = time.time()
            jobs = []
            for name, host, port, expect, allow in targets:
                if name.startswith("sub-") and rnd % max(1, args.slow_every) != 0:
                    continue
                jobs.append(pool.submit(probe_target, args.tag, name, host, port,
                                        expect, layers, allow))
                time.sleep(0.4)  # 错峰，避免瞬时并发把机场打挂
            if "l4" in layers:
                for rv in resolvers:
                    for nm in DNS_NAMES:
                        jobs.append(pool.submit(dns_probe, args.tag, rv, nm))
            for f in as_completed(jobs):
                try:
                    res = f.result()
                except Exception as e:
                    res = None
                if res is None:
                    continue
                if isinstance(res, list):
                    for r in res:
                        emit(r)
                else:
                    emit(res)
            emit(system_sample())
            spent = time.time() - r0
            time.sleep(max(0.5, args.interval - spent))
    emit({"ts": now(), "kind": "stop", "tag": args.tag, "rounds": rnd})
    OUT.close()


def dns_probe(tag, server, name):
    ok, el, ips, err = dns_query(server, name)
    return {"ts": now(), "tag": tag, "target": "dns:%s" % name, "layer": "l4",
            "ok": ok, "ms": el, "server": server, "ips": ips, "err": err}


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(0)
