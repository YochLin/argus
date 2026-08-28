"""Offline significance test for 網 6 (post-gap drift, Phase 25 §2).

Reads one or more strategyscan_results_*.csv runs and, for each, compares the
screen's full-trade replay returns against the random-entry control that ran
under identical exit rules in the same run and the same date window.

Two readings per slice:

  overall      screen's mean exit return minus the control's, over every
               control trade in the window. This is the pre-registered §2.5
               number.
  date-matched same difference, but the control is restricted to the dates
               the screen actually fired on. PEAD is an event screen and
               events cluster in earnings season, so the overall number can
               flatter (or punish) it purely for WHEN it traded. If the two
               readings disagree, the overall one is measuring the calendar.

Significance is a date-clustered bootstrap — resample DATES with replacement,
not trades — because same-day trades co-move. Same method as rank_study.py
and the exit-layer study. Stdlib only, no pandas.

The screen rows come from a run's strategyscan_results_*.csv; the control
rows come from the same run's -dump-trades file, because writeCSV drops the
baseline (it is a few hundred thousand rows). So every run needs both:

    strategyscan -market=us -history-file=CACHE -range=10y \
                 -date-from=... -date-to=... -dump-trades=DUMP

    python3 pead_study.py "LABEL=results.csv,dump.csv" ...

The 2026-08-27 run quoted in CheckPostGapDriftExact's doc comment was, with
CACHE built by `-build-history` over each universe:

    for slice in "-date-from=2016-11-01 -date-to=2021-10-31" "-date-from=2021-11-01"; do
      strategyscan -market=us -range=10y -dump-trades=dump.csv $slice \
                   -history-file=sp400_daily.csv -universe=sp400
      strategyscan -market=us -range=10y -dump-trades=dump.csv $slice \
                   -history-file=us_daily.csv
    done

Note the -slippage-pct sensitivity §2.6 asked for is pointless against this
control: slippage is a constant per-round-trip charge that the control pays
too, so it cancels exactly out of every excess figure. It only moves absolute
return. See the same doc comment.
"""
import csv, random, statistics, sys
from collections import defaultdict

NBOOT = 400
random.seed(20260827)
SCREENS = ["post_gap_drift", "post_gap_drift_t1", "post_gap_drift_confirmed",
           "squeeze_breakout", "box_bottom", "trend_breakout", "trend_pullback",
           # Phase 25 §8.4①: same-signal delayed-entry variants (see
           # cmd/strategyscan/main.go's entryConfirmDaysFlag/confirmableStrategies).
           "squeeze_breakout_confirm", "box_bottom_confirm", "trend_breakout_confirm", "trend_pullback_confirm",
           "insider_cluster_buy"]  # Phase 25 §8.2


def load_screens(path):
    """-> {strategy: {date: [exit returns]}}, replayed trades only."""
    out = defaultdict(lambda: defaultdict(list))
    for r in csv.DictReader(open(path)):
        if r["HasTrade"] != "true":
            continue
        out[r["Strategy"]][r["Date"]].append(float(r["TradeExitRet"]))
    return out


def load_control(path):
    """-> {date: [exit returns]} from a -dump-trades file."""
    out = defaultdict(list)
    for r in csv.DictReader(open(path)):
        out[r["Date"]].append(float(r["ExitRet"]))
    return out


def diff(screen_by_date, ctrl_by_date):
    """Mean(screen) - mean(control), and a date-clustered bootstrap SE of it."""
    s_flat = [x for v in screen_by_date.values() for x in v]
    c_flat = [x for v in ctrl_by_date.values() for x in v]
    if len(s_flat) < 30 or not c_flat:
        return None
    effect = statistics.fmean(s_flat) - statistics.fmean(c_flat)

    s_dates, c_dates = list(screen_by_date), list(ctrl_by_date)
    boots = []
    for _ in range(NBOOT):
        s = [x for d in random.choices(s_dates, k=len(s_dates)) for x in screen_by_date[d]]
        c = [x for d in random.choices(c_dates, k=len(c_dates)) for x in ctrl_by_date[d]]
        if s and c:
            boots.append(statistics.fmean(s) - statistics.fmean(c))
    se = statistics.stdev(boots) if len(boots) > 1 else float("nan")
    return effect, se, len(s_flat), len(c_flat)


def report(label, paths):
    results, dump = paths.split(",")
    data = load_screens(results)
    ctrl = load_control(dump)
    print(f"\n=== {label} ===   control trades: {sum(len(v) for v in ctrl.values())}")
    print(f"{'screen':<20} {'n':>6} {'excess':>9} {'SE':>7} {'sigma':>7}   "
          f"{'n':>5} {'matched':>9} {'SE':>7} {'sigma':>7}")
    for s in SCREENS:
        if s not in data:
            continue
        rows = [f"{s:<20}"]
        for ctrl_slice in (ctrl, {d: v for d, v in ctrl.items() if d in data[s]}):
            r = diff(data[s], ctrl_slice)
            if r is None:
                rows.append(f"{sum(len(v) for v in data[s].values()):>6} {'too few':>34}")
                continue
            effect, se, ns, _ = r
            sigma = abs(effect) / se if se else float("nan")
            rows.append(f"{ns:>6} {effect:>+8.2f}% {se:>6.2f} {sigma:>6.1f}o")
        print("  ".join(rows))


for arg in sys.argv[1:]:
    label, _, path = arg.partition("=")
    report(label, path)
