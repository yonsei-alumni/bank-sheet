package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

const fallbackChromeVersion = "131.0.6778.85"

var chromeVersionPattern = regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)`)

// linuxChromeBinaries is the lookup order for a Chrome/Chromium binary on Linux.
var linuxChromeBinaries = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
}

// detectChromeUA builds a User-Agent string matching the system Chrome. Version
// is queried from the binary at runtime; OS token comes from runtime.GOOS.
// Falls back to fallbackChromeVersion if lookup fails.
func detectChromeUA() string {
	version := detectChromeVersion()
	return fmt.Sprintf(
		"Mozilla/5.0 %s AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36",
		osToken(), version,
	)
}

func osToken() string {
	switch runtime.GOOS {
	case "linux":
		return "(X11; Linux x86_64)"
	case "darwin":
		return "(Macintosh; Intel Mac OS X 10_15_7)"
	case "windows":
		return "(Windows NT 10.0; Win64; x64)"
	default:
		return "(X11; Linux x86_64)"
	}
}

func detectChromeVersion() string {
	binaries := chromeBinaries()
	for _, name := range binaries {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		out, err := exec.Command(path, "--version").Output()
		if err != nil {
			continue
		}
		if m := chromeVersionPattern.FindString(string(out)); m != "" {
			return m
		}
	}
	return fallbackChromeVersion
}

func chromeBinaries() []string {
	switch runtime.GOOS {
	case "linux":
		return linuxChromeBinaries
	case "darwin":
		return []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
	case "windows":
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	default:
		return nil
	}
}

// LaunchSystemChrome opens the given URL in the system Chrome. Used to guide the
// user through Shinhan cert registration when the automated profile lacks it.
func LaunchSystemChrome(url string) error {
	switch runtime.GOOS {
	case "linux":
		for _, name := range linuxChromeBinaries {
			if path, err := exec.LookPath(name); err == nil {
				return exec.Command(path, url).Start()
			}
		}
		return fmt.Errorf("Chrome/Chromium 바이너리를 찾을 수 없습니다")
	case "darwin":
		return exec.Command("open", "-a", "Google Chrome", url).Start()
	case "windows":
		for _, path := range chromeBinaries() {
			if cmd := exec.Command(path, url); cmd.Start() == nil {
				return nil
			}
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("지원하지 않는 OS: %s", runtime.GOOS)
	}
}

// sanitizeFilename replaces characters unsafe for filenames on common OSes.
func sanitizeFilename(s string) string {
	r := strings.NewReplacer(
		"/", "_", `\`, "_", ":", "_", "*", "_",
		"?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
	)
	out := r.Replace(s)
	out = strings.TrimSpace(out)
	if out == "" {
		return "untitled"
	}
	return out
}
