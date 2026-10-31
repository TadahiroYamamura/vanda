// E2E: 一時的な git のリポジトリに対して vanda serve を動かし、Chrome で操作する。本物の GitHub には繋がない
// （ストーリーの置き場所は設定に書かない）。サーバーはテストの中で起動し、テストの終わりに止め、ポートを手放したことも確かめる。
// 動かす vanda は、make build が作るリポジトリの直下の vanda（VANDA_BIN で差し替えられる）。
import { test, expect, type Page } from "@playwright/test";
import { spawn, execFileSync, type ChildProcess } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, writeFileSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const bin = process.env.VANDA_BIN ?? resolve(here, "..", "..", "vanda");

const sha = (s: string) => "sha256:" + createHash("sha256").update(s, "utf8").digest("hex");

// 指紋の元の JSON は、docs/formats.md の決まった形を手で書く（約束リストを読んで作らない）
const termJSON = `{"definition":"タスクを見分ける名前","id":"task-id","name":"タスクの ID"}`;
const claimJSON = (statement: string) =>
  `{"evidence":[{"layers":["model"],"stage":"test","type":"scenario-test"}],"id":"task-id-unique","layers":["model"],"statement":"${statement}","subjects":["task-id"]}`;

const config = (version: string) => `version = "${version}"

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[annotation]]
paths = ["e2e/*.txtar"]
adapter = "txtar"

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
stage = "test"
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
stage = "test"
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

// buildRepo は、main（#1 で合意済）と、ストーリー #24 が約束を変え・足し、シナリオを変えた story-24 を作る。HEAD は story-24。
function buildRepo(): Repo {
  const r = makeRepo();
  const version = execFileSync(bin, ["version"], { encoding: "utf8" }).match(/\d+\.\d+\.\d+/)![0];
  r.git("init", "-q", "-b", "main");
  r.git("config", "user.name", "合意する人");
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
  r.git("add", "-A");
  r.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "-m", "#1 はじめの約束を置く");
  r.git("switch", "-q", "-c", "story-24");
  r.write(".vanda/promises.toml", promises("タスクの ID は、すべてのタスクを通して重ならない", true, true));
  r.git("add", "-A");
  r.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "-m", "#24 約束を変え、題の約束を足す");
  r.write("e2e/add-task.txtar", scenarioStory);
  r.git("add", "-A");
  r.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "-m", "#24 シナリオを変える");
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
            // 同じポートで待ち受けられれば、サーバーはポートを手放している
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

// select は、一覧の 1 件を選ぶ。
async function select(page: Page, text: string) {
  await page.getByText(text, { exact: false }).first().click();
}

async function agreeAll(page: Page, items: string[]) {
  for (const item of items) {
    await select(page, item);
    const button = page.getByRole("button", { name: /合意する/ });
    await expect(button).toBeEnabled();
    const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
    await button.click();
    expect((await res).status(), `${item} に合意できない`).toBe(200);
  }
}

const items = ["task-id-unique", "task-title-required", "e2e/add-task.txtar"];

test("ストーリーを選び一覧を見て全部に合意するとvanda checkの合意の審査が通る", async ({ page }) => {
  expect(repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main").code, "合意する前から審査が通っている").toBe(1);
  await page.goto(serve.url + "/");
  // 開いたときの既定は、設定のベースブランチ。今どのベースブランチと比べているかを、選ぶ所のほかにも示す
  const baseSelect = page.getByRole("combobox", { name: "ベースブランチ" });
  await expect(baseSelect).toHaveValue("main");
  await expect(page.getByText(/main/).filter({ visible: true }).first()).toBeVisible();
  // ストーリーを選ぶ
  const storySelect = page.getByRole("combobox", { name: "ストーリー" });
  const value = await storySelect.locator("option", { hasText: "#24" }).first().getAttribute("value");
  await storySelect.selectOption(value ?? "#24");
  for (const item of items) await expect(page.getByText(item).first()).toBeVisible();
  await expect(page.getByText(/全 3 件|3 件/).first()).toBeVisible();
  await agreeAll(page, items);
  const check = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(check.code, check.out).toBe(0);
  // 合意は 1 件ごとに 1 つのコミット
  expect(repo.git("rev-list", "--count", "main..HEAD")).toBe(String(2 + items.length));
});

test("合意した約束を変えると一覧に差分ありとして戻る", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await agreeAll(page, ["task-id-unique"]);
  const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  repo.write(".vanda/promises.toml", text.replace("すべてのタスクを通して重ならない", "どのタスクとも重ならない"));
  repo.git("commit", "-q", "-am", "#24 文を直す");
  await page.reload();
  await select(page, "task-id-unique");
  await expect(page.getByText(/差分あり/).first()).toBeVisible();
  await expect(page.getByText(/提案中/).first()).toBeVisible();
});

test("ベースブランチと見方を切り替えると一覧が変わる", async ({ page }) => {
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

  // ストーリー全体の見方の選択肢は、文の全体でなく、ストーリーの ID と「変更」で探す（文言の細部に頼らない）
  const view = page.getByRole("combobox", { name: "見方" });
  const storyOption = view.locator("option").filter({ hasText: "#24" }).filter({ hasText: "変更" }).first();
  await view.selectOption((await storyOption.getAttribute("value")) ?? "story");
  await expect(page).toHaveURL(/view=story/);
  for (const item of items) await expect(page.getByText(item).first()).toBeVisible();
});

// 約束の詳細とファイルの画面から「ストーリーのレビューへ戻る」で戻ると、行く前のレビューの画面（見方・ベースブランチ・
// ストーリー・選んでいた 1 件）に戻る。ブラウザーの「戻る」には頼らない（PO の報告では、こちらだけが選ぶ前に戻っていた）。
// ストーリー全体の見方ではベースブランチを選ぶ所が出ないので、見方とベースブランチの組は 3 つ。それと戻る経路 3 つの
// 組み合わせを全部回す。
const states = [
  { name: "ブランチごとの見方でベースブランチを既定のmainにして", story: false, base: "main" },
  { name: "ブランチごとの見方でベースブランチをreleaseに変えて", story: false, base: "release" },
  { name: "ストーリー全体の見方に切り替えて", story: true, base: "main" },
];
const routes = [
  { name: "約束の詳細から直接", via: "claim" },
  { name: "約束の詳細からエビデンスのファイルを経て", via: "claim-file" },
  { name: "エビデンスの予定と実際の表からファイルを経て", via: "review-file" },
] as const;

for (const v of states) {
  for (const route of routes) {
      test(`${v.name}選んだ約束で${route.name}ストーリーのレビューへ戻ると行く前と同じ状態に戻る`, async ({ page }) => {
        // シナリオに task-id-unique のエビデンスを付け、約束の文も変える。release は、このコミットの直前から分かれたことに
        // する（release と比べても task-id-unique が一覧に出て、main と比べたときとは一覧が変わる）
        repo.git("branch", "release", "story-24");
        repo.write("e2e/add-task.txtar", "# シナリオ\n# @vanda:evidence task-id-unique\n! exec vanda-task add a\n-- a.txt --\na\n");
        const text = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
        repo.write(".vanda/promises.toml", text.replace("すべてのタスクを通して重ならない", "どのタスクとも重ならない"));
        repo.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "-am", "#24 シナリオをエビデンスにする");

        await page.goto(serve.url + "/");
        const storySelect = page.getByRole("combobox", { name: "ストーリー" });
        const storyValue = (await storySelect.locator("option", { hasText: "#24" }).first().getAttribute("value")) ?? "#24";
        await storySelect.selectOption(storyValue);
        const baseSelect = page.getByRole("combobox", { name: "ベースブランチ" });
        if (v.base !== "main") await baseSelect.selectOption(v.base);
        const view = page.getByRole("combobox", { name: "見方" });
        if (v.story) {
          const storyOption = view.locator("option").filter({ hasText: "#24" }).filter({ hasText: "変更" }).first();
          await view.selectOption((await storyOption.getAttribute("value")) ?? "story");
        }
        if (!v.story && v.base === "release") await expect(page.getByText("task-title-required")).toHaveCount(0);
        else await expect(page.getByText("task-title-required").first()).toBeVisible();
        await select(page, "task-id-unique");
        const claimLink = page.getByRole("link", { name: "約束の詳細を開く" });
        await expect(claimLink).toHaveAttribute("href", /task-id-unique/);
        const before = {
          view: await view.inputValue(),
          base: v.story ? null : await baseSelect.inputValue(),
          story: await storySelect.inputValue(),
        };
        if (!v.story) expect(before.base).toBe(v.base);

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

        await expect(page.getByRole("combobox", { name: "見方" })).toHaveValue(before.view);
        if (before.base !== null) await expect(page.getByRole("combobox", { name: "ベースブランチ" })).toHaveValue(before.base);
        await expect(page.getByRole("combobox", { name: "ストーリー" })).toHaveValue(before.story);
        await expect(page.getByRole("link", { name: "約束の詳細を開く" }), "選んでいた task-id-unique の 1 件の詳細に戻っていない").toHaveAttribute("href", /task-id-unique/);
        await expect(page.getByRole("button", { name: /合意する/ })).toBeVisible();
      });
  }
}

// RV-16 と CL-08 の前提: task-id-unique を、main（v1）、#24 の最初のコミット（v2）の後に、さらに 2 回変える（v3・v4）。
// 版ごとに、文・理由・条件・レイヤー・エビデンスの予定に、ほかの版に出てこない文字を入れる。
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
stage = "test"
type = "scenario-test"
layers = ["model"]
${c.plan ? `
[[claim.evidence]]
stage = "plan"
type = "model-check"
layers = ["model"]
detail = "${c.plan}"
` : ""}`;

