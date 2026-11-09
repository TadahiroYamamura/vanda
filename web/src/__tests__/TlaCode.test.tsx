// CL-07: TLA+ は、見せるときは数学の記号で、コピーしたときはリポジトリの文字のまま（ASCII）。
// 置き換えの表と決まりは docs/formats.md の「TLA+ の記号」。期待値は、その表から手で書く。
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TlaCode } from "../components/TlaCode";
import { readFileSync } from "node:fs";

// vitest は CSS を読み込まない（?raw も空になる）ので、ファイルを直に読む（jsdom では import.meta.url が file: でないので、vitest を動かす web/ からの道で読む）
const styles = readFileSync("src/styles.css", "utf8");

afterEach(cleanup);

// 表の書き方の全部。[書き方を含む TLA+ の文, 見せる文]
const table: [string, string, string][] = [
  ["==", "S == x", "S ≜ x"],
  ["/\\", "S == a /\\ b", "S ≜ a ∧ b"],
  ["\\land", "S == a \\land b", "S ≜ a ∧ b"],
  ["\\/", "S == a \\/ b", "S ≜ a ∨ b"],
  ["\\lor", "S == a \\lor b", "S ≜ a ∨ b"],
  ["~", "S == ~a", "S ≜ ¬a"],
  ["\\lnot", "S == \\lnot a", "S ≜ ¬ a"],
  ["\\neg", "S == \\neg a", "S ≜ ¬ a"],
  ["=>", "S == a => b", "S ≜ a ⇒ b"],
  ["<=>", "S == a <=> b", "S ≜ a ⇔ b"],
  ["\\equiv", "S == a \\equiv b", "S ≜ a ⇔ b"],
  ["\\A", "S == \\A x \\in T : P", "S ≜ ∀ x ∈ T : P"],
  ["\\E", "S == \\E x \\in T : P", "S ≜ ∃ x ∈ T : P"],
  ["#", "S == a # b", "S ≜ a ≠ b"],
  ["/=", "S == a /= b", "S ≜ a ≠ b"],
  ["<=", "S == a <= b", "S ≜ a ≤ b"],
  ["=<", "S == a =< b", "S ≜ a ≤ b"],
  ["\\leq", "S == a \\leq b", "S ≜ a ≤ b"],
  [">=", "S == a >= b", "S ≜ a ≥ b"],
  ["\\geq", "S == a \\geq b", "S ≜ a ≥ b"],
  ["'", "S == x' = x", "S ≜ x′ = x"],
  ["\\o", "S == f \\o g", "S ≜ f ∘ g"],
  ["\\circ", "S == f \\circ g", "S ≜ f ∘ g"],
  ["\\in", "S == a \\in T", "S ≜ a ∈ T"],
  ["\\notin", "S == a \\notin T", "S ≜ a ∉ T"],
  ["\\subseteq", "S == A \\subseteq B", "S ≜ A ⊆ B"],
  ["\\cup", "S == A \\cup B", "S ≜ A ∪ B"],
  ["\\union", "S == A \\union B", "S ≜ A ∪ B"],
  ["\\cap", "S == A \\cap B", "S ≜ A ∩ B"],
  ["\\intersect", "S == A \\intersect B", "S ≜ A ∩ B"],
  ["\\X", "S == A \\X B", "S ≜ A × B"],
  ["\\times", "S == A \\times B", "S ≜ A × B"],
  ["|->", "S == [a |-> 1]", "S ≜ [a ↦ 1]"],
  ["->", "S == [A -> B]", "S ≜ [A → B]"],
  ["<< >>", "S == <<a, b>>", "S ≜ ⟨a, b⟩"],
  ["[]", "S == []P", "S ≜ □P"],
  ["<>", "S == <>P", "S ≜ ◇P"],
  ["~>", "S == P ~> Q", "S ≜ P ↝ Q"],
  ["\\div", "S == a \\div b", "S ≜ a ÷ b"],
];

