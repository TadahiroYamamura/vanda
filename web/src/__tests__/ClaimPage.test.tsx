// CL（約束の詳細）の画面が、API の結果を正しく見せるか。API は MSW で差し替える。
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { App } from "../App";
import type { ClaimDetail, ClaimHistory, Entry } from "../api";
import { isInaccessible } from "@testing-library/dom";
import { apiError, claimDetail, claimEntry, claimHistory, expectLoading, gate, claimRef, ctx, featureRef, fileEntry, plain, review, server, story, termRef } from "./fixtures";

beforeAll(() => server.listen({ onUnhandledFrame: "error" }));
afterAll(() => server.close());
beforeEach(() => window.history.pushState({}, "", "/claims/task-id-unique?view=branch&base=main"));
afterEach(() => {
  cleanup();
  server.resetHandlers();
});

const bodyText = () => document.body.textContent ?? "";
// shownText は、見えている文字（閉じた枠のように、見えなくしたものの中の文字を除く）
const shownText = (root: HTMLElement = document.body) => {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let out = "";
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if (n.parentElement && !isInaccessible(n.parentElement)) out += n.textContent ?? "";
  }
  return out;
};

function open(d: ClaimDetail, h: ClaimHistory = claimHistory()) {
  server.use(
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/claims/:id", () => HttpResponse.json(d)),
    http.get("/api/claims/:id/history", () => HttpResponse.json(h)),
  );
  render(<App />);
}

const tla = "Unique == \\A a, b \\in DOMAIN tasks : tasks[a] = tasks[b] => a = b";

const detail = claimDetail({
  evidence: [
    { stage: "implementation", type: "scenario-test", layers: ["model"], detail: null, present: true,
      found: [{ adapter: "txtar", file: "e2e/add-task.txtar", line: 1, name: "add-task", text: "exec vanda add 買い物\n" }] },
    { stage: "plan", type: "model-check", layers: ["model"], detail: null, present: true,
      found: [{ adapter: "tla", file: "spec/Task.tla", line: 5, name: "Unique", text: tla }] },
    { stage: "implementation", type: "guardrail", layers: ["model"], detail: null, present: true,
      found: [{ adapter: "sql", file: "db/schema.sql", line: 4, name: "CREATE UNIQUE INDEX t ON task (id);", text: "CREATE UNIQUE INDEX t ON task (id);\n" }] },
    { stage: "implementation", type: "unit-test", layers: ["model"], detail: null, present: true,
      found: [{ adapter: "go", file: "internal/task/task_test.go", line: 6, name: "TestUnique", text: null }] },
    { stage: "implementation", type: "random-check", layers: ["model"], detail: plain("まだ書いていない"), present: false, found: [] },
  ],
});

const history = claimHistory({
  origin: [
    { commit: "1".repeat(40), date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" },
    { commit: "2".repeat(40), date: "2026-10-20T12:00:00+09:00", subject: "#24 文を変える", stories: ["#24"], change: "modified" },
  ],
  agreements: [
    { user: { name: "前に承認した人", email: "a@example.com" }, agreedAt: "2026-10-02T12:00:00+09:00", story: "#1", fingerprint: "sha256:" + "b".repeat(64), current: false },
  ],
});

// CL-01
// @vanda:evidence claim-detail-fields
it("約束の詳細は文と理由と条件とレイヤーと対象を見せる", async () => {
  open(detail);
  expect(await screen.findByText(/タスクの ID は重ならない/)).toBeTruthy();
  for (const t of ["重なると消す相手を決められない", "タスクが 1 つ以上ある", "モデル", "タスクの ID"]) {
    await waitFor(() => expect(bodyText()).toContain(t));
  }
});

// CL-02・CL-07・RV-08
// @vanda:evidence claim-detail-evidence-content
// @vanda:evidence tla-math-symbols
it("エビデンスをタイプごとにシナリオは本文をTLAは定義を記号でガードレールは定義をテストは名前と場所で見せる", async () => {
  open(detail);
  await screen.findByText(/タスクの ID は重ならない/);
  await waitFor(() => expect(bodyText()).toContain("exec vanda add 買い物"));
  for (const t of ["CREATE UNIQUE INDEX t ON task (id);", "TestUnique", "internal/task/task_test.go", "spec/Task.tla", "e2e/add-task.txtar", "まだ書いていない"]) {
    expect(bodyText()).toContain(t);
  }
  // TLA+ は数学の記号で見せる
  expect(bodyText()).toContain("Unique ≜ ∀ a, b ∈ DOMAIN tasks : tasks[a] = tasks[b] ⇒ a = b");
  // TLA+ のファイル全体へのリンク
  const fileLinks = screen.getAllByRole("link").filter((a) => (a.getAttribute("href") ?? "").includes("spec%2FTask.tla") || (a.getAttribute("href") ?? "").includes("spec/Task.tla"));
  expect(fileLinks.length, "TLA+ のファイル全体へのリンクがない").toBeGreaterThan(0);
});

// CL-04・CL-05
// @vanda:evidence claim-detail-origin
// @vanda:evidence claim-detail-agreements
it("変更履歴にコミットとストーリーを承認の履歴に承認した人とストーリーと今の中身に合うかを見せる", async () => {
  open(detail, history);
  await screen.findByText(/タスクの ID は重ならない/);
  await waitFor(() => expect(bodyText()).toContain("#24 文を変える"));
  for (const t of ["#1 はじめの約束を置く", "#24", "前に承認した人"]) expect(bodyText()).toContain(t);
  // 今の中身に合わない承認しかないので、承認済とは見せない
  expect(bodyText()).toContain("提案中");
});

// @vanda:evidence claim-detail-error-shown
it("約束の詳細を読めないときは理由を出す", async () => {
  server.use(
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/claims/:id", () => apiError(404, "約束 task-id-unique は約束リストにない")),
  );
  render(<App />);
  expect(await screen.findByText(/約束 task-id-unique は約束リストにない/)).toBeTruthy();
});

// RV-18: 約束の詳細の画面とファイルの画面でも、ストーリーのレビューから移ってきて、そのものがレビューの一覧にあれば、
// 承認・取り消しのボタンを出す。レビューから移ってきたかは、レビューの画面と同じ URL のクエリ（view・base・story）で分かる
const fromReview = "?view=branch&base=main&story=%2324";

function openFromReview(path: string, entries: Entry[], d: ClaimDetail = claimDetail()) {
  window.history.pushState({}, "", path);
  server.use(
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/review", () => HttpResponse.json(review(entries))),
    http.get("/api/story", () => HttpResponse.json(story)),
    http.get("/api/claims/:id", () => HttpResponse.json(d)),
    http.get("/api/claims/:id/history", () => HttpResponse.json(claimHistory())),
    http.get("/api/file", () => HttpResponse.json({ path: "e2e/add-task.txtar", text: "exec vanda add 買い物\n" })),
  );
  render(<App />);
}

// @vanda:evidence agree-from-detail-pages
it("ストーリーのレビューから移ってきた約束の詳細の画面とファイルの画面で一覧にあるものには承認のボタンを出す", async () => {
  openFromReview("/claims/task-id-unique" + fromReview, [claimEntry(1, "task-id-unique")]);
  expect(await screen.findByRole("button", { name: /承認する/ })).toBeTruthy();
  cleanup();
  openFromReview("/files" + fromReview + "&path=" + encodeURIComponent("e2e/add-task.txtar"), [fileEntry(1, "e2e/add-task.txtar")]);
  expect(await screen.findByRole("button", { name: /承認する/ })).toBeTruthy();
});

// @vanda:evidence agree-from-detail-pages
it("ストーリーのレビューから移ってきた約束の詳細の画面で承認済の約束には取り消しのボタンを出す", async () => {
  const e = claimEntry(1, "task-id-unique", { state: "agreed", revoke: { possible: true, reason: null, others: [] } });
  openFromReview("/claims/task-id-unique" + fromReview, [e], claimDetail({ state: "agreed" }));
  expect(await screen.findByRole("button", { name: /取り消/ })).toBeTruthy();
});

// @vanda:evidence agree-from-detail-pages-notes
it("一覧にない約束とファイルは変わっていないと示しストーリーが分からないときはレビューから開くと承認できると示す", async () => {
  openFromReview("/claims/task-id-unique" + fromReview, [claimEntry(1, "other-claim")]);
  expect(await screen.findByText("このストーリーでは変わっていない約束です。")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /承認する/ })).toBeNull();
  cleanup();
  openFromReview("/files" + fromReview + "&path=" + encodeURIComponent("e2e/add-task.txtar"), [claimEntry(1, "task-id-unique")]);
  expect(await screen.findByText("このストーリーでは変わっていないファイルです。")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /承認する/ })).toBeNull();
  cleanup();
  // URL にレビューのクエリがない（直接開いた）
  openFromReview("/claims/task-id-unique", [claimEntry(1, "task-id-unique")]);
  expect(await screen.findByText("ストーリーのレビューから開くと、承認できます。")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /承認する/ })).toBeNull();
});

