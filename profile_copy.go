package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const profileDirName = "shinhan-profile"

// chromeProfileSrc returns (profileRoot, defaultDir) for the system Chrome
// install on this OS. `profileRoot` contains Local State; `defaultDir` is the
// per-profile directory with Cookies, Preferences, IndexedDB, etc.
func chromeProfileSrc() (profileRoot, defaultDir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	switch runtime.GOOS {
	case "linux":
		profileRoot = filepath.Join(home, ".config", "google-chrome")
	case "darwin":
		profileRoot = filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		profileRoot = filepath.Join(local, "Google", "Chrome", "User Data")
	default:
		return "", "", fmt.Errorf("지원하지 않는 OS: %s", runtime.GOOS)
	}
	defaultDir = filepath.Join(profileRoot, "Default")
	if _, err := os.Stat(defaultDir); err != nil {
		return "", "", fmt.Errorf("Chrome 프로필을 찾을 수 없습니다: %s", defaultDir)
	}
	return profileRoot, defaultDir, nil
}

// localProfileDst returns the absolute path to ./shinhan-profile next to the
// program's current working directory.
func localProfileDst() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, profileDirName), nil
}

// CopyMinimalChromeProfile mirrors only the files Shinhan's browser-cert login
// needs from an existing Chrome profile into dstRoot. The destination layout
// matches Chrome's so Playwright can boot the profile directly.
//
// Copied:
//   - {src}/Local State
//   - {src}/Default/Preferences
//   - {src}/Default/Cookies[-journal]
//   - {src}/Default/Local Storage/leveldb/*
//   - {src}/Default/Session Storage/*                        (if present)
//   - {src}/Default/IndexedDB/https_*shinhan*.leveldb/*      (origin-filtered)
//
// Progress messages go to log if non-nil.
func CopyMinimalChromeProfile(srcRoot, srcDefault, dstRoot string, log func(string)) error {
	if log == nil {
		log = func(string) {}
	}
	dstDefault := filepath.Join(dstRoot, "Default")
	if err := os.MkdirAll(dstDefault, 0o755); err != nil {
		return err
	}

	log("  Local State 복사...")
	if err := copyFile(
		filepath.Join(srcRoot, "Local State"),
		filepath.Join(dstRoot, "Local State"),
	); err != nil {
		return fmt.Errorf("Local State 복사 실패: %w", err)
	}
	log("  Preferences 복사...")
	if err := copyFile(
		filepath.Join(srcDefault, "Preferences"),
		filepath.Join(dstDefault, "Preferences"),
	); err != nil {
		return fmt.Errorf("Preferences 복사 실패: %w", err)
	}
	log("  Cookies 복사...")
	if err := copyFile(
		filepath.Join(srcDefault, "Cookies"),
		filepath.Join(dstDefault, "Cookies"),
	); err != nil {
		return fmt.Errorf("Cookies 복사 실패: %w", err)
	}
	// Cookies-journal may not exist on a cleanly-closed profile.
	_ = copyFile(
		filepath.Join(srcDefault, "Cookies-journal"),
		filepath.Join(dstDefault, "Cookies-journal"),
	)

	log("  Local Storage 복사...")
	if err := copyDirContents(
		filepath.Join(srcDefault, "Local Storage", "leveldb"),
		filepath.Join(dstDefault, "Local Storage", "leveldb"),
		nil,
	); err != nil {
		return fmt.Errorf("Local Storage 복사 실패: %w", err)
	}

	log("  Session Storage 복사...")
	_ = copyDirContents(
		filepath.Join(srcDefault, "Session Storage"),
		filepath.Join(dstDefault, "Session Storage"),
		nil,
	)

	srcIDB := filepath.Join(srcDefault, "IndexedDB")
	dstIDB := filepath.Join(dstDefault, "IndexedDB")
	entries, err := os.ReadDir(srcIDB)
	if err != nil {
		return fmt.Errorf("IndexedDB 디렉토리 읽기 실패: %w", err)
	}
	if err := os.MkdirAll(dstIDB, 0o755); err != nil {
		return err
	}
	shinhanFound := false
	for _, e := range entries {
		if !e.IsDir() || !strings.Contains(e.Name(), "shinhan") {
			continue
		}
		shinhanFound = true
		if err := copyDirContents(
			filepath.Join(srcIDB, e.Name()),
			filepath.Join(dstIDB, e.Name()),
			nil,
		); err != nil {
			return fmt.Errorf("IndexedDB %s 복사 실패: %w", e.Name(), err)
		}
		log(fmt.Sprintf("  IndexedDB 복사: %s", e.Name()))
	}
	if !shinhanFound {
		log("  [안내] IndexedDB에 shinhan 관련 항목이 없습니다. 인증서 미등록 가능성.")
	}
	return nil
}

// copyFile copies one file, creating parent dirs. Returns os.IsNotExist-wrapped
// error if src does not exist (caller may ignore for optional files).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// copyDirContents recursively copies src into dst. Missing src is a no-op.
// keep, if non-nil, is a per-entry filter: return false to skip.
func copyDirContents(src, dst string, keep func(name string) bool) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("디렉토리가 아님: %s", src)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if keep != nil && !keep(e.Name()) {
			continue
		}
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDirContents(s, d, keep); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
}
