package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var screenshotCmd = &cobra.Command{
	Use:   "screenshot [output.png]",
	Short: "Capture a fitted screenshot of the viewer canvas (headless Chrome)",
	Long: `Opens the running code-view page in headless Chrome, calls the same
"fit all" logic the browser UI uses to zoom/pan every card into view, then
captures a screenshot of just the viewport (not the whole OS window).

Requires Google Chrome (or Chromium) installed locally, and 'code-view serve'
already running.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := "code-view-screenshot.png"
		if len(args) == 1 {
			out = args[0]
		}
		width, _ := cmd.Flags().GetInt("width")
		height, _ := cmd.Flags().GetInt("height")
		wait, _ := cmd.Flags().GetDuration("wait")

		url := baseURL(cmd) + "/"

		allocCtx, cancelAlloc := chromedp.NewExecAllocator(
			context.Background(),
			append(chromedp.DefaultExecAllocatorOptions[:], chromedp.WindowSize(width, height))...,
		)
		defer cancelAlloc()

		ctx, cancel := chromedp.NewContext(allocCtx)
		defer cancel()

		ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
		defer cancelTimeout()

		var buf []byte
		err := chromedp.Run(ctx,
			chromedp.Navigate(url),
			chromedp.Sleep(wait),
			chromedp.Evaluate(`typeof fitAll === "function" && fitAll()`, nil),
			chromedp.Sleep(200*time.Millisecond),
			// Element-scoped chromedp.Screenshot (Page.captureScreenshot with a
			// DOM.getBoxModel-derived clip) hangs against this Chrome build --
			// plain CaptureScreenshot avoids the box-model round trip entirely
			// and just grabs the current viewport, which fitAll() has already
			// framed to the content.
			chromedp.CaptureScreenshot(&buf),
		)
		if err != nil {
			return fmt.Errorf("screenshot failed (is Chrome installed, and is 'code-view serve' running at %s?): %w", url, err)
		}

		if err := os.WriteFile(out, buf, 0o644); err != nil {
			return err
		}
		fmt.Println("saved", out)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(screenshotCmd)
	screenshotCmd.Flags().Int("port", 7878, "Server port")
	screenshotCmd.Flags().Int("width", 1600, "Headless browser viewport width")
	screenshotCmd.Flags().Int("height", 1000, "Headless browser viewport height")
	screenshotCmd.Flags().Duration("wait", 500*time.Millisecond, "How long to wait for content to render before fitting/capturing")
}
