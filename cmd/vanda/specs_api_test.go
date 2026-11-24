package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// CL-01・02・04・05 仕様の詳細（GET /api/specs/<仕様の ID>）と、ファイルの中身（GET /api/file）。
// リポジトリは review_repo_test.go の newReviewRepo（HEAD は story-24）。

type specFound struct {
	Adapter string  `json:"adapter"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	Name    string  `json:"name"`
	Text    *string `json:"text"`
	State   *string `json:"state"`
}

// foundStates は、エビデンスの found の場所ごとの承認の状態（null は ""）を返す。
func foundStates(d specDetail) map[string]string {
	out := map[string]string{}
	for _, e := range d.Evidence {
		for _, f := range e.Found {
			out[f.File] = str(f.State)
		}
	}
	return out
}

type specDetail struct {
	ID        string `json:"id"`
	Statement frags  `json:"statement"`
	Rationale frags  `json:"rationale"`
	Condition frags  `json:"condition"`
	Layers    []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"layers"`
	State       string   `json:"state"`
	Fingerprint string   `json:"fingerprint"`
	Flags       []string `json:"flags"`
	Guesses     []frags  `json:"guesses"`
	Evidence    []struct {
		Stage  string      `json:"stage"`
		Type   string      `json:"type"`
		Layers []string    `json:"layers"`
		Detail frags       `json:"detail"`
		Found  []specFound `json:"found"`
	} `json:"evidence"`
}

// specHistory は、仕様の変更履歴と承認の履歴（GET /api/specs/<仕様の ID>/history）
type specHistory struct {
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

// getHistory は、仕様 id の変更履歴と承認の履歴を読む。
func getHistory(t *testing.T, s *vandaServer, id string) specHistory {
	t.Helper()
	var h specHistory
	if code, msg := s.get("/api/specs/"+id+"/history", &h); code != 200 {
		t.Fatalf("仕様 %s の変更履歴と承認の履歴を読めない（%d）: %s", id, code, msg)
	}
	return h
}

// 期待する found。line は、アノテーションが付いているもの（定義・関数・SQL の文）の行（オーナーの決定）。
// シナリオはファイル全体に付くので、アノテーションのある 1 行目とする。
type wantFound struct {
	adapter, file, name string
	line                int
	text                *string // nil なら text は null
}

func sp(s string) *string { return &s }

func checkFound(t *testing.T, typ string, got []specFound, want []wantFound) {
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
		// TLA+ の定義は終わりの空行を含めない（オーナーの決定）。定義の最後の行の改行を含めるかは決まっていないので、どちらでもよい
		case w.text != nil && (g.Text == nil || (*g.Text != *w.text && *g.Text != strings.TrimSuffix(*w.text, "\n"))):
			t.Errorf("%s の text が思ったものでない: %v\n--- 思ったもの\n%s", typ, g.Text, *w.text)
		}
	}
}

