package review

import (
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// SpecContent・TermContent・FeatureContent・PropertyContent・TruthContent・FileContent は、一覧と詳細に出す中身。省いた項目は nil（JSON の null）。
// 自然言語で書く項目は、参照を置き換えたセグメントの配列にする。
type SpecContent struct {
	Statement []Piece           `json:"statement"`
	Rationale []Piece           `json:"rationale"`
	Condition []Piece           `json:"condition"`
	Refines   []string          `json:"refines"`
	Assumes   []string          `json:"assumes"`
	Layers    []string          `json:"layers"`
	Evidence  []EvidenceContent `json:"evidence"`
	Flags     []string          `json:"flags"`
	Guesses   [][]Piece         `json:"guesses"`
	// ModelCheckWaiver は、TLA+ で確かめない理由。承認の対象の中身なので、一覧で見せる
	ModelCheckWaiver []Piece `json:"modelCheckWaiver"`
}

type EvidenceContent struct {
	Stage  string   `json:"stage"`
	Type   string   `json:"type"`
	Layers []string `json:"layers"`
	Detail []Piece  `json:"detail"`
}

type TermContent struct {
	Name       string   `json:"name"`
	Definition []Piece  `json:"definition"`
	PartOf     *string  `json:"partOf"`
	Assumes    []string `json:"assumes"`
}

type FeatureContent struct {
	Name         string   `json:"name"`
	Definition   []Piece  `json:"definition"`
	Refines      []string `json:"refines"`
	Core         *bool    `json:"core"`
	Formal       *bool    `json:"formal"`
	FormalWaiver []Piece  `json:"formalWaiver"`
}

type PropertyContent struct {
	Statement []Piece  `json:"statement"`
	Rationale []Piece  `json:"rationale"`
	Refines   []string `json:"refines"`
	Assumes   []string `json:"assumes"`
}

type TruthContent struct {
	Statement []Piece `json:"statement"`
}

type FileContent struct {
	Text string `json:"text"`
}

func optStr(has bool, s string) *string {
	if !has {
		return nil
	}
	return &s
}

func strs(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

// Content は、項目の中身を、書いたとおりに返す。参照は r で置き換え、今の約束リストにない指す先は base から探す。
func Content(e snapshot.Entry, r *refs, base *side) any {
	switch {
	case e.Spec != nil:
		return specContent(e, r, base)
	case e.Term != nil:
		return TermContent{Name: e.Term.Name, Definition: r.pieces(e.Term.Definition, base), PartOf: optStr(e.Table.Has("part_of"), e.Term.PartOf), Assumes: strs(e.Term.Assumes)}
	case e.Feature != nil:
		f := e.Feature
		out := FeatureContent{Name: f.Name, Definition: r.pieces(f.Definition, base), Refines: strs(f.Refines), FormalWaiver: r.optPieces(e.Table.Has("formal_waiver"), f.FormalWaiver, base)}
		if e.Table.Has("core") {
			v := f.Core
			out.Core = &v
		}
		if e.Table.Has("formal") {
			v := f.Formal
			out.Formal = &v
		}
		return out
	case e.Property != nil:
		p := e.Property
		return PropertyContent{Statement: r.pieces(p.Statement, base), Rationale: r.optPieces(e.Table.Has("rationale"), p.Rationale, base), Refines: strs(p.Refines), Assumes: strs(p.Assumes)}
	case e.Truth != nil:
		return TruthContent{Statement: r.pieces(e.Truth.Statement, base)}
	}
	return nil
}

func specContent(e snapshot.Entry, r *refs, base *side) SpecContent {
	c := e.Spec
	out := SpecContent{
		Statement: r.pieces(c.Statement, base),
		Rationale: r.optPieces(e.Table.Has("rationale"), c.Rationale, base),
		Condition: r.optPieces(e.Table.Has("condition"), c.Condition, base),
		Refines:   strs(c.Refines),
		Assumes:   strs(c.Assumes),
		Layers:    strs(c.Layers),
		Evidence:  []EvidenceContent{},
		Flags:     strs(c.Flags),
		Guesses:   [][]Piece{},

		ModelCheckWaiver: r.optPieces(e.Table.Has("model_check_waiver"), c.ModelCheckWaiver, base),
	}
	for _, g := range c.Guesses {
		out.Guesses = append(out.Guesses, r.pieces(g, base))
	}
	for _, ev := range c.Evidence {
		out.Evidence = append(out.Evidence, EvidenceContent{Stage: ev.Stage, Type: ev.Type, Layers: strs(ev.Layers), Detail: r.optPieces(ev.Table.Has("detail"), ev.Detail, base)})
	}
	return out
}
