# ASW Calendar Exporter

This tool fetches ASW block schedules, parses the HTML plans and generates iCalendar (.ics) files.

A GitHub Actions workflow refreshes the output on a schedule and publishes a small landing page via GitHub Pages, including:
- Block navigation (derived from filenames)
- Class sub-grouping (letter + numeric styles)
- Subscribe (webcal), Copy URL, and Open file actions
- A short Google/Android help page

---
## Requirements (local)
- Go installed and available in your `PATH`
---
## Install Dependencies
In the project root:
```bash
go mod tidy
````
---
## Run Locally
```bash
go run .
```
Output is written to:
```txt
ics_files/
```
---
## Build Locally
```bash
go build
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
