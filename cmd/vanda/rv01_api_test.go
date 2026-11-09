package main_test

import (
	"encoding/json"
	"fmt"
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

// checkReview は、一覧が want とちょうど一致し（漏れも重複もない）、通し番号と総数と、各項目の形が決まりどおりかを確かめる。
func checkReview(t fataler, where string, got reviewResponse, want map[rvEntry]bool) {
	var b strings.Builder
	if got.Total != len(got.Entries) {
		fmt.Fprintf(&b, "  total が %d、entries が %d 件\n", got.Total, len(got.Entries))
	}
	set := map[rvEntry]bool{}
	for i, e := range got.Entries {
		if e.No == nil || *e.No != i+1 {
			fmt.Fprintf(&b, "  %d 件目の no が %v（1 から順に振る）\n", i+1, e.No)
		}
		r := e.rv()
		if set[r] {
			fmt.Fprintf(&b, "  2 度出た: %s\n", r)
		}
		set[r] = true
		if got.View == "branch" && len(e.OtherCommits) != 0 {
			fmt.Fprintf(&b, "  レビュー単位がブランチなのに otherCommits がある: %s\n", r)
		}
		switch {
		case e.Kind == "file" && (e.ID != nil || e.Path == nil || str(e.FileType) != rvFileType(str(e.Path))):
			fmt.Fprintf(&b, "  ファイルの id・path・fileType が決まりどおりでない: %s（id %v、fileType %q）\n", r, e.ID, str(e.FileType))
		case e.Kind != "file" && (e.ID == nil || e.Path != nil || e.FileType != nil):
			fmt.Fprintf(&b, "  約束・用語・機能の id・path・fileType が決まりどおりでない: %s\n", r)
		}
		if (e.Change == "added") != isNull(e.Before) || (e.Change == "deleted") != isNull(e.After) {
			fmt.Fprintf(&b, "  before は追加のときだけ、after は削除のときだけ null: %s\n", r)
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
	if b.Len() > 0 {
		t.Fatalf("%s の一覧が思ったものでない\n%s", where, b.String())
	}
}

// RV-01・RV-02・RV-13
// @vanda:evidence review-no-omission-no-duplicate
// @vanda:evidence review-renamed-file
// @vanda:evidence review-numbering
// @vanda:evidence review-view-branch
// @vanda:evidence review-view-story
// @vanda:evidence review-other-commits
// @vanda:evidence branch-point-merge-base
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
				checkReview(t, "ベースブランチ "+base+" の、レビュー単位がブランチのとき", got, rvSet(want))
			}
			for story, want := range ex.story {
				var got reviewResponse
				if code, msg := s.get("/api/review?view=story&story="+q(story), &got); code != 200 {
					t.Fatalf("ストーリー %s の一覧を読めない（%d）: %s", story, code, msg)
				}
				if got.View != "story" || got.MergeBase != nil || str(got.Story) != story {
					t.Errorf("view %q、mergeBase %v、story %q", got.View, got.MergeBase, str(got.Story))
				}
				checkReview(t, "ストーリー "+story+" の、レビュー単位がストーリーのとき", got, rvSet(want))
			}
		})
	}
}

// RV-01 のランダムチェック。無作為な操作の列を当てたリポジトリで、一覧がテストの中で数えたものとちょうど一致する。
// @vanda:evidence review-no-omission-no-duplicate
func TestRV01の一覧は無作為な操作の列を当ててもテストの中で数えたものとちょうど一致する(t *testing.T) {
	t.Parallel()
	runRV01Random(t, func(rt *rapid.T, bin string, r *rvRepo, branch map[rvEntry]bool, stories map[string]map[rvEntry]bool) {
		s := startServe(rt, bin, r.root, r.g.env)
		defer s.stop()
		var got reviewResponse
		if code, msg := s.get("/api/review?view=branch&base=main&story="+q("#24"), &got); code != 200 {
			rt.Fatalf("一覧を読めない（%d）: %s", code, msg)
		}
		checkReview(rt, "レビュー単位がブランチのとき", got, branch)
		for story, want := range stories {
			var got reviewResponse
			if code, msg := s.get("/api/review?view=story&story="+q(story), &got); code != 200 {
				rt.Fatalf("ストーリー %s の一覧を読めない（%d）: %s", story, code, msg)
			}
			checkReview(rt, "ストーリー "+story+" の、レビュー単位がストーリーのとき", got, want)
		}
	})
}

// RV-13・CM-06 の引数の誤り。
// @vanda:evidence api-error-shape
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
