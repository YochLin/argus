"""Offline study of the candidate-ranking layer.

service.RankAndTruncateCandidates scores each day's candidate pool with an
equal-weight blend of min-max-normalized factors and keeps the top n. This
replays that ranking over cmd/strategyscan's random-entry control: for each
date, score every name that has a replayed trade, keep the top fraction, and
compare the kept trades' mean exit return against the whole pool's.

Feed it a dump from:

    strategyscan -market=us -range=10y -history-file=CACHE \
                 -dump-trades=DUMP -baseline-trade-sample=10

Significance is a date-clustered bootstrap — resample DATES with
replacement, not trades — because same-day trades co-move, the same reason
the exit-layer study clustered. Stdlib only, no pandas.

The numbers this prints are the ones quoted in
service.RankAndTruncateCandidates' doc comment.
"""
import csv, random, statistics, sys
from collections import defaultdict

PATH = sys.argv[1]
SPLIT = "2021-11-01"   # same in/out-of-sample boundary the exit study used
NBOOT = 400
random.seed(20260826)
FACTORS = ["RS63", "RS252", "Mom12_1", "DollarVol20", "SupportDist", "AbsLevelDist"]

by_date = defaultdict(list)
for r in csv.DictReader(open(PATH)):
    rec = {"ret": float(r["ExitRet"])}
    for f in FACTORS:
        rec[f] = float(r[f]) if r[f] != "" else None
    by_date[r["Date"]].append(rec)
dates = sorted(d for d, v in by_date.items() if len(v) >= 20)

SCHEMES = {
    "live_old  RS63+$vol+absDist": ([("RS63", 1), ("DollarVol20", 1), ("AbsLevelDist", -1)], 0.0),
    "live_new  RS63+$vol+supDist": ([("RS63", 1), ("DollarVol20", 1), ("SupportDist", -1)], 0.0),
    "rs63_only":                   ([("RS63", 1)], 0.0),
    "rs63 + $vol gate (drop 20%)": ([("RS63", 1)], 0.2),
    "rs63 + $vol gate (drop 50%)": ([("RS63", 1)], 0.5),
    "rs252_only":                  ([("RS252", 1)], 0.0),
    "mom12_1_only":                ([("Mom12_1", 1)], 0.0),
}


def norm01(vals):
    present = [v for v in vals if v is not None]
    if not present:
        return [0.5] * len(vals)
    lo, hi = min(present), max(present)
    if hi == lo:
        return [0.5] * len(vals)
    return [0.5 if v is None else (v - lo) / (hi - lo) for v in vals]


def mean(xs):
    return sum(xs) / len(xs) if xs else float("nan")


def boot(diffs):
    n = len(diffs)
    ms = [mean([diffs[random.randrange(n)] for _ in range(n)]) for _ in range(NBOOT)]
    se = statistics.stdev(ms)
    return se, (abs(mean(diffs)) / se if se > 0 else float("nan"))


def run(spec, gate, frac, date_set):
    """gate = fraction of the day's pool dropped from the BOTTOM by dollar
    volume before ranking — the 'liquidity is a threshold, not a score'
    proposal. Excess is measured against the same gated pool, so the gate's
    own effect is not double-counted into the ranking's."""
    diffs, tops, alls = [], [], []
    for d in date_set:
        recs = by_date[d]
        if gate > 0:
            have = [r for r in recs if r["DollarVol20"] is not None]
            have.sort(key=lambda r: r["DollarVol20"])
            recs = have[int(len(have) * gate):]
            if len(recs) < 20:
                continue
        cols = {c: norm01([r[c] for r in recs]) for c, _ in spec}
        sc = [sum(cols[c][i] if s > 0 else 1 - cols[c][i] for c, s in spec) for i in range(len(recs))]
        order = sorted(range(len(recs)), key=lambda i: -sc[i])
        k = max(1, round(len(recs) * frac))
        t = mean([recs[i]["ret"] for i in order[:k]])
        a = mean([r["ret"] for r in recs])
        diffs.append(t - a)
        tops.append(t)
        alls.append(a)
    return diffs, mean(tops), mean(alls)


ins = [d for d in dates if d < SPLIT]
oos = [d for d in dates if d >= SPLIT]
# The exit study called 2021-11 onward "in-sample" because that is the window
# the screens were designed against; the EARLIER slice is the honest holdout.
slices = [("HOLDOUT 2016-11..2021-10", ins), ("in-sample 2021-11..2026-08", oos), ("full", dates)]

for frac in (0.2, 0.1):
    print(f"\n########## keep top {frac:.0%} ##########")
    for label, ds in slices:
        print(f"\n--- {label} ({len(ds)} dates) ---")
        print(f"{'scheme':30s} {'kept':>8s} {'pool':>8s} {'excess':>9s} {'SE':>7s} {'sigma':>6s}")
        print("-" * 72)
        for name, (spec, gate) in SCHEMES.items():
            diffs, t, a = run(spec, gate, frac, ds)
            se, sig = boot(diffs)
            print(f"{name:30s} {t:+7.2f}% {a:+7.2f}% {mean(diffs):+8.3f}% {se:6.3f} {sig:6.2f}")

print("\n########## PAIRED, per slice (top 20%) ##########")
for label, ds in slices:
    old, _, _ = run(*SCHEMES["live_old  RS63+$vol+absDist"], 0.2, ds)
    new, _, _ = run(*SCHEMES["live_new  RS63+$vol+supDist"], 0.2, ds)
    rs, _, _ = run(*SCHEMES["rs63_only"], 0.2, ds)
    for tag, a, b in (("old->new", old, new), ("old->rs63_only", old, rs), ("new->rs63_only", new, rs)):
        d = [y - x for x, y in zip(a, b)]
        se, sig = boot(d)
        print(f"  {label:28s} {tag:16s} {mean(d):+.4f}%  SE={se:.4f}  sigma={sig:.2f}")
