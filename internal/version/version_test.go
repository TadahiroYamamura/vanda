package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		sign int
	}{
		{"同じ版は0", "0.1.0", "0.1.0", 0},
		{"patchが小さい方が古い", "0.1.0", "0.1.1", -1},
		{"minorは数として比べる", "0.10.0", "0.9.0", 1},
		{"majorはminorより先に比べる", "1.0.0", "0.99.99", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := Parse(c.a)
			if err != nil {
				t.Fatal(err)
			}
			b, err := Parse(c.b)
			if err != nil {
				t.Fatal(err)
			}
			got := Compare(a, b)
			if (got < 0 && c.sign >= 0) || (got == 0 && c.sign != 0) || (got > 0 && c.sign <= 0) {
				t.Errorf("Compare(%s, %s) = %d, want sign %d", c.a, c.b, got, c.sign)
			}
		})
	}
}

func TestParseは形の違う版を受け付けない(t *testing.T) {
	for _, s := range []string{"0.1", "0.1.0.0", "v0.1.0", "0.01.0", "0.-1.0", "0.1.0-rc.1", ""} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) がエラーにならなかった", s)
		}
	}
}
