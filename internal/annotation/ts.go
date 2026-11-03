package annotation

import (
	"sort"
	"strconv"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/diag"
)

// tsAdapter: アノテーションは、test・it・describe から始まる呼び出しの直前のコメント（空行をはさまない）にだけ書ける。
// 名前は、最後の呼び出しの最初の引数の文字（テンプレート文字列なら書いてあるとおり）。describe の中なら、外側から ` > ` でつなぐ。
//
// TypeScript を読む依存を足さないため、型や構文は読まず、コメント・文字列・テンプレート文字列・正規表現を見分けるだけの字句の読み取りで済ませる。
// JSX の地の文にある引用符（Don't など）は文字列の始まりと見分けられないので、同じ行で閉じない引用符は文字列とみなさない。
// 地の文に、同じ行で対になる引用符や、対にならない括弧があると、読み違えることがある
type tsAdapter struct{}

func (tsAdapter) kinds() []string { return []string{"evidence"} }

type tsTok struct {
	kind byte // 'i' 名前、's' 文字列かテンプレート文字列（text は引用符の中の、書いてあるとおりの文字）、'p' 記号、'o' そのほか
	text string
	line int // 1 から
	off  int
}

type tsLexed struct {
	toks     []tsTok
	comments []string // 行ごとの、コメントの中の文
	code     []bool   // 行ごとに、コメントでない字句が始まるか
}

var tsRegexAfter = map[string]bool{"return": true, "typeof": true, "case": true, "do": true, "else": true, "in": true, "of": true, "new": true, "delete": true, "void": true, "throw": true, "await": true, "yield": true, "instanceof": true}

func lexTS(src string) tsLexed {
	n := strings.Count(src, "\n") + 1
	lx := tsLexed{comments: make([]string, n), code: make([]bool, n)}
	line := 1
	add := func(t tsTok) {
		lx.toks = append(lx.toks, t)
		lx.code[t.line-1] = true
	}
	regexOK := func() bool {
		if len(lx.toks) == 0 {
			return true
		}
		t := lx.toks[len(lx.toks)-1]
		switch t.kind {
		case 'p':
			// < の後の / は、正規表現でなく JSX の閉じのタグ（</p>）とみなす
			return strings.Contains("(,=:[!&|?{};+-*%>~^", t.text)
		case 'i':
			return tsRegexAfter[t.text]
		}
		return false
	}
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case strings.HasPrefix(src[i:], "//"):
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				end = len(src) - i
			}
			lx.comments[line-1] += " " + src[i+2:i+end]
			i += end
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			body := src[i+2:]
			next := len(src)
			if end >= 0 {
				body = src[i+2 : i+2+end]
				next = i + 2 + end + 2
			}
			for k, part := range strings.Split(body, "\n") {
				lx.comments[line-1+k] += " " + part
			}
			line += strings.Count(body, "\n")
			i = next
		case c == '\'' || c == '"':
			start := i
			j := i + 1
			var b strings.Builder
			for j < len(src) && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' && j+1 < len(src) && src[j+1] != '\n' {
					b.WriteByte(src[j+1])
					j += 2
					continue
				}
				b.WriteByte(src[j])
				j++
			}
			if j >= len(src) || src[j] != c {
				// 同じ行で閉じない引用符は、JSX の地の文の ' などとみなし、記号として読み進める
				add(tsTok{kind: 'p', text: string(c), line: line, off: start})
				i++
				continue
			}
			add(tsTok{kind: 's', text: b.String(), line: line, off: start})
			i = j + 1
		case c == '`':
			start, startLine := i, line
			j := skipTemplate(src, i+1)
			body := src[i+1 : max(i+1, j-1)]
			add(tsTok{kind: 's', text: body, line: startLine, off: start})
			line += strings.Count(src[i:j], "\n")
			i = j
		case c == '/' && regexOK():
			j := i + 1
			inClass, closed := false, false
			for j < len(src) && src[j] != '\n' {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '[' {
					inClass = true
				} else if src[j] == ']' {
					inClass = false
				} else if src[j] == '/' && !inClass {
					j++
					closed = true
					break
				}
				j++
			}
			if !closed {
				// 同じ行で閉じないものは、JSX の閉じのタグ（</p>）などとみなし、記号として読み進める
				add(tsTok{kind: 'p', text: "/", line: line, off: i})
				i++
				continue
			}
			for j < len(src) && isTSIdent(src[j]) {
				j++
			}
			add(tsTok{kind: 'o', text: src[i:min(j, len(src))], line: line, off: i})
			i = min(j, len(src))
		case isTSIdentStart(c):
			j := i
			for j < len(src) && isTSIdent(src[j]) {
				j++
			}
			add(tsTok{kind: 'i', text: src[i:j], line: line, off: i})
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && (isTSIdent(src[j]) || src[j] == '.') {
				j++
			}
			add(tsTok{kind: 'o', text: src[i:j], line: line, off: i})
			i = j
		default:
			add(tsTok{kind: 'p', text: string(c), line: line, off: i})
			i++
		}
	}
	return lx
}

