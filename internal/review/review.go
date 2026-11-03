// Package review は、ストーリーで追加・変更・削除されたものの一覧（RV-01）を作る。
package review

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// Error は、一覧を作れない理由と、API で返す HTTP の状態コード。
type Error struct {
	Status  int
	Message string
	Fix     string
}

// Error は、文言の決まりで「。」で終わる message と fix を、そのままつなぐ
func (e *Error) Error() string { return e.Message + e.Fix }

func internalErr(err error) *Error {
	return &Error{Status: http.StatusInternalServerError, Message: err.Error() + "。", Fix: "git の状態を確かめてください。"}
}

type Result struct {
	View      string  `json:"view"`
	Base      *string `json:"base"`
	MergeBase *string `json:"mergeBase"`
	Story     *string `json:"story"`
	Total     int     `json:"total"`
	Entries   []Entry `json:"entries"`
	Detached  []Entry `json:"detached"`
	// BranchStories は、レビュー単位がブランチのとき、分かれた点より後のコミットの Story: のトレーラーにあるストーリーの ID（名前の順）。ストーリーのときは nil
	BranchStories []string `json:"branchStories"`
	// StoryFrom は、story をどう決めたか。query（story= で名指しした）・branch（ブランチのコミットから 1 つに決まった）。決まっていなければ空
	StoryFrom string `json:"storyFrom"`
	// StoryChoice は、画面でストーリーをどう示すか（RV-13）。fixed（1 つに決まったので選ばせずに示す）・branch（ブランチのストーリーから選ばせる）・all（すべてのストーリーから選ばせる）
	StoryChoice string `json:"storyChoice"`
	// Refs は、entries と detached の中の片が指すすべての ID の、指す先
	Refs map[string]RefTarget `json:"refs"`
}

type Entry struct {
	No                *int        `json:"no"`
	Kind              string      `json:"kind"`
	ID                *string     `json:"id"`
	Path              *string     `json:"path"`
	FileType          *string     `json:"fileType"`
	Change            string      `json:"change"`
	State             *string     `json:"state"`
	Fingerprint       *string     `json:"fingerprint"`
	Flags             []string    `json:"flags"`
	Group             string      `json:"group"`
	Before            any         `json:"before"`
	After             any         `json:"after"`
	Diff              any         `json:"diff"`
	DetachedAgreement *Detached   `json:"detachedAgreement"`
	Revoke            *Revocation `json:"revoke"`
	Agree             *Agreeable  `json:"agree"`
	OtherCommits      []CommitRef `json:"otherCommits"`

	key string // 項目なら "<種類>:<ID>"、ファイルなら "path:<場所>"
}

type Detached struct {
	By []string `json:"by"`
}

type CommitRef struct {
	Commit  string   `json:"commit"`
	Subject string   `json:"subject"`
	Stories []string `json:"stories"`
}

const (
	ViewBranch = "branch"
	ViewStory  = "story"
)

type Options struct {
	Root  string
	Cfg   *config.Config
	View  string
	Base  string
	Story string
}

// state は、一覧を作るあいだに使うもの。
type state struct {
	o    Options
	h    *history
	cur  *side
	recs *agreements.Records
	refs *refs
}

