// Package annotation は、ファイルのコメントの中のアノテーション（`@vanda:<種類> <ID または理由>`）を読み、
// 付いているものの名前をアダプタで読み取る。
package annotation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
)

// Kinds は、アノテーションの種類。internal のほかは ID を 1 つ取る。
var Kinds = []string{"term", "feature", "claim", "internal", "evidence", "sensor"}

const marker = "@vanda:"

type Annotation struct {
	File  string
	Line  int
	Kind  string
	Value string // ID か、internal なら理由
	Name  string // 付いているものの名前（アダプタが読み取ったもの）
	// Adapter は、読み取ったアダプタ（go・txtar・tla・sql）。外部のコマンドのアダプタなら空
	Adapter string
}

// adapter は、ファイルの中身と、アノテーションのある行を受け取り、行ごとに付いているものの名前を返す。
// comments は、行ごとに、その言語のコメントの中の文だけを返す（コメントの外の @vanda: を数えないため）。
// nil を返せば、すべての行の全体を見る（言語を知らない外部のコマンドのアダプタ）。
type adapter interface {
	comments(file string, src []byte) ([]string, []diag.Error)
	// kinds は、その言語に書けるアノテーションの種類。nil なら、すべて
	kinds() []string
	names(file string, src []byte, lines []int) (map[int]string, []diag.Error)
}

// Read は、files（リポジトリの直下からの場所の一覧）のうち、設定の [[annotation]] のパターンに当たるものを読む。
func Read(root string, files []string, rules []config.AnnotationRule) ([]Annotation, diag.Errors) {
	var out []Annotation
	var errs []diag.Error
	for _, f := range files {
		var hit []config.AnnotationRule
		for _, r := range rules {
			for _, p := range r.Paths {
				if glob.Match(p, f) {
					hit = append(hit, r)
					break
				}
			}
		}
		if len(hit) == 0 {
			continue
		}
		if len(hit) > 1 {
			lines := make([]string, len(hit))
			for i, r := range hit {
				lines[i] = fmt.Sprintf("%d 行目", r.Line)
			}
			errs = append(errs, diag.Error{File: f, What: config.File + " の [[annotation]]（" + strings.Join(lines, "・") + "）の 2 つ以上のパターンに当たります", Fix: "どれか 1 つの [[annotation]] にだけ当たるように、パターンを直してください"})
			continue
		}
		a := adapterFor(root, hit[0])
		if a == nil {
			errs = append(errs, diag.Error{File: f, What: fmt.Sprintf("%s の %d 行目の [[annotation]] のパターンに当たりますが、使うアダプタが決まっていません", config.File, hit[0].Line), Fix: "その [[annotation]] に、adapter か command を書いてください"})
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			errs = append(errs, diag.Error{File: f, What: "ファイルを読めません（" + err.Error() + "）", Fix: "ファイルを読む権限を確かめてください"})
			continue
		}
		texts, cerrs := a.comments(f, src)
		errs = append(errs, cerrs...)
		if len(cerrs) > 0 {
			continue
		}
		if texts == nil {
			texts = strings.Split(string(src), "\n")
		}
		all, ferrs := scan(f, texts)
		errs = append(errs, ferrs...)
		var found []Annotation
		for _, x := range all {
			if ks := a.kinds(); ks != nil && !contains(ks, x.Kind) {
				errs = append(errs, diag.Error{File: f, Line: x.Line, What: "@vanda:" + x.Kind + " は、" + hit[0].Adapter + " のアダプタのファイルには書けません", Fix: "書けるのは " + strings.Join(ks, "・") + " です。種類を直すか、アノテーションを別のファイルに移してください"})
				continue
			}
			found = append(found, x)
		}
		if len(found) == 0 {
			continue
		}
		lines := make([]int, len(found))
		for i, x := range found {
			lines[i] = x.Line
		}
		names, nerrs := a.names(f, src, lines)
		errs = append(errs, nerrs...)
		for _, x := range found {
			if n, ok := names[x.Line]; ok {
				x.Name = n
				x.Adapter = hit[0].Adapter
				out = append(out, x)
			}
		}
	}
	if len(errs) > 0 {
		diag.Sort(errs)
		return out, errs
	}
	return out, nil
}

func adapterFor(root string, r config.AnnotationRule) adapter {
	switch r.Adapter {
	case "go":
		return goAdapter{}
	case "txtar":
		return txtarAdapter{}
	case "tla":
		return tlaAdapter{}
	case "sql":
		return sqlAdapter{}
	}
	if len(r.Command) > 0 {
		return commandAdapter{root: root, command: r.Command}
	}
	return nil
}

// scan は、行ごとの文 texts の中の `@vanda:` を探して、種類と ID（または理由）を読む。
func scan(file string, texts []string) ([]Annotation, []diag.Error) {
	var out []Annotation
	var errs []diag.Error
	for i, line := range texts {
		n := i + 1
		at := strings.Index(line, marker)
		if at < 0 {
			continue
		}
		rest := line[at+len(marker):]
		if strings.Contains(rest, marker) {
			errs = append(errs, diag.Error{File: file, Line: n, What: "1 行に @vanda: が 2 つ以上あります", Fix: "アノテーションを、1 行に 1 つずつ書いてください"})
			continue
		}
		k := 0
		for k < len(rest) && rest[k] >= 'a' && rest[k] <= 'z' {
			k++
		}
		kind, arg := rest[:k], rest[k:]
		glued := arg != "" && arg[0] != ' ' && arg[0] != '\t' && arg[0] != '\r'
		if !contains(Kinds, kind) || glued {
			word := kind
			if f := strings.Fields(arg); glued && len(f) > 0 {
				word += f[0]
			}
			errs = append(errs, diag.Error{File: file, Line: n, What: "@vanda:" + word + " は決まった種類ではありません", Fix: "@vanda: の後を、" + strings.Join(Kinds, "・") + " のどれかにしてください"})
			continue
		}
		arg = trimCloser(strings.TrimSpace(arg))
		if kind == "internal" {
			if arg == "" {
				errs = append(errs, diag.Error{File: file, Line: n, What: "@vanda:internal に理由がありません", Fix: "@vanda:internal <仕様の中だけで使う理由> のように書いてください"})
				continue
			}
			out = append(out, Annotation{File: file, Line: n, Kind: kind, Value: arg})
			continue
		}
		fields := strings.Fields(arg)
		switch len(fields) {
		case 0:
			errs = append(errs, diag.Error{File: file, Line: n, What: "@vanda:" + kind + " に ID がありません", Fix: "@vanda:" + kind + " <ID> のように書いてください"})
			continue
		case 1:
		default:
			errs = append(errs, diag.Error{File: file, Line: n, What: "@vanda:" + kind + " に ID が 2 つ以上あります", Fix: "1 つのアノテーションには ID を 1 つだけ書き、複数の ID は行を分けて書いてください"})
			continue
		}
		out = append(out, Annotation{File: file, Line: n, Kind: kind, Value: fields[0]})
	}
	return out, errs
}

// trimCloser は、アノテーションと同じ行で閉じたブロックのコメントの閉じ（`*)`・`*/`・`-->`）を除く。
func trimCloser(s string) string {
	for _, c := range []string{"*)", "*/", "-->"} {
		if t, ok := strings.CutSuffix(s, c); ok {
			return strings.TrimSpace(t)
		}
	}
	return s
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
