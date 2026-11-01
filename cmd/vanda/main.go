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
	"github.com/TadahiroYamamura/vanda/internal/story"
	"github.com/TadahiroYamamura/vanda/internal/version"
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
	root.AddCommand(newVersionCommand(), newDoctorCommand(), newCheckCommand(), newServeCommand(), newInitCommand())
	return root
}

// errFailed は、審査が失敗したことを表す。中身はすでに出しているので、もう出さない。
var errFailed = errors.New("審査が失敗しました")

func newCheckCommand() *cobra.Command {
	var base, story string
	cmd := &cobra.Command{
		Use:   "check <審査の設定のファイル>",
		Short: "審査を行います",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("審査の設定のファイルがありません。審査の設定のファイルを 1 つ渡してください（例: vanda check .vanda/checks/push.toml。既定の審査はありません）。")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			src, err := os.ReadFile(args[0])
			if err != nil {
				return errors.New(args[0] + ": 審査の設定のファイルを読めません（" + err.Error() + "）。今のディレクトリからの場所を確かめてください。")
			}
			specs, errs := check.LoadSpecs(args[0], src)
			if errs != nil {
				return errs
			}
			pass, err := check.Run(check.Options{Root: root, Cfg: cfg, Specs: specs, Base: base, Story: story, Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			if !pass {
				return errFailed
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", `ベースブランチ（scope = "story" の審査があるときに必要です）`)
	cmd.Flags().StringVar(&story, "story", "", "ストーリーの ID（コミットのメッセージから決まらないときに渡します）")
	return cmd
}

// version は、.vanda/ がなくても、版が合わなくても動く（版が合わないときに、どの版かを確かめるため）。
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "版を表示します",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.Current)
			return nil
		},
	}
}

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "設定と版を確かめます",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			conn, err := story.Check(root, cfg)
			if err != nil {
				return err
			}
			if conn != "" {
				conn = "。" + conn
			}
			fmt.Fprintf(cmd.OutOrStdout(), "設定を確かめました: %s（リポジトリ %s、版 %s）%s\n", config.File, root, cfg.Version, conn)
			return nil
		},
	}
}

// loadConfig は、git のリポジトリの根を探し、設定を読んで版を確かめる。
func loadConfig() (string, *config.Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", nil, errors.New("今のディレクトリが分かりません（" + err.Error() + "）。")
	}
	root, rerr := repo.Root(wd)
	if rerr != nil {
		return "", nil, rerr
	}
	cfg, errs := config.Load(root)
	if errs != nil {
		return "", nil, diag.Errors(errs)
	}
	return root, cfg, nil
}
