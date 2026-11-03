package review

import (
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// ClaimContent・TermContent・FeatureContent・FileContent は、一覧と詳細に出す中身。省いた項目は nil（JSON の null）。
type ClaimContent struct {
	Statement string            `json:"statement"`
	Rationale *string           `json:"rationale"`
	Condition *string           `json:"condition"`
	Subjects  []string          `json:"subjects"`
	Layers    []string          `json:"layers"`
	Evidence  []EvidenceContent `json:"evidence"`
	Flags     []string          `json:"flags"`
	Guesses   []string          `json:"guesses"`
	// ModelCheckWaiver は、TLA+ で確かめない理由。合意の対象の中身なので、一覧で見せる
	ModelCheckWaiver *string `json:"modelCheckWaiver"`
}

type EvidenceContent struct {
	Stage  string   `json:"stage"`
	Type   string   `json:"type"`
	Layers []string `json:"layers"`
	Detail *string  `json:"detail"`
}

type TermContent struct {
	Name       string  `json:"name"`
	Definition string  `json:"definition"`
	PartOf     *string `json:"partOf"`
}

type FeatureContent struct {
	Name         string  `json:"name"`
	Definition   string  `json:"definition"`
	Core         *bool   `json:"core"`
	Formal       *bool   `json:"formal"`
	FormalWaiver *string `json:"formalWaiver"`
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

// Content は、項目の中身を、書いたとおりに返す。
func Content(e snapshot.Entry) any {
	switch {
	case e.Claim != nil:
		c := e.Claim
		out := ClaimContent{
			Statement: c.Statement,
			Rationale: optStr(e.Table.Has("rationale"), c.Rationale),
			Condition: optStr(e.Table.Has("condition"), c.Condition),
			Subjects:  strs(c.Subjects),
			Layers:    strs(c.Layers),
			Evidence:  []EvidenceContent{},
			Flags:     strs(c.Flags),
			Guesses:   strs(c.Guesses),

			ModelCheckWaiver: optStr(e.Table.Has("model_check_waiver"), c.ModelCheckWaiver),
		}
		for _, ev := range c.Evidence {
			out.Evidence = append(out.Evidence, EvidenceContent{Stage: ev.Stage, Type: ev.Type, Layers: strs(ev.Layers), Detail: optStr(ev.Table.Has("detail"), ev.Detail)})
		}
		return out
	case e.Term != nil:
		return TermContent{Name: e.Term.Name, Definition: e.Term.Definition, PartOf: optStr(e.Table.Has("part_of"), e.Term.PartOf)}
	case e.Feature != nil:
		f := e.Feature
		out := FeatureContent{Name: f.Name, Definition: f.Definition, FormalWaiver: optStr(e.Table.Has("formal_waiver"), f.FormalWaiver)}
		if e.Table.Has("core") {
			v := f.Core
			out.Core = &v
		}
		if e.Table.Has("formal") {
			v := f.Formal
			out.Formal = &v
		}
		return out
	}
	return nil
}
