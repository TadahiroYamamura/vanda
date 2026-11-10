// 画面の部品のテストで使う、API の結果の例と、API を差し替える MSW のサーバー。
// 形は docs/formats.md の「GUI の API」。中身は、テストの中で手で作る（vanda の出力から作らない）。
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { isInaccessible } from "@testing-library/dom";
import { expect } from "vitest";
import type { ClaimDetail, ClaimHistory, Context, Entry, Piece, RefTarget, Review, Story } from "../api";

export const ctx: Context = {
  version: "0.1.0",
  branch: "story-24",
  head: "0123456789abcdef0123456789abcdef01234567",
  defaultBase: "main",
  bases: ["main", "release"],
  user: { name: "承認する人", email: "po@example.com" },
  stories: [
    { id: "#24", inBranch: true },
    { id: "#1", inBranch: false },
  ],
  layers: [
    { id: "model", name: "モデル" },
    { id: "db", name: "DB" },
  ],
  storyStore: "github",
  trailerErrors: [],
};

// plain は、参照のない文をセグメントの配列にする（自然言語で書く項目は、API でセグメントの配列になる。docs/plans/stage4-references.md）。
export const plain = (text: string) => [{ text }];

// termRef・featureRef・claimRef は、レスポンスの最上位の refs の 1 つ（docs/formats.md の「自然言語で書く項目（セグメントの配列）」）。
// pieces を省けば、text を参照のない 1 つのセグメントにする
export const termRef = (name: string, text: string, current = true, pieces: Piece[] = plain(text)): RefTarget => ({ kind: "term", name, text, current, pieces });
export const featureRef = (name: string, text: string, o: { current?: boolean; core: boolean; formal: boolean; pieces?: Piece[] }): RefTarget => ({
  kind: "feature", name, text, current: o.current ?? true, pieces: o.pieces ?? plain(text), core: o.core, formal: o.formal,
});
export const claimRef = (text: string, current = true, pieces: Piece[] = plain(text)): RefTarget => ({ kind: "claim", name: null, text, current, pieces });

export function claimEntry(no: number, id: string, over: Partial<Entry> = {}): Entry {
  return {
    no,
    kind: "claim",
    id,
    path: null,
    fileType: null,
    change: "modified",
    state: "proposed",
    fingerprint: "sha256:" + id.padEnd(64, "0").slice(0, 64),
    flags: [],
    group: "layer:model",
    before: {
      statement: plain(`${id} の前の文`),
      rationale: null,
      condition: null,
      subjects: ["task-id"],
      layers: ["model"],
      evidence: [{ stage: "implementation", type: "unit-test", layers: ["model"], detail: null }],
      flags: [],
      guesses: [],
      modelCheckWaiver: null,
    },
    after: {
      statement: plain(`${id} の後の文`),
      rationale: null,
      condition: null,
      subjects: ["task-id"],
      layers: ["model"],
      evidence: [{ stage: "implementation", type: "unit-test", layers: ["model"], detail: null }],
      flags: [],
      guesses: [],
      modelCheckWaiver: null,
    },
    detachedAgreement: null,
    agree: { possible: true, reason: null },
    otherCommits: [],
    diff: null,
    ...over,
  };
}

export function fileEntry(no: number, path: string, over: Partial<Entry> = {}): Entry {
  return {
    no,
    kind: "file",
    id: null,
    path,
    fileType: "scenario",
    change: "modified",
    state: "proposed",
    fingerprint: "sha256:" + "f".repeat(64),
    flags: [],
    group: "files",
    before: { text: `${path} の前の中身` },
    after: { text: `${path} の後の中身` },
    detachedAgreement: null,
    agree: { possible: true, reason: null },
    otherCommits: [],
    diff: null,
    ...over,
  };
}

export function review(entries: Entry[], over: Partial<Review> = {}): Review {
  return {
    view: "branch",
    base: "main",
    mergeBase: "fedcba9876543210fedcba9876543210fedcba98",
    story: "#24",
    total: entries.length,
    entries,
    detached: [],
    refs: {},
    ...over,
  };
}

export const story: Story = {
  id: "#24",
  title: "タスクの ID を重ならないようにする",
  body: "本文です",
  url: "https://github.com/owner/app/issues/24",
  attachments: [{ name: "画面の案", url: "https://github.com/user-attachments/assets/1" }],
};

export function claimDetail(over: Partial<ClaimDetail> = {}): ClaimDetail {
  return {
    id: "task-id-unique",
    statement: plain("タスクの ID は重ならない"),
    rationale: plain("重なると消す相手を決められない"),
    condition: plain("タスクが 1 つ以上ある"),
    layers: [{ id: "model", name: "モデル" }],
    subjects: [{ kind: "term", id: "task-id", name: "タスクの ID" }],
    state: "proposed",
    fingerprint: "sha256:" + "a".repeat(64),
    flags: [],
    guesses: [],
    evidence: [],
    refs: {},
    ...over,
  };
}

// claimHistory は、約束の変更履歴と承認の履歴（GET /api/claims/<ID>/history）
export function claimHistory(over: Partial<ClaimHistory> = {}): ClaimHistory {
  return { origin: [], agreements: [], ...over };
}

export function apiError(status: number, message: string) {
  return HttpResponse.json({ error: { message } }, { status });
}

// requests は、テストの中で受けたリクエスト（方法と URL と中身）を記録する。
export const requests: { method: string; url: URL; body: unknown }[] = [];

export const server = setupServer();

server.events.on("request:start", async ({ request }) => {
  let body: unknown = null;
  if (request.method !== "GET") {
    try {
      body = await request.clone().json();
    } catch {
      body = null;
    }
  }
  requests.push({ method: request.method, url: new URL(request.url), body });
});

// handlers は、よく使う既定のレスポンス。テストごとに server.use で上書きする。
export function defaultHandlers(r: Review) {
  return [
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/review", () => HttpResponse.json(r)),
    http.get("/api/story", () => HttpResponse.json(story)),
    http.get("/api/claims/:id/history", () => HttpResponse.json(claimHistory())),
  ];
}

// gate は、テストが open を呼ぶまでレスポンスを止めておくための門。API が遅い間の画面を確かめるのに使う
export function gate() {
  let open!: () => void;
  const wait = new Promise<void>((resolve) => (open = resolve));
  return { open, wait };
}

// expectLoading は、el（「読み込んでいます…」の文字を持つ要素。getByText で探したもの）が読み込み中のマーク（loading-indicator）か確かめる。
// 文字はスクリーンリーダーに読み上げられ、スピナー（文字のない要素。el の中か、el と並ぶもの）は飾りとしてスクリーンリーダーの読み上げから外れている
export function expectLoading(el: HTMLElement, where: string) {
  expect(el.textContent ?? "", `${where}: 「読み込んでいます…」の文字がない`).toContain("読み込んでいます…");
  expect(isInaccessible(el), `${where}: 「読み込んでいます…」がスクリーンリーダーの読み上げから外れている`).toBe(false);
  const empty = (x: Element) => (x.textContent ?? "").trim() === "";
  let marks = Array.from(el.querySelectorAll<HTMLElement>("*")).filter(empty);
  if (marks.length === 0 && el.parentElement) marks = Array.from(el.parentElement.children).filter((x): x is HTMLElement => x !== el && empty(x));
  expect(marks.length, `${where}: スピナーがない`).toBeGreaterThan(0);
  for (const m of marks) expect(isInaccessible(m), `${where}: スピナーが飾りになっていない（スクリーンリーダーに読み上げられる）`).toBe(true);
}
