package main_test

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TLC が -simulate file= で書き出すトレースの値（クロスチェックのオラクルの出力）を読む。
// 読むのは、Agreement のモデルの変数に出てくる形だけ: 表 [a |-> v]、関数 (k :> v @@ …)、集合 {…}、組 <<…>>、
// 文字列、整数、TRUE・FALSE、モデルの値（i1 などの裸の名前）。

// tlaModel は、モデルの値（定数の集合の要素）。
type tlaModel string

// tlaSet と tlaTuple は、集合と組。集合の要素の順は、TLC が書いた順のまま持つ。
type (
	tlaSet   []any
	tlaTuple []any
)

// tlaRecord と tlaFunc は、表と関数。関数の鍵は、モデルの値か文字列か整数を、文字列にしたもの。
type (
	tlaRecord map[string]any
	tlaFunc   map[string]any
)

type tlaReader struct {
	src string
	pos int
}

func parseTLA(src string) (any, error) {
	r := &tlaReader{src: src}
	v, err := r.value()
	if err != nil {
		return nil, err
	}
	r.space()
	if r.pos != len(r.src) {
		return nil, r.errorf("値の後に余りがある")
	}
	return v, nil
}

func (r *tlaReader) errorf(format string, args ...any) error {
	return fmt.Errorf("TLA+ の値の %d 文字目: %s: %q", r.pos, fmt.Sprintf(format, args...), r.src)
}

func (r *tlaReader) space() {
	for r.pos < len(r.src) && strings.ContainsRune(" \t\r\n", rune(r.src[r.pos])) {
		r.pos++
	}
}

func (r *tlaReader) take(tok string) bool {
	r.space()
	if strings.HasPrefix(r.src[r.pos:], tok) {
		r.pos += len(tok)
		return true
	}
	return false
}

func (r *tlaReader) ident() string {
	r.space()
	start := r.pos
	for r.pos < len(r.src) {
		c := r.src[r.pos]
		if c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			r.pos++
			continue
		}
		break
	}
	return r.src[start:r.pos]
}

// list は、open と close のあいだの、カンマで区切った値を読む。
func (r *tlaReader) list(close string) ([]any, error) {
	var out []any
	if r.take(close) {
		return out, nil
	}
	for {
		v, err := r.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		if r.take(close) {
			return out, nil
		}
		if !r.take(",") {
			return nil, r.errorf("%s かカンマがない", close)
		}
	}
}

func keyString(v any) (string, error) {
	switch k := v.(type) {
	case tlaModel:
		return string(k), nil
	case string:
		return k, nil
	case int:
		return strconv.Itoa(k), nil
	}
	return "", fmt.Errorf("関数の鍵にできない値 %v", v)
}

func (r *tlaReader) value() (any, error) {
	r.space()
	if r.pos >= len(r.src) {
		return nil, r.errorf("値がない")
	}
	switch {
	case r.take("<<"):
		vs, err := r.list(">>")
		return tlaTuple(vs), err
	case r.take("{"):
		vs, err := r.list("}")
		return tlaSet(vs), err
	case r.take("["):
		rec := tlaRecord{}
		if r.take("]") {
			return rec, nil
		}
		for {
			name := r.ident()
			if name == "" || !r.take("|->") {
				return nil, r.errorf("表の項目の名前と |-> がない")
			}
			v, err := r.value()
			if err != nil {
				return nil, err
			}
			rec[name] = v
			if r.take("]") {
				return rec, nil
			}
			if !r.take(",") {
				return nil, r.errorf("] かカンマがない")
			}
		}
	case r.take("("):
		fn := tlaFunc{}
		for {
			k, err := r.value()
			if err != nil {
				return nil, err
			}
			if !r.take(":>") {
				return nil, r.errorf(":> がない")
			}
			v, err := r.value()
			if err != nil {
				return nil, err
			}
			ks, err := keyString(k)
			if err != nil {
				return nil, r.errorf("%v", err)
			}
			fn[ks] = v
			if r.take(")") {
				return fn, nil
			}
			if !r.take("@@") {
				return nil, r.errorf(") か @@ がない")
			}
		}
	case r.src[r.pos] == '"':
		var b strings.Builder
		r.pos++
		for r.pos < len(r.src) {
			c := r.src[r.pos]
			r.pos++
			switch c {
			case '"':
				return b.String(), nil
			case '\\':
				if r.pos >= len(r.src) {
					return nil, r.errorf("文字列が \\ で終わっている")
				}
				b.WriteByte(r.src[r.pos])
				r.pos++
			default:
				b.WriteByte(c)
			}
		}
		return nil, r.errorf("文字列が閉じていない")
	case r.src[r.pos] == '-' || r.src[r.pos] >= '0' && r.src[r.pos] <= '9':
		start := r.pos
		r.pos++
		for r.pos < len(r.src) && r.src[r.pos] >= '0' && r.src[r.pos] <= '9' {
			r.pos++
		}
		n, err := strconv.Atoi(r.src[start:r.pos])
		if err != nil {
			return nil, r.errorf("整数でない")
		}
		return n, nil
	}
	name := r.ident()
	switch name {
	case "":
		return nil, r.errorf("読めない文字")
	case "TRUE":
		return true, nil
	case "FALSE":
		return false, nil
	}
	return tlaModel(name), nil
}

