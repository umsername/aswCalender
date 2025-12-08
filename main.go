package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	ics "github.com/arran4/golang-ical"
	"golang.org/x/net/html/charset"
)

const (
	// ===== Source configuration (HTTP default) =====
	//
	// Standard mode:
	// - scheduleURL points to the ASW overview page
	// - baseASWURL is used to resolve relative links
	//
	// scheduleURL = "https://www.asw-ggmbh.de/laufender-studienbetrieb/stundenplaene"
	// baseASWURL  = "https://www.asw-ggmbh.de"
	//
	// Local mode:
	// - scheduleURL can point to a local HTML file
	// - baseASWURL is ignored in this case
	// - relative detail links are resolved against the directory of that file
	//
	// Examples:
	// Linux/macOS:
	// scheduleURL = "file:///home/user/asw/stundenplaene.html"
	// Windows (note the triple slash and forward slashes):
	// scheduleURL = "file:///C:/Users/asw/Downloads/stundenplaene.html"
	//
	// If your local snapshot is a mirrored structure and contains relative links
	// like "/laufender-studienbetrieb/xyz", you can optionally set baseASWURL
	// to a local folder-based "file://" root by adjusting getDocument/link resolution.
	// For now, simplest approach is: keep the overview + detail pages in one folder.
	scheduleURL = "https://www.asw-ggmbh.de/laufender-studienbetrieb/stundenplaene"
	baseASWURL  = "https://www.asw-ggmbh.de"

	// Output folder for generated .ics files
	outputDir = "ics_files"

	// Safety guard for HTTP parsing:
	// If the website structure changes, we fail fast instead of publishing garbage.
	// This guard is intentionally NOT enforced in local file mode.
	minExpectedLinks = 20

	// Date format used by the schedule header cells (e.g., 27.12.2000)
	dateFormat = "27.01.2006"

	// Timezone for generated calendar events
	tzID = "Europe/Berlin"

	// Polite identification for HTTP mode
	userAgent = "ASW-ICS-Exporter/1.0 (+github.com/umsername/aswCalender)"
)

type ScheduleLink struct {
	CourseName string
	URL        string
}

type ScheduleEvent struct {
	CourseName  string
	Summary     string
	Location    string
	Description string
	Start       time.Time
	End         time.Time
}

func main() {
	log.Println("ASW schedule parser and ICS generator started")

	// Clean output dir for deterministic results
	if err := os.RemoveAll(outputDir); err != nil {
		log.Printf("warning: failed to clean output dir: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("failed to create output dir: %v", err)
	}

	isLocalMode, localBaseDir := detectLocalMode(scheduleURL)

	links, err := parseMainSchedulePage(scheduleURL, isLocalMode, localBaseDir)
	if err != nil {
		log.Fatalf("failed to parse main schedule page: %v", err)
	}

	// Guard only for HTTP mode
	if !isLocalMode && len(links) < minExpectedLinks {
		log.Fatalf(
			"critical: only %d links found (expected > %d). Page structure may have changed.",
			len(links), minExpectedLinks,
		)
	}

	log.Printf("found %d schedule links, starting generation", len(links))

	// Collect aggregated events per class key.
	classEvents := map[string][]ScheduleEvent{}

	for _, link := range links {
		log.Printf("processing course: %s", link.CourseName)

		events, err := parseScheduleDetails(link)
		if err != nil {
			log.Printf("warning: failed to parse %s: %v", link.CourseName, err)
			continue
		}

		if len(events) == 0 {
			log.Printf("no events found for %s, skipping", link.CourseName)
			continue
		}

		// 1) Generate individual block ICS.
		if err := generateICS(link.CourseName, events); err != nil {
			log.Printf("failed to generate ICS for %s: %v", link.CourseName, err)
		} else {
			log.Printf("ICS created for %s with %d events", link.CourseName, len(events))
		}

		// 2) Add to aggregated class bucket.
		classKey := extractClassKey(link.CourseName)
		classEvents[classKey] = append(classEvents[classKey], events...)
	}

	// 3) Generate aggregated ICS per class.
	for classKey, evs := range classEvents {
		if len(evs) == 0 {
			continue
		}

		// Optional hardening: deduplicate aggregated events.
		evs = dedupeEvents(evs)

		if err := generateICS(classKey, evs); err != nil {
			log.Printf("failed to generate aggregated ICS for %s: %v", classKey, err)
			continue
		}
		log.Printf("aggregated ICS created for %s with %d events", classKey, len(evs))
	}

	log.Printf("done. files are in: %s", outputDir)

	if err := generateSite(); err != nil {
		log.Printf("warning: site generation failed: %v", err)
	}
}

func detectLocalMode(url string) (bool, string) {
	if strings.HasPrefix(url, "file://") {
		path := strings.TrimPrefix(url, "file://")
		// Base dir for resolving relative links in local mode
		return true, filepath.Dir(path)
	}
	return false, ""
}

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

	req.Header.Set("User-Agent", userAgent)

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

// Resolve href into a full URL depending on mode.
func resolveURL(href string, isLocalMode bool, localBaseDir string) string {
	href = strings.TrimSpace(href)

	// Already absolute HTTP
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	// Already absolute file URL
	if strings.HasPrefix(href, "file://") {
		return href
	}

	if isLocalMode && localBaseDir != "" {
		// Treat leading "/" as relative-to-base-dir for local snapshots
		h := strings.TrimPrefix(href, "/")
		return "file://" + filepath.Join(localBaseDir, h)
	}

	// Website mode
	if strings.HasPrefix(href, "/") {
		return baseASWURL + href
	}

	// Relative without leading slash
	return baseASWURL + "/" + href
}

// Step 1: Extract schedule links from the main ASW page.
// This is intentionally flexible because the link text can vary by cohort/block naming.
func parseMainSchedulePage(url string, isLocalMode bool, localBaseDir string) ([]ScheduleLink, error) {
	doc, err := getDocument(url)
	if err != nil {
		return nil, err
	}

	var extracted []ScheduleLink

	// Match typical program identifiers and block naming (Block / Blockphase).
	// Example: DBING-01-2024 - 4. Blockphase
	reText := regexp.MustCompile(`^DB[A-Z]+[-–]\s*.*(Blockphase|Block)\s*$`)

	// Also keep a fallback check on the href to reduce false positives.
	reHref := regexp.MustCompile(`(?i)(block|blockphase).*\.html?$`)

	doc.Find("a").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		href, ok := s.Attr("href")
		if !ok {
			return
		}

		fullURL := resolveURL(href, isLocalMode, localBaseDir)

		if reText.MatchString(text) || reHref.MatchString(href) {
			extracted = append(extracted, ScheduleLink{
				CourseName: normalizeCourseName(text),
				URL:        fullURL,
			})
		}
	})

	// Deduplicate by URL.
	seen := map[string]bool{}
	var uniq []ScheduleLink
	for _, l := range extracted {
		if seen[l.URL] {
			continue
		}
		seen[l.URL] = true
		uniq = append(uniq, l)
	}

	return uniq, nil
}

