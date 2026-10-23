#!/usr/bin/env python3
"""flowsniff2.py — 分清「谁在代理、流量走哪条腿」（只读）。
用法: flowsniff2.py [秒数] [网卡]
输出:
  A) 其他内网设备 -> 外网 的流（判断他们是不是走 .85 这台 dae）
  B) .85(本机/dae) -> 外网 的流，标注是否为订阅节点
  C) dae0 / dae0peer / ens2 计数增量（看有没有真的过 netkit）
"""
import base64, json, os, re, socket, struct, sys, time
from collections import defaultdict

SECS = int(sys.argv[1]) if len(sys.argv) > 1 else 30
IFACE = sys.argv[2] if len(sys.argv) > 2 else "ens2"
SUB_DIR = "/etc/dae/subs"

def node_map():
    entries = []
    for fn in sorted(os.listdir(SUB_DIR)):
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
            line = line.strip()
            if not line:
                continue
            name = line.split("#", 1)[1][:24] if "#" in line else ""
            body = line.split("#")[0]
            host = port = None
            if body.startswith("vmess://"):
                try:
                    j = json.loads(base64.b64decode(body[8:] + "==").decode("utf-8", "replace"))
                    host, port, name = j.get("add"), int(j.get("port") or 0), (j.get("ps") or name)[:24]
                except Exception:
                    continue
            else:
                m = re.match(r"[a-z0-9]+://[^@]*@([^:/?#]+):(\d+)", body)
                if not m:
                    continue
                host, port = m.group(1), int(m.group(2))
            if not host:
                continue
            try:
                for fam, _, _, _, sa in socket.getaddrinfo(host, None, socket.AF_INET):
                    entries.append(((sa[0], port), name, host))
            except Exception:
                pass
    nm = {}
    for k, name, host in entries:
        nm.setdefault(k, "%s @%s" % (name or "?", host))
    return nm

NM = node_map()
def cls(ip, port):
    if (ip, port) in NM:
        return "订阅节点 " + NM[(ip, port)]
    if ip.startswith("192.168.1."):
        return "内网"
    return "外网(非节点)"

def cnt(iface):
    b = "/sys/class/net/%s/statistics/" % iface
    if not os.path.isdir(b):
        return None
    g = lambda k: int(open(b + k).read())
    return (g("rx_bytes"), g("tx_bytes"), g("rx_packets"), g("tx_packets"))

def human(n):
    for u in ["B", "K", "M", "G"]:
        if abs(n) < 1024:
            return "%d%s" % (n, u)
        n /= 1024.0
    return "%.1fT" % n

ifaces = ["ens2", "dae0"]
c0 = {i: cnt(i) for i in ifaces}
s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(3))
s.bind((IFACE, 0)); s.settimeout(0.5)

client, local, hosts_client, hosts_local = defaultdict(int), defaultdict(int), {}, {}
end = time.time() + SECS
while time.time() < end:
    try:
        pkt, addr = s.recvfrom(65535)
    except socket.timeout:
        continue
    if len(pkt) < 34 or pkt[12:14] != b"\x08\x00":
        continue
    ip = pkt[14:]
    if ip[9] not in (6, 17):
        continue
    ihl = (ip[0] & 0x0F) * 4
    src = socket.inet_ntoa(ip[12:16]); dst = socket.inet_ntoa(ip[16:20])
    sport, dport = struct.unpack("!HH", ip[ihl:ihl+4])
    ln = struct.unpack("!H", ip[2:4])[0]
    if src.startswith("192.168.1.") and not dst.startswith("192.168.1."):
        k = (src, dst, dport)
        client[k] += ln
        hosts_client.setdefault(src, set()).add(dst)
    elif src == "192.168.1.85" and not dst.startswith("192.168.1."):
        k = (src, dst, dport)
        local[k] += ln
s.close()
c1 = {i: cnt(i) for i in ifaces}

print("=== A) 内网其他设备 -> 外网（谁在直接用代理/或走 .85）%ds ===" % SECS)
if not client:
    print("  （没有其他内网设备向外发包经过本机网卡）")
for (a, b, p), v in sorted(client.items(), key=lambda kv: -kv[1])[:12]:
    print("  %-15s -> %-15s:%-5d %-8s %s" % (a, b, p, human(v), cls(b, p)))
print("  出现的客户端: %s" % ", ".join(sorted(hosts_client)[:12]))

print("=== B) 本机(.85) -> 外网 前 10（dae 自己发起的连接）===")
for (a, b, p), v in sorted(local.items(), key=lambda kv: -kv[1])[:10]:
    print("  %-15s -> %-15s:%-5d %-8s %s" % (a, b, p, human(v), cls(b, p)))
tot_local = sum(local.values())
tot_node = sum(v for (a, b, p), v in local.items() if (b, p) in NM)
print("  本机发出合计 %s，其中打给订阅节点 %s" % (human(tot_local), human(tot_node)))

print("=== C) 网卡计数增量（%ds）===" % SECS)
for i in ifaces:
    if not c0[i] or not c1[i]:
        print("  %-8s 不存在" % i); continue
    print("  %-8s rx +%-9s tx +%-9s pkt rx/tx +%d/+%d" % (
        i, human(c1[i][0]-c0[i][0]), human(c1[i][1]-c0[i][1]), c1[i][2]-c0[i][2], c1[i][3]-c0[i][3]))
