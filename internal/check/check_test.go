package check

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/guardrail"
	"github.com/TadahiroYamamura/vanda/internal/promises"
)

type repoT struct {
	t    *testing.T
	root string
}

func newRepo(t *testing.T) *repoT {
	t.Helper()
	r := &repoT{t: t, root: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	return r
}

func (r *repoT) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=T", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = r.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (r *repoT) write(files map[string]string) {
	r.t.Helper()
	for name, body := range files {
		p := filepath.Join(r.root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

// commit は、作業ツリーのすべてを 1 つのコミットにする。story が空でなければ、メッセージの末尾に Story: のトレーラーを書く。
// 同じ大きさのファイルを同じ時刻のうちに書き換えると、git add -A が stat の一致から変わっていないとみなし、書き換えを
// コミットに入れないことがあるので、索引から一度外して、毎回すべてを読み直させる
func (r *repoT) commit(subject, story string) {
	r.t.Helper()
	msg := subject
	if story != "" {
		msg += "\n\nStory: " + story
	}
	r.commitMsg(msg)
}

// commitMsg は、作業ツリーのすべてを、メッセージ msg のままでコミットする。読み直させる理由は commit と同じ
func (r *repoT) commitMsg(msg string) {
	r.t.Helper()
	r.git("rm", "-r", "-q", "--cached", "--ignore-unmatch", ".")
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", msg)
}

// agreeAll は、今の約束リストの項目と合意の対象のファイルのすべてに、story で合意した記録を書く。
// 合意の記録の指紋は、指紋の作り方そのものを別のテスト（agreements）で確かめたうえで、ここでは入力を作るためだけに使う。
func (r *repoT) agreeAll(story string, cfg *config.Config, extra string) {
	r.t.Helper()
	files := r.files()
	l, errs := promises.Load(r.root, files, cfg.Promises, cfg.PromisesDefault)
	if errs != nil {
		r.t.Fatal(errs)
	}
	var b strings.Builder
	b.WriteString(extra)
	add := func(kind, key, fp, path string) {
		b.WriteString("[[agreement]]\nkind = \"" + kind + "\"\nkey = \"" + key + "\"\nfingerprint = \"" + fp + "\"\n")
		if path != "" {
			b.WriteString("path = \"" + path + "\"\n")
		}
		b.WriteString("user = { name = \"PO\", email = \"po@example.com\" }\nagreed_at = 2026-10-31T12:00:00+09:00\nstory = \"" + story + "\"\n\n")
	}
	for _, it := range items(l) {
		fp, err := agreements.ItemFingerprint(it.raw)
		if err != nil {
			r.t.Fatal(err)
		}
		add(it.kind, it.id, fp, "")
	}
	guards, _ := guardrail.Files(files, cfg.Guardrails)
	e := &env{root: r.root, cfg: cfg, files: files, guards: guards}
	for _, f := range e.targetFiles() {
		src, err := os.ReadFile(filepath.Join(r.root, f))
		if err != nil {
			r.t.Fatal(err)
		}
		fp := agreements.FileFingerprint(src)
		add("file", fp, fp, f)
	}
	r.write(map[string]string{".vanda/agreements.toml": b.String()})
}

func (r *repoT) files() []string {
	r.t.Helper()
	var out []string
	err := filepath.Walk(r.root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(r.root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

func (r *repoT) config() *config.Config {
	r.t.Helper()
	cfg, errs := config.Load(r.root)
	if errs != nil {
		r.t.Fatal(errs)
	}
	return cfg
}

func (r *repoT) run(checks, base, story string) (bool, string, error) {
	r.t.Helper()
	specs, errs := LoadSpecs("checks.toml", []byte(checks))
	if errs != nil {
		r.t.Fatal(errs)
	}
	var out, errOut bytes.Buffer
	pass, err := Run(Options{Root: r.root, Cfg: r.config(), Specs: specs, Base: base, Story: story, Out: &out, Err: &errOut})
	// 多くのテストは結果の行と誤りの中身をまとめて確かめるので、両方をつないで返す。出し分けは別のテストで確かめる
	return pass, out.String() + errOut.String(), err
}

const baseConfig = `version = "0.1.0"

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[layer]]
id = "db"
name = "DB"
description = "保存"

[[annotation]]
paths = ["**/*_test.go"]
adapter = "go"
types = ["unit-test", "cross-check", "random-check", "interface-test", "concurrency-test", "sensor"]
[[annotation]]
paths = ["e2e/*.txtar"]
adapter = "txtar"
types = ["scenario-test"]
[[annotation]]
paths = ["spec/*.tla"]
adapter = "tla"
types = ["model-check"]
[[annotation]]
paths = ["db/*.sql"]
adapter = "sql"
types = ["guardrail"]
[[guardrail]]
adapter = "file"
paths = ["db/schema.sql"]
`

const basePromises = `[[term]]
id = "task"
name = "タスク"
definition = "やること"

[[feature]]
id = "add-task"
name = "タスクの登録"
definition = "タスクを足す"

[[claim]]
id = "task-unique"
subjects = ["task"]
statement = "タスクは重ならない"
layers = ["model", "db"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
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
stage = "implementation"
type = "human-review"
layers = ["model"]
`

var baseFiles = map[string]string{
	".vanda/config.toml":   baseConfig,
	".vanda/promises.toml": basePromises,
	"x/task_test.go":       "package x\n\n// @vanda:evidence task-unique\nfunc TestUnique(t *testing.T) {}\n",
	"e2e/add.txtar":        "# @vanda:evidence task-unique\nexec vanda version\n-- a.txt --\n@vanda:evidence nothing\n",
	"spec/Task.tla":        "---- MODULE Task ----\n\\* @vanda:term task\nTasks == {1, 2}\nVARIABLE tasks\nTypeOK == tasks \\subseteq Tasks\n\\* @vanda:claim task-unique\nUnique == TRUE\n====\n",
	"db/schema.sql":        "-- @vanda:evidence task-unique\nCREATE UNIQUE INDEX t ON task(id);\n",
}

const allChecks = `
[[check]]
name = "promise-list-shape"
scope = "all"
[[check]]
name = "annotation-links"
scope = "all"
[[check]]
name = "agreement"
scope = "all"
[[check]]
name = "agreement-targets"
scope = "all"
[[check]]
name = "evidence-present"
scope = "all"
[[check]]
name = "unread-files"
scope = "all"
`

func goodRepo(t *testing.T) *repoT {
	r := newRepo(t)
	r.write(baseFiles)
	r.agreeAll("#1", r.config(), "")
	r.commit("(feat) はじめ", "#1")
	return r
}

// @vanda:evidence check-evidence-proof-and-unchecked
func TestRunは整った約束リストと合意とエビデンスなら全部の審査を通す(t *testing.T) {
	r := goodRepo(t)
	pass, out, err := r.run(allChecks, "", "")
	if err != nil || !pass {
		t.Fatalf("pass=%v err=%v\n%s", pass, err, out)
	}
	want := `審査 promise-list-shape（all）: 通った
審査 annotation-links（all）: 通った
審査 agreement（all）: 通った
審査 agreement-targets（all）: 通った
審査 evidence-present（all）: 通った
審査 unread-files（all）: 通った
`
	if out != want {
		t.Errorf("got:\n%s", out)
	}
}

// expect は、出力に、want の 1 つずつについて、w[0] で始まり w[1:] をこの順に含む行があるかを確かめる。
func expect(t *testing.T, out string, want [][]string) {
	t.Helper()
	for _, w := range want {
		if !lineWith(out, w[0], w[1:]...) {
			t.Errorf("出力に、%q で始まり %q を含む行がない:\n%s", w[0], w[1:], out)
		}
	}
}

// @vanda:evidence promise-list-part-of
// @vanda:evidence promise-list-id-unique
// @vanda:evidence promise-list-id-shape
// @vanda:evidence promise-list-formal-waiver
// @vanda:evidence promise-list-subjects-exist
// @vanda:evidence promise-list-layers-known
// @vanda:evidence promise-list-every-layer-has-evidence
// @vanda:evidence promise-list-fixed-values
// @vanda:evidence promise-list-evidence-layers-within-claim
func TestPromiseListShapeは値とIDと指す先の誤りを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{".vanda/promises.toml": `[[term]]
id = "task"
name = "タスク"
definition = "やること"
part_of = "part"

[[term]]
id = "part"
name = "部分"
definition = "一部"
part_of = "task"

[[feature]]
id = "task"
name = "重なり"
definition = "x"

[[feature]]
id = "2nd"
name = "n"
definition = "d"
core = true
formal = false

[[feature]]
id = "plain"
name = "n"
definition = "d"
formal_waiver = "要らない"

[[claim]]
id = "c"
subjects = ["c", "nothing"]
statement = "s"
layers = ["model", "ui"]
flags = ["ai-proposal", "maybe"]

[[claim.evidence]]
stage = "someday"
type = "e2e"
layers = ["db"]
`})
	r.commit("(test) 約束リストを誤らせる", "")
	pass, out, _ := r.run("[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n", "", "")
	if pass {
		t.Fatal("通ってしまった")
	}
	expect(t, out, [][]string{
		{"審査 promise-list-shape（all）: 失敗（"},
		{".vanda/promises.toml:5: ", "part_of", "循環", "（task → part → task）"},
		{".vanda/promises.toml:11: ", "part_of", "循環", "（part → task → part）"},
		{".vanda/promises.toml:14: ", `ID "task"`, ".vanda/promises.toml:2 の用語（task）", "重な"},
		{".vanda/promises.toml:19: ", `機能の ID "2nd"`, "決まった形"},
		{".vanda/promises.toml:23: ", "コア機能", "formal = false", "formal_waiver"},
		{".vanda/promises.toml:29: ", "formal_waiver を書けるのは", "core = true かつ formal = false"},
		{".vanda/promises.toml:33: ", `subjects の "c"`, "用語にも機能にも", "（約束の ID）"},
		{".vanda/promises.toml:33: ", `subjects の "nothing"`, "用語にも機能にも"},
		{".vanda/promises.toml:35: ", `レイヤー "ui"`, "設定に"},
		{".vanda/promises.toml:35: ", `レイヤー "model"`, "エビデンス"},
		{".vanda/promises.toml:36: ", `フラグ "maybe"`, "決まった値"},
		{".vanda/promises.toml:39: ", `ステージ "someday"`, "決まった値"},
		{".vanda/promises.toml:40: ", `タイプ "e2e"`, "決まった値"},
		{".vanda/promises.toml:41: ", `エビデンスのレイヤー "db"`, "約束の layers"},
	})
}

// @vanda:evidence annotation-links-tla-types
// @vanda:evidence annotation-links-kinds
func TestAnnotationLinksは指す先のないIDと種類の違いと型の用語の抜けを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{
		"x/more_test.go": "package x\n\n// @vanda:evidence task\nfunc TestA(t *testing.T) {}\n\n// @vanda:sensor nothing\nfunc TestB(t *testing.T) {}\n",
		"spec/Task.tla":  "---- MODULE Task ----\n\\* @vanda:feature add-task\nTasks == {1, 2}\nVARIABLE tasks\nTypeOK == tasks \\subseteq Tasks\n\\* @vanda:claim task-unique\nUnique == TRUE\n====\n",
	})
	r.commit("(test) アノテーションを誤らせる", "")
	_, out, _ := r.run("[[check]]\nname = \"annotation-links\"\nscope = \"all\"\n", "", "")
	expect(t, out, [][]string{
		{"spec/Task.tla:2: ", "仕様の型 Tasks", "TypeOK", "@vanda:feature"},
		{"spec/Task.tla:3: ", "仕様の型 Tasks", "TypeOK", "@vanda:term", "@vanda:internal"},
		{"x/more_test.go:3: ", `@vanda:evidence の "task"`, "用語の ID", "約束の ID"},
		{"x/more_test.go:6: ", `@vanda:sensor の "nothing"`, "約束リストの約束"},
	})
}

// @vanda:evidence check-agreement-targets
// @vanda:evidence check-agreement-flags-left
func TestAgreementは提案中とフラグと合意していないファイルを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{
		".vanda/promises.toml": strings.Replace(basePromises, `statement = "タスクは重ならない"`, "statement = \"タスクは決して重ならない\"\nguesses = [\"g\"]", 1),
		"e2e/add.txtar":        "# @vanda:evidence task-unique\nexec vanda version\n",
	})
	r.commit("(test) 約束とシナリオを変える", "")
	_, out, _ := r.run("[[check]]\nname = \"agreement\"\nscope = \"all\"\n", "", "")
	expect(t, out, [][]string{
		{"審査 agreement（all）: 失敗（3 件）"},
		{".vanda/promises.toml:12: ", "約束 task-unique の今の中身", "合意", "提案中", "sha256:"},
		{".vanda/promises.toml:15: ", "約束 task-unique", "推測（guesses）"},
		{"e2e/add.txtar: ", "ファイルの今の中身", "合意", "提案中", "sha256:"},
	})
}