// 置き換えない所と、長いものから当てる決まり。[説明, TLA+ の文, 見せる文]
const rules: [string, string, string][] = [
  ["文字列の中は置き換えない", 'S == "a == b /\\ c"', 'S ≜ "a == b /\\ c"'],
  ["\\* のコメントの中は置き換えない", "S == x \\* a == b /\\ c", "S ≜ x \\* a == b /\\ c"],
  ["(* *) のコメントの中は置き換えない", "S == (* a == b *) x", "S ≜ (* a == b *) x"],
  ["4 つ以上の - だけの行は置き換えない", "---- MODULE M ----\n----\nS == x", "---- MODULE M ----\n----\nS ≜ x"],
  ["4 つ以上の = だけの行は置き換えない", "S == x\n====", "S ≜ x\n===="],
  ["\\ の後に英字が続く所は短い書き方を当てない", "S == \\inx", "S ≜ \\inx"],
  ["<=> は <= と > にしない", "S == a <=> b", "S ≜ a ⇔ b"],
  ["|-> は | と -> にしない", "S == [a |-> b]", "S ≜ [a ↦ b]"],
  ["~> は ~ と > にしない", "S == a ~> b", "S ≜ a ↝ b"],
  ["\\notin は \\in にしない", "S == a \\notin b", "S ≜ a ∉ b"],
  ["\\intersect は \\in にしない", "S == a \\intersect b", "S ≜ a ∩ b"],
];

// codeElement は、見せている文がちょうど入っている、いちばん内側の要素（コピーのボタンなどを含めない）。
function codeElement(container: HTMLElement, shown: string): HTMLElement {
  let found: HTMLElement | null = null;
  container.querySelectorAll<HTMLElement>("*").forEach((el) => {
    if (el.textContent === shown && (!found || found.contains(el))) found = el;
  });
  if (!found) throw new Error(`「${shown}」だけを持つ要素がない: ${container.innerHTML}`);
  return found;
}

// copyRange は、範囲を選んで、選んだ所の要素に copy を送り、クリップボードに書かれた文を返す。
// ブラウザは、選んだ文を含む要素に copy を送る。user-event の copy は、フォーカスのある要素に送るので使わない。
function copyRange(range: Range): string {
  const sel = window.getSelection();
  sel?.removeAllRanges();
  sel?.addRange(range);
  const data: Record<string, string> = {};
  const target = range.startContainer.nodeType === Node.TEXT_NODE ? range.startContainer.parentElement! : (range.startContainer as HTMLElement);
  const ev = new Event("copy", { bubbles: true, cancelable: true }) as ClipboardEvent;
  Object.defineProperty(ev, "clipboardData", {
    value: {
      setData: (type: string, v: string) => {
        data[type] = v;
      },
      getData: (type: string) => data[type] ?? "",
    },
  });
  const notCancelled = target.dispatchEvent(ev);
  // 取り消さなければ、ブラウザは見せている文をそのままコピーする
  return notCancelled ? sel?.toString() ?? "" : data["text/plain"] ?? "";
}

function copyAll(container: HTMLElement, shown: string): string {
  const range = document.createRange();
  range.selectNodeContents(codeElement(container, shown));
  return copyRange(range);
}

describe("TLA+ の表の書き方を数学の記号で見せコピーするとリポジトリの文字に戻る", () => {
  for (const [op, src, shown] of table) {
    // @vanda:evidence tla-math-symbols
    // @vanda:evidence tla-copy-ascii
    it(`${op} を見せるときは記号にしコピーすると元の文字に戻る`, async () => {
      const { container } = render(<TlaCode source={src} />);
      expect(container.textContent).toContain(shown);
      expect(copyAll(container, shown)).toBe(src);
    });
  }
});

// 名前を変数で付けた it には付けられないので、まとまりの describe に付ける
// @vanda:evidence tla-no-replace-in-strings-comments
describe("TLA+ の置き換えない所と長いものから当てる決まり", () => {
  for (const [name, src, shown] of rules) {
    it(name, async () => {
      const { container } = render(<TlaCode source={src} />);
      expect(container.textContent).toContain(shown);
      expect(copyAll(container, shown)).toBe(src);
    });
  }
});

