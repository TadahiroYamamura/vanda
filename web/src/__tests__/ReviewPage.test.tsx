// RV（ストーリーのレビュー）の画面が、API の結果を正しく見せるか。API は MSW で差し替える。
// 画面に出す言葉は、約束リスト（.vanda/promises.toml）の用語（AI 案・推測・差分あり・提案中・合意済・合意 など）で探す。
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { App } from "../App";
import type { Entry } from "../api";
import { apiError, claimDetail, claimEntry, ctx, defaultHandlers, fileEntry, requests, review, server, story } from "./fixtures";

beforeAll(() => server.listen({ onUnhandledFrame: "error" }));
afterAll(() => server.close());
beforeEach(() => {
  requests.length = 0;
  window.history.pushState({}, "", "/?view=branch&base=main");
});
afterEach(() => {
  cleanup();
  server.resetHandlers();
});

// open は、既定の応答で画面を開く。extra は既定より先に当てる応答（server.use は後に足したものが先に当たる）。
function open(entries: Entry[], over = {}, ...extra: Parameters<typeof server.use>) {
  server.use(...defaultHandlers(review(entries, over)));
  if (extra.length > 0) server.use(...extra);
  render(<App />);
}

// openEntry は、一覧で text の件を選び、1 件の詳細を開く。
async function openEntry(text: RegExp) {
  const [item] = await screen.findAllByText(text);
  await userEvent.setup().click(item);
}

const bodyText = () => document.body.textContent ?? "";

const reviewRequests = () => requests.filter((r) => r.url.pathname === "/api/review");

describe("RV-04 フラグ", () => {
  // @vanda:evidence review-flag-labels
  it("フラグは短い言葉の札で出し説明は札に触れたときかフォーカスしたときに出す", async () => {
    open([claimEntry(1, "task-id-unique", { flags: ["ai-proposal", "guess", "changed-since-agreement"] })]);
    const user = userEvent.setup();
    for (const label of [/^AI\s?案/, /^推測/, /^差分あり/]) {
      const [badge] = await screen.findAllByText(label, { selector: "*:not([role=tooltip])" });
      expect(within(badge).queryByRole("tooltip"), "触れる前から説明が出ている").toBeNull();
      await user.hover(badge);
      const tip = await within(badge).findByRole("tooltip");
      expect((tip.textContent ?? "").length).toBeGreaterThan(0);
      await user.unhover(badge);
      await waitFor(() => expect(within(badge).queryByRole("tooltip")).toBeNull());
      badge.focus();
      expect(await within(badge).findByRole("tooltip")).toBeTruthy();
      badge.blur();
    }
  });

  // @vanda:evidence review-flag-labels
  it("フラグのないものには札を出さない", async () => {
    open([claimEntry(1, "plain-claim")]);
    await screen.findAllByText(/plain-claim/);
    for (const label of [/AI\s?案/, /推測/, /差分あり/]) expect(screen.queryByText(label)).toBeNull();
  });
});

