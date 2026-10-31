package annotation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os/exec"
	"path"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/diag"
)

// goAdapter: アノテーションは、func の宣言の直前のコメント（doc comment）にだけ書ける。名前は func の名前（メソッドなら `T.M`）。
type goAdapter struct{}

func (goAdapter) names(file string, src []byte, lines []int) (map[int]string, []diag.Error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
	if err != nil {
		var list scanner.ErrorList
		if errors.As(err, &list) && len(list) > 0 {
			return nil, []diag.Error{{File: file, Line: list[0].Pos.Line, What: "Go として読めない（" + list[0].Msg + "）", Fix: "Go の書き方に合わせて直す"}}
		}
		return nil, []diag.Error{{File: file, What: "Go として読めない（" + err.Error() + "）", Fix: "Go の書き方に合わせて直す"}}
	}
	byLine := map[int]string{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Doc == nil {
			continue
		}
		name := fd.Name.Name
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			if r := recvName(fd.Recv.List[0].Type); r != "" {
				name = r + "." + name
			}
		}
		for l := fset.Position(fd.Doc.Pos()).Line; l <= fset.Position(fd.Doc.End()).Line; l++ {
			byLine[l] = name
		}
	}
	out := map[int]string{}
	var errs []diag.Error
	for _, l := range lines {
		n, ok := byLine[l]
		if !ok {
			errs = append(errs, diag.Error{File: file, Line: l, What: "アノテーションが func の宣言の直前のコメントにない", Fix: "付けたい func の宣言のすぐ上のコメント（空行をはさまない）に移す"})
			continue
		}
		out[l] = n
	}
	return out, errs
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	case *ast.ParenExpr:
		return recvName(t.X)
	}
	return ""
}

// txtarAdapter: アノテーションは、最初のファイルの区切り（`-- 名前 --`）より前の注釈にだけ書け、ファイル全体に付く。
// 名前はファイルの名前から `.txtar` を除いたもの（testscript のサブテストの名前に合わせる）。
type txtarAdapter struct{}

func (txtarAdapter) names(file string, src []byte, lines []int) (map[int]string, []diag.Error) {
	first := 0
	for i, line := range strings.Split(string(src), "\n") {
		if isTxtarMarker(strings.TrimSuffix(line, "\r")) {
			first = i + 1
			break
		}
	}
	name := strings.TrimSuffix(path.Base(file), ".txtar")
	out := map[int]string{}
	var errs []diag.Error
	for _, l := range lines {
		if first > 0 && l >= first {
			errs = append(errs, diag.Error{File: file, Line: l, What: fmt.Sprintf("アノテーションが、最初のファイルの区切り（%d 行目）より後にある", first), Fix: "アノテーションを、ファイルの冒頭の注釈（最初の -- 名前 -- より前）に移す"})
			continue
		}
		out[l] = name
	}
	return out, errs
}

// isTxtarMarker は、golang.org/x/tools/txtar と同じく、`-- ` で始まり ` --` で終わり、間に名前のある行かを返す。
func isTxtarMarker(line string) bool {
	if !strings.HasPrefix(line, "-- ") || !strings.HasSuffix(line, " --") || len(line) < 6 {
		return false
	}
	return strings.TrimSpace(line[3:len(line)-3]) != ""
}

// tlaAdapter: アノテーションは、コメント（`\*`、`(* *)`）の中に書く。付くのは、その後の、空行でもコメントでもない最初の行の定義
// （`Name ==`、`Name(…) ==`。`LOCAL` が付いてもよい）。
type tlaAdapter struct{}

