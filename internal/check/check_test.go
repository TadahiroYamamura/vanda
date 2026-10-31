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

func (r *repoT) commit(msg string) {
	r.t.Helper()
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

[[annotation]]
paths = ["e2e/*.txtar"]
adapter = "txtar"

[[annotation]]
paths = ["spec/*.tla"]
adapter = "tla"

[[annotation]]
paths = ["db/*.sql"]
adapter = "sql"

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
stage = "test"
type = "unit-test"
layers = ["model"]

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
	r.commit("(feat) はじめ #1")
	return r
}

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

func expect(t *testing.T, out string, want []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("出力に %q がない:\n%s", w, out)
		}
	}
}

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
	pass, out, _ := r.run("[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n", "", "")
	if pass {
		t.Fatal("通ってしまった")
	}
	expect(t, out, []string{
		"審査 promise-list-shape（all）: 失敗（",
		`.vanda/promises.toml:5: part_of が循環している（task → part → task）。`,
		`.vanda/promises.toml:11: part_of が循環している（part → task → part）。`,
		`.vanda/promises.toml:14: ID "task" が、.vanda/promises.toml:2 の用語（task）と重なっている。`,
		`.vanda/promises.toml:19: 機能の ID "2nd" が決まった形でない。`,
		`.vanda/promises.toml:23: コア機能で formal = false なのに、形式手法を使わない理由（formal_waiver）がない。`,
		`.vanda/promises.toml:29: formal_waiver を書けるのは core = true かつ formal = false のときだけ。`,
		`.vanda/promises.toml:33: subjects の "c" が用語にも機能にもない（約束の ID）。`,
		`.vanda/promises.toml:33: subjects の "nothing" が用語にも機能にもない。`,
		`.vanda/promises.toml:35: レイヤー "ui" が設定にない。`,
		`.vanda/promises.toml:35: レイヤー "model" を確かめるエビデンスがない。`,
		`.vanda/promises.toml:36: フラグ "maybe" は決まった値でない。`,
		`.vanda/promises.toml:39: ステージ "someday" は決まった値でない。`,
		`.vanda/promises.toml:40: タイプ "e2e" は決まった値でない。`,
		`.vanda/promises.toml:41: エビデンスのレイヤー "db" が、約束の layers にない。`,
	})
}

func TestAnnotationLinksは指す先のないIDと種類の違いと型の用語の抜けを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{
		"x/more_test.go": "package x\n\n// @vanda:evidence task\nfunc TestA(t *testing.T) {}\n\n// @vanda:sensor nothing\nfunc TestB(t *testing.T) {}\n",
		"spec/Task.tla":  "---- MODULE Task ----\n\\* @vanda:feature add-task\nTasks == {1, 2}\nVARIABLE tasks\nTypeOK == tasks \\subseteq Tasks\n\\* @vanda:claim task-unique\nUnique == TRUE\n====\n",
	})
	_, out, _ := r.run("[[check]]\nname = \"annotation-links\"\nscope = \"all\"\n", "", "")
	expect(t, out, []string{
		`spec/Task.tla:3: 仕様の型 Tasks（TypeOK で使っている）に @vanda:feature が付いている。`,
		`spec/Task.tla:3: 仕様の型 Tasks（TypeOK で使っている）に @vanda:term も @vanda:internal もない。`,
		`x/more_test.go:3: @vanda:evidence の "task" は用語の ID で、約束の ID でない。`,
		`x/more_test.go:6: @vanda:sensor の "nothing" が約束リストの約束にない。`,
	})
}