describe("RV-01・RV-02・RV-05 一覧と中身", () => {
  // @vanda:evidence review-numbering
  it("通し番号と総数を出し追加と変更と削除と名前の変更を区別して見せる", async () => {
    open([
      claimEntry(1, "c-added", { change: "added", before: null }),
      claimEntry(2, "c-modified"),
      claimEntry(3, "c-deleted", { change: "deleted", after: null, state: null, fingerprint: null }),
      fileEntry(4, "e2e/new.txtar", { change: "renamed", oldPath: "e2e/old.txtar" }),
    ]);
    for (const id of ["c-added", "c-modified", "c-deleted", "e2e/new.txtar"]) await screen.findAllByText(new RegExp(id));
    expect(bodyText()).toMatch(/4/);
    for (const w of [/追加/, /変更/, /削除/, /名前の変更/]) expect(bodyText()).toMatch(w);
    // 名前の変更の前の場所は、1 件の詳細に出す
    await openEntry(/e2e\/new\.txtar/);
    expect(bodyText()).toContain("e2e/old.txtar");
  });

  // @vanda:evidence review-before-after
  it("変更したものは変更前と変更後の中身を見せる", async () => {
    open([claimEntry(1, "c-modified")]);
    const user = userEvent.setup();
    const [item] = await screen.findAllByText(/c-modified/);
    if (!screen.queryByText(/c-modified の前の文/)) await user.click(item);
    expect(await screen.findByText(/c-modified の前の文/)).toBeTruthy();
    expect(await screen.findByText(/c-modified の後の文/)).toBeTruthy();
  });

  // @vanda:evidence review-state-shown
  it("一覧の1件ごとに合意の状態を提案中か合意済かで示す", async () => {
    const states = [["proposed", "提案中"], ["agreed", "合意済"]] as const;
    open(states.flatMap(([st], i) => [claimEntry(2 * i + 1, `c-${st}`, { state: st }), fileEntry(2 * i + 2, `e2e/${st}.txtar`, { state: st })]));
    for (const [st, label] of states) {
      for (const name of [`c-${st}`, `e2e/${st}.txtar`]) {
        const [item] = await screen.findAllByText(name);
        const li = item.closest("li")!;
        expect(li, `${name} の一覧の 1 件がない`).toBeTruthy();
        for (const [, l] of states) expect((li.textContent ?? "").includes(l), `${name} の「${l}」`).toBe(l === label);
      }
    }
  });

  // @vanda:evidence tla-math-symbols
  it("オラクルの定義のファイルの中身は数学の記号で見せる", async () => {
    open([fileEntry(1, "spec/Task.tla", { fileType: "oracle", before: { text: "S == a /\\ b\n" }, after: { text: "S == a \\/ b\n" } })]);
    await openEntry(/spec\/Task\.tla/);
    // 差分で文字ごとに分けて見せることがあるので、要素ごとでなく画面の文で探す
    await waitFor(() => expect(bodyText()).toContain("a ∨ b"));
    expect(bodyText()).toContain("a ∧ b");
    expect(bodyText()).not.toContain("/\\");
  });
});

describe("RV-10 長い一覧をたたむ", () => {
  // @vanda:evidence review-groups-fold
  it("長いまとまりはたたんでたたんだ件数を示し開くと中が見える", async () => {
    const files = Array.from({ length: 60 }, (_, i) => fileEntry(i + 2, `e2e/s${String(i).padStart(2, "0")}.txtar`));
    open([claimEntry(1, "c-one"), ...files]);
    await screen.findAllByText(/c-one/);
    expect(screen.queryByText(/e2e\/s59\.txtar/)).toBeNull();
    expect(document.body.textContent).toMatch(/60/);
    const user = userEvent.setup();
    const toggle = screen.getAllByRole("button").find((b) => /60/.test(b.textContent ?? "") || /ファイル/.test(b.textContent ?? ""));
    expect(toggle, "たたんだまとまりを開くボタンがない").toBeTruthy();
    await user.click(toggle!);
    expect((await screen.findAllByText(/e2e\/s59\.txtar/)).length).toBeGreaterThan(0);
  });
});

describe("RV-06 合意する", () => {
  // @vanda:evidence agree-on-screen
  it("合意すると見せていた中身の指紋とストーリーを送り一覧を読み直す", async () => {
    const e = claimEntry(1, "task-id-unique");
    open([e]);
    let posted: unknown = null;
    server.use(
      http.post("/api/agree", async ({ request }) => {
        posted = await request.json();
        return HttpResponse.json({ commit: "c".repeat(40), agreement: {} });
      }),
    );
    const user = userEvent.setup();
    const [item] = await screen.findAllByText(/task-id-unique/);
    let button = screen.queryAllByRole("button", { name: /合意する/ })[0];
    if (!button) {
      await user.click(item);
      button = await screen.findByRole("button", { name: /合意する/ });
    }
    const before = reviewRequests().length;
    await user.click(button);
    await waitFor(() => expect(posted).toEqual({ kind: "claim", id: "task-id-unique", path: null, fingerprint: e.fingerprint, story: "#24" }));
    await waitFor(() => expect(reviewRequests().length).toBeGreaterThan(before));
  });

  // @vanda:evidence agree-refusal-shown
  it("合意を断られたら理由を出す", async () => {
    open([claimEntry(1, "task-id-unique")]);
    server.use(http.post("/api/agree", () => apiError(409, "約束リストに書きかけの変更がある。先にコミットする")));
    const user = userEvent.setup();
    const [item] = await screen.findAllByText(/task-id-unique/);
    let button = screen.queryAllByRole("button", { name: /合意する/ })[0];
    if (!button) {
      await user.click(item);
      button = await screen.findByRole("button", { name: /合意する/ });
    }
    await user.click(button);
    expect(await screen.findByText(/約束リストに書きかけの変更がある/)).toBeTruthy();
  });

  // @vanda:evidence context-no-user-cannot-agree
  it("合意できない人のときは合意のボタンを押せず理由を出す", async () => {
    server.use(http.get("/api/context", () => HttpResponse.json({ ...ctx, user: null })));
    server.use(http.get("/api/review", () => HttpResponse.json(review([claimEntry(1, "task-id-unique")]))));
    server.use(http.get("/api/story", () => HttpResponse.json(story)));
    render(<App />);
    const user = userEvent.setup();
    const [item] = await screen.findAllByText(/task-id-unique/);
    if (!screen.queryAllByRole("button", { name: /合意する/ })[0]) await user.click(item);
    for (const b of screen.queryAllByRole("button", { name: /合意する/ })) expect((b as HTMLButtonElement).disabled).toBe(true);
    expect(document.body.textContent).toMatch(/user\.name|user\.email/);
  });
});

