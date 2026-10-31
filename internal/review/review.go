// Package review は、ストーリーで追加・変更・削除されたものの一覧（RV-01）を作る。
package review

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

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

func (e *Error) Error() string { return e.Message + "。" + e.Fix }

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
}

type Entry struct {
	No                *int        `json:"no"`
	Kind              string      `json:"kind"`
	ID                *string     `json:"id"`
	Path              *string     `json:"path"`
	OldPath           *string     `json:"oldPath"`
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
			return nil, &Error{Status: http.StatusBadRequest, Message: "このブランチの変更を見るための、ベースブランチ（base）がありません。", Fix: "base にベースブランチを渡してください。"}
		}
		mb, err := repo.Git(o.Root, "merge-base", o.Base, "HEAD")
		if err != nil {
			return nil, &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("ベースブランチ %s から分かれた点が分かりません（%v）。", o.Base, err), Fix: "今のブランチと履歴のつながったブランチを選んでください。"}
		}
		mb = strings.TrimSpace(mb)
		res.Base, res.MergeBase = &o.Base, &mb
		story, e := s.branchStory(mb)
		if e != nil {
			return nil, e
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
			return nil, &Error{Status: http.StatusBadRequest, Message: "ストーリーの変更を見るための、ストーリーの ID（story）がありません。", Fix: "story にストーリーの ID を渡してください。"}
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
		return nil, &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("見方 %q はありません。", o.View), Fix: "view を branch か story にしてください。"}
	}
	if s.o.Story != "" {
		st := s.o.Story
		res.Story = &st
	}
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
	return res, nil
}

func (s *state) loadCurrent() *Error {
	snap, err := snapshot.Working(s.o.Root, s.o.Cfg)
	if err != nil {
		return internalErr(err)
	}
	if snap.ListErrs != nil {
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("約束リストに誤りがあるため、一覧を作れません（%d 件。最初の 1 件: %s）。", len(snap.ListErrs), snap.ListErrs[0].Error()), Fix: "vanda check の promise-list-shape で誤りを確かめて、直してください。"}
	}
	recs, rerrs := agreements.Load(s.o.Root, snap.Files, s.o.Cfg.Agreements, s.o.Cfg.AgreementsDefault)
	if rerrs != nil {
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("合意の記録に誤りがあるため、一覧を作れません（%d 件。最初の 1 件: %s）。", len(rerrs), rerrs[0].Error()), Fix: "vanda check の agreement-targets で誤りを確かめて、直してください。"}
	}
	s.cur = newSide(snap)
	s.recs = recs
	return nil
}

func (s *state) checkStory(id string) *Error {
	if s.o.Cfg.IDFull == nil || !s.o.Cfg.IDFull.MatchString(id) {
		return &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("ストーリーの ID %s が、書き方（%s）に合いません。", id, s.o.Cfg.IDPattern), Fix: "設定の id_pattern に合う ID を渡してください。"}
	}
	return nil
}

