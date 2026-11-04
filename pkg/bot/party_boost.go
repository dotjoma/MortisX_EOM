package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"madrigal-multibot/pkg/models"
)

// PartyBoostManager coordinates the persistent state and roster discovery for Party Boost.
type PartyBoostManager struct {
	mu         sync.RWMutex
	config     models.PartyBoostConfig
	configPath string
	botManager *BotManager

	ctx             context.Context
	cancel          context.CancelFunc
	running         bool
	stopWg          sync.WaitGroup
	inviteCooldowns map[string]time.Time
	lastInviteTick  time.Time
	lastScrollTick  time.Time
}

// NewPartyBoostManager creates a new manager bound to the BotManager.
func NewPartyBoostManager(bm *BotManager, customConfigPath ...string) *PartyBoostManager {
	p := defaultPartyBoostConfigPath()
	if len(customConfigPath) > 0 && customConfigPath[0] != "" {
		p = customConfigPath[0]
	}

	mgr := &PartyBoostManager{
		config:          models.DefaultPartyBoostConfig(),
		configPath:      p,
		botManager:      bm,
		inviteCooldowns: make(map[string]time.Time),
	}

	// Attempt to load persistent config from disk, falling back to defaults
	_, _ = mgr.Load()
	mgr.Start()
	return mgr
}

func defaultPartyBoostConfigPath() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Morti", "party_boost.json")
}

// GetConfig returns a thread-safe copy of the active configuration.
func (m *PartyBoostManager) GetConfig() models.PartyBoostConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig updates the in-memory configuration and saves it to disk.
func (m *PartyBoostManager) UpdateConfig(cfg models.PartyBoostConfig) error {
	cfg.Sanitize()
	m.mu.Lock()
	m.config = cfg
	m.mu.Unlock()

	return m.Save()
}

// Save writes the current configuration to disk as formatted JSON.
func (m *PartyBoostManager) Save(customPath ...string) error {
	m.mu.RLock()
	cfg := m.config
	path := m.configPath
	m.mu.RUnlock()

	if len(customPath) > 0 && customPath[0] != "" {
		path = customPath[0]
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal party boost config: %w", err)
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write tmp party boost config: %w", err)
	}
	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile)
		return os.WriteFile(path, data, 0600)
	}

	return nil
}

// Load reads and parses the configuration from disk.
func (m *PartyBoostManager) Load(customPath ...string) (models.PartyBoostConfig, error) {
	path := m.configPath
	if len(customPath) > 0 && customPath[0] != "" {
		path = customPath[0]
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			def := models.DefaultPartyBoostConfig()
			m.mu.Lock()
			m.config = def
			m.mu.Unlock()
			_ = m.Save(path)
			return def, nil
		}
		return m.GetConfig(), err
	}

	var parsed models.PartyBoostConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		return m.GetConfig(), fmt.Errorf("failed to parse party boost config: %w", err)
	}

	parsed.Sanitize()
	m.mu.Lock()
	m.config = parsed
	m.mu.Unlock()

	return parsed, nil
}

