package bot

import (
	"encoding/json"
	"testing"

	"madrigal-multibot/pkg/models"
)

func TestItemCleanerProtections(t *testing.T) {
	rules := CleanRules{
		AutoCleanJunk:       true,
		AutoSellBelowRarity: 120,
	}

	charData := map[string]interface{}{
		"penya":    1000.0,
		"pet_out":  "uid_active_pet",
		"inv":      make([]interface{}, 112),
	}

	// 1. Monster Album Card Piece with cost: 0 must be PROTECTED
	slotCard := map[string]interface{}{
		"id": "II_MONSTERALBUM_AIBATT_PIECE",
		"n":  10,
	}
	metaCard := &models.ItemDef{
		ID:   "II_MONSTERALBUM_AIBATT_PIECE",
		K3:   "IK3_MONSTER_ALBUM",
		Cost: 0,
		Rare: 0,
	}
	decCard := DecideCleanAction(slotCard, metaCard, 1000, rules, charData)
	if decCard.Action != ActionKeep {
		t.Fatalf("expected Monster Album card piece to be Kept, got %v (%s)", decCard.Action, decCard.Reason)
	}

	// 2. Bound SProtect with cost: 0 and bd: true (not marked junk) must be PROTECTED
	slotSProtect := map[string]interface{}{
		"id": "II_SYS_SYS_SCR_SPROTECT",
		"n":  5,
		"bd": true,
	}
	metaSProtect := &models.ItemDef{
		ID:   "II_SYS_SYS_SCR_SPROTECT",
		Cost: 0,
		Rare: 0,
	}
	decSProtect := DecideCleanAction(slotSProtect, metaSProtect, 1000, rules, charData)
	if decSProtect.Action != ActionKeep {
		t.Fatalf("expected Bound SProtect to be Kept, got %v (%s)", decSProtect.Action, decSProtect.Reason)
	}

	// 3. Mounts / Wings / Cloak / Jewelry with non-standard equip slot must be PROTECTED
	slotWings := map[string]interface{}{
		"id": "II_ARM_WINGS_ANGEL",
		"n":  1,
	}
	metaWings := &models.ItemDef{
		ID:   "II_ARM_WINGS_ANGEL",
		Slot: "wings",
		Cost: 10000,
		Rare: 0,
	}
	decWings := DecideCleanAction(slotWings, metaWings, 1000, rules, charData)
	if decWings.Action != ActionKeep {
		t.Fatalf("expected Wings to be Kept, got %v (%s)", decWings.Action, decWings.Reason)
	}

	// 4. Set gear or Rare >= 200 must be PROTECTED
	slotSet := map[string]interface{}{
		"id": "II_ARM_M_VAG_SET_SUIT",
		"n":  1,
	}
	metaSet := &models.ItemDef{
		ID:   "II_ARM_M_VAG_SET_SUIT",
		Slot: "suit",
		Cost: 5000,
		Rare: 10,
		Set:  "1",
	}
	decSet := DecideCleanAction(slotSet, metaSet, 1000, rules, charData)
	if decSet.Action != ActionKeep {
		t.Fatalf("expected Set item to be Kept, got %v (%s)", decSet.Action, decSet.Reason)
	}

	// 5. Upgraded gear (+3) or Awakened ("ro") must be PROTECTED
	slotUp := map[string]interface{}{
		"id": "II_ARM_M_VAG_SUIT01",
		"n":  1,
		"up": 3,
	}
	metaUp := &models.ItemDef{
		ID:   "II_ARM_M_VAG_SUIT01",
		Slot: "suit",
		Cost: 500,
		Rare: 1,
	}
	decUp := DecideCleanAction(slotUp, metaUp, 1000, rules, charData)
	if decUp.Action != ActionKeep {
		t.Fatalf("expected upgraded item to be Kept, got %v (%s)", decUp.Action, decUp.Reason)
	}

	// 6. Active deployed pet matching charData["pet_out"] must be PROTECTED
	slotPet := map[string]interface{}{
		"id":  "II_PET_BEAR",
		"n":   1,
		"uid": "uid_active_pet",
	}
	metaPet := &models.ItemDef{
		ID: "II_PET_BEAR",
		K3: "IK3_PET",
	}
	decPet := DecideCleanAction(slotPet, metaPet, 1000, rules, charData)
	if decPet.Action != ActionKeep {
		t.Fatalf("expected active deployed pet to be Kept, got %v (%s)", decPet.Action, decPet.Reason)
	}
}

