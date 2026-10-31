package main_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// GET /api/review の項目のうち、RV-04（フラグ）・RV-10（まとまりと並び）・RV-05（変更前と変更後の中身）と、GET /api/context。
// リポジトリは、この 3 つを一度に見られるように組み立てる（newFieldsRepo）。

// 約束 flag-0〜flag-7 は、番号の 3 つのビットで、AI 案（1）・推測（2）・前の合意（4）の有無を表す（8 通りを全部回す）。
// 前の合意のあるものは、main で今と違う中身に合意してある。
const (
	bitAI      = 1
	bitGuess   = 2
	bitAgreed  = 4
	flagClaims = 8
)

const fieldsConfig = `version = %q

[[layer]]
id = "cli"
name = "CLI"
description = "引数と出力の形"

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

[[guardrail]]
adapter = "file"
paths = ["db/*.sql"]

[gui]
base = "main"
`

func fieldsFlagClaim(i int, story bool) string {
	var b strings.Builder
	statement := "v0"
	if story {
		statement = "v1"
	}
	fmt.Fprintf(&b, "\n[[claim]]\nid = \"flag-%d\"\nsubjects = [\"t-keep\"]\nstatement = %q\nlayers = [\"model\"]\n", i, statement)
	if story {
		if i&bitAI != 0 {
			b.WriteString("flags = [\"ai-proposal\"]\n")
		}
		switch {
		case i&bitGuess != 0:
			b.WriteString("guesses = [\"確かめていない\"]\n")
		case i == bitAI:
			// 空の guesses は推測でない
			b.WriteString("guesses = []\n")
		}
	}
	b.WriteString("\n[[claim.evidence]]\nstage = \"test\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n")
	return b.String()
}

func fieldsFlagFP(i int, statement string) string {
	return sha256Hex([]byte(fmt.Sprintf(`{"evidence":[{"layers":["model"],"stage":"test","type":"unit-test"}],"id":"flag-%d","layers":["model"],"statement":%q,"subjects":["t-keep"]}`, i, statement)))
}

const fieldsBaseRest = `
[[term]]
id = "t-keep"
name = "変えない用語"
definition = "約束の対象"

[[term]]
id = "t-mod"
name = "変える用語"
definition = "前"

[[term]]
id = "t-del"
name = "消す用語"
definition = "消す"

[[feature]]
id = "f-mod"
name = "変える機能"
definition = "前"
core = true
formal = false
formal_waiver = "理由 1"

[[feature]]
id = "f-del"
name = "消す機能"
definition = "消す"

[[claim]]
id = "z-cli"
subjects = ["t-keep"]
statement = "CLI の約束"
layers = ["cli", "model"]

[[claim.evidence]]
stage = "test"
type = "scenario-test"
layers = ["cli", "model"]

[[claim]]
id = "a-db"
subjects = ["t-keep"]
statement = "DB の約束"
layers = ["db"]

[[claim.evidence]]
stage = "implementation"
type = "guardrail"
layers = ["db"]

[[claim]]
id = "flag-agreed"
subjects = ["t-keep"]
statement = "v0"
layers = ["model"]

[[claim.evidence]]
stage = "test"
type = "unit-test"
layers = ["model"]
`

const fieldsStoryRest = `
[[term]]
id = "t-keep"
name = "変えない用語"
definition = "約束の対象"

[[term]]
id = "t-mod"
name = "変える用語"
definition = "後"
part_of = "t-keep"

[[feature]]
id = "f-mod"
name = "変える機能"
definition = "前"
core = true
formal = false
formal_waiver = "理由 2"

[[feature]]
id = "f-new"
name = "新しい機能"
definition = "足す"

[[claim]]
id = "z-cli"
subjects = ["t-keep"]
statement = "CLI の約束（改）"
layers = ["cli", "model"]

[[claim.evidence]]
stage = "test"
type = "scenario-test"
layers = ["cli", "model"]

[[claim]]
id = "a-db"
subjects = ["t-keep"]
statement = "DB の約束（改）"
rationale = "理由"
condition = "条件"
layers = ["db"]

[[claim.evidence]]
stage = "implementation"
type = "guardrail"
layers = ["db"]
detail = "制約で拒む"

[[claim]]
id = "flag-agreed"
subjects = ["t-keep"]
statement = "v1"
layers = ["model"]

[[claim.evidence]]
stage = "test"
type = "unit-test"
layers = ["model"]
`

