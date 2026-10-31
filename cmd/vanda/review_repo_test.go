package main_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 段 2 の API と E2E のテストで使う、一時的な git のリポジトリ。
// main ブランチに、合意済の約束・用語・機能と、シナリオ・TLA+・ガードレールの定義・Go のテストを置き、
// story-24 ブランチで、ストーリー #24 が約束を変え・足し、シナリオを変える。HEAD は story-24 にある。
// 指紋は、docs/formats.md の決まった形の JSON を手で書いて作る（約束リストを読んで作らない）。

const (
	reviewUserName  = "合意する人"
	reviewUserEmail = "po@example.com"
	reviewStory     = "#24"
	reviewBranch    = "story-24"
)

const reviewConfig = `version = %q

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[layer]]
id = "db"
name = "DB"
description = "保存"

[[annotation]]
paths = ["e2e/*.txtar"]
adapter = "txtar"

[[annotation]]
paths = ["spec/*.tla"]
adapter = "tla"

[[annotation]]
paths = ["db/*.sql"]
adapter = "sql"

[[annotation]]
paths = ["**/*_test.go"]
adapter = "go"

[[guardrail]]
adapter = "file"
paths = ["db/schema.sql"]
`

// 約束リストのコメント・空行・項目の順は、合意でフラグを外したときに変わらないことを確かめるのに使う。
const reviewPromisesBase = `# 約束リスト（テストの例）

[[term]]
id = "task"
name = "タスク"
definition = "やること"

[[term]]
id = "task-id"
name = "タスクの ID"
definition = "タスクを見分ける名前"
part_of = "task"

[[feature]]
id = "add-task"
name = "タスクの登録"
definition = "タスクを足す"

# ID の重なり
[[claim]]
id = "task-id-unique"
subjects = ["task-id"]
statement = "タスクの ID は重ならない"
layers = ["model", "db"]

[[claim.evidence]]
stage = "test"
type = "scenario-test"
layers = ["model"]

[[claim.evidence]]
stage = "plan"
type = "model-check"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "guardrail"
layers = ["db"]

[[claim.evidence]]
stage = "test"
type = "unit-test"
layers = ["model"]
`

const reviewPromisesStory = `# 約束リスト（テストの例）

[[term]]
id = "task"
name = "タスク"
definition = "やること"

[[term]]
id = "task-id"
name = "タスクの ID"
definition = "タスクを見分ける名前"
part_of = "task"

[[feature]]
id = "add-task"
name = "タスクの登録"
definition = "タスクを足す"

# ID の重なり
[[claim]]
id = "task-id-unique"
subjects = ["task-id"]
statement = "タスクの ID は、すべてのタスクを通して重ならない"
layers = ["model", "db"]
# AI が足した言い回し
flags = ["ai-proposal"]
guesses = ["タスクは消しても ID を使い回さない"]

[[claim.evidence]]
stage = "test"
type = "scenario-test"
layers = ["model"]

[[claim.evidence]]
stage = "plan"
type = "model-check"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "guardrail"
layers = ["db"]

[[claim.evidence]]
stage = "test"
type = "unit-test"
layers = ["model"]

[[claim]]
id = "task-title-required"
subjects = ["add-task"]
statement = "タスクの題は空にできない"
layers = ["model"]
flags = ["ai-proposal"]
guesses = ["題の長さの上限はまだ決めていない"]

[[claim.evidence]]
stage = "test"
type = "scenario-test"
layers = ["model"]
`

const reviewScenarioBase = `# @vanda:evidence task-id-unique
vanda-task add a
vanda-task add a
! stderr .
-- a.txt --
a
`

const reviewScenarioStory = `# @vanda:evidence task-id-unique
vanda-task add a
! vanda-task add a
stderr 'ID a はもうある'
-- a.txt --
a
`

const reviewTLA = `---- MODULE Task ----
EXTENDS Naturals

\* @vanda:claim task-id-unique
Unique == \A a, b \in DOMAIN tasks : tasks[a] = tasks[b] => a = b

Next == /\ tasks' = [tasks EXCEPT ![1] = "x"]
        \/ ~(tasks = <<>>)
====
`

const reviewSchema = `CREATE TABLE task (id TEXT NOT NULL, title TEXT NOT NULL);

-- @vanda:evidence task-id-unique
CREATE UNIQUE INDEX task_id_unique ON task (id);
`

const reviewGoTest = `package task

import "testing"

// @vanda:evidence task-id-unique
func TestUnique(t *testing.T) {}
`

