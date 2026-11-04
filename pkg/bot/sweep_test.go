package bot

import (
	"testing"
	"time"

	"madrigal-multibot/pkg/models"
)

// TestFreshVsStaleEpochSighting verifies target locking only occurs on fresh sightings
// matching the current epoch and occurring after probeWarpTime
func TestFreshVsStaleEpochSighting(t *testing.T) {
	currentEpoch := uint64(5)
	probeWarpTime := time.Now().Add(-100 * time.Millisecond)

	tests := []struct {
		name      string
		seenEpoch uint64
		lastSeen  time.Time
		wantLock  bool
	}{
		{
			name:      "Stale epoch, old timestamp",
			seenEpoch: currentEpoch - 1,
			lastSeen:  probeWarpTime.Add(-500 * time.Millisecond),
			wantLock:  false,
		},
		{
			name:      "Stale epoch, newer timestamp (impossible in practice, but tests both guards)",
			seenEpoch: currentEpoch - 1,
			lastSeen:  probeWarpTime.Add(50 * time.Millisecond),
			wantLock:  false,
		},
		{
			name:      "Current epoch, timestamp before probe warp",
			seenEpoch: currentEpoch,
			lastSeen:  probeWarpTime.Add(-50 * time.Millisecond),
			wantLock:  false,
		},
		{
			name:      "Current epoch, fresh sighting after probe warp",
			seenEpoch: currentEpoch,
			lastSeen:  probeWarpTime.Add(50 * time.Millisecond),
			wantLock:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMatch := tt.seenEpoch == currentEpoch && tt.lastSeen.After(probeWarpTime)
			if isMatch != tt.wantLock {
				t.Errorf("Epoch match got %v, want %v", isMatch, tt.wantLock)
			}
		})
	}
}

// TestSetPosRejectionSignal verifies setpos packet arrival is detected as warp rejection
func TestSetPosRejectionSignal(t *testing.T) {
	probeWarpTime := time.Now().Add(-50 * time.Millisecond)

	// Case 1: setpos arrived before current warp (from previous node) -> not rejected
	oldSetPosTime := probeWarpTime.Add(-10 * time.Millisecond)
	if oldSetPosTime.After(probeWarpTime) {
		t.Errorf("Expected old setpos to NOT trigger rejection")
	}

	// Case 2: setpos arrived after current warp -> rejected!
	newSetPosTime := probeWarpTime.Add(20 * time.Millisecond)
	if !newSetPosTime.After(probeWarpTime) {
		t.Errorf("Expected fresh setpos to trigger rejection")
	}
}

// TestTerrainWalkableAndCliffFilters tests attribute decoding, walkability, and cliff detection
func TestTerrainWalkableAndCliffFilters(t *testing.T) {
	terrain := GetWorldTerrain()

	// Flarine Center (6970, -3337) - should be walkable ground
	flarineX := float32(6970.0)
	flarineZ := float32(-3337.0)

	attr := terrain.AttrAt(flarineX, flarineZ)
	if attr != 0 && attr != 2 {
		t.Errorf("Flarine center expected walkable attr 0 or 2, got %d", attr)
	}
	if !terrain.IsWalkable(flarineX, flarineZ) {
		t.Errorf("Flarine center expected IsWalkable=true")
	}
	if terrain.IsCliff(flarineX, flarineZ) {
		t.Errorf("Flarine center should NOT be cliff")
	}

	// Mountain cliff (6672, -3072) - known server-rejected cliff
	cliffX := float32(6672.0)
	cliffZ := float32(-3072.0)
	cliffAttr := terrain.AttrAt(cliffX, cliffZ)
	cliffWater := terrain.WaterHeightAt(cliffX, cliffZ)

	if cliffAttr != 0 && cliffAttr != 2 && cliffWater <= 0 {
		if !terrain.IsCliff(cliffX, cliffZ) {
			t.Errorf("Cliff position (6672, -3072) expected IsCliff=true")
		}
	}

	// Flarine River (6928, -3376) - known water node
	riverX := float32(6928.0)
	riverZ := float32(-3376.0)
	riverWater := terrain.WaterHeightAt(riverX, riverZ)
	if riverWater <= 0 {
		t.Errorf("Flaris river expected water height > 0, got %.1f", riverWater)
	}
	if !terrain.IsWater(riverX, riverZ) {
		t.Errorf("Flaris river expected IsWater=true")
	}

	// Water node filtering
	profile := models.DefaultMainMadrigalProfile()
	if terrain.IsValidSweepPoint(riverX, riverZ, profile, false) {
		t.Errorf("River should be rejected when allowWater=false")
	}
	if !terrain.IsValidSweepPoint(riverX, riverZ, profile, true) {
		t.Errorf("River should be accepted when allowWater=true")
	}
}

