"""Offline study of the initial stop width, in R.

Feed it a sweep from:

    strategyscan -market=us -range=10y -history-file=CACHE \\
                 -baseline-trade-sample=10 -stop-sweep=1,1.5,2,2.5,3,4,5

Percentage return cannot compare two stop widths. paper.SuggestShares sizes
a position so that a stop-out loses a fixed share of equity, so a wider stop
buys fewer shares and the same +5% price move is a different amount of
money. R — (exit-entry)/(entry-stop) — is what stays comparable: it is the
P&L per unit of risk actually taken, and at a fixed risk budget the
portfolio return is proportional to it.

Friction is reported separately because it is not neutral across widths: a
fixed percentage of price becomes a LARGER share of a tighter stop, so part
of any tight-stop penalty is the slippage assumption rather than the market.
gross R adds the friction back so the two can be told apart.

R alone would still mislead, because simulateTrade replays with the position
cap OFF (MaxPositionPct = 0, so the simulated BUY always fills). Risk-based
sizing puts 100*riskPct/stopPct percent of equity into a trade, which for a
tight stop exceeds any sane concentration limit — at the live 1% risk a 2.5%
stop asks for 39% of the account. PAPER_MAX_POSITION_PCT truncates that, and
a truncated position earns proportionally less, so "capped R" scales each
width by how much of its implied position actually survives the cap. That is
the column to read for a live decision; plain R is what an uncapped account
would have earned.
"""
import csv, random, statistics, sys
from collections import defaultdict

PATH = sys.argv[1]
FRICTION_PCT = float(sys.argv[2]) if len(sys.argv) > 2 else 0.2  # US: 0.1%/side round trip, no commission
# Live paper-account sizing: RISK_PCT_PER_TRADE defaults to 0, which
# bot.paperConfig turns into 1.0; PAPER_MAX_POSITION_PCT defaults to 25.
RISK_PCT = float(sys.argv[3]) if len(sys.argv) > 3 else 1.0
MAX_POSITION_PCT = float(sys.argv[4]) if len(sys.argv) > 4 else 25.0


def cap_scale(stop_pct):
    """Fraction of the risk-based position that survives the concentration
    cap. 1.0 means the cap does not bind at this stop width."""
    want = 100 * RISK_PCT / stop_pct   # position as % of equity
    return min(1.0, MAX_POSITION_PCT / want)
SPLIT = "2021-11-01"
NBOOT = 400
random.seed(20260826)

rows = defaultdict(list)   # (strategy, stopATR) -> [(date, R, grossR, ret, days, reason)]
widths, strats = set(), set()
for r in csv.DictReader(open(PATH)):
    entry, stop = float(r["Entry"]), float(r["Stop"])
    if entry <= 0 or stop <= 0 or stop >= entry:
        continue
    stop_pct = (entry - stop) / entry * 100
    ret = float(r["ExitRet"])
    w = float(r["StopATR"])
    rows[(r["Strategy"], w)].append(
        (r["Date"], ret / stop_pct, (ret + FRICTION_PCT) / stop_pct, ret, int(r["HoldDays"]),
         r["ExitReason"], stop_pct, ret / stop_pct * cap_scale(stop_pct)))
    widths.add(w)
    strats.add(r["Strategy"])
widths = sorted(widths)
strats = sorted(strats)


def mean(xs):
    return sum(xs) / len(xs) if xs else float("nan")


def by_date(recs, idx):
    d = defaultdict(list)
    for rec in recs:
        d[rec[0]].append(rec[idx])
    return d


def paired(a, b, idx=1):
    da, db = by_date(a, idx), by_date(b, idx)
    shared = sorted(set(da) & set(db))
    diffs = [mean(db[d]) - mean(da[d]) for d in shared]
    if len(diffs) < 5:
        return float("nan"), float("nan")
    ms = [mean([diffs[random.randrange(len(diffs))] for _ in range(len(diffs))]) for _ in range(NBOOT)]
    se = statistics.stdev(ms)
    return mean(diffs), (abs(mean(diffs)) / se if se > 0 else float("nan"))


SLICES = [("HOLDOUT 2016-11..2021-10", lambda d: d < SPLIT),
          ("in-sample 2021-11..", lambda d: d >= SPLIT)]

REF = 2.0  # the live StopATRMult

for label, keep in SLICES:
    print(f"\n{'='*94}\n{label}\n{'='*94}")
    for strat in strats:
        base = [x for x in rows[(strat, REF)] if keep(x[0])]
        if len(base) < 100:
            continue
        print(f"\n{strat}")
        print(f"  {'ATR':>4s} {'n':>7s} {'stop%':>6s} {'pos%':>6s} {'ret%':>7s} {'win%':>6s} {'days':>5s} "
              f"{'mean R':>7s} {'capped R':>9s} {'R/yr':>7s}   {'vs 2 ATR':>9s} {'sigma':>6s} {'stopped%':>8s}")
        for w in widths:
            recs = [x for x in rows[(strat, w)] if keep(x[0])]
            if not recs:
                continue
            R = mean([x[1] for x in recs])
            days = mean([x[4] for x in recs])
            stopped = sum(1 for x in recs if x[5] == "stop") / len(recs) * 100
            d, sig = (0.0, float("nan")) if w == REF else paired(base, recs)
            vs = "  (ref)" if w == REF else f"{d:+8.3f}"
            sg = "" if w == REF else f"{sig:6.2f}"
            sp = mean([x[6] for x in recs])
            print(f"  {w:4.1f} {len(recs):7d} {sp:5.2f}% "
                  f"{min(MAX_POSITION_PCT, 100*RISK_PCT/sp):5.1f}% "
                  f"{mean([x[3] for x in recs]):+6.2f}% {sum(1 for x in recs if x[3] > 0)/len(recs)*100:5.1f}% "
                  f"{days:5.1f} {R:+7.3f} {mean([x[7] for x in recs]):+9.3f} "
                  f"{R/days*252:+7.2f}   {vs:>9s} {sg:>6s} {stopped:7.1f}%")

print(f"\n\n{'='*94}\nCAPPED R by stop width (position cap applied), both slices — a width is only\nworth moving to if it wins in both\n{'='*94}")
hdr = "  ".join(f"{w:>7.1f}" for w in widths)
print(f"{'strategy':20s} {'slice':9s} {hdr}   best")
for strat in strats:
    for label, keep in SLICES:
        cells, vals = [], {}
        for w in widths:
            recs = [x for x in rows[(strat, w)] if keep(x[0])]
            if len(recs) < 100:
                cells.append(f"{'-':>7s}")
                continue
            vals[w] = mean([x[7] for x in recs])
            cells.append(f"{vals[w]:+7.3f}")
        best = max(vals, key=vals.get) if vals else "-"
        print(f"{strat:20s} {'HOLDOUT' if 'HOLDOUT' in label else 'in-samp':9s} " + "  ".join(cells) + f"   {best}")
