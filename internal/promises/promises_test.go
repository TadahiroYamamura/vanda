package promises

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var names []string
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return root, names
}

func TestLoadは複数のファイルから用語と機能と約束を読む(t *testing.T) {
	root, files := write(t, map[string]string{
		".vanda/promises/a.toml": `[[term]]
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
core = true

[[feature]]
id = "list-task"
name = "タスクの一覧"
definition = "タスクを並べる"
core = true
formal = false
formal_waiver = "表示だけのため"
`,
		".vanda/promises/b.toml": `[[claim]]
id = "task-id-unique"
subjects = ["task-id"]
statement = "タスクの ID は重ならない"
rationale = "見分けるため"
condition = "いつも"
layers = ["model", "db"]
flags = ["ai-proposal"]
guesses = ["DB に一意の制約を付けられる"]

[[claim.evidence]]
stage = "test"
type = "unit-test"
layers = ["model"]
detail = "TestTaskID"

[[claim.evidence]]
stage = "implementation"
type = "guardrail"
layers = ["db"]
detail = "db/schema.sql"
`,
	})
	l, errs := Load(root, files, []string{".vanda/promises/*.toml"}, false)
	if errs != nil {
		t.Fatal(errs)
	}
	if len(l.Terms) != 2 || l.Terms[1].PartOf != "task" || l.Terms[0].PartOf != "" {
		t.Errorf("term: %+v", l.Terms)
	}
	if len(l.Features) != 2 || !l.Features[0].Formal || l.Features[1].Formal || l.Features[1].FormalWaiver != "表示だけのため" {
		t.Errorf("feature: formal を省いた core の機能は真、書いた機能は書いたまま: %+v %+v", l.Features[0], l.Features[1])
	}
	if len(l.Claims) != 1 {
		t.Fatalf("claim: %+v", l.Claims)
	}
	c := l.Claims[0]
	if c.ID != "task-id-unique" || len(c.Evidence) != 2 || c.Evidence[1].Detail != "db/schema.sql" || c.Flags[0] != "ai-proposal" || c.Guesses[0] != "DB に一意の制約を付けられる" {
		t.Errorf("claim: %+v", c)
	}
}

func TestLoadはcoreでない機能のformalを省けば偽とみなす(t *testing.T) {
	root, files := write(t, map[string]string{".vanda/promises.toml": "[[feature]]\nid = \"f\"\nname = \"n\"\ndefinition = \"d\"\ncore = false\n"})
	l, errs := Load(root, files, []string{".vanda/promises.toml"}, false)
	if errs != nil {
		t.Fatal(errs)
	}
	if l.Features[0].Formal {
		t.Error("core = false で formal を省いたのに真になった")
	}
}

func TestLoadは型の違いと足りない項目と決まっていない項目を行の付いたエラーで全部返す(t *testing.T) {
	root, files := write(t, map[string]string{".vanda/promises.toml": `[[claim]]
id = "c"
subjects = "task"
rationale = "r"
layers = []
note = "x"

[[claim.evidence]]
stage = "test"
layers = ["model"]

[[feature]]
id = "f"
name = "n"
definition = "d"
core = "yes"

[[story]]
id = "s"
`})
	_, errs := Load(root, files, []string{".vanda/promises.toml", ".vanda/more.toml"}, false)
	want := []string{
		`.vanda/promises.toml:3: subjects が文字列の配列でない。`,
		`.vanda/promises.toml:1: claim "c" の statement がない。statement = "…" を書く`,
		`.vanda/promises.toml:5: layers が空。`,
		`.vanda/promises.toml:8: claim "c" の evidence の 1 件目 の type がない。`,
		`.vanda/promises.toml:6: note は決まった項目でない。`,
		`.vanda/promises.toml:16: core が真偽でない。`,
		`.vanda/promises.toml:18: story は決まった項目でない。`,
		`.vanda/more.toml: 約束リストのファイルがない。`,
	}
	got := errs.Error()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("エラーに %q がない:\n%s", w, got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != len(want) {
		t.Errorf("エラーの数 %d, want %d:\n%s", n, len(want), got)
	}
}

func TestLoadはインラインの表の中の項目は行を書かずに場所を示す(t *testing.T) {
	root, files := write(t, map[string]string{".vanda/promises.toml": `[[claim]]
id = "c"
subjects = ["t"]
statement = "s"
rationale = "r"
condition = "c"
layers = ["model"]
evidence = [{ stage = "test", type = "unit-test", layers = "model", detail = "x" }]
`})
	_, errs := Load(root, files, []string{".vanda/promises.toml"}, false)
	want := `.vanda/promises.toml: claim "c" の evidence の 1 件目 の layers: layers が文字列の配列でない。`
	if !strings.Contains(errs.Error(), want) {
		t.Errorf("エラーに %q がない:\n%s", want, errs)
	}
}

func TestLoadは省いてよい項目を省いても読める(t *testing.T) {
	root, files := write(t, map[string]string{".vanda/promises.toml": `[[feature]]
id = "f"
name = "n"
definition = "d"

[[claim]]
id = "c"
subjects = ["f"]
statement = "s"
layers = ["model"]

[[claim.evidence]]
stage = "test"
type = "unit-test"
layers = ["model"]
`})
	l, errs := Load(root, files, []string{".vanda/promises.toml"}, false)
	if errs != nil {
		t.Fatal(errs)
	}
	if l.Features[0].Core || l.Features[0].Formal {
		t.Error("core を省いた機能は、core も formal も偽とみなす")
	}
}

func TestLoadはsubjectsとevidenceが空なら誤りにする(t *testing.T) {
	root, files := write(t, map[string]string{".vanda/promises.toml": "[[claim]]\nid = \"c\"\nsubjects = []\nstatement = \"s\"\nlayers = [\"m\"]\nevidence = []\n"})
	_, errs := Load(root, files, []string{".vanda/promises.toml"}, false)
	for _, w := range []string{".vanda/promises.toml:3: subjects が空。", ".vanda/promises.toml:6: evidence が 1 つもない。"} {
		if !strings.Contains(errs.Error(), w) {
			t.Errorf("エラーに %q がない:\n%s", w, errs)
		}
	}
}

func TestLoadは既定のファイルがなければ空とみなす(t *testing.T) {
	root, files := write(t, map[string]string{"README.md": "x"})
	l, errs := Load(root, files, []string{".vanda/promises.toml"}, true)
	if errs != nil || len(l.Claims) != 0 {
		t.Errorf("errs = %v", errs)
	}
}