// 手で書いた、指紋の元の JSON。
var reviewItemJSON = map[string]string{
	"term task":        `{"definition":"やること","id":"task","name":"タスク"}`,
	"term task-id":     `{"definition":"タスクを見分ける名前","id":"task-id","name":"タスクの ID","part_of":"task"}`,
	"feature add-task": `{"definition":"タスクを足す","id":"add-task","name":"タスクの登録"}`,
	"claim task-id-unique": `{"evidence":[{"layers":["model"],"stage":"test","type":"scenario-test"},` +
		`{"layers":["model"],"stage":"plan","type":"model-check"},` +
		`{"layers":["db"],"stage":"implementation","type":"guardrail"},` +
		`{"layers":["model"],"stage":"test","type":"unit-test"}],` +
		`"id":"task-id-unique","layers":["model","db"],"statement":"タスクの ID は重ならない","subjects":["task-id"]}`,
}

// ストーリー #24 の後の中身の、指紋の元の JSON。flags と guesses は含めない。
var reviewStoryItemJSON = map[string]string{
	"claim task-id-unique": `{"evidence":[{"layers":["model"],"stage":"test","type":"scenario-test"},` +
		`{"layers":["model"],"stage":"plan","type":"model-check"},` +
		`{"layers":["db"],"stage":"implementation","type":"guardrail"},` +
		`{"layers":["model"],"stage":"test","type":"unit-test"}],` +
		`"id":"task-id-unique","layers":["model","db"],"statement":"タスクの ID は、すべてのタスクを通して重ならない","subjects":["task-id"]}`,
	"claim task-title-required": `{"evidence":[{"layers":["model"],"stage":"test","type":"scenario-test"}],` +
		`"id":"task-title-required","layers":["model"],"statement":"タスクの題は空にできない","subjects":["add-task"]}`,
}

// ストーリー #24 のブランチごとの見方（ベースブランチ main）で、一覧に出るもの。
var reviewStoryEntries = []rvEntry{
	{Kind: "claim", Key: "task-id-unique", Change: "changed"},
	{Kind: "claim", Key: "task-title-required", Change: "added"},
	{Kind: "file", Key: "e2e/add-task.txtar", Change: "changed"},
}

type reviewRepo struct {
	t    *testing.T
	Root string
	// Env は、vanda を動かすときの環境。合意する人は、リポジトリの git の設定（user.name・user.email）からだけ決まる。
	Env []string
	g   *rvGit
}

// reviewBaseEnv は、動かしているマシンの git の設定と、作者を決める環境の変数を除いた環境を返す。
// EMAIL を除くのは、user.email がないときに git がこれを使うため。
func reviewBaseEnv(root string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "EMAIL=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CEILING_DIRECTORIES="+filepath.Dir(root))
}

func newReviewRepo(t *testing.T, version string) *reviewRepo {
	t.Helper()
	root := t.TempDir()
	r := &reviewRepo{t: t, Root: root, Env: reviewBaseEnv(root)}
	// 用意のためのコミットは、合意する人と別の作者で作る
	r.g = &rvGit{t: t, root: root, env: append(reviewBaseEnv(root),
		"GIT_AUTHOR_NAME=テスト担当", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=テスト担当", "GIT_COMMITTER_EMAIL=tester@example.com")}

	r.g.run("init", "-q", "-b", "main")
	r.g.run("config", "user.name", reviewUserName)
	r.g.run("config", "user.email", reviewUserEmail)
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(reviewConfig, version))
	writeFile(t, root, ".vanda/checks/all.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n\n[[check]]\nname = \"annotation-links\"\nscope = \"all\"\n\n[[check]]\nname = \"agreement\"\nscope = \"all\"\n\n[[check]]\nname = \"agreement-targets\"\nscope = \"all\"\n\n[[check]]\nname = \"unread-files\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/checks/merge.toml", "[[check]]\nname = \"story-origin\"\nscope = \"story\"\n\n[[check]]\nname = \"agreement\"\nscope = \"story\"\n")
	writeFile(t, root, ".vanda/promises.toml", reviewPromisesBase)
	writeFile(t, root, "e2e/add-task.txtar", reviewScenarioBase)
	writeFile(t, root, "spec/Task.tla", reviewTLA)
	writeFile(t, root, "db/schema.sql", reviewSchema)
	writeFile(t, root, "internal/task/task_test.go", reviewGoTest)
	var agreements strings.Builder
	for _, k := range []string{"term task", "term task-id", "feature add-task", "claim task-id-unique"} {
		kind, id, _ := strings.Cut(k, " ")
		agreements.WriteString(agreementTOML(kind, id, sha256Hex([]byte(reviewItemJSON[k])), "", "#1", "テスト担当", "tester@example.com"))
	}
	for path, content := range map[string]string{"e2e/add-task.txtar": reviewScenarioBase, "spec/Task.tla": reviewTLA, "db/schema.sql": reviewSchema} {
		fp := sha256Hex([]byte(content))
		agreements.WriteString(agreementTOML("file", fp, fp, path, "#1", "テスト担当", "tester@example.com"))
	}
	writeFile(t, root, ".vanda/agreements.toml", agreements.String())
	r.g.run("add", "-A")
	r.g.run("commit", "-q", "-m", "#1 はじめの約束を置く")

	r.g.run("switch", "-q", "-c", reviewBranch)
	writeFile(t, root, ".vanda/promises.toml", reviewPromisesStory)
	r.g.run("add", "-A")
	r.g.run("commit", "-q", "-m", reviewStory+" タスクの ID の約束を変え、題の約束を足す")
	writeFile(t, root, "e2e/add-task.txtar", reviewScenarioStory)
	r.g.run("add", "-A")
	r.g.run("commit", "-q", "-m", reviewStory+" 重なった ID を拒むシナリオにする")
	return r
}

// unsetUser は、リポジトリの git の設定から user.name か user.email を消す。
func (r *reviewRepo) unsetUser(key string) {
	r.g.run("config", "--unset", key)
}

func (r *reviewRepo) git(args ...string) string { return r.g.run(args...) }

// vanda は、リポジトリの根で vanda を動かし、終了コードと標準出力・標準エラーを返す。
func (r *reviewRepo) vanda(bin string, args ...string) (int, string, string) {
	r.t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = r.Root
	cmd.Env = r.Env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, stdout.String(), stderr.String()
	case errors.As(err, &exitErr):
		return exitErr.ExitCode(), stdout.String(), stderr.String()
	default:
		r.t.Fatalf("vanda %v を動かせない: %v", args, err)
		return 0, "", ""
	}
}

