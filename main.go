package main

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	periodOptions = []string{"1개월", "3개월", "6개월"}
	sortOptions   = []string{"과거순", "최신순"}
	outputOptions = []string{"계좌별 파일", "하나의 엑셀(시트별)"}
	pinPattern    = regexp.MustCompile(`^\d{6}$`)
)

// Shinhan-inspired accent palette used for the primary action button, focus
// rings, and numbered step badges.
var (
	shinhanBlue      = color.NRGBA{R: 0x00, G: 0x46, B: 0xFF, A: 0xFF}
	shinhanBlueSoft  = color.NRGBA{R: 0x00, G: 0x46, B: 0xFF, A: 0x1A}
	shinhanBlueFocus = color.NRGBA{R: 0x00, G: 0x46, B: 0xFF, A: 0x55}
	neutralDotColor  = color.NRGBA{R: 0x9A, G: 0xA0, B: 0xA6, A: 0xFF}
)

// shinhanTheme overrides the default Fyne theme with Shinhan's brand blue and
// adds slightly more generous padding for a calmer layout.
type shinhanTheme struct{ fyne.Theme }

func (t *shinhanTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary:
		return shinhanBlue
	case theme.ColorNameFocus:
		return shinhanBlueFocus
	case theme.ColorNameHover, theme.ColorNameSelection:
		return shinhanBlueSoft
	}
	return t.Theme.Color(name, variant)
}

func (t *shinhanTheme) Size(name fyne.ThemeSizeName) float32 {
	base := t.Theme.Size(name)
	if name == theme.SizeNamePadding {
		return base * 1.5
	}
	return base * 1.3
}

// fixedSizeLayout forces its children to an exact pixel size. Used for the
// circular step badges and the status dot so they render at a consistent
// diameter regardless of child MinSize.
type fixedSizeLayout struct{ w, h float32 }

func (f *fixedSizeLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.w, f.h)
}