// DiscoverRoster inspects all active bot instances in the Multibot pool and compiles
// an enriched roster snapshot with character names, levels, jobs, channels, distance to
// the selected leader, and party statuses.
func (m *PartyBoostManager) DiscoverRoster() []models.PartyRosterEntry {
	if m.botManager == nil {
		return nil
	}

	m.mu.RLock()
	cfg := m.config
	m.mu.RUnlock()

	summaries := m.botManager.ListBots()

	// Find the designated leader's position if online
	var leaderFound bool
	var leaderX, leaderZ float32
	leaderLower := strings.ToLower(strings.TrimSpace(cfg.LeaderName))

	if leaderLower != "" {
		for _, s := range summaries {
			charLower := strings.ToLower(strings.TrimSpace(s.CharacterName))
			nameLower := strings.ToLower(strings.TrimSpace(s.Name))
			if charLower == leaderLower || nameLower == leaderLower {
				if s.Status == "online" {
					leaderFound = true
					leaderX = s.Stats.PosX
					leaderZ = s.Stats.PosZ
				}
				break
			}
		}
	}

	roster := make([]models.PartyRosterEntry, 0, len(summaries))
	for _, s := range summaries {
		charName := s.CharacterName
		if charName == "" {
			charName = s.Name
		}

		isLeader := false
		if leaderLower != "" {
			isLeader = strings.EqualFold(s.CharacterName, cfg.LeaderName) || strings.EqualFold(s.Name, cfg.LeaderName)
		}

		// Calculate 2D Euclidean distance to leader
		var dist float64
		var inShareRange bool
		if leaderFound && s.Status == "online" {
			if isLeader {
				dist = 0.0
				inShareRange = true
			} else {
				dx := float64(s.Stats.PosX - leaderX)
				dz := float64(s.Stats.PosZ - leaderZ)
				dist = math.Sqrt(dx*dx + dz*dz)
				inShareRange = (dist <= models.ServerExpShareRangeM)
			}
		}

		// Query bot instance for live party status
		inParty := false
		var partyID, partyLevel, partyKind, partyShare, partyDist, partyPoints, syncLevel int
		var partyName, partyLeader string
		var syncActive, scrollFull, scrollExpUp, scrollSkillUp bool
		var modes, cd map[string]float64
		if inst := m.botManager.FindInstanceByName(s.Name); inst != nil {
			inParty, partyID, partyName, partyLeader = inst.GetPartyState()
			partyLevel, _, partyPoints, partyKind, partyShare, partyDist, syncActive, syncLevel = inst.GetPartyDetails()
			scrollFull, scrollExpUp, scrollSkillUp = inst.GetPartyScrollsState()
			modes, cd = inst.GetPartyModesAndCD()
		}

		roster = append(roster, models.PartyRosterEntry{
			BotID:              s.ID,
			AccountName:        s.Name,
			CharacterName:      charName,
			Level:              s.Stats.Level,
			Job:                s.Stats.Job,
			Channel:            s.Server,
			Status:             s.Status,
			InParty:            inParty,
			PartyID:            partyID,
			PartyName:          partyName,
			PartyLeader:        partyLeader,
			PartyLevel:         partyLevel,
			PartyKind:          partyKind,
			PartyShare:         partyShare,
			PartyDist:          partyDist,
			PartyPoints:        partyPoints,
			PartyScrollFull:    scrollFull,
			PartyScrollExpUp:   scrollExpUp,
			PartyScrollSkillUp: scrollSkillUp,
			PartyModes:         modes,
			PartySkillCD:       cd,
			IsLeader:           isLeader,
			DistanceToLeader:   math.Round(dist*10) / 10,
			InShareRange:       inShareRange,
			LevelSyncActive:    syncActive,
			SyncLevel:          syncLevel,
		})
	}

	// Sort roster: Leader first, then online bots, then by CharacterName
	sort.Slice(roster, func(i, j int) bool {
		if roster[i].IsLeader != roster[j].IsLeader {
			return roster[i].IsLeader
		}
		if (roster[i].Status == "online") != (roster[j].Status == "online") {
			return roster[i].Status == "online"
		}
		return strings.ToLower(roster[i].CharacterName) < strings.ToLower(roster[j].CharacterName)
	})

	return roster
}

// Start launches the background watchdog loop for party formation and distance leashing.
func (m *PartyBoostManager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.running = true
	m.stopWg.Add(1)
	m.mu.Unlock()

	go m.loop()
}

// Stop terminates the background watchdog loop cleanly.
func (m *PartyBoostManager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.cancel()
	m.running = false
	m.mu.Unlock()

	m.stopWg.Wait()
}

func (m *PartyBoostManager) loop() {
	defer m.stopWg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case now := <-ticker.C:
			m.watchdogTick(now)
		}
	}
}

// watchdogTick executes a single cycle of the party watchdog.
func (m *PartyBoostManager) watchdogTick(now time.Time) {
	if m.botManager == nil {
		return
	}

	cfg := m.GetConfig()
	if !cfg.Enabled {
		m.clearAllLeashes(cfg)
		return
	}

	leaderName := strings.TrimSpace(cfg.LeaderName)
	if leaderName == "" {
		return
	}

	// 1. Locate Leader Bot instance
	leaderInst := m.botManager.FindInstanceByName(leaderName)
	if leaderInst == nil {
		return
	}

	leaderSummary := leaderInst.Summary()
	if leaderSummary.Status != "online" {
		return
	}

	leaderX := leaderSummary.Stats.PosX
	leaderZ := leaderSummary.Stats.PosZ
	leaderZone := leaderSummary.Stats.Zone
	leaderServer := leaderSummary.Server

	// 2. Leader Party Rules & Mechanics Automation (Every 1 second)
	leaderInst.evaluatePartyRules()

	// 3. Leader Party Skills Maintainer Loop (Every 1 second)
	leaderInst.evaluatePartySkills(now, cfg)

	// 4. Leader Party Scrolls Maintainer Loop (Every 5 seconds)
	if m.lastScrollTick.IsZero() || now.Sub(m.lastScrollTick) >= 5*time.Second {
		m.lastScrollTick = now
		leaderInst.evaluatePartyScrolls(now, cfg)
	}

	// 5. Leader Auto-Invite Loop (Every 3 seconds)
	if cfg.AutoReinvite && now.Sub(m.lastInviteTick) >= 3*time.Second {
		m.lastInviteTick = now
		m.runLeaderAutoInvite(leaderInst, cfg, now)
	}

	// 6. Member Distance Leash Loop (Every 1 second)
	if cfg.LeashEnabled {
		m.runDistanceLeash(leaderInst, leaderX, leaderZ, leaderZone, leaderServer, cfg)
	} else {
		m.clearAllLeashes(cfg)
	}

	// 7. Auto-Sync Farm Start/Stop & Auto-Support Configuration
	m.runPartyFarmSync(leaderInst, cfg)

	// 8. Level Sync Evaluation across active party bots
	for _, inst := range m.botManager.GetAllInstances() {
		inst.evaluateLevelSync()
	}
}

