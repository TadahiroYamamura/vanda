// TLA+ を数学の記号で見せるための置き換え（CL-07）。表と決まりは docs/formats.md の「TLA+ の記号」。
// 見せ方だけで、判定には使わない。コピーしたときに元の文字へ戻せるよう、置き換えた所ごとに元の文字を持つ。

export interface Segment {
  text: string; // 見せる文字
  ascii: string; // リポジトリの文字
}

const SYMBOLS: [string, string][] = [
  ["<=>", "⇔"],
  ["|->", "↦"],
  ["==", "≜"],
  ["/\\", "∧"],
  ["\\/", "∨"],
  ["=>", "⇒"],
  ["~>", "↝"],
  ["~", "¬"],
  ["#", "≠"],
  ["/=", "≠"],
  ["<=", "≤"],
  ["=<", "≤"],
  [">=", "≥"],
  ["->", "→"],
  ["<<", "⟨"],
  [">>", "⟩"],
  ["[]", "□"],
  ["<>", "◇"],
  ["'", "′"],
];

const WORDS: Record<string, string> = {
  "\\land": "∧",
  "\\lor": "∨",
  "\\lnot": "¬",
  "\\neg": "¬",
  "\\equiv": "⇔",
  "\\A": "∀",
  "\\E": "∃",
  "\\in": "∈",
  "\\notin": "∉",
  "\\subseteq": "⊆",
  "\\cup": "∪",
  "\\union": "∪",
  "\\cap": "∩",
  "\\intersect": "∩",
  "\\X": "×",
  "\\times": "×",
  "\\leq": "≤",
  "\\geq": "≥",
  "\\o": "∘",
  "\\circ": "∘",
  "\\div": "÷",
};

const isWordChar = (c: string | undefined) => c !== undefined && /[A-Za-z0-9_]/.test(c);

// toSegments は、TLA+ の文を、置き換えた所と置き換えない所の並びにする。
// 文字列・コメントの中と、4 つ以上の - か = の並びは置き換えない。長いものから先にマッチさせる。
export function toSegments(src: string): Segment[] {
  const out: Segment[] = [];
  let plain = "";
  const flush = () => {
    if (plain) out.push({ text: plain, ascii: plain });
    plain = "";
  };
  let depth = 0; // 入れ子のブロックのコメントの深さ
  let i = 0;
  while (i < src.length) {
    const rest = src.slice(i);
    if (depth > 0) {
      if (rest.startsWith("(*")) {
        depth++;
        plain += "(*";
        i += 2;
      } else if (rest.startsWith("*)")) {
        depth--;
        plain += "*)";
        i += 2;
      } else {
        plain += src[i];
        i++;
      }
      continue;
    }
    if (rest.startsWith("(*")) {
      depth = 1;
      plain += "(*";
      i += 2;
      continue;
    }
    if (rest.startsWith("\\*")) {
      const end = src.indexOf("\n", i);
      const stop = end < 0 ? src.length : end;
      plain += src.slice(i, stop);
      i = stop;
      continue;
    }
    if (src[i] === '"') {
      let j = i + 1;
      while (j < src.length && src[j] !== '"' && src[j] !== "\n") j += src[j] === "\\" ? 2 : 1;
      plain += src.slice(i, j + 1);
      i = j + 1;
      continue;
    }
    const run = /^(-{4,}|={4,})/.exec(rest);
    if (run) {
      plain += run[0];
      i += run[0].length;
      continue;
    }
    if (src[i] === "\\") {
      const m = /^\\[A-Za-z]+/.exec(rest);
      if (m && WORDS[m[0]] && !isWordChar(src[i + m[0].length])) {
        flush();
        out.push({ text: WORDS[m[0]], ascii: m[0] });
        i += m[0].length;
        continue;
      }
    }
    const sym = SYMBOLS.find(([a]) => rest.startsWith(a));
    if (sym) {
      flush();
      out.push({ text: sym[1], ascii: sym[0] });
      i += sym[0].length;
      continue;
    }
    plain += src[i];
    i++;
  }
  flush();
  return out;
}
