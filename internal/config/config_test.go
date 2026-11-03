package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".vanda"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".vanda", "config.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadは項目を読み既定の値を埋める(t *testing.T) {
	root := writeConfig(t, `version = "0.1.0"

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[annotation]]
paths = ["**/*_test.go"]
adapter = "go"
types = ["unit-test", "cross-check", "random-check", "interface-test", "concurrency-test", "sensor"]
[[annotation]]
paths = ["docs/*.md"]
adapter = "md"
args = ["--json"]
types = ["unit-test"]

[[guardrail]]
adapter = "file"
paths = ["db/schema.sql"]
`)
	// .vanda/adapters/ のアダプタ md は、.vanda/adapters/ の直下のファイル
	if err := os.MkdirAll(filepath.Join(root, ".vanda", "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".vanda", "adapters", "md"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, errs := Load(root)
	if errs != nil {
		t.Fatal(errs)
	}
	if got := strings.Join(cfg.Promises, ","); got != ".vanda/promises.toml" {
		t.Errorf("promises の既定: %s", got)
	}
	if !cfg.PromisesDefault || !cfg.AgreementsDefault {
		t.Error("promises・agreements を書かなければ既定のファイルを使う")
	}
	if got := strings.Join(cfg.Agreements, ","); got != ".vanda/agreements.toml" {
		t.Errorf("agreements の既定: %s", got)
	}
	if len(cfg.Layers) != 1 || cfg.Layers[0].ID != "model" || cfg.Layers[0].Description != "判定と状態の変化" {
		t.Errorf("layer: %+v", cfg.Layers)
	}
	if len(cfg.Annotations) != 2 || cfg.Annotations[0].Adapter != "go" || cfg.Annotations[0].User != nil {
		t.Errorf("annotation: %+v", cfg.Annotations)
	} else if u := cfg.Annotations[1].User; cfg.Annotations[1].Adapter != "" || u == nil || u.Name != "md" || u.Path != ".vanda/adapters/md" || strings.Join(u.Args, ",") != "--json" {
		t.Errorf(".vanda/adapters/ のアダプタの annotation: %+v（User %+v）", cfg.Annotations[1], u)
	}
	if len(cfg.Guardrails) != 1 || cfg.Guardrails[0].Paths[0] != "db/schema.sql" {
		t.Errorf("guardrail: %+v", cfg.Guardrails)
	}
	for s, want := range map[string]bool{"#24": true, "#": false, "PROJ-1": false, "#24a": false} {
		m := cfg.IDPattern.FindString(s)
		if (m == s) != want {
			t.Errorf("既定の id_pattern が %q に当たるか: got %q", s, m)
		}
	}
}

// @vanda:evidence adapter-name-unknown
// @vanda:evidence config-unknown-entries-rejected
// @vanda:evidence config-version-mismatch
func TestLoadはエラーを行の付いた1件1行で全部返す(t *testing.T) {
	root := writeConfig(t, `version = "0.2.0"
promises = []
extra = 1

[[layer]]
id = "Model"
name = "モデル"

[[annotation]]
paths = ["a/**/*.go"]
adapter = "rust"
types = ["unit-test"]

[story]
id_pattern = "("
`)
	_, errs := Load(root)
	want := [][]string{
		{".vanda/config.toml:1: ", "動いている vanda（0.1.0）", "設定の版（0.2.0）より古い", "vanda を 0.2.0 に上げ"},
		{".vanda/config.toml:2: ", "promises", "パターン"},
		{".vanda/config.toml:6: ", `レイヤーの ID "Model"`, "決まった形"},
		{".vanda/config.toml:5: ", `layer "Model" の description`, `description = "…"`},
		{".vanda/config.toml:11: ", `アダプタ "rust"`, "組込み", ".vanda/adapters/rust"},
		{".vanda/config.toml:15: ", "id_pattern", "正規表現"},
		{".vanda/config.toml:3: ", "extra", "決まった項目"},
	}
	got := errs.Error()
	for _, w := range want {
		if !lineWith(got, w[0], w[1:]...) {
			t.Errorf("エラーに、%q で始まり %q を含む行がない:\n%s", w[0], w[1:], got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != len(want) {
		t.Errorf("エラーの数 %d, want %d:\n%s", n, len(want), got)
	}
}

// @vanda:evidence config-version-mismatch
func TestLoadは設定の版が動いているvandaより古いと移行を知らせる(t *testing.T) {
	root := writeConfig(t, `version = "0.0.9"`)
	_, errs := Load(root)
	if !strings.Contains(errs.Error(), "移行のコマンド") {
		t.Errorf("移行を知らせていない: %s", errs)
	}
}

// @vanda:evidence promise-list-id-shape
func TestIsID(t *testing.T) {
	for s, want := range map[string]bool{"task": true, "task-id-2": true, "2a": false, "a2": true, "": false, "Task": false, "-a": false, "a-": false, "a--b": false, "a_b": false, "タスク": false} {
		if IsID(s) != want {
			t.Errorf("IsID(%q) = %v", s, !want)
		}
	}
}

// @vanda:evidence config-id-pattern-not-empty-match
func TestLoadは空の文字列に当たるid_patternを受け付けない(t *testing.T) {
	root := writeConfig(t, "version = \"0.1.0\"\n[story]\nid_pattern = \"[0-9]*\"\n")
	_, errs := Load(root)
	if !lineWith(errs.Error(), ".vanda/config.toml:3: ", "id_pattern", "空の文字列") {
		t.Errorf("エラーが違う: %v", errs)
	}
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

// annConfig は、[[annotation]] を 1 つだけ書いた設定。
func annConfig(body string) string {
	return "version = \"0.1.0\"\n\n[[annotation]]\npaths = [\"x/*\"]\n" + body
}

// @vanda:evidence annotation-types-required
// @vanda:evidence annotation-types-empty
// @vanda:evidence annotation-types-unsatisfiable-allowed
func TestLoadはtypesがないか書けないタイプを書いた設定を誤りにし空と満たせないタイプは受け付ける(t *testing.T) {
	bad := map[string]string{
		"typesがない":     "adapter = \"go\"\n",
		"human-review": "adapter = \"go\"\ntypes = [\"human-review\"]\n",
		"monitoring":   "adapter = \"go\"\ntypes = [\"monitoring\"]\n",
		"proof":        "adapter = \"go\"\ntypes = [\"proof\"]\n",
		"決まっていないタイプ":   "adapter = \"go\"\ntypes = [\"smoke-test\"]\n",
	}
	for name, body := range bad {
		t.Run(name, func(t *testing.T) {
			if _, errs := Load(writeConfig(t, annConfig(body))); errs == nil || !strings.Contains(errs.Error(), "types") && !strings.Contains(errs.Error(), name) {
				t.Errorf("誤りにならない: %v", errs)
			}
		})
	}
	// 空の配列と、組込みのアダプタが書けない種類でしか満たせないタイプ（tla に unit-test、go に model-check）は誤りにしない
	for _, body := range []string{"adapter = \"go\"\ntypes = []\n", "adapter = \"tla\"\ntypes = [\"unit-test\"]\n", "adapter = \"go\"\ntypes = [\"model-check\"]\n"} {
		cfg, errs := Load(writeConfig(t, annConfig(body)))
		if errs != nil {
			t.Errorf("%q が誤りになった: %v", body, errs)
			continue
		}
		if strings.Contains(body, "[]") && len(cfg.Annotations[0].Types) != 0 {
			t.Errorf("types = [] が空でない: %v", cfg.Annotations[0].Types)
		}
	}
}

// @vanda:evidence config-github-only-fields
func TestLoadはstoryのrepositoryとapi_urlを組込みのgithubのときだけ受け付ける(t *testing.T) {
	ok := "version = \"0.1.0\"\n[story]\nadapter = \"github\"\nrepository = \"o/r\"\napi_url = \"https://example.com\"\n"
	if _, errs := Load(writeConfig(t, ok)); errs != nil {
		t.Fatalf("github で誤りになった: %v", errs)
	}
	for name, body := range map[string]string{
		"使わないのにrepository":  "version = \"0.1.0\"\n[story]\nrepository = \"o/r\"\n",
		"使わないのにapi_url":     "version = \"0.1.0\"\n[story]\napi_url = \"https://example.com\"\n",
		"アダプタなのにrepository": "version = \"0.1.0\"\n[story]\nadapter = \"jira\"\nrepository = \"o/r\"\n",
		"アダプタなのにapi_url":    "version = \"0.1.0\"\n[story]\nadapter = \"jira\"\napi_url = \"https://example.com\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := writeConfig(t, body)
			writeAdapter(t, root, "jira", "#!/bin/sh\n")
			if _, errs := Load(root); errs == nil || !strings.Contains(errs.Error(), ".vanda/config.toml") {
				t.Errorf("誤りにならない: %v", errs)
			}
		})
	}
}

func writeAdapter(t *testing.T, root, path, body string) {
	t.Helper()
	p := filepath.Join(root, ".vanda", "adapters", filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// @vanda:evidence project-adapter-args-only-for-project
func TestLoadはargsを組込みのアダプタとadapterのない表に書けば誤りにする(t *testing.T) {
	for name, body := range map[string]string{
		"組込みのgo":      "adapter = \"go\"\ntypes = [\"unit-test\"]\nargs = [\"x\"]\n",
		"組込みのtxtar":   "adapter = \"txtar\"\ntypes = [\"scenario-test\"]\nargs = [\"x\"]\n",
		"組込みのtla":     "adapter = \"tla\"\ntypes = [\"model-check\"]\nargs = [\"x\"]\n",
		"組込みのsql":     "adapter = \"sql\"\ntypes = [\"guardrail\"]\nargs = [\"x\"]\n",
		"組込みのts":      "adapter = \"ts\"\ntypes = [\"unit-test\"]\nargs = [\"x\"]\n",
		"adapterのない表": "types = [\"unit-test\"]\nargs = [\"x\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, errs := Load(writeConfig(t, annConfig(body))); errs == nil || !strings.Contains(errs.Error(), "args") {
				t.Errorf("誤りにならない: %v", errs)
			}
		})
	}
	root := writeConfig(t, "version = \"0.1.0\"\n[story]\nadapter = \"github\"\nrepository = \"o/r\"\nargs = [\"x\"]\n")
	if _, errs := Load(root); errs == nil || !strings.Contains(errs.Error(), "args") {
		t.Errorf("組込みの github の args が誤りにならない: %v", errs)
	}
}

// @vanda:evidence config-annotation-timeout
func TestLoadはtimeoutを1以上の整数の秒でカスタムアダプタの表にだけ受け付け省けば20秒にする(t *testing.T) {
	for name, body := range map[string]string{
		"0":           "adapter = \"md\"\ntypes = [\"unit-test\"]\ntimeout = 0\n",
		"負の数":         "adapter = \"md\"\ntypes = [\"unit-test\"]\ntimeout = -1\n",
		"小数":          "adapter = \"md\"\ntypes = [\"unit-test\"]\ntimeout = 1.5\n",
		"文字列":         "adapter = \"md\"\ntypes = [\"unit-test\"]\ntimeout = \"20\"\n",
		"組込みのgo":      "adapter = \"go\"\ntypes = [\"unit-test\"]\ntimeout = 5\n",
		"組込みのtxtar":   "adapter = \"txtar\"\ntypes = [\"scenario-test\"]\ntimeout = 5\n",
		"組込みのtla":     "adapter = \"tla\"\ntypes = [\"model-check\"]\ntimeout = 5\n",
		"組込みのsql":     "adapter = \"sql\"\ntypes = [\"guardrail\"]\ntimeout = 5\n",
		"組込みのts":      "adapter = \"ts\"\ntypes = [\"unit-test\"]\ntimeout = 5\n",
		"adapterのない表": "types = [\"unit-test\"]\ntimeout = 5\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := writeConfig(t, annConfig(body))
			writeAdapter(t, root, "md", "#!/bin/sh\n")
			if _, errs := Load(root); errs == nil || !lineWith(errs.Error(), ".vanda/config.toml:", "timeout") {
				t.Errorf("timeout の行を指す誤りにならない: %v", errs)
			}
		})
	}
	for body, want := range map[string]time.Duration{
		"adapter = \"md\"\ntypes = [\"unit-test\"]\ntimeout = 1\n":  time.Second,
		"adapter = \"md\"\ntypes = [\"unit-test\"]\ntimeout = 90\n": 90 * time.Second,
		"adapter = \"md\"\ntypes = [\"unit-test\"]\n":               20 * time.Second,
	} {
		root := writeConfig(t, annConfig(body))
		writeAdapter(t, root, "md", "#!/bin/sh\n")
		cfg, errs := Load(root)
		if errs != nil {
			t.Errorf("%q が誤りになった: %v", body, errs)
			continue
		}
		if got := cfg.Annotations[0].Timeout; got != want {
			t.Errorf("%q の timeout: %v, want %v", body, got, want)
		}
	}
}

