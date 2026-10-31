package annotation

import (
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
