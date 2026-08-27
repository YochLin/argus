"""Offline significance test for 網5【主力跟單】on its new data source (Phase
25 §4): TWSE's own T86 report instead of FinMind's secondhand copy of the
same underlying data (see internal/data/twse_t86.go).

Same method as pead_study.py (see that file for the full explanation of the
two readings and the date-clustered bootstrap) — this is the trust_follow
analog, not a different statistic. The point of this script existing
separately, rather than adding "trust_follow" to pead_study.py's SCREENS
list, is that this run answers a different pre-registered question (Phase 25
§4.4: did swapping the DATA SOURCE change the prior FinMind-era conclusion
about 網5, not whether 網5 has an edge at all — see §2.3/§4.5).

Conditions were deliberately left untouched from the existing
CheckTrustFollowExact — only the data source (finmind.GetTrustNetSeries ->
T86 cache) changed, per Phase 25 §4's "keep conditions identical, treat
threshold changes as a separate follow-up" instruction.

Usage — build the T86 cache once (slow, paced to avoid TWSE's WAF — see
twse_t86.go/t86_cache.go doc comments), then run both time slices against it:

    strategyscan -market=tw -build-t86-cache=t86.csv \
                 -date-from=2016-08-01 -date-to=2026-08-27

    for slice in "-date-from=2016-11-01 -date-to=2021-10-31" "-date-from=2021-11-01"; do
      strategyscan -market=tw -range=10y -t86-file=t86.csv $slice \
                   -dump-trades=dump.csv
    done

    python3 t86_study.py "HOLDOUT 2016-11..2021-10=results.csv,dump.csv" \
                         "in-sample 2021-11..=results2.csv,dump2.csv"
"""
import csv, random, statistics, sys
from collections import defaultdict

NBOOT = 400
random.seed(20260827)
SCREENS = ["trust_follow"]


def load_screens(path):
    out = defaultdict(lambda: defaultdict(list))
    for r in csv.DictReader(open(path)):
        if r["HasTrade"] != "true":
            continue
        out[r["Strategy"]][r["Date"]].append(float(r["TradeExitRet"]))
    return out


def load_control(path):
    out = defaultdict(list)
    for r in csv.DictReader(open(path)):
        out[r["Date"]].append(float(r["ExitRet"]))
    return out


def diff(screen_by_date, ctrl_by_date):
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
    print(f"{'screen':<15} {'n':>6} {'excess':>9} {'SE':>7} {'sigma':>7}   "
          f"{'n':>5} {'matched':>9} {'SE':>7} {'sigma':>7}")
    for s in SCREENS:
        if s not in data:
            continue
        rows = [f"{s:<15}"]
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
