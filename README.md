# ASW Calendar Exporter

This tool fetches ASW block schedules, parses the HTML plans and generates iCalendar (.ics) files.

A GitHub Actions workflow refreshes the output on a schedule and publishes a small landing page via GitHub Pages, including:
- Block navigation (e.g., DBWINFO, DBING)
- Optional class sub-grouping if detectable from filenames
- Subscribe (webcal) and Copy URL buttons

---

## Requirements (local)

- [Go](https://go.dev/dl/) (compiler/toolchain) installed
  Download Go from the official website and ensure `go` is available in your `PATH`.

---

## Install Dependencies

In the project root:

```bash
go mod tidy
```

This will download and lock all required dependencies.

---

## Run Locally

```bash
go run .
```

Output is written to:
`ics_files/`

---

## Build Locally

```bash
go build
```

---

## Publishing (GitHub)

- **Workflow:** `.github/workflows/publish-ics.yml`
- **GitHub Pages source:** GitHub Actions

The workflow generates the `.ics` files and the landing page automatically.

---

## Notes

- This repository provides code and generated calendar feeds.
- The underlying schedule content remains owned by the original provider.
- If the ASW website structure changes, the parser may require updates.
```