// Build は、一覧を作る。
func Build(o Options) (*Result, *Error) {
	s := &state{o: o, h: newHistory(o.Root, o.Cfg)}
	if err := s.loadCurrent(); err != nil {
		return nil, err
	}
	res := &Result{View: o.View, Entries: []Entry{}, Detached: []Entry{}}
	var changes []rawChange
	switch o.View {
	case ViewBranch:
		if o.Base == "" {
			return nil, &Error{Status: http.StatusBadRequest, Message: "レビュー単位がブランチのときに要る、ベースブランチ（base）がありません。", Fix: "base にベースブランチを渡してください。"}
		}
		mb, err := repo.Git(o.Root, "merge-base", o.Base, "HEAD")
		if err != nil {
			return nil, &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("ベースブランチ %s から分かれた点が分かりません（%v）。", o.Base, err), Fix: "今のブランチと履歴のつながったブランチを選んでください。"}
		}
		mb = strings.TrimSpace(mb)
		res.Base, res.MergeBase = &o.Base, &mb
		story, ids, e := s.branchStory(mb)
		if e != nil {
			return nil, e
		}
		res.BranchStories = ids
		switch {
		case o.Story != "":
			res.StoryFrom = "query"
		case story != "":
			res.StoryFrom = "branch"
		}
		s.o.Story = story
		before, err := s.h.sideAt(mb)
		if err != nil {
			return nil, internalErr(err)
		}
		changes, err = s.diffAll(before)
		if err != nil {
			return nil, internalErr(err)
		}
	case ViewStory:
		if o.Story == "" {
			return nil, &Error{Status: http.StatusBadRequest, Message: "レビュー単位がストーリーのときに要る、ストーリーの ID（story）がありません。", Fix: "story にストーリーの ID を渡してください。"}
		}
		if e := s.checkStory(o.Story); e != nil {
			return nil, e
		}
		var err error
		changes, err = s.diffStory()
		if err != nil {
			return nil, internalErr(err)
		}
	default:
		return nil, &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("レビュー単位 %q はありません。", o.View), Fix: "view を branch か story にしてください。"}
	}
	if s.o.Story != "" {
		st := s.o.Story
		res.Story = &st
	}
	if res.View == ViewBranch && res.BranchStories == nil {
		res.BranchStories = []string{}
	}
	res.StoryChoice = storyChoice(res.View, o.Story, res.BranchStories)
	entries, err := s.entries(changes)
	if err != nil {
		return nil, internalErr(err)
	}
	if err := checkCount(changes, entries); err != nil {
		return nil, err
	}
	detached, err := s.detached(entries)
	if err != nil {
		return nil, internalErr(err)
	}
	mbStr := ""
	if res.MergeBase != nil {
		mbStr = *res.MergeBase
	}
	if err := s.setRevocations(entries, mbStr); err != nil {
		return nil, internalErr(err)
	}
	if err := s.setRevocations(detached, mbStr); err != nil {
		return nil, internalErr(err)
	}
	s.setAgreeables(entries)
	s.setAgreeables(detached)
	s.order(entries)
	for i := range entries {
		n := i + 1
		entries[i].No = &n
	}
	if entries != nil {
		res.Entries = entries
	}
	res.Detached = detached
	res.Total = len(entries)
	res.Refs = s.refs.targets()
	return res, nil
}

// loadCurrent は、今の中身として HEAD のコミットを読む（コミットしていない変更は、提出される保証がないので読まない）。
func (s *state) loadCurrent() *Error {
	tree, err := repo.HeadTree(s.o.Root)
	if err != nil {
		return internalErr(err)
	}
	snap := snapshot.Committed(tree, s.o.Cfg)
	if snap.ListErrs != nil {
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("約束リストに誤りがあるため、一覧を作れません（%d 件。最初の 1 件: %s）。", len(snap.ListErrs), snap.ListErrs[0].Error()), Fix: "vanda check の promise-list-shape で誤りを確かめて、直してください。"}
	}
	recs, rerrs := agreements.LoadWith(tree.Read, snap.Files, s.o.Cfg.Agreements, s.o.Cfg.AgreementsDefault)
	if rerrs != nil {
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("合意の記録に誤りがあるため、一覧を作れません（%d 件。最初の 1 件: %s）。", len(rerrs), rerrs[0].Error()), Fix: "vanda check の agreement-targets で誤りを確かめて、直してください。"}
	}
	s.cur = newSide(snap)
	s.recs = recs
	s.refs = newRefs(s.cur)
	return nil
}

