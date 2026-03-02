package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseShinhanMoim(t *testing.T) {
	f, err := os.Open("testdata/shinhan.html")
	if err != nil {
		t.Fatalf("테스트 파일을 열 수 없습니다: %v", err)
	}
	defer f.Close()

	transactions, err := ParseShinhanMoim(f)
	if err != nil {
		t.Fatalf("파싱 실패: %v", err)
	}

	if len(transactions) != 4 {
		t.Fatalf("거래 건수: got %d, want 4", len(transactions))
	}

	tests := []struct {
		depositor string
		amount    int64
		balance   int64
		dateTime  string
	}{
		{"정상혁(8반)", 1, 100003, "2026-02-19 17:07:00"},
		{"정상혁", 1, 100002, "2026-02-17 12:55:00"},
		{"정상혁(8반)", 100000, 100001, "2026-02-17 08:50:00"},
		{"김용설(2반)", 1, 1, "2026-02-10 11:22:00"},
	}

	for i, tt := range tests {
		txn := transactions[i]
		if txn.Depositor != tt.depositor {
			t.Errorf("[%d] 입금자: got %q, want %q", i, txn.Depositor, tt.depositor)
		}
		if txn.Amount != tt.amount {
			t.Errorf("[%d] 입금액: got %d, want %d", i, txn.Amount, tt.amount)
		}
		if txn.Balance != tt.balance {
			t.Errorf("[%d] 잔액: got %d, want %d", i, txn.Balance, tt.balance)
		}
		if txn.DateTime.Format("2006-01-02 15:04:05") != tt.dateTime {
			t.Errorf("[%d] 입금일자: got %s, want %s", i, txn.DateTime.Format("2006-01-02 15:04:05"), tt.dateTime)
		}
	}
}

func TestWriteExcel(t *testing.T) {
	f, err := os.Open("testdata/shinhan.html")
	if err != nil {
		t.Fatalf("테스트 파일을 열 수 없습니다: %v", err)
	}
	defer f.Close()

	transactions, err := ParseShinhanMoim(f)
	if err != nil {
		t.Fatalf("파싱 실패: %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "test.xlsx")
	err = WriteExcel(transactions, outputPath)
	if err != nil {
		t.Fatalf("엑셀 파일 생성 실패: %v", err)
	}

	xlsx, err := excelize.OpenFile(outputPath)
	if err != nil {
		t.Fatalf("엑셀 파일 열기 실패: %v", err)
	}
	defer xlsx.Close()

	// 헤더 검증
	headerTests := map[string]string{
		"A1": "입금일자",
		"B1": "입금자",
		"C1": "입금액",
		"D1": "잔액",
	}
	for cell, want := range headerTests {
		got, _ := xlsx.GetCellValue("Sheet1", cell)
		if got != want {
			t.Errorf("헤더 %s: got %q, want %q", cell, got, want)
		}
	}

	// 첫 번째 데이터 행 검증
	val, _ := xlsx.GetCellValue("Sheet1", "A2")
	if val != "2026-02-19 17:07:00" {
		t.Errorf("A2 입금일자: got %q, want %q", val, "2026-02-19 17:07:00")
	}

	val, _ = xlsx.GetCellValue("Sheet1", "B2")
	if val != "정상혁(8반)" {
		t.Errorf("B2 입금자: got %q, want %q", val, "정상혁(8반)")
	}

	// 행 수 검증 (헤더 1행 + 거래 4행 = 5행)
	rows, _ := xlsx.GetRows("Sheet1")
	if len(rows) != 5 {
		t.Errorf("행 수: got %d, want 5", len(rows))
	}
}

func TestUniqueFilename(t *testing.T) {
	dir := t.TempDir()

	base := filepath.Join(dir, "test.xlsx")
	got := uniqueFilename(base)
	if got != base {
		t.Errorf("첫 번째 파일: got %q, want %q", got, base)
	}

	os.WriteFile(base, []byte{}, 0644)

	got = uniqueFilename(base)
	want := filepath.Join(dir, "test(1).xlsx")
	if got != want {
		t.Errorf("두 번째 파일: got %q, want %q", got, want)
	}

	os.WriteFile(want, []byte{}, 0644)

	got = uniqueFilename(base)
	want = filepath.Join(dir, "test(2).xlsx")
	if got != want {
		t.Errorf("세 번째 파일: got %q, want %q", got, want)
	}
}

func TestParseWon(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"+ 1원", 1},
		{"+ 100,000원", 100000},
		{"100,003원", 100003},
		{"1원", 1},
		{"- 50,000원", -50000},
		{"잔액 100,003원", 100003},
	}

	for _, tt := range tests {
		got, err := parseWon(tt.input)
		if err != nil {
			t.Errorf("parseWon(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseWon(%q): got %d, want %d", tt.input, got, tt.want)
		}
	}
}
