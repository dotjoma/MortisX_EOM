package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"madrigal-multibot/pkg/client"
	"madrigal-multibot/pkg/models"
)

// TestValidatorSplitsAndRejectsNonStackableX2 verifies that the inventory validator
// detects slots with count > maxStack, clamps them, and splits excess into free slots.
// Also verifies addItemToInventoryWithUp never merges non-stackable items.
func TestValidatorSplitsAndRejectsNonStackableX2(t *testing.T) {
	// 1. Test ValidateAndRepairInventory on corrupted non-stackable x2 slot
	invList := make([]interface{}, 10)
	for i := range invList {
		invList[i] = map[string]interface{}{}
	}
	invList[0] = map[string]interface{}{
		"id": "II_ARM_F_VAG_SUIT02",
		"n":  2,
	}

	repaired := models.ValidateAndRepairInventory(invList)
	if !repaired {
		t.Fatalf("expected ValidateAndRepairInventory to return true for repair")
	}

	slot0 := invList[0].(map[string]interface{})
	if slot0["n"] != 1 {
		t.Fatalf("expected slot 0 count to be clamped to 1, got %v", slot0["n"])
	}

	slot1 := invList[1].(map[string]interface{})
	if slot1["id"] != "II_ARM_F_VAG_SUIT02" || slot1["n"] != 1 {
		t.Fatalf("expected excess count to be split to slot 1 as x1, got id=%v n=%v", slot1["id"], slot1["n"])
	}

	// 2. Test addItemToInventoryWithUp for non-stackable item: must NOT merge into existing slot
	inv2 := make([]interface{}, 10)
	for i := range inv2 {
		inv2[i] = map[string]interface{}{}
	}

	added1 := addItemToInventoryWithUp(inv2, "II_ARM_F_VAG_SUIT02", 1, 0)
	if !added1 {
		t.Fatalf("failed to add first suit")
	}
	added2 := addItemToInventoryWithUp(inv2, "II_ARM_F_VAG_SUIT02", 1, 0)
	if !added2 {
		t.Fatalf("failed to add second suit")
	}

	s0 := inv2[0].(map[string]interface{})
	s1 := inv2[1].(map[string]interface{})
	if s0["id"] != "II_ARM_F_VAG_SUIT02" || s0["n"] != 1 {
		t.Fatalf("expected first suit in slot 0 x1, got %v", s0)
	}
	if s1["id"] != "II_ARM_F_VAG_SUIT02" || s1["n"] != 1 {
		t.Fatalf("expected second suit in slot 1 x1 (no merging), got %v", s1)
	}

	// 3. Test addItemToInventoryWithUp for stackable item: MUST merge into existing slot
	addedFood1 := addItemToInventoryWithUp(inv2, "II_GEN_FOO_INS_LOLLIPOP", 10, 0)
	if !addedFood1 {
		t.Fatalf("failed to add food 1")
	}
	addedFood2 := addItemToInventoryWithUp(inv2, "II_GEN_FOO_INS_LOLLIPOP", 15, 0)
	if !addedFood2 {
		t.Fatalf("failed to add food 2")
	}

	s2 := inv2[2].(map[string]interface{})
	if s2["id"] != "II_GEN_FOO_INS_LOLLIPOP" || s2["n"] != 25 {
		t.Fatalf("expected stackable item to merge to count 25, got %v", s2)
	}
}