func (s *state) checkStory(id string) *Error {
	if s.o.Cfg.IDFull == nil || !s.o.Cfg.IDFull.MatchString(id) {
		return &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("ストーリーの ID %s が、書き方（%s）に合いません。", id, s.o.Cfg.IDPattern), Fix: "設定の id_pattern に合う ID を渡してください。"}
	}
	return nil
}

// branchStory は、レビュー単位がブランチのときのストーリーを、vanda check と同じく決める。渡されていればそれを、なければ分かれた点より後のコミットの Story: のトレーラーにある ID が 1 つのときだけそれを使う。
func (s *state) branchStory(mb string) (string, []string, *Error) {
	ids, err := storyIDsIn(s.o.Root, s.o.Cfg, mb+"..HEAD")
	if err != nil {
		return "", nil, internalErr(fmt.Errorf("コミットのメッセージを読めません（%v）", err))
	}
	sort.Strings(ids)
	if s.o.Story != "" {
		return s.o.Story, ids, s.checkStory(s.o.Story)
	}
	if len(ids) == 1 {
		return ids[0], ids, nil
	}
	return "", ids, nil
}

// storyChoice は、画面でストーリーをどう示すかを決める。レビュー単位がブランチで、ブランチのコミットからストーリーが 1 つに決まるなら示すだけ、
// 2 つ以上ならそのブランチのストーリーから、見つからなければすべてのストーリーから選ばせる。story= で別のストーリーを名指ししたら、それを選べるようにする
func storyChoice(view, named string, branch []string) string {
	if view != ViewBranch {
		return "all"
	}
	switch {
	case len(branch) == 1 && (named == "" || named == branch[0]):
		return "fixed"
	case len(branch) >= 2:
		return "branch"
	}
	return "all"
}

// rawChange は、比べる元と今で中身の違う鍵 1 つ。before はその鍵の比べる元の時点。
type rawChange struct {
	key    string
	before *side
	others []CommitRef
}

