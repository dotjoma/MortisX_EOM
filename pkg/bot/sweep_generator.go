package bot

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"madrigal-multibot/pkg/models"
)

// TerrainTile holds parsed water and attribute data for a 512x512 tile
type TerrainTile struct {
	X       int
	Z       int
	Water   [16][16]float32
	Attrs   []byte // 16641 bytes (129 x 129 grid)
	Heights []byte // 66564 bytes (129 x 129 float32 heights)
}

// WorldTerrain manages FlyFF/Madrigal height and attribute tiles
type WorldTerrain struct {
	mu     sync.RWMutex
	loaded bool
	tiles  map[string]*TerrainTile
}

var (
	globalTerrain     *WorldTerrain
	globalTerrainOnce sync.Once

	// Cache for pre-generated sweep nodes per profile key
	sweepCacheMu sync.RWMutex
	sweepCache   = make(map[string][]models.SweepNode)
)

func resolveFilePath(relPath string) string {
	candidates := []string{
		relPath,
		filepath.Join("..", relPath),
		filepath.Join("..", "..", relPath),
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, relPath),
			filepath.Join(exeDir, "..", relPath),
			filepath.Join(exeDir, "..", "..", relPath),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return relPath
}

// GetWorldTerrain returns a singleton WorldTerrain instance, lazily loaded
func GetWorldTerrain() *WorldTerrain {
	globalTerrainOnce.Do(func() {
		globalTerrain = &WorldTerrain{
			tiles: make(map[string]*TerrainTile),
		}
		globalTerrain.load()
	})
	return globalTerrain
}

func (wt *WorldTerrain) load() {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	terrainJsonPath := resolveFilePath(filepath.Join("madrigal_dump", "world", "terrain.json"))
	data, err := os.ReadFile(terrainJsonPath)
	if err != nil {
		wt.loaded = true
		return
	}

	var meta struct {
		Tiles map[string]struct {
			X     int             `json:"x"`
			Z     int             `json:"z"`
			Water [][]json.Number `json:"water"`
		} `json:"tiles"`
	}

	if err := json.Unmarshal(data, &meta); err != nil {
		wt.loaded = true
		return
	}

	tileDir := resolveFilePath(filepath.Join("madrigal_dump", "world", "tiles"))

	for key, t := range meta.Tiles {
		tile := &TerrainTile{
			X: t.X,
			Z: t.Z,
		}
		for r := 0; r < 16 && r < len(t.Water); r++ {
			for c := 0; c < 16 && c < len(t.Water[r]); c++ {
				if h, err := t.Water[r][c].Float64(); err == nil {
					tile.Water[r][c] = float32(h)
				}
			}
		}

		// Read attribute file: {tx}_{tz}_a.bin
		attrPath := filepath.Join(tileDir, fmt.Sprintf("%d_%d_a.bin", t.X, t.Z))
		if attrBytes, err := os.ReadFile(attrPath); err == nil && len(attrBytes) >= 16641 {
			tile.Attrs = attrBytes
		}

		// Read height file: {tx}_{tz}_h.bin
		hPath := filepath.Join(tileDir, fmt.Sprintf("%d_%d_h.bin", t.X, t.Z))
		if hBytes, err := os.ReadFile(hPath); err == nil && len(hBytes) >= 66564 {
			tile.Heights = hBytes
		}

		wt.tiles[key] = tile
	}

	wt.loaded = true
}

// HeightAt returns the physical terrain height at world coordinates (x, z).
// Returns 100.0 (baseline) if height data is unavailable.
func (wt *WorldTerrain) HeightAt(x, z float32) float32 {
	fx := x
	fz := -z
	if fz < 0 {
		fz = -fz
	}

	tx := int(math.Floor(float64(fx) / 512.0))
	tz := int(math.Floor(float64(fz) / 512.0))
	key := fmt.Sprintf("%d_%d", tx, tz)

	wt.mu.RLock()
	tile, exists := wt.tiles[key]
	wt.mu.RUnlock()

	if !exists || len(tile.Heights) < 66564 {
		return 100.0
	}

	localX := fx - float32(tx*512)
	localZ := fz - float32(tz*512)

	gx := localX / 4.0
	gz := localZ / 4.0
	col0 := int(math.Floor(float64(gx)))
	row0 := int(math.Floor(float64(gz)))
	txf := gx - float32(col0)
	tzf := gz - float32(row0)

	h00 := sampleHeight(tile.Heights, col0, row0)
	h10 := sampleHeight(tile.Heights, col0+1, row0)
	h01 := sampleHeight(tile.Heights, col0, row0+1)
	h11 := sampleHeight(tile.Heights, col0+1, row0+1)
	return (h00*(1-txf)+h10*txf)*(1-tzf) + (h01*(1-txf)+h11*txf)*tzf
}

