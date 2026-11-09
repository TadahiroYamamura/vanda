package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// RV-14: ストーリーが前に承認したのに、今の中身にそのストーリーの承認がなくなったものを示す（GET /api/review）。
// 一覧に出るもの（比べる元と今で変わったもの）は entries[].detachedAgreement に、変わっていないものは detached に出る。

const rv14Term = "[[term]]\nid = \"t\"\nname = \"n\"\ndefinition = \"d\"\n"

func rv14Promises(a, b int) string {
	claim := func(id string, v int) string {
		return fmt.Sprintf("\n[[claim]]\nid = %q\nsubjects = [\"t\"]\nstatement = \"v%d\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n", id, v)
	}
	return rv14Term + claim("item-a", a) + claim("item-b", b)
}

// rv14FP は、docs/formats.md の決まった形の JSON を手で書き、そのフィンガープリントを返す。
func rv14FP(id string, v int) string {
	return sha256Hex([]byte(fmt.Sprintf(`{"evidence":[{"layers":["model"],"stage":"implementation","type":"unit-test"}],"id":%q,"layers":["model"],"statement":"v%d","subjects":["t"]}`, id, v)))
}

type rv14Repo struct {
	t    *testing.T
	root string
	g    *rvGit
	env  []string
}

// rv14Time は、2026-01-01 10:00（+09:00）から d だけ後の日時。
func rv14Time(d time.Duration) string {
	return time.Date(2026, 1, 1, 10, 0, 0, 0, time.FixedZone("", 9*60*60)).Add(d).Format(time.RFC3339)
}

// agree は、承認の記録を 1 件書き足す。承認した日時は、minute 分のコミットの 30 秒前にする。
// 失効した承認の by は、承認の記録の agreed_at とコミットの日時を比べて決める（docs/formats.md）ので、日時を本物らしく並べる。
func (r *rv14Repo) agree(id string, v int, story string, minute int) {
	rec := agreementTOML("claim", id, rv14FP(id, v), "", story, "テスト担当", "tester@example.com")
	rec = strings.Replace(rec, "agreed_at = 2026-10-31T12:00:00+09:00", "agreed_at = "+rv14Time(time.Duration(minute)*time.Minute-30*time.Second), 1)
	f, err := os.OpenFile(filepath.Join(r.root, ".vanda", "agreements.toml"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		r.t.Fatal(err)
	}
	_, err = f.WriteString(rec)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		r.t.Fatal(err)
	}
}

// commit は、minute 分の日時（作者とコミットした人の両方）でコミットする。
func (r *rv14Repo) commit(subject, story string, minute int) {
	at := rv14Time(time.Duration(minute) * time.Minute)
	g := &rvGit{t: r.t, root: r.root, env: append(append([]string{}, r.g.env...), "GIT_AUTHOR_DATE="+at, "GIT_COMMITTER_DATE="+at)}
	g.commitAll(subject, story)
}

