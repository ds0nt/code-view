package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [file]",
	Short: "Show a diff in the viewer (git diff of file, or pipe a unified diff)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")

		var diffContent []byte
		var fileName string

		if len(args) == 0 && from == "" {
			// Read unified diff from stdin
			var err error
			diffContent, err = io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			fileName = ""
		} else if from != "" && to != "" {
			// Diff two files
			out, err := exec.Command("diff", "-u", from, to).Output()
			if err != nil && len(out) == 0 {
				return fmt.Errorf("diff failed: %w", err)
			}
			diffContent = out
			fileName = to
		} else if len(args) == 1 {
			// git diff of a specific file
			out, err := exec.Command("git", "diff", "HEAD", "--", args[0]).Output()
			if err != nil && len(out) == 0 {
				// Try staged diff
				out, err = exec.Command("git", "diff", "--cached", "--", args[0]).Output()
				if err != nil && len(out) == 0 {
					return fmt.Errorf("no git diff found for %s", args[0])
				}
			}
			if len(out) == 0 {
				// Try diff between working tree and last commit
				out, _ = exec.Command("git", "diff", "--", args[0]).Output()
			}
			diffContent = out
			fileName = args[0]
		}

		url := baseURL(cmd) + "/api/diff"
		if fileName != "" {
			url += "?file=" + fileName
		}
		resp, err := http.Post(url, "text/plain", bytes.NewReader(diffContent))
		if err != nil {
			return fmt.Errorf("server not running? start with: code-view serve\n%w", err)
		}
		resp.Body.Close()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
	diffCmd.Flags().Int("port", 7878, "Server port")
	diffCmd.Flags().String("from", "", "Original file for file-to-file diff")
	diffCmd.Flags().String("to", "", "New file for file-to-file diff")
}