func sampleHeight(buf []byte, col, row int) float32 {
	if col < 0 {
		col = 0
	} else if col > 128 {
		col = 128
	}
	if row < 0 {
		row = 0
	} else if row > 128 {
		row = 128
	}
	idx := (row*129 + col) * 4
	if idx+4 > len(buf) {
		return 100.0
	}
	bits := binary.LittleEndian.Uint32(buf[idx : idx+4])
	return math.Float32frombits(bits)
}

// AttrAt returns the terrain attribute byte at Godot world coordinates (x, z)
// Convention: Godot vector p=(x, y, -z). FlyFF world coords have z < 0, so fz = -z.
func (wt *WorldTerrain) AttrAt(x, z float32) byte {
	fx := x
	fz := -z
	if fz < 0 {
		fz = -fz
	}

	tx := int(math.Floor(float64(fx) / 512.0))
	tz := int(math.Floor(float64(fz) / 512.0))
	key := fmt.Sprintf("%d_%d", tx, tz)

	wt.mu.RLock()
	tile, exists := wt.tiles[key]
	wt.mu.RUnlock()

	if !exists {
		return 9 // Out of map bounds
	}
	if len(tile.Attrs) < 16641 {
		return 0 // Default walkable if no attribute map exists
	}

	i := int((fx - float32(tx)*512.0) / 4.0)
	j := int((fz - float32(tz)*512.0) / 4.0)
	if i < 0 {
		i = 0
	} else if i > 127 {
		i = 127
	}
	if j < 0 {
		j = 0
	} else if j > 127 {
		j = 127
	}

	idx := j*129 + i
	if idx >= len(tile.Attrs) {
		return 9
	}
	return tile.Attrs[idx]
}

// WaterHeightAt returns the water plane height at Godot world coordinates (x, z)
func (wt *WorldTerrain) WaterHeightAt(x, z float32) float32 {
	fx := x
	fz := -z
	if fz < 0 {
		fz = -fz
	}

	tx := int(math.Floor(float64(fx) / 512.0))
	tz := int(math.Floor(float64(fz) / 512.0))
	key := fmt.Sprintf("%d_%d", tx, tz)

	wt.mu.RLock()
	tile, exists := wt.tiles[key]
	wt.mu.RUnlock()

	if !exists {
		return 0
	}

	pi := int((fx - float32(tx)*512.0) / 32.0)
	pj := int((fz - float32(tz)*512.0) / 32.0)
	if pi < 0 {
		pi = 0
	} else if pi > 15 {
		pi = 15
	}
	if pj < 0 {
		pj = 0
	} else if pj > 15 {
		pj = 15
	}

	return tile.Water[pj][pi]
}

// IsWalkable returns true if attr is 0 or 2 (standard walkable terrain)
func (wt *WorldTerrain) IsWalkable(x, z float32) bool {
	a := wt.AttrAt(x, z)
	return a == 0 || a == 2
}

// IsCliff returns true if attr is not in {0,2} AND water_height <= 0
func (wt *WorldTerrain) IsCliff(x, z float32) bool {
	a := wt.AttrAt(x, z)
	wh := wt.WaterHeightAt(x, z)
	return (a != 0 && a != 2) && wh <= 0
}

// IsWater returns true if water height > 0
func (wt *WorldTerrain) IsWater(x, z float32) bool {
	return wt.WaterHeightAt(x, z) > 0
}

