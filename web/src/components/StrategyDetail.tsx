import type { StrategyAlert } from "../api";
import type { Dictionary } from "../i18n";
import { STRAT_COLOR, stratChannelNote, stratText, stratValid } from "../strategies";

interface Props {
  dict: Dictionary;
  alert: StrategyAlert;
  ticker: string;
  onClose: () => void;
}

// The rail's detail for a picked strategy alert (Phase 27 P5): what the
// backtests concluded about the strategy, the text that was actually pushed
// (which already carries the conditions that held), and the LLM's call around
// that date.
export function StrategyDetail({ dict, alert, ticker, onClose }: Props) {
  const txt = stratText(dict, alert.type);
  const valid = stratValid(dict, alert.type, ticker);
  return (
    <div className="pat-detail">
      <div className="pat-head">
        <span className="pat-badge big strat" style={{ color: STRAT_COLOR, borderColor: STRAT_COLOR }}>
          {txt.code}
        </span>
        <span className="pat-name">{txt.name}</span>
        <button type="button" className="pat-close" onClick={onClose}>
          ✕
        </button>
      </div>
      <div className="pat-meta">
        <span>{stratChannelNote(dict, alert)}</span>
      </div>
      <div className="strat-valid-row">
        <span className={`strat-valid ${valid.tone}`}>{valid.label}</span>
        <span className="strat-valid-note">{valid.note}</span>
      </div>
      <div>
        <div className="pat-sub">{dict.stratMsgTitle}</div>
        <div className="strat-msg">{alert.message}</div>
      </div>
      <div>
        <div className="pat-sub">{dict.stratVerdictTitle}</div>
        {alert.verdict ? (
          <div className="strat-verdict">
            <span className={`strat-act ${alert.verdict.action}`}>{alert.verdict.action}</span>
            <span className="strat-verdict-date">{alert.verdict.date}</span>
            <div className="strat-reason">{alert.verdict.reason}</div>
          </div>
        ) : (
          <div className="strat-verdict-none">{dict.stratVerdictNone}</div>
        )}
      </div>
      <div className="pat-disclaimer">{dict.stratDisclaimer}</div>
    </div>
  );
}
