package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type Annotation struct {
	Text    string `json:"text"`
	Comment string `json:"comment"`
}

var showCmd = &cobra.Command{
	Use:   "show <file>",
	Short: "Show a file in the code viewer",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lang, _ := cmd.Flags().GetString("lang")
		desc, _ := cmd.Flags().GetString("desc")
		annStrs, _ := cmd.Flags().GetStringArray("annotation")
		linesFlag, _ := cmd.Flags().GetString("lines")

		var content []byte
		var name string

		if len(args) == 0 {
			var err error
			content, err = os.ReadFile("/dev/stdin")
			if err != nil {
				return err
			}
			name = "stdin"
		} else {
			var err error
			content, err = os.ReadFile(args[0])
			if err != nil {
				return err
			}
			name = args[0]
			if lang == "" {
				lang = extToLang(filepath.Ext(name))
			}
		}

		// Slice to requested line range (1-based, inclusive)
		if linesFlag != "" {
			lines := strings.Split(string(content), "\n")
			start, end := 1, len(lines)
			parts := strings.SplitN(linesFlag, "-", 2)
			if v, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				start = v
			}
			if len(parts) == 2 {
				if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					end = v
				}
			} else {
				end = start
			}
			if start < 1 {
				start = 1
			}
			if end > len(lines) {
				end = len(lines)
			}
			content = []byte(strings.Join(lines[start-1:end], "\n"))
			name = fmt.Sprintf("%s:%d-%d", name, start, end)
		}

		var anns []Annotation
		for _, s := range annStrs {
			parts := strings.SplitN(s, "::", 2)
			if len(parts) == 2 {
				anns = append(anns, Annotation{Text: parts[0], Comment: parts[1]})
			}
		}

		type payload struct {
			File        string       `json:"file"`
			Content     string       `json:"content"`
			Lang        string       `json:"lang"`
			Description string       `json:"description"`
			Annotations []Annotation `json:"annotations,omitempty"`
		}
		body, _ := json.Marshal(payload{
			File:        name,
			Content:     string(content),
			Lang:        lang,
			Description: desc,
			Annotations: anns,
		})
		url := baseURL(cmd) + "/api/show"
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("server not running? start with: code-view serve\n%w", err)
		}
		resp.Body.Close()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
	showCmd.Flags().Int("port", 7878, "Server port")
	showCmd.Flags().StringP("lang", "l", "", "Language for syntax highlighting")
	showCmd.Flags().StringP("desc", "d", "", "Short description shown on the card")
	showCmd.Flags().StringArrayP("annotation", "a", nil, `Annotation in "text::comment" format (repeatable)`)
	showCmd.Flags().StringP("lines", "n", "", "Line range to show, e.g. 10-50 or 42")
}

func extToLang(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	m := map[string]string{
		"go":   "go",
		"py":   "python",
		"js":   "javascript",
		"ts":   "typescript",
		"jsx":  "javascript",
		"tsx":  "typescript",
		"rs":   "rust",
		"c":    "c",
		"cpp":  "cpp",
		"h":    "c",
		"java": "java",
		"rb":   "ruby",
		"sh":   "bash",
		"yaml": "yaml",
		"yml":  "yaml",
		"json": "json",
		"toml": "toml",
		"md":   "markdown",
		"html": "html",
		"css":  "css",
		"sql":  "sql",
		"tf":   "hcl",
		"lua":  "lua",
		"php":  "php",
		"cs":   "csharp",
		"kt":   "kotlin",
		"swift": "swift",
	}
	if l, ok := m[ext]; ok {
		return l
	}
	return ""
}