func TestItemCleanerRouting(t *testing.T) {
	rules := CleanRules{
		AutoCleanJunk:       true,
		AutoSellBelowRarity: 120,
	}
	charData := map[string]interface{}{
		"penya": 1000.0,
		"inv":   make([]interface{}, 112),
	}

	// 1. Normal monster drop gear (Cost: 100, Rare: 1) -> Ruta 3 (Auto Sell)
	slotGear := map[string]interface{}{
		"id": "II_ARM_M_VAG_SUIT01",
		"n":  2,
	}
	metaGear := &models.ItemDef{
		ID:   "II_ARM_M_VAG_SUIT01",
		Slot: "suit",
		Cost: 100,
		Rare: 1,
	}
	decGear := DecideCleanAction(slotGear, metaGear, 1000, rules, charData)
	if decGear.Action != ActionSell {
		t.Fatalf("expected normal gear to be Sold, got %v (%s)", decGear.Action, decGear.Reason)
	}
	if decGear.UnitPrice != 25 || decGear.TotalPrice != 50 {
		t.Fatalf("expected unit price 25 total 50, got unit=%d total=%d", decGear.UnitPrice, decGear.TotalPrice)
	}

	// 2. Low-cost item (Cost: 1, e.g. arrow or card piece) marked junk -> Ruta 2 (Auto Trash, 0 Penya)
	slotArrow := map[string]interface{}{
		"id": "II_WEA_ARR_WOOD",
		"n":  50,
		"jk": true,
	}
	metaArrow := &models.ItemDef{
		ID:   "II_WEA_ARR_WOOD",
		Cost: 1,
		Rare: 1,
	}
	decArrow := DecideCleanAction(slotArrow, metaArrow, 1000, rules, charData)
	if decArrow.Action != ActionTrash {
		t.Fatalf("expected cost < 4 item to be Trashed, got %v (%s)", decArrow.Action, decArrow.Reason)
	}
	if decArrow.TotalPrice != 0 {
		t.Fatalf("expected 0 penya for trashed item, got %d", decArrow.TotalPrice)
	}

	// 3. Bound item marked junk -> Ruta 2 (Auto Trash, 0 Penya)
	slotBoundJunk := map[string]interface{}{
		"id": "II_ARM_M_VAG_SUIT02",
		"n":  1,
		"jk": true,
		"bd": true,
	}
	metaBound := &models.ItemDef{
		ID:   "II_ARM_M_VAG_SUIT02",
		Slot: "suit",
		Cost: 500,
		Rare: 1,
	}
	decBound := DecideCleanAction(slotBoundJunk, metaBound, 1000, rules, charData)
	if decBound.Action != ActionTrash {
		t.Fatalf("expected bound junk to be Trashed, got %v (%s)", decBound.Action, decBound.Reason)
	}
	if decBound.TotalPrice != 0 {
		t.Fatalf("expected 0 penya for bound trashed item, got %d", decBound.TotalPrice)
	}

	// 4. Quest item marked junk -> Ruta 2 (Auto Trash, 0 Penya)
	slotQuest := map[string]interface{}{
		"id": "II_SYS_QUE_QUESTITEM",
		"n":  1,
		"jk": true,
	}
	metaQuest := &models.ItemDef{
		ID:   "II_SYS_QUE_QUESTITEM",
		K3:   "IK3_QUEST",
		Cost: 200,
		Rare: 1,
	}
	decQuest := DecideCleanAction(slotQuest, metaQuest, 1000, rules, charData)
	if decQuest.Action != ActionTrash {
		t.Fatalf("expected quest junk to be Trashed, got %v (%s)", decQuest.Action, decQuest.Reason)
	}
	if decQuest.TotalPrice != 0 {
		t.Fatalf("expected 0 penya for quest trashed item, got %d", decQuest.TotalPrice)
	}

	// 5. Default safe penya cap overflow (wallet = 298,990, item price = 50 -> exceeds 299,000)
	decOverflow := DecideCleanAction(slotGear, metaGear, 298990, rules, charData)
	if decOverflow.Action != ActionTrash {
		t.Fatalf("expected penya cap overflow item to be Trashed, got %v (%s)", decOverflow.Action, decOverflow.Reason)
	}
	if decOverflow.TotalPrice != 0 {
		t.Fatalf("expected 0 penya on overflow trash, got %d", decOverflow.TotalPrice)
	}

	// 6. Configurable cap (e.g. 120,000)
	customRules := CleanRules{
		AutoCleanJunk:       true,
		AutoSellBelowRarity: 120,
		MaxPenyaCap:         120000,
	}
	decCustomOverflow := DecideCleanAction(slotGear, metaGear, 119990, customRules, charData)
	if decCustomOverflow.Action != ActionTrash {
		t.Fatalf("expected custom 120k cap overflow to be Trashed, got %v", decCustomOverflow.Action)
	}
}