function commit(r: Repo, subject: string): string {
  r.git("add", "-A");
  r.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "commit", "-q", "-m", subject);
  return r.git("rev-parse", "HEAD");
}

// commitV3 は、DB のレイヤーを設定に足し、task-id-unique を v3 にする。
function commitV3(r: Repo): string {
  const cfg = readFileSync(join(r.root, ".vanda/config.toml"), "utf8");
  r.write(".vanda/config.toml", cfg.replace("[[annotation]]", '[[layer]]\nid = "db"\nname = "データベース"\ndescription = "保存の制約"\n\n[[annotation]]'));
  r.write(".vanda/promises.toml", promisesAt(v3));
  return commit(r, "#24 理由と条件を足す");
}

// 由来の行と合意の記録の行は、CL の約束の詳細と同じく表の行として探す。行の中にボタンがあればそれを、なければ行を押して選ぶ
async function choose(row: ReturnType<Page["getByRole"]>) {
  await expect(row).toBeVisible();
  const button = row.getByRole("button");
  if ((await button.count()) > 0) await button.first().click();
  else await row.click();
}
const rowOf = (page: Page, text: string) => page.getByRole("row").filter({ hasText: text }).first();

async function openReviewDetail(page: Page) {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await select(page, "task-id-unique");
  await expect(page.getByRole("link", { name: "約束の詳細を開く" })).toBeVisible();
}

