package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// WriteExcel writes transactions to an Excel file.
func WriteExcel(transactions []Transaction, outputPath string) error {
	f := excelize.NewFile()
	defer f.Close()
	if err := fillSheet(f, "Sheet1", transactions); err != nil {
		return err
	}
	return f.SaveAs(outputPath)
}

// WriteExcelMultiSheet writes one sheet per account into a single workbook.
// Sheet names are sanitized to Excel's 31-char / forbidden-char rules. Order of
// sheets follows `order`; any accounts in `transactions` not in `order` are
// appended alphabetically.
func WriteExcelMultiSheet(transactions map[string][]Transaction, order []string, outputPath string) error {
	f := excelize.NewFile()
	defer f.Close()

	seen := map[string]bool{}
	names := []string{}
	for _, name := range order {
		if _, ok := transactions[name]; ok && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	for name := range transactions {
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}

	usedSheet := map[string]bool{}
	for i, name := range names {
		sheet := uniqueSheetName(sanitizeSheetName(name), usedSheet)
		usedSheet[sheet] = true
		if i == 0 {
			// Rename the default Sheet1 to the first account's sheet name.
			if err := f.SetSheetName("Sheet1", sheet); err != nil {
				return err
			}
		} else {
			if _, err := f.NewSheet(sheet); err != nil {
				return err
			}
		}
		if err := fillSheet(f, sheet, transactions[name]); err != nil {
			return err
		}
	}
	return f.SaveAs(outputPath)
}

func fillSheet(f *excelize.File, sheet string, transactions []Transaction) error {
	headers := []string{"입금일자", "입금자", "입금액", "잔액"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err := f.SetCellStyle(sheet, "A1", "D1", headerStyle); err != nil {
		return err
	}

	numberStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 3}) // #,##0

	for i, txn := range transactions {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), txn.DateTime.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), txn.Depositor)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), txn.Amount)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), txn.Balance)
		f.SetCellStyle(sheet, fmt.Sprintf("C%d", row), fmt.Sprintf("D%d", row), numberStyle)
	}

	f.SetColWidth(sheet, "A", "A", 22)
	f.SetColWidth(sheet, "B", "B", 18)
	f.SetColWidth(sheet, "C", "C", 15)
	f.SetColWidth(sheet, "D", "D", 15)
	return nil
}

// sanitizeSheetName replaces characters Excel forbids in sheet names and
// truncates to the 31-character limit.
func sanitizeSheetName(name string) string {
	r := strings.NewReplacer(
		"/", "_", `\`, "_", "[", "(", "]", ")",
		":", "_", "*", "_", "?", "_",
	)
	out := strings.TrimSpace(r.Replace(name))
	if out == "" {
		out = "Sheet"
	}
	// Truncate by rune so we don't cut a multi-byte character in half.
	runes := []rune(out)
	if len(runes) > 31 {
		runes = runes[:31]
	}
	return string(runes)
}

func uniqueSheetName(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	for i := 2; i < 1000; i++ {
		suffix := fmt.Sprintf(" (%d)", i)
		room := 31 - len([]rune(suffix))
		trunk := []rune(base)
		if len(trunk) > room {
			trunk = trunk[:room]
		}
		candidate := string(trunk) + suffix
		if !used[candidate] {
			return candidate
		}
	}
	return base
}

// uniqueFilename returns a unique filename by appending (1), (2), etc.
// if the file already exists, like browser download behavior.
func uniqueFilename(basePath string) string {
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return basePath
	}

	ext := filepath.Ext(basePath)
	name := strings.TrimSuffix(basePath, ext)

	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s(%d)%s", name, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
