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

// TestSaveRefusedSurgicalRollbackPreservesWebSocketState verifies that when /api/save returns
// "Save refused", only the offending inventory mutation is rolled back, the offending item is blacklisted,
// and real-time WebSocket state (pos, exp, level, hp) and new loot are preserved without killing the bot.
func TestSaveRefusedSurgicalRollbackPreservesWebSocketState(t *testing.T) {
	var saveCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/save" {
			count := atomic.AddInt32(&saveCount, 1)
			if count == 1 {
				// Initial save OK
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"success":true}`))
				return
			}
			// Second save triggers Save refused
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Save refused: illegal penya delta","resync":1}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	customCli := client.NewGameHttpClient()
	customCli.BaseURL = server.URL

	bot := NewBotInstance(models.BotProfile{
		ID:            "bot_rollback_test",
		CharacterName: "RollbackHero",
	}, nil, nil, nil, nil)

	bot.httpCli = customCli
	bot.Status = "online"
	bot.sessionToken = "tok_test"
	bot.charFileName = "chars/test_0.json"

	// Initial clean character save
	initChar := map[string]interface{}{
		"name":  "RollbackHero",
		"level": 60,
		"exp":   1234567,
		"hp":    5000,
		"pos":   []interface{}{7400.0, 85.0, -3900.0},
		"penya": 10000.0,
		"inv": []interface{}{
			map[string]interface{}{"id": "II_ARM_M_VAG_SUIT01", "n": 1},
			map[string]interface{}{"id": "II_SYS_SYS_SCR_SPROTECT", "n": 5, "bd": true},
		},
	}
	rawBytes, _ := json.Marshal(initChar)
	bot.saveFiles[bot.charFileName] = string(rawBytes)
	bot.saveDirty = true

	// Step 1: Initial save flush succeeds
	bot.flushSave(false)
	if atomic.LoadInt32(&saveCount) != 1 {
		t.Fatalf("expected 1 initial save, got %d", atomic.LoadInt32(&saveCount))
	}

	// Step 2: Bot sells item in slot 0 (Vagrant Suit)
	// This captures snapshotPreMutation
	_, err := bot.SellShopItem(0, 1, 25)
	if err != nil {
		t.Fatalf("SellShopItem failed: %v", err)
	}

	// Simulate real-time WebSocket state changes (level up, exp gain, movement)
	bot.mu.Lock()
	var currentData map[string]interface{}
	_ = json.Unmarshal([]byte(bot.saveFiles[bot.charFileName]), &currentData)
	currentData["level"] = 61
	currentData["exp"] = 1500000
	currentData["hp"] = 5200
	currentData["pos"] = []interface{}{7450.0, 85.0, -3850.0}
	updBytes, _ := json.Marshal(currentData)
	bot.saveFiles[bot.charFileName] = string(updBytes)
	bot.saveDirty = true
	bot.mu.Unlock()

	// Step 3: flushSave triggers "Save refused" from server
	bot.flushSave(false)
	if atomic.LoadInt32(&saveCount) != 2 {
		t.Fatalf("expected 2nd save call, got %d", atomic.LoadInt32(&saveCount))
	}

	// Step 4: Verify surgical rollback results
	bot.mu.Lock()
	status := bot.Status
	saveStopped := bot.saveStopped
	finalDataStr := bot.saveFiles[bot.charFileName]
	blacklisted := bot.sessionBlacklist["II_ARM_M_VAG_SUIT01"]
	bot.mu.Unlock()

	// Bot must NOT be in error state
	if status == "error" || saveStopped {
		t.Fatalf("bot died or halted saving! status=%s, stopped=%v", status, saveStopped)
	}

	// Offending item must be blacklisted for this session
	if !blacklisted {
		t.Fatalf("expected II_ARM_M_VAG_SUIT01 to be blacklisted after save refusal")
	}

	var finalData map[string]interface{}
	_ = json.Unmarshal([]byte(finalDataStr), &finalData)

	// Real-time WebSocket combat/movement fields must be preserved!
	if lvl, _ := finalData["level"].(float64); int(lvl) != 61 {
		t.Fatalf("expected level 61 preserved, got %v", finalData["level"])
	}
	if exp, _ := finalData["exp"].(float64); int(exp) != 1500000 {
		t.Fatalf("expected exp 1500000 preserved, got %v", finalData["exp"])
	}

	// Penya must be rolled back to pre-mutation (10000)
	if penya, _ := finalData["penya"].(float64); penya != 10000.0 {
		t.Fatalf("expected penya rolled back to 10000, got %v", penya)
	}

	// Inventory slot 0 must be restored to II_ARM_M_VAG_SUIT01
	invList, _ := finalData["inv"].([]interface{})
	if len(invList) < 1 {
		t.Fatalf("expected inventory items")
	}
	s0, _ := invList[0].(map[string]interface{})
	if s0["id"] != "II_ARM_M_VAG_SUIT01" {
		t.Fatalf("expected slot 0 restored to II_ARM_M_VAG_SUIT01, got %v", s0)
	}

	// Subsequent attempt to sell blacklisted item must be rejected immediately
	_, errBlacklist := bot.SellShopItem(0, 1, 25)
	if errBlacklist == nil {
		t.Fatalf("expected sell of blacklisted item to fail")
	}
}

// TestStrictMutexSerializationSaveAndClaim verifies that cli.Claim waits for flushSave
// to complete and cannot run concurrently with an in-flight save.
func TestStrictMutexSerializationSaveAndClaim(t *testing.T) {
	var inFlightSaves int32
	var inFlightClaims int32
	var maxConcurrentOps int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/save" {
			s := atomic.AddInt32(&inFlightSaves, 1)
			c := atomic.LoadInt32(&inFlightClaims)
			if s+c > maxConcurrentOps {
				atomic.StoreInt32(&maxConcurrentOps, s+c)
			}
			time.Sleep(100 * time.Millisecond)
			atomic.AddInt32(&inFlightSaves, -1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		if r.URL.Path == "/api/claim" {
			c := atomic.AddInt32(&inFlightClaims, 1)
			s := atomic.LoadInt32(&inFlightSaves)
			if s+c > maxConcurrentOps {
				atomic.StoreInt32(&maxConcurrentOps, s+c)
			}
			time.Sleep(100 * time.Millisecond)
			atomic.AddInt32(&inFlightClaims, -1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"day":1,"items":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	customCli := client.NewGameHttpClient()
	customCli.BaseURL = server.URL

	bot := NewBotInstance(models.BotProfile{
		ID:            "bot_mutex_test",
		CharacterName: "MutexHero",
	}, nil, nil, nil, nil)

	bot.httpCli = customCli
	bot.Status = "online"
	bot.sessionToken = "tok_test"
	bot.charFileName = "chars/test_0.json"
	bot.saveFiles[bot.charFileName] = `{"name":"MutexHero","inv":[]}`
	bot.saveDirty = true

	// Concurrently trigger flushSave and executeClaimTransaction
	done := make(chan bool, 2)
	go func() {
		bot.flushSave(false)
		done <- true
	}()

	time.Sleep(10 * time.Millisecond) // Let save acquire mutex first

	go func() {
		_, _ = bot.executeClaimTransaction("tok_test", map[string]interface{}{"k": "daily", "char": 0})
		done <- true
	}()

	<-done
	<-done

	if maxConcurrentOps > 1 {
		t.Fatalf("expected max concurrent save+claim operations to be 1, got %d (race condition!)", maxConcurrentOps)
	}
}
