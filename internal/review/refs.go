package review

import (
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// Piece は、自然言語で書く項目の文の片。参照なら Ref に指す ID を持ち、Text は見せる文字（用語と機能は名前、約束は ID）。
type Piece struct {
	Text string `json:"text"`
	Ref  string `json:"ref,omitempty"`
}

// RefTarget は、片が指す先。Name は用語と機能なら名前、約束なら nil。Text は、用語と機能なら定義、約束なら文を、参照を見せる文字に置き換えたもの。
// Core・Formal は、機能のときだけ持つ（コア機能か・形式手法を使う機能か）。
type RefTarget struct {
	Kind    string  `json:"kind"`
	Name    *string `json:"name"`
	Text    string  `json:"text"`
	Current bool    `json:"current"`
	// Pieces は、Text を片に分けたもの。中の参照の指す先は、refs に足さない（枠の中の参照は見た目だけにし、次の枠もリンクも出さないため）
	Pieces []Piece `json:"pieces"`
	Core   *bool   `json:"core,omitempty"`
	Formal *bool   `json:"formal,omitempty"`
}

// refs は、参照を見せる文字に置き換え、応答の中の片が指した先を集める。
type refs struct {
	cur  map[string]snapshot.Entry
	used map[string]RefTarget
}

func newRefs(cur *side) *refs {
	r := &refs{used: map[string]RefTarget{}}
	if cur != nil {
		r.cur = cur.byID()
	}
	return r
}

// lookup は、指す先を今の約束リストから探し、なければ比べる元 base から探す（reference-diff の決まり）。
func (r *refs) lookup(id string, base *side) (snapshot.Entry, bool, bool) {
	if e, ok := r.cur[id]; ok {
		return e, true, true
	}
	if base != nil {
		if e, ok := base.byID()[id]; ok {
			return e, false, true
		}
	}
	return snapshot.Entry{}, false, false
}

func shown(e snapshot.Entry) string {
	switch {
	case e.Term != nil:
		return e.Term.Name
	case e.Feature != nil:
		return e.Feature.Name
	}
	return e.ID
}

// split は、文 s を片に分ける。指す先のない参照と壊れた参照は、書いたままの文字の片にする。record なら、指した先を集める。
func (r *refs) split(s string, base *side, record bool) []Piece {
	out := []Piece{}
	text := func(t string) {
		if n := len(out); n > 0 && out[n-1].Ref == "" {
			out[n-1].Text += t
			return
		}
		out = append(out, Piece{Text: t})
	}
	for _, p := range promises.SplitRefs(s) {
		if p.Ref == "" {
			text(p.Text)
			continue
		}
		e, current, ok := r.lookup(p.Ref, base)
		if !ok {
			text(p.Text)
			continue
		}
		out = append(out, Piece{Text: shown(e), Ref: p.Ref})
		if record {
			if _, done := r.used[p.Ref]; !done {
				r.used[p.Ref] = r.target(e, current, base)
			}
		}
	}
	return out
}

func (r *refs) target(e snapshot.Entry, current bool, base *side) RefTarget {
	t := RefTarget{Kind: e.Kind, Current: current}
	var body string
	switch {
	case e.Term != nil:
		t.Name, body = ptr(e.Term.Name), e.Term.Definition
	case e.Feature != nil:
		t.Name, body = ptr(e.Feature.Name), e.Feature.Definition
		t.Core, t.Formal = ptr(e.Feature.Core), ptr(e.Feature.Formal)
	case e.Claim != nil:
		body = e.Claim.Statement
	}
	t.Pieces = r.split(body, base, false)
	t.Text = plain(t.Pieces)
	return t
}

func (r *refs) pieces(s string, base *side) []Piece { return r.split(s, base, true) }

func (r *refs) optPieces(has bool, s string, base *side) []Piece {
	if !has {
		return nil
	}
	return r.pieces(s, base)
}

// targets は、集めた指す先を返す。何も指していなければ空の表。
func (r *refs) targets() map[string]RefTarget { return r.used }

func plain(ps []Piece) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString(p.Text)
	}
	return b.String()
}

// refDiff は、片どうしを見せる文字で比べ、差分の 1 つが参照にかかる所を参照の境で分けて、参照の ID を付ける。
// delete は変更前の側、equal と insert は変更後の側の参照の位置で決める。
func refDiff(before, after []Piece) []DiffPart {
	parts := charDiff(plain(before), plain(after))
	bs, as := spans(before), spans(after)
	var out []DiffPart
	bpos, apos := 0, 0
	for _, p := range parts {
		sp, pos := as, apos
		if p.Op == "delete" {
			sp, pos = bs, bpos
		}
		out = append(out, cut(p, pos, sp)...)
		n := len(p.Text)
		switch p.Op {
		case "equal":
			bpos += n
			apos += n
		case "delete":
			bpos += n
		default:
			apos += n
		}
	}
	if out == nil {
		out = []DiffPart{}
	}
	return out
}

type span struct {
	start, end int
	ref        string
}

func spans(ps []Piece) []span {
	var out []span
	pos := 0
	for _, p := range ps {
		if p.Ref != "" {
			out = append(out, span{pos, pos + len(p.Text), p.Ref})
		}
		pos += len(p.Text)
	}
	return out
}

// cut は、文字の位置 pos から始まる差分の 1 つ p を、参照の境で分ける。境は文字の境なので、UTF-8 の途中では分けない。
func cut(p DiffPart, pos int, sp []span) []DiffPart {
	var out []DiffPart
	start, end := pos, pos+len(p.Text)
	at := start
	emit := func(to int, ref string) {
		if to > at {
			out = append(out, DiffPart{Op: p.Op, Text: p.Text[at-start : to-start], Ref: ref})
			at = to
		}
	}
	for _, s := range sp {
		if s.end <= at || s.start >= end {
			continue
		}
		emit(max(s.start, at), "")
		emit(min(s.end, end), s.ref)
	}
	emit(end, "")
	return out
}
