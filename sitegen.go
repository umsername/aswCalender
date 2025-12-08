package main

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	publicDir    = "public"
	publicICSDir = "public/ics_files"
	sourcePage   = "https://www.asw-ggmbh.de/laufender-studienbetrieb/stundenplaene"
)

type fileGroup map[string]map[string][]string // block -> subgroup -> files

var (
	// Aggregated class calendars:
	// - letter classes: DBWINFO-A04.ics, DBBWL-B03.ics ...
	// - numeric classes: DBING-01.ics, DBMAB-03.ics, DBWI-05.ics ...
	aggClassRe = regexp.MustCompile(`^DB[A-Z]+-(?:[A-Z]\d{2,3}|\d{2})\.ics$`)

	// Aggregated program-level calendar like DBING.ics (if generated)
	aggBlockRe = regexp.MustCompile(`^DB[A-Z]+\.ics$`)

	// Block / program prefix from filenames
	blockRe = regexp.MustCompile(`^(DB[A-Z]+)`)

	// Class subgroup matcher for BOTH styles
	// Captures:
	//  1) block/program (DBBWL, DBWINFO, DBING, DBMAB, DBWI, ...)
	//  2) class token (A04, B03, 01, 03, 05, ...)
	classRe = regexp.MustCompile(`^(DB[A-Z]+)-((?:[A-Z]\d{2,3})|(?:\d{2}))`)
)

func generateSite() error {
	if err := os.MkdirAll(publicICSDir, 0755); err != nil {
		return err
	}

	// Copy ICS files to public folder for GitHub Pages
	icsFiles, err := filepath.Glob(filepath.Join(outputDir, "*.ics"))
	if err != nil {
		return err
	}

	for _, src := range icsFiles {
		dst := filepath.Join(publicICSDir, filepath.Base(src))
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}

	// Collect names from public dir (the actual published set)
	pubFiles, err := filepath.Glob(filepath.Join(publicICSDir, "*.ics"))
	if err != nil {
		return err
	}

	names := make([]string, 0, len(pubFiles))
	for _, p := range pubFiles {
		names = append(names, filepath.Base(p))
	}
	sort.Strings(names)

	aggregated, _ := splitAggregated(names)

	blocksAgg := groupFiles(aggregated)
	blocksAll := groupFiles(names)

	blockOrderAgg := sortedKeys(blocksAgg)
	blockOrderAll := sortedKeys(blocksAll)

	if err := os.MkdirAll(publicDir, 0755); err != nil {
		return err
	}

	// Aggregated index page
	if err := renderPage(
		filepath.Join(publicDir, "index.html"),
		"ASW Class Calendars",
		"Aggregated calendars per class/block. Recommended for subscription.",
		blocksAgg,
		blockOrderAgg,
		true,
		true,
		false,
	); err != nil {
		return err
	}

	// Full listing page
	if err := renderPage(
		filepath.Join(publicDir, "all.html"),
		"ASW All Calendars",
		"All generated calendars including individual block files.",
		blocksAll,
		blockOrderAll,
		true,
		false,
		true,
	); err != nil {
		return err
	}

	// Help page for Google/Android
	if err := renderGoogleHelpPage(
		filepath.Join(publicDir, "help-google.html"),
	); err != nil {
		return err
	}

	return nil
}

func splitAggregated(names []string) (aggregated []string, individual []string) {
	for _, name := range names {
		if aggClassRe.MatchString(name) || aggBlockRe.MatchString(name) {
			aggregated = append(aggregated, name)
		} else {
			individual = append(individual, name)
		}
	}
	return
}

func groupFiles(fileList []string) fileGroup {
	blocks := make(fileGroup)

	for _, name := range fileList {
		n := normName(name)

		bm := blockRe.FindStringSubmatch(n)
		block := "Other"
		if len(bm) == 2 {
			block = bm[1]
		}

		cm := classRe.FindStringSubmatch(n)
		cls := ""
		if len(cm) == 3 {
			cls = cm[2]
		}

		if _, ok := blocks[block]; !ok {
			blocks[block] = map[string][]string{}
		}

		if cls != "" {
			blocks[block][cls] = append(blocks[block][cls], name)
		} else {
			blocks[block]["__items__"] = append(blocks[block]["__items__"], name)
		}
	}

	// Sort inner slices
	for _, sub := range blocks {
		for k := range sub {
			sort.Strings(sub[k])
		}
	}

	return blocks
}

