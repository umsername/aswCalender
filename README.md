# ASW Calendar Exporter

This tool fetches ASW block schedules, parses the HTML plans and generates iCalendar (`.ics`) files.

A GitHub Actions workflow refreshes the output on a schedule and publishes a small landing page via GitHub Pages, including:
- Block navigation (derived from filenames)
- Class sub-grouping (letter + numeric styles)
- Subscribe (webcal / iOS best supported), Copy URL, and Download file actions
- A short Google/Android help page

---

## Quickstart (recommended)

A prebuilt Docker image is available via GitHub Container Registry:

- `ghcr.io/umsername/asw-exporter:latest`

### Run (HTTPS mode)

#### Bash (Linux/macOS/Git-Bash)

```bash
mkdir -p out
docker run --rm \
  -v "$PWD/out:/data" \
  -e ASW_OUTPUT_DIR="/data/ics_files" \
  -e ASW_PUBLIC_DIR="/data/public" \
  ghcr.io/umsername/asw-exporter:latest
````

#### PowerShell (Windows)

```powershell
New-Item -ItemType Directory -Force -Path .\out | Out-Null

docker run --rm `
  -v "$($PWD.Path)\out:/data" `
  -e ASW_OUTPUT_DIR="/data/ics_files" `
  -e ASW_PUBLIC_DIR="/data/public" `
  ghcr.io/umsername/asw-exporter:latest
```

Output:

```txt
./out/ics_files/
./out/public/
```

---

## Local development

### Requirements

* Go installed and available in your `PATH`

### Install dependencies

```bash
go mod tidy
```

### Run

```bash
go run .
```

---

## Configuration

The app uses environment variables with sensible defaults:

* `ASW_SCHEDULE_URL`
  Default: ASW overview page (HTTPS)

* `ASW_BASE_URL`
  Default: `https://www.asw-ggmbh.de`

* `ASW_OUTPUT_DIR`
  Default: `ics_files`

* `ASW_PUBLIC_DIR`
  Default: `public`

* `ASW_USER_AGENT`
  Default: `ASW-ICS-Exporter/1.0 (+github.com/umsername/aswCalender)`

---

## Use a `.env` file (optional)

A sample file is provided as `.env.example`.

For local builds on Windows PowerShell:

```powershell
Get-Content .env | ForEach-Object {
    if ($_ -match '^\s*#' -or $_ -match '^\s*$') { return }
    $k, $v = $_ -split '=', 2
    $env:$k = $v
}

go build -o asw-parser.exe .
.\asw-parser.exe
```

---

## Publishing (GitHub)

* **Workflow:** `.github/workflows/publish-ics.yml`
* **GitHub Pages source:** GitHub Actions

The workflow generates the `.ics` files and the landing page automatically.

---

## Notes

* This repository provides code and generated calendar feeds.
* The underlying schedule content remains owned by the original provider.
* If the ASW website structure changes, the parser may require updates.