func (f *fixedSizeLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

func stepBadge(n int) fyne.CanvasObject {
	circle := canvas.NewCircle(shinhanBlue)
	num := canvas.NewText(fmt.Sprintf("%d", n), color.White)
	num.Alignment = fyne.TextAlignCenter
	num.TextStyle = fyne.TextStyle{Bold: true}
	num.TextSize = 14
	return container.New(&fixedSizeLayout{w: 30, h: 30}, circle, container.NewCenter(num))
}

func sectionHeader(n int, title, hint string) fyne.CanvasObject {
	titleLbl := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	children := []fyne.CanvasObject{}
	if n > 0 {
		children = append(children, stepBadge(n))
	}
	children = append(children, titleLbl)
	if hint != "" {
		children = append(children, widget.NewLabelWithStyle(hint, fyne.TextAlignLeading, fyne.TextStyle{Italic: true}))
	}
	return container.NewHBox(children...)
}

func section(n int, title, hint string, content fyne.CanvasObject) fyne.CanvasObject {
	// Indent content under the step badge so the workflow reads as a clear
	// numbered list. When n == 0 we still indent to keep vertical rhythm.
	indent := canvas.NewRectangle(color.Transparent)
	indent.SetMinSize(fyne.NewSize(38, 0))
	body := container.NewBorder(nil, nil, indent, nil, content)
	return container.NewVBox(sectionHeader(n, title, hint), body)
}

type appState struct {
	w       fyne.Window
	browser *ShinhanBrowser
	saveDir string

	pinEntry     *widget.Entry
	headfulCheck *widget.Check
	loginBtn     *widget.Button

	accountGroup *widget.CheckGroup
	selectAllBtn *widget.Button

	periodRadio *widget.RadioGroup
	sortRadio   *widget.RadioGroup
	outputRadio *widget.RadioGroup

	saveDirLabel *widget.Label
	saveDirBtn   *widget.Button

	downloadBtn *widget.Button
	logEntry    *widget.Entry
	logScroll   *container.Scroll
	logBuf      strings.Builder

	statusLabel *widget.Label
	statusDot   *canvas.Circle
	progressBar *widget.ProgressBarInfinite
}

func main() {
	a := app.NewWithID("com.github.benelog.bank-sheet")
	a.Settings().SetTheme(&shinhanTheme{Theme: theme.DefaultTheme()})
	w := a.NewWindow("신한은행 거래내역 → 엑셀")
	w.Resize(fyne.NewSize(820, 940))

	cwd, _ := os.Getwd()

	s := &appState{w: w, saveDir: cwd}

	// Step 1 — login
	s.pinEntry = widget.NewPasswordEntry()
	s.pinEntry.SetPlaceHolder("6자리 PIN")
	s.headfulCheck = widget.NewCheck("브라우저 창 표시 (디버그)", nil)
	s.loginBtn = widget.NewButtonWithIcon("로그인 & 통장 불러오기", theme.LoginIcon(), s.onLogin)

	// Step 2 — account selection
	s.accountGroup = widget.NewCheckGroup(nil, nil)
	s.accountGroup.Horizontal = false
	accountScroll := container.NewVScroll(s.accountGroup)
	accountScroll.SetMinSize(fyne.NewSize(0, 180))

	s.selectAllBtn = widget.NewButtonWithIcon("모두 선택 / 해제", theme.ConfirmIcon(), func() {
		if len(s.accountGroup.Selected) == len(s.accountGroup.Options) {
			s.accountGroup.SetSelected(nil)
		} else {
			s.accountGroup.SetSelected(append([]string{}, s.accountGroup.Options...))
		}
	})
	s.selectAllBtn.Disable()

	// Step 3 — options
	s.periodRadio = widget.NewRadioGroup(periodOptions, nil)
	s.periodRadio.Horizontal = true
	s.periodRadio.SetSelected("6개월")

	s.sortRadio = widget.NewRadioGroup(sortOptions, nil)
	s.sortRadio.Horizontal = true
	s.sortRadio.SetSelected("과거순")

	s.outputRadio = widget.NewRadioGroup(outputOptions, nil)
	s.outputRadio.Horizontal = true
	s.outputRadio.SetSelected("계좌별 파일")

	s.saveDirLabel = widget.NewLabel(s.saveDir)
	s.saveDirLabel.Truncation = fyne.TextTruncateClip
	s.saveDirBtn = widget.NewButtonWithIcon("폴더 변경", theme.FolderOpenIcon(), s.onPickSaveDir)

	// Primary action
	s.downloadBtn = widget.NewButtonWithIcon("엑셀로 다운로드", theme.DownloadIcon(), s.onDownload)
	s.downloadBtn.Importance = widget.HighImportance
	s.downloadBtn.Disable()

	// Log panel — a non-disabled MultiLineEntry so text stays at full
	// foreground contrast and Korean renders with the proportional font.
	// We never read back from the entry; log() appends to logBuf and rewrites
	// the entry from it, so any accidental user edits don't corrupt history.
	s.logEntry = widget.NewMultiLineEntry()
	s.logEntry.Wrapping = fyne.TextWrapWord
	s.logEntry.SetPlaceHolder("진행 상황이 여기에 표시됩니다.")
	s.logScroll = container.NewVScroll(s.logEntry)
	s.logScroll.SetMinSize(fyne.NewSize(0, 200))

	// Status footer
	s.statusDot = canvas.NewCircle(neutralDotColor)
	statusDotWrap := container.New(&fixedSizeLayout{w: 12, h: 12}, s.statusDot)
	s.statusLabel = widget.NewLabel("대기 중")
	s.statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	s.progressBar = widget.NewProgressBarInfinite()
	s.progressBar.Stop()
	s.progressBar.Hide()

	// Composition
	loginRow := container.NewBorder(nil, nil,
		widget.NewLabel("PIN"), s.loginBtn,
		s.pinEntry,
	)
	loginBody := container.NewVBox(loginRow, s.headfulCheck)

	accountBody := container.NewVBox(
		accountScroll,
		container.NewHBox(s.selectAllBtn),
	)

	optionsBody := container.New(
		&twoColForm{},
		widget.NewLabel("조회 기간"), s.periodRadio,
		widget.NewLabel("정렬"), s.sortRadio,
		widget.NewLabel("출력"), s.outputRadio,
		widget.NewLabel("저장 위치"),
		container.NewBorder(nil, nil, nil, s.saveDirBtn, s.saveDirLabel),
	)

	// App title header
	topTitle := widget.NewRichText(
		&widget.TextSegment{
			Style: widget.RichTextStyle{
				TextStyle: fyne.TextStyle{Bold: true},
				SizeName:  theme.SizeNameHeadingText,
			},
			Text: "신한은행 거래내역 변환",
		},
	)
	topSubtitle := widget.NewLabelWithStyle(
		"모임통장 HTML 거래내역을 엑셀(xlsx)로 변환합니다.",
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
	)
	topHeader := container.NewVBox(
		container.NewPadded(container.NewVBox(topTitle, topSubtitle)),
		widget.NewSeparator(),
	)

	// Sections
	sections := container.NewVBox(
		section(1, "로그인", "· 신한은행에 로그인합니다.", loginBody),
		widget.NewSeparator(),
		section(2, "통장 선택", "· 다운로드할 모임통장을 고릅니다.", accountBody),
		widget.NewSeparator(),
		section(3, "조건 설정", "· 조회 기간, 정렬, 저장 위치를 지정합니다.", optionsBody),
		container.NewPadded(s.downloadBtn),
		widget.NewSeparator(),
		section(0, "진행 로그", "· 작업이 시작되면 여기에 상태가 표시됩니다.", s.logScroll),
	)

	// Status footer
	statusRow := container.NewBorder(nil, nil,
		container.NewHBox(statusDotWrap, s.statusLabel),
		nil,
		s.progressBar,
	)
	footer := container.NewVBox(widget.NewSeparator(), container.NewPadded(statusRow))

	body := container.NewVScroll(container.NewPadded(sections))

	content := container.NewBorder(topHeader, footer, nil, nil, body)
	w.SetContent(content)

	w.SetCloseIntercept(func() {
		if s.browser != nil {
			go func() { _ = s.browser.Close() }()
		}
		w.Close()
	})

	w.ShowAndRun()
}

func (s *appState) log(msg string) {
	fyne.Do(func() {
		if s.logBuf.Len() > 0 {
			s.logBuf.WriteByte('\n')
		}
		s.logBuf.WriteString(msg)
		s.logEntry.SetText(s.logBuf.String())
		s.logEntry.CursorRow = strings.Count(s.logBuf.String(), "\n")
		s.logScroll.ScrollToBottom()
	})
}

func (s *appState) logf(format string, args ...any) {
	s.log(fmt.Sprintf(format, args...))
}

// setStatus updates the footer status line. busy=true animates the progress
// bar and turns the status dot blue; busy=false hides the bar and resets the
// dot to a neutral gray.
func (s *appState) setStatus(msg string, busy bool) {
	fyne.Do(func() {
		if busy {
			if msg == "" {
				msg = "작업 중..."
			}
			s.statusLabel.SetText(msg)
			s.statusDot.FillColor = shinhanBlue
			s.statusDot.Refresh()
			s.progressBar.Show()
			s.progressBar.Start()
		} else {
			s.progressBar.Stop()
			s.progressBar.Hide()
			if msg == "" {
				msg = "대기 중"
			}
			s.statusLabel.SetText(msg)
			s.statusDot.FillColor = neutralDotColor
			s.statusDot.Refresh()
		}
	})
}

func (s *appState) onPickSaveDir() {
	d := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		s.saveDir = uri.Path()
		s.saveDirLabel.SetText(s.saveDir)
	}, s.w)
	if s.saveDir != "" {
		if uri, err := storage.ListerForURI(storage.NewFileURI(s.saveDir)); err == nil {
			d.SetLocation(uri)
		}
	}
	d.Show()
}

