package main_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// CL-01・02・04・05 約束の詳細（GET /api/claims/<約束の ID>）と、ファイルの中身（GET /api/file）。
// リポジトリは review_repo_test.go の newReviewRepo（HEAD は story-24）。

type claimFound struct {
	Adapter string  `json:"adapter"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	Name    string  `json:"name"`
	Text    *string `json:"text"`
	State   *string `json:"state"`
}

// foundStates は、エビデンスの found の場所ごとの合意の状態（null は ""）を返す。
func foundStates(d claimDetail) map[string]string {
	out := map[string]string{}
	for _, e := range d.Evidence {
		for _, f := range e.Found {
			out[f.File] = str(f.State)
		}
	}
	return out
}

type claimDetail struct {
	ID        string  `json:"id"`
	Statement string  `json:"statement"`
	Rationale *string `json:"rationale"`
	Condition *string `json:"condition"`
	Layers    []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"layers"`
	Subjects []struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"subjects"`
	State       string   `json:"state"`
	Fingerprint string   `json:"fingerprint"`
	Flags       []string `json:"flags"`
	Guesses     []string `json:"guesses"`
	Evidence    []struct {
		Stage  string       `json:"stage"`
		Type   string       `json:"type"`
		Layers []string     `json:"layers"`
		Detail *string      `json:"detail"`
		Found  []claimFound `json:"found"`
	} `json:"evidence"`
	Origin []struct {
		Commit  string   `json:"commit"`
		Subject string   `json:"subject"`
		Stories []string `json:"stories"`
		Change  string   `json:"change"`
	} `json:"origin"`
	Agreements []struct {
		User struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
		Story       string `json:"story"`
		Fingerprint string `json:"fingerprint"`
		Current     bool   `json:"current"`
	} `json:"agreements"`
}

// 期待する found。line は、アノテーションが付いているもの（定義・関数・SQL の文）の行（監督の決定）。
// シナリオはファイル全体に付くので、アノテーションのある 1 行目とする。
type wantFound struct {
	adapter, file, name string
	line                int
	text                *string // nil なら text は null
}

func sp(s string) *string { return &s }

func checkFound(t *testing.T, typ string, got []claimFound, want []wantFound) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s の found が %d 件（%d 件のはず）: %+v", typ, len(got), len(want), got)
		return
	}
	for i, w := range want {
		g := got[i]
		if g.Adapter != w.adapter || g.File != w.file || g.Name != w.name || g.Line != w.line {
			t.Errorf("%s の found: %+v（思ったもの %+v）", typ, g, w)
		}
		switch {
		case w.text == nil && g.Text != nil:
			t.Errorf("%s の text は null のはず: %q", typ, *g.Text)
		// TLA+ の定義は終わりの空行を含めない（監督の決定）。定義の最後の行の改行を含めるかは決まっていないので、どちらでもよい
		case w.text != nil && (g.Text == nil || (*g.Text != *w.text && *g.Text != strings.TrimSuffix(*w.text, "\n"))):
			t.Errorf("%s の text が思ったものでない: %v\n--- 思ったもの\n%s", typ, g.Text, *w.text)
		}
	}
}

// CL-01・02・04・05
func Test約束の詳細は文とレイヤーと対象とエビデンスのタイプごとの中身と由来と合意の履歴を示す(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	// 推測だけを書き換えたコミット。指紋が変わらないので、由来には出ない
	writeFile(t, r.Root, ".vanda/promises.toml", strings.Replace(reviewPromisesStory,
		"guesses = [\"タスクは消しても ID を使い回さない\"]", "guesses = [\"消したタスクの ID は使い回さない\"]", 1))
	r.git("commit", "-q", "-am", "#25 推測を書き換える")

	var got claimDetail
	if code, msg := s.get("/api/claims/task-id-unique", &got); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	if got.ID != "task-id-unique" || got.Statement != "タスクの ID は、すべてのタスクを通して重ならない" || got.Rationale != nil || got.Condition != nil {
		t.Errorf("id・文・理由・条件: %q %q %v %v", got.ID, got.Statement, got.Rationale, got.Condition)
	}
	if len(got.Layers) != 2 || got.Layers[0].ID != "model" || got.Layers[0].Name != "モデル" || got.Layers[1].ID != "db" || got.Layers[1].Name != "DB" {
		t.Errorf("レイヤー: %+v", got.Layers)
	}
	if len(got.Subjects) != 1 || got.Subjects[0].Kind != "term" || got.Subjects[0].ID != "task-id" || got.Subjects[0].Name != "タスクの ID" {
		t.Errorf("対象: %+v", got.Subjects)
	}
	if got.State != "proposed" || got.Fingerprint != reviewClaimFP ||
		!reflect.DeepEqual(got.Flags, []string{"ai-proposal"}) || !reflect.DeepEqual(got.Guesses, []string{"消したタスクの ID は使い回さない"}) {
		t.Errorf("state %q、fingerprint %q、flags %v、guesses %v", got.State, got.Fingerprint, got.Flags, got.Guesses)
	}

	wantEvidence := []struct {
		stage, typ string
		layers     []string
		found      []wantFound
	}{
		{"test", "scenario-test", []string{"model"}, []wantFound{{"txtar", "e2e/add-task.txtar", "add-task", 1, sp(reviewScenarioStory)}}},
		{"plan", "model-check", []string{"model"}, []wantFound{{"tla", "spec/Task.tla", "Unique", 5,
			sp("Unique == \\A a, b \\in DOMAIN tasks : tasks[a] = tasks[b] => a = b\n")}}},
		{"implementation", "guardrail", []string{"db"}, []wantFound{{"sql", "db/schema.sql", "CREATE UNIQUE INDEX task_id_unique ON task (id);", 4, sp(reviewSchema)}}},
		{"test", "unit-test", []string{"model"}, []wantFound{{"go", "internal/task/task_test.go", "TestUnique", 6, nil}}},
	}
	if len(got.Evidence) != len(wantEvidence) {
		t.Fatalf("エビデンスが %d 件: %+v", len(got.Evidence), got.Evidence)
	}
	for i, w := range wantEvidence {
		g := got.Evidence[i]
		if g.Stage != w.stage || g.Type != w.typ || !reflect.DeepEqual(g.Layers, w.layers) || g.Detail != nil {
			t.Errorf("%d 件目のエビデンス: %+v", i+1, g)
		}
		checkFound(t, w.typ, g.Found, w.found)
	}
	// 合意の対象のファイルは、今の中身の指紋に合う合意の記録があれば agreed、なければ proposed。Go のテストは null。
	// シナリオは #24 で変えたので、#1 の合意は今の中身に合わない。TLA+ と SQL は #1 のまま
	wantStates := map[string]string{"e2e/add-task.txtar": "proposed", "spec/Task.tla": "agreed", "db/schema.sql": "agreed", "internal/task/task_test.go": ""}
	if got := foundStates(got); !reflect.DeepEqual(got, wantStates) {
		t.Errorf("found の合意の状態: %v（思ったもの %v）", got, wantStates)
	}

	first := r.git("rev-parse", "main")
	changed := r.git("log", "-1", "--format=%H", "--fixed-strings", "--grep=タスクの ID の約束を変え", reviewBranch)
	if len(got.Origin) != 2 {
		t.Fatalf("由来が 2 件（作ったコミットと、文を変えたコミット）でない: %+v", got.Origin)
	}
	// 由来はコミットの日時の新しい順（同じ日時なら、履歴で後のコミットが先）。どちらでも、文を変えたコミットが先
	o0, o1 := got.Origin[0], got.Origin[1]
	if o0.Commit != changed || !reflect.DeepEqual(o0.Stories, []string{"#24"}) || o0.Change != "modified" {
		t.Errorf("由来の 1 件目: %+v", o0)
	}
	if o1.Commit != first || o1.Subject != "#1 はじめの約束を置く" || !reflect.DeepEqual(o1.Stories, []string{"#1"}) || o1.Change != "added" {
		t.Errorf("由来の 2 件目: %+v", o1)
	}

	if len(got.Agreements) != 1 || got.Agreements[0].Story != "#1" || got.Agreements[0].Current ||
		got.Agreements[0].Fingerprint != sha256Hex([]byte(reviewItemJSON["claim task-id-unique"])) || got.Agreements[0].User.Name != "テスト担当" {
		t.Errorf("合意の履歴: %+v", got.Agreements)
	}

	// 今の中身に合意すると、合意の履歴に足され、今の中身に合うと分かる。前の合意は消えず、合わないまま
	f, err := os.OpenFile(filepath.Join(r.Root, ".vanda", "agreements.toml"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(agreementTOML("claim", "task-id-unique", reviewClaimFP, "", reviewStory, reviewUserName, reviewUserEmail))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	r.git("commit", "-q", "-am", "#24 で合意する")
	var again claimDetail
	if code, msg := s.get("/api/claims/task-id-unique", &again); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	// 合意の履歴は合意した日時の新しい順。agreementTOML はどれも同じ日時なので、ファイルで後に書いた #24 の合意が先
	if len(again.Agreements) != 2 || !again.Agreements[0].Current || again.Agreements[0].Story != reviewStory || again.Agreements[1].Current {
		t.Errorf("合意した後の合意の履歴: %+v", again.Agreements)
	}

	// シナリオの今の中身に、#24 でないストーリー（#30）で合意しても、合意の状態は agreed になる（ストーリーによらない）
	f, err = os.OpenFile(filepath.Join(r.Root, ".vanda", "agreements.toml"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(agreementTOML("file", reviewFileFP, reviewFileFP, "e2e/add-task.txtar", "#30", reviewUserName, reviewUserEmail))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	r.git("commit", "-q", "-am", "#30 でシナリオに合意する")
	var agreedFile claimDetail
	if code, msg := s.get("/api/claims/task-id-unique", &agreedFile); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	wantStates["e2e/add-task.txtar"] = "agreed"
	if got := foundStates(agreedFile); !reflect.DeepEqual(got, wantStates) {
		t.Errorf("シナリオに合意した後の found の合意の状態: %v（思ったもの %v）", got, wantStates)
	}
}

// CL-02: エビデンスの予定があっても、指すアノテーションがなければ found は空
func Test約束の詳細は指すアノテーションのないエビデンスの予定をfoundが空のものとして示す(t *testing.T) {
	t.Parallel()
	_, _, s := setupAgree(t)
	var got claimDetail
	if code, msg := s.get("/api/claims/task-title-required", &got); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Type != "scenario-test" || got.Evidence[0].Found == nil || len(got.Evidence[0].Found) != 0 {
		t.Errorf("エビデンス: %+v", got.Evidence)
	}
	if len(got.Origin) != 1 || got.Origin[0].Change != "added" || !reflect.DeepEqual(got.Origin[0].Stories, []string{"#24"}) || len(got.Agreements) != 0 {
		t.Errorf("由来 %+v、合意の履歴 %+v", got.Origin, got.Agreements)
	}
}

func Test約束の詳細とファイルの中身はないものを404にする(t *testing.T) {
	t.Parallel()
	_, _, s := setupAgree(t)
	for _, path := range []string{
		"/api/claims/no-such-claim",
		"/api/claims/task-id", // 用語の ID
		"/api/file?path=" + q("README.md"),
		"/api/file?path=" + q(".vanda/config.toml"),
		"/api/file?path=" + q("../outside.txt"),
	} {
		if code, msg := s.get(path, nil); code != 404 || msg == "" {
			t.Errorf("%s に %d（%q）を返した。404 で、理由を出す", path, code, msg)
		}
	}
}

func Testファイルの中身は合意の対象のファイルとアノテーションを読むファイルを返す(t *testing.T) {
	t.Parallel()
	_, _, s := setupAgree(t)
	for path, want := range map[string]string{
		"spec/Task.tla":              reviewTLA,
		"e2e/add-task.txtar":         reviewScenarioStory,
		"db/schema.sql":              reviewSchema,
		"internal/task/task_test.go": reviewGoTest,
	} {
		var got struct {
			Path string `json:"path"`
			Text string `json:"text"`
		}
		if code, msg := s.get("/api/file?path="+q(path), &got); code != 200 || got.Path != path || got.Text != want {
			t.Errorf("%s: %d（%q）、%+v", path, code, msg, got)
		}
	}
}