// IsValidSweepPoint checks if a point is within profile bounds, on an existing tile,
// and passes the cliff, walkability, and water filters.
func (wt *WorldTerrain) IsValidSweepPoint(x, z float32, profile models.MapProfile, allowWater bool) bool {
	fx := x
	fz := -z
	if fz < 0 {
		fz = -fz
	}

	tx := int(math.Floor(float64(fx) / 512.0))
	tz := int(math.Floor(float64(fz) / 512.0))

	if tx < profile.MinTileX || tx > profile.MaxTileX || tz < profile.MinTileZ || tz > profile.MaxTileZ {
		return false
	}

	key := fmt.Sprintf("%d_%d", tx, tz)
	wt.mu.RLock()
	_, exists := wt.tiles[key]
	wt.mu.RUnlock()
	if !exists {
		return false
	}

	if wt.IsCliff(x, z) {
		return false
	}

	isWater := wt.IsWater(x, z)
	if isWater && !allowWater {
		return false
	}

	if !isWater && !wt.IsWalkable(x, z) {
		return false
	}

	return true
}

func isTravelHubName(name string) bool {
	n := strings.ToLower(name)
	if strings.Contains(n, "lodestar") || strings.Contains(n, "madren") {
		return true
	}
	if strings.Contains(n, "collecting area") || strings.Contains(n, "garden of rhisis") {
		return true
	}
	if strings.Contains(n, "pumpkin town") {
		return true
	}
	return false
}

func dist2D(x1, z1, x2, z2 float32) float32 {
	dx := x1 - x2
	dz := z1 - z2
	return float32(math.Sqrt(float64(dx*dx + dz*dz)))
}

// NearestNeighborSort performs a greedy nearest-neighbor sort on nodes starting from (startX, startZ)
func NearestNeighborSort(nodes []models.SweepNode, startX, startZ float32) []models.SweepNode {
	if len(nodes) <= 1 {
		return nodes
	}

	unvisited := make([]models.SweepNode, len(nodes))
	copy(unvisited, nodes)

	sorted := make([]models.SweepNode, 0, len(nodes))
	currX, currZ := startX, startZ

	for len(unvisited) > 0 {
		bestIdx := 0
		bestDist := dist2D(currX, currZ, unvisited[0].X, unvisited[0].Z)

		for i := 1; i < len(unvisited); i++ {
			d := dist2D(currX, currZ, unvisited[i].X, unvisited[i].Z)
			if d < bestDist {
				bestDist = d
				bestIdx = i
			}
		}

		closest := unvisited[bestIdx]
		sorted = append(sorted, closest)
		currX, currZ = closest.X, closest.Z

		// Remove visited
		unvisited = append(unvisited[:bestIdx], unvisited[bestIdx+1:]...)
	}

	return sorted
}

