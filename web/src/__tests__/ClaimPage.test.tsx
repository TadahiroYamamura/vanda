// CL（約束の詳細）の画面が、API の結果を正しく見せるか。API は MSW で差し替える。
import { afterAll, afterEach, beforeAll, beforeEach, expect, it } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { App } from "../App";
import type { ClaimDetail, Entry } from "../api";
import { apiError, claimDetail, claimEntry, ctx, fileEntry, review, server, story } from "./fixtures";

beforeAll(() => server.listen({ onUnhandledFrame: "error" }));
afterAll(() => server.close());
beforeEach(() => window.history.pushState({}, "", "/claims/task-id-unique?view=branch&base=main"));
afterEach(() => {
  cleanup();
  server.resetHandlers();
});

const bodyText = () => document.body.textContent ?? "";

function open(d: ClaimDetail) {
  server.use(
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/claims/:id", () => HttpResponse.json(d)),
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
    { stage: "implementation", type: "random-check", layers: ["model"], detail: "まだ書いていない", present: false, found: [] },
  ],
  origin: [
    { commit: "1".repeat(40), date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" },
    { commit: "2".repeat(40), date: "2026-10-20T12:00:00+09:00", subject: "#24 文を変える", stories: ["#24"], change: "modified" },
  ],
  agreements: [
    { user: { name: "前に合意した人", email: "a@example.com" }, agreedAt: "2026-10-02T12:00:00+09:00", story: "#1", fingerprint: "sha256:" + "b".repeat(64), current: false },
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
it("由来にコミットとストーリーを合意の履歴に合意した人とストーリーと今の中身に合うかを見せる", async () => {
  open(detail);
  await screen.findByText(/タスクの ID は重ならない/);
  await waitFor(() => expect(bodyText()).toContain("#24 文を変える"));
  for (const t of ["#1 はじめの約束を置く", "#24", "前に合意した人"]) expect(bodyText()).toContain(t);
  // 今の中身に合わない合意しかないので、合意済とは見せない
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
// 合意・取り消しのボタンを出す。レビューから移ってきたかは、レビューの画面と同じ URL のクエリ（view・base・story）で分かる
const fromReview = "?view=branch&base=main&story=%2324";

function openFromReview(path: string, entries: Entry[], d: ClaimDetail = claimDetail()) {
  window.history.pushState({}, "", path);
  server.use(
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/review", () => HttpResponse.json(review(entries))),
    http.get("/api/story", () => HttpResponse.json(story)),
    http.get("/api/claims/:id", () => HttpResponse.json(d)),
    http.get("/api/file", () => HttpResponse.json({ path: "e2e/add-task.txtar", text: "exec vanda add 買い物\n" })),
  );
  render(<App />);
}

// @vanda:evidence agree-from-detail-pages
it("ストーリーのレビューから移ってきた約束の詳細の画面とファイルの画面で一覧にあるものには合意のボタンを出す", async () => {
  openFromReview("/claims/task-id-unique" + fromReview, [claimEntry(1, "task-id-unique")]);
  expect(await screen.findByRole("button", { name: /合意する/ })).toBeTruthy();
  cleanup();
  openFromReview("/files" + fromReview + "&path=" + encodeURIComponent("e2e/add-task.txtar"), [fileEntry(1, "e2e/add-task.txtar")]);
  expect(await screen.findByRole("button", { name: /合意する/ })).toBeTruthy();
});

// @vanda:evidence agree-from-detail-pages
it("ストーリーのレビューから移ってきた約束の詳細の画面で合意済の約束には取り消しのボタンを出す", async () => {
  const e = claimEntry(1, "task-id-unique", { state: "agreed", revoke: { possible: true, reason: null, others: [] } });
  openFromReview("/claims/task-id-unique" + fromReview, [e], claimDetail({ state: "agreed" }));
  expect(await screen.findByRole("button", { name: /取り消/ })).toBeTruthy();
});

// @vanda:evidence agree-from-detail-pages-notes
it("一覧にない約束とファイルは変わっていないと示しストーリーが分からないときはレビューから開くと合意できると示す", async () => {
  openFromReview("/claims/task-id-unique" + fromReview, [claimEntry(1, "other-claim")]);
  expect(await screen.findByText("このストーリーでは変わっていない約束です。")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /合意する/ })).toBeNull();
  cleanup();
  openFromReview("/files" + fromReview + "&path=" + encodeURIComponent("e2e/add-task.txtar"), [claimEntry(1, "task-id-unique")]);
  expect(await screen.findByText("このストーリーでは変わっていないファイルです。")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /合意する/ })).toBeNull();
  cleanup();
  // URL にレビューのクエリがない（直接開いた）
  openFromReview("/claims/task-id-unique", [claimEntry(1, "task-id-unique")]);
  expect(await screen.findByText("ストーリーのレビューから開くと、合意できます。")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /合意する/ })).toBeNull();
});