// branchStory は、ブランチごとの見方のストーリーを、vanda check と同じく決める。渡されていればそれを、なければ分かれた点より後のコミットのメッセージにある ID が 1 つのときだけそれを使う。
func (s *state) branchStory(mb string) (string, *Error) {
	if s.o.Story != "" {
		return s.o.Story, s.checkStory(s.o.Story)
	}
	msgs, err := repo.Git(s.o.Root, "log", "--format=%B", mb+"..HEAD")
	if err != nil {
		return "", internalErr(fmt.Errorf("コミットのメッセージを読めません（%v）", err))
	}
	ids := storyIDs(s.o.Cfg, msgs)
	if len(ids) == 1 {
		return ids[0], nil
	}
	return "", nil
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

// diffAll は、before と今の中身を比べる（ブランチごとの見方）。
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

// diffStory は、鍵ごとに、ストーリーの ID を持つコミットのうちその鍵を変えた最初のコミットの親の中身と、今の中身を比べる（ストーリー全体の見方）。
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

// entries は、違う鍵を一覧の 1 件ずつにする。中身が同じで場所だけが変わったファイルは、名前の変更の 1 件にまとめる。
func (s *state) entries(changes []rawChange) ([]Entry, error) {
	var out []Entry
	type fileSide struct {
		rc   rawChange
		path string
		fp   string
	}
	var deleted, added []fileSide
	for _, rc := range changes {
		path, isFile := strings.CutPrefix(rc.key, "path:")
		if !isFile {
			out = append(out, s.itemEntry(rc))
			continue
		}
		_, inBefore := rc.before.types[path]
		_, inAfter := s.cur.types[path]
		switch {
		case inBefore && !inAfter:
			fp, err := rc.before.fileFP(path)
			if err != nil {
				return nil, err
			}
			deleted = append(deleted, fileSide{rc, path, fp})
		case !inBefore && inAfter:
			fp, err := s.cur.fileFP(path)
			if err != nil {
				return nil, err
			}
			added = append(added, fileSide{rc, path, fp})
		default:
			e, err := s.fileEntry(rc, path, path, "modified")
			if err != nil {
				return nil, err
			}
			out = append(out, e)
		}
	}
	used := map[int]bool{}
	for _, a := range added {
		pair := -1
		for i, d := range deleted {
			if !used[i] && d.fp == a.fp {
				pair = i
				break
			}
		}
		rc, change, old := a.rc, "added", ""
		if pair >= 0 {
			used[pair] = true
			d := deleted[pair]
			// 比べる元は、前の場所の比べる元の時点（ストーリー全体の見方では、場所ごとに時点が違う）
			rc = rawChange{key: a.rc.key, before: d.rc.before, others: s.mergeCommits(d.rc.others, a.rc.others)}
			change, old = "renamed", d.path
		}
		e, err := s.fileEntry(rc, a.path, old, change)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	for i, d := range deleted {
		if used[i] {
			continue
		}
		e, err := s.fileEntry(d.rc, d.path, d.path, "deleted")
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// mergeCommits は、2 つのコミットの並びを、重ねずに、履歴の古い順にまとめる。
// 名前の変更では、前の場所と今の場所のどちらかを変えたコミットを、どちらも示す（見落とすより、多めに示すほうを選んだ）。
func (s *state) mergeCommits(a, b []CommitRef) []CommitRef {
	if a == nil && b == nil {
		return nil
	}
	order := map[string]int{}
	for i, c := range s.h.commits {
		order[c.Hash] = i
	}
	seen := map[string]bool{}
	out := []CommitRef{}
	for _, c := range append(append([]CommitRef{}, a...), b...) {
		if !seen[c.Commit] {
			seen[c.Commit] = true
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].Commit] < order[out[j].Commit] })
	return out
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
		e.Before = Content(be)
	}
	if inAfter {
		e.After = Content(ae)
		s.itemState(&e, ae)
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
			if a.Kind == cur.Kind && a.Key == cur.ID {
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

// fileEntry は、ファイルの 1 件を作る。path は今の場所（削除なら前の場所）、old は比べる元の場所。
func (s *state) fileEntry(rc rawChange, path, old, change string) (Entry, error) {
	e := Entry{Kind: "file", Path: ptr(path), Change: change, Flags: []string{}, Group: "files", OtherCommits: others(rc), key: "path:" + path}
	if change == "renamed" {
		e.OldPath = ptr(old)
	}
	if change != "added" {
		b, err := rc.before.text(old)
		if err != nil {
			return e, err
		}
		e.Before = FileContent{Text: string(b)}
		e.FileType = ptr(rc.before.types[old])
	}
	if change != "deleted" {
		if err := s.fileState(&e, path, old); err != nil {
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

func (s *state) fileState(e *Entry, path, old string) error {
	b, err := s.cur.text(path)
	if err != nil {
		return err
	}
	e.After = FileContent{Text: string(b)}
	e.FileType = ptr(s.cur.types[path])
	fp := agreements.FileFingerprint(b)
	e.Fingerprint = ptr(fp)
	agreed := s.recs.Agreed("file", fp, fp)
	e.State = ptr(stateName(agreed))
	if !agreed {
		for _, a := range s.recs.Agreements {
			if a.Kind == "file" && (a.Path == path || a.Path == old) {
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

// checkCount は、一覧の数が、違う鍵の数から名前の変更の数を引いたものに合うかを確かめる。
func checkCount(changes []rawChange, es []Entry) *Error {
	renamed := 0
	seen := map[string]bool{}
	for _, e := range es {
		if seen[e.key] {
			return &Error{Status: http.StatusInternalServerError, Message: "一覧に同じもの（" + e.key + "）が 2 度出ています。", Fix: "vanda の不具合です。開発者に知らせてください。"}
		}
		seen[e.key] = true
		if e.Change == "renamed" {
			renamed++
		}
	}
	if want := len(changes) - renamed; len(es) != want {
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("一覧の数（%d）と差分の数（%d）が合いません。", len(es), want), Fix: "vanda の不具合です。開発者に知らせてください。"}
	}
	return nil
}

// detached は、ストーリーの外れた合意（RV-14）を探す。一覧にあるものはその件に印を付け、ないものを返す。
func (s *state) detached(es []Entry) ([]Entry, error) {
	out := []Entry{}
	story := s.o.Story
	if story == "" {
		return out, nil
	}
	// 鍵ごとの、そのストーリーの最後の合意の日時
	last := map[string]time.Time{}
	var keys []string
	for _, a := range s.recs.Agreements {
		if a.Story != story {
			continue
		}
		k := a.Kind + ":" + a.Key
		if a.Kind == "file" {
			k = "path:" + a.Path
		}
		if t, ok := last[k]; !ok || a.AgreedAt.After(t) {
			if !ok {
				keys = append(keys, k)
			}
			last[k] = a.AgreedAt
		}
	}
	sort.Strings(keys)
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
			if s.recs.AgreedBy("file", fp, fp, story) {
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
		by, err := s.changedBy(k, last[k])
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

// changedBy は、日時 since より後のコミットのうち、鍵 key の中身を変えたもののストーリーの ID を返す。
// 合意のコミットを履歴から探す代わりに、合意の記録の日時と、コミットの日時を比べる（合意の記録を手で書いた場合も扱えるように）。
func (s *state) changedBy(key string, since time.Time) ([]string, error) {
	commits, err := s.h.loadCommits()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, c := range commits {
		if !c.Date.After(since) {
			continue
		}
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
		return e, s.fileState(&e, f, f)
	}
	cur := s.cur.items[key]
	e := Entry{Kind: cur.Kind, ID: ptr(cur.ID), Change: "unchanged", Flags: []string{}, OtherCommits: []CommitRef{}, key: key, After: Content(cur), Group: s.group(cur)}
	s.itemState(&e, cur)
	return e, nil
}
