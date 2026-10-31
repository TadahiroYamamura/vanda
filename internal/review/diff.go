package review

import (
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// DiffPart は、文字の単位の差分の 1 つ。
type DiffPart struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// DiffLine は、行の単位の差分の 1 行。
type DiffLine struct {
	Op        string     `json:"op"`
	Before    *int       `json:"before"`
	After     *int       `json:"after"`
	Text      string     `json:"text"`
	Parts     []DiffPart `json:"parts"`
	NoNewline bool       `json:"noNewline,omitempty"`
}

type FileDiff struct {
	Lines []DiffLine `json:"lines"`
}

type ItemDiff struct {
	Fields map[string][]DiffPart `json:"fields"`
	// Evidence は、約束のエビデンスの予定の、エビデンスの単位の差分。約束でなければ nil
	Evidence *EvidenceDiff `json:"evidence,omitempty"`
}

// EvidenceDiff は、変更前と変更後のエビデンスの予定の、1 つずつの印。添字は before・after の evidence の添字にそろう。
type EvidenceDiff struct {
	Before []EvidenceMark `json:"before"`
	After  []EvidenceMark `json:"after"`
}

// EvidenceMark は、エビデンス 1 つの印。op は、変更前では equal・modified・removed、変更後では equal・modified・added。
// modified のときだけ、layers（レイヤーを「・」でつないだ文字）と detail（詳細）の文字の単位の差分を持つ
type EvidenceMark struct {
	Op string `json:"op"`
	// Pair は、対応したもう片方の側のエビデンスの添字（before の印なら after の添字）。対応がなければ nil
	Pair   *int       `json:"pair"`
	Layers []DiffPart `json:"layers"`
	Detail []DiffPart `json:"detail"`
}

var opName = map[diffmatchpatch.Operation]string{
	diffmatchpatch.DiffEqual:  "equal",
	diffmatchpatch.DiffDelete: "delete",
	diffmatchpatch.DiffInsert: "insert",
}

// charDiff は、文字の単位の差分。細切れの一致が多いと、日本語でどこが変わったかを読みにくいので、DiffCleanupSemantic でまとめる。
func charDiff(a, b string) []DiffPart {
	dmp := diffmatchpatch.New()
	ds := dmp.DiffCleanupSemantic(dmp.DiffMain(a, b, false))
	out := []DiffPart{}
	for _, d := range ds {
		if d.Text != "" {
			out = append(out, DiffPart{Op: opName[d.Type], Text: d.Text})
		}
	}
	return out
}

type rawLine struct {
	text      string
	noNewline bool
}

// splitLines は、行の終わりの改行で分ける。最後の行に改行がなければ、その行に印を付ける。
func splitLines(s string) []rawLine {
	var out []rawLine
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, rawLine{text: s, noNewline: true})
			break
		}
		out = append(out, rawLine{text: s[:i]})
		s = s[i+1:]
	}
	return out
}

// fileDiff は、行の単位の差分を作り、書き換えた所（消した行のすぐ後に足した行が続く所）は、行の中を文字の単位で比べる。
func fileDiff(before, after string) FileDiff {
	dmp := diffmatchpatch.New()
	a, b, lines := dmp.DiffLinesToChars(before, after)
	ds := dmp.DiffCharsToLines(dmp.DiffMain(a, b, false), lines)
	out := FileDiff{Lines: []DiffLine{}}
	bn, an := 0, 0
	var dels, ins []string
	flush := func() {
		if len(dels) == 0 && len(ins) == 0 {
			return
		}
		delText, insText := strings.Join(dels, ""), strings.Join(ins, "")
		var delParts, insParts [][]DiffPart
		if len(dels) > 0 && len(ins) > 0 {
			parts := charDiff(delText, insText)
			delParts = partsByLine(parts, "insert")
			insParts = partsByLine(parts, "delete")
		}
		for i, l := range splitLines(delText) {
			bn++
			dl := DiffLine{Op: "delete", Before: ptr(bn), Text: l.text, NoNewline: l.noNewline}
			if delParts != nil {
				dl.Parts = delParts[i]
			}
			out.Lines = append(out.Lines, dl)
		}
		for i, l := range splitLines(insText) {
			an++
			dl := DiffLine{Op: "insert", After: ptr(an), Text: l.text, NoNewline: l.noNewline}
			if insParts != nil {
				dl.Parts = insParts[i]
			}
			out.Lines = append(out.Lines, dl)
		}
		dels, ins = nil, nil
	}
	for _, d := range ds {
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			dels = append(dels, d.Text)
		case diffmatchpatch.DiffInsert:
			ins = append(ins, d.Text)
		default:
			flush()
			for _, l := range splitLines(d.Text) {
				bn++
				an++
				out.Lines = append(out.Lines, DiffLine{Op: "equal", Before: ptr(bn), After: ptr(an), Text: l.text, NoNewline: l.noNewline})
			}
		}
	}
	flush()
	return out
}

