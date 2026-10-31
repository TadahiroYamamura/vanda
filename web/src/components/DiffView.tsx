import type { DiffLine, DiffPart } from "../api";
import type { Lang } from "../highlight";
import { CodeLine, symbolsByLine, useTokens } from "./Code";

// 差分は API（docs/formats.md の entries[].diff）が作ったものを、そのまま見せる。

// 空白だけの差分は、背景だけでは見えにくい（改行だけなら幅もない）ので、印のクラスを付ける
function wsClass(text: string): string {
  if (!/^\s+$/.test(text)) return "diff-part";
  return text.includes("\n") ? "diff-part ws nl" : "diff-part ws";
}

// side が both なら、消した文字と足した文字の両方を 1 列で見せる
export function Parts(props: { parts: DiffPart[]; side: "before" | "after" | "both" }) {
  const hide = props.side === "before" ? "insert" : props.side === "after" ? "delete" : "";
  return (
    <>
      {props.parts
        .filter((p) => p.op !== hide)
        .map((p, i) =>
          p.op === "delete" ? (
            <del key={i} className={wsClass(p.text)}>
              {p.text}
            </del>
          ) : p.op === "insert" ? (
            <ins key={i} className={wsClass(p.text)}>
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

// LineDiff は、行の単位の差分。変更前と変更後をそれぞれ全体で色付けし（文法の状態が行をまたぐため）、行の差分の色を背景に、
// 書き換えた行の中の文字の単位の差分を、色付けの上に下線で重ねる。
export function LineDiff(props: { lines: DiffLine[]; lang: Lang | null }) {
  const beforeText = props.lines.filter((l) => l.op !== "insert").map((l) => l.text).join("\n");
  const afterText = props.lines.filter((l) => l.op !== "delete").map((l) => l.text).join("\n");
  const beforeTokens = useTokens(beforeText, props.lang);
  const afterTokens = useTokens(afterText, props.lang);
  const tla = props.lang === "tlaplus";
  const beforeSyms = tla ? symbolsByLine(beforeText) : null;
  const afterSyms = tla ? symbolsByLine(afterText) : null;
  return (
    <table className="diff-lines" data-lang={props.lang ?? undefined}>
      <tbody>
        {props.lines.map((l, i) => {
          const useAfter = l.op !== "delete";
          const idx = (useAfter ? l.after : l.before) ?? 0;
          const tokens = (useAfter ? afterTokens : beforeTokens)?.[idx - 1] ?? null;
          const syms = (useAfter ? afterSyms : beforeSyms)?.[idx - 1];
          const parts = l.parts ? l.parts.filter((p) => p.op !== (l.op === "delete" ? "insert" : "delete")) : null;
          return (
            <tr key={i} className={`diff-line diff-${l.op}`}>
              <td className="diff-no-before">{l.before ?? ""}</td>
              <td className="diff-no-after">{l.after ?? ""}</td>
              <td className="diff-mark">{mark[l.op]}</td>
              <td className="diff-text">
                <CodeLine line={l.text} tokens={tokens} symbols={syms} parts={parts} />
                {l.noNewline && <span className="diff-no-newline">（終わりに改行なし）</span>}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
