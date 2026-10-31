// API の形は docs/formats.md の「GUI の API」。画面は API の結果を見せるだけで、判定を持たない。

export type Kind = "claim" | "term" | "feature" | "file";
export type FileType = "scenario" | "oracle" | "guardrail";
export type Change = "added" | "modified" | "deleted" | "renamed" | "unchanged";
export type State = "proposed" | "agreed";
export type Flag = "ai-proposal" | "guess" | "changed-since-agreement";

export interface User {
  name: string;
  email: string;
}

export interface LayerRef {
  id: string;
  name: string;
}

export interface Context {
  version: string;
  branch: string;
  head: string;
  defaultBase: string;
  bases: string[];
  user: User | null;
  stories: { id: string; inBranch: boolean }[];
  layers: LayerRef[];
  storyStore: string;
}

export interface EvidenceContent {
  stage: string;
  type: string;
  layers: string[];
  detail: string | null;
}

export interface ClaimContent {
  statement: string;
  rationale: string | null;
  condition: string | null;
  subjects: string[];
  layers: string[];
  evidence: EvidenceContent[];
  flags: string[];
  guesses: string[];
}

export interface TermContent {
  name: string;
  definition: string;
  partOf: string | null;
}

export interface FeatureContent {
  name: string;
  definition: string;
  core: boolean | null;
  formal: boolean | null;
  formalWaiver: string | null;
}

export interface FileContent {
  text: string;
}

export type Content = ClaimContent | TermContent | FeatureContent | FileContent;

export interface CommitRef {
  commit: string;
  subject: string;
  stories: string[];
}

export interface DiffPart {
  op: "equal" | "delete" | "insert";
  text: string;
}

export interface DiffLine {
  op: "equal" | "delete" | "insert";
  before: number | null;
  after: number | null;
  text: string;
  parts: DiffPart[] | null;
  noNewline?: boolean;
}

export type Diff = { lines: DiffLine[] } | { fields: Record<string, DiffPart[]> };

export interface Entry {
  no: number | null;
  kind: Kind;
  id: string | null;
  path: string | null;
  oldPath: string | null;
  fileType: FileType | null;
  change: Change;
  state: State | null;
  fingerprint: string | null;
  flags: Flag[];
  group: string;
  before: Content | null;
  after: Content | null;
  diff: Diff | null;
  detachedAgreement: { by: string[] } | null;
  revoke?: Revocation | null;
  otherCommits: CommitRef[];
}

export interface Revocation {
  possible: boolean;
  reason: string | null;
  others: User[];
}

export interface RevokeResult {
  commit: string;
  stillAgreed: boolean;
  removed: number;
}

export interface Review {
  view: "branch" | "story";
  base: string | null;
  mergeBase: string | null;
  story: string | null;
  total: number;
  entries: Entry[];
  detached: Entry[];
}

export interface Story {
  id: string;
  title: string;
  body: string;
  url: string;
  attachments: { name: string; url: string }[];
}

export interface Found {
  adapter: string;
  file: string;
  line: number;
  name: string;
  text: string | null;
}

export interface EvidenceDetail extends EvidenceContent {
  found: Found[];
  present: boolean | null;
}

export interface ClaimDetail {
  id: string;
  statement: string;
  rationale: string | null;
  condition: string | null;
  layers: LayerRef[];
  subjects: { kind: "term" | "feature" | null; id: string; name: string | null }[];
  state: State;
  fingerprint: string | null;
  flags: string[];
  guesses: string[];
  evidence: EvidenceDetail[];
  origin: { commit: string; date: string; subject: string; stories: string[]; change: "added" | "modified" }[];
  agreements: { user: User; agreedAt: string; story: string; fingerprint: string; current: boolean; commit?: string | null }[];
}

export interface ClaimAt {
  id: string;
  commit: string;
  date: string;
  subject: string;
  stories: string[];
  content: ClaimContent;
  layers?: LayerRef[];
}

export interface AgreeResult {
  commit: string;
}

export class ApiError extends Error {
  readonly status: number;
  readonly fix: string;
  constructor(status: number, message: string, fix: string) {
    super(message);
    this.status = status;
    this.fix = fix;
  }
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, init);
  } catch (e) {
    throw new ApiError(0, `vanda のサーバーに繋がりません（${String(e)}）。`, "vanda serve が動いているかを確かめてください。");
  }
  const text = await res.text();
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    throw new ApiError(res.status, `サーバーの応答が JSON ではありません（${res.status}）: ${text.slice(0, 200)}`, "vanda serve を起動し直してください。");
  }
  if (!res.ok) {
    const err = (body as { error?: { message?: string; fix?: string } }).error;
    throw new ApiError(res.status, err?.message ?? `サーバーが ${res.status} を返しました。`, err?.fix ?? "");
  }
  return body as T;
}

const q = (params: Record<string, string | null | undefined>) => {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v) u.set(k, v);
  return u.toString();
};

export const api = {
  context: () => call<Context>("/api/context"),
  review: (view: string, base: string | null, story: string | null) =>
    call<Review>(`/api/review?${q({ view, base, story })}`),
  story: (id: string) => call<Story>(`/api/story?${q({ id })}`),
  claim: (id: string) => call<ClaimDetail>(`/api/claims/${encodeURIComponent(id)}`),
  claimAt: (id: string, commit: string) => call<ClaimAt>(`/api/claims/${encodeURIComponent(id)}/at/${encodeURIComponent(commit)}`),
  file: (path: string) => call<{ path: string; text: string }>(`/api/file?${q({ path })}`),
  revoke: (e: Entry, story: string, base: string | null) =>
    call<RevokeResult>("/api/revoke", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        kind: e.kind,
        id: e.kind === "file" ? null : e.id,
        path: e.kind === "file" ? e.path : null,
        fingerprint: e.fingerprint,
        story,
        base,
      }),
    }),
  agree: (e: Entry, story: string) =>
    call<AgreeResult>("/api/agree", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        kind: e.kind,
        id: e.kind === "file" ? null : e.id,
        path: e.kind === "file" ? e.path : null,
        fingerprint: e.fingerprint,
        story,
      }),
    }),
};
