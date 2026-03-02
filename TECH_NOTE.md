# 기술 노트

## 기술 스택
- Go 언어 1.24+
* [Fyne](https://fyne.io/) : GUI 프레임워크


## 빌드

```bash
# Linux + Windows 동시 빌드
make

# Linux만
make linux

# Windows만 (크로스 컴파일, x86_64-w64-mingw32-gcc 필요)
make windows
```

빌드 결과물은 `dist/` 디렉터리에 생성됩니다.

## 실행

```bash
# 소스에서 직접 실행
go run .

# 빌드된 바이너리 실행
./dist/bank-sheet-linux-amd64
```
