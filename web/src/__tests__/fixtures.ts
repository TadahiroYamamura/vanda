// 画面の部品のテストで使う、API の結果の例と、API を差し替える MSW のサーバー。
// 形は docs/formats.md の「GUI の API」。中身は、テストの中で手で作る（vanda の出力から作らない）。
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import type { ClaimDetail, Context, Entry, Review, Story } from "../api";

export const ctx: Context = {
  version: "0.1.0",
  branch: "story-24",
  head: "0123456789abcdef0123456789abcdef01234567",
  defaultBase: "main",
  bases: ["main", "release"],
  user: { name: "合意する人", email: "po@example.com" },
  stories: [
    { id: "#24", inBranch: true },
    { id: "#1", inBranch: false },
  ],
  layers: [
    { id: "model", name: "モデル" },
    { id: "db", name: "DB" },
  ],
  storyStore: "github",
};

export function claimEntry(no: number, id: string, over: Partial<Entry> = {}): Entry {
  return {
    no,
    kind: "claim",
    id,
    path: null,
    oldPath: null,
    fileType: null,
    change: "modified",
    state: "proposed",
    fingerprint: "sha256:" + id.padEnd(64, "0").slice(0, 64),
    flags: [],
    group: "layer:model",
    before: {
      statement: `${id} の前の文`,
      rationale: null,
      condition: null,
      subjects: ["task-id"],
      layers: ["model"],
      evidence: [{ stage: "test", type: "unit-test", layers: ["model"], detail: null }],
      flags: [],
      guesses: [],
    },
    after: {
      statement: `${id} の後の文`,
      rationale: null,
      condition: null,
      subjects: ["task-id"],
      layers: ["model"],
      evidence: [{ stage: "test", type: "unit-test", layers: ["model"], detail: null }],
      flags: [],
      guesses: [],
    },
    detachedAgreement: null,
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
    oldPath: null,
    fileType: "scenario",
    change: "modified",
    state: "proposed",
    fingerprint: "sha256:" + "f".repeat(64),
    flags: [],
    group: "files",
    before: { text: `${path} の前の中身` },
    after: { text: `${path} の後の中身` },
    detachedAgreement: null,
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
    statement: "タスクの ID は重ならない",
    rationale: "重なると消す相手を決められない",
    condition: "タスクが 1 つ以上ある",
    layers: [{ id: "model", name: "モデル" }],
    subjects: [{ kind: "term", id: "task-id", name: "タスクの ID" }],
    state: "proposed",
    fingerprint: "sha256:" + "a".repeat(64),
    flags: [],
    guesses: [],
    evidence: [],
    origin: [],
    agreements: [],
    ...over,
  };
}

export function apiError(status: number, message: string) {
  return HttpResponse.json({ error: { message } }, { status });
}

// requests は、テストの中で受けた要求（方法と URL と中身）を記録する。
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

// handlers は、よく使う既定の応答。テストごとに server.use で上書きする。
export function defaultHandlers(r: Review) {
  return [
    http.get("/api/context", () => HttpResponse.json(ctx)),
    http.get("/api/review", () => HttpResponse.json(r)),
    http.get("/api/story", () => HttpResponse.json(story)),
  ];
}
