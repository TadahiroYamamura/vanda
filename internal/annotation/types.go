package annotation

import (
	"go/ast"
	"strings"
)

// TypeDecl は、TLA+ のモジュールで、仕様の型とみなす定義。
type TypeDecl struct {
	Name string
	Line int
}

// TLATypes は、TLA+ の中身 src から、仕様の型とみなす定義を返す。
// 型とみなすのは、TypeOK の中で `\in`・`\subseteq` の右（次の `/\`・`\/` か行の終わりまで）に出てくる識別子のうち、
// 同じモジュールの最上位（行の先頭から書いた）の定義の名前。TypeOK がなければ、何も返さない。
func TLATypes(src []byte) []TypeDecl {
	rows := strings.Split(string(src), "\n")
	code, _ := splitTLA(rows)
	defs := map[string]int{}
	var order []string
	typeOK := -1
	for i, c := range code {
		if c == "" || c[0] == ' ' || c[0] == '\t' {
			continue
		}
		name, ok := tlaDefName(c)
		if !ok {
			continue
		}
		if _, dup := defs[name]; !dup {
			defs[name] = i + 1
			order = append(order, name)
		}
		if name == "TypeOK" && typeOK < 0 {
			typeOK = i
		}
	}
	if typeOK < 0 {
		return nil
	}
	used := map[string]bool{}
	for i := typeOK; i < len(code); i++ {
		c := code[i]
		if i > typeOK && (isTopLevelDef(c) || strings.HasPrefix(strings.TrimSpace(c), "====")) {
			break
		}
		if i == typeOK {
			c = c[strings.Index(c, "==")+2:]
		}
		for _, op := range []string{`\in`, `\subseteq`} {
			for rest := c; ; {
				j := strings.Index(rest, op)
				if j < 0 {
					break
				}
				rest = rest[j+len(op):]
				// \in の後に識別子の文字が続くもの（\intersect など）は、別の演算子
				if rest != "" && isIdent(rest[0]) {
					continue
				}
				rhs := rest
				for _, stop := range []string{`/\`, `\/`} {
					if k := strings.Index(rhs, stop); k >= 0 {
						rhs = rhs[:k]
					}
				}
				for _, id := range identifiers(rhs) {
					if _, ok := defs[id]; ok && id != "TypeOK" {
						used[id] = true
					}
				}
			}
		}
	}
	var out []TypeDecl
	for _, n := range order {
		if used[n] {
			out = append(out, TypeDecl{Name: n, Line: defs[n]})
		}
	}
	return out
}

func isTopLevelDef(c string) bool {
	if c == "" || c[0] == ' ' || c[0] == '\t' {
		return false
	}
	_, ok := tlaDefName(c)
	return ok
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func identifiers(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		if s[i] == '\\' {
			// \X や \union のような演算子は識別子でない
			i++
			for i < len(s) && isIdent(s[i]) {
				i++
			}
			continue
		}
		if !isIdent(s[i]) {
			i++
			continue
		}
		j := i
		for j < len(s) && isIdent(s[j]) {
			j++
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

// TLADefinition は、TLA+ の中身 src の、行 annLine（1 から数える）のアノテーションが付いている定義の文を返す。
// 定義の行から、次の最上位の定義か `====` の前までのうち、終わりの空行とコメントだけの行（次の定義のアノテーションなど）を除いたもの。
func TLADefinition(src []byte, annLine int) (string, bool) {
	rows := strings.Split(string(src), "\n")
	code, _ := splitTLA(rows)
	start := -1
	for j := annLine; j < len(code); j++ {
		if strings.TrimSpace(code[j]) != "" {
			start = j
			break
		}
	}
	if start < 0 {
		return "", false
	}
	if _, ok := tlaDefName(code[start]); !ok {
		return "", false
	}
	end := len(code)
	for j := start + 1; j < len(code); j++ {
		if isTopLevelDef(code[j]) || strings.HasPrefix(strings.TrimSpace(code[j]), "====") {
			end = j
			break
		}
	}
	for end > start+1 && strings.TrimSpace(code[end-1]) == "" {
		end--
	}
	return strings.Join(rows[start:end], "\n"), true
}

// TargetLine は、行 annLine（1 から数える）のアノテーションが付いているものの行を返す。
// go は func の宣言の行、tla は定義の行、sql は文の行、txtar はファイル全体に付くので 1。見つからなければ annLine。
func TargetLine(adapter, file string, src []byte, annLine int) int {
	rows := strings.Split(string(src), "\n")
	switch adapter {
	case "txtar":
		return 1
	case "go":
		fset, f, errs := parseGo(file, src)
		if errs != nil {
			return annLine
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if ok && fd.Doc != nil && fset.Position(fd.Doc.Pos()).Line <= annLine && annLine <= fset.Position(fd.Doc.End()).Line {
				return fset.Position(fd.Pos()).Line
			}
		}
	case "tla":
		code, _ := splitTLA(rows)
		for j := annLine; j < len(code); j++ {
			if strings.TrimSpace(code[j]) != "" {
				return j + 1
			}
		}
	case "sql":
		for j := annLine; j < len(rows); j++ {
			if t := strings.TrimSpace(rows[j]); t != "" && !strings.HasPrefix(t, "--") {
				return j + 1
			}
		}
	}
	return annLine
}