// newRV14Repo は、次の履歴を作る（分は、コミットの日時）。
//
//	main:     0 分 #9 はじめ（item-a・item-b は v0。#9 が承認）
//	          1 分 #24 が item-b の v0 を承認
//	          2 分 #30 が item-b を v1 に変える
//	          3 分 #30 が item-b の v1 を承認       ← story-24 はここから分かれる
//	story-24: 4 分 #24 が item-a を v1 に変える
//	          5 分 #24 が item-a の v1 を承認
//	          6 分 #30 が item-a を v2 に変える
func newRV14Repo(t *testing.T, version string) *rv14Repo {
	root := t.TempDir()
	r := &rv14Repo{t: t, root: root, env: reviewBaseEnv(root)}
	r.g = &rvGit{t: t, root: root, env: append(reviewBaseEnv(root),
		"GIT_AUTHOR_NAME=テスト担当", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=テスト担当", "GIT_COMMITTER_EMAIL=tester@example.com")}
	r.g.run("init", "-q", "-b", "main")
	r.g.run("config", "user.name", reviewUserName)
	r.g.run("config", "user.email", reviewUserEmail)
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf("version = %q\n\n[[layer]]\nid = \"model\"\nname = \"モデル\"\ndescription = \"判定と状態の変化\"\n", version))
	writeFile(t, root, ".vanda/promises.toml", rv14Promises(0, 0))
	writeFile(t, root, ".vanda/agreements.toml", agreementTOML("term", "t", sha256Hex([]byte(`{"definition":"d","id":"t","name":"n"}`)), "", "#9", "テスト担当", "tester@example.com"))
	r.agree("item-a", 0, "#9", 0)
	r.agree("item-b", 0, "#9", 0)
	r.commit("はじめ", "#9", 0)
	r.agree("item-b", 0, "#24", 1)
	r.commit("item-b を承認する", "#24", 1)
	writeFile(t, root, ".vanda/promises.toml", rv14Promises(0, 1))
	r.commit("item-b を変える", "#30", 2)
	r.agree("item-b", 1, "#30", 3)
	r.commit("item-b を承認する", "#30", 3)
	r.g.run("switch", "-q", "-c", "story-24")
	writeFile(t, root, ".vanda/promises.toml", rv14Promises(1, 1))
	r.commit("item-a を変える", "#24", 4)
	r.agree("item-a", 1, "#24", 5)
	r.commit("item-a を承認する", "#24", 5)
	writeFile(t, root, ".vanda/promises.toml", rv14Promises(2, 1))
	r.commit("item-a を変える", "#30", 6)
	return r
}

func findEntry(es []reviewEntry, id string) *reviewEntry {
	for i := range es {
		if str(es[i].ID) == id {
			return &es[i]
		}
	}
	return nil
}

func checkDetached(t *testing.T, where string, got reviewResponse) {
	t.Helper()
	a := findEntry(got.Entries, "item-a")
	if a == nil {
		t.Fatalf("%s: 一覧に item-a がない", where)
	}
	if a.DetachedAgreement == nil || !reflect.DeepEqual(a.DetachedAgreement.By, []string{"#30"}) {
		t.Errorf("%s: item-a の detachedAgreement が {by: [#30]} でない: %+v", where, a.DetachedAgreement)
	}
	if str(a.State) != "proposed" || !reflect.DeepEqual(a.Flags, []string{"changed-since-agreement"}) || str(a.Fingerprint) != rv14FP("item-a", 2) {
		t.Errorf("%s: item-a の state %q、flags %v、fingerprint %q", where, str(a.State), a.Flags, str(a.Fingerprint))
	}
	if findEntry(got.Entries, "item-b") != nil {
		t.Errorf("%s: 比べる元と今で変わっていない item-b が entries に出た", where)
	}
	if got.Total != len(got.Entries) {
		t.Errorf("%s: total %d が entries の数 %d と違う（detached は数えない）", where, got.Total, len(got.Entries))
	}
	if len(got.Detached) != 1 {
		t.Fatalf("%s: detached が 1 件（item-b）でない: %+v", where, got.Detached)
	}
	b := got.Detached[0]
	if str(b.ID) != "item-b" || b.Kind != "claim" || b.No != nil || b.Change != "unchanged" || !isNull(b.Before) || isNull(b.After) ||
		b.DetachedAgreement == nil || !reflect.DeepEqual(b.DetachedAgreement.By, []string{"#30"}) {
		t.Errorf("%s: detached の item-b が決まりどおりでない: %+v（by %+v）", where, b, b.DetachedAgreement)
	}
}

// RV-14
// @vanda:evidence review-detached-shown
// @vanda:evidence review-detached-unchanged-shown
// @vanda:evidence review-other-commits
func TestRV14はストーリーの承認が失効したものをレビュー単位がブランチのときもストーリーのときも変えたストーリーと一緒に示す(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newRV14Repo(t, version)

	// 用意した履歴が、段 1 の承認のチェックから見ても、#24 の失効した承認が item-a と item-b であることを確かめておく
	checks := t.TempDir()
	writeFile(t, checks, "story.toml", "[[check]]\nname = \"agreement\"\nscope = \"story\"\n")
	rr := &reviewRepo{t: t, Root: r.root, Env: r.env}
	code, _, errs := rr.vanda(bin, "check", filepath.Join(checks, "story.toml"), "--base", "main", "--story", "#24")
	if code != 1 || !strings.Contains(errs, "item-a") || !strings.Contains(errs, "item-b") {
		t.Fatalf("段 1 のチェックで、#24 の item-a と item-b が止まらない（%d）\n%s", code, errs)
	}

	s := startServe(t, bin, r.root, r.env)

	var branch reviewResponse
	if code, msg := s.get("/api/review?view=branch&base=main&story="+q("#24"), &branch); code != 200 {
		t.Fatalf("レビュー単位がブランチのときの一覧を読めない（%d）: %s", code, msg)
	}
	checkDetached(t, "レビュー単位がブランチのとき", branch)

	var story reviewResponse
	if code, msg := s.get("/api/review?view=story&story="+q("#24"), &story); code != 200 {
		t.Fatalf("レビュー単位がストーリーのときの一覧を読めない（%d）: %s", code, msg)
	}
	checkDetached(t, "レビュー単位がストーリーのとき", story)
	if a := findEntry(story.Entries, "item-a"); a != nil {
		if len(a.OtherCommits) != 1 || !reflect.DeepEqual(a.OtherCommits[0].Stories, []string{"#30"}) || a.OtherCommits[0].Subject != "item-a を変える" {
			t.Errorf("レビュー単位がストーリーのときの item-a の otherCommits が、#30 のコミット 1 つでない: %+v", a.OtherCommits)
		}
	}

	// #24 が今の中身を承認し直すと、失効した承認ではなくなる
	r.agree("item-a", 2, "#24", 7)
	r.agree("item-b", 1, "#24", 7)
	r.commit("承認し直す", "#24", 7)
	var again reviewResponse
	if code, msg := s.get("/api/review?view=branch&base=main&story="+q("#24"), &again); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	a := findEntry(again.Entries, "item-a")
	if a == nil || a.DetachedAgreement != nil || str(a.State) != "agreed" || len(a.Flags) != 0 {
		t.Errorf("承認し直した item-a が、承認済で失効していないものになっていない: %+v", a)
	}
	if len(again.Detached) != 0 {
		t.Errorf("承認し直したのに detached が残っている: %+v", again.Detached)
	}

	// ほかのストーリーの承認しかない #30 から見ると、#30 の承認は失効していない（item-b は #30 が承認した v1 のまま）
	var other reviewResponse
	if code, msg := s.get("/api/review?view=branch&base=main&story="+q("#30"), &other); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	if len(other.Detached) != 0 {
		t.Errorf("#30 の承認は失効していないのに detached がある: %+v", other.Detached)
	}
}
