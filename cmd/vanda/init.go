package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
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

// runInit は、問いかけて確かめ、.vanda/ を作る。問いかけの途中の Ctrl+C は、何も作らずに止める。
func runInit(cmd *cobra.Command, root string, d initcmd.Defaults) error {
	out := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())
	fmt.Fprintf(out, "%s に .vanda/ を作ります。問いに答えてください（[ ] の中は既定の答えで、空のまま Enter で使います）。\n\n", root)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	type result struct {
		files [][2]string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		a, err := initcmd.Ask(root, d, in, out)
		if err != nil {
			done <- result{err: err}
			return
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
		yes, err := initcmd.Confirm(in, out)
		if err == nil && !yes {
			err = initcmd.ErrDeclined
		}
		done <- result{files: files, err: err}
	}()
	var r result
	select {
	case <-sig:
		fmt.Fprintln(out)
		return errors.New("中断したため、何も作りませんでした。")
	case r = <-done:
	}
	if r.err != nil {
		return r.err
	}
	// 書き始めた後の Ctrl+C では止めない（作りかけの .vanda/ を残さないため。書くのはすぐに終わる）
	if err := initcmd.Write(root, r.files); err != nil {
		return err
	}
	return report(out, root, r.files)
}

// report は、作った設定を vanda doctor と同じく確かめ、作ったファイルと次にすることを出す。
func report(out io.Writer, root string, files [][2]string) error {
	cfg, errs := config.Load(root)
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
	fmt.Fprintln(out, "  3. 作ったファイルを確かめて、コミットしてください（vanda init はコミットしません）。")
	return nil
}
