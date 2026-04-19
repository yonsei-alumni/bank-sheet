package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

const (
	shinhanLoginURL = "https://m.shinhan.com/mw/pg/SP0102S0200F01?mid=270040220100&groupId=493705"
)

// LoginStatus distinguishes cert-missing from other login failures so the
// caller can route the user to the cert-registration flow (open system Chrome).
type LoginStatus int

const (
	LoginOK LoginStatus = iota
	LoginNoCert
)

// ShinhanBrowser wraps a single Playwright session against Shinhan's mobile site.
// One instance per login attempt; call Close when done.
type ShinhanBrowser struct {
	pw      *playwright.Playwright
	ctx     playwright.BrowserContext
	page    playwright.Page
	listURL string
	debug   string       // directory for failure dumps
	log     func(string) // progress log; always non-nil
}

// NewShinhanBrowser installs Playwright's Chromium on first use and launches a
// persistent context rooted at userDataDir. headful=true shows the window.
func NewShinhanBrowser(userDataDir string, headful bool, log func(string)) (*ShinhanBrowser, error) {
	if log == nil {
		log = func(string) {}
	}
	log("Playwright 드라이버/Chromium 확인 중... (최초 실행 시 ~170MB 다운로드, 수 분 소요)")
	if err := installPlaywright(log); err != nil {
		return nil, fmt.Errorf("Playwright 설치 실패: %w", err)
	}
	log("Chromium 준비 완료. 브라우저 기동 중...")

	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("Playwright 실행 실패: %w", err)
	}

	ctx, err := pw.Chromium.LaunchPersistentContext(userDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(!headful),
		Viewport:  &playwright.Size{Width: 414, Height: 896},
		UserAgent: playwright.String(detectChromeUA()),
	})
	if err != nil {
		_ = pw.Stop()
		return nil, fmt.Errorf("브라우저 기동 실패: %w", err)
	}

	var page playwright.Page
	if pages := ctx.Pages(); len(pages) > 0 {
		page = pages[0]
	} else {
		page, err = ctx.NewPage()
		if err != nil {
			_ = ctx.Close()
			_ = pw.Stop()
			return nil, err
		}
	}

	return &ShinhanBrowser{
		pw:    pw,
		ctx:   ctx,
		page:  page,
		debug: userDataDir,
		log:   log,
	}, nil
}

// installPlaywright runs playwright.Install with stdout/stderr piped through a
// line scanner that forwards each line to the UI log. If the pipe can't be
// created, it falls back to a silent install.
func installPlaywright(log func(string)) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return playwright.Install(&playwright.RunOptions{
			Browsers: []string{"chromium"},
			Verbose:  false,
		})
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if line == "" {
				continue
			}
			log("  [playwright] " + line)
		}
	}()
	installErr := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
		Verbose:  true,
		Stdout:   pw,
		Stderr:   pw,
	})
	_ = pw.Close()
	<-done
	_ = pr.Close()
	return installErr
}

// Close tears down the browser context and Playwright driver.
func (b *ShinhanBrowser) Close() error {
	if b == nil {
		return nil
	}
	var firstErr error
	if b.ctx != nil {
		if err := b.ctx.Close(); err != nil {
			firstErr = err
		}
	}
	if b.pw != nil {
		if err := b.pw.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Login drives the "신한인증서 → PIN keypad" flow. Returns LoginNoCert when the
// copied profile has no registered Shinhan cert (the "신한인증서" link routes to
// the phone-auth / issuance page instead of opening the PIN keypad iframe).
func (b *ShinhanBrowser) Login(pin string) (LoginStatus, error) {
	if _, err := b.page.Goto(shinhanLoginURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return 0, fmt.Errorf("로그인 페이지 진입 실패: %w", err)
	}

	if err := b.page.GetByText("신한인증서").First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(15000),
	}); err != nil {
		return 0, fmt.Errorf("신한인증서 클릭 실패: %w", err)
	}

	// Short wait for the PIN keypad iframe. If it doesn't show, the account
	// likely has no cert registered on this profile.
	if _, err := b.page.WaitForSelector("iframe#shinhanSignIframeTag", playwright.PageWaitForSelectorOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(7000),
	}); err != nil {
		if b.isNoCertPage() {
			return LoginNoCert, nil
		}
		return 0, fmt.Errorf("PIN 키패드 iframe을 찾지 못함: %w", err)
	}

	if err := b.typePIN(pin); err != nil {
		return 0, err
	}
	if err := b.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(20000),
	}); err != nil {
		return 0, fmt.Errorf("로그인 후 로드 대기 실패: %w", err)
	}
	b.page.WaitForTimeout(1500)
	b.listURL = b.page.URL()
	return LoginOK, nil
}