func subgroupOrder(keys []string) []string {
	hasItems := false
	classes := []string{}

	for _, k := range keys {
		if k == "__items__" {
			hasItems = true
		} else {
			classes = append(classes, k)
		}
	}

	sort.Strings(classes)
	if hasItems {
		classes = append(classes, "__items__")
	}
	return classes
}

func sortedKeys(m fileGroup) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func normName(name string) string {
	n := name
	n = strings.ReplaceAll(n, "_", "-")
	n = strings.ReplaceAll(n, " ", "-")
	n = strings.TrimSuffix(n, ".ics")
	n = regexp.MustCompile(`-{2,}`).ReplaceAllString(n, "-")
	return n
}

func niceLabel(fname string) string {
	base := strings.TrimSuffix(fname, ".ics")
	return strings.ReplaceAll(base, "_", " ")
}

func renderPage(path, title, subtitle string, blocks fileGroup, blockOrder []string, showToolbar bool, navToAll bool, navToIndex bool) error {
	var b strings.Builder

	b.WriteString("<!doctype html><html><head><meta charset='utf-8'>")
	b.WriteString("<meta name='viewport' content='width=device-width, initial-scale=1'>")
	b.WriteString("<title>" + html.EscapeString(title) + "</title>")
	b.WriteString("<style>" + siteCSS() + "</style>")
	b.WriteString("</head><body>")

	b.WriteString("<header>")
	b.WriteString("<h1>" + html.EscapeString(title) + "</h1>")
	b.WriteString("<p>" + html.EscapeString(subtitle) + "</p>")
	b.WriteString("</header>")

	b.WriteString("<div class='navline'>")
	if navToAll {
		b.WriteString("<a class='navlink' href='all.html'>Show individual calendars</a>")
	}
	if navToIndex {
		b.WriteString("<a class='navlink' href='index.html'>Back to class calendars</a>")
	}
	b.WriteString("<a class='navlink secondary' href='" + html.EscapeString(sourcePage) + "'>Source page</a>")
	b.WriteString("</div>")

	if showToolbar && len(blockOrder) > 0 {
		b.WriteString("<div class='toolbar'>")
		for _, block := range blockOrder {
			count := 0
			for _, files := range blocks[block] {
				count += len(files)
			}
			safeBlock := html.EscapeString(block)
			b.WriteString("<a class='toolbtn' href='#" + safeBlock + "'>")
			b.WriteString("<span>" + safeBlock + "</span>")
			b.WriteString("<span class='count'>" + strconvI(count) + "</span>")
			b.WriteString("</a>")
		}
		b.WriteString("</div>")
	}

	// Platform hint box
	b.WriteString("<div class='infobox'><div>")
	b.WriteString("<div class='infobox-title'>Quick setup</div>")
	b.WriteString("<div class='infobox-body'>")
	b.WriteString("Use <b>Subscribe</b> for webcal subscription (best supported on Apple). ")
	b.WriteString("For Google Calendar on Android/Windows, ")
	b.WriteString("<a href='help-google.html'>follow this guide</a>. ")
	b.WriteString("Alternatively use <b>Copy URL</b> to add the feed manually or <b>Open file</b> for a one-time import.")
	b.WriteString("</div>")
	b.WriteString("</div></div>")


	b.WriteString("<main>")

	totalFiles := 0
	for _, block := range blocks {
		for _, files := range block {
			totalFiles += len(files)
		}
	}

	if totalFiles == 0 {
		b.WriteString("<section class='group'><h2>No files</h2>")
		b.WriteString("<p class='small'>No ICS files were generated yet.</p></section>")
	} else {
		for _, block := range blockOrder {
			blockDict := blocks[block]
			blockTotal := 0
			for _, files := range blockDict {
				blockTotal += len(files)
			}

			safeBlock := html.EscapeString(block)
			b.WriteString("<section class='group' id='" + safeBlock + "'>")
			b.WriteString("<h2>" + safeBlock + " <span class='badge'>" + strconvI(blockTotal) + " files</span></h2>")

			keys := make([]string, 0, len(blockDict))
			for k := range blockDict {
				keys = append(keys, k)
			}
			keys = subgroupOrder(keys)

			for _, k := range keys {
				items := blockDict[k]
				if len(items) == 0 {
					continue
				}

				b.WriteString("<div class='subgroup'>")

				if k == "__items__" {
					b.WriteString("<div class='subhead'>General <span class='subbadge'>" + strconvI(len(items)) + "</span></div>")
				} else {
					b.WriteString("<div class='subhead'>Class " + html.EscapeString(k) + " <span class='subbadge'>" + strconvI(len(items)) + "</span></div>")
				}

				b.WriteString("<ul>")
				for _, name := range items {
					label := niceLabel(name)
					safeName := html.EscapeString(name)
					safeLabel := html.EscapeString(label)

					b.WriteString("<li>")
					b.WriteString("<div class='row'>")
					b.WriteString("<div class='row-left'>")
					b.WriteString("<div class='file'>" + safeLabel + "</div>")
					b.WriteString("<div class='small'>" + safeName + "</div>")
					b.WriteString("</div>")
					b.WriteString("<div class='actions'>")
					b.WriteString("<button class='btn btn-primary' onclick=\"subscribe('" + safeName + "')\">Subscribe</button>")
					b.WriteString("<button class='btn' onclick=\"copyUrl('" + safeName + "', this)\">Copy URL</button>")
					b.WriteString("<a class='btn' href='ics_files/" + safeName + "'>Open file</a>")
					b.WriteString("</div>")
					b.WriteString("</div>")
					b.WriteString("</li>")
				}
				b.WriteString("</ul>")
				b.WriteString("</div>")
			}

			b.WriteString("</section>")
		}
	}

	b.WriteString("</main>")
	b.WriteString("<footer>Updated by GitHub Actions on schedule.</footer>")
	b.WriteString(siteJS())
	b.WriteString("</body></html>")

	return os.WriteFile(path, []byte(b.String()), 0644)
}

