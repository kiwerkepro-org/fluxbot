package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
)

// OpenBrowser öffnet einen Browser (Chrome/Chromium) auf dem lokalen System.
// Funktioniert auf Windows, macOS und Linux.
func OpenBrowser(url string) error {
	if url == "" {
		url = "http://localhost:9090"
	}

	switch runtime.GOOS {
	case "windows":
		// Windows: `start chrome <url>`
		cmd := exec.Command("cmd", "/c", "start", "chrome", url)
		// Suppress output
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			// Fallback: versuche Chrome vom ProgramFiles-Pfad
			chromePath := `C:\Program Files\Google\Chrome\Application\chrome.exe`
			if _, err := os.Stat(chromePath); err == nil {
				cmd := exec.Command(chromePath, url)
				cmd.Stdout = nil
				cmd.Stderr = nil
				return cmd.Start()
			}
			log.Printf("[System] ⚠️  Chrome konnte nicht geöffnet werden: %v", err)
			return fmt.Errorf("Chrome nicht gefunden")
		}
		return nil

	case "darwin":
		// macOS: `open -a "Google Chrome" <url>`
		cmd := exec.Command("open", "-a", "Google Chrome", url)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			log.Printf("[System] ⚠️  Chrome konnte nicht geöffnet werden: %v", err)
			return fmt.Errorf("Chrome nicht gefunden")
		}
		return nil

	case "linux":
		// Linux: `google-chrome <url>` oder `chromium-browser <url>`
		browsers := []string{"google-chrome", "chromium-browser", "chromium"}
		for _, browser := range browsers {
			cmd := exec.Command(browser, url)
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
		log.Printf("[System] ⚠️  Kein Browser gefunden")
		return fmt.Errorf("Chrome/Chromium nicht gefunden")

	default:
		return fmt.Errorf("Plattform nicht unterstützt: %s", runtime.GOOS)
	}
}