// @vanda:evidence agreements-point-to-existing-items
func TestAgreementTargetsは約束リストにない項目を指す合意を出す(t *testing.T) {
	r := goodRepo(t)
	cfg := r.config()
	r.agreeAll("#1", cfg, "[[agreement]]\nkind = \"claim\"\nkey = \"gone\"\nfingerprint = \"sha256:0000000000000000000000000000000000000000000000000000000000000000\"\nuser = { name = \"PO\", email = \"po@example.com\" }\nagreed_at = 2026-10-31T12:00:00Z\nstory = \"#1\"\n\n")
	r.commit("(test) ない約束への合意を足す", "")
	_, out, _ := r.run("[[check]]\nname = \"agreement-targets\"\nscope = \"all\"\n", "", "")
	expect(t, out, [][]string{{".vanda/agreements.toml:3: ", "合意の記録", `約束 "gone"`, "指して"}})
}

// @vanda:evidence check-evidence-present
// @vanda:evidence check-evidence-proof-and-unchecked
func TestEvidencePresentはタイプに合うアノテーションのない予定とproofを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{
		".vanda/promises.toml": basePromises + "\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"proof\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"sensor\"\nlayers = [\"model\"]\n",
		"db/schema.sql":        "CREATE UNIQUE INDEX t ON task(id);\n",
	})
	r.commit("(test) 予定を足しガードレールの定義のアノテーションを外す", "")
	_, out, _ := r.run("[[check]]\nname = \"evidence-present\"\nscope = \"all\"\n", "", "")
	expect(t, out, [][]string{
		{"審査 evidence-present（all）: 失敗（3 件）"},
		{".vanda/promises.toml:34: ", "約束 task-unique の guardrail のエビデンス", "types に guardrail", "@vanda:evidence task-unique"},
		{".vanda/promises.toml:44: ", "タイプ proof"},
		{".vanda/promises.toml:49: ", "約束 task-unique の sensor のエビデンス", "types に sensor", "@vanda:sensor task-unique"},
	})
}

