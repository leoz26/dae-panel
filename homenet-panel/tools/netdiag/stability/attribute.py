#!/usr/bin/env python3
"""把 dae 日志里的「组内选中哪个节点」与探针的成败时间线对齐，
算出**每个节点的失败率**，用来定位「坏节点」。

用法：
    journalctl -u dae --since "-2 hour" -o short-iso > /tmp/dae.log
    attribute.py --log /tmp/dae.log --jsonl probe-a85.jsonl [--group proxy] [--target-prefix global]

日志行格式（dae v2.1.1）：
    ... INFO Group selects dialer dialer=🇭🇰 广东-香港 DP group=proxy network=udp6
    ... INFO Group re-selects dialer _new_dialer=🇭🇰 ... _old_dialer=🇯🇵 ... group=proxy ... network=tcp4
"""

import argparse
import datetime
import json
import re
from collections import defaultdict

TS_RE = re.compile(r"^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})")
SEL_RE = re.compile(r"selects dialer dialer=(.+?) (?:group|network)=")
RESEL_RE = re.compile(r"_new_dialer=(.+?) _old_dialer=")
GROUP_RE = re.compile(r"group=(\S+)")
NET_RE = re.compile(r"network=(\S+)")


def parse_epoch(line):
    m = TS_RE.match(line)
    if not m:
        return None
    dt = datetime.datetime.strptime(m.group(1), "%Y-%m-%dT%H:%M:%S")
    # 日志为本地时间（+08:00）；统一按本地时区换算成 epoch
    return dt.replace(tzinfo=datetime.timezone(
        datetime.timedelta(hours=8))).timestamp()


def load_events(path, group):
    ev = []
    with open(path, encoding="utf-8", errors="ignore") as f:
        for line in f:
            if "group=" not in line:
                continue
            g = GROUP_RE.search(line)
            if not g or g.group(1) != group:
                continue
            net = NET_RE.search(line)
            network = net.group(1) if net else ""
            name = None
            m = RESEL_RE.search(line)
            if m:
                name = m.group(1)
            else:
                m = SEL_RE.search(line)
                if m:
                    name = m.group(1)
            if not name:
                continue
            ts = parse_epoch(line)
            if ts is None:
                continue
            ev.append((ts, name, network))
    ev.sort(key=lambda x: x[0])
    return ev


def load_probe(path, prefix):
    rows = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line.startswith("{"):
                continue
            r = json.loads(line)
            if r.get("layer") != "l3":
                continue
            if prefix and not str(r.get("target", "")).startswith(prefix):
                continue
            rows.append(r)
    rows.sort(key=lambda r: r["ts"])
    return rows


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--log", required=True)
    ap.add_argument("--jsonl", required=True)
    ap.add_argument("--group", default="proxy")
    ap.add_argument("--target-prefix", default="global")
    ap.add_argument("--window", type=float, default=60.0,
                    help="只在「选中事件」后 N 秒内归因（默认 60s）")
    args = ap.parse_args()

    ev = load_events(args.log, args.group)
    rows = load_probe(args.jsonl, args.target_prefix)
    if not ev:
        print("日志里没有 group=%s 的选中事件" % args.group)
        return
    if not rows:
        print("探针数据里没有匹配的 l3 记录")
        return

    stat = defaultdict(lambda: {"ok": 0, "fail": 0, "errs": defaultdict(int)})
    i = 0
    cur = None
    cur_ts = 0.0
    for r in rows:
        while i < len(ev) and ev[i][0] <= r["ts"]:
            cur_ts, cur = ev[i][0], ev[i][1]
            i += 1
        if cur is None or r["ts"] - cur_ts > args.window:
            continue
        s = stat[cur]
        if r.get("ok"):
            s["ok"] += 1
        else:
            s["fail"] += 1
            s["errs"][r.get("err") or "noerr"] += 1

    print("节点归因（group=%s，窗口 %.0fs，探针 l3 目标前缀 %r）" % (
        args.group, args.window, args.target_prefix))
    print("")
    print("| 节点 | 成功 | 失败 | 失败率 | 主要错误 |")
    print("|---|---|---|---|---|")
    for name, s in sorted(stat.items(), key=lambda kv: -(kv[1]["ok"] + kv[1]["fail"])):
        total = s["ok"] + s["fail"]
        err = max(s["errs"].items(), key=lambda kv: kv[1])[0] if s["errs"] else ""
        print("| %s | %d | %d | %.0f%% | %s |" % (
            name, s["ok"], s["fail"], 100.0 * s["fail"] / total, err))

    tot_ok = sum(s["ok"] for s in stat.values())
    tot = sum(s["ok"] + s["fail"] for s in stat.values())
    print("")
    print("合计：%d/%d 成功（%.1f%%），归因覆盖 %d 条探针记录" % (
        tot_ok, tot, 100.0 * tot_ok / tot if tot else 0, tot))


if __name__ == "__main__":
    main()
