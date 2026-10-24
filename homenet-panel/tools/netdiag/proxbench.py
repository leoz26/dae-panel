#!/usr/bin/env python3
"""proxbench.py — 不同旁路由/网关环境下的代理路径性能对比探针。

用途：在任意一台"客户端"设备上运行，量化"我的流量经这条代理路径出去有多快"，
      并输出 JSON + 人类可读日志，便于对比不同旁路由（dae / OpenWrt / 其它）的差异。

只做只读测量：DNS 解析、TCP 建连、TLS 握手、UDP DNS、小文件下载、出口 IP。
不改任何配置，不写系统文件（除 --out 指定的日志）。

用法：
    python3 proxbench.py --label openwrt-vm --rounds 3 --out logs/openwrt-vm.json
"""
import argparse
import json
import os
import socket
import ssl
import statistics
import struct
import subprocess
import sys
import time
import urllib.request

CN_TARGETS = [("www.baidu.com", 443), ("www.qq.com", 443), ("223.5.5.5", 443)]
GLOBAL_TARGETS = [("www.google.com", 443), ("github.com", 443),
                  ("www.youtube.com", 443), ("www.cloudflare.com", 443)]
DNS_HOSTS = ["www.baidu.com", "www.google.com", "github.com", "www.youtube.com"]
UDP_DNS_SERVERS = [("223.5.5.5", 53), ("1.1.1.1", 53)]
IP_SERVICES = [
    "https://api.ipify.org?format=json",
    "https://www.cloudflare.com/cdn-cgi/trace",
    "https://ifconfig.me/ip",
    "https://icanhazip.com",
]
DOWNLOAD_URL = "https://speed.cloudflare.com/__down?bytes=2000000"
TLS_CTX = ssl.create_default_context()
TLS_CTX.check_hostname = False
TLS_CTX.verify_mode = ssl.CERT_NONE


def now():
    return time.time()


def ms(a, b):
    return round((b - a) * 1000, 1)


def local_facts():
    facts = {}
    try:
        if sys.platform == "darwin":
            out = subprocess.run(["/sbin/route", "-n", "get", "default"],
                                 capture_output=True, text=True, timeout=5).stdout
            for line in out.splitlines():
                if "gateway:" in line:
                    facts["default_gateway"] = line.split(":", 1)[1].strip()
                if "interface:" in line:
                    facts["default_iface"] = line.split(":", 1)[1].strip()
        else:
            out = subprocess.run(["ip", "-4", "route", "show", "default"],
                                 capture_output=True, text=True, timeout=5).stdout
            facts["default_route"] = out.strip().splitlines()[0] if out.strip() else ""
    except Exception as exc:
        facts["error"] = str(exc)[:120]
    try:
        facts["hostname"] = socket.gethostname()
    except Exception:
        pass
    facts["platform"] = sys.platform
    facts["dns_servers"] = dns_servers()
    for key, env in (("http_proxy", "http_proxy"), ("https_proxy", "https_proxy")):
        val = os.environ.get(env) or os.environ.get(env.upper())
        if val:
            facts[key] = val
    return facts


def dns_servers():
    servers = []
    try:
        with open("/etc/resolv.conf", "r", encoding="utf-8", errors="replace") as fh:
            for line in fh:
                parts = line.split()
                if len(parts) >= 2 and parts[0] == "nameserver":
                    servers.append(parts[1])
    except OSError:
        if sys.platform == "darwin":
            out = subprocess.run(["/usr/sbin/scutil", "--dns"],
                                 capture_output=True, text=True, timeout=5).stdout
            for line in out.splitlines():
                line = line.strip()
                if line.startswith("nameserver["):
                    servers.append(line.split(":", 1)[1].strip())
    seen, out = set(), []
    for s in servers:
        if s not in seen:
            seen.add(s)
            out.append(s)
    return out[:4]