func TestAgreementは提案中とフラグと合意していないファイルを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{
		".vanda/promises.toml": strings.Replace(basePromises, `statement = "タスクは重ならない"`, "statement = \"タスクは決して重ならない\"\nguesses = [\"g\"]", 1),
		"e2e/add.txtar":        "# @vanda:evidence task-unique\nexec vanda version\n",
	})
	_, out, _ := r.run("[[check]]\nname = \"agreement\"\nscope = \"all\"\n", "", "")
	expect(t, out, []string{
		"審査 agreement（all）: 失敗（3 件）",
		".vanda/promises.toml:12: 約束 task-unique の今の中身に合意していない（提案中。指紋 sha256:",
		".vanda/promises.toml:15: 約束 task-unique に推測（guesses）が残っている。",
		"e2e/add.txtar: ファイルの今の中身に合意していない（提案中。指紋 sha256:",
	})
}

func TestAgreementTargetsは約束リストにない項目を指す合意を出す(t *testing.T) {
	r := goodRepo(t)
	cfg := r.config()
	r.agreeAll("#1", cfg, "[[agreement]]\nkind = \"claim\"\nkey = \"gone\"\nfingerprint = \"sha256:0000000000000000000000000000000000000000000000000000000000000000\"\nuser = { name = \"PO\", email = \"po@example.com\" }\nagreed_at = 2026-10-31T12:00:00Z\nstory = \"#1\"\n\n")
	_, out, _ := r.run("[[check]]\nname = \"agreement-targets\"\nscope = \"all\"\n", "", "")
	expect(t, out, []string{`.vanda/agreements.toml:3: 合意の記録が、約束リストにない約束 "gone" を指している。`})
}

func TestEvidencePresentはタイプに合うアノテーションのない予定とproofを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{
		".vanda/promises.toml": basePromises + "\n[[claim.evidence]]\nstage = \"test\"\ntype = \"proof\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"test\"\ntype = \"sensor\"\nlayers = [\"model\"]\n",
		"db/schema.sql":        "CREATE UNIQUE INDEX t ON task(id);\n",
	})
	_, out, _ := r.run("[[check]]\nname = \"evidence-present\"\nscope = \"all\"\n", "", "")
	expect(t, out, []string{
		"審査 evidence-present（all）: 失敗（3 件）",
		".vanda/promises.toml:34: 約束 task-unique の guardrail のエビデンスがない（sql のアダプタのファイルに @vanda:evidence task-unique がない）。",
		".vanda/promises.toml:44: タイプ proof は段 1 では扱えない",
		".vanda/promises.toml:49: 約束 task-unique の sensor のエビデンスがない（go のアダプタのファイルに @vanda:sensor task-unique がない）。",
	})
}

func TestUnreadFilesは設定と審査の設定のほかの読まれていないファイルを出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{".vanda/checks/push.toml": "", ".vanda/notes.md": "x", ".vanda/checks/old/a.toml": ""})
	_, out, _ := r.run("[[check]]\nname = \"unread-files\"\nscope = \"all\"\n", "", "")
	expect(t, out, []string{"審査 unread-files（all）: 失敗（2 件）", ".vanda/checks/old/a.toml: .vanda/ にあるのに", ".vanda/notes.md: .vanda/ にあるのに"})
}

func TestStoryOriginは約束リストを変えたコミットにストーリーのIDがなければ出す(t *testing.T) {
	r := goodRepo(t)
	r.git("checkout", "-q", "-b", "work")
	r.write(map[string]string{"README.md": "x"})
	r.commit("(docs) ID のないコミット（約束リストを変えない）")
	r.write(map[string]string{".vanda/promises.toml": basePromises + "\n"})
	r.commit("(feat) ID のないコミット")
	r.write(map[string]string{".vanda/promises.toml": basePromises + "\n\n"})
	r.commit("(feat) ID のあるコミット #2")
	_, out, _ := r.run("[[check]]\nname = \"story-origin\"\nscope = \"story\"\n", "main", "")
	expect(t, out, []string{"審査 story-origin（story）: 失敗（1 件）", "「(feat) ID のないコミット」が約束リスト（.vanda/promises.toml）を変えているのに、メッセージにストーリーの ID（#[0-9]+ の形）がない"})
}

const storyAgreement = "[[check]]\nname = \"agreement\"\nscope = \"story\"\n"

