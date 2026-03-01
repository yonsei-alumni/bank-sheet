# CLAUDE.md

## Project Overview

A Go desktop app (Fyne GUI) that converts bank transaction HTML files to Excel (xlsx).
Currently supports Shinhan Bank group accounts (모임통장).

## Tech Stack

- Language: Go 1.24+
- GUI: Fyne v2 (`fyne.io/fyne/v2`)
- HTML parsing: goquery (`github.com/PuerkitoBio/goquery`)
- Excel generation: excelize (`github.com/xuri/excelize/v2`)
- Requires CGO (Fyne dependency)

## Project Structure

- `main.go` - Fyne GUI, file selection/conversion flow, custom theme
- `shinhan.go` - Shinhan Bank group account HTML parsing (`ParseShinhanMoim`), `Transaction` type
- `excel.go` - Excel file generation (`WriteExcel`), duplicate filename handling (`uniqueFilename`)
- `shinhan_test.go` - Parsing, Excel generation, currency parsing tests
- `testdata/shinhan.html` - Test HTML fixture
- `Makefile` - Linux/Windows cross-compilation

## Build & Run

```bash
go run .           # Run from source
make               # Build for Linux + Windows (output in dist/)
make linux         # Linux only
make windows       # Windows only (requires x86_64-w64-mingw32-gcc)
```

## Test

```bash
go test ./...
```

## Coding Conventions

- User-facing messages (UI, errors) are written in Korean
- Code comments follow English Go doc style
- To add a new bank: create a separate file (e.g., `woori.go`) with a parsing function, then add an entry to the `bankType` radio group in `main.go`
