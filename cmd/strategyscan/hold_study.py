"""Offline study of the max-holding-period (time stop).

Feed it a sweep from:

    strategyscan -market=us -range=10y -history-file=CACHE \\
                 -baseline-trade-sample=10 -hold-sweep=5,10,20,30,40,60,90,120

Every entry is replayed at every horizon in one pass, so the horizons are
paired by construction: the comparison between two of them uses identical
entries and differs only in when an unresolved trade is forced closed.

Two metrics, because they disagree and only one of them answers a
capital-allocation question:

  ret/trade  — what a longer hold obviously wins, by staying in the market
  ann%       — mean return scaled by mean holding days to 252 trading days,
               which is what matters when the capital could be in the next
               trade instead

Significance is a date-clustered bootstrap (resample entry DATES, not
trades) because same-day entries co-move.
"""
import csv, random, statistics, sys
from collections import defaultdict

PATH = sys.argv[1]
SPLIT = "2021-11-01"
NBOOT = 400
random.seed(20260826)

# key: (strategy, hold) -> list of (date, ret, holddays)
rows = defaultdict(list)
holds, strats = set(), set()
for r in csv.DictReader(open(PATH)):
    h = int(r["Hold"])
    rows[(r["Strategy"], h)].append((r["Date"], float(r["ExitRet"]), int(r["HoldDays"])))
    holds.add(h)
    strats.add(r["Strategy"])
holds = sorted(holds)
strats = sorted(strats)


def mean(xs):
    return sum(xs) / len(xs) if xs else float("nan")


def stats(recs):
    if not recs:
        return None
    rets = [x[1] for x in recs]
    days = [x[2] for x in recs]
    mr, md = mean(rets), mean(days)
    return {
        "n": len(recs),
        "ret": mr,
        "win": sum(1 for x in rets if x > 0) / len(rets) * 100,
        "days": md,
        "ann": mr / md * 252 if md > 0 else float("nan"),
    }


def by_date(recs):
    d = defaultdict(list)
    for date, ret, _ in recs:
        d[date].append(ret)
    return d


def paired_sigma(a, b):
    """Per-date mean difference (b - a) over dates both horizons share,
    bootstrapped over dates."""
    da, db = by_date(a), by_date(b)
    shared = sorted(set(da) & set(db))
    diffs = [mean(db[d]) - mean(da[d]) for d in shared]
    if len(diffs) < 5:
        return float("nan"), float("nan")
    ms = [mean([diffs[random.randrange(len(diffs))] for _ in range(len(diffs))]) for _ in range(NBOOT)]
    se = statistics.stdev(ms)
    return mean(diffs), (abs(mean(diffs)) / se if se > 0 else float("nan"))


SLICES = [("HOLDOUT 2016-11..2021-10", lambda d: d < SPLIT),
          ("in-sample 2021-11..", lambda d: d >= SPLIT)]

for label, keep in SLICES:
    print(f"\n{'='*78}\n{label}\n{'='*78}")
    for strat in strats:
        base = [x for x in rows[(strat, 60)] if keep(x[0])]
        if len(base) < 100:
            print(f"\n{strat}: {len(base)} trades at h=60 — too few to read, skipped")
            continue
        print(f"\n{strat}")
        print(f"  {'hold':>5s} {'n':>7s} {'ret/trade':>10s} {'win%':>6s} {'days':>6s} {'ann%':>8s}   {'vs h=60':>9s} {'sigma':>6s}")
        for h in holds:
            recs = [x for x in rows[(strat, h)] if keep(x[0])]
            st = stats(recs)
            if st is None:
                continue
            d, sig = (0.0, float("nan")) if h == 60 else paired_sigma(base, recs)
            vs = "  (ref)" if h == 60 else f"{d:+8.3f}%"
            sg = "" if h == 60 else f"{sig:6.2f}"
            print(f"  {h:5d} {st['n']:7d} {st['ret']:+9.2f}% {st['win']:5.1f}% {st['days']:6.1f} {st['ann']:+7.1f}%   {vs:>9s} {sg:>6s}")


# The decision view: annualized return per horizon, both slices side by side.
# A per-screen time stop is only worth adding if some screen's best horizon
# is the same in both — otherwise it is a curve fit to one half of the data.
print(f"\n\n{'='*78}\nANNUALIZED %, both slices — a screen wants a time stop only if its\npeak lands in the same place twice\n{'='*78}")
hdr = "  ".join(f"{h:>6d}" for h in holds)
print(f"{'strategy':22s} {'slice':10s} {hdr}   best")
for strat in strats:
    for label, keep in SLICES:
        cells, anns = [], {}
        for h in holds:
            st = stats([x for x in rows[(strat, h)] if keep(x[0])])
            if st is None or st["n"] < 100:
                cells.append(f"{'-':>6s}")
                continue
            anns[h] = st["ann"]
            cells.append(f"{st['ann']:+6.1f}")
        best = max(anns, key=anns.get) if anns else "-"
        tag = "HOLDOUT" if "HOLDOUT" in label else "in-samp"
        print(f"{strat:22s} {tag:10s} " + "  ".join(cells) + f"   {best}")