describe("RV-13・RV-14 レビュー単位と外れた合意", () => {
  // @vanda:evidence review-detached-shown
  // @vanda:evidence review-detached-unchanged-shown
  it("外れた合意をそれを外したストーリーと一緒に示す", async () => {
    const detached = claimEntry(0, "c-detached", { no: null, change: "unchanged", before: null, detachedAgreement: { by: ["#30"] } });
    open([claimEntry(1, "c-changed", { detachedAgreement: { by: ["#31"] } })], { detached: [detached] });
    await screen.findAllByText(/c-changed/);
    await screen.findAllByText(/c-detached/);
    expect(bodyText()).toMatch(/合意が外れた/);
    await openEntry(/c-changed/);
    await waitFor(() => expect(bodyText()).toContain("#31"));
    await openEntry(/c-detached/);
    await waitFor(() => expect(bodyText()).toContain("#30"));
  });

  // @vanda:evidence review-other-commits
  it("レビュー単位がストーリーのときはほかのストーリーが途中で変えたことを示す", async () => {
    window.history.pushState({}, "", "/?view=story&story=%2324");
    open(
      [
        claimEntry(1, "c-shared", {
          otherCommits: [{ commit: "d".repeat(40), subject: "#30 で同じ約束を変える", stories: ["#30"] }],
        }),
      ],
      { view: "story", base: null, mergeBase: null },
    );
    await openEntry(/c-shared/);
    await waitFor(() => expect(bodyText()).toContain("#30"));
    const req = reviewRequests().at(-1)!;
    expect(req.url.searchParams.get("view")).toBe("story");
    expect(req.url.searchParams.get("story")).toBe("#24");
  });

  // @vanda:evidence context-default-base
  // @vanda:evidence context-base-always-shown
  it("今比べているベースブランチを示し選び直すとそのベースブランチで読み直す", async () => {
    open([claimEntry(1, "c-one")]);
    await screen.findAllByText(/c-one/);
    expect(document.body.textContent).toContain("main");
    const user = userEvent.setup();
    const select = screen.getAllByRole("combobox").find((s) => within(s).queryByRole("option", { name: "release" }));
    expect(select, "ベースブランチを選ぶところがない").toBeTruthy();
    await user.selectOptions(select!, "release");
    await waitFor(() => expect(reviewRequests().at(-1)!.url.searchParams.get("base")).toBe("release"));
  });
});

describe("CM-08 ストーリーの置き場所", () => {
  // @vanda:evidence story-shown-on-screen
  it("ストーリーの題と本文と添付ファイルへのリンクを見せる", async () => {
    open([claimEntry(1, "c-one")]);
    expect(await screen.findByText(/タスクの ID を重ならないようにする/)).toBeTruthy();
    expect(screen.getByText(/本文です/)).toBeTruthy();
    const link = screen.getByRole("link", { name: /画面の案/ }) as HTMLAnchorElement;
    expect(link.href).toBe("https://github.com/user-attachments/assets/1");
  });

  // @vanda:evidence story-error-shown-on-screen
  it("ストーリーの置き場所の誤りは黙って空にせず理由を出す", async () => {
    open([claimEntry(1, "c-one")], {}, http.get("/api/story", () => apiError(502, "Bad credentials")));
    expect(await screen.findByText(/Bad credentials/)).toBeTruthy();
  });

  // @vanda:evidence context-story-store
  it("ストーリーの置き場所が設定にないときは読みに行かず注記で出す", async () => {
    server.use(http.get("/api/context", () => HttpResponse.json({ ...ctx, storyStore: "" })));
    server.use(http.get("/api/review", () => HttpResponse.json(review([claimEntry(1, "c-one")]))));
    render(<App />);
    await screen.findAllByText(/c-one/);
    expect(requests.some((r) => r.url.pathname === "/api/story")).toBe(false);
    expect(document.body.textContent).toMatch(/置き場所/);
  });

  // @vanda:evidence review-error-shown
  it("一覧を読めないときは理由を出す", async () => {
    server.use(http.get("/api/context", () => HttpResponse.json(ctx)));
    server.use(http.get("/api/story", () => HttpResponse.json(story)));
    server.use(http.get("/api/review", () => apiError(500, "一覧の数と差分の数が合わない")));
    render(<App />);
    expect(await screen.findByText(/一覧の数と差分の数が合わない/)).toBeTruthy();
  });
});

