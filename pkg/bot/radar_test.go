package bot

import (
	"strings"
	"testing"
	"time"
)

func TestNearbyPlayerStalePruning(t *testing.T) {
	b := &BotInstance{
		nearbyPlayers: make(map[int]*RemotePlayer),
		playerNames:   make(map[string]int),
	}

	now := time.Now()
	// Add fresh player
	b.nearbyPlayers[101] = &RemotePlayer{
		ID:       101,
		Name:     "FreshPlayer",
		X:        100,
		Y:        10,
		Z:        200,
		LastSeen: now,
	}
	b.playerNames["freshplayer"] = 101

	// Add stale player (20 seconds ago)
	b.nearbyPlayers[102] = &RemotePlayer{
		ID:       102,
		Name:     "StaleGhost",
		X:        500,
		Y:        10,
		Z:        600,
		LastSeen: now.Add(-20 * time.Second),
	}
	b.playerNames["staleghost"] = 102

	// Prune players older than 10 seconds
	b.pruneStaleNearbyPlayers(10 * time.Second)

	b.playersMu.RLock()
	defer b.playersMu.RUnlock()

	if _, ok := b.nearbyPlayers[101]; !ok {
		t.Errorf("Expected FreshPlayer (101) to remain in nearbyPlayers")
	}
	if _, ok := b.playerNames["freshplayer"]; !ok {
		t.Errorf("Expected freshplayer to remain in playerNames")
	}

	if _, ok := b.nearbyPlayers[102]; ok {
		t.Errorf("Expected StaleGhost (102) to be pruned from nearbyPlayers")
	}
	if _, ok := b.playerNames["staleghost"]; ok {
		t.Errorf("Expected staleghost to be pruned from playerNames")
	}
}

func TestEarlyExitFreshSightingVerification(t *testing.T) {
	b := &BotInstance{
		nearbyPlayers: make(map[int]*RemotePlayer),
		playerNames:   make(map[string]int),
	}

	targetName := "TestTarget"
	cleanTarget := strings.ToLower(targetName)

	warpTime := time.Now()

	// Pre-warp sighting (stale from previous waypoint)
	b.nearbyPlayers[201] = &RemotePlayer{
		ID:       201,
		Name:     targetName,
		X:        100,
		Y:        10,
		Z:        -100,
		LastSeen: warpTime.Add(-500 * time.Millisecond),
	}
	b.playerNames[cleanTarget] = 201

	// Check if pre-warp sighting triggers early exit (it must NOT)
	b.playersMu.RLock()
	var foundP *RemotePlayer
	if id, ok := b.playerNames[cleanTarget]; ok {
		if p, ok := b.nearbyPlayers[id]; ok && p.LastSeen.After(warpTime) {
			foundP = p
		}
	}
	b.playersMu.RUnlock()

	if foundP != nil {
		t.Errorf("Pre-warp sighting should not trigger early exit before new snap packet")
	}

	// Server streams fresh snap packet post-warp
	time.Sleep(10 * time.Millisecond)
	postWarpSeen := time.Now()
	b.playersMu.Lock()
	b.nearbyPlayers[201].LastSeen = postWarpSeen
	b.nearbyPlayers[201].X = 6970.0
	b.nearbyPlayers[201].Z = -3337.0
	b.playersMu.Unlock()

	// Re-check: fresh sighting must trigger early exit
	b.playersMu.RLock()
	if id, ok := b.playerNames[cleanTarget]; ok {
		if p, ok := b.nearbyPlayers[id]; ok && p.LastSeen.After(warpTime) {
			foundP = p
		}
	}
	b.playersMu.RUnlock()

	if foundP == nil {
		t.Errorf("Expected fresh sighting post-warp to be detected for early exit")
	}
	if foundP.X != 6970.0 {
		t.Errorf("Expected updated coordinates 6970.0, got %f", foundP.X)
	}
}

func TestPartyRosterIsNotAOILock(t *testing.T) {
	warpTime := time.Now()
	p := &RemotePlayer{
		ID:        9,
		Name:      "MockPlayer",
		X:         1,
		Z:         1,
		LastSeen:  warpTime.Add(20 * time.Millisecond),
		SeenEpoch: 3,
		Source:    sightParty,
	}
	if p.IsFreshAOI(3, warpTime) {
		t.Fatal("party roster coordinates must not count as a radar lock")
	}
	p.Source = sightJoin
	if !p.IsFreshAOI(3, warpTime) {
		t.Fatal("join packet after probe warp should lock")
	}
}

func TestWaypointNavigationLogic(t *testing.T) {
	// Test DetermineMap
	if m := DetermineMap(6970.0, -3337.0); m != "madrigal" {
		t.Errorf("Expected Flarine to be on madrigal map, got %s", m)
	}
	if m := DetermineMap(7244.0, -9135.0); m != "azria" {
		t.Errorf("Expected Azria coords to be azria map, got %s", m)
	}
	if m := DetermineMap(5950.0, -5400.0); m != "coral" {
		t.Errorf("Expected Coral Island coords to be coral map, got %s", m)
	}

	// Test FindClosestLodestar
	flarisLs := FindClosestLodestar("madrigal", 6970.0, -3337.0)
	if flarisLs == nil || !strings.Contains(flarisLs.Name, "Flarine") {
		t.Errorf("Expected Flarine lodestar near Flarine center, got %+v", flarisLs)
	}

	darkonLs := FindClosestLodestar("madrigal", 3800.0, -4400.0)
	if darkonLs == nil || !strings.Contains(darkonLs.Name, "Darkon") {
		t.Errorf("Expected Darkon lodestar near Darkon center, got %+v", darkonLs)
	}

	// Test MoveStep configuration & Invisibility toggle
	b := &BotInstance{}
	b.Profile.Preferences.MoveStep = 3.5
	b.Profile.Preferences.Invisible = false

	b.SetWaypoint(7000.0, -3300.0)
	if !b.hasWaypoint {
		t.Errorf("Expected hasWaypoint to be true")
	}
	if !b.invisToggledByWaypoint || !b.Profile.Preferences.Invisible {
		t.Errorf("Expected auto-invisibility to be enabled upon setting waypoint")
	}

	// Clear waypoint should restore invisibility
	b.ClearWaypoint()
	if b.hasWaypoint {
		t.Errorf("Expected hasWaypoint to be false after ClearWaypoint")
	}
	if b.invisToggledByWaypoint || b.Profile.Preferences.Invisible {
		t.Errorf("Expected auto-invisibility to revert back to false after ClearWaypoint")
	}
}
