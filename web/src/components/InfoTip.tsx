interface Props {
  text: string;
}

// A pure-CSS "what does this chart show" explanation bubble — :hover/:focus-within
// is enough for a small ⓘ trigger, no state and no popover dependency. aria-label
// carries the full text so screen readers get it without needing the hover.
export function InfoTip({ text }: Props) {
  return (
    <span className="info-tip" tabIndex={0} aria-label={text}>
      <span aria-hidden="true">ⓘ</span>
      <span className="info-tip-bubble" role="tooltip">
        {text}
      </span>
    </span>
  );
}
