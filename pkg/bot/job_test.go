package bot

import (
	"testing"

	"madrigal-multibot/pkg/models"
)

func TestStatDistributionPreset(t *testing.T) {
	// 1. Pure STR preset
	s, sta, d, i := models.CalculateStatDistribution(models.StatModePreset, "pure_str", models.CustomStatRatio{}, 15, 15, 15, 15, 50)
	if s != 50 || sta != 0 || d != 0 || i != 0 {
		t.Fatalf("expected 50 STR, got s=%d, sta=%d, d=%d, i=%d", s, sta, d, i)
	}

	// 2. Pure INT preset
	s, sta, d, i = models.CalculateStatDistribution(models.StatModePreset, "pure_int", models.CustomStatRatio{}, 15, 15, 15, 15, 80)
	if s != 0 || sta != 0 || d != 0 || i != 80 {
		t.Fatalf("expected 80 INT, got s=%d, sta=%d, d=%d, i=%d", s, sta, d, i)
	}

	// 3. Dex Cap 43 preset (Blade classic meta: base dex 15 -> needs 28 to hit 43, rest STR)
	s, sta, d, i = models.CalculateStatDistribution(models.StatModePreset, "dex_cap_43", models.CustomStatRatio{}, 15, 15, 15, 15, 118)
	if d != 28 {
		t.Fatalf("expected 28 DEX added to reach 43, got %d", d)
	}
	if s != 90 {
		t.Fatalf("expected 90 STR added (118 - 28), got %d", s)
	}
	if s+sta+d+i != 118 {
		t.Fatalf("stat point sum mismatch: %d != 118", s+sta+d+i)
	}
}

func TestStatDistributionThreshold(t *testing.T) {
	// Custom threshold: Cap DEX at 40, rest in STA
	cfg := models.CustomStatRatio{
		CapStat:  "dex",
		CapValue: 40,
		ThenStat: "sta",
	}
	// Starting at 20 DEX with 50 free points
	s, sta, d, i := models.CalculateStatDistribution(models.StatModeThreshold, "", cfg, 15, 15, 20, 15, 50)
	if d != 20 { // 40 - 20 = 20
		t.Fatalf("expected 20 DEX added, got %d", d)
	}
	if sta != 30 { // 50 - 20 = 30
		t.Fatalf("expected 30 STA added, got %d", sta)
	}
	if s != 0 || i != 0 {
		t.Fatalf("expected 0 STR/INT, got s=%d, i=%d", s, i)
	}
}

func TestStatDistributionRatio(t *testing.T) {
	// 70% STR / 30% STA with 100 free points
	cfg := models.CustomStatRatio{
		StrPercent: 70,
		StaPercent: 30,
	}
	s, sta, d, i := models.CalculateStatDistribution(models.StatModeRatio, "", cfg, 15, 15, 15, 15, 100)
	if s != 70 || sta != 30 || d != 0 || i != 0 {
		t.Fatalf("expected 70 STR / 30 STA, got s=%d, sta=%d, d=%d, i=%d", s, sta, d, i)
	}
	if s+sta+d+i != 100 {
		t.Fatalf("expected exact 100 points, got %d", s+sta+d+i)
	}

	// Odd points test: 70% / 30% with 33 points
	s, sta, d, i = models.CalculateStatDistribution(models.StatModeRatio, "", cfg, 15, 15, 15, 15, 33)
	if s+sta+d+i != 33 {
		t.Fatalf("rounding point leak! expected exactly 33 points total, got %d (s=%d, sta=%d)", s+sta+d+i, s, sta)
	}
}
