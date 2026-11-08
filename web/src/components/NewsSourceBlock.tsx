import { useEffect, useState, type FormEvent } from "react";
import { ApiError, blockNewsSource, fetchBlockedNewsSources, unblockNewsSource, type BlockedNewsSource } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
}

// The settings page's news-source blocklist. Unlike every other section
// there, each action writes straight to the database and applies on the
// next news fetch — no Save, no restart — which is why it renders apart
// from the .env groups above it.
export function NewsSourceBlock({ dict, onUnauthorized }: Props) {
  const [sources, setSources] = useState<BlockedNewsSource[] | null>(null);
  const [value, setValue] = useState("");
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);
  const [added, setAdded] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const [rowError, setRowError] = useState<string | null>(null);

  const load = () =>
    fetchBlockedNewsSources()
      .then((r) => setSources(r.sources ?? []))
      .catch(() => setSources([]));

  useEffect(() => {
    void load();
  }, []);

  async function add() {
    const source = value.trim();
    if (!source) {
      setAddError(dict.newsBlockErrEmpty);
      return;
    }
    setAdding(true);
    setAddError(null);
    setAdded(null);
    try {
      await blockNewsSource(source);
      setValue("");
      setAdded(source);
      await load();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized(add);
      else if (e instanceof ApiError && e.status === 409) setAddError(dict.newsBlockErrDup.replace("%s", source));
      else setAddError(dict.newsBlockErrSave.replace("%s", e instanceof ApiError ? e.message : dict.error));
    } finally {
      setAdding(false);
    }
  }

  async function unblock(source: string) {
    setPending(source);
    setRowError(null);
    setAdded(null);
    try {
      await unblockNewsSource(source);
      await load();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized(() => void unblock(source));
      else setRowError(source);
    } finally {
      setPending(null);
    }
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!adding) void add();
  }

  return (
    <section className="news-block" aria-labelledby="news-block-title">
      <header className="news-block-head">
        <h2 id="news-block-title">{dict.newsBlockTitle}</h2>
        <span className="news-block-badge">{dict.newsBlockBadge}</span>
        <p>{dict.newsBlockDesc}</p>
      </header>

      <form className="news-block-add" onSubmit={onSubmit}>
        <label htmlFor="news-block-input">{dict.newsBlockAddLabel}</label>
        <div className="news-block-add-row">
          <input
            id="news-block-input"
            type="text"
            autoComplete="off"
            value={value}
            placeholder={dict.newsBlockPlaceholder}
            disabled={adding}
            aria-invalid={!!addError}
            aria-describedby="news-block-hint"
            className={addError ? "news-block-input-error" : undefined}
            onChange={(e) => {
              setValue(e.target.value);
              setAddError(null);
              setAdded(null);
            }}
          />
          <button type="submit" disabled={adding}>
            {adding ? dict.newsBlocking : dict.newsBlockBtn}
          </button>
        </div>
        {addError && (
          <div className="news-block-error" role="alert">
            {addError}
          </div>
        )}
        {added && (
          <div className="news-block-ok" role="status">
            {dict.newsBlockOk.replace("%s", added)}
          </div>
        )}
        <span id="news-block-hint" className="news-block-hint">
          {dict.newsBlockAddHint}
        </span>
      </form>

      {sources === null ? (
        <div className="loading">{dict.loading}</div>
      ) : sources.length === 0 ? (
        <div className="news-block-empty">{dict.newsBlockEmpty}</div>
      ) : (
        <>
          <div className="news-block-count">{dict.newsBlockCount.replace("%s", String(sources.length))}</div>
          <ul className="news-block-list">
            {sources.map((s) => (
              <li key={s.source} className={rowError === s.source ? "news-block-row-failed" : undefined}>
                <div className={`news-block-row${pending === s.source ? " news-block-row-pending" : ""}`}>
                  <span className="news-block-name">{s.source}</span>
                  <span className="news-block-date mono">
                    {dict.newsBlockedOn} <time dateTime={s.createdAt.slice(0, 10)}>{s.createdAt.slice(0, 10)}</time>
                  </span>
                  <button
                    type="button"
                    disabled={pending !== null}
                    aria-label={`${dict.newsUnblock} ${s.source}`}
                    onClick={() => void unblock(s.source)}
                  >
                    {pending === s.source ? dict.newsUnblocking : dict.newsUnblock}
                  </button>
                </div>
                {rowError === s.source && (
                  <div className="news-block-row-error" role="alert">
                    <span>{dict.newsUnblockErr}</span>
                    <button type="button" onClick={() => void unblock(s.source)}>
                      {dict.newsRetry}
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}
