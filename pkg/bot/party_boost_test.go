package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"madrigal-multibot/pkg/client"
	"madrigal-multibot/pkg/models"
)

func TestPartyBoostManager_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "party_boost.json")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, configPath)

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LordTank"
	cfg.SelectedMembers = []string{"Support1", "Support2", "DPS1"}
	cfg.LeashDistance = 55.0
	cfg.SkillsEnabled[models.PartySkillFortuneCircle] = true
	cfg.ScrollsEnabled[models.PartyScrollSelectedParty] = true

	if err := mgr.UpdateConfig(cfg); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Verify fresh instance loads identical state from disk
	mgr2 := NewPartyBoostManager(bm, configPath)
	loaded := mgr2.GetConfig()

	if !loaded.Enabled {
		t.Errorf("expected Enabled to be true")
	}
	if loaded.LeaderName != "LordTank" {
		t.Errorf("expected leader 'LordTank', got '%s'", loaded.LeaderName)
	}
	if len(loaded.SelectedMembers) != 3 {
		t.Fatalf("expected 3 members, got %d", len(loaded.SelectedMembers))
	}
	if loaded.LeashDistance != 55.0 {
		t.Errorf("expected leash 55.0, got %f", loaded.LeashDistance)
	}
	if !loaded.SkillsEnabled[models.PartySkillFortuneCircle] {
		t.Errorf("expected Fortune Circle to be enabled")
	}
	if !loaded.ScrollsEnabled[models.PartyScrollSelectedParty] {
		t.Errorf("expected Selected Party scroll to be enabled")
	}
}

func TestPartyBoostManager_DiscoverRoster(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "party_boost.json")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, configPath)

	// Create 4 mock bots
	p1 := models.BotProfile{
		ID:            "bot_1",
		Name:          "acc1",
		CharacterName: "LeaderBot",
		Server:        "Channel 1",
	}
	s1 := models.DefaultStats()
	s1.Level = 60
	s1.Job = "Knight"
	s1.PosX = 100
	s1.PosZ = 100
	_, _ = bm.CreateBot(p1, &s1)
	inst1 := bm.FindInstanceByName("acc1")
	inst1.Status = "online"
	inst1.SetPartyState(true, 1001, "TopParty", "LeaderBot")

	p2 := models.BotProfile{
		ID:            "bot_2",
		Name:          "acc2",
		CharacterName: "NearMember",
		Server:        "Channel 1",
	}
	s2 := models.DefaultStats()
	s2.Level = 55
	s2.Job = "Ringmaster"
	s2.PosX = 120 // Distance dx = 20, dz = 20 -> dist ~ 28.28m
	s2.PosZ = 120
	_, _ = bm.CreateBot(p2, &s2)
	inst2 := bm.FindInstanceByName("acc2")
	inst2.Status = "online"
	inst2.SetPartyState(true, 1001, "TopParty", "LeaderBot")

	p3 := models.BotProfile{
		ID:            "bot_3",
		Name:          "acc3",
		CharacterName: "FarMember",
		Server:        "Channel 1",
	}
	s3 := models.DefaultStats()
	s3.Level = 15
	s3.Job = "Vagrant"
	s3.PosX = 300 // Distance dx = 200, dz = 200 -> dist ~ 282.8m (> 64m)
	s3.PosZ = 300
	_, _ = bm.CreateBot(p3, &s3)
	inst3 := bm.FindInstanceByName("acc3")
	inst3.Status = "online"

	p4 := models.BotProfile{
		ID:            "bot_4",
		Name:          "acc4",
		CharacterName: "OfflineMember",
		Server:        "Channel 1",
	}
	s4 := models.DefaultStats()
	_, _ = bm.CreateBot(p4, &s4)
	inst4 := bm.FindInstanceByName("acc4")
	inst4.Status = "disconnected"

	// Configure LeaderName
	cfg := mgr.GetConfig()
	cfg.LeaderName = "LeaderBot"
	_ = mgr.UpdateConfig(cfg)

	roster := mgr.DiscoverRoster()
	if len(roster) != 4 {
		t.Fatalf("expected 4 roster entries, got %d", len(roster))
	}

	// First entry must be the designated leader
	leader := roster[0]
	if !leader.IsLeader || leader.CharacterName != "LeaderBot" {
		t.Errorf("expected first entry to be LeaderBot, got %+v", leader)
	}
	if leader.DistanceToLeader != 0.0 || !leader.InShareRange {
		t.Errorf("leader should have 0 distance and InShareRange true")
	}
	if !leader.InParty || leader.PartyID != 1001 || leader.PartyLeader != "LeaderBot" {
		t.Errorf("unexpected leader party state: %+v", leader)
	}

	// Find NearMember
	var near *models.PartyRosterEntry
	var far *models.PartyRosterEntry
	var offline *models.PartyRosterEntry
	for i := range roster {
		switch roster[i].CharacterName {
		case "NearMember":
			near = &roster[i]
		case "FarMember":
			far = &roster[i]
		case "OfflineMember":
			offline = &roster[i]
		}
	}

	if near == nil || !near.InShareRange || near.DistanceToLeader < 28.0 || near.DistanceToLeader > 29.0 {
		t.Errorf("unexpected NearMember: %+v", near)
	}
	if !near.InParty || near.PartyID != 1001 {
		t.Errorf("NearMember should be in party: %+v", near)
	}

	if far == nil || far.InShareRange || far.DistanceToLeader < 280.0 {
		t.Errorf("FarMember should be out of 64m share range: %+v", far)
	}
	if far.InParty {
		t.Errorf("FarMember should not be in party: %+v", far)
	}

	if offline == nil || offline.Status != "disconnected" || offline.DistanceToLeader != 0.0 {
		t.Errorf("unexpected offline entry: %+v", offline)
	}
}