// isNoCertPage heuristically checks whether the current page is the phone-auth
// / cert-issuance page rather than the PIN keypad.
func (b *ShinhanBrowser) isNoCertPage() bool {
	content, err := b.page.Content()
	if err != nil {
		return false
	}
	for _, kw := range []string{"휴대폰", "본인확인", "인증서 발급", "인증서발급"} {
		if strings.Contains(content, kw) {
			return true
		}
	}
	return false
}

// typePIN reads the randomized PIN keypad inside the iframe, maps each visible
// digit button to its digit, then clicks in PIN order.
func (b *ShinhanBrowser) typePIN(pin string) error {
	frame := b.page.FrameLocator("iframe#shinhanSignIframeTag")
	zero := frame.Locator("button", playwright.FrameLocatorLocatorOptions{HasText: "0"}).First()
	if err := zero.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(15000),
	}); err != nil {
		return fmt.Errorf("PIN 키패드 버튼 대기 실패: %w", err)
	}

	buttons, err := frame.Locator("button").All()
	if err != nil {
		return fmt.Errorf("키패드 버튼 목록 실패: %w", err)
	}

	digitTo := map[string]playwright.Locator{}
	for _, btn := range buttons {
		text, err := btn.InnerText(playwright.LocatorInnerTextOptions{Timeout: playwright.Float(500)})
		if err != nil {
			continue
		}
		text = strings.TrimSpace(text)
		if len(text) == 1 && text >= "0" && text <= "9" {
			if _, ok := digitTo[text]; !ok {
				digitTo[text] = btn
			}
		}
	}

	var missing []string
	for _, r := range pin {
		if _, ok := digitTo[string(r)]; !ok {
			missing = append(missing, string(r))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("PIN 키패드 매핑 실패 (누락 숫자: %v)", missing)
	}

	for _, r := range pin {
		if err := digitTo[string(r)].Click(); err != nil {
			return fmt.Errorf("PIN 버튼 클릭 실패: %w", err)
		}
		b.page.WaitForTimeout(150)
	}
	return nil
}

// ListAccounts returns the set of 모임통장 names shown on the account-list page.
// Uses a single page.Evaluate call that walks up the DOM from each div.truncate
// looking for an ancestor containing a "원" amount — much faster than issuing
// a Playwright RPC per element, which previously stalled on default timeouts.
func (b *ShinhanBrowser) ListAccounts() ([]string, error) {
	if b.listURL == "" {
		return nil, errors.New("먼저 로그인이 필요합니다")
	}
	b.log("  통장 목록 페이지 진입...")
	if _, err := b.page.Goto(b.listURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		b.dumpDebug("accounts")
		return nil, err
	}

	b.log("  통장 카드 DOM 대기...")
	if err := b.page.Locator("div.truncate").First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateAttached,
		Timeout: playwright.Float(20000),
	}); err != nil {
		b.dumpDebug("accounts")
		return nil, fmt.Errorf("통장 카드(div.truncate) 로드 실패: %w", err)
	}
	b.page.WaitForTimeout(500)

	b.log("  통장 이름 추출(JS)...")
	// Walk up to 12 levels for each div.truncate searching for a "원" amount.
	// Single round-trip to the page — avoids per-locator timeout storms.
	result, err := b.page.Evaluate(`() => {
        const amountRe = /[0-9,]+\s*원/;
        const divs = document.querySelectorAll('div.truncate');
        const out = [];
        for (const d of divs) {
            const text = (d.innerText || d.textContent || '').trim();
            if (!text) continue;
            let hasAmount = false;
            let cur = d.parentElement;
            for (let i = 0; i < 12 && cur; i++) {
                const t = cur.innerText || cur.textContent || '';
                if (amountRe.test(t)) { hasAmount = true; break; }
                cur = cur.parentElement;
            }
            out.push({ text, hasAmount });
        }
        return out;
    }`)
	if err != nil {
		b.dumpDebug("accounts")
		return nil, fmt.Errorf("통장 목록 스캔 실패: %w", err)
	}

	items, ok := result.([]any)
	if !ok {
		b.dumpDebug("accounts")
		return nil, fmt.Errorf("통장 목록 스캔 결과 타입 오류: %T", result)
	}

	seen := map[string]bool{}
	var withAmount, allCandidates []string
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["text"].(string)
		hasAmount, _ := m["hasAmount"].(bool)
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		allCandidates = append(allCandidates, name)
		if hasAmount {
			withAmount = append(withAmount, name)
		}
	}
	b.log(fmt.Sprintf("  div.truncate 후보 %d개 중 원-금액이 있는 카드 %d개",
		len(allCandidates), len(withAmount)))

	if len(withAmount) > 0 {
		return withAmount, nil
	}
	if len(allCandidates) > 0 {
		b.log("  [안내] 원-금액 카드가 없어 후보를 그대로 반환합니다. 배너가 섞여 있을 수 있습니다.")
		return allCandidates, nil
	}
	b.dumpDebug("accounts")
	return nil, errors.New("통장 목록을 찾을 수 없습니다 (UI 변경 가능성)")
}

