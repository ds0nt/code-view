package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var instructionsCmd = &cobra.Command{
	Use:   "instructions",
	Short: "Print usage instructions for Claude (AI agent)",
	Long:  "Prints a guide for an AI agent on how to use code-view during a coding session.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(claudeInstructions)
	},
}

func init() {
	rootCmd.AddCommand(instructionsCmd)
}

const claudeInstructions = `
# code-view — Instructions for Claude

code-view is a browser-based code viewer you push files and diffs to in real-time.
The server must be running: start it with ./run.sh or 'code-view serve'
It listens on http://127.0.0.1:7878 by default.

## RULE: Auto-push on read
Every time you read a file to explore or work on it, immediately push it to the
viewer so the user can follow along in the browser.

## Show a file
$ code-view show <file>                           # auto-detects language
$ code-view show -l go -d "description" <file>   # force language + description
$ code-view show -n 10-50 <file>                  # show only lines 10–50
$ code-view show -n 42 <file>                     # show just line 42
$ code-view show -l go -d "description" \
    -n 80-120 \
    -a "func handleShow::receives the push" \
    -a "s.broadcast::fans to all SSE clients" \
    <file>                                        # excerpt + annotations

-n (--lines): slice to a line range before sending — the card title shows file.go:10-50
-d (--desc):  short description shown on the card — always include one
-a (--annotation): "text::comment" — highlights first match in gold, arrow-key navigable

Prefer -n over sending whole files. Send the relevant function or section, not the world.

Supported lang values: go, python, javascript, typescript, rust, bash, json, yaml,
  markdown, html, css, sql, c, cpp, java, ruby, toml, hcl, lua, php, csharp, kotlin, swift

## Show a diff
$ git diff HEAD -- <file> | code-view diff
$ code-view diff <file>              # git diff of that file
$ code-view diff --from a.go --to b.go

## Ask the user to accept or reject something
$ code-view prompt "Apply this change?"   # blocks until user clicks in browser
  → prints "accept" or "reject" to stdout

The prompt bar appears at the bottom of the browser with Accept/Reject buttons.
Use this before applying significant changes, destructive operations, or when unsure.

## Clear the viewer
$ code-view clear

RULE: Call 'code-view clear' at the START of every new user task or question.
This keeps the viewer focused on what's currently relevant.

## Workflow pattern
1. User asks something new → immediately run: code-view clear
2. Read the file → immediately push to code-view show
3. If making a change → show the diff with code-view diff
4. Optionally prompt for accept/reject
5. Apply the change
6. Push the updated file to code-view show

## Tips
- Push files as you READ them, not just when you edit them
- Add annotations (-a) whenever you want to highlight interesting sections
- The viewer replays history on browser refresh — state is preserved
- run.sh in the project root starts the server and opens the browser
`