func (s *appState) onLogin() {
	pin := strings.TrimSpace(s.pinEntry.Text)
	if !pinPattern.MatchString(pin) {
		dialog.ShowError(errors.New("PIN은 6자리 숫자여야 합니다."), s.w)
		return
	}
	s.loginBtn.Disable()
	s.downloadBtn.Disable()
	s.selectAllBtn.Disable()
	s.accountGroup.Options = nil
	s.accountGroup.Refresh()

	headful := s.headfulCheck.Checked

	go func() {
		success := false
		defer func() {
			fyne.Do(func() {
				s.loginBtn.Enable()
				if success {
					s.selectAllBtn.Enable()
					s.downloadBtn.Enable()
				}
			})
			if success {
				s.setStatus("로그인 완료. 통장을 선택하세요.", false)
			} else {
				s.setStatus("대기 중", false)
			}
		}()

		s.setStatus("Chrome 프로필 경로 확인 중...", true)
		srcRoot, srcDef, err := chromeProfileSrc()
		if err != nil {
			s.log("[오류] " + err.Error())
			return
		}
		dstRoot, err := localProfileDst()
		if err != nil {
			s.log("[오류] " + err.Error())
			return
		}

		s.setStatus("Chrome 프로필 복사 중...", true)
		s.logf("Chrome 프로필 최소 복사 → %s", dstRoot)
		if err := CopyMinimalChromeProfile(srcRoot, srcDef, dstRoot, s.log); err != nil {
			s.log("[오류] 프로필 복사 실패: " + err.Error())
			return
		}

		s.setStatus("Playwright Chromium 준비 중... (첫 실행 시 ~170MB 다운로드)", true)
		browser, err := NewShinhanBrowser(dstRoot, headful, s.log)
		if err != nil {
			s.log("[오류] " + err.Error())
			return
		}

		// Replace any previous session.
		if s.browser != nil {
			_ = s.browser.Close()
		}
		s.browser = browser

		s.setStatus("신한은행 로그인 중...", true)
		s.log("신한은행 로그인 시도...")
		status, err := browser.Login(pin)
		if err != nil {
			s.log("[오류] 로그인 실패: " + err.Error())
			_ = browser.Close()
			s.browser = nil
			return
		}
		if status == LoginNoCert {
			s.log("[안내] 신한인증서가 등록되지 않았습니다.")
			_ = browser.Close()
			s.browser = nil
			fyne.Do(s.showNoCertDialog)
			return
		}
		s.log("로그인 성공. 통장 목록 조회 중...")

		s.setStatus("통장 목록 조회 중...", true)
		accounts, err := browser.ListAccounts()
		if err != nil {
			s.log("[오류] 통장 목록 조회 실패: " + err.Error())
			return
		}
		s.logf("통장 %d개 조회됨", len(accounts))

		fyne.Do(func() {
			s.accountGroup.Options = accounts
			s.accountGroup.Refresh()
		})
		success = true
	}()
}

