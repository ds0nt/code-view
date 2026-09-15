package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/ds0nt/code-view/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the code-view server and open the browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		bind, _ := cmd.Flags().GetString("bind")
		noBrowser, _ := cmd.Flags().GetBool("no-browser")

		if !noBrowser {
			go func() {
				time.Sleep(200 * time.Millisecond)
				host := bind
				if bind == "0.0.0.0" {
					host = "127.0.0.1"
				}
				openBrowser(fmt.Sprintf("http://%s:%d", host, port))
			}()
		}

		s := server.New(bind, port)
		return s.Start()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().Int("port", 7878, "Port to listen on")
	serveCmd.Flags().String("bind", "127.0.0.1", "Address to bind (use 0.0.0.0 to expose on network)")
	serveCmd.Flags().Bool("no-browser", false, "Don't open browser automatically")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
