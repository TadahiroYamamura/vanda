// Package annotation は、ファイルのコメントの中のアノテーション（`@vanda:<種類> <ID または理由>`）を読み、
// 付いているものの名前をアダプタで読み取る。
package annotation

import (
	"fmt"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

// AdapterInfo は、組み込みのアノテーションの読み取りのアダプタの、言語の見つけ方と既定のパターン。
// vanda init が、見つかった言語ごとに、読むファイルのパターンを質問するときに使う
type AdapterInfo struct {
	Name            string   // 設定の [[annotation]] の adapter に書く名前
	Title           string   // 質問に出す、言語の名前
	Suffixes        []string // 見つけ方（git が追っているファイルの名前の終わり。どれかで終われば見つかったとする）
	DefaultPatterns []string // 読むファイルのパターンの既定
	// DefaultTypes は、満たすエビデンスのタイプの既定。vanda init が [[annotation]] の types にそのまま書く
	DefaultTypes []string
}

// Builtins は、組み込みのアノテーションの読み取りのアダプタの一覧。アダプタを足したら、ここにも足す
var Builtins = []AdapterInfo{
	{Name: "go", Title: "Go のテスト（*_test.go）", Suffixes: []string{"_test.go"}, DefaultPatterns: []string{"**/*_test.go"}, DefaultTypes: []string{"unit-test", "cross-check", "random-check", "interface-test", "concurrency-test", "sensor"}},
	{Name: "txtar", Title: "シナリオ（*.txtar）", Suffixes: []string{".txtar"}, DefaultPatterns: []string{"**/*.txtar"}, DefaultTypes: []string{"scenario-test"}},
	{Name: "tla", Title: "TLA+（*.tla）", Suffixes: []string{".tla"}, DefaultPatterns: []string{"**/*.tla"}, DefaultTypes: []string{"model-check"}},
	{Name: "sql", Title: "SQL（*.sql）", Suffixes: []string{".sql"}, DefaultPatterns: []string{"**/*.sql"}, DefaultTypes: []string{"guardrail"}},
	{Name: "ts", Title: "TypeScript のテスト（*.test.ts・*.test.tsx）", Suffixes: []string{".test.ts", ".test.tsx"}, DefaultPatterns: []string{"**/*.test.ts", "**/*.test.tsx"}, DefaultTypes: []string{"unit-test"}},
}

// Kinds は、アノテーションの種類。internal のほかは ID を 1 つ取る。
var Kinds = []string{"term", "feature", "claim", "internal", "evidence", "sensor"}

const marker = "@vanda:"

type Annotation struct {
	File  string
	Line  int
	Kind  string
	Value string // ID か、internal なら理由
	Name  string // 付いているものの名前（アダプタが読み取ったもの）
	// Adapter は、読み取ったアダプタ（go・txtar・tla・sql）。カスタムアダプタなら空
	Adapter string
	// Types は、読んだ [[annotation]] の types（このアノテーションが満たせるエビデンスのタイプ）
	Types []string
}

// adapter は、ファイルの中身と、アノテーションのある行を受け取り、行ごとに付いているものの名前を返す。
// comments は、行ごとに、その言語のコメントの中の文だけを返す（コメントの外の @vanda: を数えないため）。
// nil を返せば、すべての行の全体を見る（言語を知らないカスタムアダプタ）。
type adapter interface {
	comments(file string, src []byte) ([]string, []diag.Error)
	// kinds は、その言語に書けるアノテーションの種類。nil なら、すべて
	kinds() []string
	names(file string, src []byte, lines []int) (map[int]string, []diag.Error)
}

// Read は、files（リポジトリの直下からの場所の一覧）のうち、設定の [[annotation]] のパターンにマッチするものを読む。
func Read(root string, files []string, rules []config.AnnotationRule) ([]Annotation, diag.Errors) {
	return ReadWith(root, tomlread.OSReader(root), files, rules)
}

// ReadWith は、Read と同じく読む。ファイルの中身は read で読む。root は、カスタムアダプタを動かすディレクトリ。
func ReadWith(root string, read tomlread.Reader, files []string, rules []config.AnnotationRule) ([]Annotation, diag.Errors) {
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
			errs = append(errs, diag.Error{File: f, What: config.File + " の [[annotation]]（" + strings.Join(lines, "・") + "）の 2 つ以上のパターンにマッチします", Fix: "どれか 1 つの [[annotation]] にだけマッチするように、パターンを直してください"})
			continue
		}
		a := adapterFor(root, hit[0])
		if a == nil {
			errs = append(errs, diag.Error{File: f, What: fmt.Sprintf("%s の %d 行目の [[annotation]] のパターンにマッチしますが、使うアダプタが決まっていません", config.File, hit[0].Line), Fix: "その [[annotation]] に、adapter（組み込みのアダプタか、.vanda/adapters/ に置いたカスタムアダプタの名前）を書いてください"})
			continue
		}
		src, err := read(f)
		if err != nil {
			errs = append(errs, diag.Error{File: f, What: "ファイルを読めません（" + err.Error() + "）", Fix: "ファイルを読む権限を確かめてください"})
			continue
		}
		if v, ok := a.(interface {
			fileErrors(string, []byte) []diag.Error
		}); ok {
			errs = append(errs, v.fileErrors(f, src)...)
		}
		texts, cerrs := a.comments(f, src)
		errs = append(errs, cerrs...)
		if len(cerrs) > 0 {
			continue
		}
		var asked map[int]string
		if ua, ok := a.(userAdapter); ok {
			// カスタムアダプタは、vanda が言語を知らないので、@vanda: のある行を先に渡し、アダプタがアノテーションだと
			// 答えた行だけを読む（コメントの外の @vanda: を、書き方の誤りにしないため）
			rows := strings.Split(string(src), "\n")
			var lines []int
			for i, r := range rows {
				if strings.Contains(dropCodeSpans(r), marker) {
					lines = append(lines, i+1)
				}
			}
			if len(lines) == 0 {
				continue
			}
			names, _, aerrs := ua.ask(f, src, lines)
			errs = append(errs, aerrs...)
			if len(aerrs) > 0 {
				continue
			}
			texts = make([]string, len(rows))
			for l := range names {
				texts[l-1] = rows[l-1]
			}
			asked = names
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
		names, nerrs := asked, []diag.Error(nil)
		if names == nil {
			names, nerrs = a.names(f, src, lines)
		}
		errs = append(errs, nerrs...)
		for _, x := range found {
			if n, ok := names[x.Line]; ok {
				x.Name = n
				x.Adapter = hit[0].Adapter
				x.Types = hit[0].Types
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
	if r.User != nil {
		t := r.Timeout
		if t <= 0 {
			// 設定を読まずに作った規則（timeout を埋めていないもの）は、省いたときと同じに待つ
			t = config.DefaultAdapterTimeout
		}
		return userAdapter{root: root, u: r.User, timeout: t}
	}
	switch r.Adapter {
	case "go":
		return goAdapter{}
	case "txtar":
		return txtarAdapter{}
	case "tla":
		return tlaAdapter{}
	case "sql":
		return sqlAdapter{}
	case "ts":
		return tsAdapter{}
	}
	return nil
}

// scan は、行ごとの文 texts の中の `@vanda:` を探して、種類と ID（または理由）を読む。
func scan(file string, texts []string) ([]Annotation, []diag.Error) {
	var out []Annotation
	var errs []diag.Error
	for i, line := range texts {
		n := i + 1
		line = dropCodeSpans(line)
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

// dropCodeSpans は、バッククォートで囲んだ所（Markdown のコードと同じく、同じ数のバッククォートで開いて閉じる）を空白 1 つに置き換える。
// 説明の文で書き方に触れた `@vanda:term` を、アノテーションとして読まないため。閉じていないバッククォートは、そのまま残す。
func dropCodeSpans(line string) string {
	if !strings.Contains(line, "`") {
		return line
	}
	var b strings.Builder
	i := 0
	for i < len(line) {
		if line[i] != '`' {
			b.WriteByte(line[i])
			i++
			continue
		}
		open := i
		for i < len(line) && line[i] == '`' {
			i++
		}
		run := i - open
		closeAt := -1
		for j := i; j < len(line); {
			if line[j] != '`' {
				j++
				continue
			}
			k := j
			for k < len(line) && line[k] == '`' {
				k++
			}
			if k-j == run {
				closeAt = k
				break
			}
			j = k
		}
		if closeAt < 0 {
			b.WriteString(line[open:i])
			continue
		}
		b.WriteByte(' ')
		i = closeAt
	}
	return b.String()
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
