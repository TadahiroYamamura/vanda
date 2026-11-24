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
	Assumes  []ItemRef            `json:"assumes"`
	Refs     map[string]RefTarget `json:"refs"`
}

// FeatureDetail は、機能の詳細。Formal は、formal を書いていなければ core と同じ値。
type FeatureDetail struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Definition []Piece              `json:"definition"`
	Core       bool                 `json:"core"`
	Formal     bool                 `json:"formal"`
	Refines    []ItemRef            `json:"refines"`
	Claims     []ClaimSummary       `json:"specs"`
	Refs       map[string]RefTarget `json:"refs"`
}

// NamedRef は、用語を指すもの。Name は、今の約束リストにない ID なら nil。
type NamedRef struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// ClaimSummary は、機能を refines する仕様の 1 つ。
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
	d := &TermDetail{ID: t.ID, Name: t.Name, Definition: s.refs.pieces(t.Definition, nil), Children: []NamedRef{}, Assumes: s.itemRefs(t.Assumes)}
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
	d := &FeatureDetail{ID: f.ID, Name: f.Name, Definition: s.refs.pieces(f.Definition, nil), Core: f.Core, Formal: f.Formal, Refines: s.itemRefs(f.Refines)}
	d.Claims = s.claimsAbout(f.ID)
	d.Refs = s.refs.targets()
	return d, nil
}

// claimsAbout は、機能 feature を refines する仕様を、約束リストに書いた順に返す。
func (s *state) claimsAbout(feature string) []ClaimSummary {
	out := []ClaimSummary{}
	seen := map[string]bool{}
	for _, e := range snapshot.Entries(s.cur.snap.List) {
		if e.Claim == nil || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		for _, up := range e.Claim.Refines {
			if up == feature {
				out = append(out, ClaimSummary{ID: e.ID, Statement: s.refs.pieces(e.Claim.Statement, nil)})
				break
			}
		}
	}
	return out
}

// PropertyDetail は、性質の詳細。
type PropertyDetail struct {
	ID          string               `json:"id"`
	Statement   []Piece              `json:"statement"`
	Rationale   []Piece              `json:"rationale"`
	Refines     []ItemRef            `json:"refines"`
	Assumes     []ItemRef            `json:"assumes"`
	State       string               `json:"state"`
	Fingerprint *string              `json:"fingerprint"`
	Refs        map[string]RefTarget `json:"refs"`
}

// TruthDetail は、前提の詳細。
type TruthDetail struct {
	ID          string               `json:"id"`
	Statement   []Piece              `json:"statement"`
	State       string               `json:"state"`
	Fingerprint *string              `json:"fingerprint"`
	Refs        map[string]RefTarget `json:"refs"`
}

// currentItem は、今の約束リストから、種類 kind・ID id の項目を探す。なければ 404。
func (s *state) currentItem(kind, id string) (snapshot.Entry, *Error) {
	if e := s.loadCurrent(); e != nil {
		return snapshot.Entry{}, e
	}
	cur, ok := s.cur.items[kind+":"+id]
	if !ok {
		return snapshot.Entry{}, &Error{Status: http.StatusNotFound, Message: kindName[kind] + " " + id + " は約束リストにありません。", Fix: kindName[kind] + "の ID を確かめてください。"}
	}
	return cur, nil
}

// agreedState は、項目 e の今の中身の、承認の状態とフィンガープリントを返す。フィンガープリントを作れなければ nil。
func (s *state) agreedState(e snapshot.Entry) (string, *string) {
	fp := s.cur.itemFP[e.Key()]
	if fp == "" {
		return stateName(false), nil
	}
	return stateName(s.recs.Agreed(e.Kind, e.ID, fp)), &fp
}

// Property は、性質の詳細を返す。今の約束リストにない ID なら 404。
func Property(o Options, id string) (*PropertyDetail, *Error) {
	s := &state{o: o}
	e, err := s.currentItem("property", id)
	if err != nil {
		return nil, err
	}
	p := e.Property
	d := &PropertyDetail{ID: p.ID, Statement: s.refs.pieces(p.Statement, nil), Rationale: s.refs.optPieces(e.Table.Has("rationale"), p.Rationale, nil),
		Refines: s.itemRefs(p.Refines), Assumes: s.itemRefs(p.Assumes)}
	d.State, d.Fingerprint = s.agreedState(e)
	d.Refs = s.refs.targets()
	return d, nil
}

// Truth は、前提の詳細を返す。今の約束リストにない ID なら 404。
func Truth(o Options, id string) (*TruthDetail, *Error) {
	s := &state{o: o}
	e, err := s.currentItem("truth", id)
	if err != nil {
		return nil, err
	}
	d := &TruthDetail{ID: e.ID, Statement: s.refs.pieces(e.Truth.Statement, nil)}
	d.State, d.Fingerprint = s.agreedState(e)
	d.Refs = s.refs.targets()
	return d, nil
}

// ItemHistoryOf は、性質・前提（種類 kind）の変更履歴と承認の履歴を返す。形は、仕様のもの（ClaimHistoryOf）と同じ。
func ItemHistoryOf(o Options, kind, id string) (*ClaimHistory, *Error) {
	s := &state{o: o, h: newHistory(o.Root, o.Cfg)}
	cur, err := s.currentItem(kind, id)
	if err != nil {
		return nil, err
	}
	return s.historyOf(cur)
}
