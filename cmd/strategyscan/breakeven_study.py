"""Offline study of §8.4③'s breakeven-stop overlay (paper.Config.BreakevenAtR).

Feed it two -dump-trades CSVs from the SAME baseline population (same
-date-from/-date-to, same -market/-history-file, same -baseline-trade-sample)
that differ ONLY in -breakeven-at-r — one run at 0 (off), one at the R value
under test. Paired by (Date, Ticker) so the comparison is the same trade
under two exit rules, not two different samples; same bootstrap-SE method as
stop_study.py's paired().

    strategyscan -market=us -history-file=CACHE -date-from=A -date-to=B \\
                 -breakeven-at-r=0 -dump-trades=off.csv
    strategyscan -market=us -history-file=CACHE -date-from=A -date-to=B \\
                 -breakeven-at-r=1 -dump-trades=on.csv
    python3 breakeven_study.py off.csv on.csv
"""
import csv, random, statistics, sys
from collections import defaultdict

OFF_PATH, ON_PATH = sys.argv[1], sys.argv[2]
NBOOT = 400
random.seed(20260902)


def load(path):
    rows = {}
    for r in csv.DictReader(open(path)):
        rows[(r["Date"], r["Ticker"])] = float(r["ExitRet"])
    return rows


def mean(xs):
    return sum(xs) / len(xs) if xs else float("nan")


off, on = load(OFF_PATH), load(ON_PATH)
shared = sorted(set(off) & set(on))
if len(shared) < 100:
    print(f"only {len(shared)} shared (Date, Ticker) trades between the two dumps — refusing to conclude anything")
    sys.exit(1)

diffs = [on[k] - off[k] for k in shared]
d = mean(diffs)
ms = [mean([diffs[random.randrange(len(diffs))] for _ in range(len(diffs))]) for _ in range(NBOOT)]
se = statistics.stdev(ms)
sigma = abs(d) / se if se > 0 else float("nan")

win_off = sum(1 for k in shared if off[k] > 0) / len(shared) * 100
win_on = sum(1 for k in shared if on[k] > 0) / len(shared) * 100

print(f"n={len(shared)} shared trades")
print(f"mean ExitRet off={mean([off[k] for k in shared]):+.4f}%  on={mean([on[k] for k in shared]):+.4f}%")
print(f"paired diff (on - off) = {d:+.4f}%  se={se:.4f}  sigma={sigma:.2f}")
print(f"win rate off={win_off:.1f}%  on={win_on:.1f}%")
