package spec_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 定数は Agreement.cfg と同じ範囲にする（性質を検査した範囲で、前提が起きることを確かめるため）。
const constants = `CONSTANTS
    Items = {i1, i2}
    Files = {i1, i2}
    Stories = {A, B}
    MaxFp = 2
SPECIFICATION Spec
CHECK_DEADLOCK FALSE
`

func TestAgreementの性質の前提と合意の場面はどれも一度は起きる(t *testing.T) {
	tlc, err := exec.LookPath("tlc")
	if err != nil {
		// make check（と CI）は VANDA_REQUIRE_TLC=1 を立てる。黙って飛ばされて、CI で何も確かめていないことに気づかないのを防ぐため
		if os.Getenv("VANDA_REQUIRE_TLC") == "1" {
			t.Fatalf("VANDA_REQUIRE_TLC=1 なのに tlc がない（nix develop の中で動かす）")
		}
		t.Skip("tlc がないので飛ばす（nix develop の中で動かす）")
	}
	cases := []struct {
		name   string
		action bool // 操作の性質（PROPERTY）か、状態の性質（INVARIANT）か
	}{
		{"NeverOwnChangePasses", false},
		{"NeverOwnChangeUnagreed", false},
		{"NeverAllPassAfterChange", false},
		{"NeverTwoStoriesAgreeSame", false},
		{"NeverBrokenWhileAgreed", false},
		{"NeverOtherAgreesOnBroken", true},
		{"NeverRepair", true},
		{"NeverOwnChangeAfterAgree", true},
		{"NeverBrokenByOwnChangeOnly", false},
		{"NeverDeletedWithoutAgreement", false},
		{"NeverAllPassAfterDelete", false},
		{"NeverCopyOfAgreedProposed", false},
		{"NeverRenameAgreed", true},
		{"NeverStoryPassWithDiff", false},
		{"NeverStoryPassAfterDelete", false},
		{"NeverAddedThenDeletedIgnored", false},
		{"NeverReaddedAfterDeletionAgreed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"Agreement.tla", "AgreementVacuity.tla"} {
				src, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, f), src, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			kind := "INVARIANT"
			if c.action {
				kind = "PROPERTY"
			}
			cfg := constants + kind + " " + c.name + "\n"
			if err := os.WriteFile(filepath.Join(dir, "v.cfg"), []byte(cfg), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(tlc, "-noGenerateSpecTE", "-workers", "1", "-metadir", filepath.Join(dir, "meta"), "-config", "v.cfg", "AgreementVacuity.tla")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("%s が破られなかった（その場面が一度も起きていない）\n%s", c.name, out)
			}
			if !strings.Contains(string(out), c.name+" is violated") {
				t.Fatalf("%s が破られたのではない理由で TLC が失敗した\n%s", c.name, out)
			}
		})
	}
}