func renderGoogleHelpPage(path string) error {
	var b strings.Builder

	title := "Google Calendar setup"
	subtitle := "How to add these ASW calendars on Android and Google Calendar."

	b.WriteString("<!doctype html><html><head><meta charset='utf-8'>")
	b.WriteString("<meta name='viewport' content='width=device-width, initial-scale=1'>")
	b.WriteString("<title>" + html.EscapeString(title) + "</title>")
	b.WriteString("<style>" + siteCSS() + "</style>")
	b.WriteString("</head><body>")

	b.WriteString("<header>")
	b.WriteString("<h1>" + html.EscapeString(title) + "</h1>")
	b.WriteString("<p>" + html.EscapeString(subtitle) + "</p>")
	b.WriteString("</header>")

	b.WriteString("<div class='navline'>")
	b.WriteString("<a class='navlink' href='index.html'>Back to class calendars</a>")
	b.WriteString("<a class='navlink secondary' href='all.html'>All calendars</a>")
	b.WriteString("<a class='navlink secondary' href='" + html.EscapeString(sourcePage) + "'>Source page</a>")
	b.WriteString("</div>")

	b.WriteString("<main>")

	b.WriteString("<section class='group'>")
	b.WriteString("<h2>Option 1: Subscribe by URL (recommended)</h2>")
	b.WriteString("<div class='small'>Best for automatic updates.</div>")
	b.WriteString("<div class='subgroup'>")
	b.WriteString("<div class='subhead'>Steps</div>")
	b.WriteString("<ul>")
	b.WriteString("<li><div class='row'><div class='row-left'>")
	b.WriteString("<div class='file'>Open <a href=\"https://calendar.google.com\" target=\"_blank\" rel=\"noopener noreferrer\">Google Calendar</a> on the web</div>")
	b.WriteString("<div class='small'>Use a browser on Android or desktop.</div>")
	b.WriteString("</div></div></li>")
	b.WriteString("<li><div class='row'><div class='row-left'>")
	b.WriteString("<div class='file'>Go to “Other calendars” → “From URL”</div>")
	b.WriteString("<div class='small'>This menu is not reliably available in the Android app.</div>")
	b.WriteString("</div></div></li>")
	b.WriteString("<li><div class='row'><div class='row-left'>")
	b.WriteString("<div class='file'>Copy the HTTPS link from this site</div>")
	b.WriteString("<div class='small'>Use the “Copy URL” button next to your class calendar.</div>")
	b.WriteString("</div></div></li>")
	b.WriteString("<li><div class='row'><div class='row-left'>")
	b.WriteString("<div class='file'>Paste the link and confirm</div>")
	b.WriteString("<div class='small'>The calendar should appear and update automatically.</div>")
	b.WriteString("</div></div></li>")
	b.WriteString("</ul>")
	b.WriteString("</div>")
	b.WriteString("</section>")

	b.WriteString("<section class='group'>")
	b.WriteString("<h2>Option 2: Import the file</h2>")
	b.WriteString("<div class='small'>Good for one-time import, not ideal for updates.</div>")
	b.WriteString("<div class='subgroup'>")
	b.WriteString("<div class='subhead'>Steps</div>")
	b.WriteString("<ul>")
	b.WriteString("<li><div class='row'><div class='row-left'>")
	b.WriteString("<div class='file'>Tap “Open file”</div>")
	b.WriteString("<div class='small'>Download the .ics file to your device.</div>")
	b.WriteString("</div></div></li>")
	b.WriteString("<li><div class='row'><div class='row-left'>")
	b.WriteString("<div class='file'>Open it with your calendar app</div>")
	b.WriteString("<div class='small'>Depending on vendor apps, the import dialog appears automatically.</div>")
	b.WriteString("</div></div></li>")
	b.WriteString("</ul>")
	b.WriteString("</div>")
	b.WriteString("</section>")

	b.WriteString("<section class='group'>")
	b.WriteString("<h2>Note</h2>")
	b.WriteString("<div class='small'>")
	b.WriteString("The “Subscribe” button uses the webcal protocol which is best supported on Apple devices. ")
	b.WriteString("On many Android setups, the safest path is using the HTTPS link or importing the file.")
	b.WriteString("</div>")
	b.WriteString("</section>")

	b.WriteString("</main>")
	b.WriteString("<footer>Updated by GitHub Actions on schedule.</footer>")
	b.WriteString("</body></html>")

	return os.WriteFile(path, []byte(b.String()), 0644)
}