// RV-16。中身は CL-04・CL-05 と同じ（由来はコミットとストーリーの ID、合意の履歴は合意した人・ストーリー・今の中身に合うか）
test("レビューの画面の1件の詳細に約束の由来と合意の履歴が出て合意すると今の中身に合う合意が履歴に足される", async ({ page }) => {
  const c1 = repo.git("rev-parse", "main");
  const c2 = repo.git("rev-parse", "story-24~1");
  const c3 = commitV3(repo);
  await openReviewDetail(page);

  // 由来: 約束を作り・変えたコミットだけ。シナリオだけを変えたコミットは出ない
  for (const [hash, subject, story] of [
    [c1, "#1 はじめの約束を置く", "#1"],
    [c2, "#24 約束を変え、題の約束を足す", "#24"],
    [c3, "#24 理由と条件を足す", "#24"],
  ]) {
    const row = rowOf(page, subject);
    await expect(row, `由来に ${subject} がない`).toBeVisible();
    await expect(row).toContainText(hash.slice(0, 7));
    await expect(row).toContainText(story);
  }
  await expect(page.getByText("#24 シナリオを変える")).toHaveCount(0);

  // 合意の履歴: #1 の合意は v1 への合意なので、今の中身に合わない
  const old = rowOf(page, "テスト担当");
  await expect(old).toContainText("#1");
  await expect(old).toContainText(/合いません/);
  await expect(page.getByRole("row").filter({ hasText: "合意する人" })).toHaveCount(0);

  const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
  await page.getByRole("button", { name: /合意する/ }).click();
  expect((await res).status()).toBe(200);
  const now = rowOf(page, "合意する人");
  await expect(now, "合意した後に、合意の履歴に足されていない").toBeVisible();
  await expect(now).toContainText("#24");
  await expect(now).toContainText(/合います/);
  await expect(rowOf(page, "テスト担当")).toContainText(/合いません/);
});

