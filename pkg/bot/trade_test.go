package bot

import (
	"encoding/json"
	"testing"
	"time"

	"madrigal-multibot/pkg/models"
)

func TestGetNearbyPlayers(t *testing.T) {
	b := &BotInstance{
		nearbyPlayers: make(map[int]*RemotePlayer),
		playerNames:   make(map[string]int),
	}

	b.nearbyPlayers[101] = &RemotePlayer{
		ID:       101,
		Name:     "Boywar",
		X:        6929.1,
		Y:        100.0,
		Z:        -3228.2,
		LastSeen: time.Now(),
	}
	// Stale player (> 30s old)
	b.nearbyPlayers[102] = &RemotePlayer{
		ID:       102,
		Name:     "OldGhost",
		X:        6900.0,
		Y:        100.0,
		Z:        -3200.0,
		LastSeen: time.Now().Add(-40 * time.Second),
	}

	list := b.GetNearbyPlayers()
	if len(list) != 1 {
		t.Fatalf("Expected 1 fresh nearby player, got %d", len(list))
	}
	if list[0].Name != "Boywar" || list[0].ID != 101 {
		t.Errorf("Unexpected player info: %+v", list[0])
	}
}

func TestAutoAcceptTradePreference(t *testing.T) {
	prefs := models.BotPreferences{
		AutoAcceptTrade: true,
	}
	if !prefs.AutoAcceptTrade {
		t.Errorf("Expected AutoAcceptTrade to be true")
	}
}

func TestLootReceivedUpdatesInventory(t *testing.T) {
	b := &BotInstance{
		ID:           "test-bot",
		saveFiles:    make(map[string]string),
		charFileName: "chars/test_0.json",
		Profile: models.BotProfile{
			CharacterName: "TotoyB",
		},
	}

	charData := map[string]interface{}{
		"name":  "TotoyB",
		"penya": float64(1000),
		"inv":   make([]interface{}, 10),
	}
	bytes, _ := json.Marshal(charData)
	b.saveFiles["chars/test_0.json"] = string(bytes)
	b.charSlot = 0

	var emittedBotID string
	var emittedInv map[string]interface{}
	b.SetOnInventoryUpdated(func(botID string, invData map[string]interface{}) {
		emittedBotID = botID
		emittedInv = invData
	})

	// Simulate receiving loot: 500 penya, 2 VitalDrink500, 1 Wooden Sword (+3)
	itemsRaw := []interface{}{
		[]interface{}{"II_GEN_POT_DRI_VITAL500", float64(2), float64(0)},
		[]interface{}{"II_WEA_SWO_WOODEN", float64(1), float64(3)},
	}
	b.handleLootReceived(500, itemsRaw)

	if emittedBotID != "test-bot" {
		t.Errorf("Expected callback for test-bot, got %s", emittedBotID)
	}
	if emittedInv == nil {
		t.Fatalf("Expected inventory data in callback")
	}

	penyaVal, _ := emittedInv["penya"].(float64)
	if penyaVal != 1500 {
		t.Errorf("Expected penya to be 1500, got %v", penyaVal)
	}

	invList, ok := emittedInv["inv"].([]interface{})
	if !ok || len(invList) == 0 {
		t.Fatalf("Expected inv list")
	}

	firstSlot, ok := invList[0].(map[string]interface{})
	if !ok || firstSlot["id"] != "II_GEN_POT_DRI_VITAL500" || int(toFloat32(firstSlot["n"])) != 2 {
		t.Errorf("Unexpected first slot: %+v", firstSlot)
	}

	secondSlot, ok := invList[1].(map[string]interface{})
	if !ok || secondSlot["id"] != "II_WEA_SWO_WOODEN" || int(toFloat32(secondSlot["up"])) != 3 {
		t.Errorf("Unexpected second slot: %+v", secondSlot)
	}
}