// CL-01・02・04・05
// @vanda:evidence item-detail-in-one-place
// @vanda:evidence agreement-history-kept
func Test仕様の詳細は文とレイヤーとエビデンスのタイプごとの中身と変更履歴と承認の履歴を示す(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	// 推測だけを書き換えたコミット。フィンガープリントが変わらないので、変更履歴には出ない
	writeFile(t, r.Root, ".vanda/promises.toml", strings.Replace(reviewPromisesStory,
		"guesses = [\"タスクは消しても ID を使い回さない\"]", "guesses = [\"消したタスクの ID は使い回さない\"]", 1))
	r.g.commitTracked("推測を書き換える", "#25")

	var got specDetail
	if code, msg := s.get("/api/specs/task-id-unique", &got); code != 200 {
		t.Fatalf("仕様の詳細を読めない（%d）: %s", code, msg)
	}
	if got.ID != "task-id-unique" || got.Statement.String() != "タスクの ID は、すべてのタスクを通して重ならない" || got.Rationale != nil || got.Condition != nil {
		t.Errorf("id・文・理由・条件: %q %q %v %v", got.ID, got.Statement, got.Rationale, got.Condition)
	}
	if len(got.Layers) != 2 || got.Layers[0].ID != "model" || got.Layers[0].Name != "モデル" || got.Layers[1].ID != "db" || got.Layers[1].Name != "DB" {
		t.Errorf("レイヤー: %+v", got.Layers)
	}
	if got.State != "proposed" || got.Fingerprint != reviewSpecFP ||
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
	// 承認の対象のファイルは、今の中身のフィンガープリントに合う承認の記録があれば agreed、なければ proposed。Go のテストは null。
	// シナリオは #24 で変えたので、#1 の承認は今の中身に合わない。TLA+ と SQL は #1 のまま
	wantStates := map[string]string{"e2e/add-task.txtar": "proposed", "spec/Task.tla": "agreed", "db/schema.sql": "agreed", "internal/task/task_test.go": ""}
	if got := foundStates(got); !reflect.DeepEqual(got, wantStates) {
		t.Errorf("found の承認の状態: %v（思ったもの %v）", got, wantStates)
	}

	first := r.git("rev-parse", "main")
	changed := r.git("log", "-1", "--format=%H", "--fixed-strings", "--grep=タスクの ID の仕様を変え", reviewBranch)
	h := getHistory(t, s, "task-id-unique")
	if len(h.Origin) != 2 {
		t.Fatalf("変更履歴が 2 件（作ったコミットと、文を変えたコミット）でない: %+v", h.Origin)
	}
	// 変更履歴はコミットの日時の新しい順（同じ日時なら、履歴で後のコミットが先）。どちらでも、文を変えたコミットが先
	o0, o1 := h.Origin[0], h.Origin[1]
	if o0.Commit != changed || !reflect.DeepEqual(o0.Stories, []string{"#24"}) || o0.Change != "modified" {
		t.Errorf("変更履歴の 1 件目: %+v", o0)
	}
	if o1.Commit != first || o1.Subject != "はじめの仕様を置く" || !reflect.DeepEqual(o1.Stories, []string{"#1"}) || o1.Change != "added" {
		t.Errorf("変更履歴の 2 件目: %+v", o1)
	}

	if len(h.Agreements) != 1 || h.Agreements[0].Story != "#1" || h.Agreements[0].Current ||
		h.Agreements[0].Fingerprint != sha256Hex([]byte(reviewItemJSON["spec task-id-unique"])) || h.Agreements[0].User.Name != "テスト担当" {
		t.Errorf("承認の履歴: %+v", h.Agreements)
	}

	// 今の中身を承認すると、承認の履歴に足され、今の中身に合うと分かる。前の承認は消えず、合わないまま
	f, err := os.OpenFile(filepath.Join(r.Root, ".vanda", "agreements.toml"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(agreementTOML("spec", "task-id-unique", reviewSpecFP, "", reviewStory, reviewUserName, reviewUserEmail))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	r.g.commitTracked("承認する", "#24")
	var again specDetail
	if code, msg := s.get("/api/specs/task-id-unique", &again); code != 200 {
		t.Fatalf("仕様の詳細を読めない（%d）: %s", code, msg)
	}
	if again.ID != "task-id-unique" || again.State != "agreed" {
		t.Errorf("今の中身を承認した後の id と state: %q %q（task-id-unique と agreed のはず）", again.ID, again.State)
	}
	// 承認の履歴は承認した日時の新しい順。agreementTOML はどれも同じ日時なので、ファイルで後に書いた #24 の承認が先
	if ah := getHistory(t, s, "task-id-unique"); len(ah.Agreements) != 2 || !ah.Agreements[0].Current || ah.Agreements[0].Story != reviewStory || ah.Agreements[1].Current {
		t.Errorf("承認した後の承認の履歴: %+v", ah.Agreements)
	}

	// シナリオの今の中身に、#24 でないストーリー（#30）で承認しても、承認の状態は agreed になる（ストーリーによらない）
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
	r.g.commitTracked("シナリオを承認する", "#30")
	var agreedFile specDetail
	if code, msg := s.get("/api/specs/task-id-unique", &agreedFile); code != 200 {
		t.Fatalf("仕様の詳細を読めない（%d）: %s", code, msg)
	}
	wantStates["e2e/add-task.txtar"] = "agreed"
	if got := foundStates(agreedFile); !reflect.DeepEqual(got, wantStates) {
		t.Errorf("シナリオを承認した後の found の承認の状態: %v（思ったもの %v）", got, wantStates)
	}
}

// CL-02: エビデンスの予定があっても、指すアノテーションがなければ found は空
// @vanda:evidence item-detail-in-one-place
func Test仕様の詳細は指すアノテーションのないエビデンスの予定をfoundが空のものとして示す(t *testing.T) {
	t.Parallel()
	_, _, s := setupAgree(t)
	var got specDetail
	if code, msg := s.get("/api/specs/task-title-required", &got); code != 200 {
		t.Fatalf("仕様の詳細を読めない（%d）: %s", code, msg)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Type != "scenario-test" || got.Evidence[0].Found == nil || len(got.Evidence[0].Found) != 0 {
		t.Errorf("エビデンス: %+v", got.Evidence)
	}
	if h := getHistory(t, s, "task-title-required"); len(h.Origin) != 1 || h.Origin[0].Change != "added" || !reflect.DeepEqual(h.Origin[0].Stories, []string{"#24"}) || len(h.Agreements) != 0 {
		t.Errorf("変更履歴 %+v、承認の履歴 %+v", h.Origin, h.Agreements)
	}
}

// spec-detail-shown-before-history: 仕様の詳細は変更履歴と承認の履歴を返さず、それらは /history で返す（docs/formats.md）
// @vanda:evidence item-detail-in-one-place
func Test仕様の詳細は変更履歴と承認の履歴を含めず変更履歴と承認の履歴はhistoryで返しない仕様は404にする(t *testing.T) {
	t.Parallel()
	_, _, s := setupAgree(t)
	var detail map[string]json.RawMessage
	if code, msg := s.get("/api/specs/task-id-unique", &detail); code != 200 {
		t.Fatalf("仕様の詳細を読めない（%d）: %s", code, msg)
	}
	for _, k := range []string{"origin", "agreements"} {
		if _, ok := detail[k]; ok {
			t.Errorf("仕様の詳細に %s がある: %s", k, detail[k])
		}
	}
	for _, k := range []string{"id", "state", "statement", "layers", "guesses", "evidence"} {
		if _, ok := detail[k]; !ok {
			t.Errorf("仕様の詳細に %s がない", k)
		}
	}
	var history map[string]json.RawMessage
	if code, msg := s.get("/api/specs/task-id-unique/history", &history); code != 200 {
		t.Fatalf("変更履歴と承認の履歴を読めない（%d）: %s", code, msg)
	}
	var keys []string
	for k := range history {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"agreements", "origin"}) {
		t.Errorf("変更履歴と承認の履歴の項目: %v（agreements と origin だけのはず）", keys)
	}
	h := getHistory(t, s, "task-id-unique")
	if len(h.Origin) == 0 || len(h.Agreements) == 0 {
		t.Errorf("変更履歴か承認の履歴が空: %+v", h)
	}
	code, msg := s.get("/api/specs/no-such-spec/history", nil)
	if code != 404 || msg != "仕様 no-such-spec は約束リストにありません。" {
		t.Errorf("ない仕様の変更履歴と承認の履歴: %d %q", code, msg)
	}
}

// 承認の履歴は承認した日時の新しい順、同じ日時なら承認の記録のファイルで後に書いたものが先（docs/formats.md）。
// ファイルの順と日時の順をずらして書き、並べ替えを 2 回かけるような誤りでも並びが崩れれば分かるようにする
// @vanda:evidence agreement-history-kept
func Test承認の履歴は承認した日時の新しい順に並び同じ日時ならファイルで後に書いたものが先になる(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	// ファイルに書く順の承認（ストーリーで見分ける）と、承認した日時
	recs := []struct{ story, at string }{
		{"#41", "2026-10-02T12:00:00+09:00"},
		{"#42", "2026-10-05T12:00:00+09:00"},
		{"#43", "2026-10-03T12:00:00+09:00"},
		{"#44", "2026-10-05T12:00:00+09:00"},
		{"#45", "2026-10-01T12:00:00+09:00"},
	}
	want := []string{"#44", "#42", "#43", "#41", "#45"}
	var b strings.Builder
	for _, rec := range recs {
		b.WriteString(strings.Replace(agreementTOML("spec", "task-id-unique", reviewSpecFP, "", rec.story, reviewUserName, reviewUserEmail),
			"agreed_at = 2026-10-31T12:00:00+09:00", "agreed_at = "+rec.at, 1))
	}
	writeFile(t, r.Root, ".vanda/agreements.toml", readFileString(t, r.Root, ".vanda/agreements.toml")+b.String())
	r.g.commitTracked("日時の違う承認を足す", "#41")
	var got []string
	for _, a := range getHistory(t, s, "task-id-unique").Agreements {
		if strings.HasPrefix(a.Story, "#4") {
			got = append(got, a.Story)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("承認の履歴の並び: %v（思ったもの %v）", got, want)
	}
}

// @vanda:evidence mistakes-reported-with-fix
func Test仕様の詳細とファイルの中身はないものを404にする(t *testing.T) {
	t.Parallel()
	_, _, s := setupAgree(t)
	for _, path := range []string{
		"/api/specs/no-such-spec",
		"/api/specs/task-id", // 用語の ID
		"/api/file?path=" + q("README.md"),
		"/api/file?path=" + q(".vanda/config.toml"),
		"/api/file?path=" + q("../outside.txt"),
	} {
		if code, msg := s.get(path, nil); code != 404 || msg == "" {
			t.Errorf("%s に %d（%q）を返した。404 で、理由を出す", path, code, msg)
		}
	}
}

func Testファイルの中身は承認の対象のファイルとアノテーションを読むファイルを返す(t *testing.T) {
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

// CL-08: GET /api/specs/<ID>/at/<コミット> は、そのコミットの時点の仕様の中身を返す（docs/formats.md）。
// 期待する文は、newReviewRepo が main と story-24 に書いた約束リストから手で書いた。
// @vanda:evidence agreement-history-kept
func Testその時点の仕様の中身は変更履歴のコミットと承認の記録のコミットの時点の文を返す(t *testing.T) {
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
		if code, msg := s.get("/api/specs/task-id-unique/at/"+commit, &got); code != 200 {
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
		if code, msg := s.get("/api/specs/task-id-unique/at/"+commit[:7], &short); code != 200 || short.Content.Statement.String() != c.want {
			t.Errorf("短いハッシュで %s の時点を読めない（%d %q）: %+v", c.rev, code, msg, short)
		}
	}
	// 承認の記録の commit（記録を書き足したコミット）を渡すと、その時点の中身を返す。#1 の承認は main のコミットで書いた
	detail := getHistory(t, s, "task-id-unique")
	if len(detail.Agreements) == 0 || detail.Agreements[0].Commit == nil {
		t.Fatalf("承認の履歴の commit を読めない: %+v", detail)
	}
	if *detail.Agreements[0].Commit != r.git("rev-parse", "main") {
		t.Errorf("#1 の承認の commit が main のコミットでない: %s", *detail.Agreements[0].Commit)
	}
}

// CL-08: ハッシュでない形のコミットは 400、ないコミットとその時点にない仕様は 404。どれも理由を出す
// @vanda:evidence agreement-history-kept
func Testその時点の仕様の中身はハッシュでない形を400にしないコミットとその時点にない仕様を404にする(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	main := r.git("rev-parse", "main")
	cases := []struct {
		name, path string
		want       int
	}{
		{"ハッシュでない文字", "/api/specs/task-id-unique/at/main", 400},
		{"短すぎるハッシュ", "/api/specs/task-id-unique/at/abc", 400},
		{"16進でない文字", "/api/specs/task-id-unique/at/zzzzzzz", 400},
		{"65 文字のハッシュ", "/api/specs/task-id-unique/at/" + strings.Repeat("a", 65), 400},
		{"ないコミット", "/api/specs/task-id-unique/at/" + strings.Repeat("0", 40), 404},
		// task-title-required は story-24 で足したので、main の時点にはない
		{"その時点にない仕様", "/api/specs/task-title-required/at/" + main, 404},
	}
	for _, c := range cases {
		if code, msg := s.get(c.path, nil); code != c.want || msg == "" {
			t.Errorf("%s: %d（%q）を返した。%d で理由を出す", c.name, code, msg, c.want)
		}
	}
}

// CL-08: その時点のレイヤーのうち今の設定にないものは名前を空にし、その時点の約束リストが読めなければ 500 で理由を出す。
// どちらの時点も、コミットした後に今の約束リストを元に戻す（今の中身は読めるままにする）
// @vanda:evidence agreement-history-kept
func Testその時点の仕様の中身は今の設定にないレイヤーの名前を空にし読めない約束リストの時点を500にして理由を出す(t *testing.T) {
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
	broken := commitPromises(now+"\n[[spec]\n", "約束リストを壊す")
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
	if code, msg := s.get("/api/specs/task-id-unique/at/"+gone, &got); code != 200 {
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

	code, msg := s.get("/api/specs/task-id-unique/at/"+broken, nil)
	if code != 500 || msg == "" {
		t.Errorf("読めない約束リストの時点: %d（%q）を返した。500 で理由を出す", code, msg)
	}
}

// GET /api/file は、承認の対象のファイルとアノテーションを読むファイルの中身を返し、ほかのファイルは 404 にする
// @vanda:evidence server-operated-only-from-own-screen
func Testファイルの中身のAPIは承認の対象とアノテーションを読むファイルだけを返しほかを404にする(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	// 設定の txtar・tla・sql・go のパターンと、ガードレールの paths にマッチするもの（中身は newReviewRepo が書いたもの）
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
	writeFile(t, r.Root, "README.md", "どのパターンにもマッチしない\n")
	for _, path := range []string{".vanda/config.toml", ".vanda/promises.toml", "README.md", "e2e/none.txtar", "../e2e/add-task.txtar", "/etc/passwd"} {
		if code, msg := s.get("/api/file?path="+q(path), nil); code != 404 || msg == "" {
			t.Errorf("%s: %d（%q）を返した。404 で理由を出す", path, code, msg)
		}
	}
}

// @vanda:evidence changes-traceable-to-story
func Test仕様の変更履歴はコミットごとにそのコミットのStoryのトレーラーのストーリーを結ぶ(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	statement := "タスクの ID は、すべてのタスクを通して重ならない"
	change := func(to, msg string) string {
		t.Helper()
		text := readFileString(t, r.Root, ".vanda/promises.toml")
		writeFile(t, r.Root, ".vanda/promises.toml", strings.Replace(text, statement, to, 1))
		statement = to
		// 同じ大きさの書き換えを git が見落とさないよう、インデックスから外して読み直させる（commitAll と同じ）
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
		r.git("log", "-1", "--format=%H", "--fixed-strings", "--grep=タスクの ID の仕様を変え", reviewBranch): {"#24"},
		none:  nil,
		other: {"#33"},
	}
	got := getHistory(t, s, "task-id-unique")
	if len(got.Origin) != len(want) {
		t.Fatalf("変更履歴が %d 件（%d 件のはず）: %+v", len(got.Origin), len(want), got.Origin)
	}
	for _, o := range got.Origin {
		w, ok := want[o.Commit]
		if !ok {
			t.Errorf("思っていないコミットが変更履歴にある: %+v", o)
			continue
		}
		if len(o.Stories) != len(w) || (len(w) > 0 && !reflect.DeepEqual(o.Stories, w)) {
			t.Errorf("コミット %s（%s）のストーリー: %v（思ったもの %v）", o.Commit, o.Subject, o.Stories, w)
		}
	}
}

// claimLeft は、JSON の値の中の、名前に claim を含む項目と、値が claim の kind を、場所を付けて返す。
// 利用者が書いた文（ID、ファイルの中身など）は値なので、見ない
func claimLeft(where string, v any) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if strings.Contains(strings.ToLower(k), "claim") {
				out = append(out, where+"."+k)
			}
			if s, ok := child.(string); ok && k == "kind" && strings.EqualFold(s, "claim") {
				out = append(out, where+".kind = "+s)
			}
			out = append(out, claimLeft(where+"."+k, child)...)
		}
	case []any:
		for i, child := range x {
			out = append(out, claimLeft(fmt.Sprintf("%s[%d]", where, i), child)...)
		}
	}
	return out
}

// 型の名前 claim は spec に改名した（docs/formats.md の「GUI の API」）。前のパスと値は読み替えない
func TestAPIのパスとレスポンスの項目とkindの値にclaimが残っていない(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	// 性質と前提と、それを指す参照（refs に種類が出る）も一覧と詳細に出るようにする
	writeFile(t, r.Root, ".vanda/promises.toml", strings.Replace(reviewPromisesStory, `statement = "タスクの題は空にできない"`, `statement = "{@id:task-id-unique} と同じく、{@id:disk-works} のもとでタスクの題は空にできない"`, 1)+
		"\n[[truth]]\nid = \"disk-works\"\nstatement = \"ディスクに書いたものは、読み戻せる\"\n\n[[property]]\nid = \"reliable\"\nstatement = \"頼れる\"\n")
	r.g.commitAll("前提と性質を足す", reviewStory)
	main := r.git("rev-parse", "main")

	// 前のパスは、どれも 404
	for _, path := range []string{
		"/api/claims/task-id-unique",
		"/api/claims/task-id-unique/history",
		"/api/claims/task-id-unique/at/" + main,
		"/api/claims",
		"/api/claim/task-id-unique",
	} {
		if res := rawCall(s.URL, path, http.MethodGet, path, nil); res.err != nil || res.code != 404 {
			t.Errorf("前のパス %s が %d を返した（404 のはず。%v）", path, res.code, res.err)
		}
	}

	// 今のパスのレスポンスに、claim の付く項目も、claim の kind もない
	for _, path := range []string{
		"/api/context",
		"/api/review?view=branch&base=main",
		"/api/review?view=story&story=" + q(reviewStory),
		"/api/review?view=story&story=" + q("#1"),
		"/api/specs/task-id-unique",
		"/api/specs/task-title-required",
		"/api/specs/task-id-unique/history",
		"/api/specs/task-id-unique/at/" + main,
		"/api/terms/task-id",
		"/api/features/add-task",
		"/api/properties/no-lost-task",
		"/api/properties/no-lost-task/history",
		"/api/truths/disk-works",
		"/api/truths/disk-works/history",
		"/api/file?path=" + q("spec/Task.tla"),
	} {
		res := rawCall(s.URL, path, http.MethodGet, path, nil)
		if res.err != nil || res.code != 200 {
			t.Errorf("%s を読めない（%d、%v）: %s", path, res.code, res.err, res.body)
			continue
		}
		var v any
		if err := json.Unmarshal(res.body, &v); err != nil {
			t.Errorf("%s の中身が JSON でない: %v", path, err)
			continue
		}
		if left := claimLeft(path, v); len(left) > 0 {
			sort.Strings(left)
			t.Errorf("%s のレスポンスに claim が残っている:\n  %s", path, strings.Join(left, "\n  "))
		}
	}

	// 承認と取り消しは、kind の claim を受け付けない。何も書かない。同じリクエストは、kind を spec にすれば通る（断ったのが、ほかの理由でないこと）
	for _, c := range []struct {
		path string
		body func(kind string) map[string]any
	}{
		{"/api/agree", func(kind string) map[string]any {
			return map[string]any{"kind": kind, "id": "task-id-unique", "path": nil, "fingerprint": reviewSpecFP, "story": reviewStory}
		}},
		{"/api/revoke", func(kind string) map[string]any {
			return map[string]any{"kind": kind, "id": "task-id-unique", "path": nil, "fingerprint": reviewSpecFP, "story": reviewStory, "base": "main"}
		}},
	} {
		head := r.git("rev-parse", "HEAD")
		before := readFileString(t, r.Root, ".vanda/agreements.toml")
		if res := rawCall(s.URL, c.path, http.MethodPost, c.path, c.body("claim")); res.err != nil || res.code < 400 || res.code >= 500 {
			t.Errorf("kind が claim の %s が %d を返した（リクエストの誤りとして断るはず。%v）: %s", c.path, res.code, res.err, res.body)
		}
		if r.git("rev-parse", "HEAD") != head || readFileString(t, r.Root, ".vanda/agreements.toml") != before {
			t.Errorf("kind が claim の %s で、コミットか承認の記録が変わった", c.path)
		}
		if code, msg := s.post(c.path, c.body("spec"), nil); code != 200 {
			t.Errorf("kind が spec の %s ができない（%d）: %s", c.path, code, msg)
		}
	}
}