// @vanda:evidence project-adapter-name
// @vanda:evidence project-adapter-overrides-builtin
func TestLoadはvanda_adaptersの直下のファイルとディレクトリのrunを名前で指し組込みと同じ名前ならそちらを使う(t *testing.T) {
	root := writeConfig(t, "version = \"0.1.0\"\n\n[[annotation]]\npaths = [\"a/*\"]\nadapter = \"md\"\ntypes = [\"unit-test\"]\n\n[[annotation]]\npaths = [\"b/*\"]\nadapter = \"rs\"\ntypes = [\"unit-test\"]\n\n[[annotation]]\npaths = [\"c/*\"]\nadapter = \"go\"\ntypes = [\"unit-test\"]\n\n[[annotation]]\npaths = [\"d/*\"]\nadapter = \"txtar\"\ntypes = [\"scenario-test\"]\n")
	writeAdapter(t, root, "md", "#!/bin/sh\n")
	writeAdapter(t, root, "rs/run", "#!/bin/sh\n")
	writeAdapter(t, root, "go", "#!/bin/sh\n")
	cfg, errs := Load(root)
	if errs != nil {
		t.Fatal(errs)
	}
	a := cfg.Annotations
	if a[0].User == nil || a[0].User.Path != ".vanda/adapters/md" {
		t.Errorf("ファイルの形: %+v", a[0].User)
	}
	if a[1].User == nil || a[1].User.Path != ".vanda/adapters/rs/run" {
		t.Errorf("ディレクトリの形: %+v", a[1].User)
	}
	if a[2].Adapter != "" || a[2].User == nil || a[2].User.Path != ".vanda/adapters/go" {
		t.Errorf("組込みと同じ名前の go が .vanda/adapters/ のものにならない: %+v %+v", a[2], a[2].User)
	}
	if a[3].Adapter != "txtar" || a[3].User != nil {
		t.Errorf("同じ名前がなければ組込みのまま: %+v", a[3])
	}
}

// 1 行目に動かし方がないものは、動かしたときに分かるので、annotation の Read のテストで確かめる
//
// @vanda:evidence project-adapter-not-runnable
func TestUserAdapterのCheckはない実行できないrunがないアダプタを場所と直し方の誤りにする(t *testing.T) {
	root := t.TempDir()
	writeAdapter(t, root, "ok", "#!/bin/sh\n")
	writeAdapter(t, root, "norun/other", "#!/bin/sh\n")
	p := filepath.Join(root, ".vanda", "adapters", "noexec")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if e := (&UserAdapter{Name: "ok", Path: ".vanda/adapters/ok"}).Check(root); e != nil {
		t.Errorf("動かせるアダプタが誤りになった: %v", e)
	}
	for _, u := range []UserAdapter{
		{Name: "missing", Path: ".vanda/adapters/missing"},
		{Name: "noexec", Path: ".vanda/adapters/noexec"},
		{Name: "norun", Path: ".vanda/adapters/norun/run"},
	} {
		e := u.Check(root)
		if e == nil || !strings.Contains(e.Error(), u.Path) || e.Fix == "" {
			t.Errorf("%s: 場所と直し方の誤りにならない: %v", u.Name, e)
		}
	}
}