func (tlaAdapter) names(file string, src []byte, lines []int) (map[int]string, []diag.Error) {
	rows := strings.Split(string(src), "\n")
	code, comment := splitTLA(rows)
	out := map[int]string{}
	var errs []diag.Error
	for _, l := range lines {
		at := strings.Index(rows[l-1], marker)
		if at < 0 || !comment[l-1][at] {
			errs = append(errs, diag.Error{File: file, Line: l, What: "アノテーションがコメントの中にない", Fix: `\* か (* *) のコメントの中に書く`})
			continue
		}
		next := 0
		for j := l; j < len(code); j++ {
			if strings.TrimSpace(code[j]) != "" {
				next = j + 1
				break
			}
		}
		if next == 0 {
			errs = append(errs, diag.Error{File: file, Line: l, What: "アノテーションの後に定義がない", Fix: "アノテーションを、付けたい定義のすぐ上に書く"})
			continue
		}
		name, ok := tlaDefName(code[next-1])
		if !ok {
			errs = append(errs, diag.Error{File: file, Line: l, What: fmt.Sprintf("アノテーションの次の行（%d 行目）が、Name == か Name(…) == の定義でない", next), Fix: "アノテーションを、付けたい定義のすぐ上に書く"})
			continue
		}
		out[l] = name
	}
	return out, errs
}

// splitTLA は、行ごとに、コメントを空白にした文と、各バイトがコメントの中かを返す。ブロックのコメントは入れ子にできる。
func splitTLA(rows []string) ([]string, [][]bool) {
	code := make([]string, len(rows))
	mask := make([][]bool, len(rows))
	depth := 0
	for r, line := range rows {
		b := []byte(line)
		m := make([]bool, len(b))
		inStr := false
		for i := 0; i < len(b); i++ {
			switch {
			case depth > 0:
				m[i] = true
				if strings.HasPrefix(line[i:], "(*") {
					depth++
					m[i+1] = true
					i++
				} else if strings.HasPrefix(line[i:], "*)") {
					depth--
					m[i+1] = true
					i++
				}
			case inStr:
				if b[i] == '\\' && i+1 < len(b) {
					i++
				} else if b[i] == '"' {
					inStr = false
				}
			case b[i] == '"':
				inStr = true
			case strings.HasPrefix(line[i:], `\*`):
				for k := i; k < len(b); k++ {
					m[k] = true
				}
				i = len(b)
			case strings.HasPrefix(line[i:], "(*"):
				depth++
				m[i], m[i+1] = true, true
				i++
			}
		}
		c := make([]byte, len(b))
		for i := range b {
			if m[i] {
				c[i] = ' '
			} else {
				c[i] = b[i]
			}
		}
		code[r] = string(c)
		mask[r] = m
	}
	return code, mask
}