// @vanda:evidence check-unread-files
func TestUnreadFilesは設定と審査の設定のほかの読まれていないファイルを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{".vanda/checks/push.toml": "", ".vanda/notes.md": "x", ".vanda/checks/old/a.toml": ""})
	r.commit("(test) 読まれないファイルを足す", "")
	_, out, _ := r.run("[[check]]\nname = \"unread-files\"\nscope = \"all\"\n", "", "")
	expect(t, out, [][]string{{"審査 unread-files（all）: 失敗（2 件）"}, {".vanda/checks/old/a.toml: ", ".vanda/ に", "読まれて"}, {".vanda/notes.md: ", ".vanda/ に", "読まれて"}})
}

// @vanda:evidence check-story-origin
func TestStoryOriginは約束リストを変えたコミットにストーリーのIDがなければ出す(t *testing.T) {
	r := goodRepo(t)
	r.git("checkout", "-q", "-b", "work")
	r.write(map[string]string{"README.md": "x"})
	r.commit("(docs) ID のないコミット（約束リストを変えない）", "")
	r.write(map[string]string{".vanda/promises.toml": basePromises + "\n"})
	r.commit("(feat) ID のないコミット", "")
	r.write(map[string]string{".vanda/promises.toml": basePromises + "\n\n"})
	r.commit("(feat) ID のあるコミット", "#2")
	_, out, _ := r.run("[[check]]\nname = \"story-origin\"\nscope = \"story\"\n", "main", "")
	expect(t, out, [][]string{{"審査 story-origin（story）: 失敗（1 件）"}, {"", "「(feat) ID のないコミット」", "約束リスト（.vanda/promises.toml）", "Story:", "（#[0-9]+ の形）"}})
}

