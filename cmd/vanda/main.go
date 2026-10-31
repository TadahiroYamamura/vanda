package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "vanda",
		Short:         "エビデンス・ファースト開発の道具一式",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}
