package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// baseURL returns the server base URL from --host/--port flags or CODEVIEW_HOST env var.
func baseURL(cmd *cobra.Command) string {
	if env := os.Getenv("CODEVIEW_HOST"); env != "" {
		return env
	}
	host, _ := cmd.Root().PersistentFlags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	return fmt.Sprintf("http://%s:%d", host, port)
}

func init() {
	rootCmd.PersistentFlags().String("host", "127.0.0.1", "Server host (env: CODEVIEW_HOST)")
}