const storyAgreement = "[[check]]\nname = \"agreement\"\nscope = \"story\"\n"

// @vanda:evidence check-agreement-story-scope
func TestAgreementの範囲storyはそのストーリーで変わったものだけを見る(t *testing.T) {
	r := goodRepo(t)
	r.git("checkout", "-q", "-b", "work")
	// 範囲の外（分かれた点より前から提案中）のものは見ない
	r.write(map[string]string{".vanda/agreements.toml": ""})
	r.commit("(chore) 合意を消す", "#2")
	pass, out, err := r.run(storyAgreement, "main", "")
	if err != nil || !pass {
		t.Fatalf("pass=%v err=%v\n%s", pass, err, out)
	}
	r.write(map[string]string{".vanda/promises.toml": strings.Replace(basePromises, "やること", "すること", 1)})
	_, out, _ = r.run(storyAgreement, "main", "")
	expect(t, out, [][]string{{"審査 agreement（story #2）: 失敗（1 件）"}, {".vanda/promises.toml:2: ", "用語 task の今の中身", "合意"}})
}

// @vanda:evidence check-agreement-story-scope
func TestAgreementの範囲storyはほかのストーリーが合意してもそのストーリーの外れた合意で止める(t *testing.T) {
	r := goodRepo(t)
	r.git("checkout", "-q", "-b", "work")
	changed := strings.Replace(basePromises, "やること", "すること", 1)
	r.write(map[string]string{".vanda/promises.toml": changed})
	cfg := r.config()
	// 今の中身には #2 が合意し、#1 の前の合意は外れた
	old, err := os.ReadFile(filepath.Join(r.root, ".vanda/agreements.toml"))
	if err != nil {
		t.Fatal(err)
	}
	r.agreeAll("#2", cfg, string(old))
	r.commit("(feat) 用語を変える", "#2")
	pass, out, _ := r.run(storyAgreement, "main", "")
	if !pass {
		t.Fatalf("#2 の範囲は通るはず:\n%s", out)
	}
	_, out, _ = r.run(storyAgreement, "main", "#1")
	expect(t, out, [][]string{{"審査 agreement（story #1）: 失敗（1 件）"}, {".vanda/promises.toml:2: ", "用語 task の", "ストーリー #1 の合意が外れ"}})
}

