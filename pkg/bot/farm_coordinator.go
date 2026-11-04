package bot

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LOCK HIERARCHY / CONCURRENCY RULES:
// 1. BotInstance.mu
// 2. FarmEngine.mu
// 3. FarmCoordinator.mu
// 4. ChannelCoordinator.mu
//
// FarmCoordinator and ChannelCoordinator NEVER acquire BotInstance or FarmEngine locks.
// Mutexes are held strictly for microsecond in-memory lookups.
// No I/O, no network calls, no disk operations are ever performed under these locks.

const (
	DefaultClaimTTL     = 20 * time.Second
	MinAnchorSeparation = 30.0            // meters minimum distance between assigned bot anchors
	MinZoneDwellTime    = 2 * time.Minute // minimum time in zone before idle rebalance
	MaxTransitDistance  = 800.0           // meters preferred distance threshold for relocation
)

// BotZoneAssignment tracks the currently assigned farm anchor and spawn for a bot.
type BotZoneAssignment struct {
	BotID        string
	MapName      string
	ChannelID    int
	AnchorX      float32
	AnchorZ      float32
	Radius       float32
	SpawnMobID   string
	SpawnMobName string
	SpawnMobLv   int
	AssignedAt   time.Time
	LastDwellAt  time.Time
	Spawn        *SpawnMeta
}

// MobClaim represents ownership of a mob by one or more bots (e.g. self-defense co-owners).
type MobClaim struct {
	MobID     int
	Owners    map[string]time.Time // botID -> lease expiresAt
	CreatedAt time.Time
}

// ChannelCoordinator manages mob claims and zone allocations for a single (map, channel) scope.
type ChannelCoordinator struct {
	mu              sync.RWMutex
	mapName         string
	channelID       int
	claims          map[int]*MobClaim             // mobID -> claim
	botClaims       map[string]map[int]struct{}   // botID -> set of claimed mobIDs
	zones           map[string]*BotZoneAssignment // botID -> zone assignment
	confirmedSpawns map[string]time.Time          // spawnKey ("x:z") -> last seen live mob
}

func newChannelCoordinator(mapName string, channelID int) *ChannelCoordinator {
	return &ChannelCoordinator{
		mapName:         mapName,
		channelID:       channelID,
		claims:          make(map[int]*MobClaim),
		botClaims:       make(map[string]map[int]struct{}),
		zones:           make(map[string]*BotZoneAssignment),
		confirmedSpawns: make(map[string]time.Time),
	}
}

// FarmCoordinator coordinates multi-bot automation across channels and maps within a single process.
type FarmCoordinator struct {
	mu       sync.RWMutex
	channels map[string]*ChannelCoordinator // "map:ch" -> coordinator
	onLog    func(botID, category, message, level string)
}

// GlobalCoordinator is the default singleton instance for the multibot process.
var (
	globalCoordinatorOnce sync.Once
	globalCoordinator     *FarmCoordinator
)

// GetGlobalCoordinator returns the singleton coordinator instance.
func GetGlobalCoordinator() *FarmCoordinator {
	globalCoordinatorOnce.Do(func() {
		globalCoordinator = NewFarmCoordinator(nil)
	})
	return globalCoordinator
}

// NewFarmCoordinator creates a new coordinator instance.
func NewFarmCoordinator(onLog func(botID, category, message, level string)) *FarmCoordinator {
	return &FarmCoordinator{
		channels: make(map[string]*ChannelCoordinator),
		onLog:    onLog,
	}
}

// SetLogger configures the logging callback for coordinator events.
func (fc *FarmCoordinator) SetLogger(onLog func(botID, category, message, level string)) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.onLog = onLog
}

func (fc *FarmCoordinator) emitLog(botID, msg, level string) {
	fc.mu.RLock()
	cb := fc.onLog
	fc.mu.RUnlock()
	if cb != nil {
		cb(botID, "coord", msg, level)
	}
}

func makeChannelKey(mapName string, channelID int) string {
	m := strings.ToLower(strings.TrimSpace(mapName))
	if m == "" {
		m = "madrigal"
	}
	return fmt.Sprintf("%s:%d", m, channelID)
}

var channelRegex = regexp.MustCompile(`(?i)(?:channel\s*|ch=)(\d+)`)

