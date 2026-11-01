// CL（約束の詳細）の画面が、API の結果を正しく見せるか。API は MSW で差し替える。
import { afterAll, afterEach, beforeAll, beforeEach, expect, it } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { App } from "../App";
import type { ClaimDetail } from "../api";
import { apiError, claimDetail, ctx, server } from "./fixtures";

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
    { stage: "test", type: "scenario-test", layers: ["model"], detail: null, present: true,
      found: [{ adapter: "txtar", file: "e2e/add-task.txtar", line: 1, name: "add-task", text: "exec vanda add 買い物\n" }] },
    { stage: "plan", type: "model-check", layers: ["model"], detail: null, present: true,
      found: [{ adapter: "tla", file: "spec/Task.tla", line: 5, name: "Unique", text: tla }] },
    { stage: "implementation", type: "guardrail", layers: ["model"], detail: null, present: true,
      found: [{ adapter: "sql", file: "db/schema.sql", line: 4, name: "CREATE UNIQUE INDEX t ON task (id);", text: "CREATE UNIQUE INDEX t ON task (id);\n" }] },
    { stage: "test", type: "unit-test", layers: ["model"], detail: null, present: true,
      found: [{ adapter: "go", file: "internal/task/task_test.go", line: 6, name: "TestUnique", text: null }] },
    { stage: "test", type: "random-check", layers: ["model"], detail: "まだ書いていない", present: false, found: [] },
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

it("約束の詳細を読めないときは理由を出す", async () => {
  server.use(
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/claims/:id", () => apiError(404, "約束 task-id-unique は約束リストにない")),
  );
  render(<App />);
  expect(await screen.findByText(/約束 task-id-unique は約束リストにない/)).toBeTruthy();
});