// partsByLine は、文字の単位の差分から、片側（skip でない側）の部分を、行ごとに分けて返す。行の終わりの改行は含めない。
func partsByLine(parts []DiffPart, skip string) [][]DiffPart {
	out := [][]DiffPart{{}}
	for _, p := range parts {
		if p.Op == skip {
			continue
		}
		for i, seg := range strings.Split(p.Text, "\n") {
			if i > 0 {
				out = append(out, []DiffPart{})
			}
			if seg != "" {
				out[len(out)-1] = append(out[len(out)-1], DiffPart{Op: p.Op, Text: seg})
			}
		}
	}
	return out
}

// itemDiff は、約束・用語・機能の文字の項目ごとの差分。両方に書いてある項目だけを入れる。
func itemDiff(before, after any) ItemDiff {
	fields := map[string][]DiffPart{}
	add := func(name string, a, b *string) {
		if a != nil && b != nil {
			fields[name] = charDiff(*a, *b)
		}
	}
	out := ItemDiff{Fields: fields}
	switch b := before.(type) {
	case ClaimContent:
		a, _ := after.(ClaimContent)
		add("statement", &b.Statement, &a.Statement)
		add("rationale", b.Rationale, a.Rationale)
		add("condition", b.Condition, a.Condition)
		out.Evidence = evidenceDiff(b.Evidence, a.Evidence)
	case TermContent:
		a, _ := after.(TermContent)
		add("name", &b.Name, &a.Name)
		add("definition", &b.Definition, &a.Definition)
	case FeatureContent:
		a, _ := after.(FeatureContent)
		add("name", &b.Name, &a.Name)
		add("definition", &b.Definition, &a.Definition)
		add("formalWaiver", b.FormalWaiver, a.FormalWaiver)
	}
	return out
}

// evidenceDiff は、変更前と変更後のエビデンスの予定を対応づけ、1 つずつに印を付ける。
// ステージとタイプが同じものを同じエビデンスとみなす。同じステージとタイプが 2 つ以上あれば、それぞれ現れた順に、
// 1 つ目どうし、2 つ目どうし、と対応させ、余ったものは消した（変更前）か足した（変更後）とする。
// 対応したものは、レイヤーと詳細が同じなら equal、どちらかが違えば modified
func evidenceDiff(before, after []EvidenceContent) *EvidenceDiff {
	key := func(e EvidenceContent) string { return e.Stage + "\x00" + e.Type }
	d := &EvidenceDiff{Before: make([]EvidenceMark, len(before)), After: make([]EvidenceMark, len(after))}
	for i := range d.Before {
		d.Before[i] = EvidenceMark{Op: "removed"}
	}
	for i := range d.After {
		d.After[i] = EvidenceMark{Op: "added"}
	}
	queue := map[string][]int{}
	for i, e := range before {
		queue[key(e)] = append(queue[key(e)], i)
	}
	text := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	for j, a := range after {
		q := queue[key(a)]
		if len(q) == 0 {
			continue
		}
		i := q[0]
		queue[key(a)] = q[1:]
		b := before[i]
		bl, al := strings.Join(b.Layers, "・"), strings.Join(a.Layers, "・")
		bd, ad := text(b.Detail), text(a.Detail)
		if bl == al && bd == ad {
			d.Before[i] = EvidenceMark{Op: "equal", Pair: ptr(j)}
			d.After[j] = EvidenceMark{Op: "equal", Pair: ptr(i)}
			continue
		}
		layers, detail := charDiff(bl, al), charDiff(bd, ad)
		d.Before[i] = EvidenceMark{Op: "modified", Pair: ptr(j), Layers: layers, Detail: detail}
		d.After[j] = EvidenceMark{Op: "modified", Pair: ptr(i), Layers: layers, Detail: detail}
	}
	return d
}
