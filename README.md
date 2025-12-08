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

## Local HTML mode (optional)

By default, this project loads the schedule via HTTP(S).
If ASW (or anyone else) wants a fully offline/local setup, the clean approach is to swap the document loader to support `file://...`.

### 1) Replace your loader with a unified `getDocument`

Use this implementation:

```go
// getDocument loads HTML from either HTTP(S) or a local file.
// Local files must be referenced using the file:// prefix.
func getDocument(url string) (*goquery.Document, error) {
	// Local file support via file://
	if strings.HasPrefix(url, "file://") {
		path := strings.TrimPrefix(url, "file://")
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		return goquery.NewDocumentFromReader(f)
	}

	// Default: HTTP(S)
	client := &http.Client{Timeout: 20 * time.Second}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Polite identification
	req.Header.Set("User-Agent", "ASW-ICS-Exporter/1.0")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d %s", res.StatusCode, res.Status)
	}

	reader, err := charset.NewReader(res.Body, res.Header.Get("Content-Type"))
	if err != nil {
		reader = res.Body
	}

	return goquery.NewDocumentFromReader(reader)
}
```

### 2) Update calls

Replace:

```go
doc, err := httpGetDocument(url)
```

with:

```go
doc, err := getDocument(url)
```

### 3) Adjust constants for local entry points

Example for a local main page:

```go
const (
	// Local copy of the main schedule page:
	scheduleURL = "file://./stundenplaene.html"

	// Keep base URL if your local main page still links to online detail pages:
	baseASWURL = "https://www.asw-ggmbh.de"

	outputDir        = "ics_files"
	minExpectedLinks = 40
	dateFormat       = "02.01.2006"
	tzID             = "Europe/Berlin"
)
```

If you also store detail pages locally, ensure your local main page links are adjusted accordingly (e.g., `file://...`), or extend the URL resolver to map relative references to your local folder.

---

## Notes

* This repository provides code and generated calendar feeds.
* The underlying schedule content remains owned by the original provider.
* If the ASW website structure changes, the parser may require updates.
