package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// RV-01・RV-02・RV-13 の一覧を、GET /api/review（docs/formats.md）で確かめる。
// 期待する一覧は、rv01_model_test.go の数え方と、rv01_examples_test.go の手で作った例から作る。

type reviewEntry struct {
	No                *int            `json:"no"`
	Kind              string          `json:"kind"`
	ID                *string         `json:"id"`
	Path              *string         `json:"path"`
	FileType          *string         `json:"fileType"`
	Change            string          `json:"change"`
	State             *string         `json:"state"`
	Fingerprint       *string         `json:"fingerprint"`
	Flags             []string        `json:"flags"`
	Group             string          `json:"group"`
	Before            json.RawMessage `json:"before"`
	After             json.RawMessage `json:"after"`
	Diff              json.RawMessage `json:"diff"`
	DetachedAgreement *struct {
		By []string `json:"by"`
	} `json:"detachedAgreement"`
	OtherCommits []struct {
		Commit  string   `json:"commit"`
		Subject string   `json:"subject"`
		Stories []string `json:"stories"`
	} `json:"otherCommits"`
}

type reviewResponse struct {
	View      string        `json:"view"`
	Base      *string       `json:"base"`
	MergeBase *string       `json:"mergeBase"`
	Story     *string       `json:"story"`
	Total     int           `json:"total"`
	Entries   []reviewEntry `json:"entries"`
	Detached  []reviewEntry `json:"detached"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (e reviewEntry) key() string {
	if e.Kind == "file" {
		return str(e.Path)
	}
	return str(e.ID)
}

func (e reviewEntry) rv() rvEntry {
	var others []string
	for _, c := range e.OtherCommits {
		if len(c.Stories) == 0 {
			others = append(others, "-")
		} else {
			others = append(others, strings.Join(c.Stories, "+"))
		}
	}
	sort.Strings(others)
	return rvEntry{Kind: e.Kind, Key: e.key(), Change: e.Change, Others: strings.Join(others, ",")}
}

// rvFileType は、rvConfig のパターンから手で決めた、ファイルの種類（fileType）。
func rvFileType(path string) string {
	switch {
	case strings.HasPrefix(path, "e2e/"):
		return "scenario"
	case strings.HasPrefix(path, "spec/"):
		return "oracle"
	case strings.HasPrefix(path, "db/"):
		return "guardrail"
	}
	return "?"
}

func isNull(raw json.RawMessage) bool { return len(raw) == 0 || string(raw) == "null" }

// checkReview は、一覧の変わったものが want とちょうど一致し（漏れも重複もない）、通し番号と総数と、各項目の形が決まりどおりかを確かめる。
// rvRepo は承認の記録を書かないので、今（HEAD のコミットの中身 cur）ある承認の対象のうち want にないものは、どれも承認済でない。
// それらが、変わっていない提案中の行として 1 度ずつ出て、ほかに変わっていない行がないことも確かめる。
// 変わっていない、承認済でない行は entries に入る。rvRepo には承認がないので、detached は空（docs/formats.md）。
func checkReview(t fataler, where string, got reviewResponse, want map[rvEntry]bool, cur rvState) {
	var b strings.Builder
	unchanged := map[string]int{}
	noteUnchanged := func(e reviewEntry) {
		unchanged[rvRowKey(e.Kind, e.key())]++
		if str(e.State) != "proposed" || e.Fingerprint == nil {
			fmt.Fprintf(&b, "  変わっていない行が、フィンガープリントのある提案中の行でない: %s %s（state %q）\n", e.Kind, e.key(), str(e.State))
		}
	}
	if len(got.Detached) != 0 {
		fmt.Fprintf(&b, "  承認の記録がないのに detached が %d 件ある\n", len(got.Detached))
	}
	if got.Total != len(got.Entries) {
		fmt.Fprintf(&b, "  total が %d、entries が %d 件\n", got.Total, len(got.Entries))
	}
	set := map[rvEntry]bool{}
	for i, e := range got.Entries {
		if e.No == nil || *e.No != i+1 {
			fmt.Fprintf(&b, "  %d 件目の no が %v（1 から順に振る）\n", i+1, e.No)
		}
		r := e.rv()
		if e.Change == "unchanged" {
			noteUnchanged(e)
		} else {
			if set[r] {
				fmt.Fprintf(&b, "  2 度出た: %s\n", r)
			}
			set[r] = true
		}
		if got.View == "branch" && len(e.OtherCommits) != 0 {
			fmt.Fprintf(&b, "  レビュー単位がブランチなのに otherCommits がある: %s\n", r)
		}
		switch {
		case e.Kind == "file" && (e.ID != nil || e.Path == nil || str(e.FileType) != rvFileType(str(e.Path))):
			fmt.Fprintf(&b, "  ファイルの id・path・fileType が決まりどおりでない: %s（id %v、fileType %q）\n", r, e.ID, str(e.FileType))
		case e.Kind != "file" && (e.ID == nil || e.Path != nil || e.FileType != nil):
			fmt.Fprintf(&b, "  約束リストの項目の id・path・fileType が決まりどおりでない: %s\n", r)
		}
		// まとまり（group）は、種類から決まる（docs/formats.md）。rvConfig のレイヤーは model だけ
		if want := map[string]string{"spec": "layer:model", "term": "terms", "feature": "features", "property": "properties", "truth": "truths", "file": "files"}[e.Kind]; e.Group != want {
			fmt.Fprintf(&b, "  group が %q（%q のはず）: %s\n", e.Group, want, r)
		}
		if (e.Change == "added" || e.Change == "unchanged") != isNull(e.Before) || (e.Change == "deleted") != isNull(e.After) {
			fmt.Fprintf(&b, "  before は追加と変わっていないときだけ、after は削除のときだけ null: %s\n", r)
		}
		if e.Change == "unchanged" && (!isNull(e.Diff) || len(e.OtherCommits) != 0) {
			fmt.Fprintf(&b, "  変わっていない行に diff か otherCommits がある: %s\n", r)
		}
		if (e.Change == "deleted") != (e.Fingerprint == nil) {
			fmt.Fprintf(&b, "  fingerprint は削除のときだけ null: %s\n", r)
		}
		// 削除したものにも、消したことへの承認があるかで状態を付ける
		if s := str(e.State); s != "proposed" && s != "agreed" {
			fmt.Fprintf(&b, "  state が proposed・agreed のどちらでもない（%q）: %s\n", s, r)
		}
	}
	if d := rvDiffEntries(want, set); d != "" {
		b.WriteString(d)
	}
	changed := map[string]bool{}
	for e := range want {
		changed[rvRowKey(e.Kind, e.Key)] = true
	}
	wantUnchanged := map[string]bool{}
	for k, v := range rvKeyed(cur) {
		_, name, _ := strings.Cut(k, ":")
		if row := rvRowKey(v.kind, name); !changed[row] {
			wantUnchanged[row] = true
		}
	}
	rows := map[string]bool{}
	for row := range wantUnchanged {
		rows[row] = true
	}
	for row := range unchanged {
		rows[row] = true
	}
	for _, row := range sortedKeys(rows) {
		switch n := unchanged[row]; {
		case !wantUnchanged[row]:
			fmt.Fprintf(&b, "  余計な変わっていない行: %s\n", row)
		case n != 1:
			fmt.Fprintf(&b, "  変えていない承認済でない %s が、変わっていない行として %d 度出た（1 度だけ出る）\n", row, n)
		}
	}
	if b.Len() > 0 {
		t.Fatalf("%s の一覧が思ったものでない\n%s", where, b.String())
	}
}

// rvRowKey は、一覧の行を、項目かファイルかと、ID か場所で見分ける。
func rvRowKey(kind, key string) string {
	if kind == "file" {
		return "file " + key
	}
	return "item " + key
}

func sortedKeys(m map[string]bool) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// RV-01・RV-02・RV-13
// @vanda:evidence owner-reviews-every-change
// @vanda:evidence check-passes-only-when-agreed
func TestRV01の一覧はAPIで手で作った例と合う(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, ex := range rvExamples {
		t.Run(ex.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			r := newRVRepo(t, root, version, "#1", pickFirst)
			ex.build(r)
			s := startServe(t, bin, root, r.g.env)
			for base, want := range ex.branch {
				var got reviewResponse
				if code, msg := s.get("/api/review?view=branch&base="+q(base), &got); code != 200 {
					t.Fatalf("ベースブランチ %s の一覧を読めない（%d）: %s", base, code, msg)
				}
				if got.View != "branch" || str(got.Base) != base || str(got.MergeBase) != r.mergeBase(base).hash {
					t.Errorf("view %q、base %q、mergeBase %q（マージベースは %s）", got.View, str(got.Base), str(got.MergeBase), r.mergeBase(base).hash)
				}
				checkReview(t, "ベースブランチ "+base+" の、レビュー単位がブランチのとき", got, rvSet(want), r.head().state)
			}
			for story, want := range ex.story {
				var got reviewResponse
				if code, msg := s.get("/api/review?view=story&story="+q(story), &got); code != 200 {
					t.Fatalf("ストーリー %s の一覧を読めない（%d）: %s", story, code, msg)
				}
				if got.View != "story" || got.MergeBase != nil || str(got.Story) != story {
					t.Errorf("view %q、mergeBase %v、story %q", got.View, got.MergeBase, str(got.Story))
				}
				checkReview(t, "ストーリー "+story+" の、レビュー単位がストーリーのとき", got, rvSet(want), r.head().state)
			}
		})
	}
}

// RV-01 のランダムチェック。無作為な操作の列を当てたリポジトリで、一覧がテストの中で数えたものとちょうど一致する。
// @vanda:evidence owner-reviews-every-change
func TestRV01の一覧は無作為な操作の列を当ててもテストの中で数えたものとちょうど一致する(t *testing.T) {
	t.Parallel()
	runRV01Random(t, func(rt *rapid.T, bin string, r *rvRepo, branch map[rvEntry]bool, stories map[string]map[rvEntry]bool) {
		s := startServe(rt, bin, r.root, r.g.env)
		defer s.stop()
		var got reviewResponse
		if code, msg := s.get("/api/review?view=branch&base=main&story="+q("#24"), &got); code != 200 {
			rt.Fatalf("一覧を読めない（%d）: %s", code, msg)
		}
		checkReview(rt, "レビュー単位がブランチのとき", got, branch, r.head().state)
		for story, want := range stories {
			var got reviewResponse
			if code, msg := s.get("/api/review?view=story&story="+q(story), &got); code != 200 {
				rt.Fatalf("ストーリー %s の一覧を読めない（%d）: %s", story, code, msg)
			}
			checkReview(rt, "ストーリー "+story+" の、レビュー単位がストーリーのとき", got, want, r.head().state)
		}
	})
}

// RV-13・CM-06 の引数の誤り。
// @vanda:evidence mistakes-reported-with-fix
func TestRV01の一覧はないベースブランチや足りない引数や形の誤ったストーリーのIDを400で断る(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	s := startServe(t, bin, r.Root, r.Env)
	for _, path := range []string{
		"/api/review?view=branch&base=no-such-branch",
		"/api/review?view=branch",
		"/api/review?view=story",
		"/api/review?view=story&story=" + q("PROJ-1"),
		"/api/review?view=other&base=main",
	} {
		if code, msg := s.get(path, nil); code != 400 || msg == "" {
			t.Errorf("%s に %d（%q）を返した。400 で、理由を出す", path, code, msg)
		}
	}
}

// ---- ストーリーで変えていない、承認済でないもの ----
// リポジトリは review_repo_test.go の newReviewRepoWith。main に、承認の記録を書かない仕様・用語・シナリオを置き、#24 はどれも変えない。
// 一覧に出すのは、すでにあるリポジトリに vanda を入れた時、対象のファイルすべてが、チェックは失敗するのに画面から承認できない状態になるため。

type unagreedTarget struct {
	label, kind, key, fp string
}

var reviewUnagreed = []unagreedTarget{
	{"仕様", "spec", "task-list-order", sha256Hex([]byte(reviewUnagreedSpecJSON))},
	{"用語", "term", "task-title", sha256Hex([]byte(reviewUnagreedTermJSON))},
	{"ファイル", "file", reviewUnagreedPath, sha256Hex([]byte(reviewUnagreedScenario))},
}

var unagreedViews = []struct{ name, path string }{
	{"レビュー単位がブランチのとき", "/api/review?view=branch&base=main"},
	{"レビュー単位がストーリーのとき", "/api/review?view=story&story=" + q(reviewStory)},
}

type listRow struct {
	No          *int            `json:"no"`
	Before      json.RawMessage `json:"before"`
	After       json.RawMessage `json:"after"`
	Diff        json.RawMessage `json:"diff"`
	Kind        string          `json:"kind"`
	ID          *string         `json:"id"`
	Path        *string         `json:"path"`
	Change      string          `json:"change"`
	State       string          `json:"state"`
	Fingerprint *string         `json:"fingerprint"`
	Agree       *struct {
		Possible bool `json:"possible"`
	} `json:"agree"`
}

func (r listRow) key() string {
	if r.Kind == "file" {
		return str(r.Path)
	}
	return str(r.ID)
}

type listResponse struct {
	Total    int       `json:"total"`
	Entries  []listRow `json:"entries"`
	Detached []listRow `json:"detached"`
}

func getList(t *testing.T, s *vandaServer, path string) listResponse {
	t.Helper()
	var got listResponse
	if code, msg := s.get(path, &got); code != 200 {
		t.Fatalf("一覧 %s を読めない（%d）: %s", path, code, msg)
	}
	return got
}

// listRows は、一覧のレスポンスの行を、entries と detached を合わせてすべて返す（どこにも出ないことを確かめるのに使う）。
func listRows(t *testing.T, s *vandaServer, path string) []listRow {
	t.Helper()
	got := getList(t, s, path)
	return append(got.Entries, got.Detached...)
}

func rowsOf(rows []listRow, kind, key string) []listRow {
	var found []listRow
	for _, r := range rows {
		if r.Kind == kind && r.key() == key {
			found = append(found, r)
		}
	}
	return found
}

// setupUnagreed は、リポジトリと vanda サーバーと、承認のチェック（範囲がすべて）だけのチェックの設定の場所を返す。
func setupUnagreed(t *testing.T) (string, *reviewRepo, *vandaServer, string) {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepoWith(t, version, reviewUnagreedSpec+reviewUnagreedTerm, map[string]string{reviewUnagreedPath: reviewUnagreedScenario})
	checks := t.TempDir()
	writeFile(t, checks, "all.toml", "[[check]]\nname = \"agreement\"\nscope = \"all\"\n")
	return bin, r, startServe(t, bin, r.Root, r.Env), filepath.Join(checks, "all.toml")
}

// notAgreedLines は、承認のチェックの誤りのうち、今の中身にまだ承認していないと言う行を返す。
func notAgreedLines(errs string) []string {
	var lines []string
	for _, line := range strings.Split(errs, "\n") {
		if strings.Contains(line, "まだ承認していません") && !strings.Contains(line, "消したこと") {
			lines = append(lines, line)
		}
	}
	return lines
}

// names は、誤りの行が、その対象を名指ししているか。項目は「<種類> <ID> の」、ファイルは行の頭の「<場所>: 」（.claude/rules/wording.md の誤りの形）。
func names(line, key string) bool {
	return strings.Contains(line, " "+key+" ") || strings.HasPrefix(line, key+": ")
}

// @vanda:evidence owner-reviews-every-change
func Testストーリーで変えていない承認の記録のない仕様と用語とファイルは一覧に変わっていない提案中の行として1度だけ出て承認できる(t *testing.T) {
	t.Parallel()
	_, _, s, _ := setupUnagreed(t)
	for _, v := range unagreedViews {
		got := getList(t, s, v.path)
		// #24 が変えた 3 つと、変えていない承認済でない 3 つ。変わっていない行も、総数と通し番号に数える
		if got.Total != 6 || len(got.Entries) != 6 {
			t.Errorf("%s: total %d、entries %d 件（どちらも 6 のはず）", v.name, got.Total, len(got.Entries))
		}
		for i, e := range got.Entries {
			if e.No == nil || *e.No != i+1 {
				t.Errorf("%s: %d 件目の %s の no が %v（1 から順に振る）", v.name, i+1, e.key(), e.No)
			}
		}
		if len(got.Detached) != 0 {
			t.Errorf("%s: 失効した承認がないのに detached がある: %+v", v.name, got.Detached)
		}
		for _, u := range reviewUnagreed {
			found := rowsOf(got.Entries, u.kind, u.key)
			if len(found) != 1 {
				t.Errorf("%s: %s %s が entries に %d 度出た（1 度だけ出る）", v.name, u.label, u.key, len(found))
				continue
			}
			row := found[0]
			if !isNull(row.Before) || !isNull(row.Diff) || isNull(row.After) {
				t.Errorf("%s: %s %s の before と diff が null、after が今の中身でない（before %s、diff %s、after %s）", v.name, u.label, u.key, row.Before, row.Diff, row.After)
			}
			if row.Change != "unchanged" || row.State != "proposed" || str(row.Fingerprint) != u.fp {
				t.Errorf("%s: %s %s の change %q、state %q、fingerprint %q（思ったもの unchanged・proposed・%q）", v.name, u.label, u.key, row.Change, row.State, str(row.Fingerprint), u.fp)
			}
			if row.Agree == nil || !row.Agree.Possible {
				t.Errorf("%s: %s %s を承認できると出ていない: %+v", v.name, u.label, u.key, row.Agree)
			}
		}
	}
}

// @vanda:evidence owner-reviews-every-change
func Test変えていない承認済のものと承認の対象でないGoのテストは一覧に出ずストーリーで変えたものは変わっていない行と重ならず1度だけ出る(t *testing.T) {
	t.Parallel()
	_, _, s, _ := setupUnagreed(t)
	agreed := []struct{ kind, key string }{
		{"term", "task"}, {"term", "task-id"}, {"property", "no-lost-task"}, {"feature", "add-task"},
		{"file", "spec/Task.tla"}, {"file", "db/schema.sql"},
	}
	changed := []struct{ kind, key, change string }{
		{"spec", "task-id-unique", "modified"}, {"spec", "task-title-required", "added"}, {"file", "e2e/add-task.txtar", "modified"},
	}
	for _, v := range unagreedViews {
		rows := listRows(t, s, v.path)
		for _, a := range agreed {
			if found := rowsOf(rows, a.kind, a.key); len(found) != 0 {
				t.Errorf("%s: 変えていない承認済の %s %s が一覧に出た: %+v", v.name, a.kind, a.key, found)
			}
		}
		if found := rowsOf(rows, "file", "internal/task/task_test.go"); len(found) != 0 {
			t.Errorf("%s: 承認の対象でない Go のテストが一覧に出た: %+v", v.name, found)
		}
		for _, c := range changed {
			found := rowsOf(rows, c.kind, c.key)
			if len(found) != 1 || found[0].Change != c.change {
				t.Errorf("%s: ストーリーで変えた %s %s が、%s の行 1 つでない: %+v", v.name, c.kind, c.key, c.change, found)
			}
		}
	}
}

// @vanda:evidence owner-reviews-every-change
func Test一覧に出た変えていない承認の記録のないものを承認すると一覧から消え範囲がすべての承認のチェックがその対象で失敗しなくなる(t *testing.T) {
	t.Parallel()
	bin, r, s, checks := setupUnagreed(t)
	for _, u := range reviewUnagreed {
		_, _, errs := r.vanda(bin, "check", checks)
		if !slices.ContainsFunc(notAgreedLines(errs), func(line string) bool { return names(line, u.key) }) {
			t.Fatalf("承認する前に、承認のチェックが %s %s をまだ承認していないと言わない\n%s", u.label, u.key, errs)
		}
		// 承認するのは、一覧に見せていた中身（その行のフィンガープリント）
		found := rowsOf(getList(t, s, unagreedViews[0].path).Entries, u.kind, u.key)
		if len(found) != 1 {
			t.Errorf("%s %s が entries に %d 度出た（1 度だけ出る）", u.label, u.key, len(found))
			continue
		}
		path := ""
		if u.kind == "file" {
			path = u.key
		}
		if code, msg := s.post("/api/agree", agreeBody(u.kind, u.key, path, str(found[0].Fingerprint)), nil); code != 200 {
			t.Errorf("%s %s を承認できない（%d）: %s", u.label, u.key, code, msg)
			continue
		}
		if got, want := r.git("log", "-1", "--format=%s"), "(chore) #24 で "+u.label+" "+u.key+" を承認する"; got != want {
			t.Errorf("承認のコミットの subject: %q（思ったもの %q）", got, want)
		}
		// 承認済で、変わっていないものは、一覧に出さない（docs/formats.md）
		for _, v := range unagreedViews {
			if after := rowsOf(listRows(t, s, v.path), u.kind, u.key); len(after) != 0 {
				t.Errorf("%s: 承認した %s %s が、一覧に残っている: %+v", v.name, u.label, u.key, after)
			}
		}
		_, _, errs = r.vanda(bin, "check", checks)
		for _, line := range notAgreedLines(errs) {
			if names(line, u.key) {
				t.Errorf("承認した後も、承認のチェックが %s %s で失敗する: %s", u.label, u.key, line)
			}
		}
	}
}

// @vanda:evidence owner-reviews-every-change
// @vanda:evidence check-passes-only-when-agreed
func Test範囲がすべての承認のチェックがまだ承認していないと言う対象はすべて一覧に出る(t *testing.T) {
	t.Parallel()
	bin, r, s, checks := setupUnagreed(t)
	code, _, errs := r.vanda(bin, "check", checks)
	lines := notAgreedLines(errs)
	// 用意したリポジトリで承認済でないのは、#24 が変えた 3 つと、変えていない 3 つ
	want := []string{"task-id-unique", "task-title-required", "e2e/add-task.txtar", "task-list-order", "task-title", reviewUnagreedPath}
	for _, key := range want {
		if !slices.ContainsFunc(lines, func(line string) bool { return names(line, key) }) {
			t.Fatalf("承認のチェック（%d）が %s をまだ承認していないと言わない\n%s", code, key, errs)
		}
	}
	for _, v := range unagreedViews {
		rows := listRows(t, s, v.path)
		for _, line := range lines {
			if !slices.ContainsFunc(rows, func(row listRow) bool { return names(line, row.key()) }) {
				t.Errorf("%s: 承認のチェックがまだ承認していないと言う対象が、一覧にない: %s", v.name, line)
			}
		}
	}
}

// ---- ストーリーで変えていない、承認の記録はあるのに承認のチェック（範囲がすべて）が失敗と言うもの ----
// 前のストーリーが承認し、別のストーリーが消して、消したことを承認しないままベースブランチに取り込んだ仕様とシナリオと、
// 承認済だが、フラグ（flags）か推測（guesses）が残っている仕様。

const (
	reviewRemovedSpecID = "task-list-order"
	reviewFlaggedSpecID = "task-title-short"
	reviewFlaggedSpec   = `
[[spec]]
id = "task-title-short"
refines = ["add-task"]
statement = "タスクの題は 1 行に収まる"
layers = ["model"]
flags = ["ai-proposal"]

[[spec.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
`
	reviewFlaggedSpecJSON = `{"evidence":[{"layers":["model"],"stage":"implementation","type":"unit-test"}],` +
		`"id":"task-title-short","layers":["model"],"refines":["add-task"],"statement":"タスクの題は 1 行に収まる"}`
	reviewGuessedSpecID = "task-title-trimmed"
	reviewGuessedSpec   = `
[[spec]]
id = "task-title-trimmed"
refines = ["add-task"]
statement = "タスクの題の前後の空白は除く"
layers = ["model"]
guesses = ["全角の空白も除く"]

[[spec.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
`
	reviewGuessedSpecJSON = `{"evidence":[{"layers":["model"],"stage":"implementation","type":"unit-test"}],` +
		`"id":"task-title-trimmed","layers":["model"],"refines":["add-task"],"statement":"タスクの題の前後の空白は除く"}`
)

// setupCheckFailsUnchanged は、setupUnagreed と同じ並びのものを返す。リポジトリの main には、#24 が分かれた後に次の 2 つのコミットを足し、story-24 をその上に載せ直す。
// #2: フラグの残る仕様と推測の残る仕様を承認し、仕様 task-list-order とシナリオ e2e/list-task.txtar を足して承認する。
// #3: その仕様とシナリオを消す（消したことは承認しない）。
// #24 は、どれも変えない。
func setupCheckFailsUnchanged(t *testing.T) (string, *reviewRepo, *vandaServer, string) {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepoWith(t, version, reviewFlaggedSpec+reviewGuessedSpec, nil)
	r.git("switch", "-q", "main")
	promises, err := os.ReadFile(filepath.Join(r.Root, ".vanda/promises.toml"))
	if err != nil {
		t.Fatal(err)
	}
	agreements, err := os.ReadFile(filepath.Join(r.Root, ".vanda/agreements.toml"))
	if err != nil {
		t.Fatal(err)
	}
	fileFP := sha256Hex([]byte(reviewUnagreedScenario))
	writeFile(t, r.Root, ".vanda/promises.toml", string(promises)+reviewUnagreedSpec)
	writeFile(t, r.Root, reviewUnagreedPath, reviewUnagreedScenario)
	writeFile(t, r.Root, ".vanda/agreements.toml", string(agreements)+
		agreementTOML("spec", reviewFlaggedSpecID, sha256Hex([]byte(reviewFlaggedSpecJSON)), "", "#2", "テスト担当", "tester@example.com")+
		agreementTOML("spec", reviewGuessedSpecID, sha256Hex([]byte(reviewGuessedSpecJSON)), "", "#2", "テスト担当", "tester@example.com")+
		agreementTOML("spec", reviewRemovedSpecID, sha256Hex([]byte(reviewUnagreedSpecJSON)), "", "#2", "テスト担当", "tester@example.com")+
		agreementTOML("file", fileFP, fileFP, reviewUnagreedPath, "#2", "テスト担当", "tester@example.com"))
	r.g.commitAll("並びの仕様とシナリオを足して承認する", "#2")
	// 消す前の main では、フラグと推測の残る 2 つの仕様のほかは、承認のチェックが何も言わない（足した承認の記録が、中身に合っている）
	checks := t.TempDir()
	writeFile(t, checks, "all.toml", "[[check]]\nname = \"agreement\"\nscope = \"all\"\n")
	_, _, errs := r.vanda(bin, "check", filepath.Join(checks, "all.toml"))
	for _, line := range strings.Split(strings.TrimSpace(errs), "\n") {
		if !strings.Contains(line, reviewFlaggedSpecID) && !strings.Contains(line, reviewGuessedSpecID) {
			t.Fatalf("用意の誤り: 消す前の main で、承認のチェックが、フラグと推測の残る仕様のほかで失敗する\n%s", errs)
		}
	}
	writeFile(t, r.Root, ".vanda/promises.toml", string(promises))
	if err := os.Remove(filepath.Join(r.Root, filepath.FromSlash(reviewUnagreedPath))); err != nil {
		t.Fatal(err)
	}
	r.g.commitAll("並びの仕様とシナリオを消す", "#3")
	if strings.Contains(r.git("show", "HEAD:.vanda/promises.toml"), reviewRemovedSpecID) || !strings.Contains(r.git("show", "HEAD~1:.vanda/promises.toml"), reviewRemovedSpecID) {
		t.Fatalf("用意の誤り: #2 のコミットに仕様 %s がないか、#3 のコミットに残っている", reviewRemovedSpecID)
	}
	if strings.Contains(r.git("ls-tree", "-r", "--name-only", "HEAD"), reviewUnagreedPath) || !strings.Contains(r.git("ls-tree", "-r", "--name-only", "HEAD~1"), reviewUnagreedPath) {
		t.Fatalf("用意の誤り: #2 のコミットに %s がないか、#3 のコミットに残っている", reviewUnagreedPath)
	}
	r.git("switch", "-q", reviewBranch)
	r.git("rebase", "-q", "main")
	// #24 が変えたのは、もとの 3 つだけ（消したものと、フラグ・推測の残る仕様は変えていない）
	if got := strings.Fields(r.git("diff", "--name-only", "main", "HEAD")); !slices.Equal(got, []string{".vanda/promises.toml", "e2e/add-task.txtar"}) {
		t.Fatalf("用意の誤り: main と story-24 で違うファイルが %v", got)
	}
	diff := r.git("diff", "-U0", "main", "HEAD", "--", ".vanda/promises.toml")
	for _, id := range []string{reviewFlaggedSpecID, reviewGuessedSpecID, reviewRemovedSpecID} {
		if strings.Contains(diff, id) {
			t.Fatalf("用意の誤り: #24 が仕様 %s を変えている\n%s", id, diff)
		}
	}
	return bin, r, startServe(t, bin, r.Root, r.Env), filepath.Join(checks, "all.toml")
}

// checkFailsAndListed は、承認のチェック（範囲がすべて）の誤りに key を名指しする行があることを確かめてから、
// その対象が、どちらのレビュー単位でも、一覧（entries か detached）に出ることを確かめる。
func checkFailsAndListed(t *testing.T, bin string, r *reviewRepo, s *vandaServer, checks, label, kind, key string) {
	t.Helper()
	code, _, errs := r.vanda(bin, "check", checks)
	var lines []string
	for _, line := range strings.Split(errs, "\n") {
		if strings.Contains(line, key) {
			lines = append(lines, line)
		}
	}
	if code == 0 || len(lines) == 0 {
		t.Fatalf("承認のチェック（%d）が %s %s で失敗しない\n%s", code, label, key, errs)
	}
	for _, v := range unagreedViews {
		rows := listRows(t, s, v.path)
		if len(rowsOf(rows, kind, key)) == 0 {
			var listed []string
			for _, row := range rows {
				listed = append(listed, row.Kind+" "+row.key()+"（"+row.Change+"・"+row.State+"）")
			}
			t.Errorf("%s: 承認のチェックが失敗と言う %s %s が、一覧にない\nチェックの誤り: %s\n一覧: %s", v.name, label, key, strings.Join(lines, "\n"), strings.Join(listed, "、"))
		}
	}
}

// @vanda:evidence owner-reviews-every-change
func Testほかのストーリーが消して消したことへの承認がないままベースブランチに取り込まれた仕様とファイルは承認のチェックが失敗と言い一覧に出る(t *testing.T) {
	t.Parallel()
	bin, r, s, checks := setupCheckFailsUnchanged(t)
	t.Run("仕様", func(t *testing.T) {
		checkFailsAndListed(t, bin, r, s, checks, "仕様", "spec", reviewRemovedSpecID)
	})
	t.Run("ファイル", func(t *testing.T) {
		checkFailsAndListed(t, bin, r, s, checks, "ファイル", "file", reviewUnagreedPath)
	})
}

// @vanda:evidence owner-reviews-every-change
func Testストーリーで変えていない承認済の仕様にフラグか推測が残っていれば承認のチェックが失敗と言い一覧に出る(t *testing.T) {
	t.Parallel()
	bin, r, s, checks := setupCheckFailsUnchanged(t)
	t.Run("フラグ", func(t *testing.T) {
		checkFailsAndListed(t, bin, r, s, checks, "仕様", "spec", reviewFlaggedSpecID)
	})
	t.Run("推測", func(t *testing.T) {
		checkFailsAndListed(t, bin, r, s, checks, "仕様", "spec", reviewGuessedSpecID)
	})
}