describe("CM-08 ストーリーの本文の Markdown", () => {
  const body = [
    "## 見出し",
    "",
    "- 一つ目",
    "- 二つ目",
    "",
    "| 列 | 値 |",
    "|---|---|",
    "| a | 1 |",
    "",
    "```",
    "exec vanda check",
    "```",
    "",
    "[資料](https://example.com/doc)",
    "",
    "[危ない](javascript:alert(1))",
    "",
    '<script>window.__pwned = true</script>',
    "",
    '<img src="x" onerror="window.__pwned = true">',
  ].join("\n");

  // @vanda:evidence story-body-markdown
  it("本文の見出しと箇条書きと表とコードとリンクを描き生のHTMLは要素にしない", async () => {
    open([claimEntry(1, "c-one")], {}, http.get("/api/story", () => HttpResponse.json({ ...story, body })));
    await screen.findByText(/タスクの ID を重ならないようにする/);
    const root = document.querySelector(".story-body") as HTMLElement;
    expect(root, "本文を描く .story-body がない").toBeTruthy();
    expect(within(root).getByRole("heading", { name: "見出し" })).toBeTruthy();
    expect(within(root).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["一つ目", "二つ目"]);
    expect(within(root).getByRole("table")).toBeTruthy();
    expect(root.querySelector("pre code")?.textContent).toContain("exec vanda check");
    const link = within(root).getByRole("link", { name: "資料" }) as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("https://example.com/doc");
    // 生の HTML は要素にせず、文字として出す
    expect(root.querySelector("script")).toBeNull();
    expect(root.querySelector("img[onerror]")).toBeNull();
    expect(root.textContent).toContain("<script>");
    expect((window as unknown as { __pwned?: boolean }).__pwned).toBeUndefined();
    // javascript: のリンクは href を空にする
    const danger = within(root).getByText("危ない").closest("a");
    expect(danger?.getAttribute("href") ?? "").toBe("");
  });

  // @vanda:evidence story-links-new-tab
  it("本文のリンクと置き場所で開くリンクと添付のリンクは新しいタブで開き開いた先から戻れないようにする", async () => {
    open([claimEntry(1, "c-one")], {}, http.get("/api/story", () => HttpResponse.json({ ...story, body: "[資料](https://example.com/doc)" })));
    await screen.findByText(/タスクの ID を重ならないようにする/);
    const links = [
      screen.getByRole("link", { name: "資料" }),
      screen.getByRole("link", { name: /置き場所で開く/ }),
      screen.getByRole("link", { name: /画面の案/ }),
    ];
    for (const a of links) {
      expect(a.getAttribute("target"), a.textContent ?? "").toBe("_blank");
      expect((a.getAttribute("rel") ?? "").split(/\s+/).sort()).toEqual(["noopener", "noreferrer"]);
    }
  });
});