func TestPartyBoost_LeaderAutoInvite_And_Throttling(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sentPackets := make(chan map[string]interface{}, 20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var pkt map[string]interface{}
			if err := json.Unmarshal(msg, &pkt); err == nil {
				sentPackets <- pkt
			}
		}
	}))
	defer server.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	// Create leader bot
	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	leaderInst.ws = client.NewGameWebSocket()
	if err := leaderInst.ws.Connect(wsEndpoint); err != nil {
		t.Fatalf("failed to connect leader ws: %v", err)
	}
	defer leaderInst.ws.Close()

	// Create member bot
	pMember := models.BotProfile{ID: "bot_member", Name: "acc_member", CharacterName: "MemberBot"}
	sMember := models.DefaultStats()
	_, _ = bm.CreateBot(pMember, &sMember)
	memberInst := bm.FindInstanceByName("acc_member")
	memberInst.Status = "online"

	// Configure PartyBoost
	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SelectedMembers = []string{"MemberBot"}
	cfg.AutoReinvite = true
	cfg.PartyName = "TestParty"
	cfg.AutoAdvance = false
	cfg.ItemDistMode = 0
	_ = mgr.UpdateConfig(cfg)

	now := time.Now()
	// Tick 1: should send invite
	mgr.watchdogTick(now)

	select {
	case pkt := <-sentPackets:
		if pkt["t"] != "pinvite" || pkt["name"] != "MemberBot" {
			t.Fatalf("unexpected packet: %+v", pkt)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for leader invite packet")
	}

	// Tick 2 (after 2s, within 10s cooldown): should NOT send duplicate invite
	mgr.watchdogTick(now.Add(2 * time.Second))
	select {
	case pkt := <-sentPackets:
		t.Fatalf("unexpected packet sent during cooldown: %+v", pkt)
	case <-time.After(100 * time.Millisecond):
		// Expected: no packet sent
	}

	// Tick 3 (after 11s, cooldown elapsed): should send invite again
	mgr.watchdogTick(now.Add(11 * time.Second))
	select {
	case pkt := <-sentPackets:
		if pkt["t"] != "pinvite" || pkt["name"] != "MemberBot" {
			t.Fatalf("unexpected packet: %+v", pkt)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for leader reinvite packet after cooldown")
	}

	// Now simulate member joined party (party.m update)
	leaderInst.handleIncomingPacket([]byte(`{"t":"party","id":2001,"leader":101,"name":"TestParty","dist":0,"kind":0,"share":0,"m":[[101,"LeaderBot",60,1,100,0,0,1],[102,"MemberBot",50,2,100,0,0,1]]}`))

	// Tick 4: member is in party, should not invite
	mgr.watchdogTick(now.Add(25 * time.Second))
	select {
	case pkt := <-sentPackets:
		t.Fatalf("unexpected packet sent when member already in party: %+v", pkt)
	case <-time.After(100 * time.Millisecond):
		// Expected: no packet sent
	}
}

func TestPartyBoost_MemberAutoAccept_And_FriendshipSummon(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sentPackets := make(chan map[string]interface{}, 20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var pkt map[string]interface{}
			if err := json.Unmarshal(msg, &pkt); err == nil {
				sentPackets <- pkt
			}
		}
	}))
	defer server.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pMember := models.BotProfile{ID: "bot_member", Name: "acc_member", CharacterName: "MemberBot"}
	sMember := models.DefaultStats()
	_, _ = bm.CreateBot(pMember, &sMember)
	memberInst := bm.FindInstanceByName("acc_member")
	memberInst.Status = "online"
	memberInst.ws = client.NewGameWebSocket()
	if err := memberInst.ws.Connect(wsEndpoint); err != nil {
		t.Fatalf("failed to connect member ws: %v", err)
	}
	defer memberInst.ws.Close()

	// Configure PartyBoost with LeaderName = "LeaderBot"
	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SelectedMembers = []string{"MemberBot"}
	_ = mgr.UpdateConfig(cfg)

	// 1. Incoming pinvite from designated leader
	memberInst.handleIncomingPacket([]byte(`{"t":"pinvite","from":501,"name":"LeaderBot"}`))

	select {
	case pkt := <-sentPackets:
		if pkt["t"] != "paccept" || int(pkt["from"].(float64)) != 501 {
			t.Fatalf("expected paccept for 501, got: %+v", pkt)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for paccept packet")
	}

	// 2. Incoming pinvite from a stranger -> should NOT accept
	memberInst.handleIncomingPacket([]byte(`{"t":"pinvite","from":999,"name":"RandomStranger"}`))
	select {
	case pkt := <-sentPackets:
		t.Fatalf("unexpected packet sent for stranger invite: %+v", pkt)
	case <-time.After(100 * time.Millisecond):
		// Expected: ignored
	}

	// 3. Incoming friendship summon (fsumq) from leader -> should auto-accept fsumok
	memberInst.handleIncomingPacket([]byte(`{"t":"fsumq","from":501,"name":"LeaderBot"}`))
	select {
	case pkt := <-sentPackets:
		if pkt["t"] != "fsumok" || int(pkt["from"].(float64)) != 501 {
			t.Fatalf("expected fsumok for 501, got: %+v", pkt)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for fsumok packet")
	}
}

func TestPartyBoost_DistanceLeash(t *testing.T) {
	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	sLeader.PosX = 100
	sLeader.PosZ = 100
	sLeader.Zone = "Flarine"
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"

	pMember := models.BotProfile{ID: "bot_member", Name: "acc_member", CharacterName: "MemberBot"}
	sMember := models.DefaultStats()
	sMember.PosX = 100
	sMember.PosZ = 170 // dx = 0, dz = 70m -> dist = 70m (> 60m leash limit)
	sMember.Zone = "Flarine"
	_, _ = bm.CreateBot(pMember, &sMember)
	memberInst := bm.FindInstanceByName("acc_member")
	memberInst.Status = "online"

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SelectedMembers = []string{"MemberBot"}
	cfg.LeashEnabled = true
	cfg.LeashDistance = 60.0
	_ = mgr.UpdateConfig(cfg)

	// Step 1: Trigger watchdog tick when member is 70m away
	mgr.watchdogTick(time.Now())

	if !memberInst.IsPartyLeashed() {
		t.Fatalf("expected member to be leashed at 70m distance")
	}
	wpX, wpZ, hasWp := memberInst.GetWaypoint()
	if !hasWp || wpX != 100 || wpZ != 100 {
		t.Fatalf("expected waypoint set to leader (100, 100), got hasWp=%v, (%.1f, %.1f)", hasWp, wpX, wpZ)
	}

	// Step 2: Member moves closer to leader (15m away)
	memberInst.mu.Lock()
	memberInst.Stats.PosX = 100
	memberInst.Stats.PosZ = 115 // dist = 15m (<= 20m release threshold)
	memberInst.mu.Unlock()

	mgr.watchdogTick(time.Now())

	if memberInst.IsPartyLeashed() {
		t.Fatalf("expected leash to be released when within 15m")
	}
	_, _, hasWp = memberInst.GetWaypoint()
	if hasWp {
		t.Fatalf("expected waypoint to be cleared when leash is released")
	}
}

func TestPartyBoost_PartyMechanics_AutoRename(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sentPackets := make(chan map[string]interface{}, 20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var pkt map[string]interface{}
			if err := json.Unmarshal(msg, &pkt); err == nil {
				sentPackets <- pkt
			}
		}
	}))
	defer server.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	leaderInst.ws = client.NewGameWebSocket()
	_ = leaderInst.ws.Connect(wsEndpoint)
	defer leaderInst.ws.Close()

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.PartyName = "Epic Grinders"
	cfg.AutoAdvance = false
	_ = mgr.UpdateConfig(cfg)

	// Simulate incoming party state with default party name
	leaderInst.handleIncomingPacket([]byte(`{"t":"party","id":3001,"leader":1,"name":"Default Party","lv":5,"kind":0,"share":0,"dist":1,"m":[[1,"LeaderBot",60,1,100,0,0,1]]}`))

	select {
	case pkt := <-sentPackets:
		if pkt["t"] != "pset" || pkt["k"] != "name" || pkt["v"] != "Epic Grinders" {
			t.Fatalf("expected rename pset to 'Epic Grinders', got: %+v", pkt)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for auto-rename pset packet")
	}
}