func TestNormalizeInventoryFixedLengthAndEmptySlots(t *testing.T) {
	// Start with slice of length 5 containing nil and empty map
	shortInv := []interface{}{
		nil,
		map[string]interface{}{"id": "II_ARM_M_VAG_SUIT01", "n": 1},
		map[string]interface{}{},
		nil,
	}

	norm := NormalizeInventory(shortInv)
	if len(norm) != InvSize {
		t.Fatalf("expected inventory length %d, got %d", InvSize, len(norm))
	}

	// Verify JSON marshaling produces "{}" and never "null"
	bytes, err := json.Marshal(norm)
	if err != nil {
		t.Fatalf("json marshal error: %v", err)
	}
	jsonStr := string(bytes)
	if jsonStr == "" || jsonStr[0] != '[' {
		t.Fatalf("unexpected json string: %s", jsonStr)
	}
	if containsNull := json.Valid(bytes); !containsNull {
		t.Fatalf("invalid json")
	}

	// Check each slot in slice is not nil
	for i, slot := range norm {
		if slot == nil {
			t.Fatalf("slot %d is nil", i)
		}
	}
}

func TestProcessAutoCleanPass(t *testing.T) {
	charData := map[string]interface{}{
		"penya": 100.0,
		"inv": []interface{}{
			// Slot 0: normal gear (Sell for 25)
			map[string]interface{}{"id": "II_ARM_M_VAG_SUIT01", "n": 1, "jk": true},
			// Slot 1: bound junk (Trash for 0)
			map[string]interface{}{"id": "II_ARM_M_VAG_SUIT02", "n": 1, "jk": true, "bd": true},
			// Slot 2: protected upgraded gear (Keep)
			map[string]interface{}{"id": "II_ARM_M_VAG_SUIT01", "n": 1, "up": 1},
		},
	}

	rules := CleanRules{
		AutoCleanJunk:       true,
		AutoSellBelowRarity: 120,
	}

	summary := ProcessAutoCleanPass(charData, rules, nil)
	if summary.SoldCount != 1 {
		t.Fatalf("expected 1 sold, got %d", summary.SoldCount)
	}
	if summary.TrashedCount != 1 {
		t.Fatalf("expected 1 trashed, got %d", summary.TrashedCount)
	}

	// Slot 0 was emptied
	inv := charData["inv"].([]interface{})
	if !IsEmptySlot(inv[0]) {
		t.Fatalf("expected slot 0 to be empty")
	}
	// Slot 1 was emptied
	if !IsEmptySlot(inv[1]) {
		t.Fatalf("expected slot 1 to be empty")
	}
	// Slot 2 was kept
	if IsEmptySlot(inv[2]) {
		t.Fatalf("expected slot 2 to be kept")
	}

	// Fixed length 112
	if len(inv) != InvSize {
		t.Fatalf("expected inv length %d, got %d", InvSize, len(inv))
	}
}
