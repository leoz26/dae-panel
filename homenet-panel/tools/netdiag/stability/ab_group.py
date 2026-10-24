#!/usr/bin/env python3
"""受控 A/B：改 dae 的 proxy 组策略 -> dae reload -> 短探针 -> 统计 -> 恢复基线。

用途（2026-10-25 实测）：验证「组内混入远端节点」是不是丢包主因。
结果：baseline(CF+dual) 31%，hk_only 89%，hk_fixed 93%，hk_sg_regex 见日志。
注意：只动 /etc/dae/config.d/panel-core.dae 的 proxy 块，finally 里一定恢复。
"""
import json, os, re, shutil, subprocess, time

CFG = "/etc/dae/config.d/panel-core.dae"
BASE = "/root/ab"
os.makedirs(BASE, exist_ok=True)
TS = time.strftime("%Y%m%d-%H%M%S")
BACKUP = os.path.join(BASE, "panel-core.dae.orig-%s" % TS)
shutil.copy2(CFG, BACKUP)
orig = open(CFG, encoding="utf-8").read()

GRP_RE = re.compile(r"    proxy \{.*?\n    \}\n", re.S)
assert GRP_RE.search(orig), "no proxy group block"

def with_group(body):
    return GRP_RE.sub("    proxy {\n" + body + "    }\n", orig, count=1)

VARIANTS = [
    ("baseline", orig),
    ("hk_only", with_group("        policy: min_moving_avg\n        filter: subtag(dual) && name(keyword: '\u9999\u6e2f')\n")),
    ("hk_sg_only", with_group("        policy: min_moving_avg\n        filter: subtag(dual) && (name(keyword: '\u9999\u6e2f') || name(keyword: '\u65b0\u52a0\u5761'))\n")),
    ("hk_fixed", with_group("        policy: fixed(0)\n        filter: subtag(dual) && name(keyword: '\u9999\u6e2f')\n")),
    ("baseline2", orig),
]

def reload_dae():
    p = subprocess.run(["systemctl", "reload", "dae.service"], capture_output=True, text=True, timeout=90)
    time.sleep(3)
    try:
        prog = open("/var/run/dae.progress").read().strip().replace("\n", "|")
    except OSError:
        prog = "?"
    return p.returncode, prog, (p.stderr or "").strip()[:160]

def check_forwarding():
    try:
        if open("/proc/sys/net/ipv4/ip_forward").read().strip() != "1":
            open("/proc/sys/net/ipv4/ip_forward", "w").write("1")
    except OSError as e:
        say("ip_forward failed:", e)

LOG = open(os.path.join(BASE, "ab-%s.log" % TS), "w", encoding="utf-8")
def say(*a):
    line = " ".join(str(x) for x in a)
    print(line, flush=True)
    LOG.write(line + "\n"); LOG.flush()

say("backup:", BACKUP)
results = {}
try:
    for name, text in VARIANTS:
        open(CFG, "w", encoding="utf-8").write(text)
        os.chmod(CFG, 0o600)
        v = subprocess.run(["dae", "validate", "-c", "/etc/dae/config.dae"], capture_output=True, text=True, timeout=120)
        if v.returncode != 0:
            say("[%s] validate FAILED: %s" % (name, (v.stdout + v.stderr).strip()[:300]))
            shutil.copy2(BACKUP, CFG)
            continue
        check_forwarding()
        rc, prog, err = reload_dae()
        say("[%s] reload rc=%s progress=%s %s" % (name, rc, prog, err))
        out = os.path.join(BASE, "probe-%s-%s.jsonl" % (name, TS))
        p = subprocess.run(["python3", "/root/probe.py", "--tag", "ab_" + name, "--duration", "150",
                            "--interval", "10", "--workers", "3", "--layers", "l3", "--out", out],
                           capture_output=True, text=True, timeout=400, cwd="/root")
        if p.returncode != 0:
            say("[%s] probe rc=%s err=%s" % (name, p.returncode, (p.stderr or "").strip()[:200]))
        rows = []
        if os.path.exists(out):
            for line in open(out, encoding="utf-8"):
                try:
                    r = json.loads(line)
                except ValueError:
                    continue
                if r.get("layer") == "l3" and str(r.get("target", "")).startswith("global"):
                    rows.append(r)
        ok = sum(1 for r in rows if r.get("ok"))
        results[name] = [ok, len(rows)]
        say("[%s] l3 ok %d/%d" % (name, ok, len(rows)))
finally:
    shutil.copy2(BACKUP, CFG)
    os.chmod(CFG, 0o600)
    rc, prog, err = reload_dae()
    say("RESTORED reload rc=%s progress=%s %s" % (rc, prog, err))
    say("SUMMARY " + json.dumps(results, ensure_ascii=False))
    LOG.close()
