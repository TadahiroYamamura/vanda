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
	// カスタムアダプタ md は、.vanda/adapters/ の直下のファイル
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
		t.Errorf("カスタムアダプタの annotation: %+v（User %+v）", cfg.Annotations[1], u)
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
func TestLoadは設定の版が動いているvandaより古いとvandaの形に書き換えるよう知らせてないコマンドは案内しない(t *testing.T) {
	root := writeConfig(t, `version = "0.0.9"`)
	_, errs := Load(root)
	if errs == nil || !lineWith(errs.Error(), ".vanda/config.toml:1: ", "0.0.9", ".vanda/", "書き換え") {
		t.Errorf(".vanda/ を書き換えるよう知らせていない: %v", errs)
	}
	// 移行のコマンドはまだないので、案内しない
	if errs != nil && (strings.Contains(errs.Error(), "コマンド") || strings.Contains(errs.Error(), "vanda migrate")) {
		t.Errorf("ないコマンドを案内した: %s", errs)
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

// loadErrs は、設定 body を読んだときの誤りを返す。誤りがなければ空。
func loadErrs(t *testing.T, body string) string {
	t.Helper()
	root := writeConfig(t, body)
	writeAdapter(t, root, "md", "#!/bin/sh\n")
	if _, errs := Load(root); errs != nil {
		return errs.Error()
	}
	return ""
}

// @vanda:evidence config-annotation-types-unique
func TestLoadはannotationのtypesに同じタイプを2つ以上書けば誤りにし違うタイプだけなら受け付ける(t *testing.T) {
	for name, types := range map[string]string{
		"同じタイプが2つ":    `["unit-test", "unit-test"]`,
		"同じタイプが3つ":    `["sensor", "sensor", "sensor"]`,
		"間をはさんで重なる":   `["unit-test", "random-check", "unit-test"]`,
		"2組が重なる":      `["unit-test", "sensor", "sensor", "unit-test"]`,
		"カスタムアダプタの表で": `["scenario-test", "scenario-test"]`,
	} {
		t.Run(name, func(t *testing.T) {
			adapter := "go"
			if name == "カスタムアダプタの表で" {
				adapter = "md"
			}
			got := loadErrs(t, annConfig("adapter = \""+adapter+"\"\ntypes = "+types+"\n"))
			if !lineWith(got, ".vanda/config.toml:6: ", "types", "2 つ以上") {
				t.Errorf("types の行（6 行目）を指す誤りにならない:\n%s", got)
			}
		})
	}
	if got := loadErrs(t, annConfig("adapter = \"go\"\ntypes = [\"unit-test\", \"random-check\", \"sensor\"]\n")); got != "" {
		t.Errorf("重ならない types が誤りになった:\n%s", got)
	}
}

// @vanda:evidence config-api-url-default
func TestLoadは組込みのgithubでapi_urlを省けばhttps_api_github_comを使う(t *testing.T) {
	root := writeConfig(t, "version = \"0.1.0\"\n[story]\nadapter = \"github\"\nrepository = \"o/r\"\n")
	cfg, errs := Load(root)
	if errs != nil {
		t.Fatal(errs)
	}
	if cfg.Story.APIURL != "https://api.github.com" {
		t.Errorf("api_url を省いたときの API の URL: %q", cfg.Story.APIURL)
	}
	// 書けば、書いたものを使う
	root = writeConfig(t, "version = \"0.1.0\"\n[story]\nadapter = \"github\"\nrepository = \"o/r\"\napi_url = \"https://ghe.example.com/api/v3\"\n")
	if cfg, errs = Load(root); errs != nil {
		t.Fatal(errs)
	}
	if cfg.Story.APIURL != "https://ghe.example.com/api/v3" {
		t.Errorf("api_url を書いたときの API の URL: %q", cfg.Story.APIURL)
	}
}

// @vanda:evidence config-api-url-scheme
func TestLoadはapi_urlがhttpかhttpsで始まらなければ誤りにする(t *testing.T) {
	body := func(u string) string {
		return "version = \"0.1.0\"\n[story]\nadapter = \"github\"\nrepository = \"o/r\"\napi_url = \"" + u + "\"\n"
	}
	for _, u := range []string{"api.github.com", "ftp://api.github.com", "//api.github.com", "httpx://a", "http:/a", "", " https://api.github.com"} {
		if got := loadErrs(t, body(u)); !lineWith(got, ".vanda/config.toml:5: ", "api_url") {
			t.Errorf("api_url = %q が、api_url の行（5 行目）を指す誤りにならない:\n%s", u, got)
		}
	}
	for _, u := range []string{"http://localhost:8080", "https://api.github.com", "https://ghe.example.com/api/v3/"} {
		if got := loadErrs(t, body(u)); got != "" {
			t.Errorf("api_url = %q が誤りになった:\n%s", u, got)
		}
	}
}

// @vanda:evidence config-github-repository
func TestLoadは組込みのgithubでrepositoryがないかowner_repoの形でなければ誤りにする(t *testing.T) {
	if got := loadErrs(t, "version = \"0.1.0\"\n[story]\nadapter = \"github\"\n"); !lineWith(got, ".vanda/config.toml:2: ", "repository") {
		t.Errorf("repository がないのに、[story] の行を指す誤りにならない:\n%s", got)
	}
	body := func(r string) string {
		return "version = \"0.1.0\"\n[story]\nadapter = \"github\"\nrepository = \"" + r + "\"\n"
	}
	for _, r := range []string{"", "owner", "owner/", "/repo", "a/b/c", "own er/repo", "owner/re po", "オーナー/repo", "a+b/c", "a/b:c", "a\\b"} {
		if got := loadErrs(t, body(r)); !lineWith(got, ".vanda/config.toml:4: ", "repository") {
			t.Errorf("repository = %q が、repository の行（4 行目）を指す誤りにならない:\n%s", r, got)
		}
	}
	for _, r := range []string{"a/b", "TadahiroYamamura/vanda", "Owner_1/re.po-2", "_/.", "-/-"} {
		if got := loadErrs(t, body(r)); got != "" {
			t.Errorf("repository = %q が誤りになった:\n%s", r, got)
		}
	}
}

// @vanda:evidence config-guardrail-adapter-file
func TestLoadはguardrailのadapterにfileのほかを書けば誤りにする(t *testing.T) {
	body := func(a string) string {
		return "version = \"0.1.0\"\n\n[[guardrail]]\nadapter = \"" + a + "\"\npaths = [\"db/schema.sql\"]\n"
	}
	if got := loadErrs(t, body("file")); got != "" {
		t.Errorf("adapter = \"file\" が誤りになった:\n%s", got)
	}
	for _, a := range []string{"sql", "File", "files", "go", "md", ""} {
		if got := loadErrs(t, body(a)); !lineWith(got, ".vanda/config.toml:4: ", `"`+a+`"`) {
			t.Errorf("adapter = %q が、adapter の行（4 行目）を指す誤りにならない:\n%s", a, got)
		}
	}
}

// @vanda:evidence config-id-pattern-default
func TestLoadはid_patternを省けばストーリーのIDの書き方をシャープと数字にする(t *testing.T) {
	for name, body := range map[string]string{
		"storyの表がない":            "version = \"0.1.0\"\n",
		"storyの表にid_patternがない": "version = \"0.1.0\"\n[story]\nadapter = \"github\"\nrepository = \"o/r\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, errs := Load(writeConfig(t, body))
			if errs != nil {
				t.Fatal(errs)
			}
			for s, want := range map[string]bool{"#1": true, "#24": true, "#0123456789": true, "#": false, "24": false, "#24a": false, "a#24": false, "# 24": false, "#２４": false, "PROJ-1": false, "": false} {
				if got := cfg.IDFull.MatchString(s); got != want {
					t.Errorf("%q がストーリーの ID の書き方に当たるか: %v（思ったもの %v）", s, got, want)
				}
			}
		})
	}
}

