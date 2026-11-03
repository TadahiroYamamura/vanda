package review

// このファイルは、合意と取り消しの事後条件の確かめ（センサー）を持つ。設定の [[annotation]] の **/*_sensor.go で読む
// （本番のコードのうち、センサーを持つファイルだけを読むため。**/*_test.go と同じファイルに当たらないよう、名前で分ける）。

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// Hooks は、テストから、事後条件を破る状態を作るための口。本番では nil のまま使わない。
// 呼ばれるのは、それぞれの確かめ（センサー）の直前で、root はリポジトリの根。
var Hooks struct {
	// AfterAgreeWrite は、合意の記録を書き足し（約束ならフラグを外し）た後、verify の前に呼ぶ
	AfterAgreeWrite func(root string)
	// AfterAgreeCommit は、合意のコミットの後、checkAgreeCommit の前に呼ぶ
	AfterAgreeCommit func(root string)
	// AfterRevokeCommit は、取り消しのコミットの後、checkRevokeCommit の前に呼ぶ
	AfterRevokeCommit func(root string)
}

// verify は、書いた後の合意の記録と約束リストを読み直し、合意が今の中身に合い、フラグが外れたかを確かめる（コミットする前の事後条件）。
// 破れていれば誤りを返し、Agree は書いたファイルを元に戻して 500 にする。
//
// @vanda:sensor agree-writes-and-commits
// @vanda:sensor agree-removes-flags
func (s *state) verify(rec AgreementRecord, claim *promises.Claim) error {
	snap, err := snapshot.Working(s.o.Root, s.o.Cfg)
	if err != nil {
		return err
	}
	if snap.ListErrs != nil {
		return fmt.Errorf("フラグを外した後の約束リストを読めません（%s）", snap.ListErrs[0].Error())
	}
	recs, rerrs := agreements.Load(s.o.Root, snap.Files, s.o.Cfg.Agreements, s.o.Cfg.AgreementsDefault)
	if rerrs != nil {
		return fmt.Errorf("書き足した後の合意の記録を読めません（%s）", rerrs[0].Error())
	}
	found := rec.Deleted && recs.DeletionAgreedBy(rec.Kind, rec.target(), rec.Story) ||
		rec.Fingerprint != nil && recs.AgreedBy(rec.Kind, rec.target(), *rec.Fingerprint, rec.Story)
	if !found {
		return errors.New("書き足した合意が、読み直した合意の記録に見つかりません")
	}
	if claim == nil {
		return nil
	}
	after := newSide(snap)
	key := "claim:" + claim.ID
	if rec.Fingerprint == nil || after.itemFP[key] != *rec.Fingerprint {
		return errors.New("フラグを外したら、約束の指紋が変わりました")
	}
	want := map[string]any{}
	for k, v := range claim.Raw {
		if k != "flags" && k != "guesses" {
			want[k] = v
		}
	}
	if !reflect.DeepEqual(after.items[key].Raw, want) {
		return errors.New("フラグを外したら、flags・guesses のほかの項目まで変わりました")
	}
	return nil
}

// checkAgreeCommit は、合意のコミットの後に、コミットが合意で変えるファイル files だけを含み、合意の対象が書いてあるファイル sources
// （消した約束・用語・機能なら、約束リストのファイル）と files に、コミットしていない変更が残っていないかを確かめる
// （合意のコミットの時点のリポジトリが、合意した中身と合うこと）。
//
// @vanda:sensor agree-writes-and-commits
// @vanda:sensor agree-rejects-uncommitted
func checkAgreeCommit(root string, files, sources []string) error {
	if err := committedExactly(root, files); err != nil {
		return err
	}
	paths := append(append([]string{}, sources...), files...)
	dirty, err := repo.Git(root, append([]string{"status", "--porcelain", "-z", "--"}, paths...)...)
	if err != nil {
		return err
	}
	if dirty != "" {
		return fmt.Errorf("コミットの後も、%s にコミットしていない変更があります", strings.Join(paths, "・"))
	}
	return nil
}

// checkRevokeCommit は、取り消しのコミットの後に、コミットが合意の記録のファイル files だけを含む（約束リストのフラグを戻していない）かを確かめる。
//
// @vanda:sensor revoke-commits-and-keeps-flags
func checkRevokeCommit(root string, files []string) error {
	return committedExactly(root, files)
}

// committedExactly は、HEAD のコミットで変わったファイルが、ちょうど files かを確かめる。
func committedExactly(root string, files []string) error {
	out, err := repo.Git(root, "diff-tree", "--root", "--no-commit-id", "--name-only", "-r", "-z", "HEAD")
	if err != nil {
		return err
	}
	var got []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			got = append(got, f)
		}
	}
	want := append([]string(nil), files...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("コミットで変わったファイルが %s で、%s だけではありません", strings.Join(got, "・"), strings.Join(want, "・"))
	}
	return nil
}

// sensorErr は、コミットの後に事後条件が破れていたときの誤り。コミットは履歴に残っているので消さず、確かめ方を伝える。
func sensorErr(what, commit string, err error) *Error {
	return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("%sのコミット %s の後の確かめで、食い違いが見つかりました（%v）。", what, commit, err), Fix: "git show " + commit + " と git status で、コミットと作業ツリーを確かめて、直してください。"}
}
