package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

[[annotation]]
paths = ["docs/*.md"]
command = ["./adapter", "--json"]

[[guardrail]]
adapter = "file"
paths = ["db/schema.sql"]
`)
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
	if len(cfg.Annotations) != 2 || cfg.Annotations[0].Adapter != "go" || cfg.Annotations[1].Command[1] != "--json" {
		t.Errorf("annotation: %+v", cfg.Annotations)
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

[story]
id_pattern = "("
`)
	_, errs := Load(root)
	want := [][]string{
		{".vanda/config.toml:1: ", "動いている vanda（0.1.0）", "設定の版（0.2.0）より古い", "vanda を 0.2.0 に上げ"},
		{".vanda/config.toml:2: ", "promises", "パターン"},
		{".vanda/config.toml:6: ", `レイヤーの ID "Model"`, "決まった形"},
		{".vanda/config.toml:5: ", `layer "Model" の description`, `description = "…"`},
		{".vanda/config.toml:11: ", `アダプタ "rust"`, "vanda の中"},
		{".vanda/config.toml:14: ", "id_pattern", "正規表現"},
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

func TestLoadは設定の版が動いているvandaより古いと移行を知らせる(t *testing.T) {
	root := writeConfig(t, `version = "0.0.9"`)
	_, errs := Load(root)
	if !strings.Contains(errs.Error(), "移行のコマンド") {
		t.Errorf("移行を知らせていない: %s", errs)
	}
}

func TestIsID(t *testing.T) {
	for s, want := range map[string]bool{"task": true, "task-id-2": true, "2a": false, "a2": true, "": false, "Task": false, "-a": false, "a-": false, "a--b": false, "a_b": false, "タスク": false} {
		if IsID(s) != want {
			t.Errorf("IsID(%q) = %v", s, !want)
		}
	}
}

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
