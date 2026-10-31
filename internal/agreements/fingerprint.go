package agreements

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const fingerprintPrefix = "sha256:"

// FileFingerprint は、ファイルの中身そのものから指紋を作る。
func FileFingerprint(src []byte) string {
	sum := sha256.Sum256(src)
	return fingerprintPrefix + hex.EncodeToString(sum[:])
}

// ItemFingerprint は、約束リストの項目の表から flags と guesses を除き、決まった形の JSON にしてから指紋を作る。
func ItemFingerprint(raw map[string]any) (string, error) {
	m := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "flags" || k == "guesses" {
			continue
		}
		m[k] = v
	}
	s, err := CanonicalJSON(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(s))
	return fingerprintPrefix + hex.EncodeToString(sum[:]), nil
}

// CanonicalJSON は、表を、キーを名前の順（UTF-8 のバイトの順）に並べ、空白を入れない JSON にする。
// encoding/json を使わないのは、文字列の書き方（どの文字を \u にするか）を Go の版に左右されず、docs/formats.md のとおりに固定するため。
// 約束リストの項目に書ける値（文字列・真偽・整数・配列・表）だけを扱い、ほかの値はエラーにする。
func CanonicalJSON(v any) (string, error) {
	var b strings.Builder
	if err := writeJSON(&b, v); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeJSON(b *strings.Builder, v any) error {
	switch x := v.(type) {
	case string:
		if !utf8.ValidString(x) {
			return fmt.Errorf("UTF-8 でない文字列がある")
		}
		writeString(b, x)
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeJSON(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case []map[string]any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeJSON(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k)
			b.WriteByte(':')
			if err := writeJSON(b, x[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("指紋を作れない種類の値（%T）がある", v)
	}
	return nil
}

func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
