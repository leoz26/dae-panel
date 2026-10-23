#!/usr/bin/env python3
"""flowsniff.py — 被动抓包观察「流量怎么走」（只读，AF_PACKET，不改任何东西）。
用法: flowsniff.py [秒数] [网卡]   默认 20 秒 ens2
输出: 按字节排序的 IPv4 流, 并标注目的地是否为订阅节点(即"被代理")还是直连
"""
import base64, json, os, re, socket, struct, sys, time
from collections import defaultdict

SECS = int(sys.argv[1]) if len(sys.argv) > 1 else 20
IFACE = sys.argv[2] if len(sys.argv) > 2 else "ens2"
SUB_DIR = "/etc/dae/subs"

def node_ips():
    hosts = set()
    for fn in os.listdir(SUB_DIR):
        if not fn.endswith(".sub"):
            continue
        raw = open(os.path.join(SUB_DIR, fn), "rb").read()
        try:
            d = base64.b64decode(raw, validate=True)
            if b"://" in d:
                raw = d
        except Exception:
            pass
        for line in raw.decode("utf-8", "replace").splitlines():
            line = line.strip().split("#")[0]
            if line.startswith("vmess://"):
                try:
                    j = json.loads(base64.b64decode(line[8:] + "==").decode("utf-8", "replace"))
                    if j.get("add"):
                        hosts.add(j["add"])
                except Exception:
                    pass
            else:
                m = re.match(r"[a-z0-9]+://[^@]*@([^:/?#]+):\d+", line)
                if m:
                    hosts.add(m.group(1))
    ips = {}
    for h in hosts:
        try:
            for fam, _, _, _, sa in socket.getaddrinfo(h, None, socket.AF_INET):
                ips[sa[0]] = h
        except Exception:
            pass
    return ips

NODE = node_ips()

def classify(ip):
    if ip in NODE:
        return "节点:" + NODE[ip][:24]
    if ip.startswith("192.168.1."):
        return "内网"
    return "外网直连"

s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(3))
s.bind((IFACE, 0))
s.settimeout(0.5)

flows = defaultdict(lambda: {"in": 0, "out": 0, "pkt": 0})
end = time.time() + SECS
while time.time() < end:
    try:
        pkt, addr = s.recvfrom(65535)
    except socket.timeout:
        continue
    if len(pkt) < 34 or pkt[12:14] != b"\x08\x00":
        continue
    ip = pkt[14:]
    ihl = (ip[0] & 0x0F) * 4
    proto = ip[9]
    if proto not in (6, 17):
        continue
    src = socket.inet_ntoa(ip[12:16]); dst = socket.inet_ntoa(ip[16:20])
    l4 = ip[ihl:]
    if len(l4) < 4:
        continue
    sport, dport = struct.unpack("!HH", l4[:4])
    total = struct.unpack("!H", ip[2:4])[0]
    outgoing = addr[2] == socket.PACKET_OUTGOING
    # 以"内网客户端 -> 外部"为一条流的方向归一化
    if src.startswith("192.168.1."):
        key = (src, dst, dport, "tcp" if proto == 6 else "udp")
        flows[key]["out"] += total
    elif dst.startswith("192.168.1."):
        key = (dst, src, sport, "tcp" if proto == 6 else "udp")
        flows[key]["in"] += total
    else:
        key = (src, dst, dport, "tcp" if proto == 6 else "udp")
        flows[key]["out"] += total
    flows[key]["pkt"] += 1
s.close()

def human(n):
    for u in ["B", "K", "M", "G"]:
        if n < 1024:
            return "%d%s" % (n, u)
        n /= 1024.0
    return "%.1fT" % n

print("=== 观察 %ds / %s ；订阅节点 IP %d 个 ===" % (SECS, IFACE, len(NODE)))
print("%-15s %-15s %5s %-4s %8s %8s  %s" % ("客户端", "目的地", "端口", "协议", "发出", "收到", "判读"))
rows = sorted(flows.items(), key=lambda kv: -(kv[1]["out"] + kv[1]["in"]))
for (a, b, port, proto), v in rows[:20]:
    print("%-15s %-15s %5d %-4s %8s %8s  %s" % (a, b, port, proto, human(v["out"]), human(v["in"]), classify(b)))

clients = sorted({k[0] for k in flows})
print("=== 内网客户端出现数: %d -> %s ===" % (len(clients), ", ".join(clients[:12])))
proxied = sum(v["out"] for k, v in flows.items() if k[1] in NODE)
direct = sum(v["out"] for k, v in flows.items() if not k[1].startswith("192.168.1.") and k[1] not in NODE)
print("=== 发出字节: 去节点 %s / 其他外网 %s ===" % (human(proxied), human(direct)))
