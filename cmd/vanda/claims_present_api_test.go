package main_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// RV-08: 約束の詳細の evidence[].present（エビデンスが揃っているか）。エビデンスのタイプ 12 を全部回す。
// 約束 all-types は、どのタイプにも、揃っているとするアノテーション（docs/formats.md の表）がある。
// 約束 no-types は、同じ予定で、アノテーションがひとつもない。

// 期待値は docs/formats.md の「エビデンスが揃っている審査」の表と evidence[].present の説明から手で書く。
// 値は、アノテーションがあるとき・ないときの present（nil は null）。
var presentTypes = []struct {
	typ            string
	with, without  *bool
	adapterWhenSet string
}{
	{"model-check", bp(true), bp(false), "tla"},
	{"proof", bp(false), bp(false), ""},
	{"cross-check", bp(true), bp(false), "go"},
	{"random-check", bp(true), bp(false), "go"},
	{"scenario-test", bp(true), bp(false), "txtar"},
	{"unit-test", bp(true), bp(false), "go"},
	{"interface-test", bp(true), bp(false), "go"},
	{"concurrency-test", bp(true), bp(false), "go"},
	{"guardrail", bp(true), bp(false), "sql"},
	{"sensor", bp(true), bp(false), "go"},
	{"human-review", nil, nil, ""},
}

func bp(b bool) *bool { return &b }

func presentClaim(id string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n[[claim]]\nid = %q\nsubjects = [\"t\"]\nstatement = \"エビデンスのタイプ\"\nlayers = [\"model\"]\n", id)
	for _, p := range presentTypes {
		fmt.Fprintf(&b, "\n[[claim.evidence]]\nstage = \"implementation\"\ntype = %q\nlayers = [\"model\"]\n", p.typ)
	}
	return b.String()
}

// @vanda:evidence evidence-presence-shown
func Test約束の詳細はエビデンスのタイプごとに揃っているかを示す(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(`version = %q

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

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
[[annotation]]
paths = ["**/*.go"]
adapter = "go"
types = ["unit-test", "cross-check", "random-check", "interface-test", "concurrency-test", "sensor"]
`, version))
	writeFile(t, root, ".vanda/promises.toml", "[[term]]\nid = \"t\"\nname = \"n\"\ndefinition = \"d\"\n"+presentClaim("all-types")+presentClaim("no-types"))
	writeFile(t, root, "e2e/all.txtar", "# @vanda:evidence all-types\n-- a --\n")
	writeFile(t, root, "spec/All.tla", "---- MODULE All ----\n\\* @vanda:claim all-types\nP == TRUE\n====\n")
	writeFile(t, root, "db/all.sql", "-- @vanda:evidence all-types\nCREATE TABLE x (id INTEGER);\n")
	writeFile(t, root, "internal/all/all_test.go", "package all\n\nimport \"testing\"\n\n// @vanda:evidence all-types\nfunc TestAll(t *testing.T) {}\n")
	writeFile(t, root, "internal/all/all.go", "package all\n\n// @vanda:sensor all-types\nfunc Check() error { return nil }\n")
	g.commitAll("はじめ", "#1")
	s := startServe(t, bin, root, g.env)

	for _, id := range []string{"all-types", "no-types"} {
		var got struct {
			Evidence []struct {
				Type    string          `json:"type"`
				Found   []claimFound    `json:"found"`
				Present json.RawMessage `json:"present"`
			} `json:"evidence"`
		}
		if code, msg := s.get("/api/claims/"+id, &got); code != 200 {
			t.Fatalf("%s の詳細を読めない（%d）: %s", id, code, msg)
		}
		if len(got.Evidence) != len(presentTypes) {
			t.Fatalf("%s のエビデンスが %d 件", id, len(got.Evidence))
		}
		for i, p := range presentTypes {
			e := got.Evidence[i]
			want := p.without
			if id == "all-types" {
				want = p.with
			}
			wantJSON := "null"
			if want != nil {
				wantJSON = fmt.Sprint(*want)
			}
			if e.Type != p.typ || strings.TrimSpace(string(e.Present)) != wantJSON {
				t.Errorf("%s の %s の present: %s（思ったもの %s）", id, p.typ, e.Present, wantJSON)
			}
			// found があるのは、揃っているとするアダプタのアノテーションがあるときだけ
			wantFound := id == "all-types" && p.adapterWhenSet != ""
			if (len(e.Found) > 0) != wantFound || (wantFound && e.Found[0].Adapter != p.adapterWhenSet) {
				t.Errorf("%s の %s の found: %+v", id, p.typ, e.Found)
			}
		}
	}
}