// Step 2: Parse a single schedule detail page generated by sked campus.
// The exported HTML uses weekly tables and encodes events as td.v cells.
func parseScheduleDetails(link ScheduleLink) ([]ScheduleEvent, error) {
	doc, err := getDocument(link.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch error for %s: %w", link.CourseName, err)
	}

	loc, err := time.LoadLocation(tzID)
	if err != nil {
		loc = time.Local
	}

	var all []ScheduleEvent

	// Each week is represented by a table. We parse all tables and extract td.v cells with grid mapping.
	doc.Find("table").Each(func(_ int, table *goquery.Selection) {
		evs := parseWeekTable(table, link.CourseName, loc)
		if len(evs) > 0 {
			all = append(all, evs...)
		}
	})

	return all, nil
}

// Parse one weekly schedule table.
// We map header dates to logical columns and then place body cells into a grid
// using colspan/rowspan to determine the date for each td.v event cell.
func parseWeekTable(table *goquery.Selection, courseName string, loc *time.Location) []ScheduleEvent {
	var events []ScheduleEvent

	rows := table.Find("tr")
	if rows.Length() == 0 {
		return events
	}

	headerRow := rows.First()
	dateByCol, totalCols := extractHeaderDates(headerRow)
	if totalCols == 0 || len(dateByCol) == 0 {
		return events
	}

	// Occupancy array for rowspans across logical columns.
	occ := make([]int, totalCols)

	rows.Slice(1, rows.Length()).Each(func(_ int, row *goquery.Selection) {
		// Decrease occupancy counters for each new row.
		for i := range occ {
			if occ[i] > 0 {
				occ[i]--
			}
		}

		colCursor := 0

		row.ChildrenFiltered("td").Each(func(_ int, cell *goquery.Selection) {
			cs := getSpan(cell, "colspan")
			rs := getSpan(cell, "rowspan")

			// Find next free column position for this cell.
			for colCursor < totalCols && occ[colCursor] > 0 {
				colCursor++
			}
			if colCursor >= totalCols {
				return
			}

			startCol := colCursor
			endCol := startCol + cs
			if endCol > totalCols {
				endCol = totalCols
			}

			// Mark occupancy for rowspans.
			if rs > 1 {
				for c := startCol; c < endCol; c++ {
					occ[c] = rs - 1
				}
			}

			// Process event cells.
			if hasClass(cell, "v") {
				date, ok := dateByCol[startCol]
				if ok && !date.IsZero() {
					if ev, ok := parseEventCell(cell, date, courseName, loc); ok {
						events = append(events, ev)
					}
				}
			}

			colCursor = endCol
		})
	})

	return events
}