func (s *appState) showNoCertDialog() {
	msg := widget.NewLabel(
		"이 PC의 Chrome에 신한인증서가 아직 등록되지 않았습니다.\n\n" +
			"Chrome을 실행해 신한은행 모바일 페이지에서\n" +
			"먼저 신한인증서를 발급/등록한 뒤\n" +
			"이 앱에서 다시 로그인해주세요.",
	)
	msg.Wrapping = fyne.TextWrapWord
	dialog.NewCustomConfirm(
		"신한인증서 미등록",
		"Chrome 실행",
		"취소",
		msg,
		func(ok bool) {
			if !ok {
				return
			}
			if err := LaunchSystemChrome(shinhanLoginURL); err != nil {
				dialog.ShowError(fmt.Errorf("Chrome 실행 실패: %w", err), s.w)
			}
		},
		s.w,
	).Show()
}

func (s *appState) onDownload() {
	if s.browser == nil {
		dialog.ShowError(errors.New("먼저 로그인하세요."), s.w)
		return
	}
	selected := append([]string{}, s.accountGroup.Selected...)
	if len(selected) == 0 {
		dialog.ShowError(errors.New("최소 1개의 통장을 선택하세요."), s.w)
		return
	}
	period := s.periodRadio.Selected
	sortOrder := s.sortRadio.Selected
	outputMode := s.outputRadio.Selected
	saveDir := s.saveDir

	s.downloadBtn.Disable()
	s.loginBtn.Disable()

	total := len(selected)

	go func() {
		defer func() {
			fyne.Do(func() {
				s.downloadBtn.Enable()
				s.loginBtn.Enable()
			})
			s.setStatus("대기 중", false)
		}()

		s.setStatus("저장 폴더 준비 중...", true)
		dateStr := time.Now().Format("20060102")
		outDir := filepath.Join(saveDir, dateStr)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			s.log("[오류] 저장 폴더 생성 실패: " + err.Error())
			return
		}

		results := map[string]bool{}
		combined := map[string][]Transaction{}

		for i, account := range selected {
			s.setStatus(fmt.Sprintf("(%d/%d) %s 거래내역 다운로드 중...", i+1, total, account), true)
			s.log("")
			s.logf("=== [%d/%d] %s ===", i+1, total, account)
			html, err := s.browser.DownloadHTML(account, period, sortOrder)
			if err != nil {
				s.logf("[오류] 다운로드 실패: %v", err)
				results[account] = false
				continue
			}
			txns, err := ParseShinhanMoim(strings.NewReader(html))
			if err != nil {
				s.logf("[오류] 파싱 실패: %v", err)
				results[account] = false
				continue
			}
			s.logf("거래 %d건 파싱됨", len(txns))

			if outputMode == outputOptions[0] { // 계좌별 파일
				s.setStatus(fmt.Sprintf("(%d/%d) %s 엑셀 저장 중...", i+1, total, account), true)
				filename := sanitizeFilename(account) + "_" + dateStr + ".xlsx"
				outPath := uniqueFilename(filepath.Join(outDir, filename))
				if err := WriteExcel(txns, outPath); err != nil {
					s.logf("[오류] 엑셀 저장 실패: %v", err)
					results[account] = false
					continue
				}
				s.logf("저장: %s", outPath)
			} else {
				combined[account] = txns
			}
			results[account] = true
		}

		var combinedPath string
		if outputMode == outputOptions[1] && len(combined) > 0 {
			s.setStatus("엑셀 파일(시트별) 생성 중...", true)
			filename := "신한은행_" + dateStr + ".xlsx"
			combinedPath = uniqueFilename(filepath.Join(outDir, filename))
			if err := WriteExcelMultiSheet(combined, selected, combinedPath); err != nil {
				s.logf("[오류] 엑셀(시트별) 저장 실패: %v", err)
				combinedPath = ""
			} else {
				s.logf("저장: %s", combinedPath)
			}
		}

		s.log("")
		s.log("=== 완료 ===")
		allOK := true
		for _, account := range selected {
			tag := "OK"
			if !results[account] {
				tag = "FAIL"
				allOK = false
			}
			s.logf("  %s: %s", account, tag)
		}

		fyne.Do(func() {
			title := "다운로드 완료"
			if !allOK {
				title = "일부 실패"
			}
			dialog.ShowConfirm(title,
				fmt.Sprintf("저장 폴더를 열까요?\n%s", outDir),
				func(yes bool) {
					if !yes {
						return
					}
					if err := openFile(outDir); err != nil {
						dialog.ShowError(err, s.w)
					}
				}, s.w)
		})
	}()
}

func openFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		return fmt.Errorf("지원하지 않는 OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// twoColForm is a minimal 2-column layout: left column sized to widest label,
// right column takes remaining width. Used for the options grid.
type twoColForm struct{}

func (f *twoColForm) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var leftW, rightW, total float32
	for i := 0; i+1 < len(objects); i += 2 {
		l := objects[i].MinSize()
		r := objects[i+1].MinSize()
		if l.Width > leftW {
			leftW = l.Width
		}
		if r.Width > rightW {
			rightW = r.Width
		}
		rowH := l.Height
		if r.Height > rowH {
			rowH = r.Height
		}
		total += rowH + theme.Padding()
	}
	return fyne.NewSize(leftW+rightW+theme.Padding(), total)
}

func (f *twoColForm) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	pad := theme.Padding()
	var leftW float32
	for i := 0; i+1 < len(objects); i += 2 {
		if w := objects[i].MinSize().Width; w > leftW {
			leftW = w
		}
	}
	rightX := leftW + pad
	rightW := size.Width - rightX
	var y float32
	for i := 0; i+1 < len(objects); i += 2 {
		l := objects[i]
		r := objects[i+1]
		lh := l.MinSize().Height
		rh := r.MinSize().Height
		rowH := lh
		if rh > rowH {
			rowH = rh
		}
		l.Move(fyne.NewPos(0, y))
		l.Resize(fyne.NewSize(leftW, rowH))
		r.Move(fyne.NewPos(rightX, y))
		r.Resize(fyne.NewSize(rightW, rowH))
		y += rowH + pad
	}
}
