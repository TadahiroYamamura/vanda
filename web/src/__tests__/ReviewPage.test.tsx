// RV（ストーリーのレビュー）の画面が、API の結果を正しく見せるか。API は MSW で差し替える。
// 画面に出す言葉は、約束リスト（.vanda/promises.toml）の用語（AI 案・推測・差分あり・提案中・合意済・合意 など）で探す。
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { App } from "../App";
import type { Entry } from "../api";
import { apiError, claimDetail, claimEntry, ctx, defaultHandlers, fileEntry, plain, requests, review, server, story } from "./fixtures";

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

const listRegion = () => screen.getByRole("region", { name: "変わったものの一覧" });
const detailRegion = () => screen.getByRole("region", { name: "1 件の中身" });
// listRow は、一覧で名前が name の 1 件（<li>）
const listRow = async (name: string) => (await within(await screen.findByRole("region", { name: "変わったものの一覧" })).findByText(name, { exact: true })).closest("li")! as HTMLElement;

const reviewRequests = () => requests.filter((r) => r.url.pathname === "/api/review");

describe("RV-04 フラグ", () => {
  // @vanda:evidence review-flag-labels
  // 一覧ではフラグをアイコンで出す（review-list-icons）ので、札は 1 件の詳細で確かめ、一覧のアイコンにも同じ説明が出ることを確かめる
  it("フラグは詳細では短い言葉の札で出し説明は札に触れたときかフォーカスしたときに出し一覧のアイコンにも同じ説明を出す", async () => {
    open([claimEntry(1, "task-id-unique", { flags: ["ai-proposal", "guess", "changed-since-agreement"] })]);
    const user = userEvent.setup();
    await openEntry(/task-id-unique/);
    // tipOf は、触れたときとフォーカスしたときにだけ説明が出ることを確かめ、説明の文を返す
    const tipOf = async (el: HTMLElement, where: string) => {
      expect(within(el).queryByRole("tooltip"), `${where}: 触れる前から説明が出ている`).toBeNull();
      await user.hover(el);
      const text = (await within(el).findByRole("tooltip")).textContent ?? "";
      await user.unhover(el);
      await waitFor(() => expect(within(el).queryByRole("tooltip"), `${where}: 離れても説明が消えない`).toBeNull());
      el.focus();
      expect(await within(el).findByRole("tooltip"), `${where}: フォーカスしても説明が出ない`).toBeTruthy();
      el.blur();
      await waitFor(() => expect(within(el).queryByRole("tooltip")).toBeNull());
      return text;
    };
    for (const [label, word] of [[/^AI\s?案/, "AI 案"], [/^推測/, "推測"], [/^差分あり/, "差分あり"]] as const) {
      const [badge] = await within(detailRegion()).findAllByText(label, { selector: "*:not([role=tooltip])" });
      const help = await tipOf(badge, `詳細の札 ${word}`);
      expect(help.length, `詳細の札 ${word} の説明が空`).toBeGreaterThan(0);
      const icon = within(await listRow("task-id-unique")).getByRole("img", { name: word });
      expect(await tipOf(icon, `一覧のアイコン ${word}`), `一覧のアイコン ${word} に詳細の札と同じ説明が出ない`).toContain(help);
      expect(within(await listRow("task-id-unique")).getByRole("img", { name: word, description: help }), `一覧のアイコン ${word} の説明が読み上げに渡らない`).toBe(icon);
    }
  });

  // @vanda:evidence review-flag-labels
  it("フラグのないものには札を出さない", async () => {
    open([claimEntry(1, "plain-claim")]);
    await screen.findAllByText(/plain-claim/);
    for (const label of [/AI\s?案/, /推測/, /差分あり/]) {
      expect(screen.queryByText(label)).toBeNull();
      expect(screen.queryByRole("img", { name: label }), `${label} のアイコンが出ている`).toBeNull();
    }
  });
});