// TestSlowHTTPSaveDoesNotDelayDEvent verifies that slow HTTP saves do NOT block
// handleLootReceived or delay "d" (mob death) packet handling.
func TestSlowHTTPSaveDoesNotDelayDEvent(t *testing.T) {
	var saveCallCount int32
	saveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/save" {
			atomic.AddInt32(&saveCallCount, 1)
			// Simulate slow HTTP server taking 500 ms
			time.Sleep(500 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer saveServer.Close()

	customCli := client.NewGameHttpClient()
	customCli.BaseURL = saveServer.URL

	bot := NewBotInstance(models.BotProfile{
		ID:            "bot_fast_save",
		CharacterName: "TestHero",
	}, nil, nil, nil, nil)

	bot.httpCli = customCli
	bot.Status = "online"
	bot.sessionToken = "dummy_token"
	bot.charFileName = "chars/test_0.json"
	bot.saveFiles["chars/test_0.json"] = models.CreateDefaultCharacterSave("TestHero", "female")

	// 1. handleLootReceived must be synchronous in memory and NEVER block on HTTP
	t0 := time.Now()
	itemsRaw := []interface{}{
		[]interface{}{"II_ARM_F_VAG_SUIT02", 1},
	}
	bot.handleLootReceived(100, itemsRaw)
	lootDuration := time.Since(t0)

	if lootDuration > 50*time.Millisecond {
		t.Fatalf("handleLootReceived took %v (expected <50ms, must not block on HTTP!)", lootDuration)
	}
	if !bot.saveDirty {
		t.Fatalf("expected saveDirty to be true after handleLootReceived")
	}

	// 2. Start background saver flush (runs 500ms slow HTTP POST)
	go bot.flushSave(false)

	// Wait briefly so saver starts and enters HTTP sleep
	time.Sleep(30 * time.Millisecond)
	bot.mu.Lock()
	inFlight := bot.saveInFlight
	bot.mu.Unlock()
	if !inFlight {
		t.Fatalf("expected saveInFlight to be true while HTTP POST is running")
	}

	// 3. While HTTP POST is blocked for 500ms, process "d" event: must NOT be blocked!
	mobID := 999
	bot.Farm.mobsMu.Lock()
	bot.Farm.mobs[mobID] = &TrackedMob{
		ID:    mobID,
		Name:  "Peakyturtle",
		HP:    100,
		MaxHP: 100,
	}
	bot.Farm.currentTarget = bot.Farm.mobs[mobID]
	bot.Farm.mobsMu.Unlock()

	tD0 := time.Now()
	bot.Farm.OnMobPacket(map[string]interface{}{
		"e": []interface{}{
			[]interface{}{"d", mobID, 1},
		},
	})
	dDuration := time.Since(tD0)

	if dDuration > 50*time.Millisecond {
		t.Fatalf("OnMobPacket 'd' event took %v (delayed by HTTP save!)", dDuration)
	}

	bot.Farm.mobsMu.RLock()
	hasTarget := bot.Farm.currentTarget != nil
	bot.Farm.mobsMu.RUnlock()
	if hasTarget {
		t.Fatalf("expected currentTarget to be cleared immediately by event 'd'")
	}

	// Wait for background save to finish
	time.Sleep(600 * time.Millisecond)
	if atomic.LoadInt32(&saveCallCount) != 1 {
		t.Fatalf("expected 1 save call, got %d", atomic.LoadInt32(&saveCallCount))
	}
}

// TestSaveResyncPathTriggered verifies that HTTP 400 with "resync": 1 triggers
// the /api/session resync flow and does NOT retry-spam.
func TestSaveResyncPathTriggered(t *testing.T) {
	var saveAttempts int32
	var sessionCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/save" {
			atomic.AddInt32(&saveAttempts, 1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Save refused: II_ARM_F_VAG_SUIT02 x2 (stack 1)","resync":1}`))
			return
		}
		if r.URL.Path == "/api/session" {
			atomic.AddInt32(&sessionCalls, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"token":"fresh_tok","account":{"key":"test"},"files":{"chars/test_0.json":"{\"name\":\"ServerHero\",\"reloaded\":true}"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	customCli := client.NewGameHttpClient()
	customCli.BaseURL = server.URL

	bot := NewBotInstance(models.BotProfile{
		ID:            "bot_resync",
		CharacterName: "TestHero",
	}, nil, nil, nil, nil)

	bot.httpCli = customCli
	bot.Status = "online"
	bot.sessionToken = "initial_token"
	bot.charFileName = "chars/test_0.json"
	bot.saveFiles["chars/test_0.json"] = `{"name":"OldCorrupted"}`
	bot.saveDirty = true

	// Trigger save flush
	bot.flushSave(false)

	// Check /api/save was called
	if atomic.LoadInt32(&saveAttempts) != 1 {
		t.Fatalf("expected 1 save attempt, got %d", atomic.LoadInt32(&saveAttempts))
	}

	// Check /api/session was called by the resync handler
	if atomic.LoadInt32(&sessionCalls) != 1 {
		t.Fatalf("expected 1 /api/session call, got %d", atomic.LoadInt32(&sessionCalls))
	}

	// Check that saveDirty was cleared (no retry spam!)
	bot.mu.Lock()
	dirty := bot.saveDirty
	loadedData := bot.saveFiles["chars/test_0.json"]
	bot.mu.Unlock()

	if dirty {
		t.Fatalf("expected saveDirty to be false after resync to prevent retry-spam")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(loadedData), &parsed); err != nil || parsed["reloaded"] != true {
		t.Fatalf("expected saveFiles to be updated with fresh server files, got %s", loadedData)
	}
}

// TestSaveResyncCooldownAndMaxAttempts verifies that resync obeys 30s cooldown,
// limits to 3 attempts before halting auto-save, and preserves equip and inventory.
func TestSaveResyncCooldownAndMaxAttempts(t *testing.T) {
	var sessionCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/save" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Save refused","resync":1}`))
			return
		}
		if r.URL.Path == "/api/session" {
			atomic.AddInt32(&sessionCalls, 1)
			w.WriteHeader(http.StatusOK)
			charPayload := map[string]interface{}{
				"name": "PreservedHero",
				"equip": map[string]interface{}{
					"suit":   "II_ARM_F_VAG_SUIT01",
					"weapon": "II_WEA_SWO_WOODEN",
				},
				"inv": []interface{}{
					map[string]interface{}{"id": "II_WEA_SWO_WOODEN", "n": 1},
					map[string]interface{}{"id": "II_GEN_FOO_INS_LOLLIPOP", "n": 50},
				},
			}
			charBytes, _ := json.Marshal(charPayload)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"token":   "tok_preserved",
				"account": map[string]interface{}{"key": "test"},
				"files": map[string]string{
					"chars/test_0.json": string(charBytes),
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	customCli := client.NewGameHttpClient()
	customCli.BaseURL = server.URL

	bot := NewBotInstance(models.BotProfile{
		ID:            "bot_cooldown",
		CharacterName: "PreservedHero",
	}, nil, nil, nil, nil)

	bot.httpCli = customCli
	bot.Status = "online"
	bot.sessionToken = "tok_test"
	bot.charFileName = "chars/test_0.json"
	bot.saveFiles["chars/test_0.json"] = `{"name":"CorruptedState"}`

	// Attempt 1: Should trigger session resume
	bot.saveDirty = true
	bot.flushSave(false)
	if atomic.LoadInt32(&sessionCalls) != 1 {
		t.Fatalf("expected 1 session call on attempt 1, got %d", atomic.LoadInt32(&sessionCalls))
	}

	// Verify equip and inventory are intact after resume
	bot.mu.Lock()
	savedData := bot.saveFiles["chars/test_0.json"]
	resyncCount := bot.resyncCount
	bot.mu.Unlock()

	if resyncCount != 1 {
		t.Fatalf("expected resyncCount to be 1, got %d", resyncCount)
	}

	var restored map[string]interface{}
	_ = json.Unmarshal([]byte(savedData), &restored)
	equipMap, _ := restored["equip"].(map[string]interface{})
	if equipMap["suit"] != "II_ARM_F_VAG_SUIT01" || equipMap["weapon"] != "II_WEA_SWO_WOODEN" {
		t.Fatalf("expected equip to be preserved after resume, got %v", equipMap)
	}
	invList, _ := restored["inv"].([]interface{})
	if len(invList) < 2 {
		t.Fatalf("expected inventory items to be preserved after resume, got %v", invList)
	}

	// Attempt 2 immediately: Should be blocked by 30s cooldown
	bot.saveFiles["chars/test_0.json"] = `{"name":"CorruptedState2"}`
	bot.saveDirty = true
	bot.flushSave(false)
	if atomic.LoadInt32(&sessionCalls) != 1 {
		t.Fatalf("expected session call to be skipped due to cooldown, got %d", atomic.LoadInt32(&sessionCalls))
	}

	// Fast-forward cooldown by modifying lastResyncTime
	bot.mu.Lock()
	bot.lastResyncTime = time.Now().Add(-35 * time.Second)
	bot.saveFiles["chars/test_0.json"] = `{"name":"CorruptedState2"}`
	bot.saveDirty = true
	bot.mu.Unlock()

	// Attempt 2 (valid after cooldown):
	bot.flushSave(false)
	if atomic.LoadInt32(&sessionCalls) != 2 {
		t.Fatalf("expected 2 session calls after cooldown expiry, got %d", atomic.LoadInt32(&sessionCalls))
	}

	// Fast-forward cooldown for attempt 3:
	bot.mu.Lock()
	bot.lastResyncTime = time.Now().Add(-35 * time.Second)
	bot.saveFiles["chars/test_0.json"] = `{"name":"CorruptedState3"}`
	bot.saveDirty = true
	bot.mu.Unlock()

	bot.flushSave(false)
	if atomic.LoadInt32(&sessionCalls) != 3 {
		t.Fatalf("expected 3 session calls, got %d", atomic.LoadInt32(&sessionCalls))
	}

	// Fast-forward cooldown for attempt 4: Should hit max attempts (3) and halt saving
	bot.mu.Lock()
	bot.lastResyncTime = time.Now().Add(-35 * time.Second)
	bot.saveFiles["chars/test_0.json"] = `{"name":"CorruptedState4"}`
	bot.saveDirty = true
	bot.mu.Unlock()

	bot.flushSave(false)
	// sessionCalls should STILL be 3 (no fourth call made)
	if atomic.LoadInt32(&sessionCalls) != 3 {
		t.Fatalf("expected sessionCalls to remain 3 after exceeding max attempts, got %d", atomic.LoadInt32(&sessionCalls))
	}

	bot.mu.Lock()
	stopped := bot.saveStopped
	status := bot.Status
	bot.mu.Unlock()

	if !stopped {
		t.Fatalf("expected saveStopped to be true after max resync attempts")
	}
	if status != "error" {
		t.Fatalf("expected bot status to be 'error', got %s", status)
	}
}

// TestSaveFailureAlarmAfterThreeErrors verifies that 3 consecutive save failures
// trigger an alarm and expose a warning in bot status.
func TestSaveFailureAlarmAfterThreeErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Server disk failure"}`))
	}))
	defer server.Close()

	customCli := client.NewGameHttpClient()
	customCli.BaseURL = server.URL

	bot := NewBotInstance(models.BotProfile{
		ID:            "bot_alarm",
		CharacterName: "AlarmHero",
	}, nil, nil, nil, nil)

	bot.httpCli = customCli
	bot.Status = "online"
	bot.sessionToken = "tok_alarm"
	bot.charFileName = "chars/test_0.json"
	bot.saveFiles["chars/test_0.json"] = `{"name":"AlarmHero"}`

	// Fail 1
	bot.saveDirty = true
	bot.flushSave(false)
	bot.mu.Lock()
	f1 := bot.saveFailCount
	bot.mu.Unlock()
	if f1 != 1 {
		t.Fatalf("expected fail count 1, got %d", f1)
	}

	// Fail 2
	bot.saveDirty = true
	bot.flushSave(false)
	bot.mu.Lock()
	f2 := bot.saveFailCount
	bot.mu.Unlock()
	if f2 != 2 {
		t.Fatalf("expected fail count 2, got %d", f2)
	}

	// Fail 3 -> triggers alarm
	bot.saveDirty = true
	bot.flushSave(false)
	bot.mu.Lock()
	f3 := bot.saveFailCount
	statusMsg := bot.StatusMessage
	bot.mu.Unlock()

	if f3 != 3 {
		t.Fatalf("expected fail count 3, got %d", f3)
	}
	if statusMsg != "Save warning: 3 consecutive failures" {
		t.Fatalf("expected status message warning, got '%s'", statusMsg)
	}
}

// TestValidatorFullInventoryDropLogging verifies that when inventory is at max capacity (240 slots)
// with no empty slots, ValidateAndRepairInventory clamps excess count without corrupting or panicking.
func TestValidatorFullInventoryDropLogging(t *testing.T) {
	invList := make([]interface{}, 240)
	for i := 0; i < 240; i++ {
		invList[i] = map[string]interface{}{
			"id": "II_WEA_SWO_WOODEN",
			"n":  1,
		}
	}
	// Make slot 0 corrupted with non-stackable x2
	invList[0] = map[string]interface{}{
		"id": "II_ARM_F_VAG_SUIT02",
		"n":  2,
	}

	repaired := models.ValidateAndRepairInventory(invList)
	if !repaired {
		t.Fatalf("expected repair to return true")
	}

	s0 := invList[0].(map[string]interface{})
	if s0["n"] != 1 {
		t.Fatalf("expected slot 0 to be clamped to 1, got %v", s0["n"])
	}
	if len(invList) != 240 {
		t.Fatalf("expected inventory length to remain 240, got %d", len(invList))
	}
}
