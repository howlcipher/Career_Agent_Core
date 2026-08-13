//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mxschmitt/playwright-go"
)

type viewport struct {
	name   string
	width  int
	height int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: screenshot <url>")
		return
	}
	url := os.Args[1]
	outDir := "/tmp/cac_screenshots"
	_ = os.MkdirAll(outDir, 0755)

	pw, err := playwright.Run()
	if err != nil {
		fmt.Println("playwright run:", err)
		return
	}
	defer func() { _ = pw.Stop() }()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args:     []string{"--no-sandbox"},
	})
	if err != nil {
		fmt.Println("launch:", err)
		return
	}
	defer func() { _ = browser.Close() }()

	viewports := []viewport{
		{"desktop-1920", 1920, 1080},
		{"laptop-1280", 1280, 800},
		{"tablet-768", 768, 1024},
		{"mobile-390", 390, 844},
	}

	for _, vp := range viewports {
		ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
			Viewport: &playwright.Size{Width: vp.width, Height: vp.height},
		})
		if err != nil {
			fmt.Println("context:", err)
			continue
		}
		page, err := ctx.NewPage()
		if err != nil {
			_ = ctx.Close()
			fmt.Println("page:", err)
			continue
		}
		if _, err := page.Goto(url, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateNetworkidle,
		}); err != nil {
			fmt.Println("goto:", err)
			_ = page.Close()
			_ = ctx.Close()
			continue
		}
		path := filepath.Join(outDir, fmt.Sprintf("%s.png", vp.name))
		if _, err := page.Screenshot(playwright.PageScreenshotOptions{
			Path:     playwright.String(path),
			FullPage: playwright.Bool(true),
		}); err != nil {
			fmt.Println("screenshot:", err)
		} else {
			fmt.Println("saved", path)
		}
		_ = page.Close()
		_ = ctx.Close()
	}
}
