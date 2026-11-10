package review

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/annotation"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/guardrail"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

type ClaimDetail struct {
	ID          string           `json:"id"`
	Statement   []Piece          `json:"statement"`
	Rationale   []Piece          `json:"rationale"`
	Condition   []Piece          `json:"condition"`
	Layers      []LayerRef       `json:"layers"`
	Subjects    []SubjectRef     `json:"subjects"`
	State       string           `json:"state"`
	Fingerprint *string          `json:"fingerprint"`
	Flags       []string         `json:"flags"`
	Guesses     [][]Piece        `json:"guesses"`
	Evidence    []EvidenceDetail `json:"evidence"`
	// Refs は、この応答の中のセグメントが指すすべての ID の、指す先
	Refs map[string]RefTarget `json:"refs"`
}

type LayerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SubjectRef struct {
	Kind *string `json:"kind"` // 約束リストにない ID なら null
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

type EvidenceDetail struct {
	EvidenceContent
	Found []Found `json:"found"`
	// Present は、エビデンス完備のチェックと同じ判定。アノテーションで確かめないタイプ（human-review）は nil
	Present *bool `json:"present"`
}

type Found struct {
	Adapter string  `json:"adapter"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	Name    string  `json:"name"`
	Text    *string `json:"text"`
	// State は、ファイルが承認の対象（シナリオ・オラクルの定義・ガードレールの定義）のとき、今の中身のフィンガープリントに合う承認の記録があるか
	// （agreed・proposed。ストーリーによらない）。承認の対象でないもの（Go のテストなど）は nil
	State *string `json:"state"`
}

// ClaimHistory は、約束の変更履歴と承認の履歴
type ClaimHistory struct {
	Origin     []OriginRef    `json:"origin"`
	Agreements []AgreementRef `json:"agreements"`
}

type OriginRef struct {
	Commit  string   `json:"commit"`
	Date    string   `json:"date"`
	Subject string   `json:"subject"`
	Stories []string `json:"stories"`
	Change  string   `json:"change"`
}

type AgreementRef struct {
	User        User   `json:"user"`
	AgreedAt    string `json:"agreedAt"`
	Story       string `json:"story"`
	Fingerprint string `json:"fingerprint"`
	Current     bool   `json:"current"`
	// Commit は、この承認の記録を書き足したコミット（CL-08）。まだコミットしていなければ nil
	Commit *string `json:"commit"`
}

// Claim は、約束の詳細を返す。変更履歴と承認の履歴は、履歴のコミットをたどって時間がかかるので、ClaimHistoryOf で別に返す。
func Claim(o Options, id string) (*ClaimDetail, *Error) {
	s := &state{o: o, h: newHistory(o.Root, o.Cfg)}
	cur, e := s.currentClaim(id)
	if e != nil {
		return nil, e
	}
	key := "claim:" + id
	c := cur.Claim
	content := claimContent(cur, s.refs, nil)
	d := &ClaimDetail{ID: c.ID, Statement: content.Statement, Rationale: content.Rationale, Condition: content.Condition, Flags: content.Flags, Guesses: content.Guesses,
		Layers: []LayerRef{}, Subjects: []SubjectRef{}, Evidence: []EvidenceDetail{}}
	fp := s.cur.itemFP[key]
	if fp != "" {
		d.Fingerprint = ptr(fp)
	}
	d.State = stateName(fp != "" && s.recs.Agreed("claim", c.ID, fp))

	names := map[string]string{}
	for _, l := range o.Cfg.Layers {
		names[l.ID] = l.Name
	}
	for _, l := range c.Layers {
		d.Layers = append(d.Layers, LayerRef{ID: l, Name: names[l]})
	}
	for _, sub := range c.Subjects {
		ref := SubjectRef{ID: sub}
		for _, kind := range []string{"term", "feature"} {
			if e, ok := s.cur.items[kind+":"+sub]; ok {
				ref.Kind = ptr(kind)
				if e.Term != nil {
					ref.Name = ptr(e.Term.Name)
				} else {
					ref.Name = ptr(e.Feature.Name)
				}
				break
			}
		}
		d.Subjects = append(d.Subjects, ref)
	}

	anns, aerrs := annotation.ReadWith(o.Root, s.cur.text, s.cur.snap.Files, o.Cfg.Annotations)
	if aerrs != nil {
		return nil, &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("アノテーションに誤りがあるため、エビデンスを探せません（%d 件。最初の 1 件: %s）。", len(aerrs), aerrs[0].Error()), Fix: "vanda check の annotation-links で誤りを確かめて、直してください。"}
	}
	guards, _ := guardrail.Files(s.cur.snap.Files, o.Cfg.Guardrails)
	for _, ev := range content.Evidence {
		ed := EvidenceDetail{EvidenceContent: ev, Found: []Found{}}
		if config.EvidenceKind(ev.Type) != "" {
			present := true
			var found []annotation.Annotation
			for _, p := range annotation.Evidence(anns, guards, c.ID, ev.Type) {
				present = present && len(p.Found) > 0
				found = append(found, p.Found...)
			}
			for _, a := range found {
				f := Found{Adapter: a.Adapter, File: a.File, Name: a.Name}
				text, line, err := evidenceText(s.cur, a)
				f.Line = line
				if _, target := s.cur.types[a.File]; target {
					fp, err := s.cur.fileFP(a.File)
					if err != nil {
						return nil, internalErr(err)
					}
					f.State = ptr(stateName(s.recs.Agreed("file", a.File, fp)))
				}
				if err != nil {
					return nil, internalErr(err)
				}
				f.Text = text
				ed.Found = append(ed.Found, f)
			}
			ed.Present = ptr(present)
		} else if ev.Type == "proof" {
			// 段 1 のチェックでは、proof を予定に書いたら誤り（扱えるアダプタがない）
			ed.Present = ptr(false)
		}
		d.Evidence = append(d.Evidence, ed)
	}

	d.Refs = s.refs.targets()
	return d, nil
}

func (s *state) currentClaim(id string) (snapshot.Entry, *Error) {
	if e := s.loadCurrent(); e != nil {
		return snapshot.Entry{}, e
	}
	cur, ok := s.cur.items["claim:"+id]
	if !ok {
		return snapshot.Entry{}, &Error{Status: http.StatusNotFound, Message: "約束 " + id + " は約束リストにありません。", Fix: "約束の ID を確かめてください。"}
	}
	return cur, nil
}

// ClaimHistoryOf は、約束の変更履歴と承認の履歴を返す。
func ClaimHistoryOf(o Options, id string) (*ClaimHistory, *Error) {
	s := &state{o: o, h: newHistory(o.Root, o.Cfg)}
	cur, e := s.currentClaim(id)
	if e != nil {
		return nil, e
	}
	key := "claim:" + id
	c := cur.Claim
	fp := s.cur.itemFP[key]
	d := &ClaimHistory{Origin: []OriginRef{}, Agreements: []AgreementRef{}}

	commits, err := s.h.loadCommits()
	if err != nil {
		return nil, internalErr(err)
	}
	s.h.prefetch(commits)
	for _, cm := range commits {
		cs, err := s.h.changed(cm)
		if err != nil {
			return nil, internalErr(err)
		}
		if !cs.items[key] {
			continue
		}
		before, err := s.h.sideAt(cm.Parent)
		if err != nil {
			return nil, internalErr(err)
		}
		after, err := s.h.sideAt(cm.Hash)
		if err != nil {
			return nil, internalErr(err)
		}
		if _, inAfter := after.items[key]; !inAfter {
			// 消した後に同じ ID で作り直したものは、新しい約束とみなすので、消すまでの変更履歴を捨てる
			d.Origin = d.Origin[:0]
			continue
		}
		change := "modified"
		if _, inBefore := before.items[key]; !inBefore {
			change = "added"
		}
		d.Origin = append(d.Origin, OriginRef{Commit: cm.Hash, Date: cm.Date.Format(time.RFC3339), Subject: cm.Subject, Stories: strs(cm.Stories), Change: change})
	}

	var mine []*agreements.Agreement
	for _, a := range s.recs.Agreements {
		// 消したことへの承認は中身を持たず、承認の履歴（中身ごとの承認）に並べられないので除く
		if a.Kind == "claim" && a.Key == c.ID && !a.Deleted {
			mine = append(mine, a)
		}
	}
	written, err := s.agreementCommits(mine)
	if err != nil {
		return nil, internalErr(err)
	}
	for _, a := range mine {
		ref := AgreementRef{User: User{Name: a.UserName, Email: a.UserEmail}, AgreedAt: a.AgreedAt.Format(time.RFC3339), Story: a.Story, Fingerprint: a.Fingerprint, Current: fp != "" && a.Fingerprint == fp}
		if h := written[a]; h != "" {
			ref.Commit = ptr(h)
		}
		d.Agreements = append(d.Agreements, ref)
	}
	newestFirst(d, commits, mine)
	return d, nil
}

// evidenceText は、エビデンスの本文と、アノテーションが付いているものの行を返す。本文は、txtar と sql はファイル全体、tla は付いている定義、go は返さない。
func evidenceText(cur *side, a annotation.Annotation) (*string, int, error) {
	src, err := cur.text(a.File)
	if err != nil {
		return nil, 0, fmt.Errorf("%s を読めません（%v）", a.File, err)
	}
	line := annotation.TargetLine(a.Adapter, a.File, src, a.Line)
	switch a.Adapter {
	case "go", "ts":
		return nil, line, nil
	case "tla":
		def, ok := annotation.TLADefinition(src, a.Line)
		if !ok {
			return nil, 0, fmt.Errorf("%s:%d: アノテーションが付いている定義が見つかりません", a.File, a.Line)
		}
		return &def, line, nil
	}
	text := string(src)
	return &text, line, nil
}

// File は、承認の対象のファイルか、アノテーションを読むファイルの中身を返す。
func File(o Options, path string) (string, *Error) {
	notFound := &Error{Status: http.StatusNotFound, Message: path + " は、承認の対象のファイルでも、アノテーションを読むファイルでもありません。", Fix: "ファイルの場所を確かめてください。"}
	tree, err := repo.HeadTree(o.Root)
	if err != nil {
		return "", internalErr(err)
	}
	files := tree.Files
	have := false
	for _, f := range files {
		if f == path {
			have = true
			break
		}
	}
	if !have {
		return "", notFound
	}
	allowed := false
	for _, t := range snapshot.Targets(files, o.Cfg) {
		if t.Path == path {
			allowed = true
		}
	}
	for _, r := range o.Cfg.Annotations {
		for _, p := range r.Paths {
			if glob.Match(p, path) {
				allowed = true
			}
		}
	}
	if !allowed {
		return "", notFound
	}
	b, err := tree.Read(path)
	if err != nil {
		return "", internalErr(err)
	}
	return string(b), nil
}

// newestFirst は、変更履歴と承認の履歴を、新しいものが上の順に並べる。
// 変更履歴はコミットの日時の新しい順、同じ日時なら履歴で後のコミットを先に。承認の履歴は承認した日時（agreed_at）の新しい順、
// 同じ日時なら承認の記録のファイルで後に書いた記録を先に（同じ日時のときも、後から足したものが上に来るように）
func newestFirst(d *ClaimHistory, commits []commit, recs []*agreements.Agreement) {
	order := map[string]int{}
	date := map[string]time.Time{}
	for i, c := range commits {
		order[c.Hash] = i
		date[c.Hash] = c.Date
	}
	sort.SliceStable(d.Origin, func(i, j int) bool {
		a, b := d.Origin[i].Commit, d.Origin[j].Commit
		if !date[a].Equal(date[b]) {
			return date[a].After(date[b])
		}
		return order[a] > order[b]
	})
	idx := make([]int, len(d.Agreements))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool {
		a, b := recs[idx[i]].AgreedAt, recs[idx[j]].AgreedAt
		if !a.Equal(b) {
			return a.After(b)
		}
		return idx[i] > idx[j]
	})
	sorted := make([]AgreementRef, len(idx))
	for k, i := range idx {
		sorted[k] = d.Agreements[i]
	}
	d.Agreements = sorted
}