def public_ip():
    for url in IP_SERVICES:
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "proxbench/1.0"})
            with urllib.request.urlopen(req, timeout=8) as resp:
                text = resp.read().decode("utf-8", "replace").strip()
            if text.startswith("{"):
                try:
                    data = json.loads(text)
                    if isinstance(data, dict) and data.get("ip"):
                        return str(data["ip"]).strip(), url
                except ValueError:
                    pass
            if "cdn-cgi/trace" in url or "ip=" in text:
                for line in text.splitlines():
                    if line.startswith("ip="):
                        return line.split("=", 1)[1].strip(), url
            candidate = text.split()[0] if text.split() else ""
            if candidate:
                return candidate[:60], url
        except Exception:
            continue
    return "", ""


def ip_org(ip):
    if not ip:
        return {}
    for url in ("https://ipinfo.io/%s/json" % ip,
                "https://ip-api.com/json/%s?lang=zh-CN&fields=country,regionName,city,isp,org,as" % ip):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "proxbench/1.0"})
            with urllib.request.urlopen(req, timeout=8) as resp:
                data = json.loads(resp.read().decode("utf-8", "replace"))
            info = {}
            for key, out in (("country", "country"), ("city", "city"), ("isp", "isp"),
                             ("org", "org"), ("regionName", "region"), ("as", "asn")):
                val = data.get(key)
                if val:
                    info[out] = str(val)[:60]
            if info:
                return info
        except Exception:
            continue
    return {}


def dns_lookup(host):
    t0 = now()
    try:
        infos = socket.getaddrinfo(host, None)
        addrs = sorted({i[4][0] for i in infos})
        return ms(t0, now()), addrs[:3], ""
    except Exception as exc:
        return None, [], str(exc)[:80]


def tcp_tls(host, port):
    dns_ms, addrs, err = dns_lookup(host)
    if err:
        return {"host": host, "error": err}
    ip = addrs[0] if addrs else host
    t0 = now()
    try:
        sock = socket.create_connection((ip, port), timeout=8)
    except Exception as exc:
        return {"host": host, "dns_ms": dns_ms, "error": "tcp: " + str(exc)[:80]}
    t1 = now()
    try:
        tls = TLS_CTX.wrap_socket(sock, server_hostname=host)
        proto = tls.version()
        t2 = now()
        tls.close()
    except Exception as exc:
        sock.close()
        return {"host": host, "dns_ms": dns_ms, "tcp_ms": ms(t0, t1),
                "error": "tls: " + str(exc)[:80]}
    return {"host": host, "dns_ms": dns_ms, "tcp_ms": ms(t0, t1),
            "tls_ms": ms(t1, t2), "tls": proto}


def udp_dns(server, port=53, timeout=3.0):
    # 手搓一个 A 查询 example.com 的 DNS 包
    query = struct.pack(">HHHHHH", 0x1234, 0x0100, 1, 0, 0, 0)
    for part in b"example.com".split(b"."):
        query += bytes([len(part)]) + part
    query += b"\x00" + struct.pack(">HH", 1, 1)
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(timeout)
    t0 = now()
    try:
        sock.sendto(query, (server, port))
        data, _ = sock.recvfrom(2048)
        return {"server": server, "udp_ms": ms(t0, now()), "bytes": len(data)}
    except Exception as exc:
        return {"server": server, "error": str(exc)[:60]}
    finally:
        sock.close()


def download(url, timeout=25):
    t0 = now()
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "proxbench/1.0"})
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read()
        dt = now() - t0
        return {"bytes": len(body), "seconds": round(dt, 2),
                "mbps": round(len(body) * 8 / dt / 1e6, 2) if dt > 0 else None}
    except Exception as exc:
        return {"error": str(exc)[:100]}




PAGES = [("https://www.baidu.com/", "cn-page"), ("https://www.google.com/", "global-page")]


