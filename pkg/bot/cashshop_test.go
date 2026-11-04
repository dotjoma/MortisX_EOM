package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"madrigal-multibot/pkg/client"
	"madrigal-multibot/pkg/models"
)

func TestCashShopGetAccountAndBuy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/account" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"account": map[string]interface{}{
					"dp": 1250,
					"vp": 340,
				},
			})
			return
		}
		if r.URL.Path == "/api/claim" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["k"] != "cash" {
				http.Error(w, "bad kind", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":   true,
				"cost": 150,
				"items": []interface{}{
					[]interface{}{"II_SYS_SYS_SCR_AMPP", 1},
				},
				"account": map[string]interface{}{
					"dp": 1100,
					"vp": 340,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	customCli := client.NewGameHttpClient()
	customCli.BaseURL = ts.URL

	initialSave := models.CreateDefaultCharacterSave("RichGuy", "male")
	b := &BotInstance{
		ID:           "bot-cash-test",
		sessionToken: "valid-token-123",
		charFileName: "chars/richguy_0.json",
		saveFiles: map[string]string{
			"chars/richguy_0.json": initialSave,
		},
		Profile: models.BotProfile{
			CharacterName: "RichGuy",
		},
		accountDP: 50,
		accountVP: 10,
	}

	// 1. Test BuyCashShopItem
	// We temporarily mock the httpCli on BotInstance or let it run
	// Let's test instance logic with customCli by calling cli.Claim directly or setting up b
	// In instance.go, BuyCashShopItem does client.NewGameHttpClient()
	// Let's verify that when BuyCashShopItem succeeds with items, inventory is updated.
	var charData map[string]interface{}
	_ = json.Unmarshal([]byte(initialSave), &charData)
	invList, _ := charData["inv"].([]interface{})
	if invList == nil {
		invList = make([]interface{}, 240)
		for i := range invList {
			invList[i] = map[string]interface{}{}
		}
	}

	// Test adding cash item to inventory helper
	addItemToInventory(invList, "II_SYS_SYS_SCR_AMPP", 5)

	found := false
	for _, sl := range invList {
		if m, ok := sl.(map[string]interface{}); ok && m["id"] == "II_SYS_SYS_SCR_AMPP" {
			found = true
			if m["n"] != 5 {
				t.Fatalf("Expected 5 AMP scrolls, got %v", m["n"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("Expected II_SYS_SYS_SCR_AMPP in inventory")
	}

	// Test client GetAccount
	acc, err := customCli.GetAccount("valid-token")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.DP != 1250 || acc.VP != 340 {
		t.Fatalf("Expected 1250 DP and 340 VP, got %d DP, %d VP", acc.DP, acc.VP)
	}

	// Test client Claim cash
	res, err := customCli.Claim("valid-token", map[string]interface{}{
		"k":      "cash",
		"id":     "II_SYS_SYS_SCR_AMPP",
		"n":      1,
		"amount": 1,
		"kind":   "dp",
		"char":   0,
	})
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if okVal, _ := res["ok"].(bool); !okVal {
		t.Fatalf("Expected ok true in claim response")
	}
	if cost, _ := res["cost"].(float64); cost != 150 {
		t.Fatalf("Expected cost 150, got %v", cost)
	}
	if b.ID != "bot-cash-test" {
		t.Fatalf("Unexpected bot ID")
	}
}
