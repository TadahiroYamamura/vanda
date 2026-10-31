import { useMemo, useRef, useState } from "react";
import { toSegments } from "../tla";

// TlaCode は、TLA+ を数学の記号で見せ、コピーしたときはリポジトリの文字のまま（ASCII）にする（CL-07）。
export function TlaCode(props: { source: string }) {
  const segments = useMemo(() => toSegments(props.source), [props.source]);
  const ref = useRef<HTMLPreElement>(null);
  const [copied, setCopied] = useState<string | null>(null);

  const onCopy = (e: React.ClipboardEvent<HTMLPreElement>) => {
    const sel = window.getSelection();
    if (!sel || sel.rangeCount === 0 || !ref.current) return;
    const frag = sel.getRangeAt(0).cloneContents();
    frag.querySelectorAll<HTMLElement>("[data-ascii]").forEach((el) => {
      el.replaceWith(document.createTextNode(el.dataset.ascii ?? ""));
    });
    e.clipboardData.setData("text/plain", frag.textContent ?? "");
    e.preventDefault();
  };

  const copyAll = () => {
    navigator.clipboard.writeText(props.source).then(
      () => setCopied("コピーした（リポジトリの文字のまま）"),
      (err) => setCopied(`コピーできなかった（${String(err)}）`),
    );
  };

  return (
    <div className="tla">
      <div className="tla-tools">
        <button type="button" onClick={copyAll}>
          コピー
        </button>
        {copied && <span className="note">{copied}</span>}
      </div>
      <pre ref={ref} className="code" onCopy={onCopy}>
        {segments.map((s, i) =>
          s.text === s.ascii ? (
            s.text
          ) : (
            <span key={i} className="sym" data-ascii={s.ascii} title={s.ascii}>
              {s.text}
            </span>
          ),
        )}
      </pre>
    </div>
  );
}