func siteCSS() string {
	return `
:root{
  --bg:#0f1115; --card:#161a22; --text:#e6e6e6; --muted:#a7b0c0;
  --accent:#7aa2ff; --border:#262c3a; --accent-weak: rgba(122,162,255,.12);
  --success: rgba(120, 255, 170, .12);
}
*{box-sizing:border-box}
body{
  margin:0; font-family: system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif;
  background:linear-gradient(180deg, #0f1115, #0b0d12);
  color:var(--text);
}
header{
  padding:40px 20px 8px; text-align:center;
}
header h1{margin:0 0 6px; font-size:28px; letter-spacing:.3px}
header p{margin:0; color:var(--muted)}

.navline{
  max-width:1000px; margin:10px auto 0; padding:0 20px 10px;
  display:flex; gap:10px; justify-content:center; flex-wrap:wrap;
}
.navlink{
  display:inline-flex; align-items:center; gap:8px;
  padding:8px 12px; border-radius:10px;
  background:var(--accent-weak); color:var(--text);
  border:1px solid rgba(122,162,255,.35);
  text-decoration:none; font-size:12px; font-weight:600;
}
.navlink:hover{filter:brightness(1.08)}
.navlink.secondary{
  background:rgba(255,255,255,.04);
  border-color: var(--border);
  color: var(--muted);
}

/* Info box */
.infobox{
  max-width:1000px;
  margin: 6px auto 0;
  padding: 0 20px;
}
.infobox > div{
  background: rgba(255,255,255,.04);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 12px 14px;
}
.infobox-title{
  font-size: 12px;
  font-weight: 700;
  letter-spacing: .2px;
  margin-bottom: 4px;
}
.infobox-body{
  font-size: 11.5px;
  color: var(--muted);
}
.infobox-body a{
  color: var(--accent);
  text-decoration: none;
  font-weight: 600;
}
.infobox-body a:hover{
  text-decoration: underline;
}

.toolbar{
  max-width:1000px; margin:12px auto 0; padding:0 20px 8px;
  display:flex; gap:8px; flex-wrap:wrap; justify-content:center;
}
.toolbtn{
  display:inline-flex; align-items:center; gap:8px;
  padding:8px 12px; border-radius:10px;
  background:var(--accent-weak); color:var(--text);
  border:1px solid rgba(122,162,255,.35);
  text-decoration:none; font-size:12px; font-weight:600;
}
.toolbtn:hover{filter:brightness(1.08)}
.toolbtn .count{
  font-size:10px; padding:1px 6px; border-radius:999px;
  background:rgba(255,255,255,.06); border:1px solid var(--border);
  color:var(--muted);
}

main{
  max-width:1000px; margin:0 auto; padding:18px 20px 10px;
  display:grid; gap:16px;
}

.group{
  background:var(--card); border:1px solid var(--border);
  border-radius:14px; padding:18px 18px 8px;
  box-shadow: 0 6px 18px rgba(0,0,0,.25);
}
.group h2{
  margin:0 0 12px; font-size:18px;
  display:flex; align-items:center; gap:8px;
}
.badge{
  font-size:11px; padding:2px 8px; border-radius:999px;
  background:var(--accent-weak); color:var(--accent);
  border:1px solid rgba(122,162,255,.35);
}

.subgroup{
  border-top:1px solid var(--border);
  padding-top:12px; margin-top:12px;
}
.subgroup:first-of-type{
  border-top:none; padding-top:0; margin-top:0;
}
.subhead{
  display:flex; align-items:center; gap:8px;
  margin:0 0 8px; font-size:14px; color:var(--muted);
}
.subbadge{
  font-size:10px; padding:1px 7px; border-radius:999px;
  background:rgba(255,255,255,.06); border:1px solid var(--border);
}

ul{list-style:none; padding:0; margin:0}
li{border-top:1px dashed var(--border)}
li:first-child{border-top:none}

.row{
  display:flex; gap:10px; align-items:center; justify-content:space-between;
  padding:10px 6px;
}
.row-left{
  min-width:0; display:flex; flex-direction:column; gap:2px;
}
.file{
  font-weight:600; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;
  max-width:560px;
}
.file a{
  color: var(--accent);
  text-decoration: none;
  font-weight: 700;
}
.file a:hover{
  text-decoration: underline;
}
.small{
  color:var(--muted); font-size:11px;
}
.actions{
  display:flex; gap:6px; flex-wrap:wrap;
}
.btn{
  appearance:none; border:1px solid var(--border); background:rgba(255,255,255,.03);
  color:var(--text); padding:6px 9px; font-size:11px; border-radius:8px;
  cursor:pointer; text-decoration:none; font-weight:600;
}
.btn:hover{filter:brightness(1.08)}
.btn-primary{
  border-color: rgba(122,162,255,.45);
  background: var(--accent-weak);
}
.btn-success{
  border-color: rgba(120,255,170,.35);
  background: var(--success);
}

.note{
  max-width:1000px; margin:0 auto; padding:0 20px 10px;
  color:var(--muted); font-size:11px; text-align:center;
}

footer{
  max-width:1000px; margin:10px auto 40px; padding:0 20px;
  color:var(--muted); font-size:12px; text-align:center;
}
`
}

func siteJS() string {
	return `
<script>
function fileUrl(name){
  return new URL('ics_files/' + name, window.location.href).href;
}
function webcalUrl(httpsUrl){
  return httpsUrl.replace(/^https?:\/\//i, 'webcal://');
}
async function copyUrl(name, btn){
  const url = fileUrl(name);
  try{
    await navigator.clipboard.writeText(url);
    flash(btn, 'Copied', true);
  }catch(e){
    window.prompt('Copy this URL:', url);
  }
}
function subscribe(name){
  const url = fileUrl(name);
  const w = webcalUrl(url);
  window.location.href = w;
}
function flash(btn, text, ok){
  if(!btn) return;
  const old = btn.textContent;
  btn.textContent = text;
  if(ok){ btn.classList.add('btn-success'); }
  setTimeout(() => {
    btn.textContent = old;
    btn.classList.remove('btn-success');
  }, 900);
}
</script>
`
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func strconvI(i int) string {
	return fmt.Sprintf("%d", i)
}