const (
	fieldsScenarioBase  = "# 前\n-- a --\n"
	fieldsScenarioStory = "# 後\n-- a --\n"
	fieldsSQLBase       = "CREATE TABLE a (id INTEGER);\n"
	fieldsSQLStory      = "CREATE TABLE a (id INTEGER NOT NULL);\n"
	fieldsTLA           = "---- MODULE M ----\nX == 1\n====\n"
	fieldsNewScenario   = "# 足した\n-- a --\n"
)

type fieldsRepo struct {
	*reviewRepo
}

// newFieldsRepo は、main（#1）と、そこから分かれた story-24（#24）を作る。HEAD は story-24。
func newFieldsRepo(t *testing.T, version string) *fieldsRepo {
	t.Helper()
	root := t.TempDir()
	r := &reviewRepo{t: t, Root: root, Env: reviewBaseEnv(root)}
	r.g = &rvGit{t: t, root: root, env: append(reviewBaseEnv(root),
		"GIT_AUTHOR_NAME=テスト担当", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=テスト担当", "GIT_COMMITTER_EMAIL=tester@example.com")}
	r.g.run("init", "-q", "-b", "main")
	r.g.run("config", "user.name", reviewUserName)
	r.g.run("config", "user.email", reviewUserEmail)
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(fieldsConfig, version))

	var base, story strings.Builder
	base.WriteString(fieldsBaseRest)
	story.WriteString(fieldsStoryRest)
	var agreements strings.Builder
	for i := 0; i < flagClaims; i++ {
		base.WriteString(fieldsFlagClaim(i, false))
		story.WriteString(fieldsFlagClaim(i, true))
		if i&bitAgreed != 0 {
			agreements.WriteString(agreementTOML("claim", fmt.Sprintf("flag-%d", i), fieldsFlagFP(i, "v0"), "", "#1", "テスト担当", "tester@example.com"))
		}
	}
	// flag-agreed は、前の中身と今の中身の両方に合意してある（今の中身に合う合意があれば、差分ありにしない）
	agreedFP := func(v string) string {
		return sha256Hex([]byte(fmt.Sprintf(`{"evidence":[{"layers":["model"],"stage":"test","type":"unit-test"}],"id":"flag-agreed","layers":["model"],"statement":%q,"subjects":["t-keep"]}`, v)))
	}
	agreements.WriteString(agreementTOML("claim", "flag-agreed", agreedFP("v0"), "", "#1", "テスト担当", "tester@example.com"))
	sqlFP := sha256Hex([]byte(fieldsSQLBase))
	agreements.WriteString(agreementTOML("file", sqlFP, sqlFP, "db/a.sql", "#1", "テスト担当", "tester@example.com"))

	writeFile(t, root, ".vanda/promises.toml", base.String())
	writeFile(t, root, ".vanda/agreements.toml", agreements.String())
	writeFile(t, root, "e2e/b.txtar", fieldsScenarioBase)
	writeFile(t, root, "db/a.sql", fieldsSQLBase)
	writeFile(t, root, "spec/M.tla", fieldsTLA)
	r.g.run("add", "-A")
	r.g.run("commit", "-q", "-m", "#1 はじめの約束を置く")

	r.g.run("switch", "-q", "-c", reviewBranch)
	writeFile(t, root, ".vanda/promises.toml", story.String())
	writeFile(t, root, ".vanda/agreements.toml", agreements.String()+
		agreementTOML("claim", "flag-agreed", agreedFP("v1"), "", "#24", reviewUserName, reviewUserEmail))
	writeFile(t, root, "e2e/b.txtar", fieldsScenarioStory)
	writeFile(t, root, "e2e/a.txtar", fieldsNewScenario)
	writeFile(t, root, "db/a.sql", fieldsSQLStory)
	r.g.run("rm", "-q", "spec/M.tla")
	r.g.run("add", "-A")
	r.g.run("commit", "-q", "-m", "#24 約束と用語と機能とファイルを変える")
	return &fieldsRepo{r}
}

func setupFields(t *testing.T) (string, *fieldsRepo, *vandaServer, reviewResponse) {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newFieldsRepo(t, version)
	// 書いた約束リストが、段 1 の約束リストの形の審査を通ること（通らない約束リストでは、一覧の誤りと言えない）
	shape(t, bin, r.Root)
	s := startServe(t, bin, r.Root, r.Env)
	var got reviewResponse
	if code, msg := s.get("/api/review?view=branch&base=main", &got); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	return bin, r, s, got
}

// RV-04
func Testフラグは約束のflagsとguessesと前の合意から決まった順に立つ(t *testing.T) {
	_, _, _, got := setupFields(t)
	for i := 0; i < flagClaims; i++ {
		id := fmt.Sprintf("flag-%d", i)
		want := []string{}
		if i&bitAI != 0 {
			want = append(want, "ai-proposal")
		}
		if i&bitGuess != 0 {
			want = append(want, "guess")
		}
		if i&bitAgreed != 0 {
			want = append(want, "changed-since-agreement")
		}
		e := findEntry(got.Entries, id)
		if e == nil {
			t.Errorf("%s が一覧にない", id)
			continue
		}
		if e.Flags == nil || !reflect.DeepEqual(e.Flags, want) {
			t.Errorf("%s（AI 案 %v、推測 %v、前の合意 %v）のフラグ: %v（思ったもの %v）", id, i&bitAI != 0, i&bitGuess != 0, i&bitAgreed != 0, e.Flags, want)
		}
		if str(e.State) != "proposed" {
			t.Errorf("%s の state: %q", id, str(e.State))
		}
	}
	if e := findEntry(got.Entries, "flag-agreed"); e == nil || len(e.Flags) != 0 || str(e.State) != "agreed" {
		t.Errorf("前の中身と今の中身の両方に合意した flag-agreed が、合意済・フラグなしでない: %+v", e)
	}
	for path, want := range map[string][]string{"db/a.sql": {"changed-since-agreement"}, "e2e/b.txtar": {}, "e2e/a.txtar": {}} {
		var e *reviewEntry
		for i := range got.Entries {
			if str(got.Entries[i].Path) == path {
				e = &got.Entries[i]
			}
		}
		if e == nil || e.Flags == nil || !reflect.DeepEqual(e.Flags, want) {
			t.Errorf("%s のフラグ: %+v（思ったもの %v）", path, e, want)
		}
	}
}

// RV-10・RV-02
func Test一覧は設定のレイヤーの順の約束と用語と機能とファイルのまとまりに分かれその中はIDか場所の順に並ぶ(t *testing.T) {
	_, _, _, got := setupFields(t)
	want := []string{"layer:cli z-cli", "layer:model flag-0", "layer:model flag-1", "layer:model flag-2", "layer:model flag-3",
		"layer:model flag-4", "layer:model flag-5", "layer:model flag-6", "layer:model flag-7", "layer:model flag-agreed",
		"layer:db a-db",
		"terms-features f-del", "terms-features f-mod", "terms-features f-new", "terms-features t-del", "terms-features t-mod",
		"files db/a.sql", "files e2e/a.txtar", "files e2e/b.txtar", "files spec/M.tla"}
	var have []string
	for _, e := range got.Entries {
		have = append(have, e.Group+" "+e.key())
	}
	if strings.Join(have, "\n") != strings.Join(want, "\n") {
		t.Errorf("一覧の並び:\n%s\n--- 思ったもの\n%s", strings.Join(have, "\n"), strings.Join(want, "\n"))
	}
	if got.Total != len(want) {
		t.Errorf("total: %d", got.Total)
	}
}

func jsonEqual(t *testing.T, where string, got json.RawMessage, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Errorf("%s を読めない: %v（%s）", where, err, got)
		return
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("テストの %s の期待値が JSON でない: %v", where, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("%s:\n%s\n--- 思ったもの\n%s", where, got, want)
	}
}

// RV-05
func Test変更前と変更後の中身は約束リストに書いたとおりの項目で省いたものはnullになる(t *testing.T) {
	_, _, _, got := setupFields(t)
	cases := []struct {
		key, before, after string
	}{
		{"a-db",
			`{"statement":"DB の約束","rationale":null,"condition":null,"subjects":["t-keep"],"layers":["db"],"evidence":[{"stage":"implementation","type":"guardrail","layers":["db"],"detail":null}],"flags":[],"guesses":[]}`,
			`{"statement":"DB の約束（改）","rationale":"理由","condition":"条件","subjects":["t-keep"],"layers":["db"],"evidence":[{"stage":"implementation","type":"guardrail","layers":["db"],"detail":"制約で拒む"}],"flags":[],"guesses":[]}`},
		{"flag-3",
			`{"statement":"v0","rationale":null,"condition":null,"subjects":["t-keep"],"layers":["model"],"evidence":[{"stage":"test","type":"unit-test","layers":["model"],"detail":null}],"flags":[],"guesses":[]}`,
			`{"statement":"v1","rationale":null,"condition":null,"subjects":["t-keep"],"layers":["model"],"evidence":[{"stage":"test","type":"unit-test","layers":["model"],"detail":null}],"flags":["ai-proposal"],"guesses":["確かめていない"]}`},
		{"t-mod", `{"name":"変える用語","definition":"前","partOf":null}`, `{"name":"変える用語","definition":"後","partOf":"t-keep"}`},
		{"t-del", `{"name":"消す用語","definition":"消す","partOf":null}`, `null`},
		{"f-mod", `{"name":"変える機能","definition":"前","core":true,"formal":false,"formalWaiver":"理由 1"}`,
			`{"name":"変える機能","definition":"前","core":true,"formal":false,"formalWaiver":"理由 2"}`},
		{"f-new", `null`, `{"name":"新しい機能","definition":"足す","core":null,"formal":null,"formalWaiver":null}`},
		{"e2e/b.txtar", fmt.Sprintf(`{"text":%q}`, fieldsScenarioBase), fmt.Sprintf(`{"text":%q}`, fieldsScenarioStory)},
		{"spec/M.tla", fmt.Sprintf(`{"text":%q}`, fieldsTLA), `null`},
		{"e2e/a.txtar", `null`, fmt.Sprintf(`{"text":%q}`, fieldsNewScenario)},
	}
	for _, c := range cases {
		var e *reviewEntry
		for i := range got.Entries {
			if got.Entries[i].key() == c.key {
				e = &got.Entries[i]
			}
		}
		if e == nil {
			t.Errorf("%s が一覧にない", c.key)
			continue
		}
		before, after := e.Before, e.After
		if len(before) == 0 {
			before = json.RawMessage("null")
		}
		if len(after) == 0 {
			after = json.RawMessage("null")
		}
		jsonEqual(t, c.key+" の before", before, c.before)
		jsonEqual(t, c.key+" の after", after, c.after)
	}
}

type contextResponse struct {
	Version     string   `json:"version"`
	Branch      string   `json:"branch"`
	Head        string   `json:"head"`
	DefaultBase string   `json:"defaultBase"`
	Bases       []string `json:"bases"`
	User        *struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"user"`
	Stories []struct {
		ID       string `json:"id"`
		InBranch bool   `json:"inBranch"`
	} `json:"stories"`
	Layers []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"layers"`
	StoryStore string `json:"storyStore"`
}

// CM-06・CM-09
func Test画面で選べるものは今の設定とブランチと履歴から決まる(t *testing.T) {
	bin, r, s, _ := setupFields(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	// リモートを追うブランチと、除く origin/HEAD を足す。ID のないコミットも足す
	r.git("update-ref", "refs/remotes/origin/main", "main")
	r.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	r.git("branch", "release", "main")
	r.git("commit", "-q", "--allow-empty", "-m", "ID のないコミット")
	r.git("commit", "-q", "--allow-empty", "-m", "#30 で直す")

	var got contextResponse
	if code, msg := s.get("/api/context", &got); code != 200 {
		t.Fatalf("context を読めない（%d）: %s", code, msg)
	}
	if got.Version != version || got.Branch != reviewBranch || got.Head != r.git("rev-parse", "HEAD") || got.DefaultBase != "main" || got.StoryStore != "" {
		t.Errorf("version %q、branch %q、head %q、defaultBase %q、storyStore %q", got.Version, got.Branch, got.Head, got.DefaultBase, got.StoryStore)
	}
	if want := []string{"main", "origin/main", "release", reviewBranch}; !reflect.DeepEqual(got.Bases, want) {
		t.Errorf("bases: %v（思ったもの %v）", got.Bases, want)
	}
	if got.User == nil || got.User.Name != reviewUserName || got.User.Email != reviewUserEmail {
		t.Errorf("user: %+v", got.User)
	}
	var stories []string
	for _, st := range got.Stories {
		stories = append(stories, fmt.Sprintf("%s %v", st.ID, st.InBranch))
	}
	if want := []string{"#30 true", "#24 true", "#1 false"}; !reflect.DeepEqual(stories, want) {
		t.Errorf("stories: %v（思ったもの %v）", stories, want)
	}
	var layers []string
	for _, l := range got.Layers {
		layers = append(layers, l.ID+" "+l.Name)
	}
	if want := []string{"cli CLI", "model モデル", "db DB"}; !reflect.DeepEqual(layers, want) {
		t.Errorf("layers: %v（思ったもの %v）", layers, want)
	}

	// ブランチにいなければ、branch は空
	r.git("switch", "-q", "--detach")
	var detached contextResponse
	if code, msg := s.get("/api/context", &detached); code != 200 || detached.Branch != "" {
		t.Errorf("ブランチにいないときの branch: %q（%d %q）", detached.Branch, code, msg)
	}
}

// CM-08・CM-09: storyStore は、設定のストーリーの置き場所のアダプタ
func Test画面で選べるもののストーリーの置き場所は設定のアダプタを示す(t *testing.T) {
	for _, c := range []struct{ conf, want string }{
		{"\n[story]\nadapter = \"github\"\nrepository = \"owner/app\"\n", "github"},
		{"\n[story]\ncommand = [\"tools/story-adapter\"]\n", "command"},
	} {
		t.Run(c.want, func(t *testing.T) {
			bin := buildVanda(t)
			version, err := runningVersion(bin)
			if err != nil {
				t.Fatal(err)
			}
			r := newReviewRepo(t, version)
			writeFile(t, r.Root, ".vanda/config.toml", fmt.Sprintf(reviewConfig, version)+c.conf)
			r.git("commit", "-q", "-am", "設定にストーリーの置き場所を足す")
			s := startServe(t, bin, r.Root, r.Env)
			var got contextResponse
			if code, msg := s.get("/api/context", &got); code != 200 || got.StoryStore != c.want {
				t.Errorf("storyStore: %q（%d %q）", got.StoryStore, code, msg)
			}
		})
	}
}