// ParseChannelID extracts the channel number from a server string or WS endpoint (defaults to 2)
func ParseChannelID(server string) int {
	matches := channelRegex.FindStringSubmatch(server)
	if len(matches) > 1 {
		if n, err := strconv.Atoi(matches[1]); err == nil && n > 0 {
			return n
		}
	}
	return 2
}

func (fc *FarmCoordinator) getChannel(mapName string, channelID int) *ChannelCoordinator {
	key := makeChannelKey(mapName, channelID)

	fc.mu.RLock()
	cc, exists := fc.channels[key]
	fc.mu.RUnlock()
	if exists {
		return cc
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if cc, exists = fc.channels[key]; exists {
		return cc
	}
	cc = newChannelCoordinator(mapName, channelID)
	fc.channels[key] = cc
	return cc
}

// Claim attempts to claim mobID for botID exclusively.
// If already claimed by botID, lease is renewed.
// If claimed by another bot with an active lease, returns false.
// If existing claim has expired, takes over ownership.
func (fc *FarmCoordinator) Claim(mapName string, channelID int, mobID int, botID string) bool {
	if mobID <= 0 || botID == "" {
		return false
	}
	cc := fc.getChannel(mapName, channelID)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	now := time.Now()
	claim, exists := cc.claims[mobID]

	if exists {
		// Prune expired owners
		activeOwners := 0
		botIsOwner := false
		for owner, exp := range claim.Owners {
			if now.Before(exp) {
				activeOwners++
				if owner == botID {
					botIsOwner = true
				}
			} else {
				delete(claim.Owners, owner)
				cc.unindexBotClaimLocked(owner, mobID)
			}
		}

		if botIsOwner {
			// Renew lease
			claim.Owners[botID] = now.Add(DefaultClaimTTL)
			return true
		}

		if activeOwners > 0 {
			// Claimed by someone else
			return false
		}
	}

	// Create fresh claim
	cc.claims[mobID] = &MobClaim{
		MobID: mobID,
		Owners: map[string]time.Time{
			botID: now.Add(DefaultClaimTTL),
		},
		CreatedAt: now,
	}
	cc.indexBotClaimLocked(botID, mobID)
	fc.emitLog(botID, fmt.Sprintf("[COORD] Claim mob=#%d channel=%d (ttl=%v)", mobID, channelID, DefaultClaimTTL), "debug")
	return true
}

// ClaimShared adds botID as a co-owner of mobID (used for self-defense aggro).
// Both bots are valid owners; release occurs only when all co-owners release.
func (fc *FarmCoordinator) ClaimShared(mapName string, channelID int, mobID int, botID string) {
	if mobID <= 0 || botID == "" {
		return
	}
	cc := fc.getChannel(mapName, channelID)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	now := time.Now()
	claim, exists := cc.claims[mobID]
	if !exists {
		cc.claims[mobID] = &MobClaim{
			MobID: mobID,
			Owners: map[string]time.Time{
				botID: now.Add(DefaultClaimTTL),
			},
			CreatedAt: now,
		}
	} else {
		claim.Owners[botID] = now.Add(DefaultClaimTTL)
	}
	cc.indexBotClaimLocked(botID, mobID)
	fc.emitLog(botID, fmt.Sprintf("[COORD] ClaimShared (self-defense) mob=#%d channel=%d owners=%d", mobID, channelID, len(cc.claims[mobID].Owners)), "info")
}

// Renew extends the lease of mobID for botID if it is an owner.
func (fc *FarmCoordinator) Renew(mapName string, channelID int, mobID int, botID string) {
	if mobID <= 0 || botID == "" {
		return
	}
	cc := fc.getChannel(mapName, channelID)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if claim, exists := cc.claims[mobID]; exists {
		if _, isOwner := claim.Owners[botID]; isOwner {
			claim.Owners[botID] = time.Now().Add(DefaultClaimTTL)
		}
	}
}

// Release removes botID from the ownership of mobID.
// If no co-owners remain, the mob is completely freed.
func (fc *FarmCoordinator) Release(mapName string, channelID int, mobID int, botID string, reason string) {
	if mobID <= 0 || botID == "" {
		return
	}
	cc := fc.getChannel(mapName, channelID)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if claim, exists := cc.claims[mobID]; exists {
		delete(claim.Owners, botID)
		cc.unindexBotClaimLocked(botID, mobID)
		remaining := len(claim.Owners)
		if remaining == 0 {
			delete(cc.claims, mobID)
		}
		fc.emitLog(botID, fmt.Sprintf("[COORD] Release mob=#%d channel=%d reason=%s remaining_owners=%d", mobID, channelID, reason, remaining), "debug")
	}
}

// ReleaseMobAllOwners completely releases mobID across all owners.
// Called when ANY bot on the channel observes server event "d" (death) or "l" (despawn).
func (fc *FarmCoordinator) ReleaseMobAllOwners(mapName string, channelID int, mobID int, reason string) {
	if mobID <= 0 {
		return
	}
	cc := fc.getChannel(mapName, channelID)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if claim, exists := cc.claims[mobID]; exists {
		for owner := range claim.Owners {
			cc.unindexBotClaimLocked(owner, mobID)
		}
		delete(cc.claims, mobID)
		fc.emitLog("", fmt.Sprintf("[COORD] ReleaseMobAllOwners mob=#%d channel=%d reason=%s", mobID, channelID, reason), "debug")
	}
}

// ReleaseAll removes all claims held by botID (on Stop, error, disconnect, or death).
func (fc *FarmCoordinator) ReleaseAll(botID string) {
	if botID == "" {
		return
	}

	fc.mu.RLock()
	var channelList []*ChannelCoordinator
	for _, cc := range fc.channels {
		channelList = append(channelList, cc)
	}
	fc.mu.RUnlock()

	for _, cc := range channelList {
		cc.mu.Lock()
		if mobSet, exists := cc.botClaims[botID]; exists {
			for mobID := range mobSet {
				if claim, ok := cc.claims[mobID]; ok {
					delete(claim.Owners, botID)
					if len(claim.Owners) == 0 {
						delete(cc.claims, mobID)
					}
				}
			}
			delete(cc.botClaims, botID)
		}
		delete(cc.zones, botID)
		cc.mu.Unlock()
	}
	fc.emitLog(botID, fmt.Sprintf("[COORD] ReleaseAll bot=%s", botID), "debug")
}

// IsClaimedByOther checks whether mobID has an active lease held by any bot other than botID.
// If botID itself is an active owner or co-owner, it returns false (botID is allowed to engage).
func (fc *FarmCoordinator) IsClaimedByOther(mapName string, channelID int, mobID int, botID string) bool {
	if mobID <= 0 {
		return false
	}
	cc := fc.getChannel(mapName, channelID)

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	claim, exists := cc.claims[mobID]
	if !exists {
		return false
	}

	now := time.Now()
	// If botID is already an active owner/co-owner, it is not claimed by "other"
	if exp, isOwner := claim.Owners[botID]; isOwner && now.Before(exp) {
		return false
	}

	for _, exp := range claim.Owners {
		if now.Before(exp) {
			return true
		}
	}
	return false
}

// GetClaimedMob returns the first mob claimed by botID in this channel, or 0 if none.
func (fc *FarmCoordinator) GetClaimedMob(mapName string, channelID int, botID string) int {
	if botID == "" {
		return 0
	}
	cc := fc.getChannel(mapName, channelID)

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	now := time.Now()
	if mobSet, exists := cc.botClaims[botID]; exists {
		for mobID := range mobSet {
			if claim, ok := cc.claims[mobID]; ok {
				if exp, isOwner := claim.Owners[botID]; isOwner && now.Before(exp) {
					return mobID
				}
			}
		}
	}
	return 0
}

func (cc *ChannelCoordinator) indexBotClaimLocked(botID string, mobID int) {
	set, exists := cc.botClaims[botID]
	if !exists {
		set = make(map[int]struct{})
		cc.botClaims[botID] = set
	}
	set[mobID] = struct{}{}
}

func (cc *ChannelCoordinator) unindexBotClaimLocked(botID string, mobID int) {
	if set, exists := cc.botClaims[botID]; exists {
		delete(set, mobID)
		if len(set) == 0 {
			delete(cc.botClaims, botID)
		}
	}
}

// AllocateZone finds the best spawn zone for botID respecting:
// 1. Level-fit bracket (+minOffset..+maxOffset)
// 2. Minimum anchor separation >= 30m against all other bots on the channel
// 3. Physical transit distance
// 4. Capacity & graceful degradation
// 5. Idle dwell-time hysteresis
func (fc *FarmCoordinator) AllocateZone(
	mapName string, channelID int, botID string, botLevel int, botX, botZ float32,
	minOffset, maxOffset int, forceRebalance bool,
) (*SpawnMeta, bool) {
	initFarmDatabase()
	if botID == "" || len(farmSpawns) == 0 {
		return nil, false
	}
	cc := fc.getChannel(mapName, channelID)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	now := time.Now()

	if minOffset < 1 {
		minOffset = 1
	}
	if maxOffset < 1 {
		maxOffset = 14
	}
	targetLv := botLevel + maxOffset

	minLv1 := botLevel + minOffset
	maxLv1 := botLevel + maxOffset
	minLv2 := botLevel + 10
	maxLv2 := botLevel + 16
	minLv3 := botLevel + 1
	maxLv3 := botLevel + 16
	if botLevel <= 2 {
		minLv1 = 1
		minLv2 = 1
		minLv3 = 1
	}
	minLv := minLv1
	maxLv := maxLv1
	idealLv := targetLv

	// Hysteresis: If bot already has an active zone, check if dwell time and level bracket hold
	if !forceRebalance {
		if existing, ok := cc.zones[botID]; ok && existing != nil && existing.Spawn != nil {
			if now.Sub(existing.AssignedAt) < MinZoneDwellTime &&
				existing.SpawnMobLv >= minLv && existing.SpawnMobLv <= maxLv {
				return existing.Spawn, false
			}
		}
	}

	// Prune inactive zones older than 10 minutes
	for zid, zasgn := range cc.zones {
		if zasgn != nil && now.Sub(zasgn.LastDwellAt) >= 10*time.Minute {
			delete(cc.zones, zid)
		}
	}

	var bestB1, bestB2, bestB3 *SpawnMeta
	var bestMetaB1, bestMetaB2, bestMetaB3 MobMeta
	var bestBotsB1, bestBotsB2, bestBotsB3 int
	var bestDistB1, bestDistB2, bestDistB3 float32
	bestScoreB1 := float32(math.MaxFloat32)
	bestScoreB2 := float32(math.MaxFloat32)
	bestScoreB3 := float32(math.MaxFloat32)

	for i := range farmSpawns {
		sp := &farmSpawns[i]
		meta, exists := farmMonsters[sp.MobID]
		if !exists || meta.Fly || meta.Rank == "RANK_BOSS" {
			continue
		}

		inB1 := meta.Level >= minLv1 && meta.Level <= maxLv1
		inB2 := meta.Level >= minLv2 && meta.Level <= maxLv2
		inB3 := meta.Level >= minLv3 && meta.Level <= maxLv3
		if !inB1 && !inB2 && !inB3 {
			continue
		}

		// Calculate anchor separation against all other active bots on this channel
		botsInZone := 0
		closestBotDist := float32(math.MaxFloat32)
		for otherID, otherAsgn := range cc.zones {
			if otherID == botID || otherAsgn == nil {
				continue
			}
			adx := sp.CX - otherAsgn.AnchorX
			adz := sp.CZ - otherAsgn.AnchorZ
			d := float32(math.Sqrt(float64(adx*adx + adz*adz)))
			if d < closestBotDist {
				closestBotDist = d
			}
			if d < MinAnchorSeparation {
				botsInZone++
			}
		}

		dx := sp.CX - botX
		dz := sp.CZ - botZ
		dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))

		// Score: distance + level-fit + massive separation penalty for sharing with another bot
		lvDiff := float32(math.Abs(float64(meta.Level - idealLv)))
		score := dist + (lvDiff * 80.0) + (float32(botsInZone) * 1200.0)

		// Over distance limit penalty
		if dist > MaxTransitDistance {
			score += (dist - MaxTransitDistance) * 0.5
		}

		// Live confirmed spawn bonus (-50m score)
		spawnKey := fmt.Sprintf("%.0f:%.0f", sp.CX, sp.CZ)
		if lastSeen, ok := cc.confirmedSpawns[spawnKey]; ok {
			if now.Sub(lastSeen) >= 10*time.Minute {
				delete(cc.confirmedSpawns, spawnKey)
			} else if now.Sub(lastSeen) < 5*time.Minute {
				score -= 50.0
			}
		}

		// Graceful degradation tie-breaker among crowded zones
		if botsInZone > 0 && closestBotDist < math.MaxFloat32 {
			score -= closestBotDist * 2.0
		}

		if inB1 && score < bestScoreB1 {
			bestScoreB1 = score
			bestB1 = sp
			bestMetaB1 = meta
			bestBotsB1 = botsInZone
			bestDistB1 = dist
		}
		if inB2 && score < bestScoreB2 {
			bestScoreB2 = score
			bestB2 = sp
			bestMetaB2 = meta
			bestBotsB2 = botsInZone
			bestDistB2 = dist
		}
		if inB3 && score < bestScoreB3 {
			bestScoreB3 = score
			bestB3 = sp
			bestMetaB3 = meta
			bestBotsB3 = botsInZone
			bestDistB3 = dist
		}
	}

	var bestSpawn *SpawnMeta
	var bestMeta MobMeta
	var bestBotsInZone int
	var bestDist float32

	if bestB1 != nil {
		bestSpawn = bestB1
		bestMeta = bestMetaB1
		bestBotsInZone = bestBotsB1
		bestDist = bestDistB1
	} else if bestB2 != nil {
		bestSpawn = bestB2
		bestMeta = bestMetaB2
		bestBotsInZone = bestBotsB2
		bestDist = bestDistB2
	} else if bestB3 != nil {
		bestSpawn = bestB3
		bestMeta = bestMetaB3
		bestBotsInZone = bestBotsB3
		bestDist = bestDistB3
	}

	if bestSpawn == nil {
		return nil, false
	}

	cc.zones[botID] = &BotZoneAssignment{
		BotID:        botID,
		MapName:      mapName,
		ChannelID:    channelID,
		AnchorX:      bestSpawn.CX,
		AnchorZ:      bestSpawn.CZ,
		Radius:       bestSpawn.Radius,
		SpawnMobID:   bestSpawn.MobID,
		SpawnMobName: bestMeta.Name,
		SpawnMobLv:   bestMeta.Level,
		AssignedAt:   now,
		LastDwellAt:  now,
		Spawn:        bestSpawn,
	}

	fc.emitLog(botID, fmt.Sprintf("[COORD] Allocated zone %s (Lv %d) at (%.1f, %.1f) • dist=%.0fm other_bots_in_zone=%d",
		bestMeta.Name, bestMeta.Level, bestSpawn.CX, bestSpawn.CZ, bestDist, bestBotsInZone), "info")

	return bestSpawn, true
}

