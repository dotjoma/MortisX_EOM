package models

import (
	"encoding/json"
	"testing"
)

func TestDefaultPartyBoostConfig(t *testing.T) {
	cfg := DefaultPartyBoostConfig()
	if cfg.Enabled {
		t.Errorf("expected Enabled to be false by default")
	}
	if cfg.LeashDistance != 60.0 {
		t.Errorf("expected default leash distance 60.0, got %f", cfg.LeashDistance)
	}
	if cfg.ExpShareMode != 1 {
		t.Errorf("expected default exp share mode 1 (contribution), got %d", cfg.ExpShareMode)
	}
	if cfg.ItemDistMode != 1 {
		t.Errorf("expected default item dist mode 1 (sequentially), got %d", cfg.ItemDistMode)
	}
	if !cfg.AutoAdvance {
		t.Errorf("expected AutoAdvance to be true by default")
	}
	if !cfg.SkillsEnabled[PartySkillLinkAttack] {
		t.Errorf("expected Link Attack to be enabled by default")
	}
}

func TestPartyBoostConfig_Sanitize(t *testing.T) {
	cfg := PartyBoostConfig{
		LeaderName:    "SuperLeader",
		LeashDistance: -5.0,
		ExpShareMode:  99,
		ItemDistMode:  -1,
		SelectedMembers: []string{
			"SuperLeader", // Should be filtered (matches leader)
			"Member1",
			"Member2",
			"member1", // Duplicate case-insensitive
			"Member3",
			"Member4",
			"Member5",
			"Member6",
			"Member7",
			"Member8", // Over 7 cap, should be dropped
			"Member9",
		},
	}

	cfg.Sanitize()

	if cfg.LeashDistance != 60.0 {
		t.Errorf("expected leash to fallback to 60.0, got %f", cfg.LeashDistance)
	}
	if cfg.ExpShareMode != 1 {
		t.Errorf("expected exp share mode fallback to 1, got %d", cfg.ExpShareMode)
	}
	if cfg.ItemDistMode != 1 {
		t.Errorf("expected item dist mode fallback to 1, got %d", cfg.ItemDistMode)
	}
	if len(cfg.SelectedMembers) != 7 {
		t.Fatalf("expected exactly 7 members after sanitize, got %d: %v", len(cfg.SelectedMembers), cfg.SelectedMembers)
	}
	for _, m := range cfg.SelectedMembers {
		if m == "SuperLeader" {
			t.Errorf("leader should not be in SelectedMembers")
		}
	}
}

func TestPartyBoostConfig_JSONRoundTrip(t *testing.T) {
	original := DefaultPartyBoostConfig()
	original.Enabled = true
	original.LeaderName = "MainTank"
	original.SelectedMembers = []string{"Healer1", "DPS1"}
	original.ScrollsEnabled[PartyScrollExpUp] = true

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed PartyBoostConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed.LeaderName != "MainTank" || len(parsed.SelectedMembers) != 2 || !parsed.ScrollsEnabled[PartyScrollExpUp] {
		t.Errorf("mismatched parsed config: %+v", parsed)
	}
}