func TestAgreementの範囲storyはそのストーリーで変わったものだけを見る(t *testing.T) {
	r := goodRepo(t)
	r.git("checkout", "-q", "-b", "work")
	// 範囲の外（分かれた点より前から提案中）のものは見ない
	r.write(map[string]string{".vanda/agreements.toml": ""})
	r.commit("(chore) 合意を消す #2")
	pass, out, err := r.run(storyAgreement, "main", "")
	if err != nil || !pass {
		t.Fatalf("pass=%v err=%v\n%s", pass, err, out)
	}
	r.write(map[string]string{".vanda/promises.toml": strings.Replace(basePromises, "やること", "すること", 1)})
	_, out, _ = r.run(storyAgreement, "main", "")
	expect(t, out, []string{"審査 agreement（story #2）: 失敗（1 件）", ".vanda/promises.toml:2: 用語 task の今の中身に合意していない"})
}

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
	r.commit("(feat) 用語を変える #2")
	pass, out, _ := r.run(storyAgreement, "main", "")
	if !pass {
		t.Fatalf("#2 の範囲は通るはず:\n%s", out)
	}
	_, out, _ = r.run(storyAgreement, "main", "#1")
	expect(t, out, []string{"審査 agreement（story #1）: 失敗（1 件）", ".vanda/promises.toml:2: 用語 task の、ストーリー #1 の合意が外れている"})
}

func TestRunは範囲storyのストーリーが決まらなければ止める(t *testing.T) {
	r := goodRepo(t)
	r.git("checkout", "-q", "-b", "work")
	if _, _, err := r.run(storyAgreement, "", ""); err == nil || !strings.Contains(err.Error(), "--base <ブランチ> を渡す") {
		t.Errorf("--base なし: %v", err)
	}
	r.commit("(chore) ID なし")
	if _, _, err := r.run(storyAgreement, "main", ""); err == nil || !strings.Contains(err.Error(), "ストーリーの ID がない）。--story <ID> を渡す") {
		t.Errorf("ID 0 個: %v", err)
	}
	r.commit("(chore) 2 つ #3 #4")
	if _, _, err := r.run(storyAgreement, "main", ""); err == nil || !strings.Contains(err.Error(), "ストーリーの ID が #3・#4 とある）") {
		t.Errorf("ID 2 個: %v", err)
	}
	if _, _, err := r.run(storyAgreement, "main", "3"); err == nil || !strings.Contains(err.Error(), "--story 3 が、ストーリーの ID の書き方") {
		t.Errorf("--story の形: %v", err)
	}
	if _, _, err := r.run("[[check]]\nname = \"story-origin\"\nscope = \"story\"\n", "main", ""); err != nil {
		t.Errorf("story-origin はストーリーを決めない: %v", err)
	}
}

func TestLoadSpecsは審査の名前と範囲の誤りを出す(t *testing.T) {
	_, errs := LoadSpecs("c.toml", []byte("[[check]]\nname = \"x\"\nscope = \"all\"\n[[check]]\nname = \"story-origin\"\nscope = \"all\"\n[[check]]\nname = \"agreement\"\nscope = \"some\"\n"))
	expect(t, errs.Error(), []string{`c.toml:2: 審査 "x" はない。`, "c.toml:6: 審査 story-origin は範囲 all で行えない。scope を story にする", `c.toml:9: 範囲 "some" は決まった値でない。`})
	if _, errs := LoadSpecs("c.toml", []byte("check = []\n")); errs == nil {
		t.Error("審査が 1 つもないのに通った")
	}
}

func TestRunは審査ごとの結果の行を標準出力に誤りの中身を標準エラーに出す(t *testing.T) {
	r := goodRepo(t)
	r.write(map[string]string{".vanda/notes.md": "x"})
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
	if got := errOut.String(); !strings.HasPrefix(got, ".vanda/notes.md: .vanda/ にあるのに") || strings.Count(got, "\n") != 1 {
		t.Errorf("標準エラー: %q", got)
	}
}
