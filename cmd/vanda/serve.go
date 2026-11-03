package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/TadahiroYamamura/vanda/internal/server"
	"github.com/TadahiroYamamura/vanda/web"
)

func newServeCommand() *cobra.Command {
	var port int
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "手元のサーバーを起動し、ブラウザで開きます",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, _, cfg, err := loadCommittedConfig()
			if err != nil {
				return err
			}
			// 自分のマシンの中でだけ開く（CM-01）ため、127.0.0.1 でだけ待ち受ける
			ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				return fmt.Errorf("127.0.0.1:%d で待ち受けられません（%v）。--port に空いているポートを渡すか、--port 0 を渡してください（0 なら空いているポートを選びます）。", port, err)
			}
			url := "http://" + ln.Addr().String() + "/"
			srv := &http.Server{Handler: (&server.Server{Root: root, Cfg: cfg, UI: web.Files()}).Handler()}
			fmt.Fprintf(cmd.OutOrStdout(), "vanda を起動しました: %s（Ctrl+C で停止します）\n", url)
			if !noOpen {
				if err := openBrowser(url); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "ブラウザを開けませんでした（%v）。%s をブラウザで開いてください。\n", err, url)
				}
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			done := make(chan error, 1)
			go func() { done <- srv.Serve(ln) }()
			select {
			case err := <-done:
				return fmt.Errorf("サーバーが止まりました（%v）", err)
			case <-ctx.Done():
			}
			if err := srv.Shutdown(context.Background()); err != nil {
				return fmt.Errorf("サーバーを止められませんでした（%v）", err)
			}
			if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("サーバーが止まりました（%v）", err)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "待ち受けるポート。0 なら空いているポートを選びます")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "ブラウザを開きません")
	return cmd
}

func openBrowser(url string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	if err := c.Start(); err != nil {
		return err
	}
	// 開いたコマンドが終わるのを待って、終わったプロセスを残さない
	go func() {
		if err := c.Wait(); err != nil {
			log.Printf("ブラウザを開くコマンドが失敗しました（%v）。%s をブラウザで開いてください。", err, url)
		}
	}()
	return nil
}
