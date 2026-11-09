package check

import (
	"strings"
	"testing"
)

// コミットのストーリーの ID（docs/formats.md の「コミットのストーリーの ID」）。メッセージは手で書き、
// スコープ story の agreement がどのストーリーに決まったか（結果の行）か、決まらなかった誤りで見る。

// @vanda:evidence story-id-from-trailer
func Testストーリーの_IDはメッセージの最後の段落のStoryのトレーラーからだけ読みsubjectと本文のIDは数えない(t *testing.T) {
	cases := []struct {
		name, msg string
		want      string // 空なら、ストーリーが決まらない
	}{
		{"subject だけに ID", "(feat) #5 を直す", ""},
		{"本文に ID があってもトレーラーのものだけ", "(feat) 直す\n\n#5 の続き。\n\nStory: #2", "#2"},
		{"Co-Authored-By の前", "(feat) 直す\n\nStory: #2\nCo-Authored-By: A <a@example.com>", "#2"},
		{"続きの行の後", "(feat) 直す\n\nCo-Authored-By: A <a@example.com>\n  続きの行\nStory: #2", "#2"},
		{"キーが小文字", "(feat) 直す\n\nstory: #2", "#2"},
		{"キーが大文字", "(feat) 直す\n\nSTORY: #2", "#2"},
		{"最後の段落でない", "(feat) 直す\n\nStory: #2\n\n本文の最後の段落。", ""},
		{"subject だけのメッセージ", "Story: #2", ""},
		{"最後の段落にトレーラーでない行がある", "(feat) 直す\n\nStory: #2\nこれはトレーラーでない行", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := goodRepo(t)
			r.git("checkout", "-q", "-b", "work")
			r.write(map[string]string{"README.md": c.name})
			r.commitMsg(c.msg)
			_, out, err := r.run(storyAgreement, "main", "")
			if c.want == "" {
				if err == nil || !strings.Contains(err.Error(), "Story: のトレーラーがありません") {
					t.Errorf("ストーリーが決まらないはずが: err=%v\n%s", err, out)
				}
				return
			}
			if err != nil || !strings.Contains(out, "チェック agreement（story "+c.want+"）") {
				t.Errorf("ストーリーが %s に決まらない: err=%v\n%s", c.want, err, out)
			}
		})
	}
}

// スコープ story の agreement・evidence-present を含む設定では、トレーラーの誤りは --story を渡してもチェックの前の誤り。
// story-origin だけなら、約束リストを変えたそのコミットだけが story-origin の失敗。スコープがすべて all なら見ない
// @vanda:evidence story-trailer-at-most-one
// @vanda:evidence story-trailer-matches-pattern
// @vanda:evidence check-story-arg-pattern
func TestStoryのトレーラーの誤りは決めるチェックではチェックの前に止めstory_originではそのコミットの失敗にしallでは見ない(t *testing.T) {
	const (
		storyEvidence = "[[check]]\nname = \"evidence-present\"\nscope = \"story\"\n"
		storyOrigin   = "[[check]]\nname = \"story-origin\"\nscope = \"story\"\n"
		allOnly       = "[[check]]\nname = \"agreement\"\nscope = \"all\"\n[[check]]\nname = \"evidence-present\"\nscope = \"all\"\n"
	)
	bads := []struct {
		name, trailer string
		frags         []string
	}{
		{"2つ", "Story: #2\nStory: #3", []string{"2 つ", "#2・#3"}},
		{"id_patternに合わない", "Story: PROJ-1", []string{"PROJ-1", "#[0-9]+"}},
		{"id_patternに一部だけマッチする", "Story: #2a", []string{"#2a", "#[0-9]+"}},
	}
	for _, bad := range bads {
		for _, promisesChanged := range []bool{true, false} {
			name := bad.name + "・約束リストを変えない"
			if promisesChanged {
				name = bad.name + "・約束リストを変える"
			}
			t.Run(name, func(t *testing.T) {
				r := goodRepo(t)
				r.git("checkout", "-q", "-b", "work")
				r.write(map[string]string{"README.md": "x"})
				r.commit("(docs) メモ", "#2")
				if promisesChanged {
					r.write(map[string]string{".vanda/promises.toml": basePromises + "\n"})
				} else {
					r.write(map[string]string{"README.md": "y"})
				}
				r.commitMsg("(feat) 誤ったトレーラー\n\n" + bad.trailer)
				hash := r.git("rev-parse", "--short=7", "HEAD")
				hash = strings.TrimSpace(hash)
				wantErr := func(t *testing.T, what, got string) {
					t.Helper()
					for _, f := range append([]string{hash, "(feat) 誤ったトレーラー"}, bad.frags...) {
						if !strings.Contains(got, f) {
							t.Errorf("%s: 誤りに %q がない: %s", what, f, got)
						}
					}
				}

				// ストーリーを決めるチェックを含む設定は、--story があってもなくてもチェックの前に止まる（結果の行が出ない）
				for _, checks := range []string{storyAgreement, storyEvidence, storyOrigin + storyAgreement} {
					for _, story := range []string{"", "#2"} {
						what := strings.ReplaceAll(checks, "\n", " ") + " --story " + story
						_, out, err := r.run(checks, "main", story)
						if err == nil {
							t.Errorf("%s: チェックの前に止まらない\n%s", what, out)
							continue
						}
						wantErr(t, what, err.Error())
						if strings.Contains(out, "チェック ") {
							t.Errorf("%s: チェックの結果の行が出た\n%s", what, out)
						}
					}
				}

				// --story の形の誤りは、トレーラーの誤りより先に出る
				if _, _, err := r.run(storyAgreement, "main", "3"); err == nil || !strings.Contains(err.Error(), "--story 3 が、ストーリーの ID の書き方") || strings.Contains(err.Error(), hash) {
					t.Errorf("--story の形の誤りが先に出ない: %v", err)
				}

				// story-origin だけならチェックの前には止まらず、約束リストを変えたそのコミットだけが失敗
				for _, story := range []string{"", "#2"} {
					pass, out, err := r.run(storyOrigin, "main", story)
					if err != nil {
						t.Errorf("story-origin だけでチェックの前に止まった: %v", err)
						continue
					}
					if !promisesChanged {
						if !pass {
							t.Errorf("約束リストを変えていないコミットで story-origin が失敗した\n%s", out)
						}
						continue
					}
					if pass || !strings.Contains(out, "チェック story-origin（story）: 失敗（1 件）") {
						t.Errorf("約束リストを変えたコミットで story-origin が 1 件の失敗にならない\n%s", out)
					}
					wantErr(t, "story-origin", out)
				}

				// スコープがすべて all なら、--base があってもなくても見ない
				for _, base := range []string{"", "main"} {
					if _, out, err := r.run(allOnly, base, ""); err != nil || strings.Contains(out, hash) {
						t.Errorf("スコープがすべて all（--base %q）でトレーラーの誤りを見た: err=%v\n%s", base, err, out)
					}
				}
			})
		}
	}
}