func TestPartyBoost_PartyMechanics_AutoAdvance(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sentPackets := make(chan map[string]interface{}, 20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var pkt map[string]interface{}
			if err := json.Unmarshal(msg, &pkt); err == nil {
				sentPackets <- pkt
			}
		}
	}))
	defer server.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	leaderInst.ws = client.NewGameWebSocket()
	_ = leaderInst.ws.Connect(wsEndpoint)
	defer leaderInst.ws.Close()

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.PartyName = "Mortis Party"
	cfg.AutoAdvance = true
	cfg.ItemDistMode = 1
	_ = mgr.UpdateConfig(cfg)

	// Party is Lv 10 but kind is 0 (Normal Party) -> Should auto-advance
	leaderInst.handleIncomingPacket([]byte(`{"t":"party","id":3001,"leader":1,"name":"Mortis Party","lv":10,"kind":0,"share":0,"dist":1,"m":[[1,"LeaderBot",60,1,100,0,0,1]]}`))

	select {
	case pkt := <-sentPackets:
		if pkt["t"] != "pset" || pkt["k"] != "kind" || int(pkt["v"].(float64)) != 1 {
			t.Fatalf("expected pset kind=1, got: %+v", pkt)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for auto-advance pset packet")
	}
}

func TestPartyBoost_PartyMechanics_ExpShareAndDist(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sentPackets := make(chan map[string]interface{}, 20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var pkt map[string]interface{}
			if err := json.Unmarshal(msg, &pkt); err == nil {
				sentPackets <- pkt
			}
		}
	}))
	defer server.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	leaderInst.ws = client.NewGameWebSocket()
	_ = leaderInst.ws.Connect(wsEndpoint)
	defer leaderInst.ws.Close()

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.PartyName = "Mortis Party"
	cfg.AutoAdvance = false
	cfg.ExpShareMode = 1 // Contribution
	cfg.ItemDistMode = 3 // Randomly
	_ = mgr.UpdateConfig(cfg)

	// Step 1: Party is kind=0 (Normal) -> Contribution share should NOT be sent yet
	leaderInst.handleIncomingPacket([]byte(`{"t":"party","id":3001,"leader":1,"name":"Mortis Party","lv":9,"kind":0,"share":0,"dist":3,"m":[[1,"LeaderBot",60,1,100,0,0,1]]}`))

	select {
	case pkt := <-sentPackets:
		t.Fatalf("unexpected packet sent when party is not yet advanced: %+v", pkt)
	case <-time.After(100 * time.Millisecond):
		// Expected: wait until kind == 1
	}

	// Step 2: Party upgraded to kind=1 (Advanced) with dist=1 -> Should send share=1 and dist=3
	leaderInst.handleIncomingPacket([]byte(`{"t":"party","id":3001,"leader":1,"name":"Mortis Party","lv":10,"kind":1,"share":0,"dist":1,"m":[[1,"LeaderBot",60,1,100,0,0,1]]}`))

	var gotShare, gotDist bool
	for i := 0; i < 2; i++ {
		select {
		case pkt := <-sentPackets:
			if pkt["t"] == "pset" && pkt["k"] == "share" && int(pkt["v"].(float64)) == 1 {
				gotShare = true
			} else if pkt["t"] == "pset" && pkt["k"] == "dist" && int(pkt["v"].(float64)) == 3 {
				gotDist = true
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for pset packets (gotShare=%v, gotDist=%v)", gotShare, gotDist)
		}
	}

	if !gotShare || !gotDist {
		t.Fatalf("expected both share=1 and dist=3, gotShare=%v, gotDist=%v", gotShare, gotDist)
	}
}

