package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/initcmd"
	"github.com/TadahiroYamamura/vanda/internal/repo"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "問いかけに答えて .vanda/ を作ります",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 問いかけを飛ばすオプションは持たない（CMD-11）。端末でなければ止める
			if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
				return errors.New("端末で動かしていないため、問いかけができません。vanda init は端末で動かしてください。")
			}
			wd, err := os.Getwd()
			if err != nil {
				return errors.New("今のディレクトリが分かりません（" + err.Error() + "）。")
			}
			root, rerr := repo.Root(wd)
			if rerr != nil {
				// repo.Root の直し方は「.vanda/ のある」リポジトリを前提にしているので、init では言い方を変える
				return errors.New("git のリポジトリの外で動かしています。vanda init は、git のリポジトリの中で動かしてください。")
			}
			if _, err := os.Lstat(filepath.Join(root, ".vanda")); err == nil {
				return errors.New(".vanda/ がすでにあります。何も書き換えませんでした。")
			} else if !errors.Is(err, os.ErrNotExist) {
				return errors.New(".vanda/ があるかを確かめられません（" + err.Error() + "）。")
			}
			d, err := initcmd.Detect(root)
			if err != nil {
				return err
			}
			return runInit(cmd, root, d)
		},
	}
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// runInit は、問いかけて確かめ、.vanda/ を作る。
func runInit(cmd *cobra.Command, root string, d initcmd.Defaults) error {
	out := cmd.OutOrStdout()
	// 端末かを確かめたのは os.Stdin なので、問いかけも os.Stdin から読む
	t := initcmd.NewTerminal(os.Stdin, out)
	fmt.Fprintf(out, "%s に .vanda/ を作ります。問いに答えてください（既定の答えが入った状態から始まります）。\n\n", root)
	a, err := initcmd.Ask(root, d, t)
	if err != nil {
		return err
	}
	files := initcmd.Files(a)
	fmt.Fprintln(out, "\n次のファイルを作ります。")
	for _, f := range files {
		if f[1] == "" {
			fmt.Fprintf(out, "\n%s（空）\n", f[0])
			continue
		}
		fmt.Fprintf(out, "\n%s:\n", f[0])
		for _, l := range strings.Split(strings.TrimRight(f[1], "\n"), "\n") {
			fmt.Fprintln(out, strings.TrimRight("  "+l, " "))
		}
	}
	fmt.Fprintln(out)
	yes, err := initcmd.Confirm(t)
	if err != nil {
		return err
	}
	if !yes {
		return initcmd.ErrDeclined
	}
	if err := initcmd.Write(root, files); err != nil {
		return err
	}
	return report(out, root, files, len(a.Annotations) == 0)
}

// report は、作った設定を vanda doctor と同じく確かめ、作ったファイルと次にすることを出す。
// 利用者のアダプタは .vanda/ を作った後にしか置けないので、まだないことだけは誤りにせず、置くよう案内する。
func report(out io.Writer, root string, files [][2]string, noAnnotations bool) error {
	cfg, missing, errs := config.LoadAllowingMissingAdapters(root)
	if errs != nil {
		return fmt.Errorf(".vanda/ を作りましたが、設定を読めません。次の誤りを直してください。\n%s", errs.Error())
	}
	fmt.Fprintln(out, "\n.vanda/ を作りました。")
	for _, f := range files {
		fmt.Fprintln(out, "  "+f[0])
	}
	fmt.Fprintf(out, "設定を確かめました: %s（版 %s）\n", config.File, cfg.Version)
	fmt.Fprintln(out, "\n次にすること:")
	fmt.Fprintln(out, "  1. "+config.File+" に、約束のレイヤーを [[layer]] で書いてください（id・name・description）。")
	fmt.Fprintf(out, "  2. CI で vanda check .vanda/checks/ci.toml --base %s を動かすよう、CI の設定に足してください。\n", cfg.GUIBase)
	n := 3
	if noAnnotations {
		fmt.Fprintf(out, "  %d. %s に、アノテーションを読むファイルを [[annotation]] で書いてください（paths と adapter）。\n", n, config.File)
		n++
	}
	for _, name := range missing {
		fmt.Fprintf(out, "  %d. %s/%s に利用者のアダプタを置いてください。\n", n, config.AdaptersDir, name)
		n++
	}
	fmt.Fprintln(out, "vanda の中にアダプタがない言語（TypeScript など）は、利用者のアダプタを .vanda/adapters/ に置いて足せます。")
	return nil
}
