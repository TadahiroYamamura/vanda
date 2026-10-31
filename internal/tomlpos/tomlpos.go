// Package tomlpos は、TOML の元の文から、表と項目の行を見つける。
//
// BurntSushi/toml は項目の位置を外に出さないので、値はそちらで読み、行だけをここで見つける。
// ここで扱うのは、表の見出し（`[a]`、`[[a.b]]`）と、行の先頭に書いた `key = value` だけ。
// インラインの表（`{ ... }`）の中の項目の行は見つけない。見つからない項目は、呼び出し元が行なしでエラーを出す（推測の行を出さないため）。
package tomlpos

import (
	"strconv"
	"strings"
)

// Lines は、項目の場所から行（1 から数える）への対応を返す。
// 場所は、表の配列の要素を 0 から数えた添字で表す（例: `claim[0]`、`claim[0].evidence[1].layers`、`story.id_pattern`）。
func Lines(src []byte) map[string]int {
	s := &scanner{lines: map[string]int{}, counts: map[string]int{}}
	for i, line := range strings.Split(string(src), "\n") {
		s.line(i+1, strings.TrimSuffix(line, "\r"))
	}
	return s.lines
}

type scanner struct {
	lines   map[string]int
	counts  map[string]int // 表の配列ごとの、これまでの要素の数
	current string
	// 値が複数行にまたがっている間の状態
	multi string // 閉じていない複数行の文字列の区切り（`"""` か `'''`）
	depth int    // 閉じていない `[`・`{` の数
	// 見出しやキーを読めなかった後は、その表の中の行を記録しない（取り違えた行を出さないため）
	lost bool
}

func (s *scanner) line(n int, line string) {
	if s.multi != "" || s.depth > 0 {
		s.value(line)
		return
	}
	t := strings.TrimLeft(line, " \t")
	if t == "" || t[0] == '#' {
		return
	}
	if strings.HasPrefix(t, "[[") {
		segs, ok := parseKey(t[2:], "]]")
		if !ok {
			s.lost = true
			return
		}
		parent := s.resolve(segs[:len(segs)-1])
		arr := join(parent, segs[len(segs)-1])
		idx := s.counts[arr]
		s.counts[arr] = idx + 1
		s.current = arr + "[" + strconv.Itoa(idx) + "]"
		s.lost = false
		s.record(s.current, n)
		return
	}
	if t[0] == '[' {
		segs, ok := parseKey(t[1:], "]")
		if !ok {
			s.lost = true
			return
		}
		s.current = s.resolve(segs)
		s.lost = false
		s.record(s.current, n)
		return
	}
	segs, rest, ok := parseKeyValue(t)
	if !ok {
		// 読めない行の後は、値が何行続くか分からないので、この表の中の行を記録しない
		s.lost = true
		return
	}
	if !s.lost {
		s.record(join(s.current, strings.Join(segs, ".")), n)
	}
	s.value(rest)
}

func (s *scanner) record(key string, n int) {
	if _, ok := s.lines[key]; !ok {
		s.lines[key] = n
	}
}

// resolve は、見出しのキーの各段について、表の配列なら今の最後の要素を指す場所にする。
func (s *scanner) resolve(segs []string) string {
	p := ""
	for _, seg := range segs {
		p = join(p, seg)
		if c, ok := s.counts[p]; ok {
			p = p + "[" + strconv.Itoa(c-1) + "]"
		}
	}
	return p
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

// value は、値の文を読み、複数行の文字列と、閉じていない括弧の数を更新する。
func (s *scanner) value(v string) {
	i := 0
	for i < len(v) {
		if s.multi != "" {
			j := strings.Index(v[i:], s.multi)
			if j < 0 {
				return
			}
			// 区切りの直前の `\` は、基本の文字列では区切りを打ち消す
			if s.multi == `"""` && j > 0 && escaped(v[i:], j) {
				i += j + 1
				continue
			}
			i += j + 3
			for i < len(v) && v[i] == s.multi[0] {
				i++
			}
			s.multi = ""
			continue
		}
		c := v[i]
		switch {
		case c == '#':
			return
		case strings.HasPrefix(v[i:], `"""`), strings.HasPrefix(v[i:], `'''`):
			s.multi = v[i : i+3]
			i += 3
		case c == '"':
			i = skipBasic(v, i+1)
		case c == '\'':
			j := strings.IndexByte(v[i+1:], '\'')
			if j < 0 {
				return
			}
			i += j + 2
		case c == '[' || c == '{':
			s.depth++
			i++
		case c == ']' || c == '}':
			if s.depth > 0 {
				s.depth--
			}
			i++
		default:
			i++
		}
	}
}

func escaped(v string, j int) bool {
	n := 0
	for k := j - 1; k >= 0 && v[k] == '\\'; k-- {
		n++
	}
	return n%2 == 1
}

func skipBasic(v string, i int) int {
	for i < len(v) {
		switch v[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1
		default:
			i++
		}
	}
	return i
}

// parseKey は、`a.b."c"` の形のキーを読み、その後に end が来ることを確かめる。
func parseKey(t, end string) ([]string, bool) {
	segs, rest, ok := keySegs(t)
	if !ok {
		return nil, false
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, end) {
		return nil, false
	}
	return segs, true
}

func parseKeyValue(t string) ([]string, string, bool) {
	segs, rest, ok := keySegs(t)
	if !ok {
		return nil, "", false
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "=") {
		return nil, "", false
	}
	return segs, rest[1:], true
}

func keySegs(t string) ([]string, string, bool) {
	var segs []string
	for {
		t = strings.TrimLeft(t, " \t")
		if t == "" {
			return nil, "", false
		}
		var seg string
		switch t[0] {
		case '"':
			j := skipBasic(t, 1)
			if j > len(t) || t[j-1] != '"' {
				return nil, "", false
			}
			u, err := strconv.Unquote(t[:j])
			if err != nil {
				return nil, "", false
			}
			seg, t = u, t[j:]
		case '\'':
			j := strings.IndexByte(t[1:], '\'')
			if j < 0 {
				return nil, "", false
			}
			seg, t = t[1:j+1], t[j+2:]
		default:
			j := 0
			for j < len(t) && isBare(t[j]) {
				j++
			}
			if j == 0 {
				return nil, "", false
			}
			seg, t = t[:j], t[j:]
		}
		// 場所の書き方と紛れるキーは、行を記録しない
		if strings.ContainsAny(seg, ".[]") {
			return nil, "", false
		}
		segs = append(segs, seg)
		t = strings.TrimLeft(t, " \t")
		if !strings.HasPrefix(t, ".") {
			return segs, t, true
		}
		t = t[1:]
	}
}

func isBare(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}