// @vanda:evidence agreed-when-fingerprint-matches
// @vanda:evidence old-agreements-kept-as-history
// @vanda:evidence detached-agreement
// @vanda:evidence detached-until-story-reagrees
// @vanda:evidence revoke-returns-to-proposed
func Test指紋の合わない合意の記録は誤りにせず外れた合意を合意し直すまで残し記録を除くと提案中に戻す(t *testing.T) {
	r := goodRepo(t)
	r.git("checkout", "-q", "-b", "work")
	r.write(map[string]string{".vanda/promises.toml": strings.Replace(basePromises, "やること", "すること", 1)})
	cfg := r.config()
	readAgreements := func() string {
		b, err := os.ReadFile(filepath.Join(r.root, ".vanda/agreements.toml"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	old := readAgreements()
	allAgreement := "[[check]]\nname = \"agreement\"\nscope = \"all\"\n[[check]]\nname = \"agreement-targets\"\nscope = \"all\"\n"

	// 変えただけでは、今の中身に合う記録がないので提案中
	_, out, _ := r.run(allAgreement, "", "")
	expect(t, out, [][]string{{"審査 agreement（all）: 失敗（1 件）"}, {".vanda/promises.toml:2: ", "用語 task の今の中身", "まだ合意していません"}, {"審査 agreement-targets（all）: 通った"}})

	// #2 が今の中身に合意すると合意済。#1 の古い記録は指紋が合わなくても誤りにならない
	r.agreeAll("#2", cfg, old)
	withTwo := readAgreements()
	pass, out, _ := r.run(allAgreement, "", "")
	if !pass {
		t.Fatalf("#2 の合意で全部の範囲が通るはず:\n%s", out)
	}
	r.commit("(feat) 用語を変える", "#2")
	// #1 は前に合意し、今の中身に #1 の合意がないので外れたまま（#2 が合意していても）
	_, out, _ = r.run(storyAgreement, "main", "#1")
	expect(t, out, [][]string{{"審査 agreement（story #1）: 失敗（1 件）"}, {".vanda/promises.toml:2: ", "用語 task の", "ストーリー #1 の合意が外れ"}})

	// #1 が今の中身に合意し直すと、#1 の範囲も通る
	r.agreeAll("#1", cfg, withTwo)
	if pass, out, _ := r.run(storyAgreement, "main", "#1"); !pass {
		t.Fatalf("#1 が合意し直したら通るはず:\n%s", out)
	}

	// #2 の記録を除いても、#1 の今の中身への記録が残るので合意済のまま
	r.agreeAll("#1", cfg, old)
	if pass, out, _ := r.run(allAgreement, "", ""); !pass {
		t.Fatalf("#2 の記録を除いても #1 の記録で合意済のはず:\n%s", out)
	}
	// 今の中身に合う記録をすべて除くと、提案中に戻る
	r.write(map[string]string{".vanda/agreements.toml": old})
	_, out, _ = r.run(allAgreement, "", "")
	expect(t, out, [][]string{{"審査 agreement（all）: 失敗（1 件）"}, {".vanda/promises.toml:2: ", "用語 task の今の中身", "まだ合意していません"}})
}

// @vanda:evidence check-base-required-for-story
// @vanda:evidence check-story-from-commits
// @vanda:evidence check-story-arg-pattern
func TestRunは範囲storyのストーリーが決まらなければ止める(t *testing.T) {
	r := goodRepo(t)
	r.git("checkout", "-q", "-b", "work")
	if _, _, err := r.run(storyAgreement, "", ""); err == nil || !lineWith(err.Error(), "", "ベースブランチ", "--base <ブランチ>") {
		t.Errorf("--base なし: %v", err)
	}
	r.commit("(chore) ID なし", "")
	if _, _, err := r.run(storyAgreement, "main", ""); err == nil || !lineWith(err.Error(), "", "Story: のトレーラー", "--story <ID>") {
		t.Errorf("ID 0 個: %v", err)
	}
	r.commit("(chore) 直す", "#3")
	r.commit("(chore) さらに直す", "#4")
	if _, _, err := r.run(storyAgreement, "main", ""); err == nil || !lineWith(err.Error(), "", "ストーリーの ID が #3・#4", "--story <ID>") {
		t.Errorf("ID 2 個: %v", err)
	}
	if _, _, err := r.run(storyAgreement, "main", "3"); err == nil || !strings.Contains(err.Error(), "--story 3 が、ストーリーの ID の書き方") {
		t.Errorf("--story の形: %v", err)
	}
	if _, _, err := r.run("[[check]]\nname = \"story-origin\"\nscope = \"story\"\n", "main", ""); err != nil {
		t.Errorf("story-origin はストーリーを決めない: %v", err)
	}
}

// @vanda:evidence check-scope-per-check
func TestLoadSpecsは審査の名前と範囲の誤りを出す(t *testing.T) {
	_, errs := LoadSpecs("c.toml", []byte("[[check]]\nname = \"x\"\nscope = \"all\"\n[[check]]\nname = \"story-origin\"\nscope = \"all\"\n[[check]]\nname = \"agreement\"\nscope = \"some\"\n"))
	expect(t, errs.Error(), [][]string{{"c.toml:2: ", `審査 "x"`}, {"c.toml:6: ", "審査 story-origin", "範囲 all", "scope を story"}, {"c.toml:9: ", `範囲 "some"`, "決まった値"}})
	if _, errs := LoadSpecs("c.toml", []byte("check = []\n")); errs == nil {
		t.Error("審査が 1 つもないのに通った")
	}
}

// @vanda:evidence check-output-lines
// @vanda:evidence check-runs-in-order-and-fails
func TestRunは審査ごとの結果の行を標準出力に誤りの中身を標準エラーに出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{".vanda/notes.md": "x"})
	r.commit("(test) 読まれないファイルを足す", "")
	specs, errs := LoadSpecs("checks.toml", []byte("[[check]]\nname = \"unread-files\"\nscope = \"all\"\n[[check]]\nname = \"agreement\"\nscope = \"all\"\n"))
	if errs != nil {
		t.Fatal(errs)
	}
	var out, errOut bytes.Buffer
	pass, err := Run(Options{Root: r.root, Cfg: r.config(), Specs: specs, Out: &out, Err: &errOut})
	if err != nil || pass {
		t.Fatalf("pass=%v err=%v", pass, err)
	}
	if got := out.String(); got != "審査 unread-files（all）: 失敗（1 件）\n審査 agreement（all）: 通った\n" {
		t.Errorf("標準出力: %q", got)
	}
	if got := errOut.String(); !lineWith(got, ".vanda/notes.md: ", ".vanda/ に", "読まれて") || strings.Count(got, "\n") != 1 {
		t.Errorf("標準エラー: %q", got)
	}
}

// @vanda:evidence promise-list-id-shape
func TestPromiseListShapeは空のIDを誤りにする(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{".vanda/promises.toml": strings.Replace(basePromises, `id = "add-task"`, `id = ""`, 1)})
	r.commit("(test) 空の ID にする", "")
	_, out, _ := r.run("[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n", "", "")
	expect(t, out, [][]string{{".vanda/promises.toml:7: ", `機能の ID ""`, "決まった形"}})
}

