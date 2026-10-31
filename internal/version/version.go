// Package version は、vanda の版と、その比べ方を持つ。
package version

import (
	"fmt"
	"strconv"
	"strings"
)

const Current = "0.1.0"

// Semver は、major.minor.patch の 3 つの数だけを扱う。段 1 では、版の後ろの印（-rc.1 など）を使う場面がないため。
type Semver [3]int

func Parse(s string) (Semver, error) {
	var v Semver
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("版 %q が major.minor.patch の形ではありません", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return v, fmt.Errorf("版 %q が major.minor.patch の形ではありません", s)
		}
		v[i] = n
	}
	return v, nil
}

// Compare は、a が b より古ければ負、同じなら 0、新しければ正を返す。
func Compare(a, b Semver) int {
	for i := range a {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}
	return 0
}