// CL-08。今の中身として画面に出ている版（約束の詳細では v4、レビューの画面では変更前の v1 と変更後の v4）は、選んで出たか
// 見分けられないので、その画面で出ていない版だけを選ぶ。選ぶ前に出ていないことも確かめる
const pages = [
  { name: "約束の詳細の画面", open: async (page: Page) => page.goto(serve.url + "/claims/task-id-unique"), showsV1: false },
  { name: "レビューの画面", open: openReviewDetail, showsV1: true },
];

for (const pg of pages) {
  test(`${pg.name}で由来の各コミットを選ぶとそのコミットの時点の約束の中身が出る`, async ({ page }) => {
    commitV3(repo);
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "#24 文を直す");
    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();

    // その時点のレイヤーは、今の中身と同じく名前で出す。db はそのコミットで設定に足したので、名前がある
    const v3Marks: (string | RegExp)[] = [v3.statement, v3.rationale!, v3.condition!, v3.plan!, "データベース"];
    const choices: [string, (string | RegExp)[]][] = [
      ["#24 理由と条件を足す", v3Marks],
      ["#24 約束を変え、題の約束を足す", [v2Statement]],
    ];
    if (!pg.showsV1) choices.push(["#1 はじめの約束を置く", [v1Statement]]);
    for (const [, marks] of choices) {
      for (const m of marks) await expect(page.getByText(m), `選ぶ前から ${m} が出ている`).toHaveCount(0);
    }
    for (const [subject, marks] of choices) {
      await choose(rowOf(page, subject));
      for (const m of marks) await expect(page.getByText(m).first(), `${subject} を選んでも ${m} が出ない`).toBeVisible();
    }
  });

  test(`${pg.name}で約束を変える前の合意の記録を選ぶと今の中身でなく合意を書いたコミットの時点の中身が出る`, async ({ page }) => {
    commitV3(repo);
    // v3 のときに画面で合意し、その後に v4 へ変える
    await openReviewDetail(page);
    const res = page.waitForResponse((r) => r.url().includes("/api/agree"));
    await page.getByRole("button", { name: /合意する/ }).click();
    expect((await res).status()).toBe(200);
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "#24 文を直す");

    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();
    for (const m of [v3.statement, v3.rationale!]) await expect(page.getByText(m)).toHaveCount(0);
    await choose(rowOf(page, "合意する人"));
    for (const m of [v3.statement, v3.rationale!, v3.condition!, v3.plan!]) {
      await expect(page.getByText(m, { exact: false }).first(), `合意の記録を選んでも、合意した時点の ${m} が出ない`).toBeVisible();
    }
    // #1 の合意は、#1 のコミットで書いたので v1 の中身
    if (!pg.showsV1) {
      await expect(page.getByText(v1Statement, { exact: true })).toHaveCount(0);
      await choose(rowOf(page, "テスト担当"));
      await expect(page.getByText(v1Statement, { exact: true }).first()).toBeVisible();
    }
  });

  test(`${pg.name}で合意を書いたコミットの時点に約束リストが読めない合意の記録を選ぶと理由が出る`, async ({ page }) => {
    commitV3(repo);
    // 合意の記録を足したコミットで、約束リストの形を壊す。次のコミットで直す
    const agreements = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
    repo.write(".vanda/agreements.toml", agreements + agreement("claim", "task-id-unique", sha(claimJSON(v3.statement)), null, "#24").replace("テスト担当", "壊れた時に合意した人"));
    repo.write(".vanda/promises.toml", promisesAt(v3) + "\n[[claim]\n");
    commit(repo, "#24 合意を書き約束リストを壊す");
    repo.write(".vanda/promises.toml", promisesAt(v4));
    commit(repo, "#24 文を直す");

    await pg.open(page);
    await expect(page.getByText(v4.statement).first()).toBeVisible();
    await expect(page.getByText(/promises\.toml/)).toHaveCount(0);
    await choose(rowOf(page, "壊れた時に合意した人"));
    await expect(page.getByText(/promises\.toml/).first(), "その時点の約束リストが読めない理由が出ない").toBeVisible();
  });
}

