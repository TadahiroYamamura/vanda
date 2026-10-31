// Package diag は、利用者に見せるエラーの形を決める。
package diag

import (
	"fmt"
	"strings"
)

// Error は、どのファイルの何行目の、何がいけないか、どう直せばよいかを持つ。
// 行が分からないときは Line を 0 にし、Item（ID と項目）で場所を示す。推測の行は出さない。
type Error struct {
	File string
	Line int
	Item string
	What string
	Fix  string
}

func (e Error) Error() string {
	var b strings.Builder
	b.WriteString(e.File)
	if e.Line > 0 {
		fmt.Fprintf(&b, ":%d", e.Line)
	}
	b.WriteString(": ")
	if e.Line == 0 && e.Item != "" {
		b.WriteString(e.Item)
		b.WriteString(": ")
	}
	b.WriteString(e.What)
	if e.Fix != "" {
		b.WriteString("。")
		b.WriteString(e.Fix)
	}
	return b.String()
}

// Errors は、集めたエラーを 1 件 1 行で出す。
type Errors []Error

func (es Errors) Error() string {
	lines := make([]string, len(es))
	for i, e := range es {
		lines[i] = e.Error()
	}
	return strings.Join(lines, "\n")
}
