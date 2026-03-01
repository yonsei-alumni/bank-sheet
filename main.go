package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type largeTheme struct {
	fyne.Theme
}

func (t *largeTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.Theme.Size(name) * 1.3
}

func main() {
	a := app.NewWithID("com.github.benelog.bank-sheet")
	a.Settings().SetTheme(&largeTheme{Theme: theme.DefaultTheme()})
	w := a.NewWindow("통장 거래내역 변환기")
	w.Resize(fyne.NewSize(600, 400))

	message := widget.NewMultiLineEntry()
	message.SetPlaceHolder("선택된 파일 정보가 여기에 표시됩니다.")
	message.Wrapping = fyne.TextWrapWord

	bankType := widget.NewRadioGroup([]string{"신한은행 모임통장"}, func(selected string) {})
	bankType.SetSelected("신한은행 모임통장")
	bankType.Required = true

	btn := widget.NewButton("HTML 파일 선택", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				message.SetText(fmt.Sprintf("[오류] %v", err))
				return
			}
			if reader == nil {
				return // 취소됨
			}
			defer reader.Close()

			path := reader.URI().Path()
			log := func(msg string) {
				message.SetText(message.Text + msg + "\n")
			}

			message.SetText("")
			log(fmt.Sprintf("[파일 선택] %s", filepath.Base(path)))
			log(fmt.Sprintf("[경로] %s", path))

			selected := bankType.Selected
			if selected == "" {
				log("\n[오류] 은행 유형을 선택해주세요.")
				return
			}
			log(fmt.Sprintf("[은행 유형] %s", selected))
			log("")
			log("HTML 파일을 분석하고 있습니다...")

			transactions, err := ParseShinhanMoim(reader)
			if err != nil {
				log(fmt.Sprintf("[오류] %v", err))
				return
			}

			log(fmt.Sprintf("총 %d건의 거래 내역을 찾았습니다.", len(transactions)))
			log("")

			for i, txn := range transactions {
				log(fmt.Sprintf("  %d. %s | %s | %s원 | 잔액 %s원",
					i+1,
					txn.DateTime.Format("2006-01-02 15:04:05"),
					txn.Depositor,
					formatComma(txn.Amount),
					formatComma(txn.Balance),
				))
			}

			dir := filepath.Dir(path)
			baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			outputPath := uniqueFilename(filepath.Join(dir, baseName+".xlsx"))

			log("")
			log("엑셀 파일을 생성하고 있습니다...")

			err = WriteExcel(transactions, outputPath)
			if err != nil {
				log(fmt.Sprintf("[오류] 엑셀 파일 생성 실패: %v", err))
				return
			}

			log("엑셀 파일이 생성되었습니다!")
			log(fmt.Sprintf("[저장 위치] %s", outputPath))

			dialog.ShowConfirm("파일 열기", "생성된 파일을 열까요?", func(yes bool) {
				if yes {
					if err := openFile(outputPath); err != nil {
						log(fmt.Sprintf("[오류] 파일 열기 실패: %v", err))
					}
				}
			}, w)
		}, w)

		fd.SetFilter(storage.NewExtensionFileFilter([]string{".html", ".htm"}))
		fd.Show()
	})

	content := container.NewBorder(
		container.NewVBox(bankType, btn),
		nil, nil, nil,
		message,
	)
	w.SetContent(content)
	w.ShowAndRun()
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

func formatComma(n int64) string {
	if n < 0 {
		return "-" + formatComma(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}