// @vanda:evidence agree-from-detail-pages-stay
it("約束の詳細の画面で承認した後もその画面にとどまる", async () => {
  const e = claimEntry(1, "task-id-unique");
  let posted: unknown = null;
  openFromReview("/claims/task-id-unique" + fromReview, [e]);
  server.use(
    http.post("/api/agree", async ({ request }) => {
      posted = await request.json();
      return HttpResponse.json({ commit: "c".repeat(40), agreement: {} });
    }),
  );
  const button = await screen.findByRole("button", { name: /承認する/ });
  await userEvent.setup().click(button);
  await waitFor(() => expect(posted).toMatchObject({ kind: "claim", id: "task-id-unique", story: "#24" }));
  expect(window.location.pathname).toBe("/claims/task-id-unique");
  expect(await screen.findByText(/タスクの ID は重ならない/)).toBeTruthy();
});

// CL-02: エビデンスは、ステージ（計画・実装・リリース・運用の順）ごとに分け、その中をタイプごとに分ける
// @vanda:evidence claim-detail-evidence-grouping
it("エビデンスをステージの順の見出しの下にタイプの見出しで分けて見せる", async () => {
  const ev = (stage: string, type: string) => ({ stage, type, layers: ["model"], detail: null, present: false, found: [] });
  open(claimDetail({ evidence: [ev("operation", "sensor"), ev("implementation", "unit-test"), ev("plan", "model-check"), ev("release", "human-review"), ev("implementation", "guardrail"), ev("implementation", "scenario-test")] }));
  await screen.findByText(/タスクの ID は重ならない/);
  const stages = ["計画", "実装", "リリース", "運用"];
  const types: Record<string, string[]> = { 計画: ["モデル検査"], 実装: ["単体テスト", "ガードレール", "シナリオテスト"], リリース: ["人のレビュー"], 運用: ["センサー"] };
  const all = Object.values(types).flat();
  await waitFor(() => expect(screen.getAllByRole("heading").map((h) => h.textContent?.trim())).toContain("運用"));
  const got = screen.getAllByRole("heading").map((h) => (h.textContent ?? "").trim()).filter((t) => stages.includes(t) || all.includes(t));
  expect(got.filter((t) => stages.includes(t))).toEqual(stages);
  for (const [i, s] of stages.entries()) {
    const from = got.indexOf(s);
    const to = i + 1 < stages.length ? got.indexOf(stages[i + 1]) : got.length;
    expect(got.slice(from + 1, to).sort(), `ステージ「${s}」の下のタイプ`).toEqual([...types[s]].sort());
  }
});

