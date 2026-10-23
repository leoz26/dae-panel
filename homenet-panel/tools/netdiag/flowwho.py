#!/usr/bin/env python3
"""flowwho.py — 谁在通过这台 dae 上网？(只读, AF_PACKET)
按「客户端 IP」汇总：发出字节、目的地区分（订阅节点 / 其它外网）、Top 目的。
"""
import base64, json, os, re, socket, struct, sys, time
from collections import defaultdict

SECS = int(sys.argv[1]) if len(sys.argv) > 1 else 30
IFACE = sys.argv[2] if len(sys.argv) > 2 else "ens2"
SUB_DIR = "/etc/dae/subs"
SELF = "192.168.1.85"
PANEL_PORTS = {8787, 22}

def node_map():
    nm = {}
    for fn in sorted(os.listdir(SUB_DIR)):
        if not fn.endswith(".sub"):
            continue
        raw = open(os.path.join(SUB_DIR, fn), "rb").read()
        try:
            d = base64.b64decode(raw, validate=True)
            if b"://" in d: raw = d
        except Exception: pass
        for line in raw.decode("utf-8", "replace").splitlines():
            line = line.strip()
            if not line: continue
            name = line.split("#", 1)[1][:26] if "#" in line else ""
            body = line.split("#")[0]
            host = port = None
            if body.startswith("vmess://"):
                try:
                    j = json.loads(base64.b64decode(body[8:] + "==").decode("utf-8", "replace"))
                    host, port, name = j.get("add"), int(j.get("port") or 0), (j.get("ps") or name)[:26]
                except Exception: continue
            else:
                m = re.match(r"[a-z0-9]+://[^@]*@([^:/?#]+):(\d+)", body)
                if not m: continue
                host, port = m.group(1), int(m.group(2))
            if not host: continue
            try:
                for fam, _, _, _, sa in socket.getaddrinfo(host, None, socket.AF_INET):
                    nm.setdefault((sa[0], port), name or host)
            except Exception: pass
    return nm

NM = node_map()
def human(n):
    for u in ["B","K","M","G"]:
        if abs(n) < 1024: return "%d%s" % (n,u)
        n/=1024.0
    return "%.1fT" % n

per_client = defaultdict(lambda: {"out":0,"in":0,"pkt":0})
per_dst = defaultdict(int)
s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(3))
s.bind((IFACE,0)); s.settimeout(0.5)
end = time.time()+SECS
while time.time() < end:
    try: pkt, addr = s.recvfrom(65535)
    except socket.timeout: continue
    if len(pkt) < 34 or pkt[12:14] != b"\x08\x00": continue
    ip = pkt[14:]
    if ip[9] not in (6,17): continue
    ihl=(ip[0]&0x0F)*4
    src=socket.inet_ntoa(ip[12:16]); dst=socket.inet_ntoa(ip[16:20])
    if ihl+4 > len(ip): continue
    sport,dport=struct.unpack("!HH", ip[ihl:ihl+4])
    ln=struct.unpack("!H", ip[2:4])[0]
    if src.startswith("192.168.1.") and not dst.startswith("192.168.1."):
        if dport in PANEL_PORTS: continue
        c = per_client[src]; c["out"] += ln; c["pkt"] += 1
        per_dst[(src, dst, dport)] += ln
    elif src == SELF:
        c = per_client["dae(本机)"]; c["out"] += ln; c["pkt"] += 1
        per_dst[("dae(本机)", dst, dport)] += ln
    elif dst.startswith("192.168.1.") and src and not src.startswith("192.168.1."):
        per_client["入站:"+dst]["in"] += ln
s.close()

print("=== 每个客户端 -> 外网 发出字节（%ds, 网卡 %s）===" % (SECS, IFACE))
for k,v in sorted(per_client.items(), key=lambda kv: -(kv[1]["out"]+kv[1]["in"])):
    if k.startswith("入站:"): continue
    print("  %-16s 发出 %-9s 包 %-6d" % (k, human(v["out"]), v["pkt"]))
print("=== Top 目的 (客户端, 目的IP, 端口) ===")
for (a,b,p),v in sorted(per_dst.items(), key=lambda kv:-kv[1])[:15]:
    tag = "订阅节点" if (b,p) in NM else ("内网" if b.startswith("192.168.1.") else "外网")
    print("  %-16s -> %-15s:%-5d %-8s %s %s" % (a,b,p,human(v),tag,NM.get((b,p),"")[:26]))
print("=== 订阅节点条目(ip:port) 数: %d ===" % len(NM))
