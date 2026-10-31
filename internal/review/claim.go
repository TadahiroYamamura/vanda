package review

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/annotation"
	"github.com/TadahiroYamamura/vanda/internal/check"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

type ClaimDetail struct {
	ID          string           `json:"id"`
	Statement   string           `json:"statement"`
	Rationale   *string          `json:"rationale"`
	Condition   *string          `json:"condition"`
	Layers      []LayerRef       `json:"layers"`
	Subjects    []SubjectRef     `json:"subjects"`
	State       string           `json:"state"`
	Fingerprint *string          `json:"fingerprint"`
	Flags       []string         `json:"flags"`
	Guesses     []string         `json:"guesses"`
	Evidence    []EvidenceDetail `json:"evidence"`
	Origin      []OriginRef      `json:"origin"`
	Agreements  []AgreementRef   `json:"agreements"`
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
	// Present は、エビデンスが揃っている審査と同じ判定。アノテーションで確かめないタイプ（human-review・monitoring）は nil
	Present *bool `json:"present"`
}

type Found struct {
	Adapter string  `json:"adapter"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	Name    string  `json:"name"`
	Text    *string `json:"text"`
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
	// Commit は、この合意の記録を書き足したコミット（CL-08）。まだコミットしていなければ nil
	Commit *string `json:"commit"`
}

// Claim は、約束の詳細（CL-01・02・04・05）を返す。
func Claim(o Options, id string) (*ClaimDetail, *Error) {
	s := &state{o: o, h: newHistory(o.Root, o.Cfg)}
	if e := s.loadCurrent(); e != nil {
		return nil, e
	}
	key := "claim:" + id
	cur, ok := s.cur.items[key]
	if !ok {
		return nil, &Error{Status: http.StatusNotFound, Message: "約束 " + id + " は約束リストにありません。", Fix: "約束の ID を確かめてください。"}
	}
	c := cur.Claim
	content := Content(cur).(ClaimContent)
	d := &ClaimDetail{ID: c.ID, Statement: c.Statement, Rationale: content.Rationale, Condition: content.Condition, Flags: content.Flags, Guesses: content.Guesses,
		Layers: []LayerRef{}, Subjects: []SubjectRef{}, Evidence: []EvidenceDetail{}, Origin: []OriginRef{}, Agreements: []AgreementRef{}}
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

	anns, aerrs := annotation.Read(o.Root, s.cur.snap.Files, o.Cfg.Annotations)
	if aerrs != nil {
		return nil, &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("アノテーションに誤りがあるため、エビデンスを探せません（%d 件。最初の 1 件: %s）。", len(aerrs), aerrs[0].Error()), Fix: "vanda check の annotation-links で誤りを確かめて、直してください。"}
	}
	for _, ev := range content.Evidence {
		ed := EvidenceDetail{EvidenceContent: ev, Found: []Found{}}
		want, ok := check.EvidenceTypes[ev.Type]
		if ok {
			for _, a := range anns {
				if a.Adapter != want[0] || a.Kind != want[1] || a.Value != c.ID {
					continue
				}
				f := Found{Adapter: a.Adapter, File: a.File, Name: a.Name}
				text, line, err := evidenceText(o.Root, a)
				f.Line = line
				if err != nil {
					return nil, internalErr(err)
				}
				f.Text = text
				ed.Found = append(ed.Found, f)
			}
			ed.Present = ptr(len(ed.Found) > 0)
		} else if ev.Type == "proof" {
			// 段 1 の審査では、proof を予定に書いたら誤り（扱えるアダプタがない）
			ed.Present = ptr(false)
		}
		d.Evidence = append(d.Evidence, ed)
	}

	commits, err := s.h.loadCommits()
	if err != nil {
		return nil, internalErr(err)
	}
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
			// 消した後に同じ ID で作り直したものは、新しい約束とみなすので、消すまでの由来を捨てる
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
		if a.Kind == "claim" && a.Key == c.ID {
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
func evidenceText(root string, a annotation.Annotation) (*string, int, error) {
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(a.File)))
	if err != nil {
		return nil, 0, fmt.Errorf("%s を読めません（%v）", a.File, err)
	}
	line := annotation.TargetLine(a.Adapter, a.File, src, a.Line)
	switch a.Adapter {
	case "go":
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

// File は、合意の対象のファイルか、アノテーションを読むファイルの中身を返す。
func File(o Options, path string) (string, *Error) {
	notFound := &Error{Status: http.StatusNotFound, Message: path + " は、合意の対象のファイルでも、アノテーションを読むファイルでもありません。", Fix: "ファイルの場所を確かめてください。"}
	files, err := glob.Files(o.Root)
	if err != nil {
		return "", internalErr(err)
	}
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
	b, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(path)))
	if err != nil {
		return "", internalErr(err)
	}
	return string(b), nil
}

// newestFirst は、由来と合意の履歴を、新しいものが上の順に並べる。
// 由来はコミットの日時の新しい順、同じ日時なら履歴で後のコミットを先に。合意の履歴は合意した日時（agreed_at）の新しい順、
// 同じ日時なら合意の記録のファイルで後に書いた記録を先に（同じ日時のときも、後から足したものが上に来るように）
func newestFirst(d *ClaimDetail, commits []commit, recs []*agreements.Agreement) {
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