// CL-08: 変更履歴のコミットか承認の記録を選ぶと、その時点の約束の中身を示す
// @vanda:evidence claim-at-commit
it("変更履歴のコミットと承認の記録を開くとその時点の約束の中身を読んで見せる", async () => {
  const commit = "1".repeat(40);
  const agreedAt = "2".repeat(40);
  const at = new Map<string, string>([[commit, "作った時点の文"], [agreedAt, "承認した時点の文"]]);
  server.use(
    http.get("/api/claims/:id/at/:commit", ({ params }) =>
      HttpResponse.json({
        id: "task-id-unique", commit: params.commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめ", stories: ["#1"],
        content: { statement: plain(at.get(String(params.commit)) ?? "?"), rationale: null, condition: null, subjects: ["task-id"], layers: ["model"], evidence: [], flags: [], guesses: [] },
        refs: {},
      }),
    ),
  );
  open(claimDetail(), claimHistory({
    origin: [{ commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" }],
    agreements: [{ user: { name: "前に承認した人", email: "a@example.com" }, agreedAt: "2026-10-02T12:00:00+09:00", story: "#1", fingerprint: "sha256:" + "b".repeat(64), current: false, commit: agreedAt }],
  }));
  const user = userEvent.setup();
  const originRow = (await screen.findByText(/#1 はじめの約束を置く/)).closest("tr")!;
  await user.click(within(originRow).getByRole("button", { name: "その時点の中身を開く" }));
  expect(await screen.findByText("作った時点の文")).toBeTruthy();
  const agreementRow = screen.getByText(/前に承認した人/).closest("tr")!;
  await user.click(within(agreementRow).getByRole("button", { name: "その時点の中身を開く" }));
  expect(await screen.findByText("承認した時点の文")).toBeTruthy();
});

// CL-02・RV-08: 揃っているかの 3 つ（揃っている・ない・アノテーションでは確かめない）を、エビデンスの予定ごとに全部回す。
// アイコンの名前は E2E（review.spec.ts）で確かめているものと同じ
const presenceNames: [boolean | null, string][] = [
  [true, "揃っています"],
  [false, "ありません"],
  [null, "アノテーションでは確かめません"],
];

// typeGroup は、タイプの見出しのまとまり（見出しの親）
const typeGroup = (type: string) => screen.getAllByRole("heading").find((h) => (h.textContent ?? "").trim() === type)!.parentElement!;

// @vanda:evidence evidence-presence-shown
// @vanda:evidence evidence-presence-three-states
it("エビデンスの予定ごとに揃っているとないとアノテーションでは確かめないを見分けて示す", async () => {
  const types: Record<string, [string, string]> = { true: ["scenario-test", "シナリオテスト"], false: ["model-check", "モデル検査"], null: ["human-review", "人のレビュー"] };
  open(claimDetail({
    evidence: presenceNames.map(([present]) => {
      const [type] = types[String(present)];
      const found = present ? [{ adapter: "txtar", file: "e2e/add-task.txtar", line: 1, name: "add-task", text: "exec vanda add 買い物\n", state: "proposed" as const }] : [];
      return { stage: "implementation", type, layers: ["model"], detail: null, present, found };
    }),
  }));
  await screen.findByText(/タスクの ID は重ならない/);
  await waitFor(() => expect(screen.getAllByRole("heading").map((h) => (h.textContent ?? "").trim())).toContain("人のレビュー"));
  for (const [present, name] of presenceNames) {
    const [, label] = types[String(present)];
    const group = typeGroup(label);
    expect(within(group).getAllByRole("img", { name }).length, `${label} に「${name}」のアイコンがない`).toBe(1);
    for (const [, other] of presenceNames) {
      if (other !== name) expect(within(group).queryByRole("img", { name: other }), `${label} に「${other}」のアイコンがある`).toBeNull();
    }
  }
});

// @vanda:evidence found-file-state-shown
it("実際のエビデンスのうち承認の対象のファイルには今の中身が承認済か提案中かを示し承認の対象でないものには示さない", async () => {
  const states: ["agreed" | "proposed" | null, string | null][] = [["agreed", "承認済"], ["proposed", "提案中"], [null, null]];
  const files: Record<string, [string, string, string]> = {
    agreed: ["txtar", "e2e/agreed.txtar", "agreed"],
    proposed: ["tla", "spec/Proposed.tla", "Proposed"],
    null: ["go", "internal/task/task_test.go", "TestUnique"],
  };
  open(claimDetail({
    evidence: [{
      stage: "implementation", type: "unit-test", layers: ["model"], detail: null, present: true,
      found: states.map(([st]) => {
        const [adapter, file, name] = files[String(st)];
        return { adapter, file, line: 1, name, text: adapter === "go" ? null : `${name} の中身\n`, state: st };
      }),
    }],
  }));
  await screen.findByText(/タスクの ID は重ならない/);
  await waitFor(() => expect(bodyText()).toContain("internal/task/task_test.go"));
  // 1 つの見つかったものの所: 場所の文字から、ほかの場所の文字を含まない所まで上にたどる
  const allFiles = Object.values(files).map(([, f]) => f);
  const itemOf = (file: string) => {
    let el = Array.from(document.querySelectorAll<HTMLElement>("*")).find((e) => e.children.length === 0 && (e.textContent ?? "").includes(file))!;
    expect(el, `${file} が出ていない`).toBeTruthy();
    const others = allFiles.filter((f) => f !== file);
    while (el.parentElement && !others.some((f) => (el.parentElement!.textContent ?? "").includes(f))) el = el.parentElement;
    return el;
  };
  for (const [st, name] of states) {
    const [, file] = files[String(st)];
    const item = itemOf(file);
    for (const label of ["承認済", "提案中"]) {
      const got = within(item).queryAllByRole("img", { name: label }).length;
      expect(got, `${file} の「${label}」のアイコンの数`).toBe(label === name ? 1 : 0);
    }
  }
});

// jsdom は描画の幅を持たないので、幅 400px で横にスクロールしないことは E2E（review.spec.ts）で見る。ここでは、省略せずに全部出すことを見る
// @vanda:evidence evidence-detail-no-truncate
it("エビデンスの予定の詳細は長い文も改行も省略せずに全部出す", async () => {
  const detail = [
    "TLA+ のモデルで、タスクを足す・消す・足し直すを 3 回まで繰り返し、どの順でも ID が重ならないことを確かめる予定。".repeat(4),
    "定数の範囲は、タスクが 3 つまで、操作が 6 歩まで。",
    "置き場所の候補: spec/TaskLifecycleWithDeletionAndReAddition.tla",
  ].join("\n");
  open(claimDetail({ evidence: [{ stage: "plan", type: "model-check", layers: ["model"], detail: plain(detail), present: false, found: [] }] }));
  await screen.findByText(/タスクの ID は重ならない/);
  await waitFor(() => expect(bodyText()).toContain("置き場所の候補"));
  const holders = Array.from(document.querySelectorAll<HTMLElement>("*")).filter((el) => el.textContent === detail);
  expect(holders.length, "詳細の全部の文と改行をそのまま持つ要素がない（省略したか、改行を落とした）").toBeGreaterThan(0);
});

// 続いた空白・全角の空白・改行は、要素の文字にそのまま残っているかで見る（見た目で詰まらないかは E2E で見る）
const spaced = "タスクの  ID　は\n  重ならない";

// @vanda:evidence keep-user-whitespace
it("約束の詳細とその時点の中身は利用者が書いた文の続いた空白と全角の空白と改行をそのまま持つ", async () => {
  const commit = "1".repeat(40);
  server.use(
    http.get("/api/claims/:id/at/:commit", () =>
      HttpResponse.json({
        id: "task-id-unique", commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめ", stories: ["#1"],
        content: { statement: plain("その時点の" + spaced), rationale: plain("理由の" + spaced), condition: plain("条件の" + spaced), subjects: ["task-id"], layers: ["model"], evidence: [], flags: [], guesses: [] },
        layers: [{ id: "model", name: "モデル" }],
        refs: {},
      }),
    ),
  );
  open(
    claimDetail({ statement: plain(spaced), rationale: plain("今の理由の" + spaced), condition: plain("今の条件の" + spaced) }),
    claimHistory({ origin: [{ commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" }] }),
  );
  const has = (t: string) => Array.from(document.querySelectorAll<HTMLElement>("*")).some((el) => el.textContent === t);
  await waitFor(() => expect(has(spaced), "約束の文").toBe(true));
  for (const t of ["今の理由の" + spaced, "今の条件の" + spaced]) expect(has(t), t).toBe(true);
  const row = (await screen.findByText(/#1 はじめの約束を置く/)).closest("tr")!;
  await userEvent.setup().click(within(row).getByRole("button", { name: "その時点の中身を開く" }));
  await waitFor(() => expect(has("その時点の" + spaced), "その時点の文").toBe(true));
  for (const t of ["理由の" + spaced, "条件の" + spaced]) expect(has(t), t).toBe(true);
});

// CL-08 の、その時点の中身の開け閉め。変更履歴の 2 つと承認の記録の 1 つを使う
const atCommits = { first: "1".repeat(40), second: "2".repeat(40), agreed: "3".repeat(40) };
const atStatement: Record<string, string> = { [atCommits.first]: "はじめの時点の文", [atCommits.second]: "次の時点の文", [atCommits.agreed]: "承認した時点の文" };

function openHistory(at: Parameters<typeof http.get>[1] = ({ params }) =>
  HttpResponse.json({
    id: "task-id-unique", commit: params.commit, date: "2026-10-01T12:00:00+09:00", subject: "s", stories: ["#1"],
    content: { statement: plain(atStatement[String(params.commit)] ?? "?"), rationale: null, condition: null, subjects: ["task-id"], layers: ["model"], evidence: [], flags: [], guesses: [] },
    layers: [{ id: "model", name: "モデル" }],
    refs: {},
  })) {
  server.use(http.get("/api/claims/:id/at/:commit", at));
  open(claimDetail(), claimHistory({
    origin: [
      { commit: atCommits.second, date: "2026-10-20T12:00:00+09:00", subject: "#24 文を変える", stories: ["#24"], change: "modified" },
      { commit: atCommits.first, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" },
    ],
    agreements: [{ user: { name: "前に承認した人", email: "a@example.com" }, agreedAt: "2026-10-02T12:00:00+09:00", story: "#1", fingerprint: "sha256:" + "b".repeat(64), current: false, commit: atCommits.agreed }],
  }));
}

const historyRow = async (text: RegExp) => (await screen.findByText(text)).closest("tr")! as HTMLElement;
const toggleOf = (row: HTMLElement) => within(row).getByRole("button", { name: /その時点の中身を(開く|たたむ)/ });

// @vanda:evidence claim-at-commit-toggle
it("変更履歴と承認の履歴の行をクリックするとその時点の中身が開きもう一度クリックするとたたまりボタンをクリックしても1回だけ切り替わる", async () => {
  openHistory();
  const user = userEvent.setup();
  for (const [text, statement] of [[/#24 文を変える/, "次の時点の文"], [/前に承認した人/, "承認した時点の文"]] as const) {
    const row = await historyRow(text);
    const cell = within(row).getByText(text);
    expect(toggleOf(row).getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText(statement)).toBeNull();
    await user.click(cell);
    expect(await screen.findByText(statement), "行をクリックしても開かない").toBeTruthy();
    expect(toggleOf(row).getAttribute("aria-expanded")).toBe("true");
    await user.click(cell);
    await waitFor(() => expect(screen.queryByText(statement), "行をもう一度クリックしてもたたまない").toBeNull());
    expect(toggleOf(row).getAttribute("aria-expanded")).toBe("false");
    // ボタンは行の中にあるので、行の動きと二重に切り替わると、クリックしても閉じたままになる
    await user.click(toggleOf(row));
    expect(await screen.findByText(statement), "ボタンをクリックしても開かない").toBeTruthy();
    await user.click(toggleOf(row));
    await waitFor(() => expect(screen.queryByText(statement), "ボタンをクリックしてもたたまない").toBeNull());
  }
});

// @vanda:evidence claim-at-commit-multiple-open
it("その時点の中身を2つ以上開くとそれぞれが同時に出たままになり1つをたたんでもほかは開いたままにする", async () => {
  openHistory();
  const user = userEvent.setup();
  const rows: [RegExp, string][] = [[/#24 文を変える/, "次の時点の文"], [/#1 はじめの約束を置く/, "はじめの時点の文"], [/前に承認した人/, "承認した時点の文"]];
  for (const [text] of rows) await user.click(toggleOf(await historyRow(text)));
  for (const [, statement] of rows) expect(await screen.findByText(statement), `${statement} が、ほかを開いた後に出ていない`).toBeTruthy();
  await user.click(toggleOf(await historyRow(rows[0][0])));
  await waitFor(() => expect(screen.queryByText(rows[0][1]), "たたんだ時点の中身が消えない").toBeNull());
  for (const [text, statement] of rows.slice(1)) {
    expect(screen.queryByText(statement), `1 つをたたむと、${statement} も消えた`).toBeTruthy();
    expect(toggleOf(await historyRow(text)).getAttribute("aria-expanded")).toBe("true");
  }
});

// @vanda:evidence claim-at-commit-unknown-layer
it("その時点の中身のレイヤーのうち今の設定にないものはIDのまま出す", async () => {
  openHistory(({ params }) =>
    HttpResponse.json({
      id: "task-id-unique", commit: params.commit, date: "2026-10-01T12:00:00+09:00", subject: "s", stories: ["#1"],
      content: { statement: plain("古いレイヤーの時点の文"), rationale: null, condition: null, subjects: ["task-id"], layers: ["model", "old-layer"], evidence: [], flags: [], guesses: [] },
      layers: [{ id: "model", name: "モデル" }, { id: "old-layer", name: "" }],
      refs: {},
    }),
  );
  const row = await historyRow(/#1 はじめの約束を置く/);
  expect(bodyText()).not.toContain("old-layer");
  await userEvent.setup().click(toggleOf(row));
  await screen.findByText("古いレイヤーの時点の文");
  const at = row.nextElementSibling as HTMLElement;
  expect(at, "開いた中身の行がない").toBeTruthy();
  await waitFor(() => expect(at.textContent, "今の設定にないレイヤーが ID のまま出ない").toContain("old-layer"));
  expect(at.textContent).toContain("モデル");
});

// @vanda:evidence claim-at-commit-error-shown
it("その時点の約束リストが読めない承認の記録を選ぶとAPIが返した理由を出す", async () => {
  openHistory(() => apiError(500, "コミット 3333333 の .vanda/promises.toml を読めません（4 行目）"));
  const row = await historyRow(/前に承認した人/);
  await userEvent.setup().click(toggleOf(row));
  expect(await screen.findByText(/コミット 3333333 の \.vanda\/promises\.toml を読めません/)).toBeTruthy();
});

// 参照（show-references）。約束の詳細とその時点の中身の、自然言語で書く項目のセグメントと refs（docs/plans/stage4-references.md の「API の形」と「画面」）
const ref = (text: string, id: string) => ({ text, ref: id });
const claimRefs = {
  "t-statement": termRef("文の用語", "文の用語の定義"),
  "t-rationale": termRef("理由の用語", "理由の用語の定義"),
  "t-condition": termRef("条件の用語", "条件の用語の定義"),
  "t-guess": termRef("推測の用語", "推測の用語の定義"),
  "t-detail": termRef("詳細の用語", "詳細の用語の定義"),
  "f-sort": featureRef("並べ替え", "タスクを順に並べる", { core: true, formal: false }),
  "c-other": claimRef("ほかの約束の文"),
  "c-gone": claimRef("消えた約束の文", false),
  // 比べる元にだけある（今の約束リストにない）用語と機能
  "t-past": termRef("今ない用語", "今ない用語の定義", false),
  "f-past": featureRef("今ない機能", "今ない機能の定義", { current: false, core: false, formal: false }),
};
const detailWithRefs = (over: Partial<ClaimDetail> = {}) =>
  claimDetail({
    statement: [ref("文の用語", "t-statement"), { text: "と" }, ref("並べ替え", "f-sort"), { text: "と" }, ref("c-other", "c-other"), { text: "と" }, ref("c-gone", "c-gone"), { text: "と" }, ref("今ない用語", "t-past"), { text: "と" }, ref("今ない機能", "f-past"), { text: "の文" }],
    rationale: [ref("理由の用語", "t-rationale"), { text: "の理由" }],
    condition: [ref("条件の用語", "t-condition"), { text: "の条件" }],
    guesses: [[ref("推測の用語", "t-guess"), { text: "の推測" }]],
    evidence: [{ stage: "implementation", type: "unit-test", layers: ["model"], detail: [ref("詳細の用語", "t-detail"), { text: "で確かめる" }], present: false, found: [] }],
    refs: claimRefs,
    ...over,
  } as Partial<ClaimDetail>);

// 参照の要素（キーボードでフォーカスできる要素のうち、name を含む最も短いもの）
const refElementIn = (root: HTMLElement, name: string) => {
  const focusable = Array.from(root.querySelectorAll<HTMLElement>("*")).filter(
    (el) => (el.textContent ?? "").includes(name) && (el.tabIndex >= 0 || el.matches("a[href]")),
  );
  focusable.sort((a, b) => (a.textContent ?? "").length - (b.textContent ?? "").length);
  return focusable[0];
};
// 参照だけの要素で、囲む要素と見た目が違うか（jsdom は描かないので、下線や色そのものは見ない）
const expectRefLookIn = (el: HTMLElement | undefined, name: string, where: string) => {
  expect(el, `${where}: 参照 ${name} が、フォーカスできる要素で出ていない`).toBeTruthy();
  expect((el!.textContent ?? "").startsWith(name), `${where}: 参照 ${name} の要素が、前の文字も含んでいる`).toBe(true);
  // 参照だけを包む要素が重なることがある（差分の 1 つずつを包む要素など）ので、参照より多くの文字を持つ祖先（前後の文字も含む要素）まで上って比べる
  let parent = el!.parentElement;
  while (parent && (parent.textContent ?? "").length <= (el!.textContent ?? "").length) parent = parent.parentElement;
  expect(parent, `${where}: 参照 ${name} の前後の文字を含む要素がない`).toBeTruthy();
  if (!parent) return;
  expect(el!.tagName + " " + el!.className, `${where}: 参照 ${name} の見た目が、囲む要素と同じ`).not.toBe(parent.tagName + " " + parent.className);
  // 前後の参照でない文字を包む要素（ほかの参照の中のものを除く）とも、見た目が違う
  const look = (x: Element) => x.tagName + " " + x.className;
  const inRef = (x: Element) => x.closest("[tabindex], a[href]") !== null;
  for (const other of Array.from(parent.querySelectorAll("*"))) {
    if (other.children.length > 0 || !(other.textContent ?? "").trim() || other.contains(el!) || el!.contains(other) || inRef(other)) continue;
    expect(look(el!), `${where}: 参照 ${name} の見た目が、参照でない文字「${other.textContent}」と同じ`).not.toBe(look(other));
  }
};

// @vanda:evidence reference-shown
it("約束の詳細では文と理由と条件と推測とエビデンスの詳細の参照を用語と機能は名前で約束はIDで参照だと分かる見た目で出し今の約束リストにあるものだけを詳細のページへのリンクにする", async () => {
  open(detailWithRefs());
  await waitFor(() => expect(bodyText()).toContain("詳細の用語"));
  for (const name of ["文の用語", "理由の用語", "条件の用語", "推測の用語", "詳細の用語", "並べ替え", "c-other", "c-gone", "今ない用語", "今ない機能"]) {
    expectRefLookIn(refElementIn(document.body, name), name, "約束の詳細");
  }
  // ID は、マウスを乗せた時のポップアップにだけ出る
  expect(shownText(), "書いたままの参照が出た").not.toContain("{@id:");
  for (const id of ["t-statement", "t-rationale", "t-condition", "t-guess", "t-detail", "f-sort", "t-past", "f-past"]) expect(shownText(), `用語・機能の参照の指す ID ${id} が出た`).not.toContain(id);
  for (const name of ["c-gone", "今ない用語", "今ない機能"]) expect(screen.queryByRole("link", { name }), `${name} がリンクになっている`).toBeNull();
  const links: [string, string][] = [
    ["c-other", "/claims/c-other"],
    ["文の用語", "/terms/t-statement"],
    ["理由の用語", "/terms/t-rationale"],
    ["条件の用語", "/terms/t-condition"],
    ["推測の用語", "/terms/t-guess"],
    ["詳細の用語", "/terms/t-detail"],
    ["並べ替え", "/features/f-sort"],
  ];
  for (const [name, path] of links) {
    const link = screen.getByRole("link", { name });
    expect(new URL((link as HTMLAnchorElement).href).pathname, `${name} のリンク先`).toBe(path);
  }
});

// @vanda:evidence reference-shown
it("約束の履歴で開いたその時点の中身の参照もその時点のrefsの名前で参照だと分かる見た目で出す", async () => {
  const commit = "1".repeat(40);
  server.use(
    http.get("/api/claims/:id/at/:commit", () =>
      HttpResponse.json({
        id: "task-id-unique", commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめ", stories: ["#1"],
        content: { statement: [ref("その時点だけの用語", "t-old"), { text: "を使っていた" }], rationale: null, condition: null, subjects: ["task-id"], layers: ["model"], evidence: [], flags: [], guesses: [] },
        layers: [{ id: "model", name: "モデル" }],
        refs: { "t-old": termRef("その時点だけの用語", "今はない用語の定義", false) },
      }),
    ),
  );
  open(detailWithRefs(), claimHistory({ origin: [{ commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" }] }));
  const row = (await screen.findByText(/#1 はじめの約束を置く/)).closest("tr")!;
  await userEvent.setup().click(within(row).getByRole("button", { name: "その時点の中身を開く" }));
  await waitFor(() => expect(bodyText()).toContain("その時点だけの用語"));
  expectRefLookIn(refElementIn(document.body, "その時点だけの用語"), "その時点だけの用語", "その時点の中身");
  expect(shownText()).not.toContain("t-old");
  expect(screen.queryByRole("link", { name: "その時点だけの用語" }), "今の約束リストにない用語がリンクになっている").toBeNull();
});

// @vanda:evidence reference-popup
it("約束の詳細の参照にマウスを乗せた時とフォーカスした時に指す先のIDと用語と機能はその定義を約束はその文をポップアップで出し離れると消す", async () => {
  open(detailWithRefs());
  await waitFor(() => expect(bodyText()).toContain("詳細の用語"));
  const tooltipWith = (text: string) => screen.queryAllByRole("tooltip").find((t) => (t.textContent ?? "").includes(text)) ?? null;
  const user = userEvent.setup();
  const cases = [
    ["理由の用語", "理由の用語の定義", "t-rationale"],
    ["今ない用語", "今ない用語の定義", "t-past"],
    ["並べ替え", "タスクを順に並べる", "f-sort"],
    ["c-other", "ほかの約束の文", "c-other"],
    ["c-gone", "消えた約束の文", "c-gone"],
  ] as const;
  for (const [name, text, id] of cases) {
    const el = refElementIn(document.body, name)!;
    expect(el, `参照 ${name} がない`).toBeTruthy();
    expect(tooltipWith(text), `${name}: マウスを乗せる前から出ている`).toBeNull();
    await user.hover(el);
    await waitFor(() => expect(tooltipWith(text), `${name}: マウスを乗せても ${text} が出ない`).toBeTruthy());
    expect(tooltipWith(text)!.textContent, `${name}: ポップアップに ID がない`).toContain(id);
    // f-sort は、コア機能で、形式手法を使う機能ではない
    if (id === "f-sort") {
      expect(tooltipWith(text)!.textContent, "並べ替え: ポップアップにコア機能のマークがない").toContain("コア機能");
      expect(tooltipWith(text)!.textContent, "並べ替え: ポップアップに形式手法を使う機能のマークがある").not.toContain("形式手法を使う機能");
    }
    await user.unhover(el);
    await waitFor(() => expect(tooltipWith(text), `${name}: 離れても消えない`).toBeNull());
    el.focus();
    expect(document.activeElement, `${name}: フォーカスできない`).toBe(el);
    await waitFor(() => expect(tooltipWith(text), `${name}: フォーカスしても ${text} が出ない`).toBeTruthy());
    el.blur();
    await waitFor(() => expect(tooltipWith(text), `${name}: フォーカスを外しても消えない`).toBeNull());
  }
});

// CL-01
// @vanda:evidence claim-detail-state
it("約束の詳細の見出しに約束のIDと今の中身が承認済か提案中かを示す", async () => {
  for (const [state, label, other] of [["proposed", "提案中", "承認済"], ["agreed", "承認済", "提案中"]] as const) {
    window.history.pushState({}, "", "/claims/task-other-id");
    open(claimDetail({ id: "task-other-id", state }));
    const h = await screen.findByRole("heading", { level: 1 });
    expect(h.textContent, `${state} の見出しに ID がない`).toContain("task-other-id");
    expect(h.textContent, `${state} の見出しに ${label} がない`).toContain(label);
    expect(h.textContent, `${state} の見出しに ${other} がある`).not.toContain(other);
    cleanup();
    server.resetHandlers();
  }
});

// CL-01
// @vanda:evidence claim-detail-subjects
it("約束の詳細の対象には用語か機能の名前とIDを示し約束リストにないIDには約束リストにないと添える", async () => {
  open(claimDetail({
    subjects: [
      { kind: "feature", id: "add-task", name: "タスクの登録" },
      { kind: null, id: "no-such-subject", name: null },
      { kind: "term", id: "task-id", name: "タスクの ID" },
    ],
  }));
  await screen.findByText(/タスクの ID は重ならない/);
  const dt = screen.getAllByRole("term").find((el) => el.textContent === "対象");
  expect(dt, "対象の項目がない").toBeTruthy();
  const dd = dt!.nextElementSibling as HTMLElement;
  const text = dd.textContent ?? "";
  for (const t of ["タスクの登録", "add-task", "タスクの ID", "task-id", "no-such-subject（約束リストにありません）"]) expect(text, `対象に ${t} がない`).toContain(t);
  // 約束リストにある ID には添えない
  expect(text.split("（約束リストにありません）").length - 1, "約束リストにありませんを添えた数").toBe(1);
});

// CL-01
// @vanda:evidence claim-detail-guesses
it("約束の詳細に推測があれば1つずつ示す", async () => {
  const guesses = ["消したタスクの ID は使い回さない", "ID は 1 から数える", "ID は数字だけ"];
  open(claimDetail({ guesses: guesses.map(plain) }));
  await screen.findByText(/タスクの ID は重ならない/);
  const dt = screen.getAllByRole("term").find((el) => el.textContent === "推測");
  expect(dt, "推測の項目がない").toBeTruthy();
  const items = within(dt!.nextElementSibling as HTMLElement).getAllByRole("listitem").map((li) => li.textContent);
  expect(items).toEqual(guesses);
});

// CL-02
// @vanda:evidence claim-detail-found-location
it("実際のエビデンスごとに付いているものの名前とファイルと行を書いたファイルの画面へのリンクを示しTLAならファイル全体へのリンクも示す", async () => {
  window.history.pushState({}, "", "/claims/task-id-unique" + "?view=branch&base=main&story=%2324");
  open(detail);
  await screen.findByText(/タスクの ID は重ならない/);
  await waitFor(() => expect(bodyText()).toContain("TestUnique"));
  const fileURL = (file: string) => new RegExp(`^/files\\?(.*&)?path=${encodeURIComponent(file).replace(/\./g, "\\.")}(&|$)`);
  const founds = [
    { name: "add-task", file: "e2e/add-task.txtar", line: 1, tla: false },
    { name: "Unique", file: "spec/Task.tla", line: 5, tla: true },
    { name: "CREATE UNIQUE INDEX t ON task (id);", file: "db/schema.sql", line: 4, tla: false },
    { name: "TestUnique", file: "internal/task/task_test.go", line: 6, tla: false },
  ];
  for (const f of founds) {
    const link = screen.getByRole("link", { name: `${f.file}:${f.line}` });
    expect(link.getAttribute("href"), `${f.file}:${f.line} のリンクの先`).toMatch(fileURL(f.file));
    // 付いているものの名前は、リンクと同じ所に出す
    const where = link.closest("li, p, div") as HTMLElement;
    expect(where.textContent, `${f.file}:${f.line} のそばに ${f.name} がない`).toContain(f.name);
    const toFile = screen.getAllByRole("link").filter((a) => fileURL(f.file).test(a.getAttribute("href") ?? ""));
    expect(toFile.length, `${f.file} の画面へのリンクの数`).toBe(f.tla ? 2 : 1);
  }
});

// @vanda:evidence file-page-error-shown
it("ファイルの画面でファイルを読めない時はAPIが返した理由を出す", async () => {
  window.history.pushState({}, "", "/files?path=" + encodeURIComponent("e2e/none.txtar"));
  server.use(
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/file", () => apiError(404, "e2e/none.txtar は、承認の対象のファイルでもアノテーションを読むファイルでもありません。")),
  );
  render(<App />);
  expect(await screen.findByText(/e2e\/none\.txtar は、承認の対象のファイルでもアノテーションを読むファイルでもありません/)).toBeTruthy();
});

// @vanda:evidence api-error-fix-shown
it("約束の詳細の画面とファイルの画面はAPIのエラーを出す時に何がどうなっているかとどうすればよいかの両方を出す", async () => {
  const cases = [
    { path: "/claims/task-id-unique", api: "/api/claims/:id", message: "約束 task-id-unique は約束リストにありません。", fix: "約束リストを確かめてください。" },
    { path: "/files?path=" + encodeURIComponent("e2e/a.txtar"), api: "/api/file", message: "e2e/a.txtar を読めません。", fix: "ファイルをコミットしてください。" },
  ];
  for (const c of cases) {
    window.history.pushState({}, "", c.path);
    server.use(
      http.get("/api/context", () => HttpResponse.json(ctx)),
      http.get(c.api, () => HttpResponse.json({ error: { message: c.message, fix: c.fix } }, { status: 404 })),
    );
    render(<App />);
    expect(await screen.findByText(c.message, { exact: false }), `${c.path}: message がない`).toBeTruthy();
    expect(await screen.findByText(c.fix, { exact: false }), `${c.path}: fix がない`).toBeTruthy();
    cleanup();
    server.resetHandlers();
  }
});

// 用語と機能の詳細のページ（show-term-feature-pages）。API の形は docs/formats.md の GET /api/terms/<ID>・GET /api/features/<ID>
describe("用語と機能の詳細", () => {
  const openAt = (path: string, ...handlers: Parameters<typeof server.use>) => {
    window.history.pushState({}, "", path);
    server.use(http.get("/api/context", () => HttpResponse.json(ctx)), ...handlers);
    render(<App />);
  };
  // 約束リストに書いた順（ID の順と違う）
  const claims = [
    { id: "c-z", statement: [{ text: "最初に書いた約束は" }, ref("定義の中の用語", "t-in-def"), { text: "を数える" }] },
    { id: "c-a", statement: plain("次に書いた約束の文") },
    { id: "c-m", statement: plain("最後に書いた約束の文") },
  ];
  const tfRefs = { "t-parent": termRef("親にした用語", "親の定義"), "t-in-def": termRef("定義の中の用語", "定義の中の用語の定義") };
  const termDetail = (over = {}) => ({
    id: "t-mid", name: "まん中の用語",
    definition: [ref("定義の中の用語", "t-in-def"), { text: "を見分ける名前" }],
    partOf: { id: "t-parent", name: "親にした用語" },
    children: [{ id: "t-child-b", name: "子にした用語 B" }, { id: "t-child-a", name: "子にした用語 A" }],
    claims, refs: tfRefs, ...over,
  });
  const featureDetail = (over = {}) => ({
    id: "f-sort", name: "並べ替える機能",
    definition: [ref("定義の中の用語", "t-in-def"), { text: "を順に並べる" }],
    core: false, formal: false, claims, refs: tfRefs, ...over,
  });
  // inOrder は、texts が見えている文字の中に、この順に出ているか
  const expectInOrder = (texts: string[], where: string) => {
    const shown = shownText();
    let at = -1;
    for (const t of texts) {
      const i = shown.indexOf(t, at + 1);
      expect(i, `${where}: ${t} が ${texts.join("・")} の順に出ていない`).toBeGreaterThan(at);
      at = i;
    }
  };
  // expectClaimList は、約束の一覧が、書いた順に、ID と文で並び、それぞれその約束の詳細へのリンクになっているか
  const expectClaimList = (where: string) => {
    expectInOrder(["c-z", "最初に書いた約束は定義の中の用語を数える", "c-a", "次に書いた約束の文", "c-m", "最後に書いた約束の文"], where);
    const claimLinks = screen.getAllByRole("link").map((a) => new URL((a as HTMLAnchorElement).href).pathname).filter((p) => p.startsWith("/claims/"));
    expect(claimLinks, `${where}: 約束の詳細へのリンク`).toEqual(["/claims/c-z", "/claims/c-a", "/claims/c-m"]);
    for (const id of ["c-z", "c-a", "c-m"]) {
      expect(new URL((screen.getByRole("link", { name: id }) as HTMLAnchorElement).href).pathname, `${where}: ${id} のリンク`).toBe(`/claims/${id}`);
    }
  };

  // @vanda:evidence term-page-content
  it("用語の詳細のページに名前とIDと定義と親の用語と子の用語を示し定義の参照は参照の見た目で出す", async () => {
    openAt("/terms/t-mid?view=branch&base=main", http.get("/api/terms/:id", () => HttpResponse.json(termDetail())));
    await waitFor(() => expect(shownText()).toContain("まん中の用語"));
    const shown = shownText();
    for (const t of ["まん中の用語", "t-mid", "定義の中の用語を見分ける名前", "親にした用語", "子にした用語 B", "子にした用語 A"]) expect(shown, `${t} がない`).toContain(t);
    expectRefLookIn(refElementIn(document.body, "定義の中の用語"), "定義の中の用語", "用語の詳細の定義");
    // 子は API が返した順（約束リストに書いた順）
    expectInOrder(["子にした用語 B", "子にした用語 A"], "子の用語");
  });

  // @vanda:evidence term-page-content
  it("用語の詳細のページは親の用語がなければ親を示さず親が約束リストにないIDならそのIDを示す", async () => {
    openAt("/terms/t-mid", http.get("/api/terms/:id", () => HttpResponse.json(termDetail({ partOf: null, children: [], claims: [] }))));
    await waitFor(() => expect(shownText()).toContain("まん中の用語"));
    // 定義の参照（定義の中の用語）のほかに、用語の詳細へのリンクがない（約束の文の参照も除くため、約束も置かない）
    const termLinks = screen.getAllByRole("link").map((a) => new URL((a as HTMLAnchorElement).href).pathname).filter((p) => p.startsWith("/terms/"));
    expect(termLinks, "親も子もないのに用語へのリンクがある").toEqual(["/terms/t-in-def"]);
    cleanup();
    server.resetHandlers();
    openAt("/terms/t-mid", http.get("/api/terms/:id", () => HttpResponse.json(termDetail({ partOf: { id: "t-missing-parent", name: null } }))));
    await waitFor(() => expect(shownText()).toContain("まん中の用語"));
    expect(shownText(), "約束リストにない親の ID がない").toContain("t-missing-parent");
  });

  // @vanda:evidence feature-page-content
  it("機能の詳細のページに名前とIDと定義とコア機能か形式手法を使う機能かのマークを示す", async () => {
    for (const core of [true, false]) {
      for (const formal of [true, false]) {
        const where = `core=${core}・formal=${formal}`;
        openAt("/features/f-sort", http.get("/api/features/:id", () => HttpResponse.json(featureDetail({ core, formal }))));
        await waitFor(() => expect(shownText()).toContain("並べ替える機能"));
        const shown = shownText();
        for (const t of ["並べ替える機能", "f-sort", "定義の中の用語を順に並べる"]) expect(shown, `${where}: ${t} がない`).toContain(t);
        expectRefLookIn(refElementIn(document.body, "定義の中の用語"), "定義の中の用語", `${where}: 機能の詳細の定義`);
        if (core) expect(shown, `${where}: コア機能のマークがない`).toContain("コア機能");
        else expect(shown, `${where}: コア機能のマークがある`).not.toContain("コア機能");
        if (formal) expect(shown, `${where}: 形式手法を使う機能のマークがない`).toContain("形式手法を使う機能");
        else expect(shown, `${where}: 形式手法を使う機能のマークがある`).not.toContain("形式手法を使う機能");
        cleanup();
        server.resetHandlers();
      }
    }
  });

  // @vanda:evidence term-feature-page-claims
  it("用語と機能の詳細のページにそれを対象に持つ約束を書いた順にIDと文で並べそれぞれ約束の詳細へのリンクにする", async () => {
    openAt("/terms/t-mid?view=branch&base=main", http.get("/api/terms/:id", () => HttpResponse.json(termDetail())));
    await waitFor(() => expect(shownText()).toContain("最後に書いた約束の文"));
    expectClaimList("用語の詳細");
    cleanup();
    server.resetHandlers();
    openAt("/features/f-sort?view=branch&base=main", http.get("/api/features/:id", () => HttpResponse.json(featureDetail())));
    await waitFor(() => expect(shownText()).toContain("最後に書いた約束の文"));
    expectClaimList("機能の詳細");
    // 一覧のリンクから、その約束の詳細へ移る
    server.use(http.get("/api/claims/:id", ({ params }) => HttpResponse.json(claimDetail({ id: String(params.id), statement: plain(`${String(params.id)} の詳細の文`) }))));
    await userEvent.setup().click(screen.getByRole("link", { name: "c-a" }));
    expect(await screen.findByText("c-a の詳細の文")).toBeTruthy();
    expect(window.location.pathname).toBe("/claims/c-a");
  });

  // @vanda:evidence term-feature-page-claims
  it("用語と機能の詳細のページはそれを対象に持つ約束がなければ約束の詳細へのリンクを出さない", async () => {
    for (const [path, handler] of [
      ["/terms/t-mid", http.get("/api/terms/:id", () => HttpResponse.json(termDetail({ claims: [] })))],
      ["/features/f-sort", http.get("/api/features/:id", () => HttpResponse.json(featureDetail({ claims: [] })))],
    ] as const) {
      openAt(path, handler);
      await waitFor(() => expect(shownText()).toContain("定義の中の用語"));
      const claimLinks = screen.getAllByRole("link").filter((a) => new URL((a as HTMLAnchorElement).href).pathname.startsWith("/claims/"));
      expect(claimLinks, `${path}: 約束がないのに約束の詳細へのリンクがある`).toHaveLength(0);
      cleanup();
      server.resetHandlers();
    }
  });

  // @vanda:evidence term-feature-page-missing
  it("今の約束リストにないIDの用語と機能の詳細のページを開くとそのIDが約束リストにないことを示す", async () => {
    for (const [path, api, message] of [
      ["/terms/t-none", "/api/terms/:id", "用語 t-none は約束リストにありません。"],
      ["/features/f-none", "/api/features/:id", "機能 f-none は約束リストにありません。"],
    ] as const) {
      openAt(path, http.get(api, () => apiError(404, message)));
      const id = path.split("/")[2];
      await waitFor(() => expect(shownText(), `${path}: 約束リストにないことを示さない`).toContain("約束リストにありません"));
      expect(shownText(), `${path}: ID がない`).toContain(id);
      expect(shownText(), `${path}: 読み込み中のまま`).not.toContain("読み込んでいます");
      cleanup();
      server.resetHandlers();
    }
  });
});

describe("読み込みの間に先に出すもの", () => {
  // before は、a が b より文書の前にあるか
  const before = (a: Node, b: Node) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
  // ストーリーのレビューから移ってきた時の承認のボタンの読み込みと混ざらないよう、レビューのクエリを付けずに開く
  beforeEach(() => window.history.pushState({}, "", "/claims/task-id-unique"));

  // @vanda:evidence claim-detail-shown-before-history
  // @vanda:evidence loading-indicator
  it("変更履歴と承認の履歴が届く前に約束の詳細の中身を出し変更履歴と承認の履歴のエリアには読み込み中のマークを出し届いたら履歴に替える", async () => {
    const g = gate();
    server.use(
      http.get("/api/context", () => HttpResponse.json(ctx)),
      http.get("/api/claims/:id", () => HttpResponse.json(claimDetail({
        ...detail,
        state: "agreed",
        guesses: [plain("先に出す推測")],
      }))),
      http.get("/api/claims/:id/history", async () => {
        await g.wait;
        return HttpResponse.json(history);
      }),
    );
    render(<App />);
    expect(await screen.findByRole("heading", { level: 1, name: /task-id-unique/ })).toBeTruthy();
    // ID・状態・文・理由・条件・レイヤー・対象・推測・エビデンスの予定と実際のエビデンス
    for (const t of ["承認済", "タスクの ID は重ならない", "重なると消す相手を決められない", "タスクが 1 つ以上ある", "モデル", "タスクの ID", "先に出す推測",
      "exec vanda add 買い物", "TestUnique", "internal/task/task_test.go", "spec/Task.tla", "db/schema.sql", "まだ書いていない"]) {
      expect(bodyText(), `変更履歴と承認の履歴の前に ${t} が出ない`).toContain(t);
    }
    // 変更履歴と承認の履歴はまだ出さず、それぞれのエリアに読み込み中のマークを出す
    for (const t of ["#1 はじめの約束を置く", "#24 文を変える", "前に承認した人"]) expect(bodyText(), `届く前に ${t} が出た`).not.toContain(t);
    const origin = screen.getByRole("heading", { name: "変更履歴" });
    const agreements = screen.getByRole("heading", { name: "承認の履歴" });
    const loads = screen.getAllByText("読み込んでいます…");
    expect(loads.length, "読み込み中のマークが変更履歴と承認の履歴の 2 つでない").toBe(2);
    expect(before(origin, loads[0]) && before(loads[0], agreements) && before(agreements, loads[1]), "読み込み中のマークが変更履歴と承認の履歴のエリアにない").toBe(true);
    loads.forEach((el, i) => expectLoading(el, ["変更履歴", "承認の履歴"][i]));

    g.open();
    await waitFor(() => expect(bodyText()).toContain("#24 文を変える"));
    for (const t of ["#1 はじめの約束を置く", "前に承認した人"]) expect(bodyText()).toContain(t);
    expect(screen.queryByText("読み込んでいます…"), "届いた後も読み込み中のマークが残る").toBeNull();
  });

  // @vanda:evidence loading-indicator
  it("約束の詳細が届く前は読み込み中のマークを出す", async () => {
    const g = gate();
    server.use(
      http.get("/api/context", () => HttpResponse.json(ctx)),
      http.get("/api/claims/:id", async () => {
        await g.wait;
        return HttpResponse.json(detail);
      }),
      http.get("/api/claims/:id/history", () => HttpResponse.json(history)),
    );
    render(<App />);
    expectLoading(await screen.findByText("読み込んでいます…"), "約束の詳細");
    expect(bodyText()).not.toContain("タスクの ID は重ならない");
    g.open();
    expect(await screen.findByText(/タスクの ID は重ならない/)).toBeTruthy();
    await waitFor(() => expect(screen.queryByText("読み込んでいます…")).toBeNull());
  });
});
