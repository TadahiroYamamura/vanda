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
	want := []string{
		".vanda/config.toml:1: 動いている vanda（0.1.0）が、設定の版（0.2.0）より古い。vanda を 0.2.0 に上げる",
		".vanda/config.toml:2: promises にパターンがない。",
		`.vanda/config.toml:6: レイヤーの ID "Model" が決まった形でない。`,
		`.vanda/config.toml:5: layer "Model" の description がない。description = "…" を書く`,
		`.vanda/config.toml:11: アダプタ "rust" は vanda の中にない。`,
		".vanda/config.toml:14: id_pattern が正規表現として読めない",
		".vanda/config.toml:3: extra は決まった項目でない。",
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
	if !strings.Contains(errs.Error(), ".vanda/config.toml:3: id_pattern が空の文字列に当たる") {
		t.Errorf("エラーが違う: %v", errs)
	}
}
