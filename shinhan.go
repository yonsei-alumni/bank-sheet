package main

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Transaction represents a single bank transaction.
type Transaction struct {
	DateTime  time.Time
	Depositor string
	Amount    int64
	Balance   int64
}

var (
	timePattern = regexp.MustCompile(`\d{1,2}:\d{2}`)
	datePattern = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}$`)
	kst         = time.FixedZone("KST", 9*60*60)
)

// ParseShinhanMoim parses a Shinhan Bank moim account HTML page
// and extracts transaction records.
func ParseShinhanMoim(r io.Reader) ([]Transaction, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("HTML 파싱 실패: %w", err)
	}

	var transactions []Transaction

	doc.Find("div.border-secondary").Each(func(i int, dateDiv *goquery.Selection) {
		dateText := strings.TrimSpace(dateDiv.Text())
		if !datePattern.MatchString(dateText) {
			return
		}

		section := dateDiv.Parent()
		section.Find("div.py-4").Each(func(j int, txnDiv *goquery.Selection) {
			txn, err := parseShinhanTransaction(dateText, txnDiv)
			if err != nil {
				return
			}
			transactions = append(transactions, txn)
		})
	})

	if len(transactions) == 0 {
		return nil, fmt.Errorf("거래 내역을 찾을 수 없습니다")
	}

	return transactions, nil
}

func parseShinhanTransaction(dateText string, txnDiv *goquery.Selection) (Transaction, error) {
	contentDiv := txnDiv.Find("div.flex-col").First()
	childDivs := contentDiv.Children()

	if childDivs.Length() < 3 {
		return Transaction{}, fmt.Errorf("거래 항목 구조가 올바르지 않습니다")
	}

	depositor := strings.TrimSpace(childDivs.Eq(0).Find("strong").Text())

	spans1 := childDivs.Eq(1).Find("span")
	if spans1.Length() < 2 {
		return Transaction{}, fmt.Errorf("시간/금액 정보를 찾을 수 없습니다")
	}

	timeText := timePattern.FindString(strings.TrimSpace(spans1.First().Text()))
	if timeText == "" {
		return Transaction{}, fmt.Errorf("시간 정보를 찾을 수 없습니다")
	}

	amountText := strings.TrimSpace(spans1.Last().Text())

	spans2 := childDivs.Eq(2).Find("span")
	if spans2.Length() < 1 {
		return Transaction{}, fmt.Errorf("잔액 정보를 찾을 수 없습니다")
	}
	balanceText := strings.TrimSpace(spans2.Last().Text())

	dateTimeStr := strings.ReplaceAll(dateText, ".", "-") + " " + timeText + ":00"
	dt, err := time.ParseInLocation("2006-01-02 15:04:05", dateTimeStr, kst)
	if err != nil {
		return Transaction{}, fmt.Errorf("날짜 파싱 실패: %w", err)
	}

	amount, err := parseWon(amountText)
	if err != nil {
		return Transaction{}, fmt.Errorf("금액 파싱 실패: %w", err)
	}

	balance, err := parseWon(balanceText)
	if err != nil {
		return Transaction{}, fmt.Errorf("잔액 파싱 실패: %w", err)
	}

	return Transaction{
		DateTime:  dt,
		Depositor: depositor,
		Amount:    amount,
		Balance:   balance,
	}, nil
}

func parseWon(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("빈 금액")
	}

	negative := strings.Contains(s, "-")
	s = strings.NewReplacer("+", "", "-", "", "원", "", ",", "", " ", "", "잔액", "").Replace(s)
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("금액 변환 실패: %q", s)
	}
	if negative {
		n = -n
	}
	return n, nil
}