describe("RV-05 変更前と変更後の差分", () => {
  // @vanda:evidence review-file-diff-lines
  it("ファイルの差分は消した所と足した所を変わらない所と見分けられるように見せる", async () => {
    open([
      fileEntry(1, "e2e/w.txtar", {
        before: { text: "exec vanda add 買い物を登録した\nstdout ok\n" },
        after: { text: "exec vanda add 買い物を足した\nstdout ok\n" },
        diff: {
          lines: [
            { op: "delete", before: 1, after: null, text: "exec vanda add 買い物を登録した", parts: [{ op: "equal", text: "exec vanda add 買い物を" }, { op: "delete", text: "登録" }, { op: "equal", text: "した" }] },
            { op: "insert", before: null, after: 1, text: "exec vanda add 買い物を足した", parts: [{ op: "equal", text: "exec vanda add 買い物を" }, { op: "insert", text: "足" }, { op: "equal", text: "した" }] },
            { op: "equal", before: 2, after: 2, text: "stdout ok", parts: null },
          ],
        },
      } as Partial<Entry>),
    ]);
    await openEntry(/e2e\/w\.txtar/);
    await waitFor(() => expect(bodyText()).toContain("stdout ok"));
    const exact = (t: string) => Array.from(document.querySelectorAll<HTMLElement>("*")).filter((el) => el.textContent === t);
    const del = exact("登録");
    const ins = exact("足");
    expect(del.length, "消した文字だけの要素がない").toBeGreaterThan(0);
    expect(ins.length, "足した文字だけの要素がない").toBeGreaterThan(0);
    const look = (el: HTMLElement) => el.tagName + " " + el.className;
    expect(look(del[0])).not.toBe(look(ins[0]));
  });

  // @vanda:evidence review-before-after
  it("エビデンスの予定はエビデンスごとに足したものと消したものに札を付け変わったものは中の差分を見せる", async () => {
    const ev = (stage: string, type: string, detail: string | null) => ({ stage, type, layers: ["model"], detail });
    const detail = [
      { op: "equal", text: "TLA+ の" },
      { op: "delete", text: "古い計画" },
      { op: "insert", text: "新しい計画" },
      { op: "equal", text: "で確かめる" },
    ];
    const layers = [{ op: "equal", text: "model" }];
    open([
      claimEntry(1, "c-ev", {
        before: { ...claimEntry(1, "c-ev").before!, evidence: [ev("test", "scenario-test", null), ev("plan", "model-check", "TLA+ の古い計画で確かめる"), ev("release", "human-review", null)] },
        after: { ...claimEntry(1, "c-ev").after!, evidence: [ev("test", "scenario-test", null), ev("plan", "model-check", "TLA+ の新しい計画で確かめる"), ev("operation", "monitoring", null)] },
        diff: {
          fields: { statement: [{ op: "delete", text: "c-ev の前の文" }, { op: "insert", text: "c-ev の後の文" }] },
          evidence: {
            before: [{ op: "equal", layers: null, detail: null }, { op: "modified", layers, detail }, { op: "removed", layers: null, detail: null }],
            after: [{ op: "equal", layers: null, detail: null }, { op: "modified", layers, detail }, { op: "added", layers: null, detail: null }],
          },
        },
      } as Partial<Entry>),
    ]);
    await openEntry(/c-ev/);
    await waitFor(() => expect(bodyText()).toContain("新しい計画"));
    // 変更前と変更後の表の「エビデンスの予定」の行の中で見る
    const row = screen.getAllByRole("row").find((r) => (r.textContent ?? "").startsWith("エビデンスの予定"));
    expect(row, "変更前と変更後の表に、エビデンスの予定の行がない").toBeTruthy();
    const added = /足した|追加/;
    const removed = /消した|削除/;
    // エビデンス 1 つずつの項目の文字。タイプの名前を含むいちばん内側の要素から、項目（li）か、その親までを見る
    const itemText = (label: string) => {
      const hits = Array.from(row!.querySelectorAll<HTMLElement>("*")).filter((el) => (el.textContent ?? "").includes(label));
      const inner = hits[hits.length - 1];
      expect(inner, `エビデンスの予定に ${label} がない`).toBeTruthy();
      return (inner.closest("li") ?? inner.parentElement ?? inner).textContent ?? "";
    };
    expect(itemText("人のレビュー"), "消したエビデンスに札がない").toMatch(removed);
    expect(itemText("監視"), "足したエビデンスに札がない").toMatch(added);
    expect(itemText("シナリオテスト"), "変わらないエビデンスに足した・消したの札がある").not.toMatch(/足した|追加|消した|削除/);
    // 変わったエビデンスは、詳細の中の消した語と足した語を見分けられるように見せる
    const exact = (t: string) => Array.from(row!.querySelectorAll<HTMLElement>("*")).filter((el) => el.textContent === t);
    const del = exact("古い計画");
    const ins = exact("新しい計画");
    expect(del.length, "変わったエビデンスの詳細に、消した語がない").toBeGreaterThan(0);
    expect(ins.length, "変わったエビデンスの詳細に、足した語がない").toBeGreaterThan(0);
    expect(del[0].tagName + del[0].className).not.toBe(ins[0].tagName + ins[0].className);
  });

  // @vanda:evidence review-before-after
  it("約束の項目の差分は1列で消した語をdelに足した語をinsに出し変わっていない項目は中身を1回だけ出す", async () => {
    open([
      claimEntry(1, "c-diff", {
        before: { ...claimEntry(1, "c-diff").before!, statement: "タスクの ID はたぶん重ならない", rationale: "消す相手を決めるため" },
        after: { ...claimEntry(1, "c-diff").after!, statement: "タスクの ID は絶対に重ならない", rationale: "消す相手を決めるため" },
        diff: {
          fields: {
            statement: [
              { op: "equal", text: "タスクの ID は" },
              { op: "delete", text: "たぶん" },
              { op: "insert", text: "絶対に" },
              { op: "equal", text: "重ならない" },
            ],
            rationale: [{ op: "equal", text: "消す相手を決めるため" }],
          },
        },
      } as Partial<Entry>),
    ]);
    await openEntry(/c-diff/);
    await waitFor(() => expect(bodyText()).toContain("絶対に"));
    const exact = (t: string) => Array.from(document.querySelectorAll<HTMLElement>("*")).filter((el) => el.textContent === t);
    expect(exact("たぶん").map((el) => el.tagName), "消した語が del で出ない").toContain("DEL");
    expect(exact("絶対に").map((el) => el.tagName), "足した語が ins で出ない").toContain("INS");
    // 変わっていない語は del・ins の外
    for (const el of document.querySelectorAll("del, ins")) expect(el.textContent).not.toMatch(/タスクの ID は|重ならない/);
    // 変わっていない項目は、中身を 1 回だけ出す（変更前と変更後の 2 列に分けない）
    expect(bodyText().split("消す相手を決めるため").length - 1, "変わっていない理由が 1 回でない").toBe(1);
  });
});