func tlaDefName(code string) (string, bool) {
	s := strings.TrimSpace(code)
	if t, ok := strings.CutPrefix(s, "LOCAL"); ok && t != "" && (t[0] == ' ' || t[0] == '\t') {
		s = strings.TrimSpace(t)
	}
	i := 0
	for i < len(s) && (s[i] == '_' || s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z' || s[i] >= '0' && s[i] <= '9') {
		i++
	}
	name := s[:i]
	if !strings.ContainsAny(name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return "", false
	}
	rest := strings.TrimLeft(s[i:], " \t")
	if strings.HasPrefix(rest, "(") {
		depth := 0
		j := 0
		for ; j < len(rest); j++ {
			if rest[j] == '(' {
				depth++
			} else if rest[j] == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if depth != 0 {
			return "", false
		}
		rest = strings.TrimLeft(rest[j+1:], " \t")
	}
	if !strings.HasPrefix(rest, "==") {
		return "", false
	}
	return name, true
}

// sqlAdapter: アノテーションは `--` のコメントの中に書く。名前は、その後の、空行でもコメントでもない最初の行の中身をそのまま使う。
type sqlAdapter struct{}

func (sqlAdapter) names(file string, src []byte, lines []int) (map[int]string, []diag.Error) {
	rows := strings.Split(string(src), "\n")
	out := map[int]string{}
	var errs []diag.Error
	for _, l := range lines {
		line := rows[l-1]
		dash := strings.Index(line, "--")
		if dash < 0 || dash > strings.Index(line, marker) {
			errs = append(errs, diag.Error{File: file, Line: l, What: "アノテーションが -- のコメントの中にない", Fix: "-- の後に書く"})
			continue
		}
		name := ""
		for j := l; j < len(rows); j++ {
			t := strings.TrimSpace(rows[j])
			if t != "" && !strings.HasPrefix(t, "--") {
				name = t
				break
			}
		}
		if name == "" {
			errs = append(errs, diag.Error{File: file, Line: l, What: "アノテーションの後に、コメントでない行がない", Fix: "アノテーションを、付けたい定義のすぐ上に書く"})
			continue
		}
		out[l] = name
	}
	return out, errs
}

// commandAdapter は、外部のコマンドのアダプタ。受け渡しの形は docs/formats.md の「外部のコマンドのアダプタ」。
type commandAdapter struct {
	root    string
	command []string
}

type adapterRequest struct {
	Path        string        `json:"path"`
	Annotations []requestLine `json:"annotations"`
}

type requestLine struct {
	Line int `json:"line"`
}

type adapterResponse struct {
	Names []responseName `json:"names"`
}

type responseName struct {
	Line int    `json:"line"`
	Name string `json:"name"`
}

func (c commandAdapter) names(file string, _ []byte, lines []int) (map[int]string, []diag.Error) {
	req := adapterRequest{Path: file}
	for _, l := range lines {
		req.Annotations = append(req.Annotations, requestLine{Line: l})
	}
	in, err := json.Marshal(req)
	if err != nil {
		return nil, []diag.Error{{File: file, What: "アダプタに渡す JSON を作れない（" + err.Error() + "）", Fix: "vanda の不具合なので、知らせる"}}
	}
	cmdline := strings.Join(c.command, " ")
	cmd := exec.Command(c.command[0], c.command[1:]...)
	cmd.Dir = c.root
	cmd.Stdin = bytes.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, []diag.Error{{File: file, What: "アノテーションの読み取りのアダプタ（" + cmdline + "）が失敗した（" + msg + "）", Fix: "アダプタのエラーに従って直す"}}
	}
	dec := json.NewDecoder(&stdout)
	dec.DisallowUnknownFields()
	var res adapterResponse
	if err := dec.Decode(&res); err != nil {
		return nil, []diag.Error{{File: file, What: "アノテーションの読み取りのアダプタ（" + cmdline + "）の出力が、決まった形の JSON でない（" + err.Error() + "）", Fix: `アダプタが {"names": [{"line": 行, "name": "名前"}]} を出すように直す`}}
	}
	asked := map[int]bool{}
	for _, l := range lines {
		asked[l] = true
	}
	out := map[int]string{}
	var errs []diag.Error
	for _, n := range res.Names {
		switch {
		case !asked[n.Line]:
			errs = append(errs, diag.Error{File: file, What: fmt.Sprintf("アダプタ（%s）が、アノテーションのない %d 行目の名前を返した", cmdline, n.Line), Fix: "渡された行の名前だけを返すように、アダプタを直す"})
		case n.Name == "":
			errs = append(errs, diag.Error{File: file, Line: n.Line, What: "アダプタ（" + cmdline + "）が空の名前を返した", Fix: "名前を返すように、アダプタを直す"})
		case out[n.Line] != "":
			errs = append(errs, diag.Error{File: file, Line: n.Line, What: "アダプタ（" + cmdline + "）が同じ行の名前を 2 つ以上返した", Fix: "1 行に 1 つの名前を返すように、アダプタを直す"})
		default:
			out[n.Line] = n.Name
		}
	}
	for _, l := range lines {
		if _, ok := out[l]; !ok {
			errs = append(errs, diag.Error{File: file, Line: l, What: "アダプタ（" + cmdline + "）が、このアノテーションの名前を返さなかった", Fix: "渡された行のすべてに名前を返すように、アダプタを直す"})
		}
	}
	return out, errs
}