// @vanda:evidence config-id-pattern-regex
func TestLoadはid_patternが正規表現として読めなければ誤りにする(t *testing.T) {
	body := func(p string) string {
		return "version = \"0.1.0\"\n[story]\nid_pattern = '" + p + "'\n"
	}
	// (?=…) の先読みと \1 の後方参照は、RE2 にない
	for _, p := range []string{"(", "PROJ-[0-9", "x**", "(?=PROJ)[A-Z]+-[0-9]+", `(a)\1`} {
		if got := loadErrs(t, body(p)); !lineWith(got, ".vanda/config.toml:3: ", "id_pattern", "正規表現") {
			t.Errorf("id_pattern = %q が、id_pattern の行（3 行目）を指す誤りにならない:\n%s", p, got)
		}
	}
	for _, p := range []string{"PROJ-[0-9]+", `#\d+`, "(?:STORY|BUG)-[0-9]+"} {
		if got := loadErrs(t, body(p)); got != "" {
			t.Errorf("id_pattern = %q が誤りになった:\n%s", p, got)
		}
	}
}

// @vanda:evidence config-layer-id
func TestLoadはlayerのidが決まった形でないかほかのレイヤーと重なれば誤りにする(t *testing.T) {
	layer := func(id string) string {
		return "\n[[layer]]\nid = \"" + id + "\"\nname = \"n\"\ndescription = \"d\"\n"
	}
	for _, id := range []string{"Model", "1db", "-a", "a-", "a--b", "a_b", "a b", "モデル", "a.b"} {
		if got := loadErrs(t, "version = \"0.1.0\"\n"+layer(id)); !lineWith(got, ".vanda/config.toml:4: ", "ID", `"`+id+`"`) {
			t.Errorf("id = %q が、id の行（4 行目）を指す誤りにならない:\n%s", id, got)
		}
	}
	if got := loadErrs(t, "version = \"0.1.0\"\n"+layer("model")+layer("db-2")+layer("a1-b2-c3")); got != "" {
		t.Errorf("決まった形の重ならない id が誤りになった:\n%s", got)
	}
	// 重なれば、後のほうの id の行を指す
	got := loadErrs(t, "version = \"0.1.0\"\n"+layer("model")+layer("db")+layer("model"))
	if !lineWith(got, ".vanda/config.toml:14: ", `"model"`, "重な") || strings.Count(got, "\n") != 0 {
		t.Errorf("重なった id が、後のほうの id の行（14 行目）を指す 1 件の誤りにならない:\n%s", got)
	}
}