// SyncPartyFarm immediately synchronizes member FarmEngine start/stop and roles with the leader.
func (m *PartyBoostManager) SyncPartyFarm(leaderInst *BotInstance) {
	if m == nil || leaderInst == nil {
		return
	}
	cfg := m.GetConfig()
	if !cfg.Enabled {
		return
	}
	m.runPartyFarmSync(leaderInst, cfg)
}

func (m *PartyBoostManager) runPartyFarmSync(leaderInst *BotInstance, cfg models.PartyBoostConfig) {
	leaderName := strings.TrimSpace(cfg.LeaderName)
	if leaderName == "" || leaderInst == nil {
		return
	}

	leaderSummary := leaderInst.Summary()
	leaderX := leaderSummary.Stats.PosX
	leaderY := leaderSummary.Stats.PosY
	leaderZ := leaderSummary.Stats.PosZ
	leaderIsFarming := leaderInst.Farm != nil && leaderInst.Farm.IsRunning()

	for _, memberName := range cfg.SelectedMembers {
		if strings.EqualFold(memberName, leaderName) {
			continue
		}
		memInst := m.botManager.FindInstanceByName(memberName)
		if memInst == nil || memInst.Status != "online" || memInst.Farm == nil {
			continue
		}

		memInst.mu.Lock()
		job := memInst.Stats.Job
		memInst.mu.Unlock()
		jobLower := strings.ToLower(job)
		isAssist := strings.Contains(jobLower, "assist") || strings.Contains(jobLower, "ringmaster") || strings.Contains(jobLower, "billposter")

		targetMode := "hunt"
		if cfg.AutoSupport && isAssist {
			targetMode = "support"
		}

		fCfg := memInst.Farm.GetConfig()
		needsUpdate := false

		if targetMode == "support" {
			if fCfg.Mode != "support" {
				fCfg.Mode = "support"
				needsUpdate = true
			}
			if !strings.EqualFold(fCfg.FollowTarget, leaderName) {
				fCfg.FollowTarget = leaderName
				needsUpdate = true
			}
			if !fCfg.CircleHeal {
				fCfg.CircleHeal = true
				needsUpdate = true
			}
			if !fCfg.AutoResurrection {
				fCfg.AutoResurrection = true
				needsUpdate = true
			}
			if fCfg.FollowDist <= 0 || fCfg.FollowDist > 10 {
				fCfg.FollowDist = 4.0
				needsUpdate = true
			}
		}

		if leaderIsFarming {
			if !fCfg.Enabled {
				fCfg.Enabled = true
				needsUpdate = true
			}
			if fCfg.Mode != targetMode {
				fCfg.Mode = targetMode
				needsUpdate = true
			}
			// Dynamic anchor sync to Leader position
			if fCfg.AnchorX != leaderX || fCfg.AnchorY != leaderY || fCfg.AnchorZ != leaderZ {
				fCfg.AnchorX = leaderX
				fCfg.AnchorY = leaderY
				fCfg.AnchorZ = leaderZ
				needsUpdate = true
			}

			if needsUpdate {
				memInst.Farm.SetConfig(fCfg)
			}

			if !memInst.Farm.IsRunning() {
				_ = memInst.Farm.Start()
				memInst.emitLog("party", fmt.Sprintf("Party Boost: Auto-started Farm Engine in %s mode (following %s)", targetMode, leaderName), "info")
			}
		} else {
			if needsUpdate {
				memInst.Farm.SetConfig(fCfg)
			}
			// Leader is not farming -> stop member FarmEngine if running
			if memInst.Farm.IsRunning() {
				memInst.Farm.Stop()
				memInst.emitLog("party", "Party Boost: Leader stopped farming - stopped member Farm Engine", "info")
			}
		}
	}
}

// SyncNow forces an immediate execution of party assembly, auto-invites, rules, and maintainer loops.
func (m *PartyBoostManager) SyncNow() error {
	m.mu.Lock()
	m.lastInviteTick = time.Time{}
	m.lastScrollTick = time.Time{}
	m.mu.Unlock()

	m.watchdogTick(time.Now())
	return nil
}