// LoadHotspots parses mapData.json to extract lodestars, regions, monster areas, and key NPCs
func (wt *WorldTerrain) LoadHotspots(profile models.MapProfile, allowWater bool, rGuaranteed float32) []models.SweepNode {
	mapDataPath := resolveFilePath(filepath.Join("src", "features", "map-tab", "mapData.json"))
	data, err := os.ReadFile(mapDataPath)
	if err != nil {
		// Fallback to DefaultScanWaypoints if mapData.json is unavailable
		var fallback []models.SweepNode
		for _, wp := range models.DefaultScanWaypoints {
			if wt.IsValidSweepPoint(wp.X, wp.Z, profile, allowWater) {
				fallback = append(fallback, models.SweepNode{
					X:    wp.X,
					Z:    wp.Z,
					Tier: 1,
					Name: wp.Name,
				})
			}
		}
		return fallback
	}

	var mapData struct {
		Lodestars []struct {
			Name string  `json:"name"`
			X    float32 `json:"x"`
			Z    float32 `json:"z"`
		} `json:"lodestars"`
		Regions []struct {
			Name string  `json:"name"`
			X    float32 `json:"x"`
			Z    float32 `json:"z"`
			Town bool    `json:"town"`
		} `json:"regions"`
		MonsterAreas []struct {
			Name string  `json:"name"`
			X    float32 `json:"x"`
			Z    float32 `json:"z"`
		} `json:"monsterAreas"`
		NPCs []struct {
			Name string  `json:"name"`
			X    float32 `json:"x"`
			Z    float32 `json:"z"`
		} `json:"npcs"`
	}

	if err := json.Unmarshal(data, &mapData); err != nil {
		return nil
	}

	var candidates []models.SweepNode

	// 1. Lodestars
	for _, l := range mapData.Lodestars {
		candidates = append(candidates, models.SweepNode{X: l.X, Z: l.Z, Tier: 1, Name: l.Name})
	}
	// 2. Regions (towns and landmarks)
	for _, r := range mapData.Regions {
		candidates = append(candidates, models.SweepNode{X: r.X, Z: r.Z, Tier: 1, Name: r.Name})
	}
	// 3. Monster Areas
	for _, m := range mapData.MonsterAreas {
		candidates = append(candidates, models.SweepNode{X: m.X, Z: m.Z, Tier: 1, Name: m.Name})
	}
	// 4. NPCs
	for _, n := range mapData.NPCs {
		candidates = append(candidates, models.SweepNode{X: n.X, Z: n.Z, Tier: 1, Name: n.Name})
	}

	// Filter and dedupe candidates
	var deduped []models.SweepNode
	for _, c := range candidates {
		if !wt.IsValidSweepPoint(c.X, c.Z, profile, allowWater) {
			continue
		}
		tooClose := false
		for _, existing := range deduped {
			if dist2D(c.X, c.Z, existing.X, existing.Z) < rGuaranteed {
				tooClose = true
				break
			}
		}
		if !tooClose {
			deduped = append(deduped, c)
		}
	}

	return deduped
}

func (wt *WorldTerrain) appendUniqueNode(dst []models.SweepNode, n models.SweepNode, rGuar float32, profile models.MapProfile, allowWater bool) []models.SweepNode {
	if !wt.IsValidSweepPoint(n.X, n.Z, profile, allowWater) {
		return dst
	}
	for _, existing := range dst {
		if dist2D(n.X, n.Z, existing.X, existing.Z) < rGuar {
			return dst
		}
	}
	return append(dst, n)
}

// LoadTravelHubs returns server-stable town / lodestar / named-place anchors.
// These are the only probes used in hub mode; wild spawn grids are excluded.
func (wt *WorldTerrain) LoadTravelHubs(profile models.MapProfile, allowWater bool, rGuaranteed float32) []models.SweepNode {
	var hubs []models.SweepNode

	for _, wp := range models.DefaultScanWaypoints {
		hubs = wt.appendUniqueNode(hubs, models.SweepNode{X: wp.X, Z: wp.Z, Tier: 1, Name: wp.Name}, rGuaranteed, profile, allowWater)
	}

	mapDataPath := resolveFilePath(filepath.Join("src", "features", "map-tab", "mapData.json"))
	if data, err := os.ReadFile(mapDataPath); err == nil {
		var mapData struct {
			Lodestars []struct {
				Name string  `json:"name"`
				X    float32 `json:"x"`
				Z    float32 `json:"z"`
			} `json:"lodestars"`
			Regions []struct {
				Name string  `json:"name"`
				X    float32 `json:"x"`
				Z    float32 `json:"z"`
				Town bool    `json:"town"`
			} `json:"regions"`
		}
		if json.Unmarshal(data, &mapData) == nil {
			for _, l := range mapData.Lodestars {
				hubs = wt.appendUniqueNode(hubs, models.SweepNode{X: l.X, Z: l.Z, Tier: 1, Name: l.Name}, rGuaranteed, profile, allowWater)
			}
			for _, r := range mapData.Regions {
				if r.Town || isTravelHubName(r.Name) {
					hubs = wt.appendUniqueNode(hubs, models.SweepNode{X: r.X, Z: r.Z, Tier: 1, Name: r.Name}, rGuaranteed, profile, allowWater)
				}
			}
		}
	}

	named := []string{"flarine", "saintmorning", "darken", "darkon1", "darkon3", "rhisis", "madren", "pumpkin", "collect"}
	for _, name := range named {
		if x, z, ok := models.ResolveNamedPlace(name); ok {
			hubs = wt.appendUniqueNode(hubs, models.SweepNode{X: x, Z: z, Tier: 1, Name: name}, rGuaranteed, profile, allowWater)
		}
	}

	return hubs
}

