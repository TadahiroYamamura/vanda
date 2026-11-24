package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/vanda/internal/check"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/repo"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run は、コマンドを動かし、終了コードを返す。エラーは 1 件 1 行で stderr に出す。
func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCommand()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		if !errors.Is(err, errFailed) {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	return 0
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "vanda",
		Short:         "エビデンス・ファースト開発の道具一式です",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newCheckCommand(), newServeCommand(), newInitCommand())
	return root
}

// errFailed は、チェックが失敗したことを表す。中身はすでに出しているので、もう出さない。
var errFailed = errors.New("チェックが失敗しました")

func newCheckCommand() *cobra.Command {
	var base, story string
	cmd := &cobra.Command{
		Use:   "check <チェックの設定のファイル>",
		Short: "チェックします",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("チェックの設定のファイルがありません。チェックの設定のファイルを 1 つ渡してください（例: vanda check .vanda/checks/push.toml。既定のチェックはありません）。")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			root, tree, cfg, err := loadCommittedConfig()
			if err != nil {
				return err
			}
			src, err := os.ReadFile(args[0])
			if err != nil {
				return errors.New(args[0] + ": チェックの設定のファイルを読めません（" + err.Error() + "）。今のディレクトリからの場所を確かめてください。")
			}
			specs, errs := check.LoadSpecs(args[0], src)
			if errs != nil {
				return errs
			}
			pass, err := check.Run(check.Options{Root: root, Tree: tree, Cfg: cfg, Specs: specs, Base: base, Story: story, Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			if !pass {
				return errFailed
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", `ベースブランチ（scope = "story" のチェックがあるときに必要です）`)
	cmd.Flags().StringVar(&story, "story", "", "ストーリーの ID（コミットの Story: のトレーラーから決まらないときに渡します）")
	return cmd
}

// loadCommittedConfig は、git のリポジトリのルートを探し、HEAD のコミットの設定を読んでバージョンを確かめる（チェックと画面は、コミットした中身だけを読むため）。
func loadCommittedConfig() (string, *repo.Tree, *config.Config, error) {
	root, err := findRoot()
	if err != nil {
		return "", nil, nil, err
	}
	tree, err := repo.HeadTree(root)
	if err != nil {
		return "", nil, nil, err
	}
	cfg, errs := config.LoadCommitted(root, tree)
	if errs != nil {
		return "", nil, nil, diag.Errors(errs)
	}
	return root, tree, cfg, nil
}

func findRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", errors.New("今のディレクトリが分かりません（" + err.Error() + "）。")
	}
	root, rerr := repo.Root(wd)
	if rerr != nil {
		return "", rerr
	}
	return root, nil
}
