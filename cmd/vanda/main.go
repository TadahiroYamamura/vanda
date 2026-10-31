package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/repo"
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
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "vanda",
		Short:         "エビデンス・ファースト開発の道具一式",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCommand(), newDoctorCommand())
	return root
}

// version は、.vanda/ がなくても、版が合わなくても動く（版が合わないときに、どの版かを確かめるため）。
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "版を出す",
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
		Short: "設定と版を確かめる",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "設定を確かめた: %s（リポジトリ %s、版 %s）\n", config.File, root, cfg.Version)
			return nil
		},
	}
}

// loadConfig は、git のリポジトリの根を探し、設定を読んで版を確かめる。
func loadConfig() (string, *config.Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", nil, errors.New("今のディレクトリが分からない（" + err.Error() + "）")
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