// Extract mapping from logical column index to date based on the header row.
// Example header cell text: "Mo, 08.12.2025"
func extractHeaderDates(headerRow *goquery.Selection) (map[int]time.Time, int) {
	dateByCol := make(map[int]time.Time)

	cells := headerRow.ChildrenFiltered("td")
	if cells.Length() == 0 {
		return dateByCol, 0
	}

	dayRe := regexp.MustCompile(`\b(Mo|Di|Mi|Do|Fr|Sa|So),\s*(\d{2}\.\d{2}\.\d{4})\b`)

	col := 0
	total := 0

	// First pass to determine total columns based on colspans.
	cells.Each(func(_ int, c *goquery.Selection) {
		total += getSpan(c, "colspan")
	})

	// Second pass to assign dates to column ranges.
	cells.Each(func(_ int, c *goquery.Selection) {
		cs := getSpan(c, "colspan")
		text := strings.TrimSpace(c.Text())

		m := dayRe.FindStringSubmatch(text)
		if len(m) == 3 {
			d, err := time.Parse(dateFormat, m[2])
			if err == nil {
				for i := 0; i < cs; i++ {
					dateByCol[col+i] = d
				}
			}
		}

		col += cs
	})

	return dateByCol, total
}

// Parse a td.v cell into a ScheduleEvent.
func parseEventCell(cell *goquery.Selection, date time.Time, courseName string, loc *time.Location) (ScheduleEvent, bool) {
	rawHTML, err := cell.Html()
	if err != nil {
		return ScheduleEvent{}, false
	}

	lines := splitCellLines(rawHTML)
	if len(lines) == 0 {
		return ScheduleEvent{}, false
	}

	startStr, endStr := extractTimeRange(strings.Join(lines, " "))
	if startStr == "" || endStr == "" {
		return ScheduleEvent{}, false
	}

	// Skip reserved placeholders by default.
	if len(lines) >= 2 && strings.EqualFold(strings.TrimSpace(lines[1]), "Reserviert") {
		return ScheduleEvent{}, false
	}

	typeLine := ""
	moduleLine := ""
	locationLine := ""
	extra := []string{}

	if len(lines) > 1 {
		typeLine = lines[1]
	}
	if len(lines) > 2 {
		moduleLine = lines[2]
	}
	if len(lines) > 3 {
		locationLine = lines[3]
	}
	if len(lines) > 4 {
		extra = lines[4:]
	}

	startHour, startMin, ok := parseClock(startStr)
	if !ok {
		return ScheduleEvent{}, false
	}
	endHour, endMin, ok := parseClock(endStr)
	if !ok {
		return ScheduleEvent{}, false
	}

	start := time.Date(date.Year(), date.Month(), date.Day(), startHour, startMin, 0, 0, loc)
	end := time.Date(date.Year(), date.Month(), date.Day(), endHour, endMin, 0, 0, loc)

	// Build summary with pragmatic rules.
	summary := moduleLine
	if summary == "" {
		summary = typeLine
	}
	if summary == "" {
		summary = "ASW event"
	}
	if typeLine != "" && moduleLine != "" && !strings.Contains(strings.ToLower(moduleLine), strings.ToLower(typeLine)) {
		summary = fmt.Sprintf("%s (%s)", moduleLine, typeLine)
	}

	// Try to determine location.
	location := locationLine
	if location == "" {
		for _, l := range append([]string{typeLine, moduleLine}, extra...) {
			if strings.HasPrefix(l, "NK:") || strings.HasPrefix(l, "EXT:") {
				location = l
				break
			}
		}
	}

	descParts := []string{
		fmt.Sprintf("Course: %s", courseName),
	}
	if typeLine != "" {
		descParts = append(descParts, fmt.Sprintf("Type: %s", typeLine))
	}
	if moduleLine != "" {
		descParts = append(descParts, fmt.Sprintf("Module/Group: %s", moduleLine))
	}
	if location != "" {
		descParts = append(descParts, fmt.Sprintf("Location: %s", location))
	}
	for _, l := range extra {
		if l != "" {
			descParts = append(descParts, l)
		}
	}

	description := strings.Join(descParts, "\n")

	return ScheduleEvent{
		CourseName:  courseName,
		Summary:     summary,
		Location:    location,
		Description: description,
		Start:       start,
		End:         end,
	}, true
}

