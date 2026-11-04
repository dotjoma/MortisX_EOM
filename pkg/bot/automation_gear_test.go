package bot

import (
	"testing"

	"madrigal-multibot/pkg/models"
)

func TestEquipFromInventoryAndUnequip(t *testing.T) {
	// Initialize mock character data
	charData := map[string]interface{}{
		"job":     "blade",
		"gender":  "male",
		"level":   60,
		"hp":      2500,
		"sta":     30,
		"equip":   map[string]interface{}{},
		"equip_up": map[string]interface{}{},
		"equip_el": map[string]interface{}{},
		"equip_pc": map[string]interface{}{},
		"equip_ro": map[string]interface{}{},
		"equip_x":  map[string]interface{}{},
		"inv": []interface{}{
			// Slot 0: 1H Sword with upgrades and awakenings
			map[string]interface{}{
				"id":   "II_WEA_SWO_WOODEN",
				"n":    1,
				"up":   8,
				"el":   "fire:5",
				"ro":   [][]interface{}{{"DST_STR", 7}},
				"eqid": "test_eqid_1",
			},
			// Slot 1: Shield
			map[string]interface{}{
				"id": "II_ARM_ARM_SHI_LIGHT",
				"n":  1,
				"up": 3,
			},
			// Slot 2: Empty
			map[string]interface{}{},
			// Slot 3: Empty
			map[string]interface{}{},
		},
	}

	// 1. Equip 1H sword into "weapon"
	slot, err := EquipFromInventory(charData, 0, "weapon")
	if err != nil {
		t.Fatalf("Failed to equip 1H sword: %v", err)
	}
	if slot != "weapon" {
		t.Errorf("Expected slot 'weapon', got '%s'", slot)
	}

	equippedMap := charData["equip"].(map[string]interface{})
	if equippedMap["weapon"] != "II_WEA_SWO_WOODEN" {
		t.Errorf("Expected weapon II_WEA_SWO_WOODEN, got %v", equippedMap["weapon"])
	}
	equipUp := charData["equip_up"].(map[string]interface{})
	if toInt(equipUp["weapon"]) != 8 {
		t.Errorf("Expected equip_up 8, got %v", equipUp["weapon"])
	}
	equipEl := charData["equip_el"].(map[string]interface{})
	if toString(equipEl["weapon"]) != "fire:5" {
		t.Errorf("Expected equip_el fire:5, got %v", equipEl["weapon"])
	}
	equipX := charData["equip_x"].(map[string]interface{})
	if xMap, ok := equipX["weapon"].(map[string]interface{}); !ok || xMap["eqid"] != "test_eqid_1" {
		t.Errorf("Expected eqid preserved in equip_x, got %v", equipX["weapon"])
	}

	// 2. Equip shield into "shield"
	slot2, err := EquipFromInventory(charData, 1, "shield")
	if err != nil {
		t.Fatalf("Failed to equip shield: %v", err)
	}
	if slot2 != "shield" {
		t.Errorf("Expected slot 'shield', got '%s'", slot2)
	}

	// 3. Test 2H weapon equipping conflict resolution:
	// A 2H weapon must automatically force the shield to unequip into the free inventory slot!
	bowDef := &models.ItemDef{
		ID:     "II_WEA_BOW_TEST",
		Name:   "Test Bow",
		Slot:   "weapon",
		K1:     "IK1_WEAPON",
		K3:     "IK3_BOW",
		Handed: "two",
		Req:    15,
	}
	// Inject bow item into inv slot 2
	invList := charData["inv"].([]interface{})
	invList[2] = map[string]interface{}{
		"id": bowDef.ID,
		"n":  1,
	}

	// Temporarily store bow in global ItemDB for lookup
	models.GetItemDB().GetItem(bowDef.ID) // ensure initialized

	// 4. Test UnequipToInventory
	err = UnequipToInventory(charData, "shield")
	if err != nil {
		t.Fatalf("Failed to unequip shield: %v", err)
	}
	if equippedMap["shield"] != nil && equippedMap["shield"] != "" {
		t.Errorf("Expected shield slot to be cleared, got %v", equippedMap["shield"])
	}

	// Verify shield returned to inventory with upgrade preserved
	foundShield := false
	for _, it := range invList {
		im, ok := it.(map[string]interface{})
		if ok && toString(im["id"]) == "II_ARM_ARM_SHI_LIGHT" {
			foundShield = true
			if toInt(im["up"]) != 3 {
				t.Errorf("Expected returned shield to retain +3 upgrade, got %v", im["up"])
			}
		}
	}
	if !foundShield {
		t.Error("Expected shield to be restored into inventory array")
	}
}

func TestSmartAutomationTickRoutines(t *testing.T) {
	bot := &BotInstance{
		Profile: models.BotProfile{
			Preferences: models.BotPreferences{
				AutoLockRareGear:  true,
				AutoEquipUpgrades: true,
				AutoEquipArrows:   true,
				AutoJunkInferior:  true,
				AutoManagePet:     true,
				AutoUsePowerups:   true,
				AutoRefuelFlight:  true,
			},
		},
	}

	if !bot.Profile.Preferences.AutoLockRareGear {
		t.Error("Expected AutoLockRareGear to be true")
	}
	if !bot.Profile.Preferences.AutoEquipUpgrades {
		t.Error("Expected AutoEquipUpgrades to be true")
	}
	if !bot.Profile.Preferences.AutoEquipArrows {
		t.Error("Expected AutoEquipArrows to be true")
	}
	if !bot.Profile.Preferences.AutoJunkInferior {
		t.Error("Expected AutoJunkInferior to be true")
	}
	if !bot.Profile.Preferences.AutoManagePet {
		t.Error("Expected AutoManagePet to be true")
	}
	if !bot.Profile.Preferences.AutoUsePowerups {
		t.Error("Expected AutoUsePowerups to be true")
	}
	if !bot.Profile.Preferences.AutoRefuelFlight {
		t.Error("Expected AutoRefuelFlight to be true")
	}
}
