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
	ID        string `json:"id"`
	Statement frags  `json:"statement"`
	Rationale frags  `json:"rationale"`
	Condition frags  `json:"condition"`
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
	Guesses     []frags  `json:"guesses"`
	Evidence    []struct {
		Stage  string       `json:"stage"`
		Type   string       `json:"type"`
		Layers []string     `json:"layers"`
		Detail frags        `json:"detail"`
		Found  []claimFound `json:"found"`
	} `json:"evidence"`
}

// claimHistory は、約束の由来と合意の履歴（GET /api/claims/<約束の ID>/history）
type claimHistory struct {
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
		Story       string  `json:"story"`
		Fingerprint string  `json:"fingerprint"`
		Current     bool    `json:"current"`
		Commit      *string `json:"commit"`
	} `json:"agreements"`
}

// getHistory は、約束 id の由来と合意の履歴を読む。
func getHistory(t *testing.T, s *vandaServer, id string) claimHistory {
	t.Helper()
	var h claimHistory
	if code, msg := s.get("/api/claims/"+id+"/history", &h); code != 200 {
		t.Fatalf("約束 %s の由来と合意の履歴を読めない（%d）: %s", id, code, msg)
	}
	return h
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
// @vanda:evidence claim-detail-state
// @vanda:evidence claim-detail-fields
// @vanda:evidence claim-detail-evidence-content
// @vanda:evidence claim-detail-origin
// @vanda:evidence claim-detail-agreements
// @vanda:evidence found-file-state-shown
func Test約束の詳細は文とレイヤーと対象とエビデンスのタイプごとの中身と由来と合意の履歴を示す(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	// 推測だけを書き換えたコミット。指紋が変わらないので、由来には出ない
	writeFile(t, r.Root, ".vanda/promises.toml", strings.Replace(reviewPromisesStory,
		"guesses = [\"タスクは消しても ID を使い回さない\"]", "guesses = [\"消したタスクの ID は使い回さない\"]", 1))
	r.g.commitTracked("推測を書き換える", "#25")

	var got claimDetail
	if code, msg := s.get("/api/claims/task-id-unique", &got); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	if got.ID != "task-id-unique" || got.Statement.String() != "タスクの ID は、すべてのタスクを通して重ならない" || got.Rationale != nil || got.Condition != nil {
		t.Errorf("id・文・理由・条件: %q %q %v %v", got.ID, got.Statement, got.Rationale, got.Condition)
	}
	if len(got.Layers) != 2 || got.Layers[0].ID != "model" || got.Layers[0].Name != "モデル" || got.Layers[1].ID != "db" || got.Layers[1].Name != "DB" {
		t.Errorf("レイヤー: %+v", got.Layers)
	}
	if len(got.Subjects) != 1 || got.Subjects[0].Kind != "term" || got.Subjects[0].ID != "task-id" || got.Subjects[0].Name != "タスクの ID" {
		t.Errorf("対象: %+v", got.Subjects)
	}
	if got.State != "proposed" || got.Fingerprint != reviewClaimFP ||
		!reflect.DeepEqual(got.Flags, []string{"ai-proposal"}) || !reflect.DeepEqual(fragsList(got.Guesses), []string{"消したタスクの ID は使い回さない"}) {
		t.Errorf("state %q、fingerprint %q、flags %v、guesses %v", got.State, got.Fingerprint, got.Flags, got.Guesses)
	}

	wantEvidence := []struct {
		stage, typ string
		layers     []string
		found      []wantFound
	}{
		{"implementation", "scenario-test", []string{"model"}, []wantFound{{"txtar", "e2e/add-task.txtar", "add-task", 1, sp(reviewScenarioStory)}}},
		{"plan", "model-check", []string{"model"}, []wantFound{{"tla", "spec/Task.tla", "Unique", 5,
			sp("Unique == \\A a, b \\in DOMAIN tasks : tasks[a] = tasks[b] => a = b\n")}}},
		{"implementation", "guardrail", []string{"db"}, []wantFound{{"sql", "db/schema.sql", "CREATE UNIQUE INDEX task_id_unique ON task (id);", 4, sp(reviewSchema)}}},
		{"implementation", "unit-test", []string{"model"}, []wantFound{{"go", "internal/task/task_test.go", "TestUnique", 6, nil}}},
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
	h := getHistory(t, s, "task-id-unique")
	if len(h.Origin) != 2 {
		t.Fatalf("由来が 2 件（作ったコミットと、文を変えたコミット）でない: %+v", h.Origin)
	}
	// 由来はコミットの日時の新しい順（同じ日時なら、履歴で後のコミットが先）。どちらでも、文を変えたコミットが先
	o0, o1 := h.Origin[0], h.Origin[1]
	if o0.Commit != changed || !reflect.DeepEqual(o0.Stories, []string{"#24"}) || o0.Change != "modified" {
		t.Errorf("由来の 1 件目: %+v", o0)
	}
	if o1.Commit != first || o1.Subject != "はじめの約束を置く" || !reflect.DeepEqual(o1.Stories, []string{"#1"}) || o1.Change != "added" {
		t.Errorf("由来の 2 件目: %+v", o1)
	}

	if len(h.Agreements) != 1 || h.Agreements[0].Story != "#1" || h.Agreements[0].Current ||
		h.Agreements[0].Fingerprint != sha256Hex([]byte(reviewItemJSON["claim task-id-unique"])) || h.Agreements[0].User.Name != "テスト担当" {
		t.Errorf("合意の履歴: %+v", h.Agreements)
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
	r.g.commitTracked("合意する", "#24")
	var again claimDetail
	if code, msg := s.get("/api/claims/task-id-unique", &again); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	if again.ID != "task-id-unique" || again.State != "agreed" {
		t.Errorf("今の中身に合意した後の id と state: %q %q（task-id-unique と agreed のはず）", again.ID, again.State)
	}
	// 合意の履歴は合意した日時の新しい順。agreementTOML はどれも同じ日時なので、ファイルで後に書いた #24 の合意が先
	if ah := getHistory(t, s, "task-id-unique"); len(ah.Agreements) != 2 || !ah.Agreements[0].Current || ah.Agreements[0].Story != reviewStory || ah.Agreements[1].Current {
		t.Errorf("合意した後の合意の履歴: %+v", ah.Agreements)
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
	r.g.commitTracked("シナリオに合意する", "#30")
	var agreedFile claimDetail
	if code, msg := s.get("/api/claims/task-id-unique", &agreedFile); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	wantStates["e2e/add-task.txtar"] = "agreed"
	if got := foundStates(agreedFile); !reflect.DeepEqual(got, wantStates) {
		t.Errorf("シナリオに合意した後の found の合意の状態: %v（思ったもの %v）", got, wantStates)
	}
}

// CL-01: 約束リストにない ID は、約束リストの形の審査の誤りになるが、約束の詳細は出す
// @vanda:evidence claim-detail-subjects
func Test約束の詳細の対象は用語か機能の名前とIDを約束のsubjectsの順に示し約束リストにないIDは種類と名前をnullにする(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	writeFile(t, r.Root, ".vanda/promises.toml", reviewPromisesStory+`
[[claim]]
id = "task-subjects"
subjects = ["add-task", "no-such-subject", "task-id"]
statement = "対象を 3 つ持つ"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
`)
	r.g.commitTracked("対象を 3 つ持つ約束を足す", reviewStory)
	var got struct {
		Subjects []struct {
			Kind *string `json:"kind"`
			ID   string  `json:"id"`
			Name *string `json:"name"`
		} `json:"subjects"`
	}
	if code, msg := s.get("/api/claims/task-subjects", &got); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	want := []string{"feature add-task タスクの登録", "<null> no-such-subject <null>", "term task-id タスクの ID"}
	var have []string
	for _, sub := range got.Subjects {
		show := func(p *string) string {
			if p == nil {
				return "<null>"
			}
			return *p
		}
		have = append(have, show(sub.Kind)+" "+sub.ID+" "+show(sub.Name))
	}
	if strings.Join(have, "\n") != strings.Join(want, "\n") {
		t.Errorf("対象:\n%s\n--- 思ったもの\n%s", strings.Join(have, "\n"), strings.Join(want, "\n"))
	}
}

// CL-02: エビデンスの予定があっても、指すアノテーションがなければ found は空
// @vanda:evidence claim-detail-evidence-content
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
	if h := getHistory(t, s, "task-title-required"); len(h.Origin) != 1 || h.Origin[0].Change != "added" || !reflect.DeepEqual(h.Origin[0].Stories, []string{"#24"}) || len(h.Agreements) != 0 {
		t.Errorf("由来 %+v、合意の履歴 %+v", h.Origin, h.Agreements)
	}
}

// @vanda:evidence api-error-shape
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

// CL-08: GET /api/claims/<ID>/at/<コミット> は、そのコミットの時点の約束の中身を返す（docs/formats.md）。
// 期待する文は、newReviewRepo が main と story-24 に書いた約束リストから手で書いた。
// @vanda:evidence claim-at-commit
func Testその時点の約束の中身は由来のコミットと合意の記録のコミットの時点の文を返す(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	cases := []struct{ rev, want string }{
		{"main", "タスクの ID は重ならない"},
		{reviewBranch + "~1", "タスクの ID は、すべてのタスクを通して重ならない"},
	}
	for _, c := range cases {
		commit := r.git("rev-parse", c.rev)
		var got struct {
			ID      string `json:"id"`
			Commit  string `json:"commit"`
			Content struct {
				Statement frags `json:"statement"`
			} `json:"content"`
		}
		if code, msg := s.get("/api/claims/task-id-unique/at/"+commit, &got); code != 200 {
			t.Fatalf("%s の時点の中身を読めない（%d）: %s", c.rev, code, msg)
		}
		if got.ID != "task-id-unique" || got.Commit != commit || got.Content.Statement.String() != c.want {
			t.Errorf("%s の時点: %+v（思った文 %q）", c.rev, got, c.want)
		}
		// 短いハッシュ（4 文字以上）でも同じものを返す
		var short struct {
			Content struct {
				Statement frags `json:"statement"`
			} `json:"content"`
		}
		if code, msg := s.get("/api/claims/task-id-unique/at/"+commit[:7], &short); code != 200 || short.Content.Statement.String() != c.want {
			t.Errorf("短いハッシュで %s の時点を読めない（%d %q）: %+v", c.rev, code, msg, short)
		}
	}
	// 合意の記録の commit（記録を書き足したコミット）を渡すと、その時点の中身を返す。#1 の合意は main のコミットで書いた
	detail := getHistory(t, s, "task-id-unique")
	if len(detail.Agreements) == 0 || detail.Agreements[0].Commit == nil {
		t.Fatalf("合意の履歴の commit を読めない: %+v", detail)
	}
	if *detail.Agreements[0].Commit != r.git("rev-parse", "main") {
		t.Errorf("#1 の合意の commit が main のコミットでない: %s", *detail.Agreements[0].Commit)
	}
}

// CL-08: ハッシュでない形のコミットは 400、ないコミットとその時点にない約束は 404。どれも理由を出す
// @vanda:evidence claim-at-commit-errors
func Testその時点の約束の中身はハッシュでない形を400にしないコミットとその時点にない約束を404にする(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	main := r.git("rev-parse", "main")
	cases := []struct {
		name, path string
		want       int
	}{
		{"ハッシュでない文字", "/api/claims/task-id-unique/at/main", 400},
		{"短すぎるハッシュ", "/api/claims/task-id-unique/at/abc", 400},
		{"16進でない文字", "/api/claims/task-id-unique/at/zzzzzzz", 400},
		{"65 文字のハッシュ", "/api/claims/task-id-unique/at/" + strings.Repeat("a", 65), 400},
		{"ないコミット", "/api/claims/task-id-unique/at/" + strings.Repeat("0", 40), 404},
		// task-title-required は story-24 で足したので、main の時点にはない
		{"その時点にない約束", "/api/claims/task-title-required/at/" + main, 404},
	}
	for _, c := range cases {
		if code, msg := s.get(c.path, nil); code != c.want || msg == "" {
			t.Errorf("%s: %d（%q）を返した。%d で理由を出す", c.name, code, msg, c.want)
		}
	}
}

// CL-08: その時点のレイヤーのうち今の設定にないものは名前を空にし、その時点の約束リストが読めなければ 500 で理由を出す。
// どちらの時点も、コミットした後に今の約束リストを元に戻す（今の中身は読めるままにする）
// @vanda:evidence claim-at-commit-unknown-layer
// @vanda:evidence claim-at-commit-error-shown
func Testその時点の約束の中身は今の設定にないレイヤーの名前を空にし読めない約束リストの時点を500にして理由を出す(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	now := readFileString(t, r.Root, ".vanda/promises.toml")
	commitPromises := func(src, subject string) string {
		writeFile(t, r.Root, ".vanda/promises.toml", src)
		r.g.commitTracked(subject, reviewStory)
		return r.git("rev-parse", "HEAD")
	}
	const layers = `layers = ["model", "db"]`
	if strings.Count(now, layers) < 1 {
		t.Fatalf("約束リストに %s がない", layers)
	}
	gone := commitPromises(strings.Replace(now, layers, `layers = ["gone", "model"]`, 1), "今の設定にないレイヤーを書く")
	broken := commitPromises(now+"\n[[claim]\n", "約束リストを壊す")
	commitPromises(now, "約束リストを戻す")

	var got struct {
		Layers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"layers"`
		Content struct {
			Layers []string `json:"layers"`
		} `json:"content"`
	}
	if code, msg := s.get("/api/claims/task-id-unique/at/"+gone, &got); code != 200 {
		t.Fatalf("今の設定にないレイヤーのある時点を読めない（%d）: %s", code, msg)
	}
	type layer = struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	want := []layer{{"gone", ""}, {"model", "モデル"}}
	if !reflect.DeepEqual(got.Layers, want) || !reflect.DeepEqual(got.Content.Layers, []string{"gone", "model"}) {
		t.Errorf("layers: %+v、content.layers: %v（思ったもの %+v と [gone model]）", got.Layers, got.Content.Layers, want)
	}

	code, msg := s.get("/api/claims/task-id-unique/at/"+broken, nil)
	if code != 500 || msg == "" {
		t.Errorf("読めない約束リストの時点: %d（%q）を返した。500 で理由を出す", code, msg)
	}
}

// GET /api/file は、合意の対象のファイルとアノテーションを読むファイルの中身を返し、ほかのファイルは 404 にする
// @vanda:evidence file-api
func Testファイルの中身のAPIは合意の対象とアノテーションを読むファイルだけを返しほかを404にする(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	// 設定の txtar・tla・sql・go のパターンと、ガードレールの paths に当たるもの（中身は newReviewRepo が書いたもの）
	for path, want := range map[string]string{
		"e2e/add-task.txtar":         reviewScenarioStory,
		"spec/Task.tla":              reviewTLA,
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
	writeFile(t, r.Root, "README.md", "どのパターンにも当たらない\n")
	for _, path := range []string{".vanda/config.toml", ".vanda/promises.toml", "README.md", "e2e/none.txtar", "../e2e/add-task.txtar", "/etc/passwd"} {
		if code, msg := s.get("/api/file?path="+q(path), nil); code != 404 || msg == "" {
			t.Errorf("%s: %d（%q）を返した。404 で理由を出す", path, code, msg)
		}
	}
}

// @vanda:evidence origin-commit-stories
func Test約束の由来はコミットごとにそのコミットのStoryのトレーラーのストーリーを結ぶ(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	statement := "タスクの ID は、すべてのタスクを通して重ならない"
	change := func(to, msg string) string {
		t.Helper()
		text := readFileString(t, r.Root, ".vanda/promises.toml")
		writeFile(t, r.Root, ".vanda/promises.toml", strings.Replace(text, statement, to, 1))
		statement = to
		// 同じ大きさの書き換えを git が見落とさないよう、索引から外して読み直させる（commitAll と同じ）
		r.g.run("rm", "-r", "-q", "--cached", "--ignore-unmatch", ".")
		r.g.run("add", "-A")
		r.g.run("commit", "-q", "-m", msg)
		return r.git("rev-parse", "HEAD")
	}
	// トレーラーのないコミット（subject と本文の ID は数えない）と、ほかのストーリーのトレーラーのコミット
	none := change("タスクの ID は重ならない（1）", "#31 で文を変える\n\n本文の #32 も数えない")
	other := change("タスクの ID は、どのタスクとも重ならない", "文をもう一度変える\n\nStory: #33")
	want := map[string][]string{
		r.git("rev-parse", "main"): {"#1"},
		r.git("log", "-1", "--format=%H", "--fixed-strings", "--grep=タスクの ID の約束を変え", reviewBranch): {"#24"},
		none:  nil,
		other: {"#33"},
	}
	got := getHistory(t, s, "task-id-unique")
	if len(got.Origin) != len(want) {
		t.Fatalf("由来が %d 件（%d 件のはず）: %+v", len(got.Origin), len(want), got.Origin)
	}
	for _, o := range got.Origin {
		w, ok := want[o.Commit]
		if !ok {
			t.Errorf("思っていないコミットが由来にある: %+v", o)
			continue
		}
		if len(o.Stories) != len(w) || (len(w) > 0 && !reflect.DeepEqual(o.Stories, w)) {
			t.Errorf("コミット %s（%s）のストーリー: %v（思ったもの %v）", o.Commit, o.Subject, o.Stories, w)
		}
	}
}