def icmp_ping(host, count=3):
    cmd = ["ping", "-c", str(count), "-W", "2", host] if sys.platform != "darwin" else \
          ["ping", "-c", str(count), "-W", "2000", host]
    try:
        out = subprocess.run(cmd, capture_output=True, text=True, timeout=15).stdout
    except Exception as exc:
        return {"host": host, "error": str(exc)[:60]}
    loss = None
    for line in out.splitlines():
        if "packet loss" in line:
            for token in line.split(","):
                if "packet loss" in token:
                    loss = token.strip()
    m = None
    for line in out.splitlines():
        if "min/avg/max" in line and "=" in line:
            parts = line.split("=")[-1].strip().split("/")
            if len(parts) >= 3:
                m = {"min": round(float(parts[0]), 1), "avg": round(float(parts[1]), 1),
                     "max": round(float(parts[2]), 1)}
    return {"host": host, "rtt": m, "loss": loss}


def fetch_ttfb(url, timeout=15):
    import http.client
    from urllib.parse import urlsplit
    parts = urlsplit(url)
    host = parts.hostname
    path = parts.path or "/"
    t0 = now()
    try:
        conn = http.client.HTTPSConnection(host, 443, timeout=timeout, context=TLS_CTX)
        conn.connect()
        t1 = now()
        conn.request("GET", path, headers={"User-Agent": "proxbench/1.0", "Accept": "*/*"})
        resp = conn.getresponse()
        t2 = now()
        body = resp.read()
        t3 = now()
        conn.close()
        return {"url": url, "connect_ms": ms(t0, t1), "ttfb_ms": ms(t1, t2),
                "total_ms": ms(t0, t3), "bytes": len(body), "status": resp.status}
    except Exception as exc:
        return {"url": url, "error": str(exc)[:100]}

def percentile(values, pct):
    if not values:
        return None
    values = sorted(values)
    idx = min(len(values) - 1, int(round((pct / 100.0) * (len(values) - 1))))
    return values[idx]


def summarize(samples, field):
    vals = [s[field] for s in samples if isinstance(s.get(field), (int, float))]
    if not vals:
        return {}
    return {"n": len(vals), "min": min(vals), "p50": percentile(vals, 50),
            "p90": percentile(vals, 90), "max": max(vals)}


def run(label, rounds):
    report = {"label": label, "started_at": time.strftime("%Y-%m-%d %H:%M:%S"),
              "local": local_facts(), "rounds": rounds, "samples": []}
    ip, source = public_ip()
    report["exit_ip"] = ip
    report["exit_ip_source"] = source
    report["exit_org"] = ip_org(ip)
    report["ping"] = [icmp_ping("223.5.5.5"), icmp_ping("1.1.1.1")]
    report["pages"] = [fetch_ttfb(u) for u, _ in PAGES]
    report["baseline"] = {
        "cn": [tcp_tls(h, p) for h, p in CN_TARGETS],
        "global": [tcp_tls(h, p) for h, p in GLOBAL_TARGETS],
        "dns": [dict(zip(("dns_ms", "addrs", "error"), dns_lookup(h))) | {"host": h} for h in DNS_HOSTS],
        "udp_dns": [udp_dns(s, p) for s, p in UDP_DNS_SERVERS],
    }
    for i in range(rounds):
        sample = {"round": i + 1,
                  "cn": [tcp_tls(h, p) for h, p in CN_TARGETS],
                  "global": [tcp_tls(h, p) for h, p in GLOBAL_TARGETS],
                  "udp_dns": [udp_dns(s, p) for s, p in UDP_DNS_SERVERS]}
        if i == 0:
            sample["download"] = download(DOWNLOAD_URL)
        sample["pages"] = [fetch_ttfb(u) for u, _ in PAGES]
        report["samples"].append(sample)
        print("  round %d/%d done" % (i + 1, rounds), file=sys.stderr)
    report["stats"] = {
        "cn_tcp_ms": summarize([x for s in report["samples"] for x in s["cn"]], "tcp_ms"),
        "cn_tls_ms": summarize([x for s in report["samples"] for x in s["cn"]], "tls_ms"),
        "global_tcp_ms": summarize([x for s in report["samples"] for x in s["global"]], "tcp_ms"),
        "global_tls_ms": summarize([x for s in report["samples"] for x in s["global"]], "tls_ms"),
        "global_dns_ms": summarize([x for s in report["samples"] for x in s["global"]], "dns_ms"),
    }
    return report