// @vanda:evidence promise-list-id-unique
func TestPromiseListShapeは重なったIDを表の種類でなく出てきた順で後のほうの行に出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{
		".vanda/promises.toml":   "[[feature]]\nid = \"a\"\nname = \"n\"\ndefinition = \"d\"\n\n[[term]]\nid = \"a\"\nname = \"n\"\ndefinition = \"d\"\n",
		".vanda/promises/b.toml": "[[term]]\nid = \"b\"\nname = \"n\"\ndefinition = \"d\"\n",
		".vanda/promises/a.toml": "[[claim]]\nid = \"b\"\nsubjects = [\"a\"]\nstatement = \"s\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n",
		".vanda/config.toml":     strings.Replace(baseConfig, `version = "0.1.0"`, "version = \"0.1.0\"\npromises = [\".vanda/promises.toml\", \".vanda/promises/*.toml\"]", 1),
	})
	r.commit("(test) ID を重ねる", "")
	_, out, _ := r.run("[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n", "", "")
	expect(t, out, [][]string{
		{"審査 promise-list-shape（all）: 失敗（2 件）"},
		{".vanda/promises.toml:7: ", `ID "a"`, ".vanda/promises.toml:2 の機能（a）", "重な"},
		{".vanda/promises/b.toml:2: ", `ID "b"`, ".vanda/promises/a.toml:2 の約束（b）", "重な"},
	})
}

// lineWith は、got の行のうち、prefix で始まり、frags をこの順に含むものがあるかを返す。
// 誤りの文は、です・ます調などの文体の細部に頼らず、骨組み（場所と行）と、ID・意味の要の語で照合する。
func lineWith(got, prefix string, frags ...string) bool {
	for _, line := range strings.Split(got, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest, ok := line[len(prefix):], true
		for _, f := range frags {
			i := strings.Index(rest, f)
			if i < 0 {
				ok = false
				break
			}
			rest = rest[i+len(f):]
		}
		if ok {
			return true
		}
	}
	return false
}

// ---- 約束の文から作ったテスト ----

// @vanda:evidence agreements-unknown-entries-rejected
func TestAgreementTargetsは合意の記録の決まっていない表と項目を誤りにする(t *testing.T) {
	for name, extra := range map[string]string{
		"決まっていない項目": "[[agreement]]\nkind = \"term\"\nkey = \"task\"\nfingerprint = \"sha256:0000000000000000000000000000000000000000000000000000000000000000\"\nuser = { name = \"PO\", email = \"po@example.com\" }\nagreed_at = 2026-10-31T12:00:00Z\nstory = \"#1\"\nnote = \"x\"\n\n",
		"決まっていない表":  "[[memo]]\ntext = \"x\"\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := goodRepo(t)
			r.agreeAll("#1", r.config(), extra)
			r.commit("(test) 合意の記録に決まっていないものを足す", "")
			pass, out, _ := r.run("[[check]]\nname = \"agreement-targets\"\nscope = \"all\"\n", "", "")
			if pass || !lineWith(out, ".vanda/agreements.toml:") {
				t.Errorf("誤りにならない、または場所がない:\n%s", out)
			}
		})
	}
}

