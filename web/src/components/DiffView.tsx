import type { DiffLine, DiffPart } from "../api";

// 差分は API（docs/formats.md の entries[].diff）が作ったものを、そのまま見せる。

export function Parts(props: { parts: DiffPart[]; side: "before" | "after" }) {
  const hide = props.side === "before" ? "insert" : "delete";
  return (
    <>
      {props.parts
        .filter((p) => p.op !== hide)
        .map((p, i) =>
          p.op === "delete" ? (
            <del key={i} className="diff-part">
              {p.text}
            </del>
          ) : p.op === "insert" ? (
            <ins key={i} className="diff-part">
              {p.text}
            </ins>
          ) : (
            <span key={i} className="diff-part">
              {p.text}
            </span>
          ),
        )}
    </>
  );
}

const mark = { equal: " ", delete: "−", insert: "+" };

export function LineDiff(props: { lines: DiffLine[] }) {
  return (
    <table className="diff-lines">
      <tbody>
        {props.lines.map((l, i) => (
          <tr key={i} className={`diff-line diff-${l.op}`}>
            <td className="diff-no-before">{l.before ?? ""}</td>
            <td className="diff-no-after">{l.after ?? ""}</td>
            <td className="diff-mark">{mark[l.op]}</td>
            <td className="diff-text">
              {l.parts ? <Parts parts={l.parts} side={l.op === "delete" ? "before" : "after"} /> : l.text}
              {l.noNewline && <span className="diff-no-newline">（終わりに改行なし）</span>}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