describe("RV-16・RV-17 1 件の詳細の由来と合意の履歴・取り消し", () => {
  // @vanda:evidence detail-origin-and-history
  it("約束の1件の詳細に由来のコミットと合意の履歴を見せる", async () => {
    open([claimEntry(1, "task-id-unique")], {}, http.get("/api/claims/:id", () =>
      HttpResponse.json(claimDetail({
        origin: [{ commit: "1".repeat(40), date: "2026-10-01T12:00:00+09:00", subject: "#1 はじめの約束を置く", stories: ["#1"], change: "added" }],
        agreements: [{ user: { name: "前に合意した人", email: "a@example.com" }, agreedAt: "2026-10-02T12:00:00+09:00", story: "#1", fingerprint: "sha256:" + "b".repeat(64), current: false }],
      })),
    ));
    await openEntry(/task-id-unique/);
    await waitFor(() => expect(bodyText()).toContain("#1 はじめの約束を置く"));
    expect(bodyText()).toContain("前に合意した人");
  });

  // @vanda:evidence revoke-on-screen
  it("合意済の件で取り消すと今の中身の指紋とストーリーとベースブランチで取り消しを求め一覧を読み直す", async () => {
    const e = claimEntry(1, "task-id-unique", { state: "agreed", revoke: { possible: true, reason: null, others: [] } });
    let posted: unknown = null;
    open([e], {}, http.post("/api/revoke", async ({ request }) => {
      posted = await request.json();
      return HttpResponse.json({ commit: "c".repeat(40), stillAgreed: false, removed: 1 });
    }));
    await openEntry(/task-id-unique/);
    const button = await screen.findByRole("button", { name: /取り消/ });
    const before = reviewRequests().length;
    await userEvent.setup().click(button);
    await waitFor(() => expect(posted).toMatchObject({ kind: "claim", id: "task-id-unique", fingerprint: e.fingerprint, story: "#24", base: "main" }));
    await waitFor(() => expect(reviewRequests().length).toBeGreaterThan(before));
  });

  // @vanda:evidence revoke-on-screen
  it("取り消せない件には取り消しのボタンを出さない", async () => {
    open([claimEntry(1, "task-id-unique", { state: "agreed", revoke: { possible: false, reason: "ほかの人が書いた合意です", others: [] } })]);
    await openEntry(/task-id-unique/);
    await screen.findAllByText(/合意済/);
    expect(screen.queryByRole("button", { name: /取り消/ })).toBeNull();
  });
});

