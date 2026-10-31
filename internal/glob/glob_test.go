package glob

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"ダブルスターは0段のディレクトリにも当たる", "e2e/**/*.txtar", "e2e/a.txtar", true},
		{"ダブルスターは複数段のディレクトリに当たる", "e2e/**/*.txtar", "e2e/x/y/a.txtar", true},
		{"シングルスターはディレクトリの区切りをまたがない", "e2e/*.txtar", "e2e/x/a.txtar", false},
		{"先頭のダブルスターはどこにでも当たる", "**/*.go", "a/b/c.go", true},
		{"末尾のダブルスターは下のすべてに当たる", ".vanda/**", ".vanda/checks/push.toml", true},
		{"拡張子が違えば当たらない", "**/*.go", "a/b/c.tla", false},
		{"ワイルドカードのない場所は同じときだけ当たる", ".vanda/promises.toml", ".vanda/promises.toml", true},
		{"パターンより深い場所には当たらない", ".vanda/*.toml", ".vanda/checks/push.toml", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Match(c.pattern, c.path); got != c.want {
				t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
			}
		})
	}
}