func splitCellLines(rawHTML string) []string {
	// Normalize <br> variants to newline.
	s := rawHTML
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")

	// Remove remaining tags.
	tagRe := regexp.MustCompile(`<[^>]*>`)
	s = tagRe.ReplaceAllString(s, "")

	// Unescape HTML entities.
	s = html.UnescapeString(s)

	// Split and clean.
	rawLines := strings.Split(s, "\n")
	var lines []string
	footnoteRe := regexp.MustCompile(`\s*\[\d+\]\s*`)

	for _, l := range rawLines {
		l = strings.TrimSpace(l)
		l = footnoteRe.ReplaceAllString(l, "")
		if l != "" {
			lines = append(lines, l)
		}
	}

	return lines
}

func extractTimeRange(text string) (string, string) {
	re := regexp.MustCompile(`(\d{1,2}:\d{2})\s*-\s*(\d{1,2}:\d{2})`)
	m := re.FindStringSubmatch(text)
	if len(m) == 3 {
		return m[1], m[2]
	}
	return "", ""
}

func parseClock(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
	m := re.FindStringSubmatch(s)
	if len(m) != 3 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(m[1])
	min, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if h < 0 || h > 23 || min < 0 || min > 59 {
		return 0, 0, false
	}
	return h, min, true
}

func getSpan(s *goquery.Selection, attr string) int {
	v, ok := s.Attr(attr)
	if !ok {
		return 1
	}
	i, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || i < 1 {
		return 1
	}
	return i
}

func hasClass(s *goquery.Selection, class string) bool {
	return strings.Contains(" "+s.AttrOr("class", "")+" ", " "+class+" ")
}

func normalizeCourseName(s string) string {
	// Normalize different dash types to a single hyphen for consistent filenames/UIDs.
	s = strings.TrimSpace(s)
	r := strings.NewReplacer(
		"−", "-", // minus sign
		"–", "-", // en dash
		"—", "-", // em dash
		"‐", "-", // hyphen (U+2010)
		"-", "-", // non-breaking hyphen (U+2011)
		"‒", "-", // figure dash
	)
	return r.Replace(s)
}

// Derive an aggregated class key from a course/block name.
func extractClassKey(courseName string) string {
	s := normalizeCourseName(courseName)

	// Make the string easier to match.
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = regexp.MustCompile(`-{2,}`).ReplaceAllString(s, "-")

	// 1) Letter-based class pattern: DBWINFO-A04, DBBWL-B03, etc.
	reLetter := regexp.MustCompile(`\b(DB[A-Z]+)-([A-Z]\d{2,3})\b`)
	if m := reLetter.FindStringSubmatch(s); len(m) == 3 {
		return fmt.Sprintf("%s-%s", m[1], m[2])
	}

	// 2) Numeric cohort pattern: DBING-01, DBMAB-04, DBWI-05, etc.
	reNum := regexp.MustCompile(`\b(DB[A-Z]+)-(\d{2})\b`)
	if m := reNum.FindStringSubmatch(s); len(m) == 3 {
		return fmt.Sprintf("%s-%s", m[1], m[2])
	}

	// 3) Fallback to program block
	reBlock := regexp.MustCompile(`\b(DB[A-Z]+)\b`)
	if m := reBlock.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}

	return "Other"
}

// Optional hardening for aggregated files.
func dedupeEvents(in []ScheduleEvent) []ScheduleEvent {
	seen := map[string]bool{}
	out := make([]ScheduleEvent, 0, len(in))

	for _, e := range in {
		key := fmt.Sprintf("%d|%d|%s|%s|%s",
			e.Start.Unix(),
			e.End.Unix(),
			e.Summary,
			e.Location,
			e.Description,
		)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}

	return out
}

// Step 3: Generate ICS file for one course or aggregated class.
func generateICS(courseName string, events []ScheduleEvent) error {
	cal := ics.NewCalendar()
	cal.SetProductId("-//ASW Schedule Exporter//EN")
	cal.SetName(fmt.Sprintf("ASW Schedule %s", courseName))
	cal.SetTzid(tzID)

	// Sanitize for filename and UID.
	sanitizedName := regexp.MustCompile(`[^a-zA-Z0-9_-]+`).ReplaceAllString(courseName, "_")

	for i, e := range events {
		ev := cal.AddEvent(fmt.Sprintf("%s-%d-%d", sanitizedName, e.Start.Unix(), i))
		ev.SetSummary(e.Summary)
		if e.Location != "" {
			ev.SetLocation(e.Location)
		}
		if e.Description != "" {
			ev.SetDescription(e.Description)
		}
		ev.SetStartAt(e.Start)
		ev.SetEndAt(e.End)
	}

	filename := fmt.Sprintf("%s/%s.ics", outputDir, sanitizedName)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(cal.Serialize())
	return err
}