describe("RV-06 合意した後に読み直す", () => {
  // @vanda:evidence agree-on-screen-reloads
  it("画面で合意した後に一覧を読み直し合意した後の状態を見せる", async () => {
    let agreed = false;
    server.use(...defaultHandlers(review([])));
    server.use(
      http.get("/api/review", () => HttpResponse.json(review([claimEntry(1, "task-id-unique", { state: agreed ? "agreed" : "proposed" })]))),
      http.post("/api/agree", () => {
        agreed = true;
        return HttpResponse.json({ commit: "c".repeat(40), agreement: {} });
      }),
    );
    render(<App />);
    const user = userEvent.setup();
    const [item] = await screen.findAllByText(/task-id-unique/);
    const li = () => screen.getAllByText(/task-id-unique/).map((el) => el.closest("li")).find((x) => x)!;
    expect(li().textContent).toContain("提案中");
    if (!screen.queryAllByRole("button", { name: /合意する/ })[0]) await user.click(item);
    await user.click(await screen.findByRole("button", { name: /合意する/ }));
    await waitFor(() => expect(li().textContent, "合意した後の一覧が合意済にならない").toContain("合意済"));
    expect(li().textContent).not.toContain("提案中");
  });
});

describe("RV-13 ストーリーの示し方", () => {
  const storySelect = () =>
    screen.queryAllByRole("combobox").find((s) => Array.from((s as HTMLSelectElement).options).some((o) => o.value.startsWith("#"))) as HTMLSelectElement | undefined;
  const optionIds = (s: HTMLSelectElement) => Array.from(s.options).map((o) => o.value).filter((v) => v !== "");

  // @vanda:evidence review-story-choice
  it("ブランチのコミットからストーリーが1つに決まれば選ばせずに示し2つ以上ならブランチのストーリーから見つからなければすべてのストーリーから選ばせる", async () => {
    const cases = [
      { name: "1 つに決まる", over: { storyChoice: "fixed", story: "#24", branchStories: ["#24"] }, url: "/?view=branch&base=main", want: null },
      { name: "2 つ以上", over: { storyChoice: "branch", story: null, branchStories: ["#30", "#31"] }, url: "/?view=branch&base=main", want: ["#30", "#31"] },
      { name: "見つからない", over: { storyChoice: "all", story: null, branchStories: [] }, url: "/?view=branch&base=main", want: ctx.stories.map((s) => s.id) },
    ] as const;
    for (const c of cases) {
      window.history.pushState({}, "", c.url);
      open([claimEntry(1, "c-one")], c.over);
      await screen.findAllByText(/c-one/);
      const sel = storySelect();
      if (c.want === null) {
        expect(sel, `${c.name}: 選ぶ欄が出ている`).toBeUndefined();
        expect(bodyText(), `${c.name}: 決まったストーリーを示さない`).toContain("#24");
      } else {
        expect(sel, `${c.name}: 選ぶ欄がない`).toBeTruthy();
        expect(optionIds(sel!).sort(), `${c.name}: 選べるストーリー`).toEqual([...c.want].sort());
      }
      cleanup();
      server.resetHandlers();
    }
  });
});

describe("RV-02 一覧の 1 件を選ぶ", () => {
  // @vanda:evidence entry-select-anywhere
  it("一覧の1件は名前のほか番号と状態の札とフラグの札と枠の端を押しても選ばれる", async () => {
    open([claimEntry(1, "c-one"), claimEntry(2, "c-two", { flags: ["ai-proposal"] })]);
    const user = userEvent.setup();
    const nameButton = async (id: string) => (await screen.findAllByText(id)).map((el) => el.closest("button")).find((b) => b?.closest("li"))!;
    const frame = async () => (await nameButton("c-two")).closest("li")! as HTMLElement;
    const spots: [string, () => Promise<HTMLElement>][] = [
      ["番号", async () => within(await frame()).getByText("2", { exact: true })],
      ["状態の札", async () => within(await frame()).getByText(/提案中/)],
      ["フラグの札", async () => within(await frame()).getAllByText(/^AI\s?案/, { selector: "*:not([role=tooltip])" })[0]],
      ["枠の端", frame],
    ];
    for (const [spot, target] of spots) {
      await user.click(await nameButton("c-one"));
      await waitFor(() => expect((async () => (await nameButton("c-one")).getAttribute("aria-current"))()).resolves.toBe("true"));
      await user.click(await target());
      await waitFor(async () => expect((await nameButton("c-two")).getAttribute("aria-current"), `${spot}を押しても選ばれない`).toBe("true"));
    }
  });
});