// 用意したリポジトリが、段 1 の審査から見て思ったとおりの中身であることを確かめる。
// main では審査が通り、story-24 では #24 の変えたものだけが合意の審査で止まり、それらに手で合意すれば通る。
func Test段2のテストで使うリポジトリはmainで審査が通りストーリーの変えたものだけが止まる(t *testing.T) {
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)

	r.git("switch", "-q", "main")
	if code, out, errs := r.vanda(bin, "check", ".vanda/checks/all.toml"); code != 0 {
		t.Fatalf("main で審査が通らない（%d）\n%s%s", code, out, errs)
	}
	r.git("switch", "-q", reviewBranch)

	code, out, errs := r.vanda(bin, "check", ".vanda/checks/merge.toml", "--base", "main")
	if code != 1 {
		t.Fatalf("story-24 で合意の審査が止まらない（%d）\n%s%s", code, out, errs)
	}
	if !strings.Contains(out, "審査 story-origin（story）: 通った") || !strings.Contains(out, "審査 agreement（story #24）: 失敗") {
		t.Errorf("審査の結果の行が思ったものでない\n%s", out)
	}
	for _, e := range reviewStoryEntries {
		if !strings.Contains(errs, e.Key) {
			t.Errorf("合意の審査の誤りに %s が出ていない\n%s", e.Key, errs)
		}
	}
	for _, unchanged := range []string{"spec/Task.tla", "db/schema.sql", `"task"`, "add-task\""} {
		if strings.Contains(errs, unchanged) {
			t.Errorf("変えていない %s が、合意の審査の誤りに出た\n%s", unchanged, errs)
		}
	}

	// 手で合意する（フラグを外し、合意の記録を書き足す）と通る
	writeFile(t, r.Root, ".vanda/promises.toml", strings.NewReplacer(
		"flags = [\"ai-proposal\"]\n", "",
		"guesses = [\"タスクは消しても ID を使い回さない\"]\n", "",
		"guesses = [\"題の長さの上限はまだ決めていない\"]\n", "",
	).Replace(reviewPromisesStory))
	f, err := os.OpenFile(filepath.Join(r.Root, ".vanda", "agreements.toml"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	for k, j := range reviewStoryItemJSON {
		kind, id, _ := strings.Cut(k, " ")
		_, err = f.WriteString(agreementTOML(kind, id, sha256Hex([]byte(j)), "", reviewStory, reviewUserName, reviewUserEmail))
		if err != nil {
			t.Fatal(err)
		}
	}
	fp := sha256Hex([]byte(reviewScenarioStory))
	if _, err := f.WriteString(agreementTOML("file", fp, fp, "e2e/add-task.txtar", reviewStory, reviewUserName, reviewUserEmail)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := r.vanda(bin, "check", ".vanda/checks/merge.toml", "--base", "main"); code != 0 {
		t.Fatalf("手で合意しても審査が通らない（%d）\n%s%s", code, out, errs)
	}
	if code, out, errs := r.vanda(bin, "check", ".vanda/checks/all.toml"); code != 0 {
		t.Fatalf("手で合意しても、全部の範囲の審査が通らない（%d）\n%s%s", code, out, errs)
	}
}
