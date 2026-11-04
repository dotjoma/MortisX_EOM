package bot

import (
	"os"
	"testing"
)

func TestDefaultClassBuilds(t *testing.T) {
	builds := DefaultClassBuilds()

	// Verify all 12 classes are defined
	requiredJobs := []string{
		"mercenary", "blade", "knight",
		"acrobat", "jester", "ranger",
		"assist", "ringmaster", "billposter",
		"magician", "psykeeper", "elementor",
	}

	for _, job := range requiredJobs {
		b, exists := builds[job]
		if !exists {
			t.Fatalf("missing default build for class %s", job)
		}
		if b.Job != job {
			t.Fatalf("expected job %s, got %s", job, b.Job)
		}
	}

	// Verify Mercenary has 43 DEX cap meta
	merc := builds["mercenary"]
	if merc.Mode != "cap" || merc.CapStat != "dex" || merc.CapValue != 43 || merc.ThenStat != "str" {
		t.Fatalf("invalid mercenary meta build: %+v", merc)
	}

	// Verify Assist has pure INT meta
	assist := builds["assist"]
	if assist.Mode != "pure" || assist.CapStat != "int" || assist.AutoStat != 4 {
		t.Fatalf("invalid assist meta build: %+v", assist)
	}
}

func TestEnsureStatsJsonExists(t *testing.T) {
	targetPath, err := GetStatsJsonPath()
	if err != nil {
		targetPath = "stats.json"
	}

	// Clean up any existing temp file
	_ = os.Remove(targetPath)

	builds, err := EnsureStatsJsonExists()
	if err != nil {
		t.Fatalf("EnsureStatsJsonExists failed: %v", err)
	}

	if len(builds) < 12 {
		t.Fatalf("expected at least 12 builds, got %d", len(builds))
	}

	// Verify file was written to disk
	if _, statErr := os.Stat(targetPath); os.IsNotExist(statErr) {
		t.Fatalf("stats.json was not created on disk at %s", targetPath)
	}

	// Load again from disk
	loaded, lErr := LoadStatsJson()
	if lErr != nil {
		t.Fatalf("LoadStatsJson failed: %v", lErr)
	}

	if loaded["blade"].CapValue != 43 {
		t.Fatalf("expected blade cap_value 43, got %d", loaded["blade"].CapValue)
	}
}
