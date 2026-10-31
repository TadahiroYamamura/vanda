import { useEffect, useRef, useState } from "react";
import type { DiffPart } from "../api";
import { tokenize, type Lang, type Token } from "../highlight";
import { toSegments } from "../tla";

// 1 行を作るかけら。色付け・TLA+ の記号・文字の単位の差分の 3 つの切れ目で分ける。
interface Piece {
  text: string; // 見せる文字
  ascii: string; // リポジトリの文字
  style: Record<string, string>;
  diff: "delete" | "insert" | null;
}

// SymbolRange は、TLA+ の記号に置き換える所（行の中の位置）。
export interface SymbolRange {
  start: number;
  end: number;
  text: string;
}

// symbolsByLine は、TLA+ の文全体から記号に置き換える所を求め、行ごとに分ける（ブロックのコメントが行をまたぐため、文全体で求める）。
export function symbolsByLine(text: string): SymbolRange[][] {
  const lines = text.split("\n");
  const starts: number[] = [];
  let off = 0;
  for (const l of lines) {
    starts.push(off);
    off += l.length + 1;
  }
  const out: SymbolRange[][] = lines.map(() => []);
  let pos = 0;
  let li = 0;
  for (const s of toSegments(text)) {
    if (s.text !== s.ascii) {
      while (li + 1 < starts.length && starts[li + 1] <= pos) li++;
      out[li].push({ start: pos - starts[li], end: pos - starts[li] + s.ascii.length, text: s.text });
    }
    pos += s.ascii.length;
  }
  return out;
}

function pieces(line: string, tokens: Token[] | null, symbols: SymbolRange[], parts: DiffPart[] | null): Piece[] {
  const n = line.length;
  const styleAt: Record<string, string>[] = new Array(n).fill({});
  const cuts = new Set<number>([0, n]);
  let p = 0;
  for (const t of tokens ?? []) {
    for (let i = 0; i < t.content.length && p + i < n; i++) styleAt[p + i] = t.style;
    p += t.content.length;
    cuts.add(Math.min(p, n));
  }
  const diffAt: ("delete" | "insert" | null)[] = new Array(n).fill(null);
  p = 0;
  for (const part of parts ?? []) {
    for (let i = 0; i < part.text.length && p + i < n; i++) diffAt[p + i] = part.op === "equal" ? null : part.op;
    p += part.text.length;
    cuts.add(Math.min(p, n));
  }
  const symAt = new Map<number, SymbolRange>();
  const inSym = new Array(n).fill(false);
  for (const s of symbols) {
    symAt.set(s.start, s);
    for (let i = s.start; i < s.end; i++) inSym[i] = true;
    cuts.add(s.start);
    cuts.add(s.end);
  }
  const sorted = [...cuts].sort((a, b) => a - b);
  const out: Piece[] = [];
  for (let k = 0; k + 1 < sorted.length; k++) {
    const a = sorted[k];
    const b = sorted[k + 1];
    if (a >= b) continue;
    const sym = symAt.get(a);
    if (sym) {
      // 記号は 1 つのかけらにする（色付けや差分の切れ目が中にあっても、最初の文字の色と印を使う）
      out.push({ text: sym.text, ascii: line.slice(sym.start, sym.end), style: styleAt[a], diff: diffAt[a] });
      continue;
    }
    if (inSym[a]) continue;
    out.push({ text: line.slice(a, b), ascii: line.slice(a, b), style: styleAt[a], diff: diffAt[a] });
  }
  return out;
}

export function CodeLine(props: { line: string; tokens: Token[] | null; symbols?: SymbolRange[]; parts?: DiffPart[] | null }) {
  return (
    <>
      {pieces(props.line, props.tokens, props.symbols ?? [], props.parts ?? null).map((p, i) => {
        const sym = p.text !== p.ascii;
        const inner = (
          <span className={sym ? "tok sym" : "tok"} style={p.style} data-ascii={sym ? p.ascii : undefined} title={sym ? p.ascii : undefined}>
            {p.text}
          </span>
        );
        if (p.diff === "delete")
          return (
            <del key={i} className="diff-part">
              {inner}
            </del>
          );
        if (p.diff === "insert")
          return (
            <ins key={i} className="diff-part">
              {inner}
            </ins>
          );
        return <span key={i}>{inner}</span>;
      })}
    </>
  );
}

// useTokens は、text の色付けを読み込む。読み込むまでと、読み込めなかったときは null（色を付けずに見せる）。
export function useTokens(text: string, lang: Lang | null): Token[][] | null {
  const [tokens, setTokens] = useState<Token[][] | null>(null);
  useEffect(() => {
    setTokens(null);
    if (!lang) return;
    let live = true;
    tokenize(text, lang).then(
      (t) => {
        if (live) setTokens(t);
      },
      (e) => console.error(`${lang} の色付けができなかった`, e),
    );
    return () => {
      live = false;
    };
  }, [text, lang]);
  return tokens;
}

// CodeBlock は、色付けしたコードを見せる。TLA+ は数学の記号で見せ、コピーしたときはリポジトリの文字のまま（CL-07）。
export function CodeBlock(props: { source: string; lang: Lang | null }) {
  const tokens = useTokens(props.source, props.lang);
  const lines = props.source.split("\n");
  const symbols = props.lang === "tlaplus" ? symbolsByLine(props.source) : null;
  const ref = useRef<HTMLPreElement>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const onCopy = (e: React.ClipboardEvent<HTMLPreElement>) => {
    const sel = window.getSelection();
    if (!sel || sel.rangeCount === 0) return;
    const frag = sel.getRangeAt(0).cloneContents();
    frag.querySelectorAll<HTMLElement>("[data-ascii]").forEach((el) => el.replaceWith(document.createTextNode(el.dataset.ascii ?? "")));
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
    <div className={symbols ? "tla" : "code-block"}>
      {symbols && (
        <div className="tla-tools">
          <button type="button" onClick={copyAll}>
            コピー
          </button>
          {copied && <span className="note">{copied}</span>}
        </div>
      )}
      <pre ref={ref} className="code" onCopy={onCopy} data-lang={props.lang ?? undefined}>
        {lines.map((l, i) => (
          <span key={i} className="code-line">
            <CodeLine line={l} tokens={tokens?.[i] ?? null} symbols={symbols?.[i]} />
            {i + 1 < lines.length ? "\n" : ""}
          </span>
        ))}
      </pre>
    </div>
  );
}