// RV-17。取り消しのボタンの名前と API の形はまだ決まっていないので、ボタンは「取り消」を含む名前で探し、
// 取り消しが済んだかはコミットが増えたことで待つ。API で断られることは、API の形が docs/formats.md に書かれてから確かめる
const unagreeButton = (page: Page) => page.getByRole("button", { name: /取り消/ });
const commitCount = () => Number(repo.git("rev-list", "--count", "HEAD"));
const nonBlank = (t: string) => t.split("\n").filter((l) => l.trim() !== "");

// withoutRecord は、合意の記録のファイルの空行でない行から、key と story の記録 1 件（[[agreement]] から、コメント・次の記録の前まで）を除く
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

// 合意の記録のファイルに、コメントと空行を混ぜる（取り消しで、ほかの行が変わらないことを見るため）
function decorateAgreements(r: Repo) {
  const text = readFileSync(join(r.root, ".vanda/agreements.toml"), "utf8");
  const parts = text.split("\n[[agreement]]");
  r.write(".vanda/agreements.toml", "# 合意の記録（E2E の例）\n\n" + parts[0] + "\n[[agreement]]" + parts[1] + "\n# 用語の次は約束\n\n\n[[agreement]]" + parts.slice(2).join("\n[[agreement]]") + "\n# 終わりのコメント\n");
  r.write("README.md", "# 例\n");
  commit(r, "#24 合意の記録にコメントを足す");
}

test("合意済の約束で合意を取り消すとストーリーの今の中身への合意の記録だけが1つのコミットで消え提案中に戻り審査で止まりもう一度合意すると通る", async ({ page }) => {
  decorateAgreements(repo);
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await select(page, "task-id-unique");
  await expect(page.getByRole("button", { name: /合意する/ })).toBeVisible();
  await expect(unagreeButton(page), "提案中のものに取り消しが出ている").toHaveCount(0);
  await agreeAll(page, items);
  const passed = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(passed.code, passed.out).toBe(0);

  const agreementsBefore = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
  const promisesBefore = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  const countBefore = commitCount();
  // 合意と関係のない書きかけの変更（追跡しているファイルと、追跡していないファイル）
  repo.write("README.md", "# 例\n書きかけ\n");
  repo.write("メモ.txt", "書きかけ\n");

  await select(page, "task-id-unique");
  await unagreeButton(page).click();
  await expect.poll(commitCount, { message: "取り消しがコミットにならない" }).toBe(countBefore + 1);

  expect(repo.git("log", "-1", "--format=%B"), "コミットのメッセージにストーリーの ID がない").toContain("#24");
  expect(repo.git("diff", "--name-only", "HEAD~1", "HEAD").split("\n"), "合意の記録のファイルのほかに変わったものがある").toEqual([".vanda/agreements.toml"]);
  const agreementsAfter = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
  // #24 の task-id-unique への記録だけが消え、#1 の記録とコメントは同じ順で残る
  expect(nonBlank(agreementsAfter)).toEqual(withoutRecord(agreementsBefore, "task-id-unique", "#24"));
  expect(nonBlank(agreementsAfter).length).toBeLessThan(nonBlank(agreementsBefore).length);
  for (const c of ["# 合意の記録（E2E の例）", "# 用語の次は約束", "# 終わりのコメント"]) expect(agreementsAfter).toContain(c);
  // フラグは戻らない
  const promisesAfter = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
  expect(promisesAfter).toBe(promisesBefore);
  expect(promisesAfter).not.toContain("ai-proposal");
  // 書きかけの変更は残り、コミットに入らない
  // git() は前後の空白を落とすので、行ごとに空白を落として比べる
  expect(repo.git("-c", "core.quotePath=false", "status", "--porcelain", "--untracked-files=all").split("\n").map((l) => l.trim()).sort()).toEqual(["?? メモ.txt", "M README.md"]);

  await expect(page.getByText(/提案中/).first()).toBeVisible();
  // 自分で取り消したものは、RV-14 の「合意が外れた」としては出さない
  await expect(page.getByText(/合意が外れた/)).toHaveCount(0);
  const stopped = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(stopped.code, "取り消した後も合意の審査が通る").toBe(1);
  expect(stopped.out).toContain("task-id-unique");

  await agreeAll(page, ["task-id-unique"]);
  const again = repo.vanda("check", ".vanda/checks/merge.toml", "--base", "main");
  expect(again.code, again.out).toBe(0);
});