func TestPartyBoost_LevelSync(t *testing.T) {
	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	// Create High Level Bot (Lv 60)
	pHigh := models.BotProfile{ID: "bot_high", Name: "acc_high", CharacterName: "HighBot"}
	sHigh := models.DefaultStats()
	sHigh.Level = 60
	sHigh.PosX = 100
	sHigh.PosZ = -100
	_, _ = bm.CreateBot(pHigh, &sHigh)
	highInst := bm.FindInstanceByName("acc_high")
	highInst.Status = "online"

	// Create Low Level Bot (Lv 20) -> difference 40 >= 15 levels
	pLow := models.BotProfile{ID: "bot_low", Name: "acc_low", CharacterName: "LowBot"}
	sLow := models.DefaultStats()
	sLow.Level = 20
	sLow.PosX = 120
	sLow.PosZ = -120 // dist ~28m <= 80m
	_, _ = bm.CreateBot(pLow, &sLow)
	lowInst := bm.FindInstanceByName("acc_low")
	lowInst.Status = "online"

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "HighBot"
	cfg.SelectedMembers = []string{"LowBot"}
	cfg.LevelSync = true
	_ = mgr.UpdateConfig(cfg)

	// Simulate party state with both members
	partyPacket := []byte(`{"t":"party","id":4001,"leader":1,"name":"Mortis Party","lv":10,"kind":1,"share":1,"dist":1,"m":[[1,"HighBot",60,1,100,100,-100,1],[2,"LowBot",20,1,100,120,-120,1]]}`)
	highInst.handleIncomingPacket(partyPacket)

	// Verify Level Sync is activated on high-level bot: target = 20 + 5 = 25
	if !highInst.IsLevelSyncActive() {
		t.Fatalf("expected level sync to be active on high-level bot")
	}
	syncLv, active := highInst.GetLevelSync()
	if !active || syncLv != 25 {
		t.Fatalf("expected sync level 25 (20+5), got active=%v, syncLv=%d", active, syncLv)
	}

	// Now simulate LowBot leveled up to Lv 50 (gap 60 - 50 = 10 < 15)
	partyPacketLv50 := []byte(`{"t":"party","id":4001,"leader":1,"name":"Mortis Party","lv":10,"kind":1,"share":1,"dist":1,"m":[[1,"HighBot",60,1,100,100,-100,1],[2,"LowBot",50,1,100,120,-120,1]]}`)
	highInst.handleIncomingPacket(partyPacketLv50)

	// Level gap < 15 -> Level sync should deactivate
	if highInst.IsLevelSyncActive() {
		t.Fatalf("expected level sync to deactivate when gap < 15")
	}
}

