#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""whereami.py —— 在一台“测试设备”上跑，回答一个问题：我的流量到底从哪儿出去？

只用标准库，macOS / Linux 均可跑；不读任何敏感文件、不改配置、不装东西。
典型用法：

    python3 whereami.py                 # 一次性体检报告
    python3 whereami.py --json          # 机器可读
    python3 whereami.py --watch 5       # 每 5 秒采一次出口 IP，边改配置边看变化
    python3 whereami.py --via 192.168.1.85   # 额外判断“我这台设备是不是经过 .85 上网”

判定逻辑（有意保守，宁可说“不确定”也不乱下结论）：
  出口 IP 的归属 + 到境内/境外目标的时延指纹 + 路径首跳，三者互相印证。
"""
import argparse
import json
import os
import platform
import re
import socket
import ssl
import subprocess
import sys
import time

TIMEOUT = 6.0
UA = "whereami/1.0"

IFCONFIG = "/sbin/ifconfig" if os.path.exists("/sbin/ifconfig") else "ifconfig"
ROUTE = "/sbin/route" if os.path.exists("/sbin/route") else "route"
TRACEROUTE = "/usr/sbin/traceroute" if os.path.exists("/usr/sbin/traceroute") else "traceroute"
SCUTIL = "/usr/sbin/scutil" if os.path.exists("/usr/sbin/scutil") else "scutil"

CN_TARGETS = [("www.baidu.com", 443), ("223.5.5.5", 443), ("www.qq.com", 443)]
GLOBAL_TARGETS = [("api.ipify.org", 443), ("www.youtube.com", 443), ("1.1.1.1", 443)]
IP_SERVICES = [
    ("https://api.ipify.org?format=json", "ip"),
    ("https://www.cloudflare.com/cdn-cgi/trace", "trace:ip"),
    ("https://ifconfig.me/ip", None),
    ("https://icanhazip.com", None),
    ("https://ipinfo.io/ip", None),
    ("https://api.ip.sb/ip", None),
]
IP6_SERVICES = [
    ("https://api6.ipify.org?format=json", "ip"),
    ("https://v6.ident.me", None),
    ("https://ipv6.icanhazip.com", None),
]


# ---------------- 基础工具 ----------------

def http_get(url, timeout=TIMEOUT):
    import urllib.request
    req = urllib.request.Request(url, headers={"User-Agent": UA, "Accept": "*/*"})
    ctx = ssl.create_default_context()
    with urllib.request.urlopen(req, timeout=timeout, context=ctx) as r:
        return r.read(4096).decode("utf-8", "replace").strip()


def tcp_tls_ms(host, port=443, timeout=TIMEOUT):
    """返回 (tcp_ms, tls_ms)；失败返回 (None, err)。"""
    t0 = time.time()
    try:
        s = socket.create_connection((host, port), timeout=timeout)
    except Exception as e:
        return None, "ERR %s" % type(e).__name__
    tcp_ms = (time.time() - t0) * 1000
    if port != 443:
        s.close()
        return tcp_ms, None
    try:
        c = ssl.create_default_context().wrap_socket(s, server_hostname=host)
        tls_ms = (time.time() - t0) * 1000 - tcp_ms
        c.close()
        return tcp_ms, tls_ms
    except Exception as e:
        try:
            s.close()
        except Exception:
            pass
        return tcp_ms, "ERR %s" % type(e).__name__


def sh(cmd, timeout=8):
    try:
        p = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=timeout)
        return (p.stdout or "").strip()
    except Exception:
        return ""


def is_private(ip):
    try:
        parts = [int(x) for x in ip.split(".")]
    except Exception:
        return False
    if len(parts) != 4:
        return False
    a, b = parts[0], parts[1]
    return a == 10 or (a == 172 and 16 <= b <= 31) or (a == 192 and b == 168) or a == 127 or (a == 100 and 64 <= b <= 127)


# ---------------- 采集 ----------------

def local_facts():
    f = {"platform": platform.platform(), "hostname": socket.gethostname(), "interfaces": [], "gateway": "", "dns": [], "proxy_env": {}, "vpn_ifaces": []}
    # 网卡
    if platform.system() == "Linux" and sh("command -v ip"):
        try:
            data = json.loads(sh("ip -j addr") or "[]")
            for it in data:
                ips = [a["local"] for a in it.get("addr_info", []) if a.get("family") in ("inet", "inet6")]
                if ips:
                    f["interfaces"].append({"name": it.get("ifname"), "ips": ips, "up": "UP" in (it.get("flags") or [])})
        except Exception:
            pass
        m = re.search(r"default via (\S+)", sh("ip route show default"))
        if m:
            f["gateway"] = m.group(1)
    elif platform.system() == "Darwin":
        out = sh(IFCONFIG)
        cur = None
        for line in out.splitlines():
            m = re.match(r"^(\w+):\s+flags=", line)
            if m:
                cur = {"name": m.group(1), "ips": [], "up": "<UP" in line or ",UP" in line}
                f["interfaces"].append(cur)
                continue
            m = re.search(r"\binet6? (\S+)", line)
            if m and cur is not None:
                cur["ips"].append(m.group(1))
        m = re.search(r"gateway:\s*(\S+)", sh("%s -n get default" % ROUTE))
        if m:
            f["gateway"] = m.group(1)
    try:
        with open("/etc/resolv.conf") as fh:
            f["dns"] = [l.split()[1] for l in fh if l.startswith("nameserver")][:4]
    except Exception:
        pass
    if not f["dns"] and platform.system() == "Darwin":
        f["dns"] = re.findall(r"nameserver\[\d+\] : (\S+)", sh("%s --dns" % SCUTIL))[:4]
    # 默认网关兜底：Linux/Darwin 都再试一次
    if not f["gateway"] and platform.system() == "Linux":
        m = re.search(r"via (\S+)", sh("ip route get 1.1.1.1 2>/dev/null"))
        if m:
            f["gateway"] = m.group(1)
    for k in ("http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY"):
        if os.environ.get(k):
            f["proxy_env"][k] = os.environ[k]
    f["vpn_ifaces"] = sorted({i["name"] for i in f["interfaces"] if re.match(r"^(utun|tun|tap|wg|ppp|ipsec)", i["name"] or "")})
    return f


def public_ip(services=IP_SERVICES, want_json=False):
    errs = []
    for url, key in services:
        try:
            body = http_get(url)
            if key and key.startswith("trace:"):
                field = key.split(":", 1)[1]
                m = re.search(r"^%s=(\S+)$" % re.escape(field), body, re.M)
                body = m.group(1) if m else ""
            elif key:
                try:
                    body = json.loads(body).get(key, "")
                except Exception:
                    pass
            body = body.strip().strip('"')
            if re.match(r"^[0-9a-fA-F:.]+$", body):
                return (body, url, "") if want_json else (body, url)
            errs.append("%s=返回值异常" % url.split("/")[2])
        except Exception as e:
            errs.append("%s=%s" % (url.split("/")[2], type(e).__name__))
    reason = "；".join(errs) if errs else "无可用服务"
    return (None, None, reason) if want_json else (None, None)


def ip_org(ip):
    for url in ("https://ipinfo.io/%s/json" % ip, "http://ip-api.com/json/%s?lang=zh-CN" % ip):
        try:
            d = json.loads(http_get(url, timeout=5))
            parts, seen = [], set()
            for k in ("org", "isp", "asn", "country", "city"):
                v = str(d.get(k) or "").strip()
                if v and v.lower() not in seen:
                    seen.add(v.lower())
                    parts.append(v)
            org = re.sub(r"\s+", " ", " ".join(parts)).strip()
            if org:
                return org
        except Exception:
            continue
    return ""


def latency_fingerprint():
    rows = []
    for host, port in CN_TARGETS:
        tcp, tls = tcp_tls_ms(host, port)
        rows.append({"group": "cn", "host": host, "tcp_ms": tcp, "tls_ms": tls})
    for host, port in GLOBAL_TARGETS:
        tcp, tls = tcp_tls_ms(host, port)
        rows.append({"group": "global", "host": host, "tcp_ms": tcp, "tls_ms": tls})
    return rows


def trace_path(target="1.1.1.1", maxhop=8):
    if not sh("command -v %s" % TRACEROUTE):
        return None
    out = sh("%s -n -q 1 -w 1 -m %d %s" % (TRACEROUTE, maxhop, target), timeout=maxhop * 3 + 5)
    hops = []
    for line in out.splitlines()[1:]:
        parts = line.split()
        ip = ""
        for p in parts:
            m = re.match(r"^(\d+\.\d+\.\d+\.\d+)$", p)
            if m:
                ip = m.group(1)
                break
        hops.append(ip or "*")
    return {"target": target, "hops": hops}


def probe_via(host, port=8787):
    """判断本机到某台设备（如 dae 服务器）是否可达，作为“我是不是这条路”的旁证。"""
    tcp, tls = tcp_tls_ms(host, port)
    return {"host": host, "port": port, "tcp_ms": tcp, "tls_ms": tls}


# ---------------- 判定 ----------------

def verdict(local, ipv4, org, fp, path, via):
    notes = []
    gw = local.get("gateway") or "?"
    # 1) 出口 IP 归属
    if ipv4 and is_private(ipv4):
        notes.append("出口 IP 是私有/保留地址（%s）——中间还有一层 NAT，公网出口不在本机" % ipv4)
    foreign = bool(re.search(r"(HK|Hong Kong|香港|SG|Singapore|新加坡|JP|Japan|日本|US|United States|TW|Taiwan|KR|Korea)", org or "", re.I))
    idc = bool(re.search(r"(Cloud|Amazon|Google|阿里|腾讯|Aliyun|Tencent|DigitalOcean|Vultr|Linode|OVH|Hetzner|Choopa|Contabo|IDC|Data ?Center|Hosting|LLC|Ltd|Limited|Inc)", org or "", re.I))
    if ipv4 and not org:
        notes.append("查不到出口 IP 的归属（查询接口不支持或超时）")
    if foreign:
        notes.append("出口 IP 归属境外（%s）——本机流量被代理或走了境外线路" % org)
    elif idc:
        notes.append("出口 IP 属于机房/云厂商（%s）——通常是代理/VPN 出口" % org)

    # 2) 时延指纹
    cn = [r for r in fp if r["group"] == "cn" and r["tls_ms"]]
    gl = [r for r in fp if r["group"] == "global" and r["tls_ms"]]
    med = lambda xs: sorted(xs)[len(xs) // 2] if xs else None
    cn_med = med([r["tls_ms"] for r in cn])
    gl_med = med([r["tls_ms"] for r in gl])
    if cn_med and gl_med:
        ratio = gl_med / cn_med if cn_med else 0
        if ratio >= 3 and gl_med >= 300:
            notes.append("境外目标握手 %.0fms 是境内 %.0fms 的 %.1f 倍——强烈提示走了代理线路" % (gl_med, cn_med, ratio))
        elif ratio < 1.6:
            notes.append("境内/境外时延接近（%.0f vs %.0fms）——更像本地直连（或代理线路很近）" % (cn_med, gl_med))

    # 2b) 拦截指纹：本地 TCP 秒回、TLS 却很慢 —— 典型 tproxy/eBPF 拦截特征
    if gl_med and gl_med >= 300:
        fast_tcp = [r for r in fp if r["group"] == "global" and r["tcp_ms"] is not None and r["tcp_ms"] <= 10]
        if len(fast_tcp) >= 2:
            notes.append("到境外目标 TCP 只要 %.0fms 但握手 %.0fms —— 本地有透明代理/拦截在接管出境流量（dae/tproxy 的典型特征）"
                         % (min(r["tcp_ms"] for r in fast_tcp), gl_med))

    # 3) 路径首跳
    first = None
    if path and path.get("hops"):
        first = path["hops"][0]
        if first == "*" and all(h == "*" for h in path["hops"]):
            notes.append("traceroute 全为 *：探测包被防火墙/代理吞掉，路径不可见（不代表断网，时延指纹仍然有效）")
        if first == "192.168.1.85":
            notes.append("路径首跳是 192.168.1.85 —— 本机流量正是从 dae 那台机器出去的")
        elif first and first != "*":
            notes.append("路径首跳是 %s（本机默认网关 %s）" % (first, gw))
    if via and via.get("tcp_ms") is not None:
        notes.append("到 %s:%d 可达（TCP %.0fms）——这台设备与面板/dae 在一条路线上" % (via["host"], via["port"], via["tcp_ms"]))
    elif via:
        notes.append("到 %s:%d 不可达——这台设备不经过那台机器" % (via["host"], via["port"]))

    # 4) 本机线索
    if local.get("vpn_ifaces"):
        notes.append("本机存在 VPN 类接口 %s —— 流量可能在系统层被 VPN 接管" % ",".join(local["vpn_ifaces"]))
    if local.get("proxy_env"):
        notes.append("存在代理环境变量 %s —— 部分程序（curl/pip 等）会走它" % ",".join(sorted(local["proxy_env"])))

    if not notes:
        notes.append("信息不足，无法判定（可能无外网/DNS 异常）")
    return notes


def report(as_json=False, via=None, do_trace=True):
    local = local_facts()
    ipv4, src, ip_err = public_ip(want_json=True)
    ipv6, _, ip6_err = public_ip(IP6_SERVICES, want_json=True)
    org = ip_org(ipv4) if ipv4 else ""
    fp = latency_fingerprint()
    path = trace_path() if do_trace else None
    viares = probe_via(via) if via else None
    notes = verdict(local, ipv4, org, fp, path, viares)

    if as_json:
        return {"ts": int(time.time()), "local": local, "ipv4": ipv4, "ipv4_src": src, "ipv4_err": ip_err, "ipv6": ipv6,
                "org": org, "latency": fp, "path": path, "via": viares, "verdict": notes}

    out = []
    out.append("=" * 68)
    out.append("whereami —— %s (%s)" % (local["hostname"], local["platform"]))
    out.append("=" * 68)
    out.append("[1] 本机网络")
    for it in local["interfaces"]:
        if it["ips"]:
            out.append("    %-10s %s" % (it["name"], " ".join(it["ips"][:4])))
    out.append("    默认网关: %s    DNS: %s" % (local["gateway"] or "?", ", ".join(local["dns"]) or "?"))
    if local["vpn_ifaces"]:
        out.append("    VPN 接口: %s" % ",".join(local["vpn_ifaces"]))
    if local["proxy_env"]:
        out.append("    代理环境变量: %s" % json.dumps(local["proxy_env"], ensure_ascii=False))
    out.append("[2] 出口身份")
    out.append("    IPv4 出口: %s%s" % (ipv4 or "取不到", ("  (来源 %s)" % src) if src else ""))
    if not ipv4 and ip_err:
        out.append("    取不到的原因: %s" % ip_err)
    if org:
        out.append("    归属: %s" % org)
    out.append("    IPv6 出口: %s" % (ipv6 or ("无/取不到" + ("（%s）" % ip6_err if ip6_err else ""))))
    out.append("[3] 时延指纹（TCP / TLS 握手）")
    for r in fp:
        tag = "境内" if r["group"] == "cn" else "境外"
        if r["tcp_ms"] is None:
            out.append("    %-18s %s  连接失败: %s" % (r["host"], tag, r["tls_ms"]))
        else:
            tls = "%.0fms" % r["tls_ms"] if isinstance(r["tls_ms"], float) else ("失败 %s" % r["tls_ms"])
            out.append("    %-18s %s  TCP %.0fms  TLS %s" % (r["host"], tag, r["tcp_ms"], tls))
    if path:
        out.append("[4] 路径（traceroute %s）" % path["target"])
        out.append("    " + " -> ".join(path["hops"]) if path["hops"] else "    无结果")
    if viares:
        out.append("[5] 到 %s:%d" % (viares["host"], viares["port"]))
        out.append("    %s" % ("可达 TCP %.0fms" % viares["tcp_ms"] if viares["tcp_ms"] is not None else "不可达: %s" % viares["tls_ms"]))
    out.append("-" * 68)
    out.append("判定：")
    for n in notes:
        out.append("  · " + n)
    out.append("=" * 68)
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser(description="看看这台设备的流量从哪儿出去")
    ap.add_argument("--json", action="store_true", help="输出 JSON")
    ap.add_argument("--watch", type=int, default=0, help="每 N 秒采一次出口 IP")
    ap.add_argument("--via", default="", help="额外判断到某台设备的连通性，如 192.168.1.85")
    ap.add_argument("--no-trace", action="store_true", help="跳过 traceroute")
    a = ap.parse_args()
    if a.watch <= 0:
        print(report(a.json, a.via or None, not a.no_trace))
        return 0
    last = None
    try:
        while True:
            ip, _, _ = public_ip()
            ln = " ".join(filter(None, [ip or "取不到", ip_org(ip) if ip else ""]))
            mark = "" if ip == last else "   <== 变化"
            print("%s  %s%s" % (time.strftime("%H:%M:%S"), ln, mark))
            sys.stdout.flush()
            last = ip
            time.sleep(a.watch)
    except KeyboardInterrupt:
        return 0


if __name__ == "__main__":
    sys.exit(main())