test("合意の記録のファイルに書きかけの変更があると合意の取り消しを断り理由を出しコミットしない", async ({ page }) => {
  await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
  await agreeAll(page, ["task-id-unique"]);
  const dirty = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8") + "# 書きかけ\n";
  repo.write(".vanda/agreements.toml", dirty);
  const countBefore = commitCount();
  await expect(page.getByText(/agreements\.toml|合意の記録/)).toHaveCount(0);

  await select(page, "task-id-unique");
  await unagreeButton(page).click();
  await expect(page.getByText(/agreements\.toml|合意の記録/).first(), "断った理由が出ない").toBeVisible();
  expect(commitCount()).toBe(countBefore);
  expect(readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8")).toBe(dirty);
  await expect(page.getByText(/合意済/).first()).toBeVisible();
});

// 取り消せるのは、今の git の人（buildRepo の user.name「合意する人」・user.email「po@example.com」）が今のブランチで書いた
// 合意の記録だけ。誰が × どこで書いたか、の組み合わせを全部回す。ほかの人は、名前だけ・メールだけ・両方の違いを分ける
const writers = [
  { name: "自分", user: { name: "合意する人", email: "po@example.com" }, self: true },
  { name: "名前だけ違うほかの人", user: { name: "ほかの人", email: "po@example.com" }, self: false },
  { name: "メールだけ違うほかの人", user: { name: "合意する人", email: "other@example.com" }, self: false },
  { name: "名前もメールも違うほかの人", user: { name: "ほかの人", email: "other@example.com" }, self: false },
];
const places = [
  { name: "今のブランチ", onBase: false },
  { name: "ベースブランチ", onBase: true },
];

for (const w of writers) {
  for (const pl of places) {
    const can = w.self && !pl.onBase;
    test(`${w.name}が${pl.name}で書いた合意の記録は${can ? "取り消せて記録が消えた1つのコミットになる" : "取り消せず取り消しのボタンが出ない"}`, async ({ page }) => {
      // task-id-unique のフラグを外し、今の中身（#24 の文）への合意の記録を手で書く。ベースブランチなら main に書いて
      // story-24 に取り込み、分かれた点より前にする
      repo.write(".vanda/promises.toml", promises(v2Statement, false, true));
      commit(repo, "#24 フラグを外す");
      const record = agreement("claim", "task-id-unique", sha(claimJSON(v2Statement)), null, "#24")
        .replace('user = { name = "テスト担当", email = "tester@example.com" }', `user = { name = "${w.user.name}", email = "${w.user.email}" }`);
      if (pl.onBase) {
        repo.git("switch", "-q", "main");
        repo.write(".vanda/agreements.toml", readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8") + record);
        commit(repo, "#24 合意の記録を書く");
        repo.git("switch", "-q", "story-24");
        repo.git("-c", "user.name=テスト担当", "-c", "user.email=tester@example.com", "merge", "-q", "--no-edit", "main");
      } else {
        repo.write(".vanda/agreements.toml", readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8") + record);
        commit(repo, "#24 合意の記録を書く");
      }
      const agreementsBefore = readFileSync(join(repo.root, ".vanda/agreements.toml"), "utf8");
      const promisesBefore = readFileSync(join(repo.root, ".vanda/promises.toml"), "utf8");
      const countBefore = commitCount();

      await page.goto(serve.url + "/?view=branch&base=main&story=%2324");
      await select(page, "task-id-unique");
      await expect(page.getByText("今の中身に合意済みです。").first(), "手で書いた合意の記録が今の中身への合意になっていない").toBeVisible();
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
      await expect(page.getByText(/提案中/).first()).toBeVisible();
    });
  }
}
