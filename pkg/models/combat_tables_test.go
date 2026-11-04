package models

import (
	"strings"
	"testing"
)

func TestCombatTables_Verification(t *testing.T) {
	tables := GetCombatTables()
	if tables == nil {
		t.Fatal("expected GetCombatTables() to not be nil")
	}

	// 1. Verify exact source SHA-256 hashes
	const expectedGameHash = "3003d622abb8d36528e54d7946eca32b2a6d5dd40aadfbfa4bd94f8e15b9da10"
	const expectedAnimHash = "fbae1804a6498412bd870bed712b8030751d5c4b3e4318610d8cab7c56de0249"

	if !strings.EqualFold(tables.SourceSHA256.GameJSON, expectedGameHash) {
		t.Errorf("game.json SHA256 mismatch: got %s, want %s", tables.SourceSHA256.GameJSON, expectedGameHash)
	}
	if !strings.EqualFold(tables.SourceSHA256.AnimEventsJSON, expectedAnimHash) {
		t.Errorf("anim_events.json SHA256 mismatch: got %s, want %s", tables.SourceSHA256.AnimEventsJSON, expectedAnimHash)
	}

	// 2. Verify exact row counts
	const expectedJobs = 29
	const expectedMonsters = 311
	const expectedArmors = 1819

	if len(tables.Jobs) != expectedJobs {
		t.Errorf("job count mismatch: got %d, want %d", len(tables.Jobs), expectedJobs)
	}
	if len(tables.Monsters) != expectedMonsters {
		t.Errorf("monster count mismatch: got %d, want %d", len(tables.Monsters), expectedMonsters)
	}
	if len(tables.Armors) != expectedArmors {
		t.Errorf("armor count mismatch: got %d, want %d", len(tables.Armors), expectedArmors)
	}

	// 3. Verify known jobs
	vag := GetJobCombatMeta("vagrant")
	if vag.HP != 0.9 || vag.Def != 1.0 || vag.Block != 0.2 {
		t.Errorf("vagrant combat meta mismatch: %+v", vag)
	}
	merc := GetJobCombatMeta("mercenary")
	if merc.HP != 1.5 || merc.Def != 1.35 || merc.Block != 0.8 {
		t.Errorf("mercenary combat meta mismatch: %+v", merc)
	}
	ass := GetJobCombatMeta("assist")
	if ass.HP != 1.4 || ass.Def != 1.2 || ass.Block != 0.5 {
		t.Errorf("assist combat meta mismatch: %+v", ass)
	}

	// 4. Verify known monsters
	aibatt, ok := GetMonsterCombatMeta("MI_AIBATT1")
	if !ok {
		t.Fatal("MI_AIBATT1 not found in monsters table")
	}
	if aibatt.Level != 1 || aibatt.HR != 40 || aibatt.DEX != 15 || len(aibatt.AtkM) != 2 || aibatt.AtkM[0] != 15 || aibatt.AtkM[1] != 16 {
		t.Errorf("MI_AIBATT1 stats mismatch: %+v", aibatt)
	}
	if aibatt.HitFracAtk1 <= 0.0 {
		t.Errorf("expected MI_AIBATT1 to have positive HitFracAtk1, got %f", aibatt.HitFracAtk1)
	}

	buru, ok := GetMonsterCombatMeta("MI_BURUDENG3")
	if !ok {
		t.Fatal("MI_BURUDENG3 not found in monsters table")
	}
	if buru.Level != 6 || buru.HR != 40 || buru.DEX != 25 || buru.Rank != "RANK_CAPTAIN" {
		t.Errorf("MI_BURUDENG3 stats mismatch: %+v", buru)
	}

	fefern, ok := GetMonsterCombatMeta("MI_FEFERN1")
	if !ok {
		t.Fatal("MI_FEFERN1 not found in monsters table")
	}
	if fefern.Level != 15 || fefern.HR != 27 || fefern.DEX != 43 || fefern.Rank != "RANK_LOW" {
		t.Errorf("MI_FEFERN1 stats mismatch: %+v", fefern)
	}

	// 5. Verify known armors
	cream, ok := GetArmorCombatMeta("II_ARM_F_VAG_SUIT01")
	if !ok {
		t.Fatal("II_ARM_F_VAG_SUIT01 not found in armors table")
	}
	if cream.Slot != "suit" || len(cream.AB) != 2 || cream.AB[0] != 8 || cream.AB[1] != 10 || cream.Parry != 0 {
		t.Errorf("Cream Suit attributes mismatch: %+v", cream)
	}
}