// @vanda:evidence config-pattern-syntax
func TestLoadはファイルのパターンの書き方が誤っていればどの項目でも誤りにする(t *testing.T) {
	// どの項目にも、そのパターンだけを書く。wrap は、項目の行が 3 行目になるように書く
	wrap := map[string]func(p string) string{
		"promises":   func(p string) string { return "version = \"0.1.0\"\n\npromises = [\"" + p + "\"]\n" },
		"agreements": func(p string) string { return "version = \"0.1.0\"\n\nagreements = [\"" + p + "\"]\n" },
		"annotationのpaths": func(p string) string {
			return "version = \"0.1.0\"\n[[annotation]]\npaths = [\"" + p + "\"]\nadapter = \"go\"\ntypes = [\"unit-test\"]\n"
		},
		"guardrailのpaths": func(p string) string {
			return "version = \"0.1.0\"\n[[guardrail]]\npaths = [\"" + p + "\"]\nadapter = \"file\"\n"
		},
	}
	// リポジトリの直下からでない場所（/ で始まる）と、閉じていない [ と、空の文字列
	bad := []string{"/abs/a.toml", "/", "a/[b.toml", "[", "a/b[", "**/[x/*.go", ""}
	good := []string{"a.toml", "a/*.toml", "**/*_test.go", "e2e/**/*.txtar", "a/[bc].toml", "a?/b"}
	for key, w := range wrap {
		for _, p := range bad {
			if got := loadErrs(t, w(p)); !lineWith(got, ".vanda/config.toml:3: ", "パターン", `"`+p+`"`) {
				t.Errorf("%s のパターン %q が、その行（3 行目）を指す誤りにならない:\n%s", key, p, got)
			}
		}
		for _, p := range good {
			got := loadErrs(t, w(p))
			// はっきり書いたファイルがないことは、設定を読むときには確かめない
			if got != "" {
				t.Errorf("%s のパターン %q が誤りになった:\n%s", key, p, got)
			}
		}
	}
}