// @vanda:evidence annotation-links-kinds
func TestAnnotationLinksは種類ごとに指す先の表が違えば全部の組み合わせで誤りにする(t *testing.T) {
	// 種類 → 指してよい表。ID は baseFiles の用語 task・機能 add-task・約束 task-unique
	ids := map[string]string{"term": "task", "feature": "add-task", "claim": "task-unique"}
	okFor := map[string]string{"term": "term", "feature": "feature", "claim": "claim", "evidence": "claim", "sensor": "claim"}
	for kind, okTable := range okFor {
		for table, id := range ids {
			t.Run(kind+"が"+table, func(t *testing.T) {
				r := goodRepo(t)
				// どの種類も書ける .vanda/adapters/ のアダプタで読む
				r.write(map[string]string{
					".vanda/config.toml":  baseConfig + "\n[[annotation]]\npaths = [\"ext/*.ext\"]\nadapter = \"any\"\ntypes = [\"unit-test\"]\n",
					".vanda/adapters/any": "#!/bin/sh\ncat >/dev/null\necho '{\"names\":[{\"line\":1,\"name\":\"x\"}]}'\n",
					"ext/a.ext":           "@vanda:" + kind + " " + id + "\n",
				})
				if err := os.Chmod(filepath.Join(r.root, ".vanda/adapters/any"), 0o755); err != nil {
					t.Fatal(err)
				}
				r.commit("(test) アノテーションを足す", "")
				pass, out, _ := r.run("[[check]]\nname = \"annotation-links\"\nscope = \"all\"\n", "", "")
				if want := table == okTable; pass != want {
					t.Errorf("@vanda:%s が %s の ID を指して、通るのは %v のはず\n%s", kind, table, want, out)
				}
				if !pass && !lineWith(out, "ext/a.ext:1: ", "@vanda:"+kind) {
					t.Errorf("誤りが、アノテーションの行を指していない\n%s", out)
				}
			})
		}
	}
}

// @vanda:evidence check-agreement-flags-left
func TestAgreementは合意済でも約束にflagsかguessesが残っていれば誤りにする(t *testing.T) {
	for name, add := range map[string]string{"flags": "flags = [\"ai-proposal\"]", "guesses": "guesses = [\"g\"]"} {
		t.Run(name, func(t *testing.T) {
			r := goodRepo(t)
			// flags と guesses は指紋に入らないので、合意の記録は今の中身に合ったまま
			r.write(map[string]string{".vanda/promises.toml": strings.Replace(basePromises, "layers = [\"model\", \"db\"]", "layers = [\"model\", \"db\"]\n"+add, 1)})
			r.commit("(test) "+name+" を残す", "")
			pass, out, _ := r.run("[[check]]\nname = \"agreement\"\nscope = \"all\"\n", "", "")
			if pass || !lineWith(out, ".vanda/promises.toml:", "task-unique") {
				t.Errorf("%s が残っているのに通った、または約束を指していない\n%s", name, out)
			}
			if strings.Contains(out, "今の中身") {
				t.Errorf("%s だけで、今の中身が合意されていないとされた\n%s", name, out)
			}
		})
	}
}

// @vanda:evidence check-blocked-by-unreadable-input
func TestRunは読めない入力を前提にするほかの審査を読めないため行えないとして失敗にする(t *testing.T) {
	cases := []struct {
		name, file, body string
		blocked          []string
	}{
		{"約束リスト", ".vanda/promises.toml", "[[term]\n", []string{"annotation-links", "agreement", "agreement-targets", "evidence-present"}},
		{"アノテーション", "x/task_test.go", "package x\n\n// @vanda:evidence a b\nfunc TestUnique(t *testing.T) {}\n", []string{"evidence-present"}},
		{"合意の記録", ".vanda/agreements.toml", "[[agreement]\n", []string{"agreement"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := goodRepo(t)
			r.write(map[string]string{c.file: c.body})
			r.commit("(test) 読めない入力にする", "")
			_, out, _ := r.run(allChecks, "", "")
			for _, name := range c.blocked {
				if !lineWith(out, "審査 "+name+"（all）: 失敗（1 件）") {
					t.Errorf("審査 %s が、1 件の失敗になっていない\n%s", name, out)
				}
			}
			if strings.Count(out, "読めないため") < len(c.blocked) {
				t.Errorf("「読めないため行えない」の行が、前提にする審査の数（%d）より少ない\n%s", len(c.blocked), out)
			}
		})
	}
}