func insertCorridorMidpoints(hubs []models.SweepNode, rGuar, clearance float32) []models.SweepNode {
	if len(hubs) < 2 {
		return hubs
	}
	minSpan := rGuar * 2.2
	out := make([]models.SweepNode, 0, len(hubs)*2)
	out = append(out, hubs[0])
	for i := 1; i < len(hubs); i++ {
		prev := out[len(out)-1]
		next := hubs[i]
		d := dist2D(prev.X, prev.Z, next.X, next.Z)
		if d >= minSpan {
			out = append(out, models.SweepNode{
				X:    (prev.X + next.X) / 2,
				Z:    (prev.Z + next.Z) / 2,
				Tier: 2,
				Name: fmt.Sprintf("Corridor (%.0f, %.0f)", (prev.X+next.X)/2, (prev.Z+next.Z)/2),
				Fly:  true,
			})
		}
		_ = clearance
		out = append(out, next)
	}
	return out
}

// generateStaticTiers builds Tier 1, Tier 2, and Tier 3 nodes
func (wt *WorldTerrain) generateStaticTiers(profile models.MapProfile, cfg models.ScanConfig) []models.SweepNode {
	mode := cfg.SweepMode
	if mode == "" {
		mode = models.SweepModeHub
	}
	cacheKey := fmt.Sprintf("%s_%s_%.1f_%.1f_%.1f_%v",
		profile.Name, mode, cfg.RGuaranteedM, cfg.SpacingFastM, cfg.SpacingThoroughM, cfg.AllowWaterNodes)

	sweepCacheMu.RLock()
	cached, exists := sweepCache[cacheKey]
	sweepCacheMu.RUnlock()
	if exists {
		return cached
	}

	var result []models.SweepNode

	hubs := wt.LoadTravelHubs(profile, cfg.AllowWaterNodes, cfg.RGuaranteedM)
	result = append(result, hubs...)

	if mode == models.SweepModeThorough {
		hotspots := wt.LoadHotspots(profile, cfg.AllowWaterNodes, cfg.RGuaranteedM)
		for _, h := range hotspots {
			result = wt.appendUniqueNode(result, h, cfg.RGuaranteedM, profile, cfg.AllowWaterNodes)
		}

		minX := float32(profile.MinTileX) * profile.TileSize
		maxX := float32(profile.MaxTileX+1) * profile.TileSize
		minFZ := float32(profile.MinTileZ) * profile.TileSize
		maxFZ := float32(profile.MaxTileZ+1) * profile.TileSize

		spacingFast := cfg.SpacingFastM
		if spacingFast <= 0 {
			spacingFast = 200.0
		}
		for gx := minX + spacingFast/2; gx < maxX; gx += spacingFast {
			for gfz := minFZ + spacingFast/2; gfz < maxFZ; gfz += spacingFast {
				gz := -gfz
				result = wt.appendUniqueNode(result, models.SweepNode{
					X: gx, Z: gz, Tier: 2,
					Name: fmt.Sprintf("FastGrid (%.0f, %.0f)", gx, gz),
				}, cfg.RGuaranteedM, profile, cfg.AllowWaterNodes)
			}
		}

		spacingThorough := cfg.SpacingThoroughM
		if spacingThorough <= 0 {
			spacingThorough = 140.0
		}
		for gx := minX + spacingThorough/2; gx < maxX; gx += spacingThorough {
			for gfz := minFZ + spacingThorough/2; gfz < maxFZ; gfz += spacingThorough {
				gz := -gfz
				result = wt.appendUniqueNode(result, models.SweepNode{
					X: gx, Z: gz, Tier: 3,
					Name: fmt.Sprintf("ThoroughGrid (%.0f, %.0f)", gx, gz),
				}, cfg.RGuaranteedM, profile, cfg.AllowWaterNodes)
			}
		}
	}

	sweepCacheMu.Lock()
	sweepCache[cacheKey] = result
	sweepCacheMu.Unlock()

	return result
}