// @vanda:evidence tla-copy-ascii
it("TLA+ の一部だけを選んでコピーしても選んだ所がリポジトリの文字に戻る", () => {
  const shown = "S ≜ a ∧ b";
  const { container } = render(<TlaCode source={"S == a /\\ b"} />);
  const el = codeElement(container, shown);
  // 見せている文の「a ∧ b」だけを選ぶ
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
  const texts: Text[] = [];
  for (let n = walker.nextNode(); n; n = walker.nextNode()) texts.push(n as Text);
  const findPos = (ch: string): [Text, number] => {
    for (const t of texts) {
      const i = (t.textContent ?? "").indexOf(ch);
      if (i >= 0) return [t, i];
    }
    throw new Error(`${ch} が見つからない`);
  };
  const [startNode, startAt] = findPos("a");
  const [endNode, endAt] = findPos("b");
  const range = document.createRange();
  range.setStart(startNode, startAt);
  range.setEnd(endNode, endAt + 1);
  expect(copyRange(range)).toBe("a /\\ b");
});

// @vanda:evidence tla-copy-ascii
it("TLA+ のコピーのボタンをクリックするとリポジトリの文字のままクリップボードに入る", async () => {
  const user = userEvent.setup();
  const src = "S == \\A x \\in T : x' # x";
  const { getByRole } = render(<TlaCode source={src} />);
  await user.click(getByRole("button", { name: /コピー/ }));
  expect(await navigator.clipboard.readText()).toBe(src);
});

// 表の 1 行の、見せる文に出る記号と、その元の ASCII の文字数。[記号, 文字数] を出てくる順に並べる。
// どの行も先頭の == が ≜ になる。<< >> は ⟨ と ⟩ の 2 つ（どちらも 2 文字）。\A と \E の行の ∈ は \in（3 文字）。
// ほかは書き方の文字数。
function expectedSymbols(op: string, src: string, shown: string): [string, number][] {
  const syms = [...shown].filter((c) => !src.includes(c));
  return syms.map((c, i): [string, number] => {
    if (i === 0) return [c, 2];
    if (op === "<< >>") return [c, 2];
    if (c === "∈") return [c, 3];
    return [c, op.length];
  });
}

// withStyles は、画面の styles.css を文書に入れる（jsdom では App.tsx の import を通らないため）。
function withStyles(): () => void {
  const el = document.createElement("style");
  el.textContent = styles;
  document.head.appendChild(el);
  return () => el.remove();
}

// glyphOf は、記号の箱の中で、記号の文字だけを持ついちばん内側の要素を返す。
function glyphOf(box: HTMLElement): HTMLElement {
  let found: HTMLElement = box;
  box.querySelectorAll<HTMLElement>("*").forEach((el) => {
    if (el.textContent === box.textContent) found = el;
  });
  return found;
}

describe("TLA+ の記号は元の ASCII の文字数ぶんの幅の箱の中央に描く", () => {
  for (const [op, src, shown] of table) {
    // @vanda:evidence tla-symbol-keeps-width
    it(`${op} の記号は元の文字数ぶんの等幅の文字の幅を取り中央に寄せる`, () => {
      const remove = withStyles();
      try {
        const { container } = render(<TlaCode source={src} />);
        const boxes = [...container.querySelectorAll<HTMLElement>(".sym")];
        const want = expectedSymbols(op, src, shown);
        expect(boxes.map((b) => b.textContent)).toEqual(want.map(([c]) => c));
        boxes.forEach((box, i) => {
          // ch は、箱の書体（等幅）の 1 文字の幅
          expect(box.style.width, `${want[i][0]} の幅`).toBe(`${want[i][1]}ch`);
          const cs = getComputedStyle(box);
          expect(cs.display, `${want[i][0]} の箱が幅を持てない`).toBe("inline-block");
          expect(cs.textAlign, `${want[i][0]} が中央に寄っていない`).toBe("center");
        });
      } finally {
        remove();
      }
    });
  }
});