// skipTemplate は、テンプレート文字列の中（開きの逆引用符の次の i から）を読み、閉じの逆引用符の次の位置を返す。
func skipTemplate(src string, i int) int {
	for i < len(src) {
		switch {
		case src[i] == '\\':
			i += 2
		case src[i] == '`':
			return i + 1
		case strings.HasPrefix(src[i:], "${"):
			i = skipBraces(src, i+2)
		default:
			i++
		}
	}
	return len(src)
}

// skipBraces は、${ の中のコード（i から）を読み、対になる } の次の位置を返す。中の文字列とテンプレート文字列も読み飛ばす。
func skipBraces(src string, i int) int {
	depth := 1
	for i < len(src) {
		switch c := src[i]; {
		case c == '{':
			depth++
			i++
		case c == '}':
			depth--
			i++
			if depth == 0 {
				return i
			}
		case c == '`':
			i = skipTemplate(src, i+1)
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(src) && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			i = j + 1
		default:
			i++
		}
	}
	return len(src)
}

func isTSIdentStart(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isTSIdent(c byte) bool { return isTSIdentStart(c) || c >= '0' && c <= '9' }

func (tsAdapter) comments(_ string, src []byte) ([]string, []diag.Error) {
	return lexTS(string(src)).comments, nil
}

// tsCall は、test・it・describe から始まる呼び出しのつながり（例: test.describe.serial("名前", …)、it.each(表)("名前", …)）。
type tsCall struct {
	root     tsTok
	members  []string
	name     string
	named    bool // 最後の呼び出しの最初の引数が、決まった文字か
	open     int  // 最後の呼び出しの開き括弧の位置
	close    int  // 対になる閉じ括弧の位置
	describe bool
	hook     bool
	skipped  bool // .skip か .todo が付いている（実行しない）
	only     bool // .only が付いている
}

var tsHooks = map[string]bool{"beforeEach": true, "afterEach": true, "beforeAll": true, "afterAll": true, "step": true}

func tsCalls(toks []tsTok) []tsCall {
	match := map[int]int{}
	var stack []int
	for k, t := range toks {
		if t.kind != 'p' {
			continue
		}
		switch t.text {
		case "(":
			stack = append(stack, k)
		case ")":
			if len(stack) > 0 {
				match[stack[len(stack)-1]] = k
				stack = stack[:len(stack)-1]
			}
		}
	}
	var out []tsCall
	for j, t := range toks {
		if t.kind != 'i' || (t.text != "test" && t.text != "it" && t.text != "describe") {
			continue
		}
		if j > 0 && toks[j-1].kind == 'p' && toks[j-1].text == "." {
			continue
		}
		c := tsCall{root: t, members: []string{t.text}, open: -1}
		k := j + 1
		for k < len(toks) {
			if toks[k].kind == 'p' && toks[k].text == "." && k+1 < len(toks) && toks[k+1].kind == 'i' {
				c.members = append(c.members, toks[k+1].text)
				k += 2
				continue
			}
			if toks[k].kind == 'p' && toks[k].text == "(" {
				end, ok := match[k]
				if !ok {
					break
				}
				c.open, c.close = k, end
				k = end + 1
				continue
			}
			break
		}
		if c.open < 0 {
			continue
		}
		if a := c.open + 1; a+1 < len(toks) && toks[a].kind == 's' && toks[a+1].kind == 'p' && (toks[a+1].text == "," || toks[a+1].text == ")") {
			c.name, c.named = toks[a].text, true
		}
		c.describe = t.text == "describe" || len(c.members) > 1 && c.members[1] == "describe"
		for _, m := range c.members[1:] {
			c.hook = c.hook || tsHooks[m]
			c.skipped = c.skipped || m == "skip" || m == "todo"
			c.only = c.only || m == "only"
		}
		out = append(out, c)
	}
	return out
}

// tsTarget は、annLine（1 から）のアノテーションが付く、次のコードの行を返す。空行をはさむか、アノテーションの行にコードがあれば 0。
func tsTarget(lx tsLexed, rows []string, annLine int) int {
	if lx.code[annLine-1] {
		return 0
	}
	for m := annLine + 1; m <= len(rows); m++ {
		if lx.code[m-1] {
			return m
		}
		if strings.TrimSpace(rows[m-1]) == "" {
			return 0
		}
	}
	return 0
}

func (tsAdapter) names(file string, src []byte, lines []int) (map[int]string, []diag.Error) {
	lx := lexTS(string(src))
	rows := strings.Split(string(src), "\n")
	calls := tsCalls(lx.toks)
	byLine := map[int]*tsCall{}
	for k := range calls {
		if _, ok := byLine[calls[k].root.line]; !ok {
			byLine[calls[k].root.line] = &calls[k]
		}
	}
	firstOnLine := map[int]int{}
	for _, t := range lx.toks {
		if _, ok := firstOnLine[t.line]; !ok {
			firstOnLine[t.line] = t.off
		}
	}
	out := map[int]string{}
	var errs []diag.Error
	for _, l := range lines {
		m := tsTarget(lx, rows, l)
		c := byLine[m]
		if m == 0 || c == nil || firstOnLine[m] != c.root.off {
			errs = append(errs, diag.Error{File: file, Line: l, What: "アノテーションが、test・it・describe の呼び出しの直前のコメントにありません", Fix: "付けたい test・it・describe の呼び出しのすぐ上のコメント（空行をはさまない）に移してください"})
			continue
		}
		if c.hook {
			errs = append(errs, diag.Error{File: file, Line: l, What: strings.Join(c.members, ".") + " の直前には、アノテーションを書けません", Fix: "付けたい test・it・describe の呼び出しのすぐ上のコメントに移してください"})
			continue
		}
		var outer []*tsCall
		for k := range calls {
			d := &calls[k]
			if d.describe && d.open >= 0 && lx.toks[d.open].off < c.root.off && c.root.off < lx.toks[d.close].off {
				outer = append(outer, d)
			}
		}
		// 実行しないテストは何も確かめていないので、誤りにはせず、アノテーションとして返さない（エビデンスに数えない）
		skipped := c.skipped
		for _, d := range outer {
			skipped = skipped || d.skipped
		}
		if skipped {
			continue
		}
		if !c.named {
			errs = append(errs, diag.Error{File: file, Line: l, What: strings.Join(c.members, ".") + " の名前（最初の引数）が、文字列かテンプレート文字列で書いた決まった文字ではありません", Fix: "名前を、文字列で直接書いてください"})
			continue
		}
		sort.Slice(outer, func(a, b int) bool { return outer[a].open < outer[b].open })
		parts := []string{}
		ok := true
		for _, d := range outer {
			if !d.named {
				errs = append(errs, diag.Error{File: file, Line: l, What: "外側の " + strings.Join(d.members, ".") + "（" + strconv.Itoa(d.root.line) + " 行目）の名前が、文字列かテンプレート文字列で書いた決まった文字ではありません", Fix: "describe の名前を、文字列で直接書いてください"})
				ok = false
				break
			}
			parts = append(parts, d.name)
		}
		if !ok {
			continue
		}
		out[l] = strings.Join(append(parts, c.name), " > ")
	}
	return out, errs
}

// tsTargetLine は、アノテーションが付いている呼び出しの行を返す。見つからなければ annLine。
func tsTargetLine(src []byte, annLine int) int {
	lx := lexTS(string(src))
	if m := tsTarget(lx, strings.Split(string(src), "\n"), annLine); m > 0 {
		return m
	}
	return annLine
}

// fileErrors は、アノテーションの有無によらず、ファイルの .only を誤りにする（同じファイルのほかのテストを黙って実行しなくするため）。
func (tsAdapter) fileErrors(file string, src []byte) []diag.Error {
	var errs []diag.Error
	for _, c := range tsCalls(lexTS(string(src)).toks) {
		if c.only {
			errs = append(errs, diag.Error{File: file, Line: c.root.line, What: strings.Join(c.members, ".") + " があります（.only は、同じファイルのほかのテストを実行しなくします）", Fix: ".only を外してください"})
		}
	}
	return errs
}