func TestPartyBoost_PartySkillsMaintainer_PriorityAndConditions(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sentPackets := make(chan map[string]interface{}, 20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var pkt map[string]interface{}
			if err := json.Unmarshal(msg, &pkt); err == nil {
				sentPackets <- pkt
			}
		}
	}))
	defer server.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	leaderInst.ws = client.NewGameWebSocket()
	_ = leaderInst.ws.Connect(wsEndpoint)
	defer leaderInst.ws.Close()

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SkillsEnabled = map[string]bool{
		models.PartySkillLinkAttack:    true,
		models.PartySkillFortuneCircle: true,
		models.PartySkillGiftBox:       false,
		models.PartySkillSphereCircle:  false,
		models.PartySkillStretching:    false,
	}
	_ = mgr.UpdateConfig(cfg)

	// Party Lv 25, Advanced (kind: 1), 10 points
	partyPacket := []byte(`{"t":"party","id":5001,"leader":1,"name":"Mortis Party","lv":25,"kind":1,"share":1,"dist":1,"pt":10,"m":[[1,"LeaderBot",60,1,100,0,0,1]]}`)
	leaderInst.handleIncomingPacket(partyPacket)

	now := time.Now()
	leaderInst.evaluatePartySkills(now, cfg)

	// Expect Priority 1: ST_LINKATTACK (3pt), then Priority 2: ST_FORTUNECIRCLE (5pt)
	castOrder := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		select {
		case pkt := <-sentPackets:
			if pkt["t"] == "pskill" {
				castOrder = append(castOrder, pkt["id"].(string))
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for pskill packet (got %d)", len(castOrder))
		}
	}

	if len(castOrder) != 2 || castOrder[0] != models.PartySkillLinkAttack || castOrder[1] != models.PartySkillFortuneCircle {
		t.Fatalf("expected [ST_LINKATTACK, ST_FORTUNECIRCLE], got %v", castOrder)
	}

	// Verify remaining points: 10 - 3 - 5 = 2 points
	_, _, pts, _, _, _, _, _ := leaderInst.GetPartyDetails()
	if pts != 2 {
		t.Fatalf("expected 2 party points remaining, got %d", pts)
	}

	// Verify skills are now active
	if leaderInst.GetPartySkillRemaining(models.PartySkillLinkAttack) <= 0 {
		t.Fatalf("expected ST_LINKATTACK to be active")
	}
	if leaderInst.GetPartySkillRemaining(models.PartySkillFortuneCircle) <= 0 {
		t.Fatalf("expected ST_FORTUNECIRCLE to be active")
	}

	// Run again immediately -> no skills should be cast (active & insufficient points for other skills)
	leaderInst.evaluatePartySkills(now, cfg)
	select {
	case pkt := <-sentPackets:
		t.Fatalf("unexpected pskill packet dispatched: %v", pkt)
	case <-time.After(100 * time.Millisecond):
		// Success
	}
}

func TestPartyBoost_PartySkillsMaintainer_EarlyRenewalWithSkillUp(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sentPackets := make(chan map[string]interface{}, 20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var pkt map[string]interface{}
			if err := json.Unmarshal(msg, &pkt); err == nil {
				sentPackets <- pkt
			}
		}
	}))
	defer server.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	leaderInst.ws = client.NewGameWebSocket()
	_ = leaderInst.ws.Connect(wsEndpoint)
	defer leaderInst.ws.Close()

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SkillsEnabled = map[string]bool{
		models.PartySkillLinkAttack: true,
	}
	_ = mgr.UpdateConfig(cfg)

	// Party Lv 20, Advanced, 10 points, but skillup is false, link attack has 3s left
	partyPacket := []byte(`{"t":"party","id":5002,"leader":1,"name":"Mortis Party","lv":20,"kind":1,"share":1,"dist":1,"pt":10,"skillup":0,"modes":{"ST_LINKATTACK":3.0},"m":[[1,"LeaderBot",60,1,100,0,0,1]]}`)
	leaderInst.handleIncomingPacket(partyPacket)

	now := time.Now()
	// Without skillup scroll, 3s remaining means still active -> cannot renew early
	leaderInst.evaluatePartySkills(now, cfg)
	select {
	case pkt := <-sentPackets:
		t.Fatalf("expected no renewal without skillup scroll, got %v", pkt)
	case <-time.After(100 * time.Millisecond):
		// Success
	}

	// Now activate skillup scroll (skillup: 1)
	partyPacketWithSkillUp := []byte(`{"t":"party","id":5002,"leader":1,"name":"Mortis Party","lv":20,"kind":1,"share":1,"dist":1,"pt":10,"skillup":1,"modes":{"ST_LINKATTACK":3.0},"m":[[1,"LeaderBot",60,1,100,0,0,1]]}`)
	leaderInst.handleIncomingPacket(partyPacketWithSkillUp)

	// With skillup scroll and rem < 5s -> early renewal triggers
	leaderInst.evaluatePartySkills(now, cfg)
	select {
	case pkt := <-sentPackets:
		if pkt["t"] != "pskill" || pkt["id"] != models.PartySkillLinkAttack {
			t.Fatalf("expected early renewal pskill for ST_LINKATTACK, got %v", pkt)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for early renewal pskill")
	}
}

