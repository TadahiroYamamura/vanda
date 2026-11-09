// E2E: 一時的な git のリポジトリに対して vanda serve を動かし、Chrome で操作する。本物の GitHub には繋がない
// （ストーリーストアは設定に書かない）。vanda サーバーはテストの中で起動し、テストの終わりに止め、ポートを手放したことも確かめる。
// 動かす vanda は、make build が作るリポジトリの直下の vanda（VANDA_BIN で差し替えられる）。
import { test, expect, type Page } from "@playwright/test";
import { spawn, execFileSync, type ChildProcess } from "node:child_process";
import { createHash } from "node:crypto";
import { chmodSync, mkdtempSync, writeFileSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const bin = process.env.VANDA_BIN ?? resolve(here, "..", "..", "vanda");

const sha = (s: string) => "sha256:" + createHash("sha256").update(s, "utf8").digest("hex");

// フィンガープリントの元の JSON は、docs/formats.md の決まった形を手で書く（約束リストを読んで作らない）
const termJSON = `{"definition":"タスクを見分ける名前","id":"task-id","name":"タスクの ID"}`;
const claimJSON = (statement: string) =>
  `{"evidence":[{"layers":["model"],"stage":"implementation","type":"scenario-test"}],"id":"task-id-unique","layers":["model"],"statement":"${statement}","subjects":["task-id"]}`;

const config = (version: string) => `version = "${version}"

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[annotation]]
paths = ["e2e/*.txtar"]
adapter = "txtar"
types = ["scenario-test"]
[gui]
base = "main"
`;

const promises = (statement: string, flags: boolean, extra: boolean) => `# 約束リスト（E2E の例）

[[term]]
id = "task-id"
name = "タスクの ID"
definition = "タスクを見分ける名前"

[[claim]]
id = "task-id-unique"
subjects = ["task-id"]
statement = "${statement}"
layers = ["model"]
${flags ? 'flags = ["ai-proposal"]\nguesses = ["使い回さない"]\n' : ""}
[[claim.evidence]]
stage = "implementation"
type = "scenario-test"
layers = ["model"]
${extra ? `
[[claim]]
id = "task-title-required"
subjects = ["task-id"]
statement = "タスクの題は空にできない"
layers = ["model"]
flags = ["ai-proposal"]

[[claim.evidence]]
stage = "implementation"
type = "scenario-test"
layers = ["model"]
` : ""}`;

const scenarioBase = "# シナリオ\nexec vanda-task add a\n-- a.txt --\na\n";
const scenarioStory = "# シナリオ\n! exec vanda-task add a\n-- a.txt --\na\n";

function agreement(kind: string, key: string, fp: string, path: string | null, story: string) {
  return `
[[agreement]]
kind = "${kind}"
key = "${key}"
fingerprint = "${fp}"
${path ? `path = "${path}"\n` : ""}agreed_at = 2026-01-01T10:00:00+09:00
story = "${story}"
user = { name = "テスト担当", email = "tester@example.com" }
`;
}

interface Repo {
  root: string;
  env: NodeJS.ProcessEnv;
  git: (...args: string[]) => string;
  write: (path: string, text: string) => void;
  vanda: (...args: string[]) => { code: number; out: string };
}

function makeRepo(): Repo {
  const root = mkdtempSync(join(tmpdir(), "vanda-e2e-"));
  const env: NodeJS.ProcessEnv = {};
  for (const [k, v] of Object.entries(process.env)) {
    if (!k.startsWith("GIT_") && k !== "EMAIL" && k !== "GH_TOKEN" && k !== "GITHUB_TOKEN") env[k] = v;
  }
  Object.assign(env, { GIT_CONFIG_NOSYSTEM: "1", GIT_CONFIG_GLOBAL: "/dev/null", GIT_CEILING_DIRECTORIES: dirname(root) });
  const git = (...args: string[]) => execFileSync("git", args, { cwd: root, env, encoding: "utf8" }).trim();
  const write = (path: string, text: string) => {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    writeFileSync(join(root, path), text);
  };
  const vanda = (...args: string[]) => {
    try {
      return { code: 0, out: execFileSync(bin, args, { cwd: root, env, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }) };
    } catch (e) {
      const err = e as { status: number; stdout: string; stderr: string };
      return { code: err.status, out: (err.stdout ?? "") + (err.stderr ?? "") };
    }
  };
  return { root, env, git, write, vanda };
}

// buildRepo は、main（#1 で承認済）と、ストーリー #24 が約束を変え・足し、シナリオを変えた story-24 を作る。HEAD は story-24。
function buildRepo(): Repo {
  const r = makeRepo();
  const version = execFileSync(bin, ["version"], { encoding: "utf8" }).match(/\d+\.\d+\.\d+/)![0];
  r.git("init", "-q", "-b", "main");
  r.git("config", "user.name", "承認する人");
  r.git("config", "user.email", "po@example.com");
  r.write(".vanda/config.toml", config(version));
  r.write(".vanda/checks/merge.toml", '[[check]]\nname = "story-origin"\nscope = "story"\n\n[[check]]\nname = "agreement"\nscope = "story"\n');
  r.write(".vanda/promises.toml", promises("タスクの ID は重ならない", false, false));
  r.write("e2e/add-task.txtar", scenarioBase);
  r.write(
    ".vanda/agreements.toml",
    agreement("term", "task-id", sha(termJSON), null, "#1") +
      agreement("claim", "task-id-unique", sha(claimJSON("タスクの ID は重ならない")), null, "#1") +
      agreement("file", sha(scenarioBase), sha(scenarioBase), "e2e/add-task.txtar", "#1"),
  );
  commit(r, "はじめの約束を置く", "#1");
  r.git("switch", "-q", "-c", "story-24");
  r.write(".vanda/promises.toml", promises("タスクの ID は、すべてのタスクを通して重ならない", true, true));
  commit(r, "約束を変え、題の約束を足す", "#24");
  r.write("e2e/add-task.txtar", scenarioStory);
  commit(r, "シナリオを変える", "#24");
  return r;
}

interface Serve {
  url: string;
  stop: () => Promise<void>;
}

function startServe(r: Repo): Promise<Serve> {
  return new Promise((resolveStart, rejectStart) => {
    const p: ChildProcess = spawn(bin, ["serve", "--port", "0", "--no-open"], { cwd: r.root, env: r.env });
    let out = "";
    const exited = new Promise<void>((res) => p.on("exit", () => res()));
    const timer = setTimeout(() => rejectStart(new Error("vanda serve が URL を出さない\n" + out)), 15000);
    p.stdout!.on("data", (d) => {
      out += d.toString();
      const m = out.match(/http:\/\/127\.0\.0\.1:(\d+)\//);
      if (m) {
        clearTimeout(timer);
        const port = Number(m[1]);
        resolveStart({
          url: m[0].replace(/\/$/, ""),
          stop: async () => {
            p.kill("SIGINT");
            await Promise.race([exited, new Promise((res) => setTimeout(res, 10000))]);
            if (p.exitCode === null && p.signalCode === null) {
              p.kill("SIGKILL");
              throw new Error("vanda serve が割り込みから 10 秒たっても止まらない");
            }
            // 同じポートで待ち受けられれば、vanda サーバーはポートを手放している
            await new Promise<void>((res, rej) => {
              const s = createServer();
              s.once("error", (e) => rej(new Error(`vanda serve を止めた後も ${port} を手放していない: ${e}`)));
              s.listen(port, "127.0.0.1", () => s.close(() => res()));
            });
          },
        });
      }
    });
    p.stderr!.on("data", (d) => (out += d.toString()));
    p.on("exit", (code) => rejectStart(new Error(`vanda serve が終わった（${code}）\n${out}`)));
  });
}

let repo: Repo;
let serve: Serve;

test.beforeEach(async () => {
  repo = buildRepo();
  serve = await startServe(repo);
});

test.afterEach(async () => {
  await serve.stop();
  rmSync(repo.root, { recursive: true, force: true });
});

const listItem = (page: Page, text: string) => page.getByRole("listitem").filter({ hasText: text }).first();
// listIcon は、一覧の 1 件の状態やフラグのアイコンを、スクリーンリーダーが読み上げる名前で探す
const listIcon = (page: Page, text: string, name: string) => listItem(page, text).getByRole("img", { name, exact: true });

// select は、一覧の 1 件を選ぶ。
async function select(page: Page, text: string) {
  await page.getByText(text, { exact: false }).first().click();
}

async function agreeAll(page: Page, items: string[]) {
  for (const item of items) {
    await select(page, item);
    const button = page.getByRole("button", { name: /承認する/ });
    await expect(button).toBeEnabled();
    const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
    await button.click();
    expect((await res).status(), `${item} を承認できない`).toBe(200);
  }
}

const items = ["task-id-unique", "task-title-required", "e2e/add-task.txtar"];

// @vanda:evidence context-base-always-shown
// @vanda:evidence agree-on-screen
// @vanda:evidence review-numbering
test("ブランチのコミットからストーリーが決まるブランチで一覧を見て全部を承認するとvanda checkの承認のチェックが通る", async ({ page }) => {
  expect(repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main").code, "承認する前からチェックが通っている").toBe(1);
  await page.goto(serve.url + "/");
  // 開いたときの既定は、設定のベースブランチ。今どのベースブランチと比べているかを、選ぶ所のほかにも示す
  const baseSelect = page.getByRole("combobox", { name: "ベースブランチ" });
  await expect(baseSelect).toHaveValue("main");
  await expect(page.getByText(/main/).filter({ visible: true }).first()).toBeVisible();
  // story-24 のブランチのコミットにあるストーリーは #24 だけなので、選ばせずに示す（RV-13）
  await expect(page.getByText(/ストーリー\s*[:：]\s*#24/).first()).toBeVisible();
  for (const item of items) await expect(page.getByText(item).first()).toBeVisible();
  await expect(page.getByText(/全 3 件|3 件/).first()).toBeVisible();
  await agreeAll(page, items);
  const check = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(check.code, check.out).toBe(0);
  // 承認は 1 件ごとに 1 つのコミット
  expect(repo.git("rev-list", "--count", "main..HEAD")).toBe(String(2 + items.length));
});

test("承認した約束を変えると一覧に差分ありとして戻る", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await agreeAll(page, ["task-id-unique"]);
  const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  repo.write(".vanda/promises.toml", text.replace("すべてのタスクを通して重ならない", "どのタスクとも重ならない"));
  commit(repo, "文を直す", "#24");
  await page.reload();
  await select(page, "task-id-unique");
  await expect(listIcon(page, "task-id-unique", "差分あり")).toBeVisible();
  // 同じストーリーが承認した後に変えたので、一覧の状態のアイコンは提案中でなく承認が失効したを出す。提案中であることは詳細のバッジで見る
  await expect(listIcon(page, "task-id-unique", "承認が失効した")).toBeVisible();
  await expect(page.getByRole("region", { name: "1 件の中身" }).getByText("提案中", { exact: true }).first()).toBeVisible();
});

test("ベースブランチとレビュー単位を切り替えると一覧が変わる", async ({ page }) => {
  // release は、#24 の最初のコミットの後から分かれたことにする（シナリオの変更だけが残る）
  repo.git("branch", "release", "story-24~1");
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await expect(page.getByText("task-title-required").first()).toBeVisible();
  const baseSelect = page.getByRole("combobox", { name: "ベースブランチ" });
  await baseSelect.selectOption("release");
  await expect(page).toHaveURL(/base=release/);
  await expect(page.getByText("e2e/add-task.txtar").first()).toBeVisible();
  await expect(page.getByText("task-title-required")).toHaveCount(0);
  await expect(baseSelect).toHaveValue("release");
  await expect(page.getByText(/release/).filter({ visible: true }).first()).toBeVisible();

  const view = page.getByRole("combobox", { name: "レビュー単位" });
  await view.selectOption({ label: "ストーリー" });
  await expect(page).toHaveURL(/view=story/);
  for (const item of items) await expect(page.getByText(item).first()).toBeVisible();
});

// 約束の詳細とファイルの画面から「ストーリーのレビューへ戻る」で戻ると、行く前のレビューの画面（レビュー単位・ベースブランチ・
// ストーリー・選んでいた 1 件）に戻る。ブラウザーの「戻る」には頼らない（PO の報告では、こちらだけが選ぶ前に戻っていた）。
// レビュー単位がストーリーのときはベースブランチを選ぶ所が出ないので、レビュー単位とベースブランチの組は 3 つ。それと戻る経路 3 つの
// 組み合わせを全部回す。
// url があるものは、画面で選ばずに、その URL を直に開く
const states: { name: string; story: boolean; base: string; url?: string }[] = [
  { name: "レビュー単位がブランチのときにベースブランチを既定のmainにして", story: false, base: "main" },
  { name: "レビュー単位がブランチのときにベースブランチをreleaseに変えて", story: false, base: "release" },
  { name: "レビュー単位をストーリーに切り替えて", story: true, base: "main" },
  { name: "レビュー単位がストーリーのURLを直に開いて", story: true, base: "main", url: "/?view=story&story=%2324" },
];
const routes = [
  { name: "約束の詳細から直接", via: "claim" },
  { name: "約束の詳細からエビデンスのファイルを経て", via: "claim-file" },
  { name: "エビデンスの予定と実際の表からファイルを経て", via: "review-file" },
] as const;

for (const v of states) {
  for (const route of routes) {
      // @vanda:evidence back-restores-state
      test(`${v.name}選んだ約束で${route.name}ストーリーのレビューへ戻ると行く前と同じ状態に戻る`, async ({ page }) => {
        // シナリオに task-id-unique のエビデンスを付け、約束の文も変える。release は、このコミットの直前から分かれたことに
        // する（release と比べても task-id-unique が一覧に出て、main と比べたときとは一覧が変わる）
        repo.git("branch", "release", "story-24");
        repo.write("e2e/add-task.txtar", "# シナリオ\n# @vanda:evidence task-id-unique\n! exec vanda-task add a\n-- a.txt --\na\n");
        const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
        repo.write(".vanda/promises.toml", text.replace("すべてのタスクを通して重ならない", "どのタスクとも重ならない"));
        commit(repo, "シナリオをエビデンスにする", "#24");

        // レビュー単位がブランチのときは、ブランチのコミットからストーリー #24 が決まり、選ぶ欄は出ない（RV-13）
        await page.goto(serve.url + (v.url ?? "/"));
        const baseSelect = page.getByRole("combobox", { name: "ベースブランチ" });
        if (v.base !== "main" && !v.url) await baseSelect.selectOption(v.base);
        const view = page.getByRole("combobox", { name: "レビュー単位" });
        if (v.url) {
          await expect(storyField(page)).toHaveValue("#24");
        } else if (v.story) {
          await view.selectOption({ label: "ストーリー" });
          const value = (await storyField(page).locator("option", { hasText: "#24" }).first().getAttribute("value")) ?? "#24";
          await storyField(page).selectOption(value);
        } else {
          await expect(page.getByText(/ストーリー\s*[:：]\s*#24/).first()).toBeVisible();
        }
        if (!v.story && v.base === "release") await expect(page.getByText("task-title-required")).toHaveCount(0);
        else await expect(page.getByText("task-title-required").first()).toBeVisible();
        await select(page, "task-id-unique");
        const claimLink = page.getByRole("link", { name: "約束の詳細を開く" });
        await expect(claimLink).toHaveAttribute("href", /task-id-unique/);
        const before = {
          view: await view.inputValue(),
          base: v.story ? null : await baseSelect.inputValue(),
          story: v.story ? await storyField(page).inputValue() : null,
        };
        if (!v.story) expect(before.base).toBe(v.base);
        // 行く前の URL。戻った後は、既定の値を書き足したりせず、これとちょうど同じになる
        const beforeURL = page.url();

        if (route.via === "review-file") {
          await page.getByRole("link", { name: /e2e\/add-task\.txtar:1/ }).first().click();
          await expect(page).toHaveURL(/\/files\?/);
        } else {
          await claimLink.click();
          await expect(page.getByText("どのタスクとも重ならない").first()).toBeVisible();
          if (route.via === "claim-file") {
            await page.getByRole("link", { name: /e2e\/add-task\.txtar/ }).first().click();
            await expect(page).toHaveURL(/\/files\?/);
          }
        }
        await page.getByRole("link", { name: "ストーリーのレビューへ戻る" }).click();
        await expect(page, "戻った後の URL が、行く前とちょうど同じでない").toHaveURL(beforeURL);

        await expect(page.getByRole("combobox", { name: "レビュー単位" })).toHaveValue(before.view);
        if (before.base !== null) await expect(page.getByRole("combobox", { name: "ベースブランチ" })).toHaveValue(before.base);
        if (before.story !== null) await expect(storyField(page)).toHaveValue(before.story);
        else await expect(page.getByText(/ストーリー\s*[:：]\s*#24/).first(), "戻った後に、決まったストーリーが示されない").toBeVisible();
        await expect(page.getByRole("link", { name: "約束の詳細を開く" }), "選んでいた task-id-unique の 1 件の詳細に戻っていない").toHaveAttribute("href", /task-id-unique/);
        await expect(page.getByRole("button", { name: /承認する/ })).toBeVisible();
      });
  }
}

// RV-16 と CL-08 の前提: task-id-unique を、main（v1）、#24 の最初のコミット（v2）の後に、さらに 2 回変える（v3・v4）。
// バージョンごとに、文・理由・条件・レイヤー・エビデンスの予定に、ほかのバージョンに出てこない文字を入れる。
interface ClaimVersion {
  statement: string;
  rationale?: string;
  condition?: string;
  layers: string[];
  plan?: string;
}
const v1Statement = "タスクの ID は重ならない";
const v2Statement = "タスクの ID は、すべてのタスクを通して重ならない";
const v3: ClaimVersion = {
  statement: "タスクの ID は、消したタスクとも重ならない",
  rationale: "消したタスクの ID を使い回すと、古い記録と取り違える",
  condition: "タスクを一度でも登録したことがある",
  layers: ["model", "db"],
  plan: "TLA+ で消したタスクも含めて確かめる予定",
};
const v4: ClaimVersion = {
  statement: "タスクの ID は、どのタスクとも重ならない",
  rationale: "ID でタスクを 1 つに決めるため",
  condition: "タスクが 1 つ以上ある",
  layers: ["model"],
  plan: "モデル検査で重ならないことを確かめる予定",
};

const promisesAt = (c: ClaimVersion) => `[[term]]
id = "task-id"
name = "タスクの ID"
definition = "タスクを見分ける名前"

[[claim]]
id = "task-id-unique"
subjects = ["task-id"]
statement = "${c.statement}"
${c.rationale ? `rationale = "${c.rationale}"\n` : ""}${c.condition ? `condition = "${c.condition}"\n` : ""}layers = [${c.layers.map((l) => `"${l}"`).join(", ")}]

[[claim.evidence]]
stage = "implementation"
type = "scenario-test"
layers = ["model"]
${c.plan ? `
[[claim.evidence]]
stage = "plan"
type = "model-check"
layers = ["model"]
detail = "${c.plan}"
` : ""}`;

// commit は、作業ツリーのすべてを 1 つのコミットにする。story を渡せば、メッセージの末尾に Story: のトレーラーを書く
// （vanda はストーリーの ID をトレーラーからだけ読む）。同じ大きさのファイルを同じ時刻のうちに書き換えると、git add -A が
// stat の一致から変わっていないとみなし、書き換えをコミットに入れないことがあるので、インデックスから一度外して読み直させる。
// date を渡せば、作者とコミッターの日時をそれにする
function commit(r: Repo, subject: string, story: string | null, date?: string): string {
  r.git("rm", "-r", "-q", "--cached", "--ignore-unmatch", ".");
  r.git("add", "-A");
  const msg = story ? `${subject}\n\nStory: ${story}` : subject;
  execFileSync("git", ["-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "--allow-empty", "-m", msg], {
    cwd: r.root,
    env: date ? { ...r.env, GIT_AUTHOR_DATE: date, GIT_COMMITTER_DATE: date } : r.env,
  });
  return r.git("rev-parse", "HEAD");
}

// commitV3 は、DB のレイヤーを設定に足し、task-id-unique を v3 にする。
function commitV3(r: Repo, date?: string): string {
  const cfg = readFileSync(join(r.root, ".vanda/config.toml"), "utf8");
  r.write(".vanda/config.toml", cfg.replace("[[annotation]]", '[[layer]]\nid = "db"\nname = "データベース"\ndescription = "保存の制約"\n\n[[annotation]]'));
  r.write(".vanda/promises.toml", promisesAt(v3));
  return commit(r, "理由と条件を足す", "#24", date);
}

// 変更履歴の行と承認の記録の行は、CL の約束の詳細と同じく表の行として探す。その時点の中身は、行の中の開く・たたむを
// 切り替えるボタンで開く。クリックすると aria-expanded と名前が切り替わる
type Row = ReturnType<Page["getByRole"]>;
async function choose(row: Row) {
  await expect(row).toBeVisible();
  const open = row.getByRole("button", { name: "その時点の中身を開く" });
  await expect(open).toHaveAttribute("aria-expanded", "false");
  await open.click();
  await expect(row.getByRole("button", { name: "その時点の中身をたたむ" }), "開いた後に、ボタンがたたむに切り替わらない").toHaveAttribute("aria-expanded", "true");
}
async function collapse(row: Row) {
  await row.getByRole("button", { name: "その時点の中身をたたむ" }).click();
  await expect(row.getByRole("button", { name: "その時点の中身を開く" }), "たたんだ後に、ボタンが開くに切り替わらない").toHaveAttribute("aria-expanded", "false");
}
const rowOf = (page: Page, text: string) => page.getByRole("row").filter({ hasText: text }).first();

async function openReviewDetail(page: Page) {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await select(page, "task-id-unique");
  await expect(page.getByRole("link", { name: "約束の詳細を開く" })).toBeVisible();
}

// RV-16。中身は CL-04・CL-05 と同じ（変更履歴はコミットとストーリーの ID、承認の履歴は承認した人・ストーリー・今の中身に合うか）
test("レビューの画面の1件の詳細に約束の変更履歴と承認の履歴が出て承認すると今の中身に合う承認が履歴に足される", async ({ page }) => {
  const c1 = repo.git("rev-parse", "main");
  const c2 = repo.git("rev-parse", "story-24~1");
  const c3 = commitV3(repo);
  await openReviewDetail(page);

  // 変更履歴: 約束を作り・変えたコミットだけ。シナリオだけを変えたコミットは出ない
  for (const [hash, subject, story] of [
    [c1, "はじめの約束を置く", "#1"],
    [c2, "約束を変え、題の約束を足す", "#24"],
    [c3, "理由と条件を足す", "#24"],
  ]) {
    const row = rowOf(page, subject);
    await expect(row, `変更履歴に ${subject} がない`).toBeVisible();
    await expect(row).toContainText(hash.slice(0, 7));
    await expect(row).toContainText(story);
  }
  await expect(page.getByText("シナリオを変える")).toHaveCount(0);

  // 承認の履歴: #1 の承認は v1 への承認なので、今の中身に合わない
  const old = rowOf(page, "テスト担当");
  await expect(old).toContainText("#1");
  await expect(old).toContainText(/合いません/);
  await expect(page.getByRole("row").filter({ hasText: "承認する人" })).toHaveCount(0);

  const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
  await page.getByRole("button", { name: /承認する/ }).click();
  expect((await res).status()).toBe(200);
  const now = rowOf(page, "承認する人");
  await expect(now, "承認した後に、承認の履歴に足されていない").toBeVisible();
  await expect(now).toContainText("#24");
  await expect(now).toContainText(/合います/);
  await expect(rowOf(page, "テスト担当")).toContainText(/合いません/);
});

// CL-08。今の中身として画面に出ているバージョン（約束の詳細では v4、レビューの画面では変更前の v1 と変更後の v4）は、選んで出たか
// 見分けられないので、その画面で出ていないバージョンだけを選ぶ。選ぶ前に出ていないことも確かめる
const pages = [
  { name: "約束の詳細の画面", open: async (page: Page) => page.goto(serve.url + "/claims/task-id-unique"), showsV1: false },
  { name: "レビューの画面", open: openReviewDetail, showsV1: true },
];

for (const pg of pages) {
  // @vanda:evidence claim-at-commit
  test(`${pg.name}で変更履歴の各コミットを選ぶとそのコミットの時点の約束の中身が出る`, async ({ page }) => {
    commitV3(repo);
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "文を直す", "#24");
    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();

    // その時点のレイヤーは、今の中身と同じく名前で出す。db はそのコミットで設定に足したので、名前がある
    const v3Marks: (string | RegExp)[] = [v3.statement, v3.rationale!, v3.condition!, v3.plan!, "データベース"];
    const choices: [string, (string | RegExp)[]][] = [
      ["理由と条件を足す", v3Marks],
      ["約束を変え、題の約束を足す", [v2Statement]],
    ];
    if (!pg.showsV1) choices.push(["はじめの約束を置く", [v1Statement]]);
    for (const [, marks] of choices) {
      for (const m of marks) await expect(page.getByText(m), `選ぶ前から ${m} が出ている`).toHaveCount(0);
    }
    for (const [subject, marks] of choices) {
      await choose(rowOf(page, subject));
      for (const m of marks) await expect(page.getByText(m).first(), `${subject} を選んでも ${m} が出ない`).toBeVisible();
    }
  });

  test(`${pg.name}で変更履歴のコミットを選ぶとその時点のレイヤーのうち今の設定にないものはIDのまま出る`, async ({ page }) => {
    // レイヤーの名前は今の設定から引く。v3 で足した db を、v4 で設定からも約束からも消す
    commitV3(repo);
    const cfg = readFileSync(join(repo.root, ".vanda/config.toml"), "utf8");
    repo.write(".vanda/config.toml", cfg.replace('[[layer]]\nid = "db"\nname = "データベース"\ndescription = "保存の制約"\n\n', ""));
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "文を直す", "#24");
    expect(readFileSync(join(repo.root, ".vanda/config.toml"), "utf8")).not.toContain('id = "db"');

    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();
    await expect(page.getByText(/\bdb\b/), "選ぶ前から db が出ている").toHaveCount(0);
    await choose(rowOf(page, "理由と条件を足す"));
    await expect(page.getByText(v3.statement).first()).toBeVisible();
    await expect(page.getByText(/\bdb\b/).first(), "今の設定にないレイヤー db が、ID のまま出ない（黙って消えている）").toBeVisible();
    await expect(page.getByText("データベース")).toHaveCount(0);
  });

  // @vanda:evidence claim-at-commit
  test(`${pg.name}で約束を変える前の承認の記録を選ぶと今の中身でなく承認を書いたコミットの時点の中身が出る`, async ({ page }) => {
    commitV3(repo);
    // v3 のときに画面で承認し、その後に v4 へ変える
    await openReviewDetail(page);
    const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
    await page.getByRole("button", { name: /承認する/ }).click();
    expect((await res).status()).toBe(200);
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "文を直す", "#24");

    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();
    for (const m of [v3.statement, v3.rationale!]) await expect(page.getByText(m)).toHaveCount(0);
    await choose(rowOf(page, "承認する人"));
    for (const m of [v3.statement, v3.rationale!, v3.condition!, v3.plan!]) {
      await expect(page.getByText(m, { exact: false }).first(), `承認の記録を選んでも、承認した時点の ${m} が出ない`).toBeVisible();
    }
    // #1 の承認は、#1 のコミットで書いたので v1 の中身
    if (!pg.showsV1) {
      await expect(page.getByText(v1Statement, { exact: true })).toHaveCount(0);
      await choose(rowOf(page, "テスト担当"));
      await expect(page.getByText(v1Statement, { exact: true }).first()).toBeVisible();
    }
  });

  test(`${pg.name}で変更履歴のコミットを開くとその時点の推測と対象が出る`, async ({ page }) => {
    // 前のバージョンは、対象が用語 task-old（後で消す）で、推測が 1 つ。後のバージョンで、対象を task-id に、推測を別のものに変える
    const at = (statement: string, subject: string, guess: string, oldTerm: boolean) => `[[term]]
id = "task-id"
name = "タスクの ID"
definition = "タスクを見分ける名前"
${oldTerm ? '\n[[term]]\nid = "task-old"\nname = "古いタスクの名前"\ndefinition = "前に使っていた、タスクを見分ける名前"\n' : ""}
[[claim]]
id = "task-id-unique"
subjects = ["${subject}"]
statement = "${statement}"
layers = ["model"]
guesses = ["${guess}"]

[[claim.evidence]]
stage = "implementation"
type = "scenario-test"
layers = ["model"]
`;
    const oldGuess = "消したタスクの ID は 30 日たてば使い回せるかもしれない";
    repo.write(".vanda/promises.toml", at("タスクの ID は、前の名前とも重ならない", "task-old", oldGuess, true));
    commit(repo, "推測を書く", "#24");
    repo.write(".vanda/promises.toml", at(v4.statement, "task-id", "ID は数字だけかもしれない", false));
    commit(repo, "推測と対象を変える", "#24");

    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();
    await expect(page.getByText(oldGuess), "開く前から前の推測が出ている").toHaveCount(0);
    await expect(page.getByText(/task-old|古いタスクの名前/), "開く前から前の対象が出ている").toHaveCount(0);
    await choose(rowOf(page, "推測を書く"));
    await expect(page.getByText("タスクの ID は、前の名前とも重ならない").first()).toBeVisible();
    await expect(page.getByText(oldGuess).first(), "その時点の推測が出ない").toBeVisible();
    await expect(page.getByText(/task-old|古いタスクの名前/).first(), "その時点の対象が出ない").toBeVisible();
  });

  test(`${pg.name}で2つ以上の時点を開くとそれぞれの中身が同時に出たままになり1つをたたんでもほかは開いたまま`, async ({ page }) => {
    commitV3(repo);
    await openReviewDetail(page);
    const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
    await page.getByRole("button", { name: /承認する/ }).click();
    expect((await res).status()).toBe(200);
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "文を直す", "#24");

    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();
    // 開くもの: 変更履歴の #1（v1）と #24 の最初のコミット（v2）、v3 のときの承認（v3）。レビューの画面では v1 の文が
    // 変更前として出ているので、出ている数が 1 つ増えるかで見る
    const opened: [string, string][] = [
      ["はじめの約束を置く", v1Statement],
      ["約束を変え、題の約束を足す", v2Statement],
      ["承認する人", v3.statement],
    ];
    const count = (text: string) => page.getByText(text, { exact: false }).count();
    const before = new Map<string, number>();
    for (const [, text] of opened) before.set(text, await count(text));
    for (const [rowText] of opened) await choose(rowOf(page, rowText));
    for (const [rowText, text] of opened) {
      await expect.poll(() => count(text), { message: `${rowText} の時点の中身が、ほかを開いた後に出ていない` }).toBe(before.get(text)! + 1);
    }
    await collapse(rowOf(page, "約束を変え、題の約束を足す"));
    await expect.poll(() => count(v2Statement), { message: "たたんだ時点の中身が消えない" }).toBe(before.get(v2Statement)!);
    for (const [rowText, text] of [opened[0], opened[2]]) {
      await expect.poll(() => count(text), { message: `1 つをたたむと、${rowText} の時点の中身も消えた` }).toBe(before.get(text)! + 1);
      await expect(rowOf(page, rowText).getByRole("button", { name: "その時点の中身をたたむ" })).toHaveAttribute("aria-expanded", "true");
    }
  });

  test(`${pg.name}で変更履歴と承認の履歴の行をクリックするとその時点の中身が開きもう一度クリックするとたたまり山形のボタンをクリックしても1回だけ切り替わる`, async ({ page }) => {
    commitV3(repo);
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "文を直す", "#24");
    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();

    // 変更履歴の v3 のコミットの行と、承認の履歴の #1 の承認（v1）の行。レビューの画面では v1 の文が変更前として出ているので、
    // 出ている数が 1 つ増えるか・戻るかで見る
    const count = (text: string) => page.getByText(text, { exact: false }).count();
    for (const [rowText, text] of [["理由と条件を足す", v3.statement], ["テスト担当", v1Statement]]) {
      const row = rowOf(page, rowText);
      const cell = row.getByRole("cell").filter({ hasText: rowText }).first();
      const toggle = row.getByRole("button", { name: /その時点の中身を(開く|たたむ)/ });
      const base = await count(text);
      await expect(toggle).toHaveAttribute("aria-expanded", "false");

      await cell.click();
      await expect(toggle, `${rowText} の行をクリックしても開かない`).toHaveAttribute("aria-expanded", "true");
      await expect.poll(() => count(text)).toBe(base + 1);
      await cell.click();
      await expect(toggle, `${rowText} の行をもう一度クリックしてもたたまない`).toHaveAttribute("aria-expanded", "false");
      await expect.poll(() => count(text)).toBe(base);

      // ボタンは行の中にあるので、行の動きと二重に切り替わると、クリックしても閉じたままになる
      await row.getByRole("button", { name: "その時点の中身を開く" }).click();
      await expect(toggle, `${rowText} のボタンをクリックすると、二重に切り替わって閉じたまま`).toHaveAttribute("aria-expanded", "true");
      await expect.poll(() => count(text)).toBe(base + 1);
      await row.getByRole("button", { name: "その時点の中身をたたむ" }).click();
      await expect(toggle, `${rowText} のボタンでたたむと、二重に切り替わって開いたまま`).toHaveAttribute("aria-expanded", "false");
      await expect.poll(() => count(text)).toBe(base);
    }
  });

  test(`${pg.name}の変更履歴と承認の履歴は新しいものが上に並ぶ`, async ({ page }) => {
    // 日時が同じときの順はまだ決まっていないので、日時をずらしたコミットと承認の記録だけの前後を見る。
    // buildRepo の #1 と #24 のコミットは今の日時なので、足すコミットはその 1 時間後と 2 時間後にする
    const later = (h: number) => new Date(Date.now() + h * 3600_000).toISOString();
    commitV3(repo, later(1));
    repo.write(".vanda/promises.toml", promisesAt(v4));
    // 承認の記録は、ファイルの中では日時の順にしない（一月の #1 の記録の後に、三月、二月）
    const agreements = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
    const rec = (who: string, at: string) =>
      agreement("claim", "task-id-unique", sha(claimJSON(v4.statement)), null, "#24")
        .replace("テスト担当", who)
        .replace("agreed_at = 2026-01-01T10:00:00+09:00", `agreed_at = ${at}`);
    repo.write(".vanda/agreements.toml", agreements + rec("三月に承認した人", "2026-03-01T10:00:00+09:00") + rec("二月に承認した人", "2026-02-01T10:00:00+09:00"));
    commit(repo, "文を直す", "#24", later(2));

    await pg.open(page);
    await expect(rowOf(page, "文を直す")).toBeVisible();
    const rows = await page.getByRole("row").allTextContents();
    const at = (text: string) => {
      const i = rows.findIndex((r) => r.includes(text));
      expect(i, `${text} の行がない`).toBeGreaterThanOrEqual(0);
      return i;
    };
    const oldest = Math.min(at("はじめの約束を置く"), at("約束を変え、題の約束を足す"));
    expect(at("文を直す"), "変更履歴の 2 時間後のコミットが、1 時間後のコミットより下にある").toBeLessThan(at("理由と条件を足す"));
    expect(at("理由と条件を足す"), "変更履歴の 1 時間後のコミットが、はじめのコミットより下にある").toBeLessThan(oldest);
    expect(at("三月に承認した人"), "承認の履歴の三月の承認が、二月の承認より下にある").toBeLessThan(at("二月に承認した人"));
    expect(at("二月に承認した人"), "承認の履歴の二月の承認が、一月の承認より下にある").toBeLessThan(at("テスト担当"));
  });

  test(`${pg.name}で承認を書いたコミットの時点に約束リストが読めない承認の記録を選ぶと理由が出る`, async ({ page }) => {
    commitV3(repo);
    // 承認の記録を足したコミットで、約束リストの形を壊す。次のコミットで直す
    const agreements = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
    repo.write(".vanda/agreements.toml", agreements + agreement("claim", "task-id-unique", sha(claimJSON(v3.statement)), null, "#24").replace("テスト担当", "壊れた時に承認した人"));
    repo.write(".vanda/promises.toml", promisesAt(v3) + "\n[[claim]\n");
    commit(repo, "承認を書き約束リストを壊す", "#24");
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "文を直す", "#24");

    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();
    await expect(page.getByText(/promises\.toml/)).toHaveCount(0);
    await choose(rowOf(page, "壊れた時に承認した人"));
    await expect(page.getByText(/promises\.toml/).first(), "その時点の約束リストが読めない理由が出ない").toBeVisible();
  });
}

// RV-17。取り消しのボタンの名前と API の形はまだ決まっていないので、ボタンは「取り消」を含む名前で探し、
// 取り消しが済んだかはコミットが増えたことで待つ。API で断られることは、API の形が docs/formats.md に書かれてから確かめる
const unagreeButton = (page: Page) => page.getByRole("button", { name: /取り消/ });
const commitCount = () => Number(repo.git("rev-list", "--count", "HEAD"));
const nonBlank = (t: string) => t.split("\n").filter((l) => l.trim() !== "");

// withoutRecord は、承認の記録のファイルの空行でない行から、key と story の記録 1 件（[[agreement]] から、コメント・次の記録の前まで）を除く
function withoutRecord(text: string, key: string, story: string): string[] {
  const lines = nonBlank(text);
  const out: string[] = [];
  let block: string[] = [];
  const flush = () => {
    const hit = block.some((l) => l.includes(`key = "${key}"`)) && block.some((l) => l.includes(`story = "${story}"`));
    if (!hit) out.push(...block);
    block = [];
  };
  for (const l of lines) {
    if (l.startsWith("[[agreement]]")) flush();
    if (l.startsWith("#")) {
      flush();
      out.push(l);
    } else if (block.length > 0 || l.startsWith("[[agreement]]")) block.push(l);
    else out.push(l);
  }
  flush();
  return out;
}

// CL-04。消した後に同じ ID で作り直し、推測だけを変え、横のブランチで変えてマージする。日時は、どのコミットも buildRepo のコミットより後にずらす
// @vanda:evidence claim-detail-origin
test("約束の詳細の変更履歴には作り直した後の約束のフィンガープリントが変わったマージでないコミットだけが新しい順に出て消すまでのコミットと推測だけを変えたコミットとマージのコミットは出ない", async ({ page }) => {
  const later = (h: number) => new Date(Date.now() + h * 3600_000).toISOString();
  const first = repo.git("rev-parse", "main");
  const storyChange = repo.git("rev-parse", "story-24~1");
  const termOnly = `[[term]]\nid = "task-id"\nname = "タスクの ID"\ndefinition = "タスクを見分ける名前"\n`;
  const withGuess = (text: string) => text.replace(`layers = ["model"]\n`, `layers = ["model"]\nguesses = ["作り直した後の推測"]\n`);
  repo.write(".vanda/promises.toml", termOnly);
  const deleted = commit(repo, "約束を消す", "#24", later(1));
  repo.write(".vanda/promises.toml", promisesAt(v4));
  const recreated = commit(repo, "約束を作り直す", "#24", later(2));
  repo.write(".vanda/promises.toml", withGuess(promisesAt(v4)));
  const guessed = commit(repo, "作り直した約束に推測を足す", "#30", later(3));
  repo.git("switch", "-q", "-c", "side");
  repo.write(".vanda/promises.toml", withGuess(promisesAt({ ...v4, plan: "横のブランチで変えた予定" })));
  const side = commit(repo, "横のブランチでエビデンスの予定を変える", "#31", later(4));
  repo.git("switch", "-q", "story-24");
  repo.write(".vanda/promises.toml", withGuess(promisesAt({ ...v4, rationale: "作り直した後に変えた理由" })));
  const changed = commit(repo, "作り直した約束の理由を変える", "#24", later(5));
  execFileSync("git", ["-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "merge", "-q", "--no-ff", "-m", "マージする\n\nStory: #24", "side"], {
    cwd: repo.root,
    env: { ...repo.env, GIT_AUTHOR_DATE: later(6), GIT_COMMITTER_DATE: later(6) },
  });
  const merged = repo.git("rev-parse", "HEAD");
  expect(repo.git("rev-list", "--parents", "-n", "1", "HEAD").split(" ").length, "マージのコミットができていない").toBe(3);
  // マージした後の約束リストに、両方のブランチの変更が入っている（マージのコミットでもフィンガープリントが変わる）
  const mergedPromises = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  expect(mergedPromises).toContain("横のブランチで変えた予定");
  expect(mergedPromises).toContain("作り直した後に変えた理由");

  await page.goto(serve.url + "/claims/task-id-unique");
  await expect(page.getByRole("heading", { name: "変更履歴" })).toBeVisible();
  const origin = page.getByRole("table").first();
  await expect(origin.getByText("約束を作り直す")).toBeVisible();
  const rows = await origin.locator("tbody tr").allTextContents();
  const want = [
    [changed, "作り直した約束の理由を変える"],
    [side, "横のブランチでエビデンスの予定を変える"],
    [recreated, "約束を作り直す"],
  ];
  expect(rows.length, `変更履歴の行: ${rows.join(" / ")}`).toBe(want.length);
  want.forEach(([hash, subject], i) => {
    expect(rows[i], `変更履歴の ${i + 1} 行目`).toContain(subject);
    expect(rows[i], `変更履歴の ${i + 1} 行目`).toContain(hash.slice(0, 7));
  });
  const all = rows.join("\n");
  for (const [hash, subject] of [
    [merged, "マージする"],
    [guessed, "作り直した約束に推測を足す"],
    [deleted, "約束を消す"],
    [storyChange, "約束を変え、題の約束を足す"],
    [first, "はじめの約束を置く"],
  ]) {
    expect(all, `変更履歴に ${subject} が出ている`).not.toContain(subject);
    expect(all, `変更履歴に ${subject} のコミットが出ている`).not.toContain(hash.slice(0, 7));
  }
});

// 承認の記録のファイルに、コメントと空行を混ぜる（取り消しで、ほかの行が変わらないことを見るため）
function decorateAgreements(r: Repo) {
  const text = readFileSync(join(r.root, ".vanda/agreements.toml"), "utf8");
  const parts = text.split("\n[[agreement]]");
  r.write(".vanda/agreements.toml", "# 承認の記録（E2E の例）\n\n" + parts[0] + "\n[[agreement]]" + parts[1] + "\n# 用語の次は約束\n\n\n[[agreement]]" + parts.slice(2).join("\n[[agreement]]") + "\n# 終わりのコメント\n");
  r.write("README.md", "# 例\n");
  commit(r, "承認の記録にコメントを足す", "#24");
}

// @vanda:evidence revoke-on-screen
test("承認済の約束で承認を取り消すとストーリーの今の中身への承認の記録だけが1つのコミットで消え提案中に戻りチェックで止まりもう一度承認すると通る", async ({ page }) => {
  decorateAgreements(repo);
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await select(page, "task-id-unique");
  await expect(page.getByRole("button", { name: /承認する/ })).toBeVisible();
  await expect(unagreeButton(page), "提案中のものに取り消しが出ている").toHaveCount(0);
  await agreeAll(page, items);
  const passed = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(passed.code, passed.out).toBe(0);

  const agreementsBefore = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
  const promisesBefore = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  const countBefore = commitCount();
  // 承認と関係のない書きかけの変更（追跡しているファイルと、追跡していないファイル）
  repo.write("README.md", "# 例\n書きかけ\n");
  repo.write("メモ.txt", "書きかけ\n");

  await select(page, "task-id-unique");
  await unagreeButton(page).click();
  await expect.poll(commitCount, { message: "取り消しがコミットにならない" }).toBe(countBefore + 1);

  expect(repo.git("log", "-1", "--format=%B"), "コミットのメッセージにストーリーの ID がない").toContain("#24");
  expect(repo.git("diff", "--name-only", "HEAD~1", "HEAD").split("\n"), "承認の記録のファイルのほかに変わったものがある").toEqual([".vanda/agreements.toml"]);
  const agreementsAfter = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
  // #24 の task-id-unique への記録だけが消え、#1 の記録とコメントは同じ順で残る
  expect(nonBlank(agreementsAfter)).toEqual(withoutRecord(agreementsBefore, "task-id-unique", "#24"));
  expect(nonBlank(agreementsAfter).length).toBeLessThan(nonBlank(agreementsBefore).length);
  for (const c of ["# 承認の記録（E2E の例）", "# 用語の次は約束", "# 終わりのコメント"]) expect(agreementsAfter).toContain(c);
  // フラグは戻らない
  const promisesAfter = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  expect(promisesAfter).toBe(promisesBefore);
  expect(promisesAfter).not.toContain("ai-proposal");
  // 書きかけの変更は残り、コミットに入らない
  // git() は前後の空白を落とすので、行ごとに空白を落として比べる
  expect(repo.git("-c", "core.quotePath=false", "status", "--porcelain", "--untracked-files=all").split("\n").map((l) => l.trim()).sort()).toEqual(["?? メモ.txt", "M README.md"]);

  await expect(listIcon(page, "task-id-unique", "提案中")).toBeVisible();
  // 自分で取り消したものは、RV-14 の「承認が失効した」としては出さない
  await expect(page.getByText(/承認が失効した/)).toHaveCount(0);
  await expect(page.getByRole("img", { name: "承認が失効した" })).toHaveCount(0);
  const stopped = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(stopped.code, "取り消した後も承認のチェックが通る").toBe(1);
  expect(stopped.out).toContain("task-id-unique");

  await agreeAll(page, ["task-id-unique"]);
  const again = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(again.code, again.out).toBe(0);
});

test("承認の記録のファイルに書きかけの変更があると承認の取り消しを断り理由を出しコミットしない", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await agreeAll(page, ["task-id-unique"]);
  const dirty = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8") + "# 書きかけ\n";
  repo.write(".vanda/agreements.toml", dirty);
  const countBefore = commitCount();
  await expect(page.getByText(/agreements\.toml|承認の記録/)).toHaveCount(0);

  await select(page, "task-id-unique");
  await unagreeButton(page).click();
  await expect(page.getByText(/agreements\.toml|承認の記録/).first(), "断った理由が出ない").toBeVisible();
  expect(commitCount()).toBe(countBefore);
  expect(readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8")).toBe(dirty);
  await expect(listIcon(page, "task-id-unique", "承認済")).toBeVisible();
});

// 取り消せるのは、今の git の人（buildRepo の user.name「承認する人」・user.email「po@example.com」）が今のブランチで書いた
// 承認の記録だけ。誰が × どこで書いたか、の組み合わせを全部回す。ほかの人は、名前だけ・メールだけ・両方の違いを分ける
const writers = [
  { name: "自分", user: { name: "承認する人", email: "po@example.com" }, self: true },
  { name: "名前だけ違うほかの人", user: { name: "ほかの人", email: "po@example.com" }, self: false },
  { name: "メールだけ違うほかの人", user: { name: "承認する人", email: "other@example.com" }, self: false },
  { name: "名前もメールも違うほかの人", user: { name: "ほかの人", email: "other@example.com" }, self: false },
];
const places = [
  { name: "今のブランチ", onBase: false },
  { name: "ベースブランチ", onBase: true },
];

for (const w of writers) {
  for (const pl of places) {
    const can = w.self && !pl.onBase;
    test(`${w.name}が${pl.name}で書いた承認の記録は${can ? "取り消せて記録が消えた1つのコミットになる" : "取り消せず取り消しのボタンが出ない"}`, async ({ page }) => {
      // task-id-unique のフラグを外し、今の中身（#24 の文）への承認の記録を手で書く。ベースブランチなら main に書いて
      // story-24 に取り込み、マージベースより前にする
      repo.write(".vanda/promises.toml", promises(v2Statement, false, true));
      commit(repo, "フラグを外す", "#24");
      const record = agreement("claim", "task-id-unique", sha(claimJSON(v2Statement)), null, "#24")
        .replace('user = { name = "テスト担当", email = "tester@example.com" }', `user = { name = "${w.user.name}", email = "${w.user.email}" }`);
      if (pl.onBase) {
        repo.git("switch", "-q", "main");
        repo.write(".vanda/agreements.toml", readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8") + record);
        commit(repo, "承認の記録を書く", "#24");
        repo.git("switch", "-q", "story-24");
        repo.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "merge", "-q", "--no-edit", "main");
      } else {
        repo.write(".vanda/agreements.toml", readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8") + record);
        commit(repo, "承認の記録を書く", "#24");
      }
      const agreementsBefore = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
      const promisesBefore = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
      const countBefore = commitCount();

      await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
      await select(page, "task-id-unique");
      await expect(page.getByText("今の中身を承認済みです。").first(), "手で書いた承認の記録が今の中身への承認になっていない").toBeVisible();
      if (!can) {
        await expect(unagreeButton(page)).toHaveCount(0);
        expect(commitCount()).toBe(countBefore);
        return;
      }
      await unagreeButton(page).click();
      await expect.poll(commitCount, { message: "取り消しがコミットにならない" }).toBe(countBefore + 1);
      expect(repo.git("diff", "--name-only", "HEAD~1", "HEAD").split("\n")).toEqual([".vanda/agreements.toml"]);
      expect(nonBlank(readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8"))).toEqual(withoutRecord(agreementsBefore, "task-id-unique", "#24"));
      expect(readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8"), "取り消しでフラグが戻った").toBe(promisesBefore);
      await expect(listIcon(page, "task-id-unique", "提案中")).toBeVisible();
    });
  }
}

// RV-13: レビュー単位がブランチのときのストーリーの決め方。選ぶ欄の選択肢は、ストーリーの ID だけを数える
// （まだ選んでいないことを示す選択肢などは数えない）。一覧を読み終えるまでは、すべてのストーリーが選択肢に出ることが
// あるので、選択肢は落ち着くまで待って比べる
const storyField = (page: Page) => page.getByRole("combobox", { name: "ストーリー", exact: true });
async function storyChoices(page: Page): Promise<string[]> {
  const texts = await storyField(page).locator("option").allTextContents();
  return texts.flatMap((t) => t.match(/#\d+/g) ?? []);
}
async function agreeTaskIdUnique(page: Page) {
  await select(page, "task-id-unique");
  const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
  await page.getByRole("button", { name: /承認する/ }).click();
  expect((await res).status()).toBe(200);
}

// @vanda:evidence review-story-choice
test("レビュー単位がブランチでブランチのコミットからストーリーが1つに決まるときは選ぶ欄を出さずに示しそのストーリーで承認できる", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main");
  await expect(page.getByText(/ストーリー\s*[:：]\s*#24/).first(), "決まったストーリーが示されない").toBeVisible();
  await expect(storyField(page)).toHaveCount(0);
  await agreeTaskIdUnique(page);
  expect(repo.git("log", "-1", "--format=%s")).toContain("#24 で");
});

// @vanda:evidence review-story-choice
test("レビュー単位がブランチでブランチに2つ以上のストーリーのコミットがあるときはそのブランチのストーリーだけを1度ずつ選ばせる", async ({ page }) => {
  repo.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "--allow-empty", "-m", "直す\n\nStory: #30");
  await page.goto(serve.url + "/?view=branch&base=main");
  await expect(storyField(page)).toBeVisible();
  // #1 は main のコミットにしかない。#24 は 2 つのコミットにある
  await expect.poll(async () => (await storyChoices(page)).sort()).toEqual(["#24", "#30"]);
});

// @vanda:evidence review-story-choice
test("レビュー単位がブランチでストーリーのIDを持つコミットがないときはすべてのストーリーから1度ずつ選ばせる", async ({ page }) => {
  // story-24 から ID のないコミットだけのブランチを作り、story-24 と比べる（設定の既定のベースブランチ main とは別）
  repo.git("switch", "-q", "-c", "plain");
  repo.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "--allow-empty", "-m", "ID のないコミット");
  await page.goto(serve.url + "/?view=branch&base=story-24");
  await expect(storyField(page)).toBeVisible();
  await expect.poll(async () => (await storyChoices(page)).sort()).toEqual(["#1", "#24"]);
});

test("レビュー単位がブランチでURLのstoryでストーリーを名指ししたときはそれを使う", async ({ page }) => {
  repo.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "--allow-empty", "-m", "直す\n\nStory: #30");
  await page.goto(serve.url + "/?view=branch&base=main&story=%2330");
  await agreeTaskIdUnique(page);
  expect(repo.git("log", "-1", "--format=%s"), "名指ししたストーリーで承認していない").toContain("#30 で");
});

test("レビュー単位がストーリーのときはすべてのストーリーから1度ずつ選ばせる", async ({ page }) => {
  repo.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "--allow-empty", "-m", "直す\n\nStory: #30");
  await page.goto(serve.url + "/?view=story&story=%2324");
  await expect(storyField(page)).toBeVisible();
  await expect.poll(async () => (await storyChoices(page)).sort()).toEqual(["#1", "#24", "#30"]);
});

test("レビュー単位がブランチでベースブランチを既定から変えるとそのベースブランチとのマージベースより後のストーリーで決まる", async ({ page }) => {
  // rel は #24 のコミットの後から分かれたことにする。rel と比べると、ブランチのストーリーは #30 だけ。
  // #30 で task-id-unique を変え、rel と比べても一覧に出るようにする
  repo.git("branch", "rel", "story-24");
  const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  repo.write(".vanda/promises.toml", text.replace("すべてのタスクを通して重ならない", "どのタスクとも重ならない"));
  commit(repo, "直す", "#30");
  await page.goto(serve.url + "/?view=branch&base=main");
  await expect(storyField(page)).toBeVisible();
  await expect.poll(async () => (await storyChoices(page)).sort()).toEqual(["#24", "#30"]);

  await page.getByRole("combobox", { name: "ベースブランチ" }).selectOption("rel");
  await expect(page.getByText(/ストーリー\s*[:：]\s*#30/).first(), "rel と比べたときのストーリー #30 が示されない").toBeVisible();
  await expect(storyField(page)).toHaveCount(0);
  await agreeTaskIdUnique(page);
  expect(repo.git("log", "-1", "--format=%s")).toContain("#30 で");
});

// RV-08: エビデンスが揃っているかは、各エビデンスのアイコンの名前（aria-label）で示す。揃っている・ない・
// アノテーションでは確かめない、の 3 つを全部回す
test("エビデンスの予定と実際の揃っているかは揃っているとないとアノテーションでは確かめないをアイコンの名前で示す", async ({ page }) => {
  // シナリオテストはアノテーションがあり揃っている。モデル検査はアノテーションがなくない。人のレビューはアノテーションで確かめない
  repo.write("e2e/add-task.txtar", "# シナリオ\n# @vanda:evidence task-id-unique\n! exec vanda-task add a\n-- a.txt --\na\n");
  const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  repo.write(
    ".vanda/promises.toml",
    text.replace(
      '[[claim.evidence]]\nstage = "implementation"\ntype = "scenario-test"\nlayers = ["model"]\n',
      '[[claim.evidence]]\nstage = "implementation"\ntype = "scenario-test"\nlayers = ["model"]\n\n[[claim.evidence]]\nstage = "plan"\ntype = "model-check"\nlayers = ["model"]\n\n[[claim.evidence]]\nstage = "release"\ntype = "human-review"\nlayers = ["model"]\n',
    ),
  );
  commit(repo, "エビデンスの予定を足す", "#24");
  await openReviewDetail(page);
  for (const [type, name] of [
    ["シナリオテスト", "揃っています"],
    ["モデル検査", "ありません"],
    ["人のレビュー", "アノテーションでは確かめません"],
  ]) {
    const group = typeGroup(page, type);
    await expect(group.getByRole("img", { name }), `${type} のエビデンスに「${name}」のアイコンがない`).toBeVisible();
    await expect(group.getByRole("img", { name }), `${type} のエビデンスのアイコンの title が名前と違う`).toHaveAttribute("title", name);
  }
  // 約束の詳細の画面のエビデンスも、同じアイコンで示す（3 つのエビデンスに 1 つずつ）
  await page.getByRole("link", { name: "約束の詳細を開く" }).click();
  await expect(page.getByRole("heading", { name: "シナリオテスト", exact: true }).first()).toBeVisible();
  for (const name of ["揃っています", "ありません", "アノテーションでは確かめません"]) {
    await expect(page.getByRole("img", { name, exact: true }), `約束の詳細の画面に「${name}」のアイコンが 1 つでない`).toHaveCount(1);
    await expect(page.getByRole("img", { name, exact: true })).toHaveAttribute("title", name);
  }
  await expect(page.getByText("エビデンスがありません")).toHaveCount(0);
});

// エビデンスは、タイプの見出しの下にまとまる。そのまとまり（見出しの親）の中を見る
const typeGroup = (page: Page, type: string) => page.getByRole("heading", { name: type, exact: true }).first().locator("xpath=..");

test("エビデンスの予定と実際の詳細は長い文も改行も省略せずに全部出し幅400pxでページが横にスクロールしない", async ({ page }) => {
  const lines = [
    "TLA+ のモデルで、タスクを足す・消す・足し直すを 3 回まで繰り返し、どの順でも ID が重ならないことを確かめる予定。消したタスクの ID を使い回さないことも、同じモデルで確かめる。",
    "定数の範囲は、タスクが 3 つまで、操作が 6 歩まで。",
    "置き場所の候補: spec/TaskLifecycleWithDeletionAndReAddition.tla",
  ];
  const detail = lines.join("\n");
  const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  repo.write(
    ".vanda/promises.toml",
    text.replace(
      '[[claim.evidence]]\nstage = "implementation"\ntype = "scenario-test"\nlayers = ["model"]\n',
      `[[claim.evidence]]\nstage = "implementation"\ntype = "scenario-test"\nlayers = ["model"]\n\n[[claim.evidence]]\nstage = "plan"\ntype = "model-check"\nlayers = ["model"]\ndetail = ${JSON.stringify(detail)}\n`,
    ),
  );
  commit(repo, "長い詳細を書く", "#24");
  await page.setViewportSize({ width: 400, height: 900 });
  await openReviewDetail(page);
  const group = typeGroup(page, "モデル検査");
  await expect(group, "モデル検査の見出しのまとまりがない").toBeVisible();
  const cell = group.getByText(lines[0].slice(0, 20)).first();
  await expect(cell).toBeVisible();
  // 見えている文字（innerText）に、全部の文と改行がある
  expect((await cell.innerText()).trim(), "詳細が省略されたか、改行が保たれていない").toBe(detail);
  // 欄の中で折り返し、はみ出して隠れた所がない
  const overflow = await cell.evaluate((el) => ({ w: el.scrollWidth - el.clientWidth, h: el.scrollHeight - el.clientHeight }));
  expect(overflow.w, "詳細の欄の中で、横にはみ出している").toBeLessThanOrEqual(1);
  expect(overflow.h, "詳細の欄の中で、縦にはみ出して隠れている").toBeLessThanOrEqual(1);
  const page_ = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, view: window.innerWidth }));
  expect(page_.scroll, "幅 400px で、ページが横にスクロールする").toBeLessThanOrEqual(page_.view);
});

// 一覧の 1 件は、枠のどこをクリックしても選ばれる（番号・種類・状態のアイコン・フラグのアイコン・枠の端）
test("変わったものの一覧の1件は名前のほかに番号や種類や状態のアイコンやフラグのアイコンや枠の端をクリックしても選ばれる", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  const detailOf = (id: string) => expect(page.getByRole("link", { name: "約束の詳細を開く" }), `${id} が選ばれていない`).toHaveAttribute("href", new RegExp(`/claims/${id}`));
  // task-title-required は 2 件目で、提案中のアイコンと AI 案のアイコンを持つ
  // 種類とアイコンは名前のボタンの外にあるので、1 件の枠（listitem）の中でクリックする
  const frame = page.getByRole("listitem").filter({ hasText: "task-title-required" }).first();
  await expect(frame).toBeVisible();
  const spots: [string, () => Promise<void>][] = [
    ["番号", () => frame.getByText("2", { exact: true }).first().click()],
    ["種類", () => frame.getByText("約束", { exact: true }).click()],
    ["状態のアイコン", () => frame.getByRole("img", { name: "提案中", exact: true }).click()],
    ["フラグのアイコン", () => frame.getByRole("img", { name: "AI 案", exact: true }).click()],
    ["枠の左上の端", () => frame.click({ position: { x: 2, y: 2 } })],
    ["枠の右下の端", async () => {
      const box = (await frame.boundingBox())!;
      await frame.click({ position: { x: box.width - 3, y: box.height - 3 } });
    }],
  ];
  for (const [spot, click] of spots) {
    await select(page, "task-id-unique");
    await detailOf("task-id-unique");
    await click();
    await expect(page.getByRole("link", { name: "約束の詳細を開く" }), `${spot}をクリックしても task-title-required が選ばれない`).toHaveAttribute("href", /\/claims\/task-title-required/);
  }
});

// CL-02: エビデンスは、ステージ（計画→実装→リリース→運用）の見出しの下を、タイプの見出しで分けて示す。
// 見出しは、ページの見出しのうちステージとタイプの名前のものだけを、出てくる順に見る
const stageNames: [string, string][] = [["plan", "計画"], ["implementation", "実装"], ["release", "リリース"], ["operation", "運用"]];
const typeNames: Record<string, string> = {
  "model-check": "モデル検査", proof: "証明", "cross-check": "クロスチェック", "random-check": "ランダムチェック",
  "scenario-test": "シナリオテスト", "unit-test": "単体テスト", "interface-test": "インターフェースのテスト",
  "concurrency-test": "同時実行のテスト", guardrail: "ガードレール", sensor: "センサー", "human-review": "人のレビュー",
};
// 書く順はステージの順にしない
const allStagesEvidence: [string, string][] = [
  ["operation", "sensor"], ["implementation", "unit-test"], ["plan", "model-check"], ["release", "human-review"],
  ["implementation", "scenario-test"], ["implementation", "guardrail"], ["plan", "proof"],
];
const twoStagesEvidence: [string, string][] = [["implementation", "scenario-test"], ["plan", "model-check"]];

function promisesWithEvidence(statement: string, ev: [string, string][]) {
  return `[[term]]\nid = "task-id"\nname = "タスクの ID"\ndefinition = "タスクを見分ける名前"\n\n[[claim]]\nid = "task-id-unique"\nsubjects = ["task-id"]\nstatement = "${statement}"\nlayers = ["model"]\n` +
    ev.map(([stage, type]) => `\n[[claim.evidence]]\nstage = "${stage}"\ntype = "${type}"\nlayers = ["model"]\n`).join("");
}

async function evidenceHeadings(page: Page): Promise<string[]> {
  const names = new Set([...stageNames.map(([, n]) => n), ...Object.values(typeNames)]);
  const texts = (await page.getByRole("heading").allTextContents()).map((t) => t.trim());
  return texts.filter((t) => names.has(t));
}

// 見出しの並びが、ev のステージを決まった順に、その下にそのステージのタイプだけを並べたものか確かめる
function checkHeadings(got: string[], ev: [string, string][], where: string) {
  const stagesHere = stageNames.filter(([id]) => ev.some(([s]) => s === id));
  // ステージの見出しは、エビデンスのあるものだけが決まった順に出る
  expect(got.filter((t) => stageNames.some(([, n]) => n === t)), `${where}: ステージの見出しの並び`).toEqual(stagesHere.map(([, n]) => n));
  for (const [i, [id, name]] of stagesHere.entries()) {
    const from = got.indexOf(name);
    const to = i + 1 < stagesHere.length ? got.indexOf(stagesHere[i + 1][1]) : got.length;
    const under = got.slice(from + 1, to).sort();
    const want = [...new Set(ev.filter(([s]) => s === id).map(([, t]) => typeNames[t]))].sort();
    expect(under, `${where}: ステージ「${name}」の下のタイプの見出し`).toEqual(want);
  }
  // エビデンスのないタイプの見出しは出ない
  const typesHere = new Set(ev.map(([, t]) => typeNames[t]));
  for (const n of Object.values(typeNames)) if (!typesHere.has(n)) expect(got, `${where}: エビデンスのないタイプ「${n}」の見出しが出ている`).not.toContain(n);
}

for (const pg of pages) {
  for (const [label, ev] of [["4つのステージ全部", allStagesEvidence], ["計画と実装のステージだけ", twoStagesEvidence]] as const) {
    test(`${pg.name}のエビデンスは${label}にエビデンスがあるときステージの見出しを決まった順に並べその下にそのステージのタイプの見出しだけを出す`, async ({ page }) => {
      repo.write(".vanda/promises.toml", promisesWithEvidence(v4.statement, [...ev]));
      commit(repo, "エビデンスの予定を書く", "#24");
      await pg.open(page);
      await expect(page.getByText(v4.statement).first()).toBeVisible();
      await expect.poll(() => evidenceHeadings(page), { message: "エビデンスの見出しが出ない" }).not.toEqual([]);
      checkHeadings(await evidenceHeadings(page), [...ev], pg.name);
    });
  }
}

test("約束の詳細の画面で変更履歴のコミットを開くとその時点のエビデンスもステージとタイプの見出しで分けて出る", async ({ page }) => {
  repo.write(".vanda/promises.toml", promisesWithEvidence(v3.statement, allStagesEvidence));
  commit(repo, "エビデンスの予定を書く", "#24");
  repo.write(".vanda/promises.toml", promisesWithEvidence(v4.statement, twoStagesEvidence));
  commit(repo, "エビデンスの予定を減らす", "#24");
  await page.goto(serve.url + "/claims/task-id-unique");
  await expect(page.getByText(v4.statement).first()).toBeVisible();
  await expect.poll(() => evidenceHeadings(page)).not.toEqual([]);
  const before = await evidenceHeadings(page);
  checkHeadings(before, twoStagesEvidence, "今の中身");
  await choose(rowOf(page, "エビデンスの予定を書く"));
  await expect(page.getByText(v3.statement).first()).toBeVisible();
  // 開いたその時点の中身の分だけ、見出しが増える。今の中身にないリリース・運用の見出しが、その時点の中身から出る
  await expect.poll(async () => (await evidenceHeadings(page)).filter((t) => ["リリース", "運用"].includes(t)).sort()).toEqual(["リリース", "運用"].sort());
  const after = await evidenceHeadings(page);
  checkHeadings(after.slice(before.length), allStagesEvidence, "その時点の中身");
});

// RV-18: 約束の詳細の画面とファイルの画面でも、ストーリーのレビューから移ったときは承認・取り消しができる
const annotatedScenario = "# シナリオ\n# @vanda:evidence task-id-unique\n! exec vanda-task add a\n-- a.txt --\na\n";
const recordsOf = (key: string, story: string) => {
  const text = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
  return text.split("[[agreement]]").filter((b) => b.includes(`key = "${key}"`) && b.includes(`story = "${story}"`)).length;
};
async function agreeHere(page: Page, url: RegExp) {
  const count = commitCount();
  const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
  await page.getByRole("button", { name: /承認する/ }).click();
  expect((await res).status()).toBe(200);
  await expect.poll(commitCount, { message: "承認が 1 つのコミットにならない" }).toBe(count + 1);
  await expect(page, "承認した後に、画面を移った").toHaveURL(url);
  await expect(page.getByText("承認済").first(), "承認した後に、承認済と出ない").toBeVisible();
}

async function revokeHere(page: Page, url: RegExp) {
  const count = commitCount();
  await unagreeButton(page).click();
  await expect.poll(commitCount, { message: "取り消しが 1 つのコミットにならない" }).toBe(count + 1);
  await expect(page, "取り消した後に、画面を移った").toHaveURL(url);
  await expect(page.getByText("提案中").first(), "取り消した後に、提案中と出ない").toBeVisible();
}

// RV-18 の画面の移り方は、クエリのない / から始めて画面の操作だけで移る。ストーリーは URL で名指しせず、
// ブランチのコミットから決まる（#24）
// @vanda:evidence agree-from-detail-pages
// @vanda:evidence agree-from-detail-pages-stay
test("レビューの画面から約束の詳細へ移って承認すると1コミットで記録が増えその画面で承認済になり履歴に足され戻ると一覧でも承認済でその画面で取り消せる", async ({ page }) => {
  await page.goto(serve.url + "/");
  await select(page, "task-id-unique");
  await page.getByRole("link", { name: "約束の詳細を開く" }).click();
  await expect(page).toHaveURL(/\/claims\/task-id-unique/);
  expect(recordsOf("task-id-unique", "#24")).toBe(0);
  await agreeHere(page, /\/claims\/task-id-unique/);
  expect(recordsOf("task-id-unique", "#24"), "承認の記録が 1 件増えていない").toBe(1);
  expect(repo.git("diff", "--name-only", "HEAD~1", "HEAD").split("\n").sort()).toEqual([".vanda/agreements.toml", ".vanda/promises.toml"]);
  const row = rowOf(page, "承認する人");
  await expect(row, "承認の履歴に足されていない").toBeVisible();
  await expect(row).toContainText("#24");
  await expect(row).toContainText(/合います/);

  await page.getByRole("link", { name: "ストーリーのレビューへ戻る" }).click();
  await expect(listIcon(page, "task-id-unique", "承認済"), "戻った一覧で承認済になっていない").toBeVisible();

  await page.getByRole("link", { name: "約束の詳細を開く" }).click();
  await revokeHere(page, /\/claims\/task-id-unique/);
  expect(recordsOf("task-id-unique", "#24"), "取り消しても承認の記録が残っている").toBe(0);
  await page.getByRole("link", { name: "ストーリーのレビューへ戻る" }).click();
  await expect(listIcon(page, "task-id-unique", "提案中")).toBeVisible();
});

// @vanda:evidence agree-from-detail-pages
// @vanda:evidence agree-from-detail-pages-stay
test("エビデンスのファイルのリンクからファイルの画面へ移って承認すると1コミットで記録が増えその画面で承認済になり戻ると一覧でも承認済でその画面で取り消せる", async ({ page }) => {
  repo.write("e2e/add-task.txtar", annotatedScenario);
  commit(repo, "シナリオをエビデンスにする", "#24");
  const fp = sha(annotatedScenario);
  await page.goto(serve.url + "/");
  await select(page, "task-id-unique");
  await page.getByRole("link", { name: /e2e\/add-task\.txtar:1/ }).first().click();
  await expect(page).toHaveURL(/\/files\?/);
  expect(recordsOf(fp, "#24")).toBe(0);
  await agreeHere(page, /\/files\?/);
  expect(recordsOf(fp, "#24"), "ファイルへの承認の記録が 1 件増えていない").toBe(1);
  expect(repo.git("diff", "--name-only", "HEAD~1", "HEAD")).toBe(".vanda/agreements.toml");

  await page.getByRole("link", { name: "ストーリーのレビューへ戻る" }).click();
  await expect(listIcon(page, "e2e/add-task.txtar", "承認済"), "戻った一覧でファイルが承認済になっていない").toBeVisible();

  await select(page, "task-id-unique");
  await page.getByRole("link", { name: /e2e\/add-task\.txtar:1/ }).first().click();
  await revokeHere(page, /\/files\?/);
  expect(recordsOf(fp, "#24"), "取り消してもファイルへの承認の記録が残っている").toBe(0);
});

// @vanda:evidence agree-from-detail-pages
// @vanda:evidence agree-from-detail-pages-stay
test("レビューの画面の一覧のファイルからファイルの画面へ移って承認すると1コミットで記録が増えその画面で承認済になり戻ると一覧でも承認済でその画面で取り消せる", async ({ page }) => {
  const fp = sha(scenarioStory);
  await page.goto(serve.url + "/");
  await select(page, "e2e/add-task.txtar");
  // 1 件の詳細の中の、ファイルの画面へのリンク（名前は決めていないので、行き先で探す）
  const toFile = page.locator('a[href*="/files"]').first();
  await expect(toFile, "一覧のファイルの 1 件の詳細に、ファイルの画面へのリンクがない").toBeVisible();
  await toFile.click();
  await expect(page).toHaveURL(/\/files\?/);
  expect(recordsOf(fp, "#24")).toBe(0);
  await agreeHere(page, /\/files\?/);
  expect(recordsOf(fp, "#24"), "ファイルへの承認の記録が 1 件増えていない").toBe(1);
  await page.getByRole("link", { name: "ストーリーのレビューへ戻る" }).click();
  await expect(listIcon(page, "e2e/add-task.txtar", "承認済"), "戻った一覧でファイルが承認済になっていない").toBeVisible();
  await select(page, "e2e/add-task.txtar");
  await page.locator('a[href*="/files"]').first().click();
  await revokeHere(page, /\/files\?/);
  expect(recordsOf(fp, "#24"), "取り消してもファイルへの承認の記録が残っている").toBe(0);
});

// 取り消せる範囲は RV-17 と同じ。約束の詳細の画面で、誰が × どこで書いたかを全部回す
for (const w of writers) {
  for (const pl of places) {
    const can = w.self && !pl.onBase;
    // @vanda:evidence agree-from-detail-pages
    test(`約束の詳細の画面で${w.name}が${pl.name}で書いた承認の記録は${can ? "取り消せる" : "取り消せず取り消しのボタンが出ない"}`, async ({ page }) => {
      repo.write(".vanda/promises.toml", promises(v2Statement, false, true));
      commit(repo, "フラグを外す", "#24");
      const record = agreement("claim", "task-id-unique", sha(claimJSON(v2Statement)), null, "#24")
        .replace('user = { name = "テスト担当", email = "tester@example.com" }', `user = { name = "${w.user.name}", email = "${w.user.email}" }`);
      if (pl.onBase) {
        repo.git("switch", "-q", "main");
        repo.write(".vanda/agreements.toml", readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8") + record);
        commit(repo, "承認の記録を書く", "#24");
        repo.git("switch", "-q", "story-24");
        repo.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "merge", "-q", "--no-edit", "main");
      } else {
        repo.write(".vanda/agreements.toml", readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8") + record);
        commit(repo, "承認の記録を書く", "#24");
      }
      await page.goto(serve.url + "/");
      await select(page, "task-id-unique");
      await page.getByRole("link", { name: "約束の詳細を開く" }).click();
      await expect(page.getByText("承認済").first()).toBeVisible();
      if (!can) {
        await expect(unagreeButton(page)).toHaveCount(0);
        return;
      }
      await revokeHere(page, /\/claims\/task-id-unique/);
    });
  }
}

// @vanda:evidence agree-from-detail-pages-notes
test("ストーリーのレビューから一覧にないファイルや約束を開くと承認のボタンがなくこのストーリーでは変わっていないと示す", async ({ page }) => {
  // release は story-24 の今と同じ。release と比べると一覧は空なので、どのファイルも約束も一覧にない
  repo.git("branch", "release", "story-24");
  await page.goto(serve.url + "/?view=branch&base=main");
  await select(page, "task-id-unique");
  const claimHref = await page.getByRole("link", { name: "約束の詳細を開く" }).getAttribute("href");
  expect(claimHref).toBeTruthy();
  // レビューから移るときの URL のまま、ベースブランチだけを release に、ストーリーを #24 に名指しする
  const fromReview = (path: string) => {
    const u = new URL(claimHref!, serve.url);
    u.searchParams.set("base", "release");
    u.searchParams.set("story", "#24");
    u.searchParams.delete("sel");
    return path + "?" + u.searchParams.toString();
  };
  await page.goto(serve.url + fromReview("/claims/task-id-unique"));
  await expect(page.getByText("このストーリーでは変わっていない約束です。").first()).toBeVisible();
  await expect(page.getByRole("button", { name: /承認する/ })).toHaveCount(0);
  await page.goto(serve.url + fromReview("/files") + "&path=" + encodeURIComponent("e2e/add-task.txtar"));
  await expect(page.getByText("このストーリーでは変わっていないファイルです。").first()).toBeVisible();
  await expect(page.getByRole("button", { name: /承認する/ })).toHaveCount(0);
});

// @vanda:evidence agree-from-detail-pages-notes
test("約束の詳細とファイルの画面をURLで直接開くと承認のボタンがなくストーリーのレビューから開くと承認できると示す", async ({ page }) => {
  for (const path of ["/claims/task-id-unique", "/files?path=" + encodeURIComponent("e2e/add-task.txtar")]) {
    await page.goto(serve.url + path);
    await expect(page.getByText("ストーリーのレビューから開くと、承認できます。").first(), `${path} に示されない`).toBeVisible();
    await expect(page.getByRole("button", { name: /承認する/ })).toHaveCount(0);
    await expect(unagreeButton(page)).toHaveCount(0);
  }
});

// 利用者が書いた文は、続いた空白（半角 3 つ・全角）と改行を保って出す。見えている文字（innerText）で確かめる
const ws = {
  statement: "タスクの ID は   重ならない　全角の空白の後\n2 行目の   文",
  condition: "タスクが   1 つ以上　ある\n条件の 2 行目",
  rationale: "消す相手を   決めるため　理由\n理由の 2 行目",
  guess: "推測の   文　全角\n推測の 2 行目",
  detail: "詳細の   文　全角\n詳細の 2 行目",
  definition: "タスクを   見分ける　名前\n定義の 2 行目",
};
// 後のバージョン: 文に足し、定義に空白だけを足し、理由から空白だけを消す
const wsAfter = {
  ...ws,
  statement: ws.statement + "（後）",
  definition: ws.definition.replace("見分ける", "見分ける   "),
  rationale: ws.rationale.replace("消す相手を   ", "消す相手を"),
};
const q2 = (s: string) => JSON.stringify(s);
const wsPromises = (w: typeof ws) => `[[term]]
id = "task-id"
name = "タスクの ID"
definition = ${q2(w.definition)}

[[claim]]
id = "task-id-unique"
subjects = ["task-id"]
statement = ${q2(w.statement)}
condition = ${q2(w.condition)}
rationale = ${q2(w.rationale)}
layers = ["model"]
guesses = [${q2(w.guess)}]

[[claim.evidence]]
stage = "implementation"
type = "scenario-test"
layers = ["model"]
detail = ${q2(w.detail)}
`;

// wsRepo は、空白を書いたバージョンのコミット（wsbase はそこを指す）と、それを変えた今のコミットを作る
function wsRepo() {
  repo.write(".vanda/promises.toml", wsPromises(ws));
  commit(repo, "空白を書く", "#24");
  repo.git("branch", "wsbase", "story-24");
  repo.write(".vanda/promises.toml", wsPromises(wsAfter));
  commit(repo, "空白を変える", "#24");
}
const shown = async (page: Page) => page.locator("body").innerText();
const occurrences = (text: string, s: string) => text.split(s).length - 1;

test("約束の詳細の画面は利用者が書いた文の続いた空白と全角の空白と改行を保って出す", async ({ page }) => {
  wsRepo();
  await page.goto(serve.url + "/claims/task-id-unique");
  await expect(page.getByText("（後）").first()).toBeVisible();
  const text = await shown(page);
  for (const [name, s] of Object.entries({ 文: wsAfter.statement, 条件: wsAfter.condition, 理由: wsAfter.rationale, 推測: wsAfter.guess, 詳細: wsAfter.detail })) {
    expect(text.includes(s), `${name}の空白か改行が保たれていない: ${JSON.stringify(s)}`).toBe(true);
  }
});

test("レビューの1列の差分は利用者が書いた文の続いた空白と全角の空白と改行を保ち空白だけを足した消した所にもマークを出す", async ({ page }) => {
  wsRepo();
  await page.goto(serve.url + "/?view=branch&base=wsbase&story=%2324");
  await select(page, "task-id-unique");
  await expect(page.getByText("（後）").first()).toBeVisible();
  let text = await shown(page);
  // 変わっていない項目と、足しただけの文は、つながったまま見える
  for (const [name, s] of Object.entries({ 文: wsAfter.statement, 条件: ws.condition, 推測: ws.guess, 詳細: ws.detail })) {
    expect(text.includes(s), `${name}の空白か改行が保たれていない: ${JSON.stringify(s)}`).toBe(true);
  }
  // 理由から消した空白だけの所に、マーク（del）が出て、幅がある
  const blankMark = async (tag: "del" | "ins") => {
    const marks = page.locator(tag).filter({ hasText: /^\s+$/ });
    await expect(marks.first(), `空白だけを${tag === "del" ? "消した" : "足した"}所に ${tag} がない`).toBeAttached();
    const box = await marks.first().boundingBox();
    expect(box && box.width > 0, `空白だけの ${tag} に幅がない（マークが見えない）`).toBe(true);
  };
  await blankMark("del");
  // 用語の定義に空白だけを足した所
  await page.getByRole("listitem").filter({ hasText: /task-id(?!-)/ }).first().click();
  await expect(page.getByText("定義の 2 行目").first()).toBeVisible();
  text = await shown(page);
  expect(text.includes(wsAfter.definition), `用語の定義の空白か改行が保たれていない: ${JSON.stringify(wsAfter.definition)}`).toBe(true);
  await blankMark("ins");
});

for (const pg of pages) {
  test(`${pg.name}でその時点の中身を開くと利用者が書いた文の続いた空白と全角の空白と改行を保って出す`, async ({ page }) => {
    wsRepo();
    await pg.open(page);
    await expect(page.getByText("（後）").first()).toBeVisible();
    // エビデンスと変更履歴は後から読み込まれるので、出そろってから数える
    await expect(page.getByRole("heading", { name: "シナリオテスト", exact: true }).first()).toBeVisible();
    await expect(rowOf(page, "空白を書く")).toBeVisible();
    const fields = { 文: ws.statement, 条件: ws.condition, 理由: ws.rationale, 推測: ws.guess, 詳細: ws.detail };
    const before = await shown(page);
    await choose(rowOf(page, "空白を書く"));
    await expect(page.getByText("理由の 2 行目").first()).toBeVisible();
    await expect.poll(async () => occurrences(await shown(page), ws.rationale), { message: "その時点の理由の空白か改行が保たれていない" }).toBe(occurrences(before, ws.rationale) + 1);
    const after = await shown(page);
    for (const [name, s] of Object.entries(fields)) {
      expect(occurrences(after, s), `その時点の${name}の空白か改行が保たれていない: ${JSON.stringify(s)}`).toBe(occurrences(before, s) + 1);
    }
  });
}

// 実際のエビデンスのうち承認の対象になるファイルには、承認の状態（今の中身のフィンガープリントに合う承認の記録があるか）のアイコン
// （名前と title が「提案中」「承認済」）を付ける。Go の単体テストは承認の対象でないので付けない
function withGoEvidence(r: Repo) {
  const cfg = readFileSync(join(r.root, ".vanda/config.toml"), "utf8");
  r.write(".vanda/config.toml", cfg.replace("[gui]", '[[annotation]]\npaths = ["internal/task/*_test.go"]\nadapter = "go"\ntypes = ["unit-test", "cross-check", "random-check", "interface-test", "concurrency-test", "sensor"]\n\n[gui]'));
  r.write("internal/task/task_test.go", "package task\n\nimport \"testing\"\n\n// @vanda:evidence task-id-unique\nfunc TestUnique(t *testing.T) {}\n");
  const text = readFileSync(join(r.root, ".vanda/promises.toml"), "utf8");
  r.write(
    ".vanda/promises.toml",
    text.replace(
      '[[claim.evidence]]\nstage = "implementation"\ntype = "scenario-test"\nlayers = ["model"]\n',
      '[[claim.evidence]]\nstage = "implementation"\ntype = "scenario-test"\nlayers = ["model"]\n\n[[claim.evidence]]\nstage = "implementation"\ntype = "unit-test"\nlayers = ["model"]\n',
    ),
  );
  r.write("e2e/add-task.txtar", annotatedScenario);
  commit(r, "シナリオと単体テストをエビデンスにする", "#24");
}
async function expectFileStates(page: Page, scenario: "提案中" | "承認済", where: string) {
  const sc = typeGroup(page, "シナリオテスト");
  const icon = sc.getByRole("img", { name: scenario, exact: true });
  await expect(icon, `${where}: シナリオに「${scenario}」のアイコンがない`).toBeVisible();
  await expect(icon).toHaveAttribute("title", scenario);
  await expect(sc.getByRole("img", { name: scenario === "提案中" ? "承認済" : "提案中", exact: true }), `${where}: シナリオに逆のアイコンがある`).toHaveCount(0);
  const unit = typeGroup(page, "単体テスト");
  await expect(unit.getByText("TestUnique").first()).toBeVisible();
  for (const name of ["提案中", "承認済"]) {
    await expect(unit.getByRole("img", { name, exact: true }), `${where}: Go の単体テストに「${name}」のアイコンがある`).toHaveCount(0);
  }
}

for (const pg of pages) {
  // @vanda:evidence found-file-state-shown
  test(`${pg.name}の実際のエビデンスのシナリオには承認の状態のアイコンが付き承認すると承認済に承認してから変えると提案中になりGoの単体テストには付かない`, async ({ page }) => {
    withGoEvidence(repo);
    await pg.open(page);
    await expectFileStates(page, "提案中", "承認する前");

    await page.goto(serve.url + "/");
    await agreeAll(page, ["e2e/add-task.txtar"]);
    await pg.open(page);
    await expectFileStates(page, "承認済", "承認した後");

    repo.write("e2e/add-task.txtar", annotatedScenario + "-- b.txt --\nb\n");
    commit(repo, "シナリオを足す", "#24");
    await pg.open(page);
    await expectFileStates(page, "提案中", "承認してから中身を変えた後");
  });
}

test("エビデンスのファイルの画面で承認して戻るとエビデンスの予定と実際のシナリオのアイコンが承認済になる", async ({ page }) => {
  withGoEvidence(repo);
  await page.goto(serve.url + "/");
  await select(page, "task-id-unique");
  await expectFileStates(page, "提案中", "承認する前");
  await page.getByRole("link", { name: /e2e\/add-task\.txtar:1/ }).first().click();
  await agreeHere(page, /\/files\?/);
  await page.getByRole("link", { name: "ストーリーのレビューへ戻る" }).click();
  await expectFileStates(page, "承認済", "ファイルの画面で承認して戻った後");
});

// CL-01: 約束の詳細に、文・理由・条件・レイヤーを示す（レイヤーは設定の名前で）
// @vanda:evidence claim-detail-fields
test("約束の詳細の画面に文と理由と条件とレイヤーの名前が出る", async ({ page }) => {
  repo.write(".vanda/promises.toml", promisesAt(v4));
  commit(repo, "理由と条件を書く", "#24");
  await page.goto(serve.url + "/claims/task-id-unique");
  for (const t of [v4.statement, v4.rationale!, v4.condition!, "モデル"]) {
    await expect(page.getByText(t, { exact: false }).first(), `約束の詳細に「${t}」が出ない`).toBeVisible();
  }
});

// RV-05: 変更したものは、変更前と変更後の中身を示す（約束は文の差分、ファイルは行）
// @vanda:evidence review-before-after
test("レビューの画面で変えた約束とファイルの変更前と変更後の中身が出る", async ({ page }) => {
  await page.goto(serve.url + "/");
  await select(page, "task-id-unique");
  // #24 で、文に「、すべてのタスクを通して」を足した。足した所は ins に出て、変わらない所も見える
  await expect(page.locator("ins").filter({ hasText: "すべてのタスクを通して" }).first(), "文の足した所が出ない").toBeVisible();
  await expect(page.getByText("重ならない", { exact: false }).first()).toBeVisible();
  await select(page, "e2e/add-task.txtar");
  // シナリオの 2 行目を「exec」から「! exec」に変えた。変更前と変更後の行が両方出る
  await expect(page.getByText("! exec vanda-task add a").first(), "ファイルの変更後の行が出ない").toBeVisible();
  await expect(page.getByText(/^exec vanda-task add a$/).first(), "ファイルの変更前の行が出ない").toBeVisible();
});

// RV-14: そのストーリーが承認した後に中身が変わったものを「承認が失効した」と示す
// @vanda:evidence review-detached-shown
test("ストーリーが承認した約束を変えると一覧に承認が失効したと出る", async ({ page }) => {
  await page.goto(serve.url + "/");
  await agreeAll(page, ["task-id-unique"]);
  const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  repo.write(".vanda/promises.toml", text.replace("すべてのタスクを通して重ならない", "どのタスクとも重ならない"));
  commit(repo, "文を直す", "#24");
  await page.reload();
  await expect(listIcon(page, "task-id-unique", "承認が失効した"), "承認が失効したと出ない").toBeVisible();
  await expect(listIcon(page, "task-id-unique", "提案中"), "承認が失効したものに提案中と出ている").toHaveCount(0);
  await select(page, "task-id-unique");
  await expect(page.getByText(/#24/).first()).toBeVisible();
});

// @vanda:evidence review-deleted-shown
test("消したファイルは一覧に削除として出て消す前の中身を見せ消したことを承認すると承認済になりストーリーの承認のチェックが通る", async ({ page }) => {
  rmSync(join(repo.root, "e2e/add-task.txtar"));
  commit(repo, "シナリオを消す", "#24");
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await select(page, "e2e/add-task.txtar");
  const detail = page.getByRole("region", { name: "1 件の中身" });
  await expect(detail.getByText("削除").first()).toBeVisible();
  // 比べる元（main）の中身を見せる
  await expect(detail.getByText(/exec vanda-task add a/).first()).toBeVisible();
  await expect(listIcon(page, "e2e/add-task.txtar", "提案中")).toBeVisible();
  const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
  await detail.getByRole("button", { name: /消したことを承認する/ }).click();
  expect((await res).status(), "消したことを承認できない").toBe(200);
  await expect(listIcon(page, "e2e/add-task.txtar", "承認済")).toBeVisible();
  const recs = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
  expect(recs, "消したことへの承認の記録が書かれていない").toMatch(/kind = "file"\nkey = "e2e\/add-task\.txtar"\ndeleted = true/);
  await agreeAll(page, ["task-id-unique", "task-title-required"]);
  const check = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(check.code, check.out).toBe(0);
});

// RV-01・RV-04: 一覧の 1 件の 2 行目に、種類・状態のアイコン・フラグのアイコンを並べる。承認すると承認済に、承認した後に中身が変わると承認が失効したに変わる
// @vanda:evidence review-list-icons
test("一覧の1件の2行目に種類と状態のアイコンとフラグのアイコンが並びマウスを乗せると言葉が出て承認すると承認済に承認の後に変えると承認が失効したに変わる", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  const row = listItem(page, "task-title-required");
  await expect(row).toBeVisible();
  // 名前の所（1 行目）にはアイコンがなく、2 行目に種類・提案中・AI 案の順に並ぶ
  await expect(row.getByRole("button").getByRole("img"), "1 行目にアイコンがある").toHaveCount(0);
  const icons = row.getByRole("img");
  await expect(icons).toHaveCount(2);
  await expect(icons.nth(0)).toHaveAccessibleName("提案中");
  await expect(icons.nth(1)).toHaveAccessibleName("AI 案");
  const kindBox = (await row.getByText("約束", { exact: true }).boundingBox())!;
  const nameBox = (await row.getByText("task-title-required", { exact: true }).boundingBox())!;
  expect(kindBox.y, "種類が名前の下の行にない").toBeGreaterThanOrEqual(nameBox.y + nameBox.height - 1);
  for (let i = 0; i < 2; i++) {
    const box = (await icons.nth(i).boundingBox())!;
    expect(box.x, `${i + 1} 番目のアイコンが種類の後ろにない`).toBeGreaterThanOrEqual(kindBox.x + kindBox.width - 1);
    expect(Math.abs(box.y + box.height / 2 - (kindBox.y + kindBox.height / 2)), `${i + 1} 番目のアイコンが種類と同じ行にない`).toBeLessThanOrEqual(6);
  }
  // マウスを乗せると言葉が出る
  await icons.nth(1).hover();
  await expect(row.getByRole("tooltip"), "マウスを乗せても AI 案と出ない").toContainText("AI 案");
  await page.mouse.move(0, 0);
  await expect(row.getByRole("tooltip")).toHaveCount(0);

  await agreeAll(page, ["task-title-required"]);
  await expect(listIcon(page, "task-title-required", "承認済"), "承認した後に承認済のアイコンにならない").toBeVisible();
  await expect(listIcon(page, "task-title-required", "提案中")).toHaveCount(0);

  const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  repo.write(".vanda/promises.toml", text.replace("タスクの題は空にできない", "タスクの題は空にも空白だけにもできない"));
  commit(repo, "題の約束を直す", "#24");
  await page.reload();
  await expect(listIcon(page, "task-title-required", "承認が失効した"), "承認の後に変えても承認が失効したのアイコンにならない").toBeVisible();
  for (const name of ["提案中", "承認済"]) await expect(listIcon(page, "task-title-required", name), `承認が失効したものに${name}のアイコンもある`).toHaveCount(0);
});

// RV-10・RV-02: 一覧は約束のレイヤー・用語と機能・ファイルのまとまりに分け、たたむとたたんだ件数を示す。1 件ごとに通し番号と総数
// @vanda:evidence review-groups-fold
// @vanda:evidence review-numbering
test("一覧はまとまりに分かれ通し番号と総数が出てまとまりをたたむと件数が出て中が隠れる", async ({ page }) => {
  await page.goto(serve.url + "/");
  await expect(page.getByText(/全 3 件/).first()).toBeVisible();
  for (const [no, name] of [["1", "task-id-unique"], ["2", "task-title-required"], ["3", "e2e/add-task.txtar"]]) {
    await expect(listItem(page, name), `${name} に通し番号 ${no} がない`).toContainText(no);
  }
  const group = page.getByRole("button", { name: /約束.*2 件/ }).first();
  await expect(group, "約束のまとまりの見出し（2 件）がない").toBeVisible();
  await group.click();
  await expect(page.getByRole("listitem").filter({ hasText: "task-title-required" }), "たたんでも約束が見えている").toHaveCount(0);
  await expect(group, "たたんだまとまりに件数が出ていない").toContainText("2 件");
  await expect(listItem(page, "e2e/add-task.txtar"), "ほかのまとまりまで隠れた").toBeVisible();
  await group.click();
  await expect(listItem(page, "task-title-required")).toBeVisible();
});

// CM-08: ストーリーストアが設定にあれば、題・本文・添付ファイルへのリンクを出す。ストーリーストアはカスタムアダプタにする
// @vanda:evidence story-shown-on-screen
test("ストーリーストアのカスタムアダプタから読んだ題と本文と添付ファイルへのリンクがレビューの画面に出る", async ({ page }) => {
  const cfg = readFileSync(join(repo.root, ".vanda/config.toml"), "utf8");
  repo.write(".vanda/config.toml", cfg + '\n[story]\nadapter = "story"\n');
  repo.write(".vanda/adapters/story", `#!/bin/sh
cat >/dev/null
printf '%s' '{"title":"タスクの ID を重ならないようにする","body":"本文の **太字** です","url":"https://example.com/issues/24","attachments":[{"name":"画面の案","url":"https://example.com/a.png"}]}'
`);
  chmodSync(join(repo.root, ".vanda/adapters/story"), 0o755);
  commit(repo, "ストーリーストアを設定する", "#24");
  await page.goto(serve.url + "/");
  await expect(page.getByText("タスクの ID を重ならないようにする").first(), "ストーリーの題が出ない").toBeVisible();
  await expect(page.getByText("太字").first(), "ストーリーの本文が出ない").toBeVisible();
  await expect(page.getByRole("link", { name: /画面の案/ }).first()).toHaveAttribute("href", "https://example.com/a.png");
});

// CL-07: TLA+ の定義は記号で見せ、コピーしたときはリポジトリの文字のまま（ASCII）にする
// @vanda:evidence tla-copy-ascii
test("約束の詳細の画面でTLAの定義をコピーするとリポジトリの文字のままになる", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const def = "Unique == \\A a, b \\in Ids : a = b => a = b";
  const cfg = readFileSync(join(repo.root, ".vanda/config.toml"), "utf8");
  repo.write(".vanda/config.toml", cfg.replace("[gui]", '[[annotation]]\npaths = ["spec/*.tla"]\nadapter = "tla"\ntypes = ["model-check"]\n\n[gui]'));
  repo.write("spec/Task.tla", "---- MODULE Task ----\nIds == {1, 2}\n\n\\* @vanda:claim task-id-unique\n" + def + "\n====\n");
  repo.write(".vanda/promises.toml", promisesAt(v4));
  commit(repo, "モデルを足す", "#24");
  await page.goto(serve.url + "/claims/task-id-unique");
  await expect(page.getByText("∀", { exact: false }).first(), "TLA+ の定義が記号で出ていない").toBeVisible();
  await page.getByRole("button", { name: /コピー/ }).first().click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText()), { message: "コピーした文字がリポジトリの文字のままでない" }).toContain(def);
  expect(await page.evaluate(() => navigator.clipboard.readText())).not.toMatch(/[∀∈⇒≜]/);
});

// TLA+ の記号の幅と書体を、Chrome が実際に描いた結果で確かめる。
// 2 行目の頭の /\ は、1 行目の /\ と同じ桁に置いてある（"Unique == " の 10 文字ぶん）。
const alignedDef = "Unique == /\\ \\A a, b \\in Ids : a = b => a = b\n          /\\ ~(Ids \\subseteq {}) /\\ [x \\in Ids |-> x] # <<1, 2>>";
// [見せる記号, 置き換える前の ASCII の文字数]。alignedDef から手で数える
const alignedSymbols: [string, number][] = [
  ["≜", 2], ["∧", 2], ["∀", 2], ["∈", 3], ["⇒", 2],
  ["∧", 2], ["¬", 1], ["⊆", 9], ["∧", 2], ["∈", 3], ["↦", 3], ["≠", 1], ["⟨", 2], ["⟩", 2],
];

async function openTlaModel(page: Page, def: string) {
  const cfg = readFileSync(join(repo.root, ".vanda/config.toml"), "utf8");
  repo.write(".vanda/config.toml", cfg.replace("[gui]", '[[annotation]]\npaths = ["spec/*.tla"]\nadapter = "tla"\ntypes = ["model-check"]\n\n[gui]'));
  repo.write("spec/Task.tla", "---- MODULE Task ----\nIds == {1, 2}\n\n\\* @vanda:claim task-id-unique\n" + def + "\n====\n");
  repo.write(".vanda/promises.toml", promisesAt(v4));
  commit(repo, "モデルを足す", "#24");
  await page.goto(serve.url + "/claims/task-id-unique");
  await expect(page.locator("pre.code", { hasText: "Unique ≜" }).first(), "TLA+ の定義が記号で出ていない").toBeVisible();
  // 書体（等幅と数学の記号）を読み終えてから測る。読む前は代わりの書体で描くので、幅が変わる
  await page.evaluate(() => document.fonts.ready.then(() => undefined));
}

interface SymbolBox {
  text: string;
  left: number;
  top: number;
  width: number;
  glyphCenter: number;
}

// measureSymbols は、TLA+ の定義の中の記号の箱と、その中に描いた記号の位置、等幅の 1 文字の幅を測る。
// 等幅の 1 文字の幅は、同じ pre の中に置いた 100 文字の 0 の幅から求める。
function measureSymbols(page: Page) {
  return page.evaluate(() => {
    const pre = [...document.querySelectorAll<HTMLElement>("pre.code")].find((p) => p.textContent?.includes("Unique ≜"))!;
    const probe = document.createElement("span");
    probe.textContent = "0".repeat(100);
    pre.appendChild(probe);
    const unit = probe.getBoundingClientRect().width / 100;
    probe.remove();
    const boxes = [...pre.querySelectorAll<HTMLElement>(".sym")].map((box) => {
      const r = box.getBoundingClientRect();
      const range = document.createRange();
      range.selectNodeContents(box);
      const g = range.getBoundingClientRect();
      return { text: box.textContent ?? "", left: r.left, top: r.top, width: r.width, glyphCenter: g.left + g.width / 2 };
    });
    return { unit, boxes };
  });
}

// @vanda:evidence tla-symbol-keeps-width
test("TLAの記号は元のASCIIの文字数ぶんの等幅の文字の幅を取り中央に描かれ縦に揃えた並びが崩れない", async ({ page }) => {
  await openTlaModel(page, alignedDef);
  const { unit, boxes } = await measureSymbols(page);
  expect(unit, "等幅の 1 文字の幅を測れない").toBeGreaterThan(0);
  expect(boxes.map((b) => b.text)).toEqual(alignedSymbols.map(([c]) => c));
  boxes.forEach((b: SymbolBox, i) => {
    const [c, n] = alignedSymbols[i];
    expect(Math.abs(b.width - n * unit), `${i + 1} 番目の ${c} の幅 ${b.width}px が ${n} 文字ぶん（${n * unit}px）でない`).toBeLessThanOrEqual(0.5);
    const center = b.left + b.width / 2;
    expect(Math.abs(b.glyphCenter - center), `${i + 1} 番目の ${c} が箱の中央（${center}px）でなく ${b.glyphCenter}px に描かれている`).toBeLessThanOrEqual(0.5);
  });
  // 1 行目と 2 行目の頭の ∧ は、同じ桁に揃う
  const first = boxes[1];
  const second = boxes[5];
  expect(second.top, "2 行目の ∧ が 1 行目より下にない").toBeGreaterThan(first.top);
  expect(Math.abs(second.left - first.left), `1 行目の ∧（${first.left}px）と 2 行目の ∧（${second.left}px）の桁が揃わない`).toBeLessThanOrEqual(0.5);
});

// @vanda:evidence tla-symbol-font
test("TLAの記号は画面に同梱したNoto Sans Mathを読み込んでその書体だけで描かれる", async ({ page }) => {
  await openTlaModel(page, alignedDef);
  await expect
    .poll(
      () => page.evaluate(() => [...document.fonts].filter((f) => f.family.replace(/["']/g, "") === "Noto Sans Math").map((f) => f.status)),
      { message: "Noto Sans Math を読み込んでいない" },
    )
    .toContain("loaded");
  // 同梱のファイルを vanda サーバーから読んだこと
  const fontURLs = await page.evaluate(() =>
    performance
      .getEntriesByType("resource")
      .map((e) => e.name)
      .filter((u) => /noto-sans-math.*\.woff2$/.test(u)),
  );
  expect(fontURLs.length, "Noto Sans Math のファイルを読んでいない").toBeGreaterThan(0);
  for (const u of fontURLs) expect(u.startsWith(serve.url + "/"), `${u} を vanda サーバーから読んでいない`).toBe(true);

  // 記号の文字だけを持ついちばん内側の要素にマークを付け、Chrome が実際に使った書体を DevTools のプロトコルで聞く。
  // シンタックスハイライトを終えると画面が要素を作り直し、マークが消えることがあるので、マークを付けるところから条件で待ち直す
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("DOM.enable");
  await cdp.send("CSS.enable");
  const usedFonts = async (): Promise<string[]> => {
    const count = await page.evaluate(() => {
      const pre = [...document.querySelectorAll<HTMLElement>("pre.code")].find((p) => p.textContent?.includes("Unique ≜"))!;
      const boxes = [...pre.querySelectorAll<HTMLElement>(".sym")];
      boxes.forEach((box, i) => {
        let inner: HTMLElement = box;
        box.querySelectorAll<HTMLElement>("*").forEach((el) => {
          if (el.textContent === box.textContent) inner = el;
        });
        inner.setAttribute("data-e2e-glyph", String(i));
      });
      return boxes.length;
    });
    const { root } = await cdp.send("DOM.getDocument", { depth: -1 });
    const got: string[] = [];
    for (let i = 0; i < count; i++) {
      const { nodeId } = await cdp.send("DOM.querySelector", { nodeId: root.nodeId, selector: `[data-e2e-glyph="${i}"]` });
      if (!nodeId) return [`${i + 1} 番目の記号の要素を見失った`];
      const { fonts } = await cdp.send("CSS.getPlatformFontsForNode", { nodeId });
      got.push(`${alignedSymbols[i]?.[0]}: ${fonts.map((f) => `${f.familyName}${f.isCustomFont ? "（同梱）" : "（OS）"}`).join("、")}`);
    }
    return got;
  };
  await expect
    .poll(usedFonts, { message: "TLA+ の記号を、同梱した Noto Sans Math だけで描いていない" })
    .toEqual(alignedSymbols.map(([c]) => `${c}: Noto Sans Math（同梱）`));
});

// RV-01 の絞り込み。story-24 の一覧は task-id-unique（文を変えた）・task-title-required（足した）・e2e/add-task.txtar の 3 件
const filterList = (page: Page) => page.getByRole("region", { name: "変わったものの一覧" });
async function expectListed(page: Page, want: string[], where: string) {
  const list = filterList(page);
  for (const item of items) {
    const n = want.includes(item) ? 1 : 0;
    await expect(list.getByText(item, { exact: true }), `${where}: ${item} が${n ? "出ない" : "出ている"}`).toHaveCount(n);
  }
}

// @vanda:evidence review-filter-text
// @vanda:evidence review-filter-not-agreed
test("一覧を文字で絞り込み承認した後に承認済でないものだけにすると承認したものが消え文字の絞り込みと重ねられる", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await expectListed(page, items, "絞り込む前");
  const box = filterList(page).getByRole("searchbox", { name: "絞り込み" });
  // ID・文・パスに、大文字と小文字を変えてマッチさせる
  await box.fill("TITLE");
  await expectListed(page, ["task-title-required"], "ID で絞り込んだとき");
  await box.fill("すべてのタスクを通して");
  await expectListed(page, ["task-id-unique"], "文で絞り込んだとき");
  await box.fill("E2E/ADD");
  await expectListed(page, ["e2e/add-task.txtar"], "パスで絞り込んだとき");
  await box.fill("");
  await expectListed(page, items, "文字を消したとき");

  await agreeAll(page, ["task-id-unique"]);
  const notAgreed = filterList(page).getByRole("checkbox", { name: "承認済でないものだけ" });
  await notAgreed.check();
  await expectListed(page, ["task-title-required", "e2e/add-task.txtar"], "承認済でないものだけにしたとき");
  await box.fill("task");
  await expectListed(page, ["task-title-required", "e2e/add-task.txtar"], "承認済でないものだけと task を重ねたとき");
  await box.fill("required");
  await expectListed(page, ["task-title-required"], "承認済でないものだけと required を重ねたとき");
  await box.fill("unique");
  await expectListed(page, [], "承認済でないものだけと承認した件の文字を重ねたとき");
  await expect(filterList(page).getByText(/絞り込みの条件にマッチするものはありません/)).toBeVisible();
  await notAgreed.uncheck();
  await expectListed(page, ["task-id-unique"], "承認済でないものだけを外したとき");
});

// @vanda:evidence review-filter-kept
test("絞り込みの条件は約束の詳細から戻っても読み込み直しても残る", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await expectListed(page, items, "絞り込む前");
  const box = filterList(page).getByRole("searchbox", { name: "絞り込み" });
  const notAgreed = filterList(page).getByRole("checkbox", { name: "承認済でないものだけ" });
  await box.fill("Title");
  await notAgreed.check();
  await expectListed(page, ["task-title-required"], "絞り込んだとき");
  const kept = async (where: string) => {
    await expect(box, `${where}: 文字が消えた`).toHaveValue("Title");
    await expect(notAgreed, `${where}: 承認済でないものだけが外れた`).toBeChecked();
    await expectListed(page, ["task-title-required"], where);
  };
  await select(page, "task-title-required");
  await page.getByRole("link", { name: "約束の詳細を開く" }).click();
  await expect(page).toHaveURL(/\/claims\/task-title-required/);
  await page.getByRole("link", { name: "ストーリーのレビューへ戻る" }).click();
  await kept("約束の詳細から戻ったとき");
  await page.reload();
  await kept("読み込み直したとき");
});
