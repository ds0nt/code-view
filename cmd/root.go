package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "code-view",
	Short: "A persistent code viewer for agents",
	Long: `code-view — real-time browser code viewer for AI agents and humans.

Push code and diffs to a browser window as you work. The server streams
events to the browser via SSE; the browser renders syntax-highlighted code
and side-by-side diffs in real time.

Run 'code-view instructions' for a full guide on how to use this as an AI agent.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