func TestPartyBoost_PartyScrollsMaintainer_AllScrolls(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sentPackets := make(chan map[string]interface{}, 20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var pkt map[string]interface{}
			if err := json.Unmarshal(msg, &pkt); err == nil {
				sentPackets <- pkt
			}
		}
	}))
	defer server.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	leaderInst.ws = client.NewGameWebSocket()
	_ = leaderInst.ws.Connect(wsEndpoint)
	defer leaderInst.ws.Close()

	// Setup character save with 5 party scrolls in inventory
	charSave := map[string]interface{}{
		"name": "LeaderBot",
		"inv": []interface{}{
			map[string]interface{}{"id": "II_SYS_SYS_SCR_SELPAR", "n": 1},
			map[string]interface{}{"id": "II_CHR_SYS_SCR_PSKILLFULL1", "n": 1},
			map[string]interface{}{"id": "II_SYS_SYS_SCR_PARTYEXPUP01", "n": 1},
			map[string]interface{}{"id": "II_SYS_SYS_SCR_PARTYSKILLUP01", "n": 1},
			map[string]interface{}{"id": "II_SYS_SYS_SCR_SUPERLEADERPARTY", "n": 1},
		},
		"buffs": map[string]interface{}{},
	}
	saveBytes, _ := json.Marshal(charSave)
	leaderInst.charFileName = "chars/leader_0.json"
	leaderInst.saveFiles = map[string]string{
		"chars/leader_0.json": string(saveBytes),
	}

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.ScrollsEnabled = map[string]bool{
		models.PartyScrollSelectedParty: true,
		models.PartyScrollSkillFull:     true,
		models.PartyScrollExpUp:         true,
		models.PartyScrollSkillUp:       true,
		models.PartyScrollSuperLeader:   true,
	}
	_ = mgr.UpdateConfig(cfg)

	// Party Lv 5 (< 10), full: 0, expup: 0, skillup: 0
	partyPacket := []byte(`{"t":"party","id":6001,"leader":1,"name":"Mortis Party","lv":5,"kind":0,"share":0,"dist":1,"full":0,"expup":0,"skillup":0,"m":[[1,"LeaderBot",60,1,100,0,0,1]]}`)
	leaderInst.handleIncomingPacket(partyPacket)

	now := time.Now()
	leaderInst.evaluatePartyScrolls(now, cfg)

	receivedScrolls := make(map[string]int)
	for i := 0; i < 5; i++ {
		select {
		case pkt := <-sentPackets:
			if pkt["t"] == "pscroll" {
				idStr := pkt["id"].(string)
				secs := int(pkt["secs"].(float64))
				receivedScrolls[idStr] = secs
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for pscroll packet (got %d: %v)", len(receivedScrolls), receivedScrolls)
		}
	}

	// Verify SELPAR sent with secs: 0
	if _, ok := receivedScrolls["II_SYS_SYS_SCR_SELPAR"]; !ok {
		t.Fatalf("expected II_SYS_SYS_SCR_SELPAR scroll packet")
	}

	// Verify sustained scrolls sent with secs: 86400 (1 day)
	if secs := receivedScrolls["II_CHR_SYS_SCR_PSKILLFULL"]; secs != 86400 {
		t.Fatalf("expected II_CHR_SYS_SCR_PSKILLFULL secs 86400, got %d", secs)
	}
	if secs := receivedScrolls["II_SYS_SYS_SCR_PARTYEXPUP"]; secs != 86400 {
		t.Fatalf("expected II_SYS_SYS_SCR_PARTYEXPUP secs 86400, got %d", secs)
	}
	if secs := receivedScrolls["II_SYS_SYS_SCR_PARTYSKILLUP"]; secs != 86400 {
		t.Fatalf("expected II_SYS_SYS_SCR_PARTYSKILLUP secs 86400, got %d", secs)
	}
	// Super Leader default duration is 7 days (604800s) according to premium.gd:224
	if secs := receivedScrolls["II_SYS_SYS_SCR_SUPERLEADERPARTY"]; secs != 604800 {
		t.Fatalf("expected II_SYS_SYS_SCR_SUPERLEADERPARTY secs 604800, got %d", secs)
	}

	// Verify inventory consumed (all 5 slots empty)
	charData, _, _, _ := leaderInst.loadCharData()
	invList := charData["inv"].([]interface{})
	for i := 0; i < 5; i++ {
		im := invList[i].(map[string]interface{})
		if len(im) > 0 && im["id"] != nil && im["id"] != "" {
			t.Fatalf("expected slot %d to be empty, got %v", i, im)
		}
	}

	// Run evaluatePartyScrolls again: scrolls are now active in buffs, so no more should be sent
	leaderInst.evaluatePartyScrolls(now, cfg)
	select {
	case pkt := <-sentPackets:
		t.Fatalf("unexpected duplicate pscroll packet: %v", pkt)
	case <-time.After(100 * time.Millisecond):
		// Success
	}
}

func TestPartyBoost_FocusFire_LeaderTargetSync(t *testing.T) {
	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	sLeader.PosX = 100
	sLeader.PosZ = 100
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	fLeader := NewFarmEngine(leaderInst)
	leaderInst.Farm = fLeader

	// Leader targets Mob #888
	leadMob := &TrackedMob{ID: 888, Name: "Aibat", HP: 1000, MaxHP: 1000, X: 110, Z: 110}
	fLeader.currentTarget = leadMob

	pMember := models.BotProfile{ID: "bot_member", Name: "acc_member", CharacterName: "MemberBot"}
	sMember := models.DefaultStats()
	sMember.PosX = 105
	sMember.PosZ = 105
	_, _ = bm.CreateBot(pMember, &sMember)
	memberInst := bm.FindInstanceByName("acc_member")
	memberInst.Status = "online"
	fMember := NewFarmEngine(memberInst)
	memberInst.Farm = fMember

	// Member sees Mob #888 and Mob #999
	fMember.mobs[888] = &TrackedMob{ID: 888, Name: "Aibat", HP: 1000, MaxHP: 1000, X: 110, Z: 110}
	fMember.mobs[999] = &TrackedMob{ID: 999, Name: "Mushpang", HP: 500, MaxHP: 500, X: 102, Z: 102}

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SelectedMembers = []string{"MemberBot"}
	cfg.FocusFire = true
	_ = mgr.UpdateConfig(cfg)

	// Member searches for leader's target mob
	foundMob := fMember.findLeaderTargetMob("LeaderBot", 105, 105, 35.0)
	if foundMob == nil || foundMob.ID != 888 {
		t.Fatalf("expected member to prioritize leader's target Mob #888, got %v", foundMob)
	}
}

