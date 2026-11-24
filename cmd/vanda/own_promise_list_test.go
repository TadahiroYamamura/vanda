package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

// このファイルのテストは、テスト用のリポジトリではなく、このリポジトリ自身の約束リストと承認の記録を読む
// （docs/plans/promise-reorg-shape-test.md の「書き直した後の約束リスト」）。

var ownRoot = filepath.Join("..", "..")

// ownRepoCopy は、このリポジトリの git が管理しているファイルを、作業ツリーの今の中身で一時のリポジトリに写し、1 つのコミットにする。
// vanda check は HEAD のコミットだけを読む。このリポジトリでそのまま動かすと、コミットする前の直しを確かめられないので、写しで動かす。
func ownRepoCopy(t *testing.T) *rvGit {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = ownRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("このリポジトリのファイルを並べられない: %v", err)
	}
	dir := t.TempDir()
	g := newRVGit(t, dir)
	g.run("init", "-q", "-b", "main")
	copied := 0
	for _, f := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		src := filepath.Join(ownRoot, filepath.FromSlash(f))
		info, err := os.Stat(src)
		if os.IsNotExist(err) {
			// 作業ツリーで消して、まだコミットしていないファイル
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		// カスタムアダプタを実行できるよう、モードも写す
		if err := os.WriteFile(dst, body, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
		copied++
	}
	if copied == 0 {
		t.Fatalf("このリポジトリのファイルを 1 つも写せなかった")
	}
	g.commitAll("このリポジトリの写し", "#1")
	return g
}

func Test_vanda自身の約束リストとアノテーションは約束リストの形とアノテーションのつながりのチェックを通る(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	g := ownRepoCopy(t)
	// チェックの設定は作業ツリーのものを読むので、コミットした後に置く
	for _, name := range []string{"promise-list-shape", "annotation-links"} {
		config := "own-" + name + ".toml"
		if err := os.WriteFile(filepath.Join(g.root, config), []byte("[[check]]\nname = \""+name+"\"\nscope = \"all\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runVandaIn(t, bin, g.root, g.env, "check", config)
		if code != 0 || strings.TrimSpace(stderr) != "" {
			t.Errorf("%s が通らない（終了コード %d、誤り %d 行）\n%s\n%s", name, code, len(strings.Split(strings.TrimSpace(stderr), "\n")), stdout, stderr)
		}
	}
}

type ownPromiseList struct {
	Property []ownItem `toml:"property"`
	Truth    []ownItem `toml:"truth"`
	Term     []ownItem `toml:"term"`
	Feature  []ownItem `toml:"feature"`
	Spec     []ownItem `toml:"spec"`
}

type ownItem struct {
	ID      string   `toml:"id"`
	Refines []string `toml:"refines"`
}

func loadOwnPromiseList(t *testing.T) ownPromiseList {
	t.Helper()
	var l ownPromiseList
	if _, err := toml.DecodeFile(filepath.Join(ownRoot, ".vanda", "promises.toml"), &l); err != nil {
		t.Fatalf("このリポジトリの約束リストを読めない: %v", err)
	}
	return l
}

func Test_vanda自身の約束リストは機能のすべてが下に性質を持たない性質を仕様のすべてが機能を1つ以上refinesしている(t *testing.T) {
	l := loadOwnPromiseList(t)
	if len(l.Property) == 0 || len(l.Feature) == 0 || len(l.Spec) == 0 {
		t.Fatalf("性質 %d 件・機能 %d 件・仕様 %d 件。どれかが 1 件もなく、確かめるものがない", len(l.Property), len(l.Feature), len(l.Spec))
	}
	isProperty := map[string]bool{}
	hasLower := map[string]bool{}
	for _, p := range l.Property {
		isProperty[p.ID] = true
	}
	for _, p := range l.Property {
		for _, upper := range p.Refines {
			hasLower[upper] = true
		}
	}
	isFeature := map[string]bool{}
	for _, f := range l.Feature {
		isFeature[f.ID] = true
		n := 0
		for _, id := range f.Refines {
			if isProperty[id] && !hasLower[id] {
				n++
			}
		}
		if n == 0 {
			t.Errorf("機能 %s は、下に性質を持たない性質を 1 つも refines していない（refines = %v）", f.ID, f.Refines)
		}
	}
	for _, s := range l.Spec {
		n := 0
		for _, id := range s.Refines {
			if isFeature[id] {
				n++
			}
		}
		if n == 0 {
			t.Errorf("仕様 %s は、機能を 1 つも refines していない（refines = %v）", s.ID, s.Refines)
		}
	}
}

// 約束リストを性質から書き直したコミット（9fa6cf2）の日時。承認は、書き直した後にやり直す
// （docs/plans/promise-reorg-shape-impl.md の 2.8 の「承認の記録」）ので、これより前の承認は、書き直す前の記録。
// 今も同じ ID で残る用語への古い記録は、ID では見分けられないので、日時で見分ける。
var ownPromiseListRewrittenAt = time.Date(2026, 11, 24, 18, 19, 24, 0, time.FixedZone("JST", 9*60*60))

func Test_vanda自身の承認の記録に約束リストを書き直す前の記録が残っていない(t *testing.T) {
	var records struct {
		Agreement []struct {
			Kind     string    `toml:"kind"`
			Key      string    `toml:"key"`
			AgreedAt time.Time `toml:"agreed_at"`
		} `toml:"agreement"`
	}
	if _, err := toml.DecodeFile(filepath.Join(ownRoot, ".vanda", "agreements.toml"), &records); err != nil {
		t.Fatalf("このリポジトリの承認の記録を読めない: %v", err)
	}
	l := loadOwnPromiseList(t)
	now := map[string]map[string]bool{"property": {}, "truth": {}, "term": {}, "feature": {}, "spec": {}}
	for kind, items := range map[string][]ownItem{"property": l.Property, "truth": l.Truth, "term": l.Term, "feature": l.Feature, "spec": l.Spec} {
		for _, it := range items {
			now[kind][it.ID] = true
		}
	}
	for i, r := range records.Agreement {
		if r.AgreedAt.Before(ownPromiseListRewrittenAt) {
			t.Errorf("%d 件目（%s %s）は、約束リストを書き直す前（%s）の記録", i+1, r.Kind, r.Key, r.AgreedAt.Format(time.RFC3339))
		}
		// 書き直す前の仕様（298 件）と機能（34 件）は、今の約束リストにない。前の形の種類 claim の記録も、ここで見つかる
		if ids, ok := now[r.Kind]; r.Kind != "file" && (!ok || !ids[r.Key]) {
			t.Errorf("%d 件目（%s %s）は、今の約束リストにない項目を指している", i+1, r.Kind, r.Key)
		}
	}
}