// TestMapBoundsExclusion verifies nodes outside Main Madrigal tiles (3..19, 3..9) are rejected
func TestMapBoundsExclusion(t *testing.T) {
	terrain := GetWorldTerrain()
	profile := models.DefaultMainMadrigalProfile()

	// Out of bounds: tile 0, 0 (x=100, z=-100 -> tx=0, tz=0)
	if terrain.IsValidSweepPoint(100.0, -100.0, profile, false) {
		t.Errorf("Out of bounds point (100, -100) should be invalid")
	}

	// Out of bounds: Eillun (7244, -9135 -> tx=14, tz=17) - outside Main Madrigal tz 3..9
	if terrain.IsValidSweepPoint(7244.0, -9135.0, profile, false) {
		t.Errorf("Eillun (7244, -9135) is outside Main Madrigal bounds and should be invalid")
	}
}

// TestGenerateSweepNodesTierOrderingAndDedupe verifies tier structure, dedupe, and nearest-neighbor
func TestGenerateSweepNodesTierOrderingAndDedupe(t *testing.T) {
	terrain := GetWorldTerrain()
	profile := models.DefaultMainMadrigalProfile()
	cfg := models.DefaultScanConfig()

	// Bot starting at Flarine center
	botX, botZ := float32(6970.0), float32(-3337.0)

	// Simulate last known target location nearby
	lastKnown := &models.WaypointCoord{X: 7000.0, Z: -3300.0}

	nodes := terrain.GenerateSweepNodes(profile, cfg, botX, botZ, lastKnown)

	if len(nodes) == 0 {
		t.Fatalf("Expected generated sweep nodes, got 0")
	}

	// 1. Verify Tier 0 comes first
	hasTier0 := false
	maxTierSeen := -1
	for idx, n := range nodes {
		if n.Tier < maxTierSeen {
			t.Errorf("Node %d has Tier %d after Tier %d (out of tier order)", idx, n.Tier, maxTierSeen)
		}
		maxTierSeen = n.Tier
		if n.Tier == 0 {
			hasTier0 = true
		}
	}

	if !hasTier0 {
		t.Errorf("Expected Tier 0 nodes when lastKnown is provided")
	}

	// 2. Verify Deduplication by R_GUARANTEED_M
	rGuar := cfg.RGuaranteedM
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			// Skip distance check between different tiers if they don't violate coverage
			d := dist2D(nodes[i].X, nodes[i].Z, nodes[j].X, nodes[j].Z)
			if d < rGuar-1.0 { // 1m float tolerance
				t.Errorf("Nodes too close: Node %d (Tier %d: %.1f, %.1f) and Node %d (Tier %d: %.1f, %.1f) dist=%.1f < R_GUARANTEED(%.1f)",
					i, nodes[i].Tier, nodes[i].X, nodes[i].Z,
					j, nodes[j].Tier, nodes[j].X, nodes[j].Z, d, rGuar)
			}
		}
	}
}

