#!/usr/bin/env python3
"""把 probe.py 的 JSONL 汇总成可读报告（Markdown）。

用法：
    analyze.py --jsonl logs/a85.jsonl logs/mac.jsonl --out report.md
"""
import argparse
import json
import statistics
from collections import defaultdict


def load(path):
    rows = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line or not line.startswith("{"):
                continue
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError:
                continue
    return rows


def pct(vals, q):
    if not vals:
        return None
    vals = sorted(vals)
    k = max(0, min(len(vals) - 1, int(round((len(vals) - 1) * q))))
    return vals[k]


def fmt(v, unit="ms"):
    if v is None:
        return "-"
    return ("%.1f%s" % (v, unit)) if isinstance(v, float) else ("%s%s" % (v, unit))


def summarize(rows):
    # (tag, layer, target) -> [ok], [ms]
    agg = defaultdict(lambda: {"ok": 0, "fail": 0, "ms": [], "errs": defaultdict(int)})
    for r in rows:
        if r.get("kind") or "layer" not in r:
            continue
        key = (r["tag"], r["layer"], r["target"])
        a = agg[key]
        if r.get("ok"):
            a["ok"] += 1
            if r.get("ms"):
                a["ms"].append(float(r["ms"]))
        else:
            a["fail"] += 1
            if r.get("err"):
                a["errs"][r["err"].split(":")[0]] += 1
    return agg


def outages(rows, layer="l3", need=3, gap_s=120):
    """同一 tag+target 连续 need 次失败算一次断流事件。"""
    seq = defaultdict(list)
    for r in rows:
        if r.get("kind") or r.get("layer") != layer:
            continue
        seq[(r["tag"], r["target"])].append(r)
    events = []
    for (tag, target), rs in seq.items():
        rs.sort(key=lambda x: x["ts"])
        run = []
        for r in rs:
            if r.get("ok"):
                if len(run) >= need:
                    events.append({"tag": tag, "target": target,
                                   "start": run[0]["ts"], "end": run[-1]["ts"],
                                   "secs": round(run[-1]["ts"] - run[0]["ts"], 1),
                                   "fails": len(run)})
                run = []
            else:
                run.append(r)
        if len(run) >= need:
            events.append({"tag": tag, "target": target, "start": run[0]["ts"],
                           "end": run[-1]["ts"], "open": True,
                           "secs": round(run[-1]["ts"] - run[0]["ts"], 1),
                           "fails": len(run)})
    return events


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--jsonl", nargs="+", required=True)
    ap.add_argument("--out", default="-")
    ap.add_argument("--title", default="稳定性报告")
    args = ap.parse_args()

    rows = []
    for p in args.jsonl:
        rows += load(p)
    if not rows:
        print("没有数据")
        return
    tags = sorted({r["tag"] for r in rows if r.get("tag")})
    starts = {r["tag"]: r for r in rows if r.get("kind") == "start"}
    stops = {r["tag"]: r for r in rows if r.get("kind") == "stop"}
    agg = summarize(rows)

    L = []
    L.append("# " + args.title)
    L.append("")
    L.append("| 探针 | 主机 | 轮数 | 层 |")
    L.append("|---|---|---|---|")
    for t in tags:
        st = starts.get(t, {})
        L.append("| %s | %s | %s | %s |" % (t, st.get("host", "?"),
                                            stops.get(t, {}).get("rounds", "-"),
                                            ",".join(st.get("layers", []))))
    L.append("")
    L.append("## 分层聚合（失败率 / p50 / p95 / max）")
    L.append("")
    L.append("| 探针 | 层 | 目标 | 成功 | 失败 | 失败率 | p50 | p95 | max | 主要错误 |")
    L.append("|---|---|---|---|---|---|---|---|---|---|")
    for (tag, layer, target), a in sorted(agg.items()):
        total = a["ok"] + a["fail"]
        fr = (100.0 * a["fail"] / total) if total else 0.0
        err = max(a["errs"].items(), key=lambda kv: kv[1])[0] if a["errs"] else ""
        L.append("| %s | %s | %s | %d | %d | %.1f%% | %s | %s | %s | %s |" % (
            tag, layer, target, a["ok"], a["fail"], fr,
            fmt(pct(a["ms"], 0.50)), fmt(pct(a["ms"], 0.95)),
            fmt(pct(a["ms"], 1.0)), err))
    L.append("")
    ev = outages(rows)
    L.append("## 断流事件（同一目标连续 ≥3 次失败，基于 l3）")
    L.append("")
    if not ev:
        L.append("无。")
    else:
        L.append("| 探针 | 目标 | 开始(UTC) | 持续 | 失败次数 | 未恢复 |")
        L.append("|---|---|---|---|---|---|")
        for e in ev:
            import datetime
            t0 = datetime.datetime.utcfromtimestamp(e["start"]).strftime("%H:%M:%S")
            L.append("| %s | %s | %s | %ss | %d | %s |" % (
                e["tag"], e["target"], t0, e["secs"], e["fails"],
                "是" if e.get("open") else ""))
    L.append("")
    L.append("## 一句话结论")
    L.append("")
    # 每个 tag 的 l3 总成功率
    score = {}
    for (tag, layer, target), a in agg.items():
        if layer != "l3":
            continue
        s = score.setdefault(tag, {"ok": 0, "fail": 0, "ms": []})
        s["ok"] += a["ok"]
        s["fail"] += a["fail"]
        s["ms"] += a["ms"]
    for tag, s in sorted(score.items()):
        total = s["ok"] + s["fail"]
        L.append("- **%s**：L3 成功 %d/%d（%.1f%%），p50 %s，p95 %s" % (
            tag, s["ok"], total, 100.0 * s["ok"] / total if total else 0,
            fmt(pct(s["ms"], 0.5)), fmt(pct(s["ms"], 0.95))))
    out = "\n".join(L) + "\n"
    if args.out == "-":
        print(out)
    else:
        with open(args.out, "w") as f:
            f.write(out)
        print("wrote " + args.out)


if __name__ == "__main__":
    main()
