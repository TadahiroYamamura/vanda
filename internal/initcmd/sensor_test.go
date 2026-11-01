package initcmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TadahiroYamamura/vanda/internal/initcmd"
)

// Write の事後条件の確かめ（init_sensor.go）のテスト。作る前からあった .vanda/ に書き込んだ状態は、Write が先に .vanda/ を
// 作るので外からは作れない。Hooks.AfterWrite の口から .vanda/ の中を変え、Write が誤りを返して .vanda/ を消さずに残すことを見る。
// Hooks はパッケージの変数なので、並べて動かさない。

var sensorFiles = [][2]string{
	{".vanda/config.toml", "version = \"0.1.0\"\n"},
	{".vanda/promises.toml", ""},
	{".vanda/agreements.toml", ""},
	{".vanda/checks/ci.toml", "[[check]]\nname = \"agreement\"\nscope = \"all\"\n"},
}

// @vanda:evidence init-keeps-existing
func TestWriteは書いた後の_vandaの中が書いたものと違えば誤りを返し_vandaを消さずに残す(t *testing.T) {
	cases := []struct {
		name    string
		breakIt func(root string) error
		left    string // 誤りの後も残っているはずのファイル
	}{
		{"破らない", nil, ".vanda/config.toml"},
		{"ほかのファイルを置く", func(root string) error {
			return os.WriteFile(filepath.Join(root, ".vanda/前からあった.toml"), []byte("x\n"), 0o644)
		}, ".vanda/前からあった.toml"},
		{"書いたファイルの中身を変える", func(root string) error {
			return os.WriteFile(filepath.Join(root, ".vanda/config.toml"), []byte("version = \"9.9.9\"\n"), 0o644)
		}, ".vanda/config.toml"},
		{"書いたファイルを消す", func(root string) error {
			return os.Remove(filepath.Join(root, ".vanda/promises.toml"))
		}, ".vanda/config.toml"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			called := false
			initcmd.Hooks.AfterWrite = func(root string) {
				called = true
				if c.breakIt != nil {
					if err := c.breakIt(root); err != nil {
						t.Error(err)
					}
				}
			}
			t.Cleanup(func() { initcmd.Hooks.AfterWrite = nil })
			err := initcmd.Write(root, sensorFiles)
			if !called {
				t.Fatalf("口が呼ばれる前に終わった: %v", err)
			}
			if c.breakIt == nil {
				if err != nil {
					t.Fatalf("破っていないのに誤りを返した: %v", err)
				}
			} else if err == nil {
				t.Fatalf("書いたものと違う .vanda/ なのに誤りを返さない")
			}
			if _, serr := os.Stat(filepath.Join(root, c.left)); serr != nil {
				t.Errorf(".vanda/ が残っていない（%s）: %v", c.left, serr)
			}
		})
	}
}