// @vanda:evidence renamed-file-needs-agreement
// @vanda:evidence check-agreement-targets
func TestAgreementは合意の対象のファイルの名前を変えたら古い場所に消したことへの合意を新しい場所に合意を求め中身を変えれば止める(t *testing.T) {
	r := goodRepo(t)
	src, err := os.ReadFile(filepath.Join(r.root, "e2e/add.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(r.root, "e2e/add.txtar"), filepath.Join(r.root, "e2e/added.txtar")); err != nil {
		t.Fatal(err)
	}
	agreement := "[[check]]\nname = \"agreement\"\nscope = \"all\"\n"
	r.commit("(test) 名前を変える", "#2")
	_, out, _ := r.run(agreement, "", "")
	expect(t, out, [][]string{
		{"審査 agreement（all）: 失敗（2 件）"},
		// 消したファイルには行がないので、そのファイルに合意した記録の行で示す
		{".vanda/agreements.toml:", "ファイルを消したこと", "まだ合意していません"},
		{"e2e/added.txtar: ", "ファイルの今の中身", "提案中"},
	})

	// 新しい場所の中身に合意し、古い場所を消したことに合意すると通る
	old, err := os.ReadFile(filepath.Join(r.root, ".vanda/agreements.toml"))
	if err != nil {
		t.Fatal(err)
	}
	fp := agreements.FileFingerprint(src)
	renamed := string(old) +
		"[[agreement]]\nkind = \"file\"\nkey = \"" + fp + "\"\nfingerprint = \"" + fp + "\"\npath = \"e2e/added.txtar\"\nuser = { name = \"PO\", email = \"po@example.com\" }\nagreed_at = 2026-11-01T12:00:00Z\nstory = \"#2\"\n\n" +
		"[[agreement]]\nkind = \"file\"\nkey = \"e2e/add.txtar\"\ndeleted = true\npath = \"e2e/add.txtar\"\nuser = { name = \"PO\", email = \"po@example.com\" }\nagreed_at = 2026-11-01T12:00:00Z\nstory = \"#2\"\n\n"
	r.write(map[string]string{".vanda/agreements.toml": renamed})
	r.commit("(test) 合意する", "#2")
	if pass, out, _ := r.run(agreement+"[[check]]\nname = \"agreement-targets\"\nscope = \"all\"\n", "", ""); !pass {
		t.Errorf("新しい場所に合意し、古い場所を消したことに合意しても止まる\n%s", out)
	}

	// 名前を元に戻すと、元の場所は同じ中身への記録があるので合意済。新しい場所は合意したことがあるので、消したことへの合意を求める
	if err := os.Rename(filepath.Join(r.root, "e2e/added.txtar"), filepath.Join(r.root, "e2e/add.txtar")); err != nil {
		t.Fatal(err)
	}
	r.commit("(test) 名前を元に戻す", "#2")
	_, out, _ = r.run(agreement, "", "")
	expect(t, out, [][]string{{"審査 agreement（all）: 失敗（1 件）"}, {".vanda/agreements.toml:", "ファイルを消したこと", "まだ合意していません"}})

	// 合意の対象は txtar・tla のパターンと [[guardrail]] の paths に当たるファイル。どれも中身を変えれば止まる
	for _, f := range []string{"e2e/add.txtar", "spec/Task.tla", "db/schema.sql"} {
		r := goodRepo(t)
		src, err := os.ReadFile(filepath.Join(r.root, f))
		if err != nil {
			t.Fatal(err)
		}
		r.write(map[string]string{f: string(src) + "\n"})
		r.commit("(test) 中身を変える", "#2")
		if pass, out, _ := r.run(agreement, "", ""); pass || !lineWith(out, f+": ") {
			t.Errorf("%s の中身を変えても止まらない\n%s", f, out)
		}
	}
}

// @vanda:evidence unread-files-excludes-adapters
func TestUnreadFilesはvanda_adaptersの下のファイルを除く(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{".vanda/adapters/a": "x", ".vanda/adapters/b/run": "x", ".vanda/adapters/c/d/e.toml": "x", ".vanda/adapters.toml": "x"})
	r.commit("(test) ファイルを足す", "")
	_, out, _ := r.run("[[check]]\nname = \"unread-files\"\nscope = \"all\"\n", "", "")
	if strings.Contains(out, ".vanda/adapters/") || !lineWith(out, ".vanda/adapters.toml: ") {
		t.Errorf(".vanda/adapters/ の下を除き、名前の似たほかのファイルは止めるはず\n%s", out)
	}
}

// @vanda:evidence annotation-types-empty
func TestEvidencePresentはtypesが空のannotationで読んだアノテーションでどのタイプも満たさない(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{".vanda/config.toml": strings.Replace(baseConfig, "adapter = \"go\"\ntypes = [\"unit-test\", \"cross-check\", \"random-check\", \"interface-test\", \"concurrency-test\", \"sensor\"]", "adapter = \"go\"\ntypes = []", 1)})
	_, out, _ := r.run("[[check]]\nname = \"evidence-present\"\nscope = \"all\"\n", "", "")
	if !lineWith(out, "審査 evidence-present（all）: 失敗（1 件）") || !lineWith(out, ".vanda/promises.toml:", "unit-test") {
		t.Errorf("types = [] の go のアノテーションで、unit-test が揃ったことになった\n%s", out)
	}
}
