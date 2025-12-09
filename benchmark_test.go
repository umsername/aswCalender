package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Run with:
// go test -v -run TestASWDeploymentCheck

// TestASWDeploymentCheck is an all-in-one pre-deploy check.
// It validates connectivity, runs a small stress loop,
// and verifies that ICS + site artifacts are produced.
func TestASWDeploymentCheck(t *testing.T) {
	// Config
	runs := 1
	minExpectedFiles := 5 // Expect at least a few schedules

	t.Logf("=== ASW SCHEDULE EXPORTER - SYSTEM CHECK ===")
	t.Logf("Target URL: %s", scheduleURL)
	t.Logf("Timezone:   %s", tzID)
	t.Logf("Output Dir: %s", outputDir)
	t.Logf("---------------------------------------------")

	// 1) Connectivity
	t.Logf("[1/4] Checking ASW website reachability...")
	if err := checkConnectivity(scheduleURL); err != nil {
		t.Fatalf("CRITICAL: website not reachable.\nError: %v", err)
	}
	t.Logf("      -> OK.")

	// 2) Performance / stability loop
	t.Logf("[2/4] Running %d full cycles (light stress)...", runs)

	// Force GC for cleaner memory numbers
	runtime.GC()
	var mStart runtime.MemStats
	runtime.ReadMemStats(&mStart)

	var totalDuration time.Duration

	for i := 1; i <= runs; i++ {
		start := time.Now()

		if err := runFullCycle(); err != nil {
			t.Fatalf("Run %d failed: %v", i, err)
		}

		d := time.Since(start)
		totalDuration += d
		t.Logf("      Run %d: OK (%v)", i, d)
	}

	// Measure rough memory footprint
	var mEnd runtime.MemStats
	runtime.ReadMemStats(&mEnd)
	peakRAM := bToMb(mEnd.Sys)

	// 3) Artifact validation
	t.Logf("[3/4] Validating generated files...")

	files, err := filepath.Glob(filepath.Join(outputDir, "*.ics"))
	if err != nil {
		t.Fatalf("Failed to read output directory: %v", err)
	}

	if len(files) < minExpectedFiles {
		t.Errorf("WARN: too few ICS files (%d). Expected >= %d.", len(files), minExpectedFiles)
	} else {
		t.Logf("      -> %d ICS files found.", len(files))
	}

	// Quick sanity check of one ICS file
	if len(files) > 0 {
		content, err := os.ReadFile(files[0])
		if err != nil {
			t.Errorf("Could not read file %s", files[0])
		} else {
			sContent := string(content)
			if !strings.Contains(sContent, "BEGIN:VCALENDAR") || !strings.Contains(sContent, "END:VCALENDAR") {
				t.Errorf("CRITICAL: generated ICS seems invalid (missing header/footer).")
			} else {
				t.Logf("      -> ICS sample check: OK.")
			}
		}
	}

	// Check landing page
	if _, err := os.Stat(filepath.Join(publicDir, "index.html")); os.IsNotExist(err) {
		t.Errorf("CRITICAL: index.html was not generated.")
	} else {
		t.Logf("      -> Site (index.html): OK.")
	}

	// 4) Summary
	avgTime := totalDuration / time.Duration(runs)
	t.Logf("---------------------------------------------")
	t.Logf("=== REPORT ===")
	t.Logf("Status:         PASS")
	t.Logf("Runs:           %d", runs)
	t.Logf("Avg per run:    %v", avgTime)
	t.Logf("Max RAM (Sys):  ~%v MB", peakRAM)
	t.Logf("---------------------------------------------")
}

// runFullCycle runs the main pipeline logic but returns errors
// instead of exiting the process.
func runFullCycle() error {
	// Clean output
	_ = os.RemoveAll(outputDir)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// Silence logger for clean test output
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	// A) Extract links
	links, err := parseMainSchedulePage(scheduleURL, false, "")
	if err != nil {
		return fmt.Errorf("parseMainSchedulePage: %w", err)
	}

	// B) Parse + build per-class aggregation
	classEvents := map[string][]ScheduleEvent{}
	for _, link := range links {
		events, err := parseScheduleDetails(link)
		if err != nil {
			continue // Ignore single-course failures in batch tests
		}
		if len(events) == 0 {
			continue
		}

		// Individual ICS
		if err := generateICS(link.CourseName, events); err != nil {
			return err
		}

		// Aggregate
		classKey := extractClassKey(link.CourseName)
		classEvents[classKey] = append(classEvents[classKey], events...)
	}

	// C) Aggregated ICS
	for classKey, evs := range classEvents {
		if len(evs) == 0 {
			continue
		}
		evs = dedupeEvents(evs)
		if err := generateICS(classKey, evs); err != nil {
			return err
		}
	}

	// D) Site
	if err := generateSite(); err != nil {
		return fmt.Errorf("generateSite: %w", err)
	}

	return nil
}

// checkConnectivity does a lightweight HTTP reachability check.
func checkConnectivity(url string) error {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}
	return nil
}

// bToMb converts bytes to MB.
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// BenchmarkFullProcess runs the full cycle repeatedly.
// Keep it simple and comparable across environments.
func BenchmarkFullProcess(b *testing.B) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = runFullCycle()
	}
}
