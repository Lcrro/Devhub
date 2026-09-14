package capture

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Capturer struct{}

func New() *Capturer { return &Capturer{} }

func (c *Capturer) Capture(parent context.Context, url, output string) error {
	browser, err := findBrowser()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		return err
	}
	_ = os.Remove(output)

	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	args := []string{
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--hide-scrollbars",
		"--no-first-run", "--no-default-browser-check", "--window-size=1280,720",
		"--virtual-time-budget=2500", "--screenshot=" + output,
	}
	if isElevatedRoot() {
		args = append(args, "--no-sandbox")
	}
	args = append(args, url)

	cmd := exec.Command(browser, args...)
	configureBrowserProcess(cmd)
	var outputLog bytes.Buffer
	cmd.Stdout = &outputLog
	cmd.Stderr = &outputLog
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start browser capture: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			if fileReady(output) {
				return nil
			}
			if err == nil {
				err = fmt.Errorf("browser exited before creating screenshot")
			}
			return fmt.Errorf("browser capture failed: %w: %s", err, tail(outputLog.String(), 1800))
		case <-ticker.C:
			if fileReady(output) {
				_ = terminateBrowserProcess(cmd)
				return nil
			}
		case <-ctx.Done():
			_ = terminateBrowserProcess(cmd)
			return fmt.Errorf("browser capture timed out: %w: %s", ctx.Err(), tail(outputLog.String(), 1800))
		}
	}
}

func fileReady(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func tail(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}

func findBrowser() (string, error) {
	names := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "msedge"}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	candidates := []string{}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium")
	}
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no supported Chrome, Chromium, or Edge browser found")
}