func TestPartyBoost_AutoSupport_Assignment(t *testing.T) {
	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"

	pMember := models.BotProfile{ID: "bot_member", Name: "acc_member", CharacterName: "HealerBot"}
	sMember := models.DefaultStats()
	sMember.Job = "Ringmaster"
	_, _ = bm.CreateBot(pMember, &sMember)
	memberInst := bm.FindInstanceByName("acc_member")
	memberInst.Status = "online"
	fMember := NewFarmEngine(memberInst)
	memberInst.Farm = fMember

	// Initially hunting
	initialCfg := fMember.GetConfig()
	initialCfg.Mode = "hunt"
	initialCfg.CircleHeal = false
	initialCfg.AutoResurrection = false
	fMember.SetConfig(initialCfg)

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SelectedMembers = []string{"HealerBot"}
	cfg.AutoSupport = true
	_ = mgr.UpdateConfig(cfg)

	mgr.watchdogTick(time.Now())

	updatedCfg := fMember.GetConfig()
	if updatedCfg.Mode != "support" {
		t.Fatalf("expected mode 'support', got '%s'", updatedCfg.Mode)
	}
	if !strings.EqualFold(updatedCfg.FollowTarget, "LeaderBot") {
		t.Fatalf("expected follow target 'LeaderBot', got '%s'", updatedCfg.FollowTarget)
	}
	if !updatedCfg.CircleHeal {
		t.Fatalf("expected CircleHeal to be enabled")
	}
	if !updatedCfg.AutoResurrection {
		t.Fatalf("expected AutoResurrection to be enabled")
	}
}

func TestPartyBoost_Resurrection_AutoAccept(t *testing.T) {
	bm := NewBotManager(nil, nil, nil)
	pBot := models.BotProfile{ID: "bot_fallen", Name: "acc_fallen", CharacterName: "FallenBot"}
	sBot := models.DefaultStats()
	sBot.HP = 0 // Fallen bot
	_, _ = bm.CreateBot(pBot, &sBot)
	botInst := bm.FindInstanceByName("acc_fallen")
	botInst.Status = "online"

	// Incoming resurrection packet from party member
	resPacket := []byte(`{"t":"assist","sk":"SI_ASS_HEAL_RESURRECTION","from":202,"name":"SupportBot","heal":850,"res":100}`)
	botInst.handleIncomingPacket(resPacket)

	botInst.mu.Lock()
	curHp := botInst.Stats.HP
	botInst.mu.Unlock()

	if curHp != 850 {
		t.Fatalf("expected fallen bot to be revived with 850 HP, got %d", curHp)
	}
}

func TestPartyBoost_FocusFire_TargetOverride_Immediate(t *testing.T) {
	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	sLeader.PosX = 100
	sLeader.PosZ = 100
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	fLeader := NewFarmEngine(leaderInst)
	leaderInst.Farm = fLeader

	pMember := models.BotProfile{ID: "bot_member", Name: "acc_member", CharacterName: "MemberBot"}
	sMember := models.DefaultStats()
	sMember.PosX = 102
	sMember.PosZ = 102
	_, _ = bm.CreateBot(pMember, &sMember)
	memberInst := bm.FindInstanceByName("acc_member")
	memberInst.Status = "online"
	fMember := NewFarmEngine(memberInst)
	memberInst.Farm = fMember

	// Member initially targeting Mob #111
	myMob := &TrackedMob{ID: 111, Name: "Aibat", HP: 500, MaxHP: 500, X: 103, Z: 103}
	fMember.mobs[111] = myMob
	fMember.currentTarget = myMob

	// Leader targets Mob #222
	leadMob := &TrackedMob{ID: 222, Name: "Mushpang", HP: 800, MaxHP: 800, X: 108, Z: 108}
	fLeader.currentTarget = leadMob
	fMember.mobs[222] = leadMob

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SelectedMembers = []string{"MemberBot"}
	cfg.FocusFire = true
	_ = mgr.UpdateConfig(cfg)

	// Run thinkHunt on member
	fMember.thinkHunt(fMember.GetConfig())

	// Member must have switched immediately to Leader's target Mob #222
	if fMember.currentTarget == nil || fMember.currentTarget.ID != 222 {
		t.Fatalf("expected member to override target to Mob #222, got %v", fMember.currentTarget)
	}
}

