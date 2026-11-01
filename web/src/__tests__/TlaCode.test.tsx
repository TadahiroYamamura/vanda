// CL-07: TLA+ は、見せるときは数学の記号で、コピーしたときはリポジトリの文字のまま（ASCII）。
// 置き換えの表と決まりは docs/formats.md の「TLA+ の記号」。期待値は、その表から手で書く。
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TlaCode } from "../components/TlaCode";

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
it("TLA+ のコピーのボタンを押すとリポジトリの文字のままクリップボードに入る", async () => {
  const user = userEvent.setup();
  const src = "S == \\A x \\in T : x' # x";
  const { getByRole } = render(<TlaCode source={src} />);
  await user.click(getByRole("button", { name: /コピー/ }));
  expect(await navigator.clipboard.readText()).toBe(src);
});
