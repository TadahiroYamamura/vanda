package main_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// コミットのストーリーの ID を、画面（API）がどう扱うか（docs/formats.md の「コミットのストーリーの ID」と GET /api/context）。
// リポジトリは review_repo_test.go の newReviewRepo（HEAD は story-24）。

type trailerError struct {
	Commit  string `json:"commit"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

type trailerContext struct {
	TrailerErrors []trailerError `json:"trailerErrors"`
	Stories       []struct {
		ID string `json:"id"`
	} `json:"stories"`
}

func storyIDs(c trailerContext) []string {
	var out []string
	for _, s := range c.Stories {
		out = append(out, s.ID)
	}
	return out
}

// changeClaim は、task-id-unique の文を変えたコミットを、メッセージ msg のままで作る。
func changeClaim(t *testing.T, r *reviewRepo, statement, msg string) string {
	t.Helper()
	text := readFileString(t, r.Root, ".vanda/promises.toml")
	const now = "タスクの ID は、すべてのタスクを通して重ならない"
	cur := now
	if !strings.Contains(text, now) {
		// 2 度目からは、前に変えた文を置き換える
		i := strings.Index(text, "statement = \"タスクの ID は")
		if i < 0 {
			t.Fatalf("約束リストに task-id-unique の文がない")
		}
		j := strings.Index(text[i:], "\n")
		cur = strings.TrimSuffix(strings.TrimPrefix(text[i:i+j], "statement = \""), "\"")
	}
	writeFile(t, r.Root, ".vanda/promises.toml", strings.Replace(text, cur, statement, 1))
	r.git("rm", "-r", "-q", "--cached", "--ignore-unmatch", ".")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

// @vanda:evidence story-id-from-trailer
func Test画面のストーリーはStoryのトレーラーからだけ読みsubjectと本文とトレーラーでない段落のIDは数えない(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)
	r.git("commit", "-q", "--allow-empty", "-m", "#81 を subject に書く")
	r.git("commit", "-q", "--allow-empty", "-m", "本文に書く\n\n#82 の続き。")
	r.git("commit", "-q", "--allow-empty", "-m", "最後の段落でない\n\nStory: #83\n\n本文の最後の段落。")
	r.git("commit", "-q", "--allow-empty", "-m", "キーが小文字\n\nstory: #84\nCo-Authored-By: A <a@example.com>")
	var got trailerContext
	if code, msg := s.get("/api/context", &got); code != 200 {
		t.Fatalf("context を読めない（%d）: %s", code, msg)
	}
	ids := strings.Join(storyIDs(got), " ")
	for _, id := range []string{"#81", "#82", "#83"} {
		if strings.Contains(ids, id) {
			t.Errorf("トレーラーでない所の %s を数えた: %s", id, ids)
		}
	}
	if !strings.Contains(ids, "#84") {
		t.Errorf("小文字のキーのトレーラーの #84 を数えない: %s", ids)
	}
	var review struct {
		BranchStories []string `json:"branchStories"`
	}
	if code, msg := s.get("/api/review?view=branch&base=main", &review); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	if want := "#24 #84"; strings.Join(review.BranchStories, " ") != want {
		t.Errorf("branchStories: %v（思ったもの %s）", review.BranchStories, want)
	}
}

// @vanda:evidence story-trailer-errors-shown
func TestStoryのトレーラーが誤ったコミットがあってもAPIは200で返しそのストーリーを空にして誤りを新しい順に返す(t *testing.T) {
	t.Parallel()
	_, r, s := setupAgree(t)

	// 誤りがないときは []
	var raw json.RawMessage
	if code, msg := s.get("/api/context", &raw); code != 200 {
		t.Fatalf("context を読めない（%d）: %s", code, msg)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if v, ok := fields["trailerErrors"]; !ok || string(v) != "[]" {
		t.Errorf("誤りがないときの trailerErrors が [] でない: %s", v)
	}

	two := changeClaim(t, r, "タスクの ID は、どのタスクとも重ならない", "トレーラーが 2 つ\n\nStory: #77\nStory: #78")
	bad := changeClaim(t, r, "タスクの ID は、決して重ならない", "トレーラーが書き方に合わない\n\nStory: PROJ-1")

	var got trailerContext
	if code, msg := s.get("/api/context", &got); code != 200 {
		t.Fatalf("トレーラーの誤りがあると context が %d: %s", code, msg)
	}
	want := []struct {
		commit, subject string
		frags           []string
	}{
		{bad, "トレーラーが書き方に合わない", []string{"PROJ-1"}},
		{two, "トレーラーが 2 つ", []string{"#77", "#78"}},
	}
	if len(got.TrailerErrors) != len(want) {
		t.Fatalf("trailerErrors が %d 件: %+v（2 件のはず）", len(got.TrailerErrors), got.TrailerErrors)
	}
	for i, w := range want {
		e := got.TrailerErrors[i]
		if len(e.Commit) < 7 || !strings.HasPrefix(w.commit, e.Commit) || e.Subject != w.subject {
			t.Errorf("%d 件目（新しい順）: %+v（コミット %s・subject %q のはず）", i+1, e, w.commit, w.subject)
		}
		for _, f := range w.frags {
			if !strings.Contains(e.Message, f) {
				t.Errorf("%d 件目の理由に %q がない: %q", i+1, f, e.Message)
			}
		}
	}
	for _, id := range storyIDs(got) {
		if id == "#77" || id == "#78" || id == "PROJ-1" {
			t.Errorf("誤ったトレーラーのストーリー %s が stories に出た", id)
		}
	}

	// ほかの API も 200 で返し、誤ったコミットのストーリーを空にする
	for _, path := range []string{"/api/review?view=branch&base=main", "/api/review?view=story&story=%2324", "/api/review?view=story&story=%231", "/api/claims/task-id-unique", "/api/claims/task-id-unique/history"} {
		var body json.RawMessage
		if code, msg := s.get(path, &body); code != 200 {
			t.Errorf("%s が %d: %s", path, code, msg)
			continue
		}
		for _, id := range []string{"#77", "#78", "PROJ-1"} {
			if strings.Contains(string(body), id) {
				t.Errorf("%s に、誤ったトレーラーのストーリー %s が出た", path, id)
			}
		}
	}
	var branch struct {
		BranchStories []string `json:"branchStories"`
	}
	if s.get("/api/review?view=branch&base=main", &branch); strings.Join(branch.BranchStories, " ") != "#24" {
		t.Errorf("branchStories: %v（#24 だけのはず）", branch.BranchStories)
	}
	var story reviewResponse
	s.get("/api/review?view=story&story=%2324", &story)
	if e := findEntry(story.Entries, "task-id-unique"); e == nil {
		t.Errorf("#24 の一覧に task-id-unique がない")
	} else {
		seen := map[string]bool{}
		for _, c := range e.OtherCommits {
			if strings.HasPrefix(two, c.Commit) || strings.HasPrefix(bad, c.Commit) {
				seen[c.Commit] = true
				if len(c.Stories) != 0 {
					t.Errorf("otherCommits の誤ったコミット %s のストーリーが空でない: %v", c.Commit, c.Stories)
				}
			}
		}
		if len(seen) != 2 {
			t.Errorf("otherCommits に、誤ったトレーラーの 2 つのコミットがない: %+v", e.OtherCommits)
		}
	}
	// #1 が main で承認した task-id-unique は、#24 と誤ったトレーラーの 2 つのコミットが変えた。失効した承認は 200 で出て、
	// by に #24 は出る（誤ったコミットのストーリーが出ないことは、上でレスポンスの全体から見た）
	var one reviewResponse
	s.get("/api/review?view=story&story=%231", &one)
	var found bool
	for _, e := range append(append([]reviewEntry{}, one.Entries...), one.Detached...) {
		if e.key() != "task-id-unique" {
			continue
		}
		found = true
		if e.DetachedAgreement == nil || !strings.Contains(strings.Join(e.DetachedAgreement.By, " "), "#24") {
			t.Errorf("#1 の task-id-unique の detachedAgreement の by に #24 がない: %+v", e.DetachedAgreement)
		}
	}
	if !found {
		t.Errorf("#1 の一覧に、承認が失効した task-id-unique がない")
	}
	detail := getHistory(t, s, "task-id-unique")
	n := 0
	for _, o := range detail.Origin {
		if o.Commit == two || o.Commit == bad {
			n++
			if len(o.Stories) != 0 {
				t.Errorf("origin の誤ったコミット %s のストーリーが空でない: %v", o.Commit, o.Stories)
			}
		}
	}
	if n != 2 {
		t.Errorf("origin に、誤ったトレーラーの 2 つのコミットがない: %+v", detail.Origin)
	}
}