describe("RV-18 詳細の画面から戻る", () => {
  // レビュー単位がストーリーのときはベースブランチを選ぶ所が出ないので、組は 3 つ。戻る経路（約束の詳細・ファイルの画面）と全部回す
  const states = [
    { name: "何も付けない URL", url: "/", over: {} },
    { name: "ブランチでベースブランチがmain", url: "/?view=branch&base=main&story=%2324", over: {} },
    { name: "ブランチでベースブランチがrelease", url: "/?view=branch&base=release&story=%2324", over: { base: "release" } },
    { name: "ストーリー", url: "/?view=story&story=%2324", over: { view: "story", base: null, mergeBase: null } },
  ];
  const routes = [
    { name: "約束の詳細", entry: "task-id-unique", link: "約束の詳細を開く" },
    { name: "ファイルの画面", entry: "e2e/add-task.txtar", link: "ファイルの画面を開く" },
  ];
  // @vanda:evidence back-restores-state
  it("約束の詳細とファイルの画面からストーリーのレビューへ戻ると行く前と同じベースブランチとレビュー単位とストーリーと選んだ件に戻る", async () => {
    for (const st of states) {
      for (const rt of routes) {
        const where = `${st.name}・${rt.name}`;
        window.history.pushState({}, "", st.url);
        requests.length = 0;
        open([claimEntry(1, "task-id-unique"), fileEntry(2, "e2e/add-task.txtar"), claimEntry(3, "c-other")], st.over,
          http.get("/api/claims/:id", () => HttpResponse.json(claimDetail())),
          http.get("/api/file", () => HttpResponse.json({ path: "e2e/add-task.txtar", text: "exec vanda add 買い物\n" })));
        const user = userEvent.setup();
        const nameButton = async (id: string) => (await screen.findAllByText(id)).map((el) => el.closest("button")).find((b) => b?.closest("li"))!;
        await user.click(await nameButton(rt.entry));
        await waitFor(async () => expect((await nameButton(rt.entry)).getAttribute("aria-current")).toBe("true"));
        const beforeURL = window.location.pathname + window.location.search;
        const reviewBefore = reviewRequests().at(-1)!.url.searchParams;
        await user.click(await screen.findByRole("link", { name: rt.link }));
        await user.click(await screen.findByRole("link", { name: "ストーリーのレビューへ戻る" }));
        await waitFor(async () => expect((await nameButton(rt.entry)).getAttribute("aria-current"), `${where}: 選んだ件に戻らない`).toBe("true"));
        expect(window.location.pathname).toBe("/");
        // 戻った後の URL は、既定の値を書き足したりせず、行く前とちょうど同じ
        expect(window.location.pathname + window.location.search, `${where}: 戻った後の URL`).toBe(beforeURL);
        for (const k of ["view", "base", "story"]) {
          expect(reviewRequests().at(-1)!.url.searchParams.get(k), `${where}: 読み直した一覧の ${k}`).toBe(reviewBefore.get(k));
        }
        cleanup();
        server.resetHandlers();
      }
    }
  });
});

describe("Story: のトレーラーの誤り", () => {
  const errors = [
    { commit: "1111111aaaaaaa1111111aaaaaaa1111111aaaaa", subject: "トレーラーが書き方に合わない", message: "Story: のトレーラーの値 PROJ-1 が、ストーリーの ID の書き方（#[0-9]+）に合いません。" },
    { commit: "2222222bbbbbbb2222222bbbbbbb2222222bbbbb", subject: "トレーラーが 2 つ", message: "Story: のトレーラーが 2 つあります（#77・#78）。1 つのコミットに書けるのは 1 つだけです。" },
  ];
  // @vanda:evidence story-trailer-errors-shown
  it("文脈にトレーラーの誤りがあると画面の上に知らせを出しコミットの短いハッシュとsubjectと理由を並べなければ出さない", async () => {
    for (const trailerErrors of [errors, []]) {
      open([claimEntry(1, "c-one")], {}, http.get("/api/context", () => HttpResponse.json({ ...ctx, trailerErrors })));
      await screen.findAllByText(/c-one/);
      const alert = screen.queryByRole("alert", { name: "Story: のトレーラーの誤り" });
      if (trailerErrors.length === 0) {
        expect(alert, "誤りがないのに知らせが出た").toBeNull();
      } else {
        expect(alert, "誤りがあるのに知らせが出ない").not.toBeNull();
        const items = within(alert!).getAllByRole("listitem");
        expect(items).toHaveLength(errors.length);
        errors.forEach((e, i) => {
          const text = items[i].textContent ?? "";
          for (const part of [e.commit.slice(0, 7), e.subject, e.message]) expect(text, `${i + 1} 件目に ${part} がない`).toContain(part);
          expect(text, `${i + 1} 件目に長いハッシュが出た`).not.toContain(e.commit.slice(0, 8));
        });
      }
      cleanup();
      server.resetHandlers();
    }
  });
});