// GetAssignedZone returns the current zone assignment for botID, if any.
func (fc *FarmCoordinator) GetAssignedZone(mapName string, channelID int, botID string) *BotZoneAssignment {
	if botID == "" {
		return nil
	}
	cc := fc.getChannel(mapName, channelID)
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.zones[botID]
}

// ConfirmLiveSpawn marks a spawn location as verified by live monster packet sighting.
func (fc *FarmCoordinator) ConfirmLiveSpawn(mapName string, channelID int, mobID string, x, z float32) {
	if mobID == "" {
		return
	}
	initFarmDatabase()
	cc := fc.getChannel(mapName, channelID)
	cc.mu.Lock()
	defer cc.mu.Unlock()

	for i := range farmSpawns {
		sp := &farmSpawns[i]
		if sp.MobID == mobID {
			dx := sp.CX - x
			dz := sp.CZ - z
			if float32(math.Sqrt(float64(dx*dx+dz*dz))) <= sp.Radius+15.0 {
				spawnKey := fmt.Sprintf("%.0f:%.0f", sp.CX, sp.CZ)
				cc.confirmedSpawns[spawnKey] = time.Now()
				if len(cc.confirmedSpawns) > 200 {
					now := time.Now()
					for k, t := range cc.confirmedSpawns {
						if now.Sub(t) > 10*time.Minute {
							delete(cc.confirmedSpawns, k)
						}
					}
				}
				return
			}
		}
	}
}
