package review

import (
	"net/http"

	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// TermDetail は、用語の詳細。
type TermDetail struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Definition []Piece `json:"definition"`
	// PartOf は、part_of に書いた親の用語。書いていなければ nil
	PartOf *NamedRef `json:"partOf"`
	// Children は、part_of にこの用語を書いた用語を、約束リストに書いた順に並べたもの
	Children []NamedRef           `json:"children"`
	Claims   []ClaimSummary       `json:"claims"`
	Refs     map[string]RefTarget `json:"refs"`
}

// FeatureDetail は、機能の詳細。Formal は、formal を書いていなければ core と同じ値。
type FeatureDetail struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Definition []Piece              `json:"definition"`
	Core       bool                 `json:"core"`
	Formal     bool                 `json:"formal"`
	Claims     []ClaimSummary       `json:"claims"`
	Refs       map[string]RefTarget `json:"refs"`
}

// NamedRef は、用語を指すもの。Name は、今の約束リストにない ID なら nil。
type NamedRef struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// ClaimSummary は、用語・機能を subjects に持つ約束の 1 つ。
type ClaimSummary struct {
	ID        string  `json:"id"`
	Statement []Piece `json:"statement"`
}

// Term は、用語の詳細を返す。今の約束リストにない ID なら 404。
func Term(o Options, id string) (*TermDetail, *Error) {
	s := &state{o: o}
	if e := s.loadCurrent(); e != nil {
		return nil, e
	}
	e, ok := s.cur.items["term:"+id]
	if !ok {
		return nil, &Error{Status: http.StatusNotFound, Message: "用語 " + id + " は約束リストにありません。", Fix: "用語の ID を確かめてください。"}
	}
	t := e.Term
	d := &TermDetail{ID: t.ID, Name: t.Name, Definition: s.refs.pieces(t.Definition, nil), Children: []NamedRef{}}
	if e.Table.Has("part_of") {
		p := NamedRef{ID: t.PartOf}
		if pe, ok := s.cur.items["term:"+t.PartOf]; ok {
			p.Name = ptr(pe.Term.Name)
		}
		d.PartOf = &p
	}
	seen := map[string]bool{}
	for _, c := range snapshot.Entries(s.cur.snap.List) {
		if c.Term == nil || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		if c.Term.PartOf == t.ID {
			d.Children = append(d.Children, NamedRef{ID: c.ID, Name: ptr(c.Term.Name)})
		}
	}
	d.Claims = s.claimsAbout(t.ID)
	d.Refs = s.refs.targets()
	return d, nil
}

// Feature は、機能の詳細を返す。今の約束リストにない ID なら 404。
func Feature(o Options, id string) (*FeatureDetail, *Error) {
	s := &state{o: o}
	if e := s.loadCurrent(); e != nil {
		return nil, e
	}
	e, ok := s.cur.items["feature:"+id]
	if !ok {
		return nil, &Error{Status: http.StatusNotFound, Message: "機能 " + id + " は約束リストにありません。", Fix: "機能の ID を確かめてください。"}
	}
	f := e.Feature
	d := &FeatureDetail{ID: f.ID, Name: f.Name, Definition: s.refs.pieces(f.Definition, nil), Core: f.Core, Formal: f.Formal}
	d.Claims = s.claimsAbout(f.ID)
	d.Refs = s.refs.targets()
	return d, nil
}

// claimsAbout は、subject を subjects に持つ約束を、約束リストに書いた順に返す。
func (s *state) claimsAbout(subject string) []ClaimSummary {
	out := []ClaimSummary{}
	seen := map[string]bool{}
	for _, e := range snapshot.Entries(s.cur.snap.List) {
		if e.Claim == nil || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		for _, sub := range e.Claim.Subjects {
			if sub == subject {
				out = append(out, ClaimSummary{ID: e.ID, Statement: s.refs.pieces(e.Claim.Statement, nil)})
				break
			}
		}
	}
	return out
}