func (m *PartyBoostManager) clearAllLeashes(cfg models.PartyBoostConfig) {
	for _, memberName := range cfg.SelectedMembers {
		if memInst := m.botManager.FindInstanceByName(memberName); memInst != nil {
			if memInst.IsPartyLeashed() {
				memInst.SetPartyLeashed(false)
				memInst.ClearWaypoint()
			}
		}
	}
}

func (m *PartyBoostManager) runLeaderAutoInvite(leaderInst *BotInstance, cfg models.PartyBoostConfig, now time.Time) {
	curPartyCount := leaderInst.PartyMemberCount()
	if curPartyCount >= 8 {
		return
	}

	for _, memberName := range cfg.SelectedMembers {
		cleanName := strings.TrimSpace(memberName)
		if cleanName == "" || strings.EqualFold(cleanName, cfg.LeaderName) {
			continue
		}

		// Find member bot instance and check online status
		memInst := m.botManager.FindInstanceByName(cleanName)
		if memInst == nil {
			continue
		}
		memSummary := memInst.Summary()
		if memSummary.Status != "online" {
			continue
		}

		targetCharName := memSummary.CharacterName
		if targetCharName == "" {
			targetCharName = cleanName
		}

		// Check if member is already in leader's party
		if leaderInst.HasPartyMember(targetCharName) || leaderInst.HasPartyMember(cleanName) {
			continue
		}

		// Anti-spam cooldown: 10s per target
		lowerTarget := strings.ToLower(targetCharName)
		m.mu.RLock()
		lastInv, exists := m.inviteCooldowns[lowerTarget]
		m.mu.RUnlock()
		if exists && now.Sub(lastInv) < 10*time.Second {
			continue
		}

		// Issue invite from leader
		if err := leaderInst.SendPartyInvite(targetCharName); err == nil {
			m.mu.Lock()
			m.inviteCooldowns[lowerTarget] = now
			m.mu.Unlock()
			leaderInst.emitLog("party", fmt.Sprintf("Party Boost: Sent party invite to %s", targetCharName), "info")
			curPartyCount++
			if curPartyCount >= 8 {
				break
			}
		}
	}
}

func (m *PartyBoostManager) runDistanceLeash(leaderInst *BotInstance, leaderX, leaderZ float32, leaderZone string, leaderServer string, cfg models.PartyBoostConfig) {
	leashThreshold := cfg.LeashDistance
	if leashThreshold <= 0 {
		leashThreshold = models.DefaultLeashDistanceM
	}

	for _, memberName := range cfg.SelectedMembers {
		cleanName := strings.TrimSpace(memberName)
		if cleanName == "" || strings.EqualFold(cleanName, cfg.LeaderName) {
			continue
		}

		memInst := m.botManager.FindInstanceByName(cleanName)
		if memInst == nil {
			continue
		}

		memSummary := memInst.Summary()
		if memSummary.Status != "online" {
			if memInst.IsPartyLeashed() {
				memInst.SetPartyLeashed(false)
			}
			continue
		}

		// Ensure member and leader are in same channel / server
		if memSummary.Server != "" && leaderServer != "" && memSummary.Server != leaderServer {
			continue
		}

		// Ensure member and leader are in same zone / map
		if memSummary.Stats.Zone != "" && leaderZone != "" && !strings.EqualFold(memSummary.Stats.Zone, leaderZone) {
			continue
		}

		dx := float64(memSummary.Stats.PosX - leaderX)
		dz := float64(memSummary.Stats.PosZ - leaderZ)
		dist := math.Sqrt(dx*dx + dz*dz)

		if memInst.IsPartyLeashed() {
			// Leash active: check if member has arrived within <= 20m
			if dist <= 20.0 {
				memInst.SetPartyLeashed(false)
				memInst.ClearWaypoint()
				memInst.emitLog("party", fmt.Sprintf("Party Boost: Rejoined leader within %.1fm (leash released)", dist), "info")
			} else {
				// Re-aim waypoint towards leader if leader moved > 3m
				wpX, wpZ, hasWp := memInst.GetWaypoint()
				if !hasWp || math.Abs(float64(wpX-leaderX)) > 3.0 || math.Abs(float64(wpZ-leaderZ)) > 3.0 {
					memInst.SetWaypoint(leaderX, leaderZ)
				}
			}
		} else {
			// Check if exceeded leash threshold (> 60m)
			if dist > leashThreshold {
				memInst.SetPartyLeashed(true)
				memInst.SetWaypoint(leaderX, leaderZ)
				memInst.emitLog("party", fmt.Sprintf("Party Boost: Distance to leader (%.1fm) exceeds leash limit (%.0fm). Leashing to leader...", dist, leashThreshold), "info")
			}
		}
	}
}
