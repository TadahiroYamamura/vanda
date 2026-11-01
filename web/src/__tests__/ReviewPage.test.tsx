// RV（ストーリーのレビュー）の画面が、API の結果を正しく見せるか。API は MSW で差し替える。
// 画面に出す言葉は docs/spec.md と docs/terms.md の言葉（AI 案・推測・差分あり・提案中・合意済・合意する など）で探す。
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { App } from "../App";
import type { Entry } from "../api";
import { apiError, claimEntry, ctx, defaultHandlers, fileEntry, requests, review, server, story } from "./fixtures";

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

  it("合意の状態を提案中と合意済で示す", async () => {
    open([claimEntry(1, "c-proposed"), claimEntry(2, "c-agreed", { state: "agreed" })]);
    await screen.findAllByText(/c-agreed/);
    expect(screen.getAllByText(/提案中/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/合意済/).length).toBeGreaterThan(0);
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
  it("ストーリーの題と本文と添付ファイルへのリンクを見せる", async () => {
    open([claimEntry(1, "c-one")]);
    expect(await screen.findByText(/タスクの ID を重ならないようにする/)).toBeTruthy();
    expect(screen.getByText(/本文です/)).toBeTruthy();
    const link = screen.getByRole("link", { name: /画面の案/ }) as HTMLAnchorElement;
    expect(link.href).toBe("https://github.com/user-attachments/assets/1");
  });

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
