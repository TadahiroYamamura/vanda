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

  await page.getByRole("combobox", { name: "見方" }).selectOption({ label: "ストーリー全体" });
  await expect(page).toHaveURL(/view=story/);
  for (const item of items) await expect(page.getByText(item).first()).toBeVisible();
});
