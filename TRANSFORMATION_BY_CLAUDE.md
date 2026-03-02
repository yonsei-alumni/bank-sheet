# Claude Code를 이용한 Google Sheet에서 입금내역 → 모금양식 자동 변환

Claude Code에서 Google Sheets 문서의 '입금내역' 시트를 읽어 '모금양식' 시트에 데이터를 채우는 작업 가이드입니다.

## 사전 준비

### 1. Google Cloud 프로젝트 설정

1. https://console.cloud.google.com 에서 프로젝트 생성 (또는 기존 프로젝트 선택)
2. **Google Sheets API 활성화**
   - https://console.cloud.google.com/apis/library/sheets.googleapis.com 에서 "사용" 클릭
3. **OAuth 동의 화면 설정**
   - https://console.cloud.google.com/apis/credentials/consent
   - External 선택 → 앱 이름, 이메일 등 필수 정보 입력
   - Audience 섹션에서 본인 Gmail을 **테스트 사용자**로 추가
4. **OAuth 클라이언트 ID 생성**
   - https://console.cloud.google.com/apis/credentials → "사용자 인증 정보 만들기" → "OAuth 클라이언트 ID"
   - 유형: **데스크톱 앱** (또는 웹 애플리케이션)
   - 생성 후 **Client ID**와 **Client Secret** 확인

### 2. credentials.json 파일 생성

아래 경로에 파일을 생성합니다:

```
~/.agents/skills/gmail-skill/credentials.json
```

```json
{
  "installed": {
    "client_id": "YOUR_CLIENT_ID.apps.googleusercontent.com",
    "client_secret": "YOUR_CLIENT_SECRET",
    "auth_uri": "https://accounts.google.com/o/oauth2/auth",
    "token_uri": "https://oauth2.googleapis.com/token",
    "redirect_uris": ["http://localhost"]
  }
}
```

> 이미 gmail-skill 등 다른 Google skill에서 이 파일을 만들어 두었다면 그대로 재사용 가능합니다.

### 3. google-sheets skill 설치

Claude Code에서 google-sheets skill이 설치되어 있어야 합니다. 설치 방법:

```
/install-skill google-sheets
```

### 4. Python 라이브러리

최초 실행 시 Claude Code가 자동으로 venv를 만들고 필요한 패키지를 설치합니다:

- `google-auth`, `google-api-python-client`, `requests`

## 실행 방법

### 1. 대상 Google Sheets 문서 준비

A, B 두 문서 모두 **네이티브 Google Sheets 형식**이어야 합니다.
Google Drive에 업로드된 `.xlsx` 파일은 Sheets API로 직접 읽고 쓸 수 없어 오류가 발생합니다.

#### 문서 형식 확인 방법

Google Drive에서 파일을 열었을 때 상단에 `.xlsx` 표시가 보이면 Excel 형식입니다.

#### Excel 파일을 Google Sheets로 변환하는 방법

1. Google Drive에서 해당 `.xlsx` 파일을 더블 클릭하여 열기
2. 상단 메뉴에서 **파일** → **Google 스프레드시트로 저장** 클릭
3. 새로 생성된 Google Sheets 문서의 URL을 사용

> 변환 후 원본 `.xlsx` 파일과 변환된 Google Sheets 파일이 Drive에 별도로 존재하게 됩니다. 작업 시 변환된 문서의 URL을 사용해야 합니다.

### 2. Claude Code에 프롬프트 입력 예시

```
A 문서의 데이터를 B 문서로 옮기려고해.

A. https://docs.google.com/spreadsheets/d/{입금내역_SPREADSHEET_ID}/
B. https://docs.google.com/spreadsheets/d/{모금양식_SPREADSHEET_ID}/

A문서의 데이터를 B문서의 '모금양식' sheet로 데이터를 채워줘.

'단과대학', '학부분반', '성명', '금액'란만 채워줘.
'단과대학'은 '{단과대학명}'으로 모두 동일하게 해줘.

'입급내역' sheet에서 입금자는 '이름(학부분반)' 형식으로 되어 있어.
'학부분반' 컬럼에는 '{단과약칭}8반'과 같은 식으로 앞에 '{단과약칭}'을 붙여줘. '(반)'이 없이 입금한 사람은 그냥 이름만 옮겨줘.
이름 부분이 동일한 사람은 금액을 합쳐서 1행으로 만들어줘.

두 Sheet의 총액이 동일한지로 결과를 검증해줘.
```

**Placeholder 설명:**

| Placeholder | 예시 | 설명 |
|-------------|------|------|
| `{입금내역_SPREADSHEET_ID}` | `11pBbHQViJ47T3TUqI2zNzUWBurhy7Xpz4NNmDUjUp5o` | 입금내역이 있는 원본 문서 ID |
| `{모금양식_SPREADSHEET_ID}` | `1HYHYYUM63zIdSpKjFrc94khtau-0FYUFOgd-u_pdvyY` | 모금양식을 채울 대상 문서 ID |
| `{단과대학명}` | `상경대학` | 단과대학 컬럼에 채울 값 |
| `{단과약칭}` | `상경` | 학부분반 앞에 붙일 접두어 |

### 3. 실행 중 동작

1. **OAuth 인증**: 브라우저가 자동으로 열리며 Google 계정 로그인을 요청합니다. 로그인 후 권한을 승인하면 자동으로 진행됩니다.
   - 최초 1회만 필요하며, 이후에는 저장된 토큰으로 자동 인증됩니다.
   - 토큰이 만료되면 자동 갱신됩니다.
2. **데이터 읽기**: Sheets API로 '입금내역' 시트를 읽습니다.
3. **데이터 가공**: 이름별로 금액을 합산하고, 학부분반 정보를 파싱합니다.
4. **데이터 쓰기**: Sheets API로 '모금양식' 시트에 결과를 씁니다.
5. **검증**: 양 시트의 총액이 일치하는지 확인합니다.

## 주의사항

- 대상 스프레드시트에 대한 **편집 권한**이 있어야 합니다.
- '모금양식' 시트의 기존 데이터가 있는 행에 덮어쓰게 됩니다.
- Google Sheets API가 Google Cloud 프로젝트에서 **활성화**되어 있어야 합니다. 비활성 상태면 403 오류가 발생하며, 활성화 후 전파까지 수십 초가 걸릴 수 있습니다.