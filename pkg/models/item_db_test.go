package models

import (
	"testing"
)

func TestItemDatabaseLoadingAndScoring(t *testing.T) {
	db := GetItemDB()
	if db == nil {
		t.Fatal("Expected ItemDatabase instance, got nil")
	}

	// 1. Verify item retrieval
	sword := db.GetItem("II_WEA_SWO_WOODEN")
	if sword == nil {
		t.Fatal("Failed to retrieve Wooden Sword (II_WEA_SWO_WOODEN)")
	}
	if sword.Slot != "weapon" {
		t.Errorf("Expected slot 'weapon', got %s", sword.Slot)
	}
	if !IsEquip(sword) {
		t.Error("Expected IsEquip to be true for Wooden Sword")
	}
	if IsTwoHanded(sword) {
		t.Error("Wooden sword is 1H, expected IsTwoHanded to be false")
	}

	// 2. Verify job line
	bladeJobs := JobLine("blade")
	expectedBladeJobs := []string{"vagrant", "mercenary", "blade"}
	if len(bladeJobs) != len(expectedBladeJobs) {
		t.Fatalf("Expected %d jobs in blade line, got %d", len(expectedBladeJobs), len(bladeJobs))
	}
	for i, j := range expectedBladeJobs {
		if bladeJobs[i] != j {
			t.Errorf("Expected %s, got %s", j, bladeJobs[i])
		}
	}

	// 3. Verify pre-equip checks
	can, errStr := CanEquip("vagrant", "male", 1, sword, 0, nil)
	if !can {
		t.Errorf("Expected Vagrant Lv 1 to equip Wooden Sword, failed: %s", errStr)
	}

	// Gender mismatch test
	femaleCostume := &ItemDef{
		Slot: "suit",
		Sex:  "f",
		Req:  1,
	}
	canFem, _ := CanEquip("vagrant", "male", 1, femaleCostume, 0, nil)
	if canFem {
		t.Error("Expected male character to be blocked from equipping female suit")
	}

	// Level mismatch test
	highLevelWeapon := &ItemDef{
		Slot: "weapon",
		Req:  60,
	}
	canHigh, _ := CanEquip("vagrant", "male", 15, highLevelWeapon, 0, nil)
	if canHigh {
		t.Error("Expected Lv 15 character to be blocked from Lv 60 weapon")
	}

	// Level down reduction test (Scroll of Reduction: ld = -10)
	canHighReduced, _ := CanEquip("vagrant", "male", 50, highLevelWeapon, -10, nil)
	if !canHighReduced {
		t.Error("Expected Lv 50 character with ld=-10 to equip Lv 60 weapon (effective req 50)")
	}

	// 4. Verify gear scoring
	scoreBase := CalculateGearScore(sword, 0, "", nil, nil, "mercenary")
	scoreUp10 := CalculateGearScore(sword, 10, "fire:10", nil, nil, "mercenary")
	if scoreUp10 <= scoreBase {
		t.Errorf("Expected +10 fire upgraded weapon to score higher than base (%f vs %f)", scoreUp10, scoreBase)
	}

	// Verify awakening bonus
	roBonus := [][]interface{}{{"DST_STR", 20}, {"DST_ATKPOWER_RATE", 10}}
	scoreAwakened := CalculateGearScore(sword, 0, "", roBonus, nil, "mercenary")
	if scoreAwakened <= scoreBase {
		t.Errorf("Expected awakened weapon to score higher than base (%f vs %f)", scoreAwakened, scoreBase)
	}
}