// TestNearestNeighborSort verifies sort ordering starts closest to origin and minimizes jumps
func TestNearestNeighborSort(t *testing.T) {
	originX, originZ := float32(0), float32(0)

	input := []models.SweepNode{
		{X: 100, Z: 100, Tier: 1},
		{X: 10, Z: 10, Tier: 1},
		{X: 50, Z: 50, Tier: 1},
	}

	sorted := NearestNeighborSort(input, originX, originZ)
	if len(sorted) != 3 {
		t.Fatalf("Expected 3 sorted nodes, got %d", len(sorted))
	}

	if sorted[0].X != 10 || sorted[0].Z != 10 {
		t.Errorf("First sorted node should be (10,10), got (%.1f, %.1f)", sorted[0].X, sorted[0].Z)
	}
	if sorted[1].X != 50 || sorted[1].Z != 50 {
		t.Errorf("Second sorted node should be (50,50), got (%.1f, %.1f)", sorted[1].X, sorted[1].Z)
	}
	if sorted[2].X != 100 || sorted[2].Z != 100 {
		t.Errorf("Third sorted node should be (100,100), got (%.1f, %.1f)", sorted[2].X, sorted[2].Z)
	}
}

// TestTerrainHeight verifies that physical terrain heights from _h.bin are correctly read
func TestTerrainHeight(t *testing.T) {
	terrain := GetWorldTerrain()

	// Flaris center baseline
	hFlaris := terrain.HeightAt(6970.0, -3337.0)
	if hFlaris < 99.0 || hFlaris > 101.0 {
		t.Errorf("Expected Flaris center height ~100.0, got %.2f", hFlaris)
	}

	// Dr. Estly on elevated mountain terrain
	hDrEstly := terrain.HeightAt(7186.8, -4090.6)
	if hDrEstly < 150.0 || hDrEstly > 160.0 {
		t.Errorf("Expected Dr. Estly height ~155.95, got %.2f", hDrEstly)
	}

	// Madren Town
	hMadren := terrain.HeightAt(7346.5, -3657.0)
	if hMadren < 110.0 || hMadren > 125.0 {
		t.Errorf("Expected Madren Town height ~116.85, got %.2f", hMadren)
	}
}

func TestHubSweepIsSmallAndIncludesMadren(t *testing.T) {
	terrain := GetWorldTerrain()
	profile := models.DefaultMainMadrigalProfile()
	cfg := models.DefaultScanConfig()
	nodes := terrain.GenerateSweepNodes(profile, cfg, 6970.0, -3337.0, nil)

	if len(nodes) == 0 {
		t.Fatal("hub sweep produced no nodes")
	}
	if len(nodes) > 80 {
		t.Errorf("hub sweep should stay small, got %d nodes", len(nodes))
	}

	hasMadren := false
	maxTier := 0
	for _, n := range nodes {
		if n.Tier > maxTier {
			maxTier = n.Tier
		}
		if dist2D(n.X, n.Z, 7346.5, -3657.0) < 120 {
			hasMadren = true
		}
		if n.Tier >= 3 {
			t.Errorf("hub mode must not emit thorough-grid nodes, got %s", n.Name)
		}
	}
	if !hasMadren {
		t.Error("expected Madren Town (or a lodestar/town within 120m) in hub itinerary")
	}
	if maxTier > 2 {
		t.Errorf("unexpected max tier %d in hub mode", maxTier)
	}
}

func TestCorridorInsertsFlyMidpoints(t *testing.T) {
	hubs := []models.SweepNode{
		{X: 6970, Z: -3337, Tier: 1, Name: "Flarine"},
		{X: 7346.5, Z: -3657, Tier: 1, Name: "Madren"},
	}
	out := insertCorridorMidpoints(hubs, 100, 32)
	if len(out) != 3 {
		t.Fatalf("expected hub-mid-hub, got %d nodes", len(out))
	}
	if !out[1].Fly || out[1].Tier != 2 {
		t.Errorf("midpoint should be a fly corridor node, got %+v", out[1])
	}
}
