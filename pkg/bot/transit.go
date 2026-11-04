package bot

import (
	"math"
	"strings"
	"sync"
	"time"

	"madrigal-multibot/pkg/models"
)

const (
	sightJoin  = "join"
	sightSnap  = "snap"
	sightParty = "party"
)

type warpPacer struct {
	mu         sync.Mutex
	lastWarp   time.Time
	burstCount int
}

func (p *warpPacer) Wait(cfg models.ScanConfig) {
	minInterval := time.Duration(cfg.MinWarpIntervalMs) * time.Millisecond
	if minInterval <= 0 {
		minInterval = 280 * time.Millisecond
	}
	burstSize := cfg.BurstSize
	if burstSize <= 0 {
		burstSize = 4
	}
	burstRest := time.Duration(cfg.BurstRestMs) * time.Millisecond
	if burstRest <= 0 {
		burstRest = 1200 * time.Millisecond
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if !p.lastWarp.IsZero() {
		elapsed := now.Sub(p.lastWarp)
		wait := minInterval
		if p.burstCount >= burstSize {
			wait = burstRest
			p.burstCount = 0
		}
		if elapsed < wait {
			time.Sleep(wait - elapsed)
		}
	}
	p.lastWarp = time.Now()
	p.burstCount++
}

func stanceY(x, z float32, fly bool, clearance float32) float32 {
	ground := GetWorldTerrain().HeightAt(x, z)
	if ground <= 0 {
		ground = 100.0
	}
	if fly {
		if clearance <= 0 {
			clearance = 32.0
		}
		return ground + clearance
	}
	// Exact terrain surface height (matching player.gd & world.gd:height_at)
	return ground
}

// SteerStep calculates next step coordinates and facing rotation towards target,
// steering around obstacles, steep slopes, cliffs, and water using Madrigal FSteer angle probing.
func SteerStep(curX, curZ, targetX, targetZ, step float32) (float32, float32, float32) {
	dx := targetX - curX
	dz := targetZ - curZ
	dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	if dist <= 0.001 {
		return curX, curZ, 0
	}
	if dist < step {
		step = dist
	}

	baseAngle := math.Atan2(float64(dx), float64(dz))
	wt := GetWorldTerrain()

	// Probing angle offsets in radians matching steer.gd [35°, -35°, 70°, -70°, 105°, -105°]
	angleOffsets := []float64{
		0.0,
		0.610865, -0.610865, // ~35 deg
		1.22173, -1.22173,   // ~70 deg
		1.8326, -1.8326,     // ~105 deg
	}

	curH := wt.HeightAt(curX, curZ)

	for _, offset := range angleOffsets {
		candidateAngle := baseAngle + offset
		candidateX := curX + float32(math.Sin(candidateAngle))*step
		candidateZ := curZ + float32(math.Cos(candidateAngle))*step

		// 1. Terrain walkability check (attributes 0 or 2)
		if !wt.IsWalkable(candidateX, candidateZ) {
			continue
		}
		// 2. Water avoidance
		if wt.IsWater(candidateX, candidateZ) {
			continue
		}
		// 3. Cliff avoidance
		if wt.IsCliff(candidateX, candidateZ) {
			continue
		}
		// 4. Steep cliff check: allow natural Madrigal slopes and ramps (up to 7.0m elevation rise/drop per step, matching steer.gd 80° limit)
		candH := wt.HeightAt(candidateX, candidateZ)
		if math.Abs(float64(candH-curH)) > 7.0 {
			continue
		}

		// Valid unblocked heading found
		return candidateX, candidateZ, float32(candidateAngle)
	}

	// Direct fallback if all probed angles constrained
	newX := curX + (dx/dist)*step
	newZ := curZ + (dz/dist)*step
	rot := float32(baseAngle)
	return newX, newZ, rot
}

func (p *RemotePlayer) IsFreshAOI(epoch uint64, warpTime time.Time) bool {
	if p == nil {
		return false
	}
	if p.Source == sightParty || p.Source == "who" {
		return false
	}
	return p.SeenEpoch == epoch && p.LastSeen.After(warpTime)
}

type LodestarPoint struct {
	Name string
	Map  string
	X    float32
	Z    float32
}

var KnownLodestars = []LodestarPoint{
	{Name: "Flarine Lodestar", Map: "madrigal", X: 6968.4, Z: -3328.9},
	{Name: "Northern Flaris Lodestar", Map: "madrigal", X: 7143.9, Z: -2974.8},
	{Name: "Southern Flaris Lodestar", Map: "madrigal", X: 6590.1, Z: -3811.9},
	{Name: "Saint Morning Town Lodestar", Map: "madrigal", X: 7846.0, Z: -2631.7},
	{Name: "Northern Saint Morning Lodestar", Map: "madrigal", X: 8258.6, Z: -2536.3},
	{Name: "Southern Saint Morning Lodestar", Map: "madrigal", X: 8470.6, Z: -3635.8},
	{Name: "Darkon 1 Lodestar", Map: "madrigal", X: 5583.2, Z: -3891.4},
	{Name: "Darkon City Lodestar", Map: "madrigal", X: 3808.4, Z: -4455.2},
	{Name: "Darkon 3 Lodestar", Map: "madrigal", X: 5497.1, Z: -4356.0},
	{Name: "Azria Entry", Map: "azria", X: 7244.0, Z: -9135.0},
	{Name: "Coral Island Entry", Map: "coral", X: 5950.0, Z: -5400.0},
	{Name: "Shaduwar Entry", Map: "shaduwar", X: 5950.0, Z: -5400.0},
}

func FindClosestLodestar(targetMap string, targetX, targetZ float32) *LodestarPoint {
	var best *LodestarPoint
	var bestDist float32 = 1e9

	for i := range KnownLodestars {
		ls := &KnownLodestars[i]
		if targetMap != "" && ls.Map != "" && !strings.EqualFold(ls.Map, targetMap) {
			continue
		}
		dx := targetX - ls.X
		dz := targetZ - ls.Z
		d := dx*dx + dz*dz
		if d < bestDist {
			bestDist = d
			best = ls
		}
	}
	return best
}

func DetermineMap(x, z float32) string {
	if z <= -8500.0 {
		return "azria"
	}
	if z <= -5000.0 && x <= 6200.0 {
		return "coral"
	}
	return "madrigal"
}