// @vanda:evidence agree-from-detail-pages-stay
it("約束の詳細の画面で合意した後もその画面にとどまる", async () => {
  const e = claimEntry(1, "task-id-unique");
  let posted: unknown = null;
  openFromReview("/claims/task-id-unique" + fromReview, [e]);
  server.use(
    http.post("/api/agree", async ({ request }) => {
      posted = await request.json();
      return HttpResponse.json({ commit: "c".repeat(40), agreement: {} });
    }),
  );
  const button = await screen.findByRole("button", { name: /合意する/ });
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

// CL-08: 由来のコミットか合意の記録を選ぶと、その時点の約束の中身を示す
// @vanda:evidence claim-at-commit
it("由来のコミットと合意の記録を開くとその時点の約束の中身を読んで見せる", async () => {
  const commit = "1".repeat(40);
  const agreedAt = "2".repeat(40);
  const at = new Map<string, string>([[commit, "作った時点の文"], [agreedAt, "合意した時点の文"]]);
  server.use(
    http.get("/api/claims/:id/at/:commit", ({ params }) =>
      HttpResponse.json({
        id: "task-id-unique", commit: params.commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめ", stories: ["#1"],
        content: { statement: at.get(String(params.commit)) ?? "?", rationale: null, condition: null, subjects: ["task-id"], layers: ["model"], evidence: [], flags: [], guesses: [] },
      }),
    ),
  );
  open(claimDetail({
    origin: [{ commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" }],
    agreements: [{ user: { name: "前に合意した人", email: "a@example.com" }, agreedAt: "2026-10-02T12:00:00+09:00", story: "#1", fingerprint: "sha256:" + "b".repeat(64), current: false, commit: agreedAt }],
  }));
  const user = userEvent.setup();
  const originRow = (await screen.findByText(/#1 はじめの約束を置く/)).closest("tr")!;
  await user.click(within(originRow).getByRole("button", { name: "その時点の中身を開く" }));
  expect(await screen.findByText("作った時点の文")).toBeTruthy();
  const agreementRow = screen.getByText(/前に合意した人/).closest("tr")!;
  await user.click(within(agreementRow).getByRole("button", { name: "その時点の中身を開く" }));
  expect(await screen.findByText("合意した時点の文")).toBeTruthy();
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
it("実際のエビデンスのうち合意の対象のファイルには今の中身が合意済か提案中かを示し合意の対象でないものには示さない", async () => {
  const states: ["agreed" | "proposed" | null, string | null][] = [["agreed", "合意済"], ["proposed", "提案中"], [null, null]];
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
    for (const label of ["合意済", "提案中"]) {
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
  open(claimDetail({ evidence: [{ stage: "plan", type: "model-check", layers: ["model"], detail, present: false, found: [] }] }));
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
        content: { statement: "その時点の" + spaced, rationale: "理由の" + spaced, condition: "条件の" + spaced, subjects: ["task-id"], layers: ["model"], evidence: [], flags: [], guesses: [] },
        layers: [{ id: "model", name: "モデル" }],
      }),
    ),
  );
  open(claimDetail({
    statement: spaced, rationale: "今の理由の" + spaced, condition: "今の条件の" + spaced,
    origin: [{ commit, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" }],
  }));
  const has = (t: string) => Array.from(document.querySelectorAll<HTMLElement>("*")).some((el) => el.textContent === t);
  await waitFor(() => expect(has(spaced), "約束の文").toBe(true));
  for (const t of ["今の理由の" + spaced, "今の条件の" + spaced]) expect(has(t), t).toBe(true);
  const row = screen.getByText(/#1 はじめの約束を置く/).closest("tr")!;
  await userEvent.setup().click(within(row).getByRole("button", { name: "その時点の中身を開く" }));
  await waitFor(() => expect(has("その時点の" + spaced), "その時点の文").toBe(true));
  for (const t of ["理由の" + spaced, "条件の" + spaced]) expect(has(t), t).toBe(true);
});

// CL-08 の、その時点の中身の開け閉め。由来の 2 つと合意の記録の 1 つを使う
const atCommits = { first: "1".repeat(40), second: "2".repeat(40), agreed: "3".repeat(40) };
const atStatement: Record<string, string> = { [atCommits.first]: "はじめの時点の文", [atCommits.second]: "次の時点の文", [atCommits.agreed]: "合意した時点の文" };

function openHistory(at: Parameters<typeof http.get>[1] = ({ params }) =>
  HttpResponse.json({
    id: "task-id-unique", commit: params.commit, date: "2026-10-01T12:00:00+09:00", subject: "s", stories: ["#1"],
    content: { statement: atStatement[String(params.commit)] ?? "?", rationale: null, condition: null, subjects: ["task-id"], layers: ["model"], evidence: [], flags: [], guesses: [] },
    layers: [{ id: "model", name: "モデル" }],
  })) {
  server.use(http.get("/api/claims/:id/at/:commit", at));
  open(claimDetail({
    origin: [
      { commit: atCommits.second, date: "2026-10-20T12:00:00+09:00", subject: "#24 文を変える", stories: ["#24"], change: "modified" },
      { commit: atCommits.first, date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" },
    ],
    agreements: [{ user: { name: "前に合意した人", email: "a@example.com" }, agreedAt: "2026-10-02T12:00:00+09:00", story: "#1", fingerprint: "sha256:" + "b".repeat(64), current: false, commit: atCommits.agreed }],
  }));
}

const historyRow = async (text: RegExp) => (await screen.findByText(text)).closest("tr")! as HTMLElement;
const toggleOf = (row: HTMLElement) => within(row).getByRole("button", { name: /その時点の中身を(開く|たたむ)/ });

// @vanda:evidence claim-at-commit-toggle
it("由来と合意の履歴の行を押すとその時点の中身が開きもう一度押すとたたまりボタンを押しても1回だけ切り替わる", async () => {
  openHistory();
  const user = userEvent.setup();
  for (const [text, statement] of [[/#24 文を変える/, "次の時点の文"], [/前に合意した人/, "合意した時点の文"]] as const) {
    const row = await historyRow(text);
    const cell = within(row).getByText(text);
    expect(toggleOf(row).getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText(statement)).toBeNull();
    await user.click(cell);
    expect(await screen.findByText(statement), "行を押しても開かない").toBeTruthy();
    expect(toggleOf(row).getAttribute("aria-expanded")).toBe("true");
    await user.click(cell);
    await waitFor(() => expect(screen.queryByText(statement), "行をもう一度押してもたたまない").toBeNull());
    expect(toggleOf(row).getAttribute("aria-expanded")).toBe("false");
    // ボタンは行の中にあるので、行の動きと二重に切り替わると、押しても閉じたままになる
    await user.click(toggleOf(row));
    expect(await screen.findByText(statement), "ボタンを押しても開かない").toBeTruthy();
    await user.click(toggleOf(row));
    await waitFor(() => expect(screen.queryByText(statement), "ボタンを押してもたたまない").toBeNull());
  }
});

// @vanda:evidence claim-at-commit-multiple-open
it("その時点の中身を2つ以上開くとそれぞれが同時に出たままになり1つをたたんでもほかは開いたままにする", async () => {
  openHistory();
  const user = userEvent.setup();
  const rows: [RegExp, string][] = [[/#24 文を変える/, "次の時点の文"], [/#1 はじめの約束を置く/, "はじめの時点の文"], [/前に合意した人/, "合意した時点の文"]];
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
      content: { statement: "古いレイヤーの時点の文", rationale: null, condition: null, subjects: ["task-id"], layers: ["model", "old-layer"], evidence: [], flags: [], guesses: [] },
      layers: [{ id: "model", name: "モデル" }, { id: "old-layer", name: "" }],
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
it("その時点の約束リストが読めない合意の記録を選ぶとAPIが返した理由を出す", async () => {
  openHistory(() => apiError(500, "コミット 3333333 の .vanda/promises.toml を読めません（4 行目）"));
  const row = await historyRow(/前に合意した人/);
  await userEvent.setup().click(toggleOf(row));
  expect(await screen.findByText(/コミット 3333333 の \.vanda\/promises\.toml を読めません/)).toBeTruthy();
});