// GenerateSweepNodes builds the complete tiered sweep itinerary:
// Tier 0 (Last known position + R_GUARANTEED ring)
// Tier 1 (Hotspots)
// Tier 2 (Fast expansion grid @ 200m)
// Tier 3 (Thorough grid @ 140m)
// Each tier is sorted nearest-neighbor from the bot's traversal point.
func (wt *WorldTerrain) GenerateSweepNodes(
	profile models.MapProfile,
	cfg models.ScanConfig,
	botX, botZ float32,
	lastKnown *models.WaypointCoord,
) []models.SweepNode {
	var finalPlan []models.SweepNode
	currX, currZ := botX, botZ

	// Tier 0: Last Known Position + Guaranteed Ring
	var tier0 []models.SweepNode
	if lastKnown != nil {
		if wt.IsValidSweepPoint(lastKnown.X, lastKnown.Z, profile, cfg.AllowWaterNodes) {
			tier0 = append(tier0, models.SweepNode{
				X:    lastKnown.X,
				Z:    lastKnown.Z,
				Tier: 0,
				Name: "Tier 0 (Last Known Position)",
			})
		}
		// 4 cardinal ring points at R_GUARANTEED_M
		r := cfg.RGuaranteedM
		ringOffsets := [][2]float32{
			{r, 0}, {-r, 0}, {0, r}, {0, -r},
		}
		for _, off := range ringOffsets {
			rx := lastKnown.X + off[0]
			rz := lastKnown.Z + off[1]
			if wt.IsValidSweepPoint(rx, rz, profile, cfg.AllowWaterNodes) {
				tier0 = append(tier0, models.SweepNode{
					X:    rx,
					Z:    rz,
					Tier: 0,
					Name: fmt.Sprintf("Tier 0 Ring (+%.0f, +%.0f)", off[0], off[1]),
				})
			}
		}
	}

	if len(tier0) > 0 {
		sorted0 := NearestNeighborSort(tier0, currX, currZ)
		finalPlan = append(finalPlan, sorted0...)
		currX = sorted0[len(sorted0)-1].X
		currZ = sorted0[len(sorted0)-1].Z
	}

	// Retrieve pre-computed Tier 1, 2, 3 nodes
	staticNodes := wt.generateStaticTiers(profile, cfg)

	var tier1, tier2, tier3 []models.SweepNode
	for _, n := range staticNodes {
		// If Tier 0 exists, ensure we skip nodes too close to Tier 0
		if len(tier0) > 0 {
			tooClose := false
			for _, t0 := range tier0 {
				if dist2D(n.X, n.Z, t0.X, t0.Z) < cfg.RGuaranteedM {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}
		}

		switch n.Tier {
		case 1:
			tier1 = append(tier1, n)
		case 2:
			tier2 = append(tier2, n)
		case 3:
			tier3 = append(tier3, n)
		}
	}

	// Sort Tier 1 (travel hubs)
	if len(tier1) > 0 {
		sorted1 := NearestNeighborSort(tier1, currX, currZ)
		if cfg.SweepMode == models.SweepModeCorridor {
			sorted1 = insertCorridorMidpoints(sorted1, cfg.RGuaranteedM, cfg.FlightClearanceM)
		}
		finalPlan = append(finalPlan, sorted1...)
		currX = sorted1[len(sorted1)-1].X
		currZ = sorted1[len(sorted1)-1].Z
	}

	// Sort Tier 2
	if len(tier2) > 0 {
		sorted2 := NearestNeighborSort(tier2, currX, currZ)
		finalPlan = append(finalPlan, sorted2...)
		currX = sorted2[len(sorted2)-1].X
		currZ = sorted2[len(sorted2)-1].Z
	}

	// Sort Tier 3
	if len(tier3) > 0 {
		sorted3 := NearestNeighborSort(tier3, currX, currZ)
		finalPlan = append(finalPlan, sorted3...)
	}

	return finalPlan
}