def verdict(report):
    stats = report["stats"]
    cn_tls = stats["cn_tls_ms"].get("p50")
    gl_tls = stats["global_tls_ms"].get("p50")
    lines = []
    if cn_tls is not None and gl_tls is not None:
        ratio = round(gl_tls / cn_tls, 1) if cn_tls else None
        lines.append("境内 TLS 中位 %s ms / 境外 TLS 中位 %s ms（境外是境内的 %s 倍）"
                     % (cn_tls, gl_tls, ratio))
        if gl_tls >= 800:
            lines.append("境外握手 ≥800ms：代理路径明显偏慢，页面首屏会可感知卡顿")
        elif gl_tls >= 400:
            lines.append("境外握手 400–800ms：能用但偏慢，多资源页面会有等待感")
        else:
            lines.append("境外握手 <400ms：代理路径良好")
    failed = [x["host"] for s in report["samples"] for x in s["global"] if x.get("error")]
    if failed:
        lines.append("有目标失败：%s" % ", ".join(sorted(set(failed))[:5]))
    pages = report.get("pages") or []
    for page in pages:
        if page.get("error"):
            lines.append("页面 %s 失败：%s" % (page["url"], page["error"]))
        else:
            lines.append("页面 %s 首字节 %s ms（总 %s ms，%s 字节，HTTP %s）"
                         % (page["url"], page["ttfb_ms"], page["total_ms"],
                            page["bytes"], page["status"]))
    dl = report["samples"][0].get("download", {}) if report["samples"] else {}
    if dl.get("mbps"):
        lines.append("2MB 下载 %s s / %s Mbps" % (dl.get("seconds"), dl.get("mbps")))
    return lines


def main():
    ap = argparse.ArgumentParser(description="代理路径性能对比探针（只读）")
    ap.add_argument("--label", default=socket.gethostname(), help="本次测量标签，如 openwrt-vm / dae-85")
    ap.add_argument("--rounds", type=int, default=3, help="重复轮数（默认 3）")
    ap.add_argument("--out", default="", help="把 JSON 结果写到这个路径；留空则打印到 stdout")
    args = ap.parse_args()
    report = run(args.label, max(1, args.rounds))
    report["verdict"] = verdict(report)

    print("=" * 64)
    print("标签: %s   出口 IP: %s %s" % (report["label"], report["exit_ip"] or "(未知)",
                                          report["exit_org"].get("country", "")))
    print("网关: %s" % report["local"].get("default_gateway", report["local"].get("default_route", "")))
    print("DNS : %s" % ", ".join(report["local"].get("dns_servers", [])))
    for key in ("cn_tcp_ms", "cn_tls_ms", "global_tcp_ms", "global_tls_ms", "global_dns_ms"):
        st = report["stats"][key]
        if st:
            print("%-14s n=%-3s min=%-7s p50=%-7s p90=%-7s max=%s"
                  % (key, st["n"], st["min"], st["p50"], st["p90"], st["max"]))
    for ping in report.get("ping", []):
        if ping.get("rtt"):
            print("%-14s min=%s avg=%s max=%s  %s" % ("ping " + ping["host"], ping["rtt"]["min"],
                                                      ping["rtt"]["avg"], ping["rtt"]["max"], ping.get("loss", "")))
    for line in report["verdict"]:
        print("  · " + line)
    print("=" * 64)

    if args.out:
        os.makedirs(os.path.dirname(os.path.abspath(args.out)), exist_ok=True)
        with open(args.out, "w", encoding="utf-8") as fh:
            json.dump(report, fh, ensure_ascii=False, indent=2)
        print("JSON 已写入 %s" % args.out)


if __name__ == "__main__":
    main()