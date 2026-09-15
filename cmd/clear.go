package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the viewer",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := baseURL(cmd) + "/api/clear"
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			return fmt.Errorf("server not running? start with: code-view serve\n%w", err)
		}
		resp.Body.Close()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(clearCmd)
	clearCmd.Flags().Int("port", 7878, "Server port")
}