func itemKeys(a, b *side) []string {
	var out []string
	for k := range union(a.itemFP, b.itemFP) {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fileKeys(a, b *side) []string {
	var out []string
	for k := range union(a.types, b.types) {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// diffAll は、before と今の中身を比べる（レビュー単位がブランチ）。
func (s *state) diffAll(before *side) ([]rawChange, error) {
	var out []rawChange
	for _, k := range itemKeys(before, s.cur) {
		if !sameItem(before, s.cur, k) {
			out = append(out, rawChange{key: k, before: before})
		}
	}
	for _, f := range fileKeys(before, s.cur) {
		same, err := sameFile(before, s.cur, f)
		if err != nil {
			return nil, err
		}
		if !same {
			out = append(out, rawChange{key: "path:" + f, before: before})
		}
	}
	return out, nil
}

// diffStory は、鍵ごとに、ストーリーの ID を持つコミットのうちその鍵を変えた最初のコミットの親の中身と、今の中身を比べる（レビュー単位がストーリー）。
func (s *state) diffStory() ([]rawChange, error) {
	commits, err := s.h.loadCommits()
	if err != nil {
		return nil, err
	}
	first := map[string]int{} // 鍵から、最初に変えたコミットの番号へ
	var keys []string
	for i, c := range commits {
		if !hasStory(c, s.o.Story) {
			continue
		}
		cs, err := s.h.changed(c)
		if err != nil {
			return nil, err
		}
		for _, k := range sortedKeys(cs) {
			if _, ok := first[k]; !ok {
				first[k] = i
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	var out []rawChange
	for _, k := range keys {
		i := first[k]
		before, err := s.h.sideAt(commits[i].Parent)
		if err != nil {
			return nil, err
		}
		same := false
		if f, ok := strings.CutPrefix(k, "path:"); ok {
			same, err = sameFile(before, s.cur, f)
			if err != nil {
				return nil, err
			}
		} else {
			same = sameItem(before, s.cur, k)
		}
		if same {
			continue
		}
		rc := rawChange{key: k, before: before, others: []CommitRef{}}
		for _, c := range commits[i+1:] {
			if hasStory(c, s.o.Story) {
				continue
			}
			cs, err := s.h.changed(c)
			if err != nil {
				return nil, err
			}
			if cs.has(k) {
				rc.others = append(rc.others, CommitRef{Commit: c.Hash, Subject: c.Subject, Stories: strs(c.Stories)})
			}
		}
		out = append(out, rc)
	}
	return out, nil
}

func (cs *changeSet) has(key string) bool {
	if f, ok := strings.CutPrefix(key, "path:"); ok {
		return cs.files[f]
	}
	return cs.items[key]
}

func sortedKeys(cs *changeSet) []string {
	var out []string
	for k := range cs.items {
		out = append(out, k)
	}
	for f := range cs.files {
		out = append(out, "path:"+f)
	}
	sort.Strings(out)
	return out
}

var kindName = map[string]string{"term": "用語", "feature": "機能", "claim": "約束", "file": "ファイル"}

// entries は、違う鍵を一覧の 1 件ずつにする。中身が同じで場所だけが変わったファイルも、まとめずに、古い場所の削除と新しい場所の追加の 2 件にする
// （ファイルは場所で見分け、どちらの場所にも合意が要るため）。
func (s *state) entries(changes []rawChange) ([]Entry, error) {
	var out []Entry
	for _, rc := range changes {
		path, isFile := strings.CutPrefix(rc.key, "path:")
		if !isFile {
			out = append(out, s.itemEntry(rc))
			continue
		}
		_, inBefore := rc.before.types[path]
		_, inAfter := s.cur.types[path]
		change := "modified"
		switch {
		case inBefore && !inAfter:
			change = "deleted"
		case !inBefore && inAfter:
			change = "added"
		}
		e, err := s.fileEntry(rc, path, change)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func ptr[T any](v T) *T { return &v }

func (s *state) itemEntry(rc rawChange) Entry {
	be, inBefore := rc.before.items[rc.key]
	ae, inAfter := s.cur.items[rc.key]
	ref := ae
	if !inAfter {
		ref = be
	}
	e := Entry{Kind: ref.Kind, ID: ptr(ref.ID), Flags: []string{}, OtherCommits: others(rc), key: rc.key}
	switch {
	case !inBefore:
		e.Change = "added"
	case !inAfter:
		e.Change = "deleted"
	default:
		e.Change = "modified"
	}
	if inBefore {
		e.Before = Content(be, s.refs, rc.before)
	}
	if inAfter {
		e.After = Content(ae, s.refs, rc.before)
		s.itemState(&e, ae)
	} else {
		e.State = ptr(stateName(s.recs.DeletionAgreed(ref.Kind, ref.ID)))
	}
	if inBefore && inAfter {
		e.Diff = itemDiff(e.Before, e.After)
	}
	e.Group = s.group(ref)
	return e
}

func others(rc rawChange) []CommitRef {
	if rc.others == nil {
		return []CommitRef{}
	}
	return rc.others
}

// itemState は、今の項目の、合意の状態・指紋・フラグを埋める。
func (s *state) itemState(e *Entry, cur snapshot.Entry) {
	fp := s.cur.itemFP[cur.Key()]
	if fp != "" {
		e.Fingerprint = ptr(fp)
	}
	agreed := fp != "" && s.recs.Agreed(cur.Kind, cur.ID, fp)
	e.State = ptr(stateName(agreed))
	if c := cur.Claim; c != nil {
		for _, f := range c.Flags {
			if f == "ai-proposal" {
				e.Flags = append(e.Flags, "ai-proposal")
				break
			}
		}
		if len(c.Guesses) > 0 {
			e.Flags = append(e.Flags, "guess")
		}
	}
	if !agreed {
		for _, a := range s.recs.Agreements {
			if a.Kind == cur.Kind && a.Key == cur.ID && !a.Deleted {
				e.Flags = append(e.Flags, "changed-since-agreement")
				break
			}
		}
	}
}

func stateName(agreed bool) string {
	if agreed {
		return "agreed"
	}
	return "proposed"
}

// fileEntry は、ファイルの 1 件を作る。path は今の場所（削除なら前の場所）。
func (s *state) fileEntry(rc rawChange, path, change string) (Entry, error) {
	e := Entry{Kind: "file", Path: ptr(path), Change: change, Flags: []string{}, Group: "files", OtherCommits: others(rc), key: "path:" + path}
	if change != "added" {
		b, err := rc.before.text(path)
		if err != nil {
			return e, err
		}
		e.Before = FileContent{Text: string(b)}
		e.FileType = ptr(rc.before.types[path])
	}
	if change == "deleted" {
		e.State = ptr(stateName(s.recs.DeletionAgreed("file", path)))
	} else {
		if err := s.fileState(&e, path); err != nil {
			return e, err
		}
	}
	if b, ok := e.Before.(FileContent); ok {
		if a, ok := e.After.(FileContent); ok {
			e.Diff = fileDiff(b.Text, a.Text)
		}
	}
	return e, nil
}

func (s *state) fileState(e *Entry, path string) error {
	b, err := s.cur.text(path)
	if err != nil {
		return err
	}
	e.After = FileContent{Text: string(b)}
	e.FileType = ptr(s.cur.types[path])
	fp := agreements.FileFingerprint(b)
	e.Fingerprint = ptr(fp)
	agreed := s.recs.Agreed("file", path, fp)
	e.State = ptr(stateName(agreed))
	if !agreed {
		for _, a := range s.recs.Agreements {
			if a.Kind == "file" && a.Path == path && !a.Deleted {
				e.Flags = append(e.Flags, "changed-since-agreement")
				break
			}
		}
	}
	return nil
}

func (s *state) group(e snapshot.Entry) string {
	if e.Claim != nil && len(e.Claim.Layers) > 0 {
		return "layer:" + e.Claim.Layers[0]
	}
	if e.Claim != nil {
		return "layer:"
	}
	return "terms-features"
}

// order は、まとまりの順（設定のレイヤーの順の約束、用語と機能、ファイル）、同じまとまりの中は ID か場所の順に並べる。
func (s *state) order(es []Entry) {
	rank := map[string]int{}
	for i, l := range s.o.Cfg.Layers {
		rank["layer:"+l.ID] = i
	}
	groupRank := func(g string) (int, string) {
		switch {
		case g == "terms-features":
			return 2, ""
		case g == "files":
			return 3, ""
		}
		if r, ok := rank[g]; ok {
			return 0, fmt.Sprintf("%08d", r)
		}
		// 設定にないレイヤーの約束は、約束リストの形の審査で誤りになる。設定のレイヤーの後に、名前の順で置く
		return 1, g
	}
	name := func(e Entry) string {
		if e.ID != nil {
			return *e.ID
		}
		return *e.Path
	}
	sort.SliceStable(es, func(i, j int) bool {
		ri, si := groupRank(es[i].Group)
		rj, sj := groupRank(es[j].Group)
		if ri != rj {
			return ri < rj
		}
		if si != sj {
			return si < sj
		}
		return name(es[i]) < name(es[j])
	})
}

// detached は、ストーリーの外れた合意（RV-14）を探す。一覧にあるものはその件に印を付け、ないものを返す。
func (s *state) detached(es []Entry) ([]Entry, error) {
	out := []Entry{}
	story := s.o.Story
	if story == "" {
		return out, nil
	}
	// 鍵ごとの、そのストーリーの中身への合意の記録
	recs := map[string][]*agreements.Agreement{}
	var keys []string
	for _, a := range s.recs.Agreements {
		// 消したことにだけ合意したものを足し直しても、外れた合意にしない（中身に合意したことがないため）
		if a.Story != story || a.Deleted {
			continue
		}
		k := a.Kind + ":" + a.Key
		if a.Kind == "file" {
			k = "path:" + a.Path
		}
		if _, ok := recs[k]; !ok {
			keys = append(keys, k)
		}
		recs[k] = append(recs[k], a)
	}
	sort.Strings(keys)
	var written map[*agreements.Agreement]string // 記録から、それを書き足したコミットへ。外れた合意があるときだけ探す
	index := map[string]int{}
	for i, e := range es {
		index[e.key] = i
	}
	for _, k := range keys {
		var fp string
		if f, ok := strings.CutPrefix(k, "path:"); ok {
			if _, ok := s.cur.types[f]; !ok {
				continue // 名前を変えたファイルは追わない
			}
			var err error
			if fp, err = s.cur.fileFP(f); err != nil {
				return nil, err
			}
			if s.recs.AgreedBy("file", f, fp, story) {
				continue
			}
		} else {
			cur, ok := s.cur.items[k]
			if !ok {
				continue // 消した項目は、合意の記録の指す先の審査で出す
			}
			fp = s.cur.itemFP[k]
			if fp != "" && s.recs.AgreedBy(cur.Kind, cur.ID, fp, story) {
				continue
			}
		}
		if written == nil {
			var all []*agreements.Agreement
			for _, k := range keys {
				all = append(all, recs[k]...)
			}
			var err error
			if written, err = s.agreementCommits(all); err != nil {
				return nil, err
			}
		}
		by, err := s.changedBy(k, recs[k], written)
		if err != nil {
			return nil, err
		}
		d := &Detached{By: by}
		if i, ok := index[k]; ok {
			es[i].DetachedAgreement = d
			continue
		}
		e, err := s.unchanged(k)
		if err != nil {
			return nil, err
		}
		e.DetachedAgreement = d
		out = append(out, e)
	}
	return out, nil
}

// changedBy は、HEAD の履歴で、記録 recs（そのストーリーの、鍵 key への合意）を最後に書き足したコミットより後のコミットのうち、
// key の中身を変えたもののストーリーの ID を返す。written は、記録からそれを書き足したコミットへの表。
// 合意の記録の agreed_at とコミットの日時を比べないのは、rebase や amend でコミットの日時が変わるため（約束 review-detached-shown）。
// まだコミットしていない記録しかなければ、それより後のコミットはないので空にする。
func (s *state) changedBy(key string, recs []*agreements.Agreement, written map[*agreements.Agreement]string) ([]string, error) {
	commits, err := s.h.loadCommits()
	if err != nil {
		return nil, err
	}
	after := len(commits)
	found := false
	for i := len(commits) - 1; i >= 0 && !found; i-- {
		for _, a := range recs {
			if written[a] == commits[i].Hash {
				after, found = i+1, true
				break
			}
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, c := range commits[after:] {
		cs, err := s.h.changed(c)
		if err != nil {
			return nil, err
		}
		if !cs.has(key) {
			continue
		}
		for _, id := range c.Stories {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}

// unchanged は、比べる元と今で変わっていない、外れた合意の 1 件を作る。
func (s *state) unchanged(key string) (Entry, error) {
	if f, ok := strings.CutPrefix(key, "path:"); ok {
		e := Entry{Kind: "file", Path: ptr(f), Change: "unchanged", Flags: []string{}, Group: "files", OtherCommits: []CommitRef{}, key: key}
		return e, s.fileState(&e, f)
	}
	cur := s.cur.items[key]
	e := Entry{Kind: cur.Kind, ID: ptr(cur.ID), Change: "unchanged", Flags: []string{}, OtherCommits: []CommitRef{}, key: key, After: Content(cur, s.refs, nil), Group: s.group(cur)}
	s.itemState(&e, cur)
	return e, nil
}
