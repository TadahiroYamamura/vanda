package promises

import (
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

// Part は、自然言語で書く項目の文を、参照とそれ以外に分けたセグメントの 1 つ。
type Part struct {
	// Text は、文字のセグメントなら見せる文字（{@@ は {@ にしたもの）。参照と壊れた参照なら、書いたままの文字（{@id:x} など）
	Text string
	// Ref は、参照なら指す ID。指す先があるかは見ていない
	Ref string
	// Broken は、{@<種類>:<値>} の形でないか、種類が id でない参照なら、何がいけないか。ほかは空
	Broken string
}

// SplitRefs は、文 s を左から読み、セグメントに分ける。{@@ は文字の {@ とし、ほかの {@ は次の } までを参照として読む。
// 隣り合う文字のセグメントはつなぐ。
func SplitRefs(s string) []Part {
	var out []Part
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out = append(out, Part{Text: text.String()})
			text.Reset()
		}
	}
	for i := 0; i < len(s); {
		if !strings.HasPrefix(s[i:], "{@") {
			text.WriteByte(s[i])
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "{@@") {
			text.WriteString("{@")
			i += 3
			continue
		}
		flush()
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			out = append(out, Part{Text: s[i:], Broken: "} がありません"})
			break
		}
		raw := s[i : i+end+1]
		i += end + 1
		kind, value, ok := strings.Cut(raw[2:len(raw)-1], ":")
		switch {
		case !ok:
			out = append(out, Part{Text: raw, Broken: ": がありません"})
		case kind != "id":
			out = append(out, Part{Text: raw, Broken: "種類 " + `"` + kind + `"` + " は id ではありません"})
		case value == "":
			out = append(out, Part{Text: raw, Broken: "ID がありません"})
		default:
			out = append(out, Part{Text: raw, Ref: value})
		}
	}
	flush()
	return out
}

// Prose は、自然言語で書く項目の 1 つの文。Index は、guesses の何番目か（ほかの項目は -1）。
type Prose struct {
	Table *tomlread.Table
	Key   string
	Index int
	Text  string
}

// Proses は、約束リストの、参照を書ける項目（名前を除く、自然言語で書く項目）の文を、書いてあるものだけ返す。
func (l *List) Proses() []Prose {
	var out []Prose
	add := func(t *tomlread.Table, key, s string) {
		if t.Has(key) {
			out = append(out, Prose{Table: t, Key: key, Index: -1, Text: s})
		}
	}
	for _, t := range l.Terms {
		add(t.Table, "definition", t.Definition)
	}
	for _, f := range l.Features {
		add(f.Table, "definition", f.Definition)
		add(f.Table, "formal_waiver", f.FormalWaiver)
	}
	for _, c := range l.Claims {
		add(c.Table, "statement", c.Statement)
		add(c.Table, "rationale", c.Rationale)
		add(c.Table, "condition", c.Condition)
		for i, g := range c.Guesses {
			out = append(out, Prose{Table: c.Table, Key: "guesses", Index: i, Text: g})
		}
		add(c.Table, "model_check_waiver", c.ModelCheckWaiver)
		for _, ev := range c.Evidence {
			add(ev.Table, "detail", ev.Detail)
		}
	}
	return out
}
