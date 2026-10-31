// コードの色付けと、ファイルの行の差分の見せ方。
// 要素とクラスの名前は、開発者が監督に報告した画面の作り（table.diff-lines、tr.diff-line、td.diff-no-before など）に従う。
// 期待値（行の種類・番号・文）は、テストの中で手で書いた diff.lines から作る。
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import { CodeBlock } from "../components/Code";
import { LineDiff } from "../components/DiffView";
import type { DiffLine } from "../api";

afterEach(() => {
  cleanup();
  vi.doUnmock("../highlight");
  vi.resetModules();
});

const lines: DiffLine[] = [
  { op: "equal", before: 1, after: 1, text: "-- 前置き", parts: null },
  { op: "delete", before: 2, after: null, text: "exec vanda add 買い物を登録した", parts: [{ op: "equal", text: "exec vanda add 買い物を" }, { op: "delete", text: "登録" }, { op: "equal", text: "した" }] },
  { op: "insert", before: null, after: 2, text: "exec vanda add 買い物を足した", parts: [{ op: "equal", text: "exec vanda add 買い物を" }, { op: "insert", text: "足" }, { op: "equal", text: "した" }] },
  { op: "insert", before: null, after: 3, text: "stdout ok", parts: null },
  { op: "equal", before: 3, after: 4, text: "end", parts: null },
  { op: "delete", before: 4, after: null, text: "last", parts: [{ op: "equal", text: "last" }], noNewline: true },
  { op: "insert", before: null, after: 5, text: "last", parts: [{ op: "equal", text: "last" }] },
];

describe("ファイルの行の差分", () => {
  it("行の種類と変更前と変更後の番号と文がAPIのdiff.linesと合う", () => {
    const { container } = render(<LineDiff lines={lines} lang={null} />);
    const rows = Array.from(container.querySelectorAll<HTMLTableRowElement>("table.diff-lines tr.diff-line"));
    expect(rows.length).toBe(lines.length);
    rows.forEach((tr, i) => {
      const l = lines[i];
      expect(tr.classList.contains(`diff-${l.op}`), `${i + 1} 行目の種類`).toBe(true);
      expect(tr.querySelector("td.diff-no-before")?.textContent?.trim(), `${i + 1} 行目の変更前の番号`).toBe(l.before === null ? "" : String(l.before));
      expect(tr.querySelector("td.diff-no-after")?.textContent?.trim(), `${i + 1} 行目の変更後の番号`).toBe(l.after === null ? "" : String(l.after));
      const text = tr.querySelector("td.diff-text");
      // 改行のない印は、行の文に含めない
      const shown = Array.from(text?.childNodes ?? []).filter((n) => !(n instanceof HTMLElement && n.classList.contains("diff-no-newline"))).map((n) => n.textContent).join("");
      expect(shown, `${i + 1} 行目の文`).toBe(l.text);
    });
  });

  it("書き換えた行の消した所と足した所をつなげると変更前と変更後の行になる", () => {
    const { container } = render(<LineDiff lines={lines} lang={null} />);
    const rows = Array.from(container.querySelectorAll<HTMLTableRowElement>("tr.diff-line"));
    const del = rows[1];
    const ins = rows[2];
    expect(Array.from(del.querySelectorAll("del.diff-part")).map((e) => e.textContent).join("")).toBe("登録");
    expect(del.querySelector("ins.diff-part")).toBeNull();
    expect(Array.from(ins.querySelectorAll("ins.diff-part")).map((e) => e.textContent).join("")).toBe("足");
    expect(ins.querySelector("del.diff-part")).toBeNull();
    expect(del.querySelector("td.diff-text")?.textContent).toBe(lines[1].text);
    expect(ins.querySelector("td.diff-text")?.textContent).toBe(lines[2].text);
    // 書き換えでない行には、文字の単位の差分を付けない
    for (const i of [0, 3, 4]) expect(rows[i].querySelector(".diff-part:is(del, ins)")).toBeNull();
  });

  it("改行のない最後の行にだけ印を付ける", () => {
    const { container } = render(<LineDiff lines={lines} lang={null} />);
    const rows = Array.from(container.querySelectorAll<HTMLTableRowElement>("tr.diff-line"));
    rows.forEach((tr, i) => {
      expect(tr.querySelector(".diff-no-newline") !== null, `${i + 1} 行目`).toBe(lines[i].noNewline === true);
    });
  });
});