func TestPartyBoost_FocusFire_CrossInstanceShadowing_And_Movement(t *testing.T) {
	bm := NewBotManager(nil, nil, nil)
	mgr := NewPartyBoostManager(bm, filepath.Join(t.TempDir(), "party_boost.json"))
	bm.partyBoost = mgr

	pLeader := models.BotProfile{ID: "bot_leader", Name: "acc_leader", CharacterName: "LeaderBot"}
	sLeader := models.DefaultStats()
	sLeader.PosX = 100
	sLeader.PosZ = 100
	_, _ = bm.CreateBot(pLeader, &sLeader)
	leaderInst := bm.FindInstanceByName("acc_leader")
	leaderInst.Status = "online"
	fLeader := NewFarmEngine(leaderInst)
	leaderInst.Farm = fLeader

	pMember := models.BotProfile{ID: "bot_member", Name: "acc_member", CharacterName: "MemberBot"}
	sMember := models.DefaultStats()
	sMember.PosX = 100
	sMember.PosZ = 100
	_, _ = bm.CreateBot(pMember, &sMember)
	memberInst := bm.FindInstanceByName("acc_member")
	memberInst.Status = "online"
	fMember := NewFarmEngine(memberInst)
	memberInst.Farm = fMember

	// Leader targets Mob #777 way out at (300, 300)
	leadMob := &TrackedMob{ID: 777, Name: "Giant Pukepuke", HP: 1500, MaxHP: 1500, X: 300, Z: 300}
	fLeader.currentTarget = leadMob

	// Member bot radar is completely empty (no local mobs)
	if len(fMember.mobs) != 0 {
		t.Fatalf("expected empty local radar initially")
	}

	cfg := mgr.GetConfig()
	cfg.Enabled = true
	cfg.LeaderName = "LeaderBot"
	cfg.SelectedMembers = []string{"MemberBot"}
	cfg.FocusFire = true
	_ = mgr.UpdateConfig(cfg)

	// Run thinkHunt on member
	fMember.thinkHunt(fMember.GetConfig())

	// Member must have shadowed Mob #777 into its local mobs map
	shadowed, exists := fMember.mobs[777]
	if !exists || shadowed == nil {
		t.Fatalf("expected mob #777 to be shadowed into member mobs map")
	}
	if fMember.currentTarget == nil || fMember.currentTarget.ID != 777 {
		t.Fatalf("expected member currentTarget to be #777, got %v", fMember.currentTarget)
	}

	// Member must have set waypoint directly to (300, 300)
	wpX, wpZ, hasWp := memberInst.GetWaypoint()
	if !hasWp || wpX != 300 || wpZ != 300 {
		t.Fatalf("expected member waypoint at (300, 300), got (%f, %f, %v)", wpX, wpZ, hasWp)
	}
}

func TestPartyBoost_SmartMode_Plus14_And_LevelUpRoam(t *testing.T) {
	bm := NewBotManager(nil, nil, nil)
	pBot := models.BotProfile{ID: "bot_solo", Name: "acc_solo", CharacterName: "SoloBot"}
	sBot := models.DefaultStats()
	sBot.Level = 10
	sBot.PosX = 100
	sBot.PosZ = 100
	_, _ = bm.CreateBot(pBot, &sBot)
	botInst := bm.FindInstanceByName("acc_solo")
	botInst.Status = "online"
	fEngine := NewFarmEngine(botInst)
	botInst.Farm = fEngine

	cfg := models.DefaultFarmConfig()
	cfg.Enabled = true
	cfg.SmartMode = true
	if cfg.SmartMinLevelOffset != 14 || cfg.SmartMaxLevelOffset != 14 {
		t.Fatalf("expected default Smart level offsets to be 14/14, got %d/%d", cfg.SmartMinLevelOffset, cfg.SmartMaxLevelOffset)
	}
	fEngine.SetConfig(cfg)

	// findOptimalSpawn at Lv 10: target is 10 + 14 = 24
	sp := fEngine.findOptimalSpawn(10, 100, 100, cfg, time.Now())
	if sp == nil {
		t.Fatalf("expected optimal spawn for Lv 10 + 14 to be found")
	}
	meta := farmMonsters[sp.MobID]
	if meta.Level < 20 || meta.Level > 26 {
		t.Fatalf("expected monster level near 24, got %d (%s)", meta.Level, meta.Name)
	}

	// Now simulate bot leveling up from Lv 10 to Lv 35
	fEngine.lastBotLevel = 10
	botInst.mu.Lock()
	botInst.Stats.Level = 35
	botInst.mu.Unlock()

	// Run thinkHunt: triggers level up roaming
	fEngine.thinkHunt(cfg)

	// Level up re-evaluation should have updated anchor to a ~49 tier spawn and set waypoint there
	if fEngine.lastBotLevel != 35 {
		t.Fatalf("expected lastBotLevel to update to 35, got %d", fEngine.lastBotLevel)
	}
	wpX, wpZ, hasWp := botInst.GetWaypoint()
	if !hasWp {
		t.Fatalf("expected waypoint to be set to new spawn upon level up")
	}
	curCfg := fEngine.GetConfig()
	if wpX != curCfg.AnchorX || wpZ != curCfg.AnchorZ {
		t.Fatalf("expected waypoint (%f, %f) to match new anchor (%f, %f)", wpX, wpZ, curCfg.AnchorX, curCfg.AnchorZ)
	}
}

