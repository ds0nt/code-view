package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

var promptCmd = &cobra.Command{
	Use:   "prompt [message]",
	Short: "Send a prompt with Accept/Reject buttons and wait for response",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		base := baseURL(cmd)

		body, _ := json.Marshal(map[string]string{"message": args[0]})
		resp, err := http.Post(base+"/api/prompt", "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}
		resp.Body.Close()

		// Long-poll for the response
		waitResp, err := http.Get(base + "/api/wait")
		if err != nil {
			return err
		}
		defer waitResp.Body.Close()

		var result map[string]string
		if err := json.NewDecoder(waitResp.Body).Decode(&result); err != nil {
			return err
		}

		fmt.Println(result["decision"])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(promptCmd)
	promptCmd.Flags().Int("port", 7878, "Port of the code-view server")
}
