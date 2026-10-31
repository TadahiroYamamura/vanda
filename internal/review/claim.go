package review

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
		return nil, &Error{Status: http.StatusNotFound, Message: "約束 " + id + " が約束リストにない", Fix: "約束の ID を確かめる"}
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
		return nil, &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("アノテーションに誤りがあり、エビデンスを探せない（%d 件。最初の 1 件: %s）", len(aerrs), aerrs[0].Error()), Fix: "vanda check の annotation-links で誤りを出して直す"}
	}
	for _, ev := range content.Evidence {
		ed := EvidenceDetail{EvidenceContent: ev, Found: []Found{}}
		want, ok := check.EvidenceTypes[ev.Type]
		if ok {
			for _, a := range anns {
				if a.Adapter != want[0] || a.Kind != want[1] || a.Value != c.ID {
					continue
				}
				f := Found{Adapter: a.Adapter, File: a.File, Line: a.Line, Name: a.Name}
				text, err := evidenceText(o.Root, a)
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

	for _, a := range s.recs.Agreements {
		if a.Kind != "claim" || a.Key != c.ID {
			continue
		}
		d.Agreements = append(d.Agreements, AgreementRef{User: User{Name: a.UserName, Email: a.UserEmail}, AgreedAt: a.AgreedAt.Format(time.RFC3339), Story: a.Story, Fingerprint: a.Fingerprint, Current: fp != "" && a.Fingerprint == fp})
	}
	return d, nil
}

// evidenceText は、エビデンスの本文を返す。txtar と sql はファイル全体、tla は付いている定義、go は返さない。
func evidenceText(root string, a annotation.Annotation) (*string, error) {
	if a.Adapter == "go" {
		return nil, nil
	}
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(a.File)))
	if err != nil {
		return nil, fmt.Errorf("%s を読めない（%v）", a.File, err)
	}
	if a.Adapter == "tla" {
		def, ok := annotation.TLADefinition(src, a.Line)
		if !ok {
			return nil, fmt.Errorf("%s:%d: アノテーションの付いている定義が見つからない", a.File, a.Line)
		}
		return &def, nil
	}
	text := string(src)
	return &text, nil
}

// File は、合意の対象のファイルか、アノテーションを読むファイルの中身を返す。
func File(o Options, path string) (string, *Error) {
	notFound := &Error{Status: http.StatusNotFound, Message: path + " は、合意の対象のファイルでも、アノテーションを読むファイルでもない", Fix: "場所を確かめる"}
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
