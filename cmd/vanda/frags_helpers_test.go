package main_test

import (
	"fmt"
	"strings"
)

// 自然言語で書く項目は、API で片（{"text": …} か {"text": …, "ref": <ID>}）の配列になる（docs/plans/stage4-references.md の「API の形」）。

type frag struct {
	Text string  `json:"text"`
	Ref  *string `json:"ref"`
}

type frags []frag

// String は、片の見せる文字をつないだ文を返す。
func (fs frags) String() string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.Text)
	}
	return b.String()
}

// norm は、片の並びを、参照でない片が続く所を 1 つにまとめた形で書く（参照は [文字→ID]）。
// 参照でない文字を何個の片に分けるかは決まっていないので、比べる前にまとめる。
func (fs frags) norm() string {
	var b strings.Builder
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			fmt.Fprintf(&b, "%q", plain.String())
			plain.Reset()
		}
	}
	for _, f := range fs {
		if f.Ref == nil {
			plain.WriteString(f.Text)
			continue
		}
		flush()
		fmt.Fprintf(&b, "[%s→%s]", f.Text, *f.Ref)
	}
	flush()
	return b.String()
}

// textOf は、参照のない文の片の並びを返す（期待値を書くため）。
func textOf(s string) frags { return frags{{Text: s}} }

// fragsOf は、期待値を手で書くためのもの。"文字" は参照でない片、"文字>ID" は参照の片。
func fragsOf(parts ...string) frags {
	var out frags
	for _, p := range parts {
		if text, id, ok := strings.Cut(p, ">"); ok {
			out = append(out, frag{Text: text, Ref: &id})
			continue
		}
		out = append(out, frag{Text: p})
	}
	return out
}

func fragsList(fss []frags) []string {
	out := []string{}
	for _, fs := range fss {
		out = append(out, fs.String())
	}
	return out
}

// refInfo は、応答の最上位の refs の 1 つ。
type refInfo struct {
	Kind    string  `json:"kind"`
	Name    *string `json:"name"`
	Text    string  `json:"text"`
	Current bool    `json:"current"`
}