// parseTraceFile は、-simulate file= が書き出したトレースの 1 本を、状態ごとの「変数 → 値」の並びにする。
// 状態は STATE_<n> == の後の「/\ 変数 = 値」の並びで、値は次の「/\ 」か空行まで続く（複数行にわたることがある）。
func parseTraceFile(src string) ([]map[string]any, error) {
	var states []map[string]any
	var cur map[string]any
	var name string
	var val strings.Builder
	flush := func() error {
		if name == "" {
			return nil
		}
		v, err := parseTLA(val.String())
		if err != nil {
			return fmt.Errorf("%d 番目の状態の %s: %w", len(states), name, err)
		}
		cur[name] = v
		name = ""
		val.Reset()
		return nil
	}
	for _, line := range strings.Split(src, "\n") {
		switch {
		case strings.HasPrefix(line, "STATE_"):
			if err := flush(); err != nil {
				return nil, err
			}
			cur = map[string]any{}
			states = append(states, cur)
		case strings.HasPrefix(line, "/\\ ") && cur != nil:
			if err := flush(); err != nil {
				return nil, err
			}
			eq := strings.Index(line, " = ")
			if eq < 0 {
				return nil, fmt.Errorf("変数の行に = がない: %q", line)
			}
			name = strings.TrimSpace(line[3:eq])
			val.WriteString(line[eq+3:])
		case strings.TrimSpace(line) == "" || strings.HasPrefix(line, "\\*") || strings.HasPrefix(line, "----") || strings.HasPrefix(line, "===="):
			if err := flush(); err != nil {
				return nil, err
			}
		default:
			if name == "" {
				return nil, fmt.Errorf("読めない行: %q", line)
			}
			val.WriteString("\n" + line)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return states, nil
}

func TestTLAの値の読み取りは手で書いた例を読める(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{`TRUE`, true},
		{`FALSE`, false},
		{`-3`, -3},
		{`12`, 12},
		{`"Agree"`, "Agree"},
		{`"a\"b\\c"`, `a"b\c`},
		{`i1`, tlaModel("i1")},
		{`{}`, tlaSet(nil)},
		{`<<>>`, tlaTuple(nil)},
		{`<<"Change", A, i2>>`, tlaTuple{"Change", tlaModel("A"), tlaModel("i2")}},
		{`(i1 :> 0)`, tlaFunc{"i1": 0}},
		{`(i1 :> {} @@ i2 :> {A, B})`, tlaFunc{"i1": tlaSet(nil), "i2": tlaSet{tlaModel("A"), tlaModel("B")}}},
		{"{ [item |-> i1, fp |-> 0, story |-> B],\n  [item |-> i2, fp |-> 2, story |-> A] }", tlaSet{
			tlaRecord{"item": tlaModel("i1"), "fp": 0, "story": tlaModel("B")},
			tlaRecord{"item": tlaModel("i2"), "fp": 2, "story": tlaModel("A")},
		}},
		{"[ agreed |-> (i1 :> TRUE @@ i2 :> FALSE),\n  broken |-> (A :> {} @@ B :> {i1}),\n  allPass |-> FALSE ]", tlaRecord{
			"agreed":  tlaFunc{"i1": true, "i2": false},
			"broken":  tlaFunc{"A": tlaSet(nil), "B": tlaSet{tlaModel("i1")}},
			"allPass": false,
		}},
	}
	for _, c := range cases {
		got, err := parseTLA(c.src)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got  %#v\n want %#v", c.src, got, c.want)
		}
	}
}

func TestTLAの値の読み取りは形の誤りをエラーにする(t *testing.T) {
	for _, src := range []string{``, `{1, 2`, `[a |-> 1`, `(i1 :> 1 @@)`, `"abc`, `<<1 2>>`, `{1} x`, `[ |-> 1]`, `?`} {
		if v, err := parseTLA(src); err == nil {
			t.Errorf("%q を誤りにしない: %#v", src, v)
		}
	}
}

func Testトレースの読み取りは状態ごとに複数行の値も含めて変数を読む(t *testing.T) {
	src := `---------------- MODULE tr_0_0 -----------------
\* <InitT line 4, col 10 to line 4, col 34 of module AgreementTrace>
STATE_1 ==
/\ agreements = {}
/\ step = <<"Init">>
/\ content = (i1 :> 0 @@ i2 :> 0)


\* <NextT line 7, col 10 to line 10, col 23 of module AgreementTrace>
STATE_2 ==
/\ agreements = { [item |-> i1, fp |-> 0, story |-> B],
  [item |-> i2, fp |-> 0, story |-> B] }
/\ step = <<"Agree", B, i2>>
/\ obs = [ agreed |-> (i1 :> TRUE @@ i2 :> TRUE),
  allPass |-> TRUE ]
`
	got, err := parseTraceFile(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{
		{"agreements": tlaSet(nil), "step": tlaTuple{"Init"}, "content": tlaFunc{"i1": 0, "i2": 0}},
		{
			"agreements": tlaSet{
				tlaRecord{"item": tlaModel("i1"), "fp": 0, "story": tlaModel("B")},
				tlaRecord{"item": tlaModel("i2"), "fp": 0, "story": tlaModel("B")},
			},
			"step": tlaTuple{"Agree", tlaModel("B"), tlaModel("i2")},
			"obs":  tlaRecord{"agreed": tlaFunc{"i1": true, "i2": true}, "allPass": true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %#v\nwant %#v", got, want)
	}
}
