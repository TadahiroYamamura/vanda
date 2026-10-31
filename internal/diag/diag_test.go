package diag

import "testing"

func TestSortはファイルの場所の順に並べ同じファイルなら行の順にする(t *testing.T) {
	es := []Error{{File: "b", Line: 1, What: "1"}, {File: "a", Line: 9, What: "2"}, {File: "a", Line: 0, What: "3"}, {File: "a", Line: 2, What: "4"}, {File: "a", Line: 2, What: "5"}}
	Sort(es)
	got := ""
	for _, e := range es {
		got += e.What
	}
	if got != "34521" {
		t.Errorf("got %s", got)
	}
}

func TestErrorは行があれば行を書き行がなければ場所を書く(t *testing.T) {
	cases := map[string]Error{
		// 直し方の後にも「。」を付ける（docs/plans/stage2-wording.md の「誤りの終わり」）
		"a.toml:3: 誤り。直す。":            {File: "a.toml", Line: 3, Item: "claim \"x\"", What: "誤り", Fix: "直す"},
		"a.toml: claim \"x\": 誤り。直す。": {File: "a.toml", Item: "claim \"x\"", What: "誤り", Fix: "直す"},
		"a.toml: 誤り":                  {File: "a.toml", What: "誤り"},
	}
	for want, e := range cases {
		if got := e.Error(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