// @vanda:evidence config-patterns-not-empty
func TestLoadはpromisesとagreementsとannotationとguardrailのpathsにパターンが1つもなければ誤りにする(t *testing.T) {
	for name, c := range map[string]struct{ body, prefix string }{
		"promises":         {"version = \"0.1.0\"\npromises = []\n", ".vanda/config.toml:2: "},
		"agreements":       {"version = \"0.1.0\"\nagreements = []\n", ".vanda/config.toml:2: "},
		"annotationのpaths": {"version = \"0.1.0\"\n[[annotation]]\npaths = []\nadapter = \"go\"\ntypes = [\"unit-test\"]\n", ".vanda/config.toml:3: "},
		"guardrailのpaths":  {"version = \"0.1.0\"\n[[guardrail]]\nadapter = \"file\"\npaths = []\n", ".vanda/config.toml:4: "},
	} {
		t.Run(name, func(t *testing.T) {
			if got := loadErrs(t, c.body); !lineWith(got, c.prefix, "パターン") {
				t.Errorf("パターンのない項目が、その行を指す誤りにならない:\n%s", got)
			}
		})
	}
}

// @vanda:evidence config-required-fields
func TestLoadはversionとlayerとannotationとguardrailの必須の項目がなければ誤りにする(t *testing.T) {
	const layerAll = "\n[[layer]]\nid = \"model\"\nname = \"n\"\ndescription = \"d\"\n"
	cases := []struct {
		name, body, prefix, key string
	}{
		{"version", "promises = [\".vanda/promises.toml\"]\n", ".vanda/config.toml:", "version"},
		{"layerのid", "version = \"0.1.0\"\n\n[[layer]]\nname = \"n\"\ndescription = \"d\"\n", ".vanda/config.toml:3: ", "id"},
		{"layerのname", "version = \"0.1.0\"\n\n[[layer]]\nid = \"model\"\ndescription = \"d\"\n", ".vanda/config.toml:3: ", "name"},
		{"layerのdescription", "version = \"0.1.0\"\n\n[[layer]]\nid = \"model\"\nname = \"n\"\n", ".vanda/config.toml:3: ", "description"},
		{"annotationのpaths", "version = \"0.1.0\"\n\n[[annotation]]\nadapter = \"go\"\ntypes = [\"unit-test\"]\n", ".vanda/config.toml:3: ", "paths"},
		{"guardrailのadapter", "version = \"0.1.0\"\n\n[[guardrail]]\npaths = [\"db/schema.sql\"]\n", ".vanda/config.toml:3: ", "adapter"},
		{"guardrailのpaths", "version = \"0.1.0\"\n\n[[guardrail]]\nadapter = \"file\"\n", ".vanda/config.toml:3: ", "paths"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := loadErrs(t, c.body); !lineWith(got, c.prefix, c.key, "ありません") {
				t.Errorf("%s がないのに、%q で始まる誤りにならない:\n%s", c.key, c.prefix, got)
			}
		})
	}
	if got := loadErrs(t, "version = \"0.1.0\"\n"+layerAll+"\n[[annotation]]\npaths = [\"x/*\"]\nadapter = \"go\"\ntypes = [\"unit-test\"]\n\n[[guardrail]]\nadapter = \"file\"\npaths = [\"db/*.sql\"]\n"); got != "" {
		t.Errorf("必須の項目を全部書いた設定が誤りになった:\n%s", got)
	}
}