// dumpDebug writes a screenshot + HTML of the current page to the profile dir
// for post-mortem inspection. Errors are swallowed — it's a diagnostic aid.
func (b *ShinhanBrowser) dumpDebug(tag string) {
	stamp := time.Now().Format("20060102-150405")
	base := filepath.Join(b.debug, fmt.Sprintf("dbg_%s_%s", sanitizeFilename(tag), stamp))
	_, _ = b.page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(base + ".png"),
		FullPage: playwright.Bool(true),
	})
	if content, cerr := b.page.Content(); cerr == nil {
		_ = os.WriteFile(base+".html", []byte(content), 0o644)
	}
	b.log(fmt.Sprintf("  [디버그] 스냅샷 저장: %s.{png,html}", base))
}

var filterTriggerPattern = regexp.MustCompile(`(1|3|6)개월.*(최신|과거)순`)

// DownloadHTML navigates to an account, sets the requested period / sort, loads
// all rows via End-key scroll, and returns the rendered HTML for parsing.
func (b *ShinhanBrowser) DownloadHTML(account, period, sortOrder string) (string, error) {
	if b.listURL == "" {
		return "", errors.New("먼저 로그인이 필요합니다")
	}
	if _, err := b.page.Goto(b.listURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return "", err
	}
	b.page.WaitForTimeout(1000)

	target := b.page.Locator("div.truncate", playwright.PageLocatorOptions{HasText: account}).First()
	if err := target.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(15000),
	}); err != nil {
		return b.fail(account, fmt.Errorf("통장 %q 찾기 실패: %w", account, err))
	}
	if err := target.Click(); err != nil {
		return b.fail(account, err)
	}
	_ = b.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(20000),
	})
	b.page.WaitForTimeout(1500)

	// Click "입출금 N원" button (the parent <button> that wraps <strong>입출금</strong>).
	btn := b.page.Locator("button", playwright.PageLocatorOptions{
		Has: b.page.Locator("strong", playwright.PageLocatorOptions{HasText: "입출금"}),
	}).First()
	if err := btn.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(20000),
	}); err != nil {
		return b.fail(account, fmt.Errorf("입출금 버튼 찾기 실패: %w", err))
	}
	if err := btn.Click(); err != nil {
		return b.fail(account, err)
	}
	_ = b.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(20000),
	})
	b.page.WaitForTimeout(1500)

	if err := b.setQueryConditions(period, sortOrder); err != nil {
		return b.fail(account, err)
	}

	b.scrollToLoadAll()

	html, err := b.page.Content()
	if err != nil {
		return b.fail(account, err)
	}
	return html, nil
}

func (b *ShinhanBrowser) setQueryConditions(period, sortOrder string) error {
	b.page.WaitForTimeout(1500)
	if err := b.page.GetByText(filterTriggerPattern).First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(15000),
	}); err != nil {
		return fmt.Errorf("조회조건 모달 열기 실패: %w", err)
	}
	b.page.WaitForTimeout(800)
	if err := b.page.GetByText(period, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		return fmt.Errorf("조회 기간(%s) 선택 실패: %w", period, err)
	}
	b.page.WaitForTimeout(200)
	if err := b.page.GetByText(sortOrder, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		return fmt.Errorf("정렬 순서(%s) 선택 실패: %w", sortOrder, err)
	}
	b.page.WaitForTimeout(200)
	if err := b.page.GetByText("확인", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).First().Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		return fmt.Errorf("확인 버튼 클릭 실패: %w", err)
	}
	_ = b.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(30000),
	})
	b.page.WaitForTimeout(1500)
	return nil
}

// scrollToLoadAll presses End until the transaction row count stabilizes.
func (b *ShinhanBrowser) scrollToLoadAll() int {
	itemSel := `button:has(img[src*="icon-deposit.png"]), button:has(img[src*="icon-withdrawal.png"])`
	prev := -1
	for i := 0; i < 50; i++ {
		if err := b.page.Keyboard().Press("End"); err != nil {
			break
		}
		b.page.WaitForTimeout(900)
		cur, err := b.page.Locator(itemSel).Count()
		if err != nil {
			break
		}
		if cur == prev {
			return cur
		}
		prev = cur
	}
	return prev
}

// fail dumps a screenshot + HTML alongside the profile dir for later inspection,
// then returns the original error so callers can log it.
func (b *ShinhanBrowser) fail(account string, err error) (string, error) {
	b.dumpDebug(account)
	return "", err
}
