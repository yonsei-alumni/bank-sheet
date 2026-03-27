# Claude Code를 이용한 Google Sheet에서 입금내역 → 모금양식 자동 변환

Claude Code에서 Google Sheets 문서의 '입금내역' 시트를 읽어 '모금양식' 시트에 데이터를 채우는 작업 가이드입니다.

## 사전 준비

### 1. gws CLI 및 스킬 설치

[gws (Google Workspace CLI)](https://github.com/googleworkspace/cli) 스킬을 설치합니다. 설치 스크립트는 아래 Gist에서 받을 수 있습니다:

https://gist.github.com/benelog/d75d9ed5220ba1c7929e25611bd6ae5c

```bash
# 1) gws + gcloud 설치, 스킬 생성
./gws_skills_install.sh
```

- `gcloud` CLI가 없으면 자동 설치 (Linux: tar.gz, macOS: brew)
- `gws`가 없으면 `npm install -g @googleworkspace/cli`로 설치
- `gws generate-skills`를 실행하여 `~/.agents/skills/gws-*` 에 스킬 파일 생성

### 2. 인증 설정

#### OAuth Client ID/Secret 준비

기존에 만들어 둔 OAuth 클라이언트가 없다면:

1. [Google Cloud Console](https://console.cloud.google.com/) 접속
2. 프로젝트 선택 (또는 새 프로젝트 생성)
3. **API 및 서비스** → **사용자 인증 정보** 이동
4. **+ 사용자 인증 정보 만들기** → **OAuth 클라이언트 ID** 클릭
5. 애플리케이션 유형: **데스크톱 앱** 선택
6. 생성 후 **Client ID**와 **Client Secret** 복사

> 이미 만들어 둔 OAuth 클라이언트가 있다면 해당 클라이언트의 Client ID/Secret을 사용하면 됩니다.

#### 인증 실행

```bash
# OAuth 클라이언트 설정 (Client ID/Secret 입력)
gws auth setup

# 브라우저에서 Google 계정 인증
gws auth login
```

### 3. 프로젝트에 스킬 연결

```bash
# 프로젝트의 .claude/skills/ 디렉토리에 심링크 생성
./gws_skills_link.sh
```

연결되는 스킬:
- `gws-sheets` — Google Sheets 스프레드시트 관리
- `gws-drive` — Google Drive 파일/폴더 관리
- `gws-docs` — Google Docs 문서 관리
- `gws-slides` — Google Slides 프레젠테이션 관리
- `gws-shared` — 인증, 공통 플래그, 출력 형식 등 공유 패턴

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

1. **인증 확인**: 사전 준비에서 `gws auth login`으로 인증을 완료한 상태여야 합니다.
2. **데이터 읽기**: gws CLI를 통해 '입금내역' 시트를 읽습니다.
3. **데이터 가공**: 이름별로 금액을 합산하고, 학부분반 정보를 파싱합니다.
4. **데이터 쓰기**: gws CLI를 통해 '모금양식' 시트에 결과를 씁니다.
5. **검증**: 양 시트의 총액이 일치하는지 확인합니다.

## 주의사항

- 대상 스프레드시트에 대한 **편집 권한**이 있어야 합니다.
- '모금양식' 시트의 기존 데이터가 있는 행에 덮어쓰게 됩니다.
- `gws auth login`으로 인증이 완료된 상태여야 합니다. 인증이 만료되면 다시 `gws auth login`을 실행합니다.