// unicodeRangeCovers は、@font-face の unicode-range（なければ全部）が、文字を含むかを返す。
function unicodeRangeCovers(range: string, ch: string): boolean {
  if (range.trim() === "") return true;
  const cp = ch.codePointAt(0)!;
  return range.split(",").some((part) => {
    const m = part.trim().match(/^U\+([0-9A-F?]+)(?:-([0-9A-F]+))?$/i);
    if (!m) return false;
    if (m[1].includes("?")) {
      return cp >= parseInt(m[1].replace(/\?/g, "0"), 16) && cp <= parseInt(m[1].replace(/\?/g, "F"), 16);
    }
    const lo = parseInt(m[1], 16);
    const hi = m[2] ? parseInt(m[2], 16) : lo;
    return cp >= lo && cp <= hi;
  });
}

// fontFaces は、styles.css の @font-face のうち、書体の名前が family のものの、項目の名前と値を返す。
// jsdom は @font-face の src を読まないので、CSS の文から取る。
function fontFaces(family: string): Map<string, string>[] {
  const blocks = styles.replace(/\/\*[\s\S]*?\*\//g, "").match(/@font-face\s*\{[^}]*\}/g) ?? [];
  return blocks
    .map((b) => {
      const props = new Map<string, string>();
      for (const decl of b.replace(/^@font-face\s*\{|\}$/g, "").split(";")) {
        const at = decl.indexOf(":");
        if (at > 0) props.set(decl.slice(0, at).trim(), decl.slice(at + 1).trim());
      }
      return props;
    })
    .filter((p) => (p.get("font-family") ?? "").replace(/["']/g, "") === family);
}

describe("TLA+ の記号は画面に同梱した Noto Sans Math で描く", () => {
  // 表の記号の全部（≜ と、各行の記号）
  const allSymbols = [...new Set(table.flatMap(([op, src, shown]) => expectedSymbols(op, src, shown).map(([c]) => c)))];

  // @vanda:evidence tla-symbol-font
  it("画面の書体の定めにNoto Sans Mathが同梱のファイルを指してありTLAの記号を全部含む範囲で定めてある", () => {
    const faces = fontFaces("Noto Sans Math");
    expect(faces.length, "Noto Sans Math の @font-face がない").toBeGreaterThan(0);
    for (const f of faces) {
      const src = f.get("src") ?? "";
      // OS の書体（local()）でなく、同梱した @fontsource/noto-sans-math のファイルを読む
      expect(src).not.toMatch(/local\(/);
      expect(src).toMatch(/url\(["']?@fontsource\/noto-sans-math\/files\/[^)]+\.woff2["']?\)/);
    }
    for (const c of allSymbols) {
      expect(
        faces.some((f) => unicodeRangeCovers(f.get("unicode-range") ?? "", c)),
        `${c}（U+${c.codePointAt(0)!.toString(16).toUpperCase()}）が Noto Sans Math の範囲にない`,
      ).toBe(true);
    }
  });

  for (const [op, src, shown] of table) {
    // @vanda:evidence tla-symbol-font
    it(`${op} の記号はNoto Sans Mathだけを書体に指して描く`, () => {
      const remove = withStyles();
      try {
        const { container } = render(<TlaCode source={src} />);
        const boxes = [...container.querySelectorAll<HTMLElement>(".sym")];
        expect(boxes.length).toBe(expectedSymbols(op, src, shown).length);
        for (const box of boxes) {
          const family = getComputedStyle(glyphOf(box)).fontFamily.replace(/["']/g, "").trim();
          // 後ろにほかの書体を並べると、Noto Sans Math を読めないときに OS の書体で描くことになる
          expect(family, `${box.textContent} の書体`).toBe("Noto Sans Math");
        }
      } finally {
        remove();
      }
    });
  }
});