// ブラウザと同じく、選んだ所の要素に copy を送り、クリップボードに書かれた文を返す（TlaCode.test.tsx と同じ確かめ方）
function copyContents(el: HTMLElement): string {
  const range = document.createRange();
  range.selectNodeContents(el);
  const sel = window.getSelection();
  sel?.removeAllRanges();
  sel?.addRange(range);
  const data: Record<string, string> = {};
  const target = el.querySelector(".code-line") ?? el;
  const ev = new Event("copy", { bubbles: true, cancelable: true });
  Object.defineProperty(ev, "clipboardData", {
    value: { setData: (t: string, v: string) => (data[t] = v), getData: (t: string) => data[t] ?? "" },
  });
  const notCancelled = target.dispatchEvent(ev);
  return notCancelled ? sel?.toString() ?? "" : data["text/plain"] ?? "";
}

const tla = "---- MODULE M ----\n\\* 置き換えないコメント a == b\nS == \\A x \\in T : x' # x /\\ <<a, b>> # <<>>\n====\n";

describe("色付け", () => {
  it("色付けを読み込んだ後もTLAは記号で見えコピーするとリポジトリの文字に戻る", async () => {
    const { container } = render(<CodeBlock source={tla} lang="tlaplus" />);
    const pre = container.querySelector<HTMLElement>("pre.code[data-lang=tlaplus]");
    expect(pre, "pre.code[data-lang=tlaplus] がない").toBeTruthy();
    // 色付けを読み込むと、かけらに色の style が付く
    await waitFor(() => expect(container.querySelector(".tok[style]"), "色付けが読み込まれない").toBeTruthy(), { timeout: 15000 });
    expect(pre!.textContent).toContain("S ≜ ∀ x ∈ T : x′ ≠ x ∧ ⟨a, b⟩ ≠ ⟨⟩");
    expect(pre!.textContent).toContain("\\* 置き換えないコメント a == b");
    for (const sym of Array.from(container.querySelectorAll<HTMLElement>(".tok.sym"))) {
      expect(sym.dataset.ascii, `記号 ${sym.textContent} に元の文字がない`).toBeTruthy();
    }
    expect(copyContents(pre!).replace(/\n$/, "")).toBe(tla.replace(/\n$/, ""));
  }, 20000);

  it("色付けを読み込めないときも文字は全部見える", async () => {
    vi.doMock("../highlight", async (importOriginal) => {
      const real = await importOriginal<typeof import("../highlight")>();
      return { ...real, tokenize: () => Promise.reject(new Error("色付けを読み込めない")) };
    });
    const { CodeBlock: Block } = await import("../components/Code");
    const sql = "-- @vanda:evidence task-id-unique\nCREATE UNIQUE INDEX t ON task (id);\n";
    const { container } = render(<Block source={sql} lang="sql" />);
    await new Promise((r) => setTimeout(r, 200));
    const pre = container.querySelector<HTMLElement>("pre.code[data-lang=sql]");
    expect(pre, "pre.code[data-lang=sql] がない").toBeTruthy();
    expect(pre!.querySelectorAll(".code-line").length).toBeGreaterThanOrEqual(2);
    expect((pre!.textContent ?? "").replace(/\n+$/, "")).toBe(sql.replace(/\n+$/, ""));
    expect(container.querySelector(".tok[style]")).toBeNull();
  });
});