describe("RV-01・RV-02・RV-05 一覧と中身", () => {
  // @vanda:evidence review-numbering
  it("通し番号と総数を出し追加と変更と削除を区別して見せる", async () => {
    open([
      claimEntry(1, "c-added", { change: "added", before: null }),
      claimEntry(2, "c-modified"),
      claimEntry(3, "c-deleted", { change: "deleted", after: null, fingerprint: null }),
      fileEntry(4, "e2e/old.txtar", { change: "deleted", after: null, fingerprint: null }),
    ]);
    for (const id of ["c-added", "c-modified", "c-deleted", "e2e/old.txtar"]) await screen.findAllByText(new RegExp(id));
    expect(bodyText()).toMatch(/4/);
    for (const w of [/追加/, /変更/, /削除/]) expect(bodyText()).toMatch(w);
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
  it("一覧の1件ごとに合意の状態を提案中か合意済かで示し削除したものも消したことへの合意で示す", async () => {
    const states = [["proposed", "提案中"], ["agreed", "合意済"]] as const;
    const del = { change: "deleted", after: null, fingerprint: null } as const;
    open(
      states.flatMap(([st], i) => [
        claimEntry(4 * i + 1, `c-${st}`, { state: st }),
        fileEntry(4 * i + 2, `e2e/${st}.txtar`, { state: st }),
        claimEntry(4 * i + 3, `c-del-${st}`, { ...del, state: st }),
        fileEntry(4 * i + 4, `e2e/del-${st}.txtar`, { ...del, state: st }),
      ]),
    );
    for (const [st, label] of states) {
      for (const name of [`c-${st}`, `e2e/${st}.txtar`, `c-del-${st}`, `e2e/del-${st}.txtar`]) {
        const li = await listRow(name);
        expect(li, `${name} の一覧の 1 件がない`).toBeTruthy();
        for (const [, l] of states) expect(within(li).queryAllByRole("img", { name: l }).length, `${name} の「${l}」`).toBe(l === label ? 1 : 0);
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
    await waitFor(() => expect(posted).toEqual({ kind: "claim", id: "task-id-unique", path: null, fingerprint: e.fingerprint, story: "#24", deleted: false }));
    await waitFor(() => expect(reviewRequests().length).toBeGreaterThan(before));
  });

  // @vanda:evidence review-deleted-shown
  it("削除したものは消す前の中身を見せ消したことに合意するとdeletedを添えて送り一覧を読み直す", async () => {
    const e = fileEntry(1, "e2e/old.txtar", { change: "deleted", after: null, fingerprint: null, before: { text: "消す前のシナリオの中身\n" } });
    open([e]);
    let posted: unknown = null;
    server.use(
      http.post("/api/agree", async ({ request }) => {
        posted = await request.json();
        return HttpResponse.json({ commit: "c".repeat(40), agreement: {} });
      }),
    );
    await openEntry(/e2e\/old\.txtar/);
    await waitFor(() => expect(within(detailRegion()).getByText(/消す前のシナリオの中身/)).toBeTruthy());
    const button = await within(detailRegion()).findByRole("button", { name: /消したことに合意する/ });
    const before = reviewRequests().length;
    await userEvent.setup().click(button);
    await waitFor(() => expect(posted).toEqual({ kind: "file", id: null, path: "e2e/old.txtar", fingerprint: null, story: "#24", deleted: true }));
    await waitFor(() => expect(reviewRequests().length).toBeGreaterThan(before));
  });

  // 約束にはなく、docs/formats.md の entries[].agree に書いてある振る舞い（外れた合意を解くために、合意済でも合意し直せる）
  it("合意済でもこのストーリーで合意できる件には合意のボタンを出し合意できない件には出さない", async () => {
    // ほかのストーリーの合意で合意済だが、このストーリーの合意が外れた件（API が合意できると答える）
    const reagree = claimEntry(1, "c-reagree", { state: "agreed", detachedAgreement: { by: ["#31"] }, agree: { possible: true, reason: null } });
    const done = claimEntry(2, "c-done", { state: "agreed", agree: { possible: false, reason: "このストーリーで今の中身にもう合意しています" } });
    open([reagree, done]);
    await openEntry(/c-reagree/);
    await waitFor(() => expect(within(detailRegion()).getAllByRole("button", { name: /合意する/ }).length).toBe(1));
    expect((within(detailRegion()).getByRole("button", { name: /合意する/ }) as HTMLButtonElement).disabled).toBe(false);
    await openEntry(/c-done/);
    await waitFor(() => expect(within(detailRegion()).getByText(/c-done/, { selector: "h2" })).toBeTruthy());
    expect(within(detailRegion()).queryAllByRole("button", { name: /合意する/ }).length).toBe(0);
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
    // 合意できるかは API（entries[].agree）が決める。git の人が分からないときの API の答えを、手で作って渡す
    const e = claimEntry(1, "task-id-unique", { agree: { possible: false, reason: "git の user.name と user.email が分かりません" } });
    server.use(http.get("/api/review", () => HttpResponse.json(review([e]))));
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
    for (const name of ["c-changed", "c-detached"]) {
      expect(within(await listRow(name)).queryAllByRole("img", { name: "合意が外れた" }).length, `${name} に合意が外れたと出ない`).toBe(1);
    }
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

describe("CM-08 ストーリーストア", () => {
  // @vanda:evidence story-shown-on-screen
  it("ストーリーの題と本文と添付ファイルへのリンクを見せる", async () => {
    open([claimEntry(1, "c-one")]);
    expect(await screen.findByText(/タスクの ID を重ならないようにする/)).toBeTruthy();
    expect(screen.getByText(/本文です/)).toBeTruthy();
    const link = screen.getByRole("link", { name: /画面の案/ }) as HTMLAnchorElement;
    expect(link.href).toBe("https://github.com/user-attachments/assets/1");
  });

  // @vanda:evidence story-error-shown-on-screen
  it("ストーリーストアの誤りは黙って空にせず理由を出す", async () => {
    open([claimEntry(1, "c-one")], {}, http.get("/api/story", () => apiError(502, "Bad credentials")));
    expect(await screen.findByText(/Bad credentials/)).toBeTruthy();
  });

  // @vanda:evidence context-story-store
  it("ストーリーストアが設定にないときは読みに行かず注記で出す", async () => {
    server.use(http.get("/api/context", () => HttpResponse.json({ ...ctx, storyStore: "" })));
    server.use(http.get("/api/review", () => HttpResponse.json(review([claimEntry(1, "c-one")]))));
    render(<App />);
    await screen.findAllByText(/c-one/);
    expect(requests.some((r) => r.url.pathname === "/api/story")).toBe(false);
    expect(document.body.textContent).toContain("題と本文なし（ストーリーストアが未設定）");
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
  it("本文のリンクとストーリーストアで開くリンクと添付のリンクは新しいタブで開き開いた先から戻れないようにする", async () => {
    open([claimEntry(1, "c-one")], {}, http.get("/api/story", () => HttpResponse.json({ ...story, body: "[資料](https://example.com/doc)" })));
    await screen.findByText(/タスクの ID を重ならないようにする/);
    const links = [
      screen.getByRole("link", { name: "資料" }),
      screen.getByRole("link", { name: /ストーリーストアで開く/ }),
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
    const ev = (stage: string, type: string, detail: string | null) => ({ stage, type, layers: ["model"], detail: detail === null ? null : plain(detail) });
    const detail = [
      { op: "equal", text: "TLA+ の" },
      { op: "delete", text: "古い計画" },
      { op: "insert", text: "新しい計画" },
      { op: "equal", text: "で確かめる" },
    ];
    const layers = [{ op: "equal", text: "model" }];
    open([
      claimEntry(1, "c-ev", {
        before: { ...claimEntry(1, "c-ev").before!, evidence: [ev("implementation", "scenario-test", null), ev("plan", "model-check", "TLA+ の古い計画で確かめる"), ev("release", "human-review", null)] },
        after: { ...claimEntry(1, "c-ev").after!, evidence: [ev("implementation", "scenario-test", null), ev("plan", "model-check", "TLA+ の新しい計画で確かめる"), ev("operation", "sensor", null)] },
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
    expect(itemText("センサー"), "足したエビデンスに札がない").toMatch(added);
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
        before: { ...claimEntry(1, "c-diff").before!, statement: plain("タスクの ID はたぶん重ならない"), rationale: plain("消す相手を決めるため") },
        after: { ...claimEntry(1, "c-diff").after!, statement: plain("タスクの ID は絶対に重ならない"), rationale: plain("消す相手を決めるため") },
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
  // @vanda:evidence revoke-reason-shown
  it("取り消せない件には取り消しのボタンを出さず取り消せない理由を示す", async () => {
    // 取り消せるかと理由は API（entries[].revoke）が決める。その答えを手で作って渡す
    for (const reason of ["ほかの人が書いた合意です", "ベースブランチにすでにある合意です", "このストーリーの合意がありません"]) {
      open([claimEntry(1, "task-id-unique", { state: "agreed", revoke: { possible: false, reason, others: [] } })]);
      await openEntry(/task-id-unique/);
      await screen.findAllByText(/合意済/);
      expect(screen.queryByRole("button", { name: /取り消/ }), `${reason}: 取り消しのボタンがある`).toBeNull();
      expect(within(detailRegion()).getByText(reason), `${reason}: 理由がない`).toBeTruthy();
      cleanup();
      server.resetHandlers();
    }
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
    const stateIcons = (name: string) => within(li()).queryAllByRole("img", { name }).length;
    expect(stateIcons("提案中")).toBe(1);
    if (!screen.queryAllByRole("button", { name: /合意する/ })[0]) await user.click(item);
    await user.click(await screen.findByRole("button", { name: /合意する/ }));
    await waitFor(() => expect(stateIcons("合意済"), "合意した後の一覧が合意済にならない").toBe(1));
    expect(stateIcons("提案中")).toBe(0);
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
  it("一覧の1件は名前のほか番号と種類と状態のアイコンとフラグのアイコンと枠の端を押しても選ばれる", async () => {
    open([claimEntry(1, "c-one"), claimEntry(2, "c-two", { flags: ["ai-proposal"] })]);
    const user = userEvent.setup();
    const nameButton = async (id: string) => (await screen.findAllByText(id)).map((el) => el.closest("button")).find((b) => b?.closest("li"))!;
    const frame = async () => (await nameButton("c-two")).closest("li")! as HTMLElement;
    const spots: [string, () => Promise<HTMLElement>][] = [
      ["番号", async () => within(await frame()).getByText("2", { exact: true })],
      ["種類", async () => within(await frame()).getByText("約束", { exact: true })],
      ["状態のアイコン", async () => within(await frame()).getByRole("img", { name: "提案中" })],
      ["フラグのアイコン", async () => within(await frame()).getByRole("img", { name: "AI 案" })],
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

describe("RV-01・RV-02・RV-04 一覧の 1 件の出し方", () => {
  const term = (no: number, id: string) =>
    claimEntry(no, id, { kind: "term", group: "terms-features", before: { name: id, definition: plain("定義"), partOf: null }, after: { name: id, definition: plain("定義"), partOf: null } });
  const feature = (no: number, id: string) =>
    claimEntry(no, id, { kind: "feature", group: "terms-features", before: { name: id, definition: plain("定義"), core: null, formal: null, formalWaiver: null }, after: { name: id, definition: plain("定義"), core: null, formal: null, formalWaiver: null } });
  // 1 行目は名前の所のボタン、2 行目はその後ろ
  const firstLine = (li: HTMLElement) => within(li).getByRole("button");
  const secondLine = (li: HTMLElement) => firstLine(li).nextElementSibling as HTMLElement;

  // @vanda:evidence review-list-first-line
  it("一覧の1件の1行目には通し番号と名前だけを出し追加と変更と削除の記号と言葉は出さず種類は2行目の左端に出し追加か変更か削除かは詳細に出す", async () => {
    const cases = [
      { e: claimEntry(1, "c-added", { change: "added", before: null }), name: "c-added", kind: "約束", change: "追加" },
      { e: claimEntry(2, "c-modified"), name: "c-modified", kind: "約束", change: "変更" },
      { e: claimEntry(3, "c-deleted", { change: "deleted", after: null, fingerprint: null }), name: "c-deleted", kind: "約束", change: "削除" },
      { e: term(4, "t-term"), name: "t-term", kind: "用語", change: null },
      { e: feature(5, "f-feature"), name: "f-feature", kind: "機能", change: null },
      { e: fileEntry(6, "e2e/old.txtar", { change: "deleted", after: null, fingerprint: null }), name: "e2e/old.txtar", kind: "シナリオ", change: "削除" },
      { e: fileEntry(7, "spec/Task.tla", { fileType: "oracle", change: "added", before: null }), name: "spec/Task.tla", kind: "オラクルの定義", change: null },
      { e: fileEntry(8, "spec/guard.cue", { fileType: "guardrail" }), name: "spec/guard.cue", kind: "ガードレールの定義", change: null },
      { e: fileEntry(9, "docs/other.md", { fileType: null }), name: "docs/other.md", kind: "ファイル", change: null },
    ];
    open(cases.map((c) => c.e));
    for (const c of cases) {
      const li = await listRow(c.name);
      const line1 = Array.from(firstLine(li).children).map((el) => el.textContent);
      expect(line1, `${c.name} の 1 行目`).toEqual([String(c.e.no), c.name]);
      expect(secondLine(li).firstElementChild?.textContent, `${c.name} の 2 行目の左端`).toBe(c.kind);
      for (const w of ["+", "~", "−", "→", "=", "追加", "変更", "削除"]) expect(li.textContent ?? "", `${c.name} の一覧に「${w}」が出ている`).not.toContain(w);
    }
    const user = userEvent.setup();
    for (const c of cases.filter((c) => c.change !== null)) {
      await user.click(firstLine(await listRow(c.name)));
      await waitFor(() =>
        expect(
          within(detailRegion()).queryAllByText((_, el) => el?.tagName === "P" && (el.textContent ?? "").split(" · ").includes(c.change!)).length,
          `${c.name} の詳細に「${c.change}」と出ない`,
        ).toBe(1),
      );
    }
  });

  // @vanda:evidence review-list-icons
  it("一覧の1件の2行目には種類の後ろに状態のアイコンを1つと立っているフラグのアイコンを並べ合意が外れたものは提案中でなく合意が外れたを出し状態は丸の形でフラグは丸のない形にする", async () => {
    const all = ["ai-proposal", "guess", "changed-since-agreement"] as const;
    const cases = [
      { e: claimEntry(1, "c-proposed"), icons: ["提案中"] },
      { e: claimEntry(2, "c-agreed-flags", { state: "agreed", flags: [...all] }), icons: ["合意済", "AI 案", "推測", "差分あり"] },
      { e: claimEntry(3, "c-guess-ai", { flags: ["guess", "ai-proposal"] }), icons: ["提案中", "推測", "AI 案"] },
      { e: claimEntry(4, "c-detached-proposed", { flags: ["changed-since-agreement"], detachedAgreement: { by: ["#31"] } }), icons: ["合意が外れた", "差分あり"] },
      { e: claimEntry(5, "c-detached-agreed", { state: "agreed", detachedAgreement: { by: ["#31"] } }), icons: ["合意が外れた"] },
      { e: fileEntry(6, "e2e/agreed.txtar", { state: "agreed" }), icons: ["合意済"] },
    ];
    const unchanged = claimEntry(0, "c-detached-unchanged", { no: null, change: "unchanged", before: null, flags: ["ai-proposal"], detachedAgreement: { by: ["#30"] } });
    open(
      cases.map((c) => c.e),
      { detached: [unchanged] },
    );
    const stateWords = ["合意済", "提案中", "合意が外れた"];
    // 丸の形は、外周の円（半径が 16px の枠の 6 以上）で見る。推測の点のような小さい円は丸の形に数えない
    const hasRing = (icon: HTMLElement) => Array.from(icon.querySelectorAll("svg circle")).some((c) => Number(c.getAttribute("r")) >= 6);
    const shapes = new Map<string, string>();
    for (const c of [...cases, { e: unchanged, icons: ["合意が外れた", "AI 案"] }]) {
      const name = c.e.kind === "file" ? c.e.path! : c.e.id!;
      const li = await listRow(name);
      expect(within(firstLine(li)).queryAllByRole("img"), `${name} の 1 行目にアイコンがある`).toEqual([]);
      const line2 = secondLine(li);
      const icons = within(line2).queryAllByRole("img");
      expect(icons.length, `${name} のアイコンの数`).toBe(c.icons.length);
      c.icons.forEach((w, i) => expect(within(line2).getByRole("img", { name: w }), `${name} の ${i + 1} 番目のアイコンが「${w}」でない`).toBe(icons[i]));
      expect(icons.filter((ic) => stateWords.some((w) => within(line2).queryByRole("img", { name: w }) === ic)).length, `${name} の状態のアイコンが 1 つでない`).toBe(1);
      // 種類の後ろに並ぶ
      const kind = line2.firstElementChild!;
      for (const ic of icons) expect(kind.compareDocumentPosition(ic) & Node.DOCUMENT_POSITION_FOLLOWING, `${name} のアイコンが種類より前にある`).toBeTruthy();
      c.icons.forEach((w, i) => {
        expect(hasRing(icons[i]), `${name} の「${w}」の丸の形`).toBe(stateWords.includes(w));
        shapes.set(w, icons[i].querySelector("svg")!.innerHTML);
      });
    }
    // 色だけに頼らないよう、6 つのアイコンは形がすべて違う
    expect(shapes.size).toBe(6);
    expect(new Set(shapes.values()).size, "形が同じアイコンがある").toBe(6);
  });

  // @vanda:evidence review-icon-names
  it("一覧のアイコンに触れたときかキーボードでフォーカスしたときにそのアイコンの言葉を出し読み上げにも同じ言葉を渡す", async () => {
    open([
      claimEntry(1, "c-proposed", { flags: ["ai-proposal", "guess", "changed-since-agreement"] }),
      claimEntry(2, "c-agreed", { state: "agreed" }),
      claimEntry(3, "c-detached", { detachedAgreement: { by: ["#31"] } }),
    ]);
    await listRow("c-proposed");
    const user = userEvent.setup();
    for (const word of ["合意済", "提案中", "合意が外れた", "AI 案", "推測", "差分あり"]) {
      // 読み上げの名前で 1 つだけ見つかる
      const icon = within(listRegion()).getByRole("img", { name: word });
      const tip = () => within(icon).queryByRole("tooltip");
      expect(tip(), `${word}: 触れる前から言葉が出ている`).toBeNull();
      await user.hover(icon);
      expect((await within(icon).findByRole("tooltip")).textContent ?? "", `${word}: 触れたときに言葉が出ない`).toMatch(new RegExp(`^${word}`));
      await user.unhover(icon);
      await waitFor(() => expect(tip(), `${word}: 離れても言葉が消えない`).toBeNull());
    }
    // キーボードでは、Tab で順に移る
    const names: string[] = [];
    for (let i = 0; i < 40 && names.length < 6; i++) {
      await user.tab();
      const el = document.activeElement as HTMLElement;
      if (el.getAttribute("role") !== "img" || !listRegion().contains(el)) continue;
      const word = el.getAttribute("aria-label") ?? "";
      expect(within(listRegion()).getByRole("img", { name: word }), `${word}: 読み上げの名前とフォーカスしたものが合わない`).toBe(el);
      expect((await within(el).findByRole("tooltip")).textContent ?? "", `${word}: フォーカスしたときに言葉が出ない`).toMatch(new RegExp(`^${word}`));
      names.push(word);
    }
    expect(names, "Tab で移ったアイコン").toEqual(["提案中", "AI 案", "推測", "差分あり", "合意済", "合意が外れた"]);
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

describe("RV-01 一覧の絞り込み", () => {
  const list = () => screen.getByRole("region", { name: "変わったものの一覧" });
  // shown は、一覧に名前 name の 1 件が出ているか
  const shown = (name: string) => within(list()).queryByText(name, { exact: true }) !== null;
  const searchBox = () => within(list()).getByRole("searchbox", { name: "絞り込み" });
  const notAgreedBox = () => within(list()).getByRole("checkbox", { name: "合意済でないものだけ" });

  const term = (no: number, id: string, name: string, definition: string, over: Partial<Entry> = {}) =>
    claimEntry(no, id, { kind: "term", group: "terms-features", before: { name, definition: plain(definition), partOf: null }, after: { name, definition: plain(definition), partOf: null }, ...over });
  const feature = (no: number, id: string, name: string, definition: string) =>
    claimEntry(no, id, { kind: "feature", group: "terms-features", before: { name, definition: plain(definition), core: null, formal: null, formalWaiver: null }, after: { name, definition: plain(definition), core: null, formal: null, formalWaiver: null } });
  // 一覧の API は、まとまりごとに続けて返す（まとまりが飛び飛びだと、同じまとまりが 2 つ出る）
  const byGroup = (es: Entry[]) => ["layer:model", "terms-features", "files"].flatMap((g) => es.filter((e) => e.group === g));
  const statementOf = (statement: string, rationale: string | null = null) => ({
    statement: plain(statement),
    rationale: rationale === null ? null : plain(rationale),
    condition: null,
    subjects: ["task-id"],
    layers: ["model"],
    evidence: [],
    flags: [],
    guesses: [],
    modelCheckWaiver: null,
  });

  // 書く文字は大文字と小文字を混ぜ、当たる側も、見る所ごとに大文字・小文字を変える
  const textEntries = () => {
    const hits: [string, Entry][] = [
      ["約束の ID", claimEntry(1, "apple-claim")],
      ["約束の文", claimEntry(2, "c-statement", { after: statementOf("APPLE を数える") })],
      ["消した約束の前の文", claimEntry(3, "c-deleted", { change: "deleted", after: null, fingerprint: null, before: statementOf("apple を消す") })],
      ["用語の ID", term(4, "t-apple", "果物", "食べるもの")],
      ["用語の名前", term(5, "t-name", "りんご ApPlE", "食べるもの")],
      ["用語の定義", term(6, "t-def", "果物", "赤い aPPLE のこと")],
      ["機能の名前", feature(7, "f-name", "Apple を並べる", "並べる")],
      ["機能の定義", feature(8, "f-def", "並べる", "APPLE を並べる")],
      ["ファイルのパス", fileEntry(9, "e2e/APPLE.txtar")],
    ];
    const misses: [string, Entry][] = [
      ["文にも ID にもない約束", claimEntry(10, "c-miss")],
      ["理由にだけある約束", claimEntry(11, "c-rationale", { after: statementOf("数える", "apple のため") })],
      ["中身にだけあるファイル", fileEntry(12, "e2e/other.txtar", { after: { text: "apple\n" } })],
      ["名前と定義にない用語", term(13, "t-miss", "果物", "食べるもの")],
    ];
    return { hits, misses };
  };

  // @vanda:evidence review-filter-text
  it("書いた文字で絞り込むと約束と用語と機能はIDと名前と文と定義にファイルはパスにその文字を大文字と小文字を区別せずに含むものだけを出す", async () => {
    const { hits, misses } = textEntries();
    const detachedHit = claimEntry(0, "apple-detached", { no: null, change: "unchanged", before: null, detachedAgreement: { by: ["#30"] } });
    const detachedMiss = claimEntry(0, "c-detached-miss", { no: null, change: "unchanged", before: null, detachedAgreement: { by: ["#30"] } });
    open(byGroup([...hits, ...misses].map(([, e]) => e)), { detached: [detachedHit, detachedMiss] });
    const name = (e: Entry) => (e.kind === "file" ? e.path! : e.id!);
    await waitFor(() => expect(shown("apple-claim")).toBe(true));
    // 絞り込む前は全部出る
    for (const [what, e] of [...hits, ...misses]) expect(shown(name(e)), `絞り込む前に${what}が出ない`).toBe(true);
    await userEvent.setup().type(searchBox(), "aPpLe");
    await waitFor(() => expect(shown("c-miss")).toBe(false));
    for (const [what, e] of hits) expect(shown(name(e)), `${what}に文字があるのに出ない`).toBe(true);
    for (const [what, e] of misses) expect(shown(name(e)), `${what}が出ている`).toBe(false);
    // 合意が外れたものも、同じく絞り込む
    expect(shown("apple-detached"), "合意が外れたもので文字があるものが出ない").toBe(true);
    expect(shown("c-detached-miss"), "合意が外れたもので文字がないものが出ている").toBe(false);
  });

  // 合意済でないもの（提案中と、合意が外れたもの）。消したものは、消したことへの合意がなければ提案中
  const stateEntries = () => {
    const kept: [string, Entry][] = [
      ["提案中の約束", claimEntry(1, "c-proposed", { state: "proposed" })],
      ["提案中のファイル", fileEntry(2, "e2e/proposed.txtar", { state: "proposed" })],
      ["合意が外れた提案中の約束", claimEntry(3, "c-detached-proposed", { state: "proposed", detachedAgreement: { by: ["#31"] } })],
      ["ほかのストーリーの合意で合意済だが合意が外れた約束", claimEntry(4, "c-detached-agreed", { state: "agreed", detachedAgreement: { by: ["#31"] } })],
      ["消したことへの合意がない消した約束", claimEntry(7, "c-deleted-proposed", { change: "deleted", after: null, state: "proposed", fingerprint: null })],
    ];
    const dropped: [string, Entry][] = [
      ["合意済の約束", claimEntry(5, "c-agreed", { state: "agreed" })],
      ["合意済のファイル", fileEntry(6, "e2e/agreed.txtar", { state: "agreed" })],
      ["消したことに合意済の消した約束", claimEntry(8, "c-deleted-agreed", { change: "deleted", after: null, state: "agreed", fingerprint: null })],
    ];
    const detached = [
      claimEntry(0, "u-detached-proposed", { no: null, change: "unchanged", before: null, state: "proposed", detachedAgreement: { by: ["#30"] } }),
      fileEntry(0, "e2e/u-detached-agreed.txtar", { no: null, change: "unchanged", before: null, state: "agreed", detachedAgreement: { by: ["#30"] } }),
    ];
    return { kept, dropped, detached };
  };

  // @vanda:evidence review-filter-not-agreed
  it("合意済でないものだけにすると提案中のものと合意が外れたものだけを出し合意済のものは消したものも含めて出さない", async () => {
    const { kept, dropped, detached } = stateEntries();
    open(byGroup([...kept, ...dropped].map(([, e]) => e)), { detached });
    const name = (e: Entry) => (e.kind === "file" ? e.path! : e.id!);
    await waitFor(() => expect(shown("c-proposed")).toBe(true));
    const user = userEvent.setup();
    expect((notAgreedBox() as HTMLInputElement).checked).toBe(false);
    await user.click(notAgreedBox());
    await waitFor(() => expect(shown("c-agreed")).toBe(false));
    for (const [what, e] of kept) expect(shown(name(e)), `${what}が出ない`).toBe(true);
    for (const [what, e] of dropped) expect(shown(name(e)), `${what}が出ている`).toBe(false);
    for (const e of detached) expect(shown(name(e)), `合意が外れたもの ${name(e)} が出ない`).toBe(true);
    // 外すと全部に戻る
    await user.click(notAgreedBox());
    await waitFor(() => expect(shown("c-agreed")).toBe(true));
    for (const [what, e] of [...kept, ...dropped]) expect(shown(name(e)), `外した後に${what}が出ない`).toBe(true);
  });

  // @vanda:evidence review-filter-not-agreed
  it("合意済でないものだけにするのと文字の絞り込みを重ねると両方に当たるものだけを出す", async () => {
    open([
      claimEntry(1, "pear-proposed", { state: "proposed" }),
      claimEntry(2, "pear-agreed", { state: "agreed" }),
      claimEntry(3, "plum-proposed", { state: "proposed" }),
      claimEntry(4, "plum-agreed", { state: "agreed" }),
    ]);
    await waitFor(() => expect(shown("pear-proposed")).toBe(true));
    const user = userEvent.setup();
    await user.type(searchBox(), "pear");
    await user.click(notAgreedBox());
    await waitFor(() => expect(shown("pear-agreed")).toBe(false));
    expect(shown("pear-proposed"), "両方に当たるものが出ない").toBe(true);
    expect(shown("plum-proposed"), "文字に当たらないものが出ている").toBe(false);
    expect(shown("plum-agreed"), "どちらにも当たらないものが出ている").toBe(false);
  });

  // @vanda:evidence review-filter-count
  it("絞り込んでいるあいだは全体の件数に加えて絞り込んだ後の件数を示し絞り込んでいないときは全体の件数だけを示す", async () => {
    open([claimEntry(1, "pear-a"), claimEntry(2, "pear-b", { state: "agreed" }), claimEntry(3, "plum-c"), claimEntry(4, "fig-e"), fileEntry(5, "e2e/pear.txtar")]);
    const heading = () => within(list()).getByRole("heading", { level: 2 }).textContent ?? "";
    await waitFor(() => expect(heading()).toMatch(/全 5 件/));
    expect(heading()).not.toMatch(/絞り込/);
    const user = userEvent.setup();
    await user.type(searchBox(), "pear");
    await waitFor(() => expect(heading()).toMatch(/絞り込んだ後 3 件/));
    expect(heading()).toMatch(/全 5 件/);
    await user.click(notAgreedBox());
    await waitFor(() => expect(heading()).toMatch(/絞り込んだ後 2 件/));
    expect(heading()).toMatch(/全 5 件/);
    // 文字を消しても、合意済でないものだけにしているあいだは絞り込んでいる
    await user.clear(searchBox());
    await waitFor(() => expect(heading()).toMatch(/絞り込んだ後 4 件/));
    await user.click(notAgreedBox());
    await waitFor(() => expect(heading()).not.toMatch(/絞り込/));
    expect(heading()).toMatch(/全 5 件/);
  });

  // @vanda:evidence review-filter-count
  it("絞り込みで1件もなければその旨を示し1件でも当たれば示さない", async () => {
    const detached = claimEntry(0, "kiwi-detached", { no: null, change: "unchanged", before: null, detachedAgreement: { by: ["#30"] } });
    open([claimEntry(1, "pear-a"), claimEntry(2, "plum-b")], { detached: [detached] });
    const none = () => within(list()).queryByText(/絞り込みの条件に当たるものはありません/) !== null;
    await waitFor(() => expect(shown("pear-a")).toBe(true));
    expect(none(), "絞り込む前から 0 件と示している").toBe(false);
    const user = userEvent.setup();
    await user.type(searchBox(), "melon");
    await waitFor(() => expect(none(), "1 件もないのに示さない").toBe(true));
    await user.clear(searchBox());
    await user.type(searchBox(), "pear");
    await waitFor(() => expect(shown("pear-a")).toBe(true));
    expect(none(), "当たるものがあるのに 0 件と示している").toBe(false);
    // 総数に数えない合意が外れたものだけが当たるときも、一覧に出るものがあるので示さない
    await user.clear(searchBox());
    await user.type(searchBox(), "kiwi");
    await waitFor(() => expect(shown("kiwi-detached")).toBe(true));
    expect(none(), "合意が外れたものが出ているのに 0 件と示している").toBe(false);
  });

  // @vanda:evidence review-filter-kept
  it("絞り込みの条件をURLに残し読み込み直しても約束の詳細から戻っても同じ条件で絞り込む", async () => {
    const entries = [claimEntry(1, "pear-a"), claimEntry(2, "pear-b", { state: "agreed" }), claimEntry(3, "plum-c")];
    const detail = http.get("/api/claims/:id", () => HttpResponse.json(claimDetail({ id: "pear-a" })));
    open(entries, {}, detail);
    await waitFor(() => expect(shown("plum-c")).toBe(true));
    const user = userEvent.setup();
    await user.type(searchBox(), "PEAR");
    await user.click(notAgreedBox());
    await waitFor(() => expect(shown("pear-b")).toBe(false));
    const filtered = () => {
      expect(shown("pear-a"), "条件に当たるものが出ない").toBe(true);
      expect(shown("pear-b"), "合意済のものが出ている").toBe(false);
      expect(shown("plum-c"), "文字に当たらないものが出ている").toBe(false);
      expect((searchBox() as HTMLInputElement).value, "絞り込みの欄の文字").toBe("PEAR");
      expect((notAgreedBox() as HTMLInputElement).checked, "合意済でないものだけの印").toBe(true);
    };
    filtered();
    const url = window.location.pathname + window.location.search;
    expect(url).toContain("PEAR");

    // 読み込み直す（同じ URL で画面を開き直す）
    cleanup();
    server.resetHandlers();
    open(entries, {}, detail);
    await waitFor(() => expect(shown("pear-a")).toBe(true));
    filtered();

    // 約束の詳細へ移って戻る
    await user.click(within(list()).getByText("pear-a", { exact: true }));
    await user.click(await screen.findByRole("link", { name: "約束の詳細を開く" }));
    await user.click(await screen.findByRole("link", { name: "ストーリーのレビューへ戻る" }));
    await waitFor(() => expect(shown("pear-a")).toBe(true));
    filtered();
  });
});

// 参照（show-references）。自然言語で書く項目は片の配列で、参照の片は ref に指す ID を持ち、応答の最上位の refs に指す先がある
// （docs/plans/stage4-references.md の「API の形」と「画面」）。
describe("参照", () => {
  const ref = (text: string, id: string) => ({ text, ref: id });
  // fieldRefs は、自然言語で書く項目の全部（約束の 6 つ、用語の定義、機能の定義と形式手法を使わない理由）に、それぞれ別の用語を指す参照を書いたときの、見せる名前
  const fieldRefs = {
    statement: "文の用語",
    rationale: "理由の用語",
    condition: "条件の用語",
    guesses: "推測の用語",
    modelCheckWaiver: "モデル検査しない理由の用語",
    detail: "エビデンスの詳細の用語",
    termDefinition: "用語の定義の用語",
    featureDefinition: "機能の定義の用語",
    formalWaiver: "形式手法を使わない理由の用語",
  };
  type Field = keyof typeof fieldRefs;
  const idOf = (field: Field) => `t-${field.toLowerCase()}`;
  const termRefs = Object.fromEntries(
    (Object.keys(fieldRefs) as Field[]).map((f) => [idOf(f), { kind: "term", name: fieldRefs[f], text: `${fieldRefs[f]}の定義`, current: true }]),
  );
  const allRefs = {
    ...termRefs,
    "f-sort": { kind: "feature", name: "並べ替え", text: "タスクを順に並べる", current: true },
    "c-other": { kind: "claim", name: null, text: "ほかの約束の文", current: true },
    "c-gone": { kind: "claim", name: null, text: "消えた約束の文", current: false },
  };

  const claimWithRefs = () =>
    claimEntry(1, "c-all", {
      change: "added",
      before: null,
      after: {
        statement: [ref(fieldRefs.statement, idOf("statement")), { text: "と" }, ref("並べ替え", "f-sort"), { text: "と" }, ref("c-other", "c-other"), { text: "と" }, ref("c-gone", "c-gone"), { text: "の文" }],
        rationale: [ref(fieldRefs.rationale, idOf("rationale")), { text: "の理由" }],
        condition: [ref(fieldRefs.condition, idOf("condition")), { text: "の条件" }],
        subjects: ["task-id"],
        layers: ["model"],
        evidence: [{ stage: "implementation", type: "unit-test", layers: ["model"], detail: [ref(fieldRefs.detail, idOf("detail")), { text: "で確かめる" }] }],
        flags: [],
        guesses: [[ref(fieldRefs.guesses, idOf("guesses")), { text: "の推測" }]],
        modelCheckWaiver: [ref(fieldRefs.modelCheckWaiver, idOf("modelCheckWaiver")), { text: "は形で確かめない" }],
      },
    } as Partial<Entry>);
  const termWithRefs = () =>
    claimEntry(2, "t-ref", {
      kind: "term", group: "terms-features", change: "added", before: null,
      after: { name: "参照を持つ用語", definition: [ref(fieldRefs.termDefinition, idOf("termDefinition")), { text: "の一部" }], partOf: null },
    } as Partial<Entry>);
  const featureWithRefs = () =>
    claimEntry(3, "f-ref", {
      kind: "feature", group: "terms-features", change: "added", before: null,
      after: {
        name: "参照を持つ機能",
        definition: [ref(fieldRefs.featureDefinition, idOf("featureDefinition")), { text: "を使う" }],
        core: true,
        formal: false,
        formalWaiver: [ref(fieldRefs.formalWaiver, idOf("formalWaiver")), { text: "のため" }],
      },
    } as Partial<Entry>);

  // refElement は、見せる文字が name の参照の要素（キーボードでフォーカスできる要素のうち、name を含む最も短いもの）。
  // 説明（tooltip）を要素の中に持つ作りもありうるので、文字がちょうど name の要素には限らない
  const refElement = (root: HTMLElement, name: string) => {
    const focusable = Array.from(root.querySelectorAll<HTMLElement>("*")).filter(
      (el) => (el.textContent ?? "").includes(name) && (el.tabIndex >= 0 || el.matches("a[href]")),
    );
    focusable.sort((a, b) => (a.textContent ?? "").length - (b.textContent ?? "").length);
    return focusable[0];
  };
  // expectRefLook は、参照が、前後の参照でない文字と見分けられる形で出ているか（参照だけの要素で、囲む要素と違う見た目の印を持つ）を確かめる。
  // jsdom は描かないので、下線や色そのものは見ない
  const expectRefLook = (el: HTMLElement | undefined, name: string, where: string) => {
    expect(el, `${where}: 参照 ${name} が、フォーカスできる要素で出ていない`).toBeTruthy();
    expect((el!.textContent ?? "").startsWith(name), `${where}: 参照 ${name} の要素が、前の文字も含んでいる`).toBe(true);
    // 参照だけを包む要素が重なることがある（差分の 1 つずつを包む要素など）ので、参照より多くの文字を持つ祖先（前後の文字も含む要素）まで上って比べる
    let parent = el!.parentElement;
    while (parent && (parent.textContent ?? "").length <= (el!.textContent ?? "").length) parent = parent.parentElement;
    expect(parent, `${where}: 参照 ${name} の前後の文字を含む要素がない`).toBeTruthy();
    if (!parent) return;
    expect(el!.tagName + " " + el!.className, `${where}: 参照 ${name} の見た目が、囲む要素と同じ`).not.toBe(parent.tagName + " " + parent.className);
    // 前後の参照でない文字を包む要素（ほかの参照の中のものを除く）とも、見た目の印が違う
    const look = (x: Element) => x.tagName + " " + x.className;
    const inRef = (x: Element) => x.closest("[tabindex], a[href]") !== null;
    for (const other of Array.from(parent.querySelectorAll("*"))) {
      if (other.children.length > 0 || !(other.textContent ?? "").trim() || other.contains(el!) || el!.contains(other) || inRef(other)) continue;
      expect(look(el!), `${where}: 参照 ${name} の見た目が、参照でない文字「${other.textContent}」と同じ`).not.toBe(look(other));
    }
  };

  // @vanda:evidence reference-shown
  it("一覧の詳細では自然言語で書く項目のどれでも参照を用語と機能は名前で約束はIDで参照だと分かる見た目で出す", async () => {
    open([claimWithRefs(), termWithRefs(), featureWithRefs()], { refs: allRefs });
    const cases: [RegExp, Field[]][] = [
      [/c-all/, ["statement", "rationale", "condition", "guesses", "modelCheckWaiver", "detail"]],
      [/t-ref/, ["termDefinition"]],
      [/f-ref/, ["featureDefinition", "formalWaiver"]],
    ];
    for (const [entry, fields] of cases) {
      await openEntry(entry);
      await waitFor(() => expect(within(detailRegion()).queryAllByText(entry).length).toBeGreaterThan(0));
      await waitFor(() => expect(detailRegion().textContent).toContain(fieldRefs[fields[0]]));
      for (const f of fields) expectRefLook(refElement(detailRegion(), fieldRefs[f]), fieldRefs[f], `${entry.source} の ${f}`);
      // 置き換えた文字だけを出し、書いたままの参照も、用語と機能の参照の指す ID も出さない
      expect(detailRegion().textContent, `${entry.source} に書いたままの参照が出た`).not.toContain("{@id:");
      for (const f of fields) expect(detailRegion().textContent, `${entry.source} に ${f} の参照の指す ID が出た`).not.toContain(idOf(f));
    }
    await openEntry(/c-all/);
    await waitFor(() => expect(detailRegion().textContent).toContain("並べ替え"));
    expectRefLook(refElement(detailRegion(), "並べ替え"), "並べ替え", "機能を指す参照");
    expectRefLook(refElement(detailRegion(), "c-other"), "c-other", "約束を指す参照");
    expectRefLook(refElement(detailRegion(), "c-gone"), "c-gone", "今ない約束を指す参照");
    expect(detailRegion().textContent, "機能の参照の指す ID が出た").not.toContain("f-sort");
  });

  // @vanda:evidence reference-shown
  it("約束を指す参照のうち今ある約束はその約束の詳細へのリンクにし用語と機能と今ない約束はリンクにしない", async () => {
    window.history.pushState({}, "", "/");
    open([claimWithRefs()], { refs: allRefs },
      http.get("/api/claims/:id", ({ params }) => HttpResponse.json(claimDetail({ id: String(params.id), statement: plain(`${String(params.id)} の詳細の文`) }))));
    await openEntry(/c-all/);
    await waitFor(() => expect(detailRegion().textContent).toContain("並べ替え"));
    const region = detailRegion();
    for (const name of [fieldRefs.statement, "並べ替え", "c-gone"]) {
      expect(within(region).queryByRole("link", { name }), `${name} がリンクになっている`).toBeNull();
    }
    const link = within(region).getByRole("link", { name: "c-other" });
    expect(new URL((link as HTMLAnchorElement).href).pathname).toBe("/claims/c-other");
    const beforeURL = window.location.pathname + window.location.search;
    const user = userEvent.setup();
    await user.click(link);
    expect(await screen.findByText("c-other の詳細の文")).toBeTruthy();
    expect(window.location.pathname).toBe("/claims/c-other");
    // 参照から移った先からも、行く前の画面の状態に戻れる（pitfall link-drops-default-state）
    await user.click(await screen.findByRole("link", { name: "ストーリーのレビューへ戻る" }));
    await waitFor(() => expect(window.location.pathname).toBe("/"));
    expect(window.location.pathname + window.location.search).toBe(beforeURL);
  });

  // @vanda:evidence reference-shown
  it("項目の差分の中の参照も名前で参照だと分かる見た目で出し消した参照はdelに足した参照はinsに出す", async () => {
    open([
      claimEntry(1, "c-diff", {
        before: { ...claimEntry(1, "c-diff").before!, statement: [ref("前も後もある用語", "t-both"), { text: "は" }, ref("消した用語", "t-gone"), { text: "を使う" }] },
        after: { ...claimEntry(1, "c-diff").after!, statement: [ref("前も後もある用語", "t-both"), { text: "は" }, ref("足した用語", "t-new"), { text: "を使う" }] },
        diff: {
          fields: {
            statement: [
              { op: "equal", text: "前も後もある用語", ref: "t-both" },
              { op: "equal", text: "は" },
              { op: "delete", text: "消した用語", ref: "t-gone" },
              { op: "insert", text: "足した用語", ref: "t-new" },
              { op: "equal", text: "を使う" },
            ],
          },
        },
      } as Partial<Entry>),
    ], {
      refs: {
        "t-both": { kind: "term", name: "前も後もある用語", text: "両方の定義", current: true },
        "t-gone": { kind: "term", name: "消した用語", text: "消した定義", current: false },
        "t-new": { kind: "term", name: "足した用語", text: "足した定義", current: true },
      },
    });
    await openEntry(/c-diff/);
    await waitFor(() => expect(detailRegion().textContent).toContain("足した用語"));
    for (const name of ["前も後もある用語", "消した用語", "足した用語"]) expectRefLook(refElement(detailRegion(), name), name, "差分");
    expect(refElement(detailRegion(), "消した用語")!.closest("del"), "消した参照が del の中にない").toBeTruthy();
    expect(refElement(detailRegion(), "足した用語")!.closest("ins"), "足した参照が ins の中にない").toBeTruthy();
    expect(refElement(detailRegion(), "前も後もある用語")!.closest("del, ins"), "変わっていない参照が del・ins の中にある").toBeNull();
  });

  // tooltipWith は、text を含む説明（role=tooltip）
  const tooltipWith = (text: string) => screen.queryAllByRole("tooltip").find((t) => (t.textContent ?? "").includes(text)) ?? null;

  // @vanda:evidence reference-popup
  it("参照に触れた時とキーボードでフォーカスした時に用語と機能はその定義を約束はその文を出し離れると消す", async () => {
    open([claimWithRefs()], { refs: allRefs });
    await openEntry(/c-all/);
    await waitFor(() => expect(detailRegion().textContent).toContain("並べ替え"));
    const user = userEvent.setup();
    const cases: [string, string][] = [
      [fieldRefs.statement, `${fieldRefs.statement}の定義`],
      ["並べ替え", "タスクを順に並べる"],
      ["c-other", "ほかの約束の文"],
      ["c-gone", "消えた約束の文"],
    ];
    for (const [name, text] of cases) {
      const el = refElement(detailRegion(), name)!;
      expect(el, `参照 ${name} がない`).toBeTruthy();
      expect(tooltipWith(text), `${name}: 触れる前から出ている`).toBeNull();
      await user.hover(el);
      await waitFor(() => expect(tooltipWith(text), `${name}: 触れても ${text} が出ない`).toBeTruthy());
      await user.unhover(el);
      await waitFor(() => expect(tooltipWith(text), `${name}: 離れても消えない`).toBeNull());
      // キーボードで（Tab で）たどり着ける
      (document.activeElement as HTMLElement | null)?.blur();
      let reached = false;
      for (let i = 0; i < 200 && !reached; i++) {
        await user.tab();
        reached = document.activeElement === el;
      }
      expect(reached, `${name}: Tab でフォーカスできない`).toBe(true);
      await waitFor(() => expect(tooltipWith(text), `${name}: フォーカスしても ${text} が出ない`).toBeTruthy());
      el.blur();
      await waitFor(() => expect(tooltipWith(text), `${name}: フォーカスを外しても消えない`).toBeNull());
    }
  });
});

describe("参照の絞り込み", () => {
  const list = () => screen.getByRole("region", { name: "変わったものの一覧" });
  const shown = (name: string) => within(list()).queryByText(name, { exact: true }) !== null;
  const searchBox = () => within(list()).getByRole("searchbox", { name: "絞り込み" });
  const ref = (text: string, id: string) => ({ text, ref: id });
  const withStatement = (no: number, id: string, statement: { text: string; ref?: string }[]) =>
    claimEntry(no, id, { after: { ...claimEntry(no, id).after!, statement } } as Partial<Entry>);

  // @vanda:evidence reference-filter
  it("書いた文字で絞り込むと参照を置き換えた後の文に当て名前で当たり書いたままの参照や指す用語のIDでは当たらない", async () => {
    const deletedBefore = { ...claimEntry(3, "c-deleted").before!, statement: [ref("りんご", "t-apple"), { text: "を消す" }] };
    const entries: Entry[] = [
      withStatement(1, "c-fruit", [ref("りんご", "t-apple"), { text: "を数える" }]),
      // {@@ を {@ にした文字の片。見せる文字に {@id:t-apple} がある
      withStatement(2, "c-literal", [{ text: "{@id:t-apple} は文字" }]),
      claimEntry(3, "c-deleted", { change: "deleted", after: null, fingerprint: null, before: deletedBefore } as Partial<Entry>),
      withStatement(4, "c-plain", [{ text: "バナナを数える" }]),
      claimEntry(5, "t-def", {
        kind: "term", group: "terms-features",
        before: { name: "果物", definition: [ref("りんご", "t-apple"), { text: "の仲間" }], partOf: null },
        after: { name: "果物", definition: [ref("りんご", "t-apple"), { text: "の仲間" }], partOf: null },
      } as Partial<Entry>),
    ];
    open(entries, { refs: { "t-apple": { kind: "term", name: "りんご", text: "赤い果物", current: true } } });
    await waitFor(() => expect(shown("c-fruit")).toBe(true));
    const user = userEvent.setup();
    const search = async (text: string, want: Record<string, boolean>) => {
      await user.clear(searchBox());
      await user.click(searchBox());
      // { は user.type では特別な文字なので、貼り付けで入れる
      await user.paste(text);
      await waitFor(() => {
        for (const [id, on] of Object.entries(want)) expect(shown(id), `「${text}」で ${id} が${on ? "出ない" : "出ている"}`).toBe(on);
      });
    };
    // 参照を置き換えた名前で当たる（約束の文、消した約束の前の文、用語の定義）
    await search("りんご", { "c-fruit": true, "c-deleted": true, "t-def": true, "c-literal": false, "c-plain": false });
    // 書いたままの参照では当たらない。見せる文字に {@id:…} がある c-literal だけに当たる
    await search("{@id:t-apple}", { "c-fruit": false, "c-deleted": false, "t-def": false, "c-literal": true, "c-plain": false });
    // 参照の指す用語の ID は見せていないので当たらない
    await search("t-apple", { "c-fruit": false, "c-deleted": false, "t-def": false, "c-literal": true, "c-plain": false });
    // 参照と、その後ろの参照でない文字をまたいで当たる
    await search("りんごを数", { "c-fruit": true, "c-deleted": false, "t-def": false, "c-literal": false, "c-plain": false });
  });
});

describe("画面の上の帯", () => {
  // @vanda:evidence header-context
  it("画面の上の帯に今のブランチとgitのuser.nameとuser.emailとvandaの版を示しブランチにいないときとuser.nameとuser.emailがないときはそう示す", async () => {
    const cases = [
      { name: "ブランチにいて人が分かる", over: {}, want: ["story-24", "合意する人", "po@example.com", "0.1.0"], not: ["ブランチにいません", "設定されていません"] },
      { name: "ブランチにいない", over: { branch: "", version: "2.3.4" }, want: ["ブランチにいません", "合意する人", "po@example.com", "2.3.4"], not: ["story-24"] },
      { name: "user.nameとuser.emailがない", over: { user: null }, want: ["story-24", "user.name", "user.email", "設定されていません", "0.1.0"], not: ["合意する人", "po@example.com"] },
    ];
    for (const c of cases) {
      open([claimEntry(1, "c-one")], {}, http.get("/api/context", () => HttpResponse.json({ ...ctx, ...c.over })));
      const banner = screen.getByRole("banner");
      await waitFor(() => expect(banner.textContent, `${c.name}: 帯に ${c.want[0]} がない`).toContain(c.want[0]));
      for (const w of c.want) expect(banner.textContent, `${c.name}: 帯に ${w} がない`).toContain(w);
      for (const w of c.not) expect(banner.textContent, `${c.name}: 帯に ${w} が出ている`).not.toContain(w);
      cleanup();
      server.resetHandlers();
    }
  });
});

describe("RV-01 一覧の件数・並び・外れた合意", () => {
  const listHeading = () => within(listRegion()).getByRole("heading", { level: 2 });

  // @vanda:evidence review-change-counts
  it("一覧の総数とともに追加と変更と削除のそれぞれの件数を示す", async () => {
    const add = { change: "added", before: null } as const;
    const del = { change: "deleted", after: null, fingerprint: null } as const;
    open([
      claimEntry(1, "c-add-1", add),
      claimEntry(2, "c-add-2", add),
      claimEntry(3, "c-mod-1"),
      claimEntry(4, "c-mod-2"),
      claimEntry(5, "c-del-1", del),
      fileEntry(6, "e2e/add.txtar", add),
      fileEntry(7, "e2e/mod.txtar"),
      fileEntry(8, "e2e/mod2.txtar"),
      fileEntry(9, "e2e/mod3.txtar"),
    ]);
    await listRow("c-add-1");
    expect(listHeading().textContent).toMatch(/(^|[^0-9])9([^0-9]|$)/);
    const text = listRegion().textContent ?? "";
    expect(text, "追加の件数").toMatch(/追加\D{0,3}3([^0-9]|$)/);
    expect(text, "変更の件数").toMatch(/変更\D{0,3}5([^0-9]|$)/);
    expect(text, "削除の件数").toMatch(/削除\D{0,3}1([^0-9]|$)/);
  });

  // @vanda:evidence review-detached-on-top
  it("一覧の外に示す外れた合意はまとまりの上に合意が外れたものとしてまとめ総数に数えないことを添える", async () => {
    const unchanged = (id: string) => claimEntry(0, id, { no: null, change: "unchanged", before: null, detachedAgreement: { by: ["#30"] } });
    open([claimEntry(1, "c-model"), fileEntry(2, "e2e/a.txtar")], { detached: [unchanged("c-detached-1"), unchanged("c-detached-2")] });
    await listRow("c-detached-1");
    const headings = within(listRegion()).getAllByRole("heading", { level: 3 });
    const top = headings.find((h) => /合意が外れたもの/.test(h.textContent ?? ""));
    expect(top, "「合意が外れたもの」の見出しがない").toBeTruthy();
    expect(headings[0], "「合意が外れたもの」の見出しが、まとまりの見出しより上にない").toBe(top);
    expect(top!.textContent, "総数に数えないことを添えていない").toMatch(/総数に数え(ない|ません)/);
    const after = (a: Node, b: Node) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
    for (const name of ["c-detached-1", "c-detached-2"]) {
      const li = await listRow(name);
      expect(after(top!, li), `${name} が「合意が外れたもの」の見出しの下にない`).toBe(true);
      expect(after(li, headings[1]), `${name} が、まとまりの見出しより下にある`).toBe(true);
    }
    // 総数は、一覧の 2 件だけ
    expect(listHeading().textContent).toMatch(/(^|[^0-9])2([^0-9]|$)/);
    expect(listHeading().textContent).not.toMatch(/(^|[^0-9])4([^0-9]|$)/);
  });

  // @vanda:evidence review-empty-note
  it("変わったものが0件なら比べる元と今のあいだで変わったものはないと示し1件でもあれば示さない", async () => {
    const note = "比べる元と今のあいだで、変わったものはありません。";
    const detached = claimEntry(0, "c-detached", { no: null, change: "unchanged", before: null, detachedAgreement: { by: ["#30"] } });
    const cases = [
      { name: "0 件", entries: [], over: {}, shown: true },
      { name: "外れた合意だけがある", entries: [], over: { detached: [detached] }, shown: true },
      { name: "1 件", entries: [claimEntry(1, "c-one")], over: {}, shown: false },
    ];
    for (const c of cases) {
      open(c.entries, c.over);
      await waitFor(() => expect(listHeading()).toBeTruthy());
      if (c.shown) expect(await within(listRegion()).findByText(note), c.name).toBeTruthy();
      else {
        await listRow("c-one");
        expect(within(listRegion()).queryByText(note), `${c.name}: 示している`).toBeNull();
      }
      cleanup();
      server.resetHandlers();
    }
  });

  // @vanda:evidence review-groups-fold-over-ten
  it("画面を開いた時11件以上のまとまりはたたみ10件以下のまとまりと選んだ件を含むまとまりは開いておく", async () => {
    const pad = (i: number) => String(i).padStart(2, "0");
    const model = Array.from({ length: 10 }, (_, i) => claimEntry(i + 1, `m-${pad(i)}`));
    const db = Array.from({ length: 11 }, (_, i) => claimEntry(i + 11, `d-${pad(i)}`, { group: "layer:db" }));
    const files = Array.from({ length: 11 }, (_, i) => fileEntry(i + 22, `e2e/f-${pad(i)}.txtar`));
    window.history.pushState({}, "", "/?view=branch&base=main&sel=" + encodeURIComponent("path:e2e/f-05.txtar"));
    open([...model, ...db, ...files]);
    await listRow("m-00");
    const shown = (name: string) => within(listRegion()).queryByText(name, { exact: true }) !== null;
    for (const e of model) expect(shown(e.id!), `10 件のまとまりの ${e.id} が見えない`).toBe(true);
    for (const e of db) expect(shown(e.id!), `11 件のまとまりの ${e.id} が見えている`).toBe(false);
    for (const e of files) expect(shown(e.path!), `選んだ件を含む 11 件のまとまりの ${e.path} が見えない`).toBe(true);
  });

  // @vanda:evidence review-order
  it("一覧は約束のまとまりを設定のレイヤーの順に用語と機能とエビデンスの本体の順に並べ通し番号を付けて出す", async () => {
    const term = (no: number, id: string) =>
      claimEntry(no, id, { kind: "term", group: "terms-features", before: { name: id, definition: plain("定義"), partOf: null }, after: { name: id, definition: plain("定義"), partOf: null } });
    const feature = (no: number, id: string) =>
      claimEntry(no, id, { kind: "feature", group: "terms-features", before: { name: id, definition: plain("定義"), core: null, formal: null, formalWaiver: null }, after: { name: id, definition: plain("定義"), core: null, formal: null, formalWaiver: null } });
    const entries = [
      claimEntry(1, "m-a"),
      claimEntry(2, "m-b"),
      claimEntry(3, "d-a", { group: "layer:db" }),
      feature(4, "f-a"),
      term(5, "t-a"),
      fileEntry(6, "db/a.sql", { fileType: "guardrail" }),
      fileEntry(7, "e2e/a.txtar"),
    ];
    open(entries);
    await listRow("m-a");
    const groups = within(listRegion()).getAllByRole("heading", { level: 3 }).map((h) => h.textContent ?? "");
    const order = [/モデル/, /DB/, /用語と機能/, /エビデンスの本体/];
    expect(groups.length, `まとまりの見出し: ${groups.join(" / ")}`).toBe(order.length);
    order.forEach((re, i) => expect(groups[i], `${i + 1} 番目のまとまり`).toMatch(re));
    const rows = within(listRegion()).getAllByRole("listitem").map((li) => Array.from(within(li).getByRole("button").children).map((el) => el.textContent));
    expect(rows).toEqual(entries.map((e) => [String(e.no), e.kind === "file" ? e.path : e.id]));
  });
});

describe("RV-02 1 件の詳細", () => {
  // @vanda:evidence review-detail-links
  it("1件の詳細に約束なら約束の詳細を開くファイルならファイルの画面を開くのリンクを出し削除したものには出さない", async () => {
    const del = { change: "deleted", after: null, fingerprint: null } as const;
    const cases = [
      { e: claimEntry(1, "c-mod"), name: "c-mod", link: "約束の詳細を開く", href: /^\/claims\/c-mod(\?|$)/ },
      { e: claimEntry(2, "c-add", { change: "added", before: null }), name: "c-add", link: "約束の詳細を開く", href: /^\/claims\/c-add(\?|$)/ },
      { e: claimEntry(3, "c-del", del), name: "c-del", link: null, href: null },
      { e: fileEntry(4, "e2e/mod.txtar"), name: "e2e/mod.txtar", link: "ファイルの画面を開く", href: /^\/files\?(.*&)?path=e2e%2Fmod\.txtar(&|$)/ },
      { e: fileEntry(5, "e2e/add.txtar", { change: "added", before: null }), name: "e2e/add.txtar", link: "ファイルの画面を開く", href: /^\/files\?(.*&)?path=e2e%2Fadd\.txtar(&|$)/ },
      { e: fileEntry(6, "e2e/del.txtar", del), name: "e2e/del.txtar", link: null, href: null },
    ];
    open(cases.map((c) => c.e), {}, http.get("/api/claims/:id", () => HttpResponse.json(claimDetail())));
    const user = userEvent.setup();
    for (const c of cases) {
      await user.click(within(await listRow(c.name)).getByRole("button"));
      await waitFor(() => expect(within(detailRegion()).getByText(c.name, { selector: "h2", exact: false })).toBeTruthy());
      for (const name of ["約束の詳細を開く", "ファイルの画面を開く"]) {
        const links = within(detailRegion()).queryAllByRole("link", { name });
        expect(links.length, `${c.name} の「${name}」のリンクの数`).toBe(name === c.link ? 1 : 0);
        if (name === c.link) expect(links[0].getAttribute("href"), `${c.name} のリンクの先`).toMatch(c.href!);
      }
    }
  });

  // @vanda:evidence review-added-content
  it("追加した約束と用語と機能とファイルは今の中身を示す", async () => {
    const claim = claimEntry(1, "c-new", {
      change: "added",
      before: null,
      after: {
        statement: plain("足した約束の文"),
        rationale: plain("足した約束の理由"),
        condition: plain("足した約束の条件"),
        subjects: ["task-id"],
        layers: ["model", "db"],
        evidence: [{ stage: "implementation", type: "random-check", layers: ["db"], detail: plain("足したエビデンスの予定の詳細") }],
        flags: [],
        guesses: [plain("足した推測の 1 つ目"), plain("足した推測の 2 つ目")],
        modelCheckWaiver: null,
      },
    });
    const term = claimEntry(2, "t-new", { kind: "term", group: "terms-features", change: "added", before: null, after: { name: "足した用語の名前", definition: plain("足した用語の定義"), partOf: "task" } });
    const feature = claimEntry(3, "f-new", {
      kind: "feature",
      group: "terms-features",
      change: "added",
      before: null,
      after: { name: "足した機能の名前", definition: plain("足した機能の定義"), core: true, formal: false, formalWaiver: plain("足した機能の形式手法を使わない理由") },
    });
    const file = fileEntry(4, "e2e/new.txtar", { change: "added", before: null, after: { text: "足したファイルの本文\n" } });
    open([claim, term, feature, file], {}, http.get("/api/claims/:id", () => HttpResponse.json(claimDetail())));
    const cases = [
      { name: "c-new", want: ["足した約束の文", "足した約束の理由", "足した約束の条件", "task-id", "足したエビデンスの予定の詳細", "足した推測の 1 つ目", "足した推測の 2 つ目"], also: [/ランダムチェック|random-check/, /モデル|model/, /DB|db/] },
      { name: "t-new", want: ["足した用語の名前", "足した用語の定義", "task"], also: [] },
      { name: "f-new", want: ["足した機能の名前", "足した機能の定義", "足した機能の形式手法を使わない理由"], also: [] },
      { name: "e2e/new.txtar", want: ["足したファイルの本文"], also: [] },
    ];
    const user = userEvent.setup();
    for (const c of cases) {
      await user.click(within(await listRow(c.name)).getByRole("button"));
      await waitFor(() => expect(detailRegion().textContent, `${c.name} の詳細に ${c.want[0]} がない`).toContain(c.want[0]));
      const text = detailRegion().textContent ?? "";
      for (const w of c.want) expect(text, `${c.name} の詳細に ${w} がない`).toContain(w);
      for (const re of c.also) expect(text, `${c.name} の詳細に ${re} がない`).toMatch(re);
    }
  });
});

describe("CM-08 ストーリーの欄", () => {
  // @vanda:evidence story-open-link
  it("ストーリーにストーリーストアのURLがあればストーリーストアで開くのリンクを出しなければ出さない", async () => {
    for (const url of ["https://github.com/owner/app/issues/24", "https://issues.example.com/browse/24", ""]) {
      open([claimEntry(1, "c-one")], {}, http.get("/api/story", () => HttpResponse.json({ ...story, url })));
      await screen.findByText(/タスクの ID を重ならないようにする/);
      const links = screen.queryAllByRole("link", { name: "ストーリーストアで開く" });
      if (url === "") expect(links.length, "URL がないのにリンクを出した").toBe(0);
      else {
        expect(links.length, `${url} のリンクの数`).toBe(1);
        expect(links[0].getAttribute("href")).toBe(url);
      }
      cleanup();
      server.resetHandlers();
    }
  });

  // @vanda:evidence story-undecided-note
  it("ストーリーが決まっていなければストーリーの欄にストーリーが決まっていないので選ぶよう示し決まっていれば示さない", async () => {
    const note = "ストーリーが決まっていません。ストーリーを選んでください。";
    open([claimEntry(1, "c-one")], { story: null, storyChoice: "all", branchStories: [] });
    await listRow("c-one");
    const panel = () => document.querySelector(".story-panel") as HTMLElement;
    expect(await within(panel()).findByText(note)).toBeTruthy();
    cleanup();
    server.resetHandlers();
    open([claimEntry(1, "c-one")]);
    await screen.findByText(/タスクの ID を重ならないようにする/);
    expect(screen.queryByText(note)).toBeNull();
  });
});

describe("RV-06・RV-17 合意と取り消しのボタン", () => {
  // @vanda:evidence agree-button-label
  it("合意のボタンにはストーリーIDで合意すると合意するストーリーのIDを書く", async () => {
    for (const id of ["#24", "#31"]) {
      window.history.pushState({}, "", `/?view=branch&base=main&story=${encodeURIComponent(id)}`);
      open([claimEntry(1, "c-one"), fileEntry(2, "e2e/a.txtar")], { story: id });
      const user = userEvent.setup();
      for (const name of ["c-one", "e2e/a.txtar"]) {
        await user.click(within(await listRow(name)).getByRole("button"));
        await waitFor(() => expect(within(detailRegion()).getByText(name, { selector: "h2", exact: false })).toBeTruthy());
        const buttons = within(detailRegion()).getAllByRole("button").filter((b) => /合意/.test(b.textContent ?? ""));
        expect(buttons.map((b) => b.textContent), `${id} の ${name} の合意のボタン`).toEqual([`ストーリー ${id} で合意する`]);
      }
      cleanup();
      server.resetHandlers();
    }
  });

  // @vanda:evidence agree-cannot-reason-shown
  it("ストーリーが決まっていないかgitのuser.nameとuser.emailがなくて合意できない時は合意のボタンを押せなくし理由を示す", async () => {
    const cases = [
      { name: "ストーリーが決まっていない", ctxOver: {}, over: { story: null, storyChoice: "all", branchStories: [] }, reason: "ストーリーを選ぶと、合意できます。" },
      { name: "user.nameとuser.emailがない", ctxOver: { user: null }, over: {}, reason: "git の user.name と user.email を設定すると、合意できます。" },
    ];
    for (const c of cases) {
      // 合意できるかと理由は API（entries[].agree）が決める。その答えを手で作って渡す
      open([claimEntry(1, "c-one", { agree: { possible: false, reason: c.reason } })], c.over, http.get("/api/context", () => HttpResponse.json({ ...ctx, ...c.ctxOver })));
      await userEvent.setup().click(within(await listRow("c-one")).getByRole("button"));
      const button = await within(detailRegion()).findByRole("button", { name: /合意する/ });
      expect((button as HTMLButtonElement).disabled, `${c.name}: 合意のボタンを押せる`).toBe(true);
      expect(within(detailRegion()).getByText(c.reason), `${c.name}: 理由がない`).toBeTruthy();
      cleanup();
      server.resetHandlers();
    }
  });

  // @vanda:evidence revoke-others-shown
  it("取り消してもほかの人の合意が残る時は取り消す前にその人の名前と合意済のままになることを示し取り消した後にも合意済のままであることを示す", async () => {
    const stays = /合意済み?のまま/;
    const cases = [
      { name: "ほかの人の合意が残る", others: [{ name: "ほかの合意した人", email: "other@example.com" }, { name: "三人目の人", email: "third@example.com" }], stillAgreed: true },
      { name: "ほかの人の合意が残らない", others: [], stillAgreed: false },
    ];
    for (const c of cases) {
      let revoked = false;
      const entry = () =>
        claimEntry(1, "c-agreed", revoked
          ? { state: c.stillAgreed ? "agreed" : "proposed", revoke: c.stillAgreed ? { possible: false, reason: "このストーリーの合意がありません", others: [] } : null }
          : { state: "agreed", agree: { possible: false, reason: "このストーリーで今の中身にもう合意しています" }, revoke: { possible: true, reason: null, others: c.others } });
      open([], {},
        http.get("/api/review", () => HttpResponse.json(review([entry()]))),
        http.post("/api/revoke", () => {
          revoked = true;
          return HttpResponse.json({ commit: "c".repeat(40), stillAgreed: c.stillAgreed, removed: 1 });
        }));
      const user = userEvent.setup();
      await user.click(within(await listRow("c-agreed")).getByRole("button"));
      const button = await within(detailRegion()).findByRole("button", { name: /取り消/ });
      const before = detailRegion().textContent ?? "";
      for (const o of c.others) expect(before, `${c.name}: 取り消す前に ${o.name} の名前がない`).toContain(o.name);
      if (c.others.length > 0) expect(before, `${c.name}: 取り消す前に合意済のままになると示さない`).toMatch(stays);
      else expect(before, `${c.name}: 取り消す前に合意済のままになると示した`).not.toMatch(stays);
      await user.click(button);
      await waitFor(() => expect(within(detailRegion()).queryByRole("button", { name: /取り消/ }), `${c.name}: 取り消した後も取り消しのボタンがある`).toBeNull());
      await waitFor(() => expect(detailRegion().textContent, `${c.name}: 取り消した後に知らせがない`).toMatch(/取り消しました/));
      const after = detailRegion().textContent ?? "";
      if (c.stillAgreed) expect(after, `${c.name}: 取り消した後に合意済のままと示さない`).toMatch(stays);
      else expect(after, `${c.name}: 取り消した後に合意済のままと示した`).not.toMatch(stays);
      cleanup();
      server.resetHandlers();
    }
  });
});

describe("API のエラー", () => {
  const fail = (status: number, message: string, fix: string) => HttpResponse.json({ error: { message, fix } }, { status });

  // @vanda:evidence api-error-fix-shown
  it("画面はAPIのエラーを出す時に何がどうなっているかとどうすればよいかの両方を出す", async () => {
    const cases = [
      { name: "一覧", method: "get", path: "/api/review", status: 500, message: "一覧の数と差分の数が合いません。", fix: "vanda の不具合として知らせてください。" },
      { name: "ストーリー", method: "get", path: "/api/story", status: 502, message: "Bad credentials", fix: "GH_TOKEN を確かめてください。" },
      { name: "画面で選べるもの", method: "get", path: "/api/context", status: 500, message: "設定を読めません（2 件。最初の 1 件: …）。", fix: "vanda doctor で誤りを確かめて、直してください。" },
      { name: "合意", method: "post", path: "/api/agree", status: 409, message: "約束 c-one の中身が、画面に表示した後に変わりました。", fix: "一覧を読み直し、今の中身を確かめてから合意してください。" },
    ] as const;
    for (const c of cases) {
      open([claimEntry(1, "c-one")], {}, http[c.method](c.path, () => fail(c.status, c.message, c.fix)));
      if (c.name === "合意") {
        const user = userEvent.setup();
        await user.click(within(await listRow("c-one")).getByRole("button"));
        await user.click(await within(detailRegion()).findByRole("button", { name: /合意する/ }));
      }
      expect(await screen.findByText(c.message, { exact: false }), `${c.name}: message がない`).toBeTruthy();
      expect(await screen.findByText(c.fix, { exact: false }), `${c.name}: fix がない`).toBeTruthy();
      cleanup();
      server.resetHandlers();
    }
  });

  // @vanda:evidence context-error-shown
  it("画面を開いた時に今の設定と選べるものを読めなければAPIが返した理由を出す", async () => {
    server.use(http.get("/api/context", () => apiError(500, "設定（.vanda/config.toml）を読めません（1 件。最初の 1 件: .vanda/config.toml:3: foo は決まった項目ではありません。）。")));
    render(<App />);
    expect(await screen.findByText(/\.vanda\/config\.toml:3: foo は決まった項目ではありません/)).toBeTruthy();
  });
});
