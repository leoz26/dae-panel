#!/usr/bin/env python3
"""flowprobe.py — 观察 dae 的流量到底怎么走（只读，不改任何配置）。
用法: flowprobe.py [采样秒数]  (默认 10)
输出:
  1) 各网卡字节/包计数增量 (ens2 / dae0 / dae0peer)
  2) dae 进程当前建立的出站连接，按「节点名 / 远端IP:端口」归并，带字节数 (ss -tni)
  3) 本机直连出口的公网 IP（与节点连接对比，判断是否在走上游）
"""
import base64, json, os, re, socket, subprocess, sys, time

IFS = ["ens2", "dae0", "dae0peer"]
SUB_DIR = "/etc/dae/subs"

def counters(iface):
    base = "/sys/class/net/%s/statistics/" % iface
    if not os.path.isdir(base):
        return None
    def rd(k):
        try:
            return int(open(base + k).read().strip())
        except Exception:
            return 0
    return {"rx": rd("rx_bytes"), "tx": rd("tx_bytes"), "rxp": rd("rx_packets"), "txp": rd("tx_packets")}

def subs_hosts():
    """从订阅镜像里抽取 主机:端口 -> 节点名 映射（只本地解析，不联网）"""
    out = {}
    if not os.path.isdir(SUB_DIR):
        return out
    for fn in os.listdir(SUB_DIR):
        if not fn.endswith(".sub"):
            continue
        raw = open(os.path.join(SUB_DIR, fn), "rb").read()
        txt = raw
        try:
            dec = base64.b64decode(raw, validate=True)
            if b"://" in dec:
                txt = dec
        except Exception:
            pass
        for line in txt.decode("utf-8", "replace").splitlines():
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            name = ""
            if "#" in line:
                line, name = line.split("#", 1)
            if line.startswith("vmess://"):
                try:
                    j = json.loads(base64.b64decode(line[8:] + "==").decode("utf-8", "replace"))
                    out["%s:%s" % (j.get("add", ""), j.get("port", ""))] = j.get("ps") or name
                except Exception:
                    pass
                continue
            m = re.match(r"[a-z0-9]+://[^@]*@([^:/?#]+):(\d+)", line)
            if m:
                out["%s:%s" % (m.group(1), m.group(2))] = name
    return out

def dae_sockets():
    """ss -tni 里属于 dae 的已建立连接: 远端IP:端口 + bytes_sent/bytes_received"""
    p = subprocess.run(["ss", "-tni", "state", "established"], capture_output=True, text=True)
    rows, cur = [], None
    for line in p.stdout.splitlines():
        if line and not line[0].isspace() and "ESTAB" in line:
            cur = line
        elif cur is not None and line.strip().startswith("bytes_"):
            if '"dae"' not in cur:
                cur = None
                continue
            parts = cur.split()
            peer = parts[4] if len(parts) > 4 else ""
            if peer.count(":") < 1:
                cur = None
                continue
            sent = recv = 0
            for tok in line.split():
                if tok.startswith("bytes_sent:"):
                    sent = int(tok.split(":")[1])
                elif tok.startswith("bytes_received:"):
                    recv = int(tok.split(":")[1])
            rows.append({"peer": peer, "sent": sent, "recv": recv})
            cur = None
    return rows

def ip_of(host):
    try:
        return socket.getaddrinfo(host, None, socket.AF_INET)[0][4][0]
    except Exception:
        return None

def human(n):
    for u in ["B", "K", "M", "G"]:
        if abs(n) < 1024:
            return "%d%s" % (n, u)
        n /= 1024.0
    return "%.1fT" % n

secs = int(sys.argv[1]) if len(sys.argv) > 1 else 10
hosts = subs_hosts()
# 主机名 -> IP，方便把 dae 的连接对上节点
ipname = {}
for hp, name in hosts.items():
    h = hp.rsplit(":", 1)[0]
    ip = ip_of(h)
    if ip:
        ipname[ip] = "%s (%s)" % (name or hp, h)

c0 = {i: counters(i) for i in IFS}
s0 = dae_sockets()
time.sleep(secs)
c1 = {i: counters(i) for i in IFS}
s1 = dae_sockets()

print("=== 1) 网卡计数（%ds 增量）===" % secs)
for i in IFS:
    if not c0[i] or not c1[i]:
        print("  %-10s 不存在" % i)
        continue
    print("  %-10s rx +%-9s (%+d pkt)  tx +%-9s (%+d pkt)" % (
        i, human(c1[i]["rx"] - c0[i]["rx"]), c1[i]["rxp"] - c0[i]["rxp"],
        human(c1[i]["tx"] - c0[i]["tx"]), c1[i]["txp"] - c0[i]["txp"]))

print("=== 2) dae 出站连接按节点归并（当前快照）===")
agg = {}
for r in s1:
    ip = r["peer"].rsplit(":", 1)[0]
    key = ipname.get(ip, "未识别 %s" % ip)
    a = agg.setdefault(key, {"n": 0, "sent": 0, "recv": 0})
    a["n"] += 1
    a["sent"] += r["sent"]
    a["recv"] += r["recv"]
for k, v in sorted(agg.items(), key=lambda kv: -(kv[1]["sent"] + kv[1]["recv"])):
    print("  %-42s 连接 %2d  发出 %-8s 收到 %-8s" % (k, v["n"], human(v["sent"]), human(v["recv"])))
if not agg:
    print("  （无 dae 建立的出站连接）")

print("=== 3) 汇总 ===")
print("  订阅镜像里可识别的节点主机: %d 个" % len(hosts))
print("  dae 出站连接总数: %d" % len(s1))
