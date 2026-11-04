package bot

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"madrigal-multibot/pkg/models"
)

//go:embed farm_mobs_data.json
var farmMobsDataRaw []byte

//go:embed item_actions.json
var itemActionsRaw []byte

type mobDataContainer struct {
	Monsters map[string]MobMeta `json:"monsters"`
	Spawns   []SpawnMeta        `json:"spawns"`
}

type MobMeta struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level int    `json:"lv"`
	HP    int    `json:"hp"`
	Rank  string `json:"rank"`
	Fly   bool   `json:"fly"`
	Aggro bool   `json:"aggro"`
	Exp   int    `json:"exp"`
}

type SpawnMeta struct {
	MobID  string  `json:"mi"`
	CX     float32 `json:"cx"`
	CZ     float32 `json:"cz"`
	Radius float32 `json:"rad"`
}

var (
	farmDbOnce   sync.Once
	farmMonsters = make(map[string]MobMeta)
	farmSpawns   = make([]SpawnMeta, 0)

	itemDbOnce sync.Once
	itemParams = make(map[string]map[string]int)
)

func initFarmDatabase() {
	farmDbOnce.Do(func() {
		if len(farmMobsDataRaw) == 0 {
			return
		}
		var container mobDataContainer
		if err := json.Unmarshal(farmMobsDataRaw, &container); err == nil {
			farmMonsters = container.Monsters
			farmSpawns = container.Spawns
		}
	})
}

func initItemDatabase() {
	itemDbOnce.Do(func() {
		if len(itemActionsRaw) == 0 {
			return
		}
		var raw map[string]struct {
			Params [][]interface{} `json:"params"`
		}
		if err := json.Unmarshal(itemActionsRaw, &raw); err == nil {
			for id, meta := range raw {
				if len(meta.Params) > 0 {
					m := make(map[string]int)
					for _, p := range meta.Params {
						if len(p) >= 2 {
							if paramName, ok := p[0].(string); ok {
								if paramVal, ok := p[1].(float64); ok {
									m[paramName] = int(paramVal)
								}
							}
						}
					}
					if len(m) > 0 {
						itemParams[id] = m
					}
				}
			}
		}
	})
}

// GetFoodValue retrieves restore value for given stat destination ("DST_HP", "DST_MP", "DST_FP"),
// matching bot.gd:418-423.
func GetFoodValue(itemID string, dst string) int {
	initItemDatabase()
	if m, ok := itemParams[itemID]; ok {
		return m[dst]
	}
	return 0
}

// Real-client constants extracted from madrigal_dump reference
const (
	ClientThinkTick              = 200 * time.Millisecond // bot.gd:113 (_t = 0.2)
	ClientStateSendHz            = 10                     // remote.gd:7 (SEND_HZ = 10.0)
	ClientRunSpeed               = 7.5                    // player.gd:6 (RUN = 7.5)
	ClientWalkSpeed              = 2.6                    // player.gd:7 (WALK = 2.6)
	ClientAggroRadius            = 20.0                   // bot.gd:335 (distance_to < 20.0)
	ClientMaxTargetYDelta        = 12.0                   // bot.gd:355 (abs(y - p.y) <= 12.0)
	ClientLeashReturnDist        = 4.0                    // bot.gd:149 (off.length() > 4.0)
	ClientFightStuckTimeout      = 12.0 * time.Second     // bot.gd:321 (_tgt_t > 12.0)
	ClientFightBlacklistDuration = 40.0 * time.Second     // bot.gd:322 (_black[mo] = 40.0)
	ClientLootStuckTimeout       = 6.0 * time.Second      // bot.gd:366 (_drop_t > 6.0)
	ClientLootBlacklistDuration  = 60.0 * time.Second     // bot.gd:366 (_black[drop] = 60.0)
	ClientLootPlayerDist         = 14.0                   // bot.gd:372 (dd < 14.0)
	ClientLootAnchorExtra        = 6.0                    // bot.gd:378 (cfg.range + 6.0)
	ClientLootArrivalReach       = 1.2                    // player.gd:414 (reach = 1.2)
	ClientItemCooldown           = 1200 * time.Millisecond // bot.gd:446 (_item_cd[k] = 1.2)
	ClientHoldSwingCooldown      = 3.0                    // bot.gd:329 (cooldown <= 3.0)
	ClientRestRecoveryThreshold  = 95.0                   // bot.gd:126 (hp_pct >= 95.0)
	ClientSaveInterval           = 8.0 * time.Second      // net.gd:302 (_sync_t = 8.0)

	ClientSmartIdleRelocateTimeout = 15 * time.Second
	ClientSmartEmptyAreaCooldown   = 180 * time.Second
	ClientSmartRelocateMaxAttempts = 5
	ClientSmartTravelStuckTimeout  = 8 * time.Second
)

// CombatReach calculates attack arrival reach matching player.gd:409 and stats.gd:221-226.
// Formula: 1.5 + target_radius + attack_reach
func CombatReach(job string, targetRadius float32) float32 {
	if targetRadius <= 0 {
		targetRadius = 0.8
	}
	attackReach := float32(0.0) // Melee baseline (Vagrant, Mercenary, Blade, Knight, Assist, Billposter, Ringmaster)
	j := strings.ToLower(job)
	switch j {
	case "magician", "psykeeper", "elementor":
		attackReach = 13.0 // Wand base reach (stats.gd:223)
	case "ranger":
		attackReach = 17.0 // Bow reach (stats.gd:224)
	case "acrobat", "jester":
		attackReach = 3.2 // Yoyo reach (stats.gd:225)
	}
	return 1.5 + targetRadius + attackReach
}

// ScoreSupplyItem implements the real client overheal penalty formula from bot.gd:440:
// score = min(v, missing) - max(0, v - missing) * 0.25
func ScoreSupplyItem(val int, missing int) float64 {
	v := float64(val)
	m := float64(missing)
	minVal := v
	if m < minVal {
		minVal = m
	}
	overheal := v - m
	if overheal < 0 {
		overheal = 0
	}
	return minVal - overheal*0.25
}

// CalcMaxHP calculates maximum HP matching formula.gd:216-224
func CalcMaxHP(level int, sta int) int {
	if level < 1 {
		level = 1
	}
	if sta < 15 {
		sta = 15
	}
	lv := float64(level)
	st := float64(sta)
	a := (0.9 * lv) / 2.0
	b := a * ((lv + 1.0) / 4.0) * (1.0 + st / 50.0) + st * 10.0
	return int(b + 80.0)
}

// CalcMaxMP calculates maximum MP matching formula.gd:226-229
func CalcMaxMP(level int, intStat int) int {
	if level < 1 {
		level = 1
	}
	if intStat < 15 {
		intStat = 15
	}
	lv := float64(level)
	i := float64(intStat)
	n := ((lv * 2.0 + i * 8.0) * 0.3 + 22.0) + i * 0.3
	res := int(n)
	if res < 1 {
		res = 1
	}
	return res
}

// CalcMaxFP calculates maximum FP matching formula.gd:231-234
func CalcMaxFP(level int, sta int) int {
	if level < 1 {
		level = 1
	}
	if sta < 15 {
		sta = 15
	}
	lv := float64(level)
	s := float64(sta)
	n := (lv * 2.0 + s * 6.0) * 0.3 + s * 0.3
	res := int(n)
	if res < 1 {
		res = 1
	}
	return res
}

// TrackedDrop represents a nearby ground item drop parsed from WS "loot" packets (matching drops.gd)
type TrackedDrop struct {
	Key       string
	MobID     int
	ItemID    string
	Count     int
	X         float32
	Y         float32
	Z         float32
	CreatedAt time.Time
}

// TrackedMob represents a nearby monster parsed from WS "mob" sync packets
type TrackedMob struct {
	ID                int
	MobIndex          string
	Name              string
	Level             int
	Exp               int
	X                 float32
	Y                 float32
	Z                 float32
	HP                int
	MaxHP             int
	Dead              bool
	Chase             bool
	TargetPlayerID    int
	ClaimedByPlayerID int
	ClaimedTime       time.Time
	BotDamaged        bool
	LastSeen          time.Time
	IsShadow          bool
	ServerVerified    bool
}

// PendingMobSwing tracks an incoming monster attack animation swing in flight.
type PendingMobSwing struct {
	MobID    int
	MobName  string
	MobIndex string
	TargetID int
	Method   int
	Ranged   bool
	DueTime  time.Time
}

// CombatObservationStats tracks incoming combat metrics and unclamped shadow HP.
type CombatObservationStats struct {
	TotalSwings   int64 `json:"total_swings"`
	TotalWhiffs   int64 `json:"total_whiffs"`
	TotalHits     int64 `json:"total_hits"`
	TotalMisses   int64 `json:"total_misses"`
	TotalBlocks   int64 `json:"total_blocks"`
	TotalDamage   int64 `json:"total_damage"`
	ShadowHP      int64 `json:"shadow_hp"`
	WouldDieCount int64 `json:"would_die_count"`
}

// FarmEngine controls autonomous hunting, smart targeting, buff upkeep, and loot collection
type FarmEngine struct {
	bot      *BotInstance
	mu       sync.RWMutex
	config   models.FarmConfig
	stats    models.FarmStats
	running  bool
	cancelCh chan struct{}

	mobsMu    sync.RWMutex
	mobs      map[int]*TrackedMob
	blacklist map[int]time.Time

	dropsMu       sync.RWMutex
	drops         map[string]*TrackedDrop
	dropBlacklist map[string]time.Time
	currentDrop   *TrackedDrop
	dropStartTime time.Time

	currentTarget          *TrackedMob
	targetStartTime        time.Time
	targetLastHp           int
	targetSwingsWithoutHit int
	targetLastServerHit    time.Time
	sessionStartTime       time.Time
	startExp         int64
	startPenya       int64
	kills            int

	lastLuckTick   time.Time
	lastPotionTick time.Time
	lastMpTick     time.Time
	lastFpTick     time.Time
	lastEvacAlert  time.Time
	lastResting    bool

	swingsMu      sync.Mutex
	pendingSwings map[int]PendingMobSwing
	obsStats      CombatObservationStats
	combatRNG     CombatRNG

	swingCounter   int
	lastAttackTick time.Time
	swingStartTime time.Time
	swingTargetID  int
	attackPhase        int // 0 = ready/approaching, 1 = swinging (250ms windup), 2 = recovery
	postKillWait       time.Time
	lastSawSpawnMob    time.Time
	lastHpProgressTime time.Time

	noTargetStartTime   time.Time
	emptyAreaCooldown   map[string]time.Time
	relocateAttempts    int
	relocatePausedUntil time.Time
	travelStartTime     time.Time
	lastTravelDist      float32
	lastTravelCheckTime time.Time
	lastBotLevel        int
	supportWaitStart    time.Time
	clock               func() time.Time
}

func NewFarmEngine(bot *BotInstance) *FarmEngine {
	initFarmDatabase()
	fe := &FarmEngine{
		bot:               bot,
		config:            models.DefaultFarmConfig(),
		mobs:              make(map[int]*TrackedMob),
		blacklist:         make(map[int]time.Time),
		drops:             make(map[string]*TrackedDrop),
		dropBlacklist:     make(map[string]time.Time),
		pendingSwings:     make(map[int]PendingMobSwing),
		emptyAreaCooldown: make(map[string]time.Time),
		clock:             time.Now,
	}
	fe.stats.State = "Idle"
	return fe
}

func (fe *FarmEngine) now() time.Time {
	if fe.clock != nil {
		return fe.clock()
	}
	return time.Now()
}

func (fe *FarmEngine) setAnchor(newX, newZ float32) {
	fe.mu.Lock()
	fe.config.AnchorX = newX
	fe.config.AnchorZ = newZ
	fe.mu.Unlock()
	fe.travelStartTime = time.Time{}
	fe.lastTravelDist = 0
	fe.lastTravelCheckTime = time.Time{}
}

// GetConfig returns current farm configuration
func (fe *FarmEngine) GetConfig() models.FarmConfig {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	return fe.config
}

// SetConfig updates farm configuration
func (fe *FarmEngine) SetConfig(cfg models.FarmConfig) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.config = cfg
}

// Start begins farm execution loop
func (fe *FarmEngine) Start() error {
	var targetWpX, targetWpZ float32
	var hasTargetWp bool

	fe.bot.mu.Lock()
	botX := fe.bot.Stats.PosX
	botY := fe.bot.Stats.PosY
	botZ := fe.bot.Stats.PosZ
	botLevel := int(fe.bot.Stats.Level)
	botPenya := int64(fe.bot.Stats.Gold)
	botHP := int64(fe.bot.Stats.HP)
	fe.bot.mu.Unlock()

	cfg := fe.GetConfig()
	anchorX := cfg.AnchorX
	anchorY := cfg.AnchorY
	anchorZ := cfg.AnchorZ
	var smartLogMsg string

	leadInstStart, _, isMemberStart := fe.getPartyLeaderInstance()
	if isMemberStart {
		leadSummary := leadInstStart.Summary()
		anchorX = leadSummary.Stats.PosX
		anchorY = leadSummary.Stats.PosY
		anchorZ = leadSummary.Stats.PosZ
	} else if cfg.SmartMode {
		effLevel := fe.getEffectiveLevel(cfg)
		if sp := fe.findOptimalSpawn(effLevel, botX, botZ, cfg, fe.now()); sp != nil {
			anchorX = sp.CX
			anchorY = botY
			anchorZ = sp.CZ
			meta := farmMonsters[sp.MobID]
			dist := float32(math.Sqrt(float64((sp.CX-botX)*(sp.CX-botX) + (sp.CZ-botZ)*(sp.CZ-botZ))))
			smartLogMsg = fmt.Sprintf("Smart Mode: Target spawn set to %s (Lv %d) at (%.1f, %.1f) • %.0fm away",
				meta.Name, meta.Level, sp.CX, sp.CZ, dist)
			if dist > cfg.Radius {
				targetWpX = sp.CX
				targetWpZ = sp.CZ
				hasTargetWp = true
			}
		} else if anchorX == 0 && anchorZ == 0 {
			anchorX = botX
			anchorY = botY
			anchorZ = botZ
		}
	} else if anchorX == 0 && anchorZ == 0 {
		anchorX = botX
		anchorY = botY
		anchorZ = botZ
	}

	fe.mu.Lock()
	if fe.running {
		fe.mu.Unlock()
		return nil
	}
	fe.running = true
	fe.cancelCh = make(chan struct{})
	fe.sessionStartTime = fe.now()
	fe.kills = 0
	fe.lastHpProgressTime = fe.now()
	fe.startExp = 0
	fe.startPenya = botPenya
	fe.lastResting = false

	fe.swingsMu.Lock()
	fe.pendingSwings = make(map[int]PendingMobSwing)
	fe.obsStats = CombatObservationStats{ShadowHP: botHP}
	fe.swingsMu.Unlock()

	fe.config.AnchorX = anchorX
	fe.config.AnchorY = anchorY
	fe.config.AnchorZ = anchorZ
	mode := fe.config.Mode
	radius := fe.config.Radius
	fe.mu.Unlock()

	if smartLogMsg != "" {
		fe.bot.emitLog("farm", smartLogMsg, "info")
	}
	if hasTargetWp {
		fe.bot.SetWaypoint(targetWpX, targetWpZ)
	}

	fe.bot.emitLog("farm", fmt.Sprintf("Farm Automation started (%s mode, anchor (%.1f, %.1f), radius %.0fm)",
		mode, anchorX, anchorZ, radius), "info")

	fe.lastBotLevel = botLevel

	if fe.bot != nil && fe.bot.IsPartyLeader() {
		fe.bot.partyMu.RLock()
		mgr := fe.bot.partyBoost
		fe.bot.partyMu.RUnlock()
		if mgr != nil {
			mgr.SyncPartyFarm(fe.bot)
		}
	}

	go fe.loop()
	return nil
}

func (fe *FarmEngine) getMap() string {
	if fe.bot == nil {
		return "madrigal"
	}
	return fe.bot.GetMap()
}

func (fe *FarmEngine) getChannelID() int {
	if fe.bot == nil {
		return 2
	}
	return fe.bot.GetChannelID()
}

func (fe *FarmEngine) getBotID() string {
	if fe.bot == nil {
		return ""
	}
	return fe.bot.ID
}

func (fe *FarmEngine) recordHpProgress() {
	fe.mu.Lock()
	fe.lastHpProgressTime = fe.now()
	fe.mu.Unlock()
}

func (fe *FarmEngine) getLastHpProgressTime() time.Time {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	return fe.lastHpProgressTime
}

// Stop ends farm execution loop
func (fe *FarmEngine) Stop() {
	fe.mu.Lock()
	if !fe.running {
		fe.mu.Unlock()
		return
	}
	fe.running = false
	if fe.cancelCh != nil {
		close(fe.cancelCh)
		fe.cancelCh = nil
	}
	fe.stats.Running = false
	fe.stats.State = "Idle"
	fe.currentTarget = nil
	fe.mu.Unlock()

	if botID := fe.getBotID(); botID != "" {
		GetGlobalCoordinator().ReleaseAll(botID)
	}

	fe.bot.ClearWaypoint()
	fe.bot.BroadcastState("stand", 0)
	fe.bot.emitLog("farm", "Farm Automation stopped.", "info")

	if fe.bot != nil && fe.bot.IsPartyLeader() {
		fe.bot.partyMu.RLock()
		mgr := fe.bot.partyBoost
		fe.bot.partyMu.RUnlock()
		if mgr != nil {
			mgr.SyncPartyFarm(fe.bot)
		}
	}
}

// IsRunning reports whether farm loop is active
func (fe *FarmEngine) IsRunning() bool {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	return fe.running
}

// IsFighting reports whether farm engine is actively attacking a live monster target
func (fe *FarmEngine) IsFighting() bool {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	return fe.running && fe.currentTarget != nil && !fe.currentTarget.Dead && fe.currentTarget.HP > 0
}

// loop runs at 50ms tick for swing queue accuracy, while gating think logic at 200ms (ClientThinkTick)
func (fe *FarmEngine) loop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var lastThink time.Time
	for {
		select {
		case <-fe.cancelCh:
			return
		case <-ticker.C:
			now := fe.now()
			fe.drainPendingSwings(now)
			if now.Sub(lastThink) >= ClientThinkTick {
				lastThink = now
				fe.tick()
			}
		}
	}
}

func toBool(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case float64:
		return b != 0
	case int:
		return b != 0
	}
	return false
}

// enqueueMobSwing schedules an incoming monster swing matching movers.gd:496-505
func (fe *FarmEngine) enqueueMobSwing(mob *TrackedMob, method int, isRanged bool, now time.Time) {
	hitFrac := 0.5
	if meta, ok := models.GetMonsterCombatMeta(mob.MobIndex); ok {
		if method == 2 && meta.HitFracAtk2 > 0 {
			hitFrac = meta.HitFracAtk2
		} else if meta.HitFracAtk1 > 0 {
			hitFrac = meta.HitFracAtk1
		}
	}
	windup := time.Duration(float64(ClientMobAnimLengthGuess) * hitFrac)
	fe.swingsMu.Lock()
	fe.pendingSwings[mob.ID] = PendingMobSwing{
		MobID:    mob.ID,
		MobName:  mob.Name,
		MobIndex: mob.MobIndex,
		TargetID: fe.bot.GetEntityID(),
		Method:   method,
		Ranged:   isRanged,
		DueTime:  now.Add(windup),
	}
	fe.swingsMu.Unlock()
	fe.bot.emitLog("combat", fmt.Sprintf("<- Mob #%d (%s) swings at you! (method %d, windup %v)", mob.ID, mob.Name, method, windup.Round(time.Millisecond)), "debug")
}

// cancelPendingSwing cancels any pending swing for a dead or despawned mob (movers.gd:508)
func (fe *FarmEngine) cancelPendingSwing(mobID int) {
	fe.swingsMu.Lock()
	delete(fe.pendingSwings, mobID)
	fe.swingsMu.Unlock()
}

// drainPendingSwings processes expired swings in 50ms tick
func (fe *FarmEngine) drainPendingSwings(now time.Time) {
	fe.swingsMu.Lock()
	if len(fe.pendingSwings) == 0 {
		fe.swingsMu.Unlock()
		return
	}
	var expired []PendingMobSwing
	for mobID, s := range fe.pendingSwings {
		if now.After(s.DueTime) || now.Equal(s.DueTime) {
			expired = append(expired, s)
			delete(fe.pendingSwings, mobID)
		}
	}
	fe.swingsMu.Unlock()

	for _, s := range expired {
		fe.processMobSwing(s, now)
	}
}

// processMobSwing evaluates range gate, computes damage, and updates shadow stats / HP clamp
func (fe *FarmEngine) processMobSwing(s PendingMobSwing, now time.Time) {
	fe.bot.mu.Lock()
	isDead := fe.bot.Stats.HP == 0
	pLevel := int(fe.bot.Stats.Level)
	pSTA := int(fe.bot.Stats.STA)
	pDEX := int(fe.bot.Stats.DEX)
	pJob := fe.bot.Stats.Job
	pX := fe.bot.Stats.PosX
	pY := fe.bot.Stats.PosY
	pZ := fe.bot.Stats.PosZ
	pHP := int64(fe.bot.Stats.HP)
	fe.bot.mu.Unlock()

	if isDead {
		return
	}

	fe.mobsMu.RLock()
	mob, exists := fe.mobs[s.MobID]
	if !exists || mob.Dead || mob.HP <= 0 {
		fe.mobsMu.RUnlock()
		return
	}
	mobX := mob.X
	mobY := mob.Y
	mobZ := mob.Z
	fe.mobsMu.RUnlock()

	// 3D range gate matching movers.gd:521
	dx := mobX - pX
	dy := mobY - pY
	dz := mobZ - pZ
	dist3D := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))

	mobRadius := ClientDefaultMobRadius
	if r, ok := MobRadiusOverrides[s.MobIndex]; ok {
		mobRadius = r
	}
	if dist3D >= 4.5+mobRadius {
		fe.swingsMu.Lock()
		fe.obsStats.TotalWhiffs++
		fe.swingsMu.Unlock()
		fe.bot.emitLog("combat", fmt.Sprintf("<- Mob #%d (%s) swing out of range (%.2fm >= %.2fm)", s.MobID, s.MobName, dist3D, 4.5+mobRadius), "debug")
		return
	}

	mobMeta, ok := models.GetMonsterCombatMeta(s.MobIndex)
	if !ok {
		mobMeta = models.MonsterCombatMeta{
			Level: 1,
			HR:    50,
			DEX:   15,
			AtkM:  []int{1, 1},
			Atk1:  []int{0, 0},
			Rank:  "",
		}
	}

	playerInput := PlayerCombatInput{
		Level:    pLevel,
		STA:      pSTA,
		DEX:      pDEX,
		Job:      pJob,
		ArmorMin: 8,
		ArmorMax: 10,
	}
	monInput := MonsterCombatInput{
		Level:      mobMeta.Level,
		HR:         mobMeta.HR,
		ER:         mobMeta.ER,
		DEX:        mobMeta.DEX,
		AtkM:       mobMeta.AtkM,
		Atk1:       mobMeta.Atk1,
		Atk2AB:     mobMeta.Atk2AB,
		Rank:       mobMeta.Rank,
		Element:    mobMeta.El,
		ElementLvl: mobMeta.Ell,
	}

	res := MonsterAttack(playerInput, monInput, s.Method, s.Ranged, fe.combatRNG)

	// Update combat timer (player.gd:1391: combat_t = 10.0 even on miss)
	fe.bot.SetCombatTimer(now.Add(10 * time.Second))

	fe.swingsMu.Lock()
	fe.obsStats.TotalSwings++
	if fe.obsStats.ShadowHP == 0 {
		fe.obsStats.ShadowHP = pHP
	}

	if res.Missed {
		fe.obsStats.TotalMisses++
		fe.swingsMu.Unlock()
		fe.bot.emitLog("combat", fmt.Sprintf("<- Mob #%d (%s) missed you!", s.MobID, s.MobName), "info")
		return
	}

	if res.Blocked {
		fe.obsStats.TotalBlocks++
	}
	fe.obsStats.TotalHits++
	fe.obsStats.TotalDamage += int64(res.Damage)
	fe.obsStats.ShadowHP -= int64(res.Damage)
	shadowHP := fe.obsStats.ShadowHP
	fe.swingsMu.Unlock()

	// Observation mode: apply damage with 1 HP clamp (no death yet)
	fe.bot.mu.Lock()
	oldHP := int64(fe.bot.Stats.HP)
	newHP := oldHP - int64(res.Damage)
	wouldDie := newHP <= 0
	if wouldDie {
		fe.bot.Stats.HP = 1 // observation safety clamp
	} else {
		fe.bot.Stats.HP = uint32(newHP)
	}
	fe.bot.mu.Unlock()

	if wouldDie {
		fe.swingsMu.Lock()
		fe.obsStats.WouldDieCount++
		fatalCount := fe.obsStats.WouldDieCount
		fe.swingsMu.Unlock()
		fe.bot.emitLog("combat", fmt.Sprintf("<- Mob #%d (%s) hit for %d dmg! WOULD DIE (Shadow HP: %d, Fatal hits: %d)", s.MobID, s.MobName, res.Damage, shadowHP, fatalCount), "error")
	} else {
		fe.bot.emitLog("combat", fmt.Sprintf("<- Mob #%d (%s) hit you for %d dmg (HP: %d, Shadow HP: %d)", s.MobID, s.MobName, res.Damage, newHP, shadowHP), "warning")
	}
}

// GetObservationStats returns current combat observation statistics
func (fe *FarmEngine) GetObservationStats() CombatObservationStats {
	fe.swingsMu.Lock()
	defer fe.swingsMu.Unlock()
	return fe.obsStats
}

func (fe *FarmEngine) tick() {
	if !fe.bot.ws.IsConnected() {
		return
	}

	// 0. Party Leash: If member bot is returning to leader, pause combat actions
	if fe.bot != nil && fe.bot.IsPartyLeashed() {
		fe.setState("Party Leash: Returning to Leader")
		return
	}

	fe.mu.Lock()
	cfg := fe.config
	fe.mu.Unlock()

	now := fe.now()

	// 1. Clean expired blacklists
	fe.mobsMu.Lock()
	for id, until := range fe.blacklist {
		if now.After(until) {
			delete(fe.blacklist, id)
		}
	}
	fe.mobsMu.Unlock()

	fe.dropsMu.Lock()
	for key, until := range fe.dropBlacklist {
		if now.After(until) {
			delete(fe.dropBlacklist, key)
		}
	}
	fe.dropsMu.Unlock()

	// 2. Lucky packet maintenance (every 2 seconds)
	if cfg.AutoLucky && now.Sub(fe.lastLuckTick) >= 2*time.Second {
		fe.lastLuckTick = now
		fe.sendLuckPacket()
	}

	// 3. Supplies & Potions check (each stat cooldown: 1.2s, matching bot.gd:409-447)
	if cfg.AutoPotions {
		fe.checkSupplies(cfg)
	}

	// 4. Smart Mode: Player Proximity Check & Area Evacuation
	if cfg.SmartMode && cfg.EvacuateOnPlayerNearby {
		if fe.checkPlayerProximity(cfg) {
			return // actively evacuating, skip combat tick
		}
	}

	// 5. HP Rest Mode
	if fe.checkRestState(cfg) {
		return
	}

	// 6. Support Mode vs Hunt Mode
	if cfg.Mode == "support" {
		fe.thinkSupport(cfg)
	} else {
		fe.thinkHunt(cfg)
	}

	// 7. Update Live Farm Stats
	fe.updateStats()
}

// OnMobPacket parses server "mob" sync packets to update nearby monster coordinates and states
func (fe *FarmEngine) OnMobPacket(raw map[string]interface{}) {
	fe.mobsMu.Lock()
	defer fe.mobsMu.Unlock()

	now := fe.now()

	// 1. Joins / Spawns: "j" -> [[id, type, x, z, hp, aggro, ri, sst, ...], ...]
	if jList, ok := raw["j"].([]interface{}); ok {
		for _, item := range jList {
			if entry, ok := item.([]interface{}); ok && len(entry) >= 5 {
				id := toInt(entry[0])
				x := toFloat32(entry[2])
				z := toFloat32(entry[3])
				hp := toInt(entry[4])
				maxHp := hp

				ri := -1
				if len(entry) >= 7 {
					ri = toInt(entry[6])
				}

				mobIdx := ""
				name := fmt.Sprintf("Monster_%d", id)
				level := 1
				exp := 2

				if ri >= 0 && ri < len(farmSpawns) {
					mobIdx = farmSpawns[ri].MobID
					if meta, exists := farmMonsters[mobIdx]; exists {
						name = meta.Name
						level = meta.Level
						if meta.Exp > 0 {
							exp = meta.Exp
						} else {
							exp = level * 2
						}
						if maxHp < meta.HP {
							maxHp = meta.HP
						}
					}
				}

				y := GetWorldTerrain().HeightAt(x, z)
				isChase := false
				if len(entry) >= 8 {
					sst := toInt(entry[7])
					isChase = (sst == 1)
				}

				claimedBy := 0
				var claimedTime time.Time
				if hp < maxHp {
					claimedBy = -1
					claimedTime = now
				}

				fe.mobs[id] = &TrackedMob{
					ID:                id,
					MobIndex:          mobIdx,
					Name:              name,
					Level:             level,
					Exp:               exp,
					X:                 x,
					Y:                 y,
					Z:                 z,
					HP:                hp,
					MaxHP:             maxHp,
					Chase:             isChase,
					ClaimedByPlayerID: claimedBy,
					ClaimedTime:       claimedTime,
					LastSeen:          now,
					ServerVerified:    true,
					IsShadow:          false,
				}
				if mobIdx != "" {
					GetGlobalCoordinator().ConfirmLiveSpawn(fe.getMap(), fe.getChannelID(), mobIdx, x, z)
				}
				fe.lastSawSpawnMob = now
			}
		}
	}

	// 2. State updates: "s" -> [[id, x, z, shp, state, target_id, yaw], ...]
	if sList, ok := raw["s"].([]interface{}); ok {
		myID := fe.bot.GetEntityID()
		for _, item := range sList {
			if entry, ok := item.([]interface{}); ok && len(entry) >= 4 {
				id := toInt(entry[0])
				mob, exists := fe.mobs[id]
				if !exists || mob.Dead {
					continue
				}
				shp := toInt(entry[3])
				if shp <= 0 {
					mob.Dead = true
					mob.HP = 0
					mob.Chase = false
					if fe.currentTarget != nil && fe.currentTarget.ID == id {
						fe.clearTarget("s_zero_hp")
					}
					continue
				}
				mob.X = toFloat32(entry[1])
				mob.Z = toFloat32(entry[2])
				mob.Y = GetWorldTerrain().HeightAt(mob.X, mob.Z)
				if mob.MaxHP < shp {
					mob.MaxHP = shp
				}
				// If mob took external damage while bot has not hit it, mark claimed by another player
				if shp < mob.HP && !mob.BotDamaged && mob.ClaimedByPlayerID == 0 {
					mob.ClaimedByPlayerID = -1
					mob.ClaimedTime = now
				}
				mob.HP = shp
				mob.LastSeen = now
				mob.ServerVerified = true
				mob.IsShadow = false
				if len(entry) >= 5 {
					st := toInt(entry[4])
					if mob.BotDamaged && !mob.Dead {
						mob.Chase = true
					} else {
						mob.Chase = (st == 1)
					}
				}
				if len(entry) >= 6 {
					targetID := toInt(entry[5])
					mob.TargetPlayerID = targetID
					if targetID > 0 && targetID != myID {
						mob.ClaimedByPlayerID = targetID
						mob.ClaimedTime = now
					}
				}
			}
		}
	}

	// 3. Events: "e" -> [["d", mob_id, killer_id, shares], ["h", mob_id, attacker_id, dmg, crit], ["!", mob_id], ["a", mob_id, target_id, dmg, crit]]
	if eList, ok := raw["e"].([]interface{}); ok {
		myID := fe.bot.GetEntityID()
		for _, item := range eList {
			if entry, ok := item.([]interface{}); ok && len(entry) >= 2 {
				evType, _ := entry[0].(string)
				mobID := toInt(entry[1])
				if evType == "d" {
					fe.cancelPendingSwing(mobID)
					if mob, exists := fe.mobs[mobID]; exists {
						mob.Dead = true
						mob.HP = 0
						mob.Chase = false
					}
					if fe.getBotID() != "" {
						fe.mu.RLock()
						coordEnabled := fe.config.CoordinatedFarm
						fe.mu.RUnlock()
						if coordEnabled {
							GetGlobalCoordinator().ReleaseMobAllOwners(fe.getMap(), fe.getChannelID(), mobID, "d")
						}
					}
					killerID := 0
					if len(entry) >= 3 {
						killerID = toInt(entry[2])
					}
					var myShare float64
					if len(entry) >= 4 {
						if shares, ok := entry[3].([]interface{}); ok {
							for _, shItem := range shares {
								if shPair, ok := shItem.([]interface{}); ok && len(shPair) >= 2 {
									if toInt(shPair[0]) == myID {
										myShare = toFloat64(shPair[1])
									}
								}
							}
						}
					}
					isOurKill := (killerID > 0 && killerID == myID) || myShare > 0 || (fe.currentTarget != nil && fe.currentTarget.ID == mobID)
					if isOurKill {
						fe.recordHpProgress()
						mobName := fmt.Sprintf("Monster_%d", mobID)
						if mob, exists := fe.mobs[mobID]; exists && mob.Name != "" {
							mobName = mob.Name
							if myShare > 0 {
								expAward := int(float64(mob.Exp) * myShare)
								if expAward < 1 {
									expAward = 1
								}
								fe.bot.AddExp(expAward, mob.Level)
							} else {
								fe.bot.AddExp(mob.Exp, mob.Level)
							}
						}
						fe.bot.emitLog("combat", fmt.Sprintf("<- %s (#%d) slain (event 'd')! Total kills: %d", mobName, mobID, fe.kills+1), "success")
						fe.kills++
						if fe.currentTarget != nil && fe.currentTarget.ID == mobID {
							fe.clearTarget("d")
						}
						fe.postKillWait = fe.now().Add(500 * time.Millisecond)

						fe.bot.mu.Lock()
						lastLoot := fe.bot.lastLootTime
						fe.bot.lastLootTime = time.Time{}
						fe.bot.mu.Unlock()
						if !lastLoot.IsZero() {
							lootLatency := time.Since(lastLoot)
							if fe.bot.telemetry != nil {
								fe.bot.telemetry.RecordLootToD(lootLatency)
							}
							if lootLatency > 500*time.Millisecond {
								fe.bot.emitLog("perf", fmt.Sprintf("[LATENCY] Loot-to-d event delay: %v", lootLatency), "warn")
							}
						}
					}
				} else if evType == "h" {
					if mob, exists := fe.mobs[mobID]; exists && len(entry) >= 4 {
						attackerID := 0
						if len(entry) >= 3 {
							attackerID = toInt(entry[2])
						}
						hitDmg := toInt(entry[3])
						crit := 0
						if len(entry) >= 5 {
							crit = toInt(entry[4])
						}
						if mob.HP > hitDmg {
							mob.HP -= hitDmg
						} else {
							mob.HP = 0
						}
						if attackerID > 0 && attackerID == myID {
							fe.recordHpProgress()
							mob.BotDamaged = true
							mob.ClaimedByPlayerID = myID
							if fe.currentTarget != nil && fe.currentTarget.ID == mobID {
								fe.targetLastHp = mob.HP
								fe.targetStartTime = fe.now()
								fe.targetSwingsWithoutHit = 0
								fe.targetLastServerHit = fe.now()
							}
							critTag := ""
							if crit == 1 {
								critTag = " [CRIT]"
							}
							fe.bot.emitLog("combat", fmt.Sprintf("<- Mob #%d (%s) took %d dmg%s (HP: %d/%d)", mobID, mob.Name, hitDmg, critTag, mob.HP, mob.MaxHP), "info")
						} else if attackerID > 0 && attackerID != myID {
							mob.ClaimedByPlayerID = attackerID
							mob.ClaimedTime = now
						}
					}
				} else if evType == "a" {
					if mob, exists := fe.mobs[mobID]; exists {
						mob.Chase = true
						targetID := 0
						if len(entry) >= 3 {
							targetID = toInt(entry[2])
							mob.TargetPlayerID = targetID
							if targetID > 0 && targetID != myID {
								mob.ClaimedByPlayerID = targetID
								mob.ClaimedTime = now
							}
						}
						method := 1
						if len(entry) >= 4 {
							method = toInt(entry[3])
						}
						isRanged := false
						if len(entry) >= 5 {
							isRanged = toBool(entry[4])
						}
						if targetID > 0 && targetID == myID {
							fe.enqueueMobSwing(mob, method, isRanged, now)
						}
					}
				} else if evType == "!" {
					if mob, exists := fe.mobs[mobID]; exists {
						mob.Chase = true
						fe.bot.emitLog("combat", fmt.Sprintf("<- Mob #%d (%s) aggro'd [!]", mobID, mob.Name), "info")
					}
				}
			}
		}
	}

	// 4. Leaves / Despawns: "l" -> [id1, id2, ...]
	if lList, ok := raw["l"].([]interface{}); ok {
		botID := fe.getBotID()
		fe.mu.RLock()
		coordEnabled := fe.config.CoordinatedFarm
		fe.mu.RUnlock()
		mapName := fe.getMap()
		chID := fe.getChannelID()
		coord := GetGlobalCoordinator()

		for _, idVal := range lList {
			id := toInt(idVal)
			fe.cancelPendingSwing(id)
			delete(fe.mobs, id)
			if coordEnabled && botID != "" {
				coord.ReleaseMobAllOwners(mapName, chID, id, "despawn")
			}
			if fe.currentTarget != nil && fe.currentTarget.ID == id {
				fe.clearTarget("despawn")
			}
		}
	}
}

// OnLootPacket handles incoming "loot" websocket packet, spawns ground drops for items,
// and updates stats matching mobsync.gd:135-150.
func (fe *FarmEngine) OnLootPacket(raw map[string]interface{}) {
	penya, _ := raw["penya"].(float64)
	itemsRaw, _ := raw["items"].([]interface{})
	mobID := toInt(raw["id"])
	x := toFloat32(raw["x"])
	z := toFloat32(raw["z"])

	// If x and z are 0 in packet, look up mob position
	if x == 0 && z == 0 && mobID > 0 {
		fe.mobsMu.RLock()
		if mob, exists := fe.mobs[mobID]; exists {
			x = mob.X
			z = mob.Z
		}
		fe.mobsMu.RUnlock()
	}

	y := GetWorldTerrain().HeightAt(x, z)
	now := fe.now()

	// Spawn tracked ground drops for items (mobsync.gd:147-149)
	if len(itemsRaw) > 0 {
		fe.dropsMu.Lock()
		for i, itRaw := range itemsRaw {
			if itArr, ok := itRaw.([]interface{}); ok && len(itArr) >= 2 {
				itemID, _ := itArr[0].(string)
				cnt := int(toFloat32(itArr[1]))
				if itemID != "" {
					key := fmt.Sprintf("%d_%s_%d", mobID, itemID, i)
					fe.drops[key] = &TrackedDrop{
						Key:       key,
						MobID:     mobID,
						ItemID:    itemID,
						Count:     cnt,
						X:         x,
						Y:         y,
						Z:         z,
						CreatedAt: now,
					}
				}
			}
		}
		fe.dropsMu.Unlock()
	}

	// Update farm loot stats
	fe.OnLootReceived(penya, itemsRaw)
}

// OnLootReceived updates farm stats when penya or items drop
func (fe *FarmEngine) OnLootReceived(penya float64, itemsRaw []interface{}) {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	nowStr := time.Now().Format("15:04:05")
	if penya > 0 {
		fe.stats.PenyaGained += int64(penya)
		fe.appendLootFeed(models.FarmLootFeedItem{
			Timestamp: nowStr,
			ItemName:  fmt.Sprintf("%d Penya", int64(penya)),
			Count:     int(penya),
			IsPenya:   true,
		})
	}

	for _, itRaw := range itemsRaw {
		if itArr, ok := itRaw.([]interface{}); ok && len(itArr) >= 2 {
			itemID, _ := itArr[0].(string)
			count := int(toFloat32(itArr[1]))
			name := itemID
			rarity := 0

			fe.appendLootFeed(models.FarmLootFeedItem{
				Timestamp: nowStr,
				ItemName:  name,
				ItemID:    itemID,
				Count:     count,
				Rarity:    rarity,
				IsPenya:   false,
			})

			// Trigger Discord alert on rare loot
			if fe.config.WebhookAlertOnRareDrop && rarity >= 2 && fe.config.DiscordWebhookURL != "" {
				go fe.sendDiscordWebhook(fmt.Sprintf("🌟 **Rare Drop Looted!**\nCharacter: `%s`\nItem: `%s` (x%d)",
					fe.bot.Profile.CharacterName, name, count), 0xffd700)
			}
		}
	}
}

func (fe *FarmEngine) appendLootFeed(item models.FarmLootFeedItem) {
	fe.stats.RecentLoot = append([]models.FarmLootFeedItem{item}, fe.stats.RecentLoot...)
	if len(fe.stats.RecentLoot) > 20 {
		fe.stats.RecentLoot = fe.stats.RecentLoot[:20]
	}
}

// OnExpGained updates farm stats when EXP is received
func (fe *FarmEngine) OnExpGained(amount int64) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.stats.ExpGained += amount
}

func (fe *FarmEngine) switchTarget(newMob *TrackedMob, reason string, botX, botZ float32) bool {
	botID := fe.getBotID()
	mapName := fe.getMap()
	chID := fe.getChannelID()
	coord := GetGlobalCoordinator()

	fe.mu.RLock()
	coordEnabled := fe.config.CoordinatedFarm
	fe.mu.RUnlock()

	// Multi-bot coordinator claim check
	if coordEnabled && botID != "" && newMob != nil {
		if reason == "aggro-switch" {
			coord.ClaimShared(mapName, chID, newMob.ID, botID)
		} else {
			if !coord.Claim(mapName, chID, newMob.ID, botID) {
				if fe.bot != nil {
					fe.bot.emitLog("target", fmt.Sprintf("[COORD] Mob #%d (%s) claim denied (claimed by another bot)", newMob.ID, newMob.Name), "debug")
				}
				return false
			}
		}
	}

	fe.mu.Lock()
	oldMob := fe.currentTarget
	fe.currentTarget = newMob
	fe.targetStartTime = fe.now()
	fe.targetSwingsWithoutHit = 0
	if newMob != nil {
		fe.targetLastHp = newMob.HP
	}
	fe.mu.Unlock()

	// Release previous target lease if switching to another mob
	if coordEnabled && botID != "" && oldMob != nil && (newMob == nil || oldMob.ID != newMob.ID) {
		coord.Release(mapName, chID, oldMob.ID, botID, reason)
	}

	oldID := 0
	oldHP := 0
	oldChase := false
	if oldMob != nil {
		oldID = oldMob.ID
		oldHP = oldMob.HP
		oldChase = oldMob.Chase
	}

	newID := 0
	newHP := 0
	newChase := false
	var dist float32
	if newMob != nil {
		newID = newMob.ID
		newHP = newMob.HP
		newChase = newMob.Chase
		dx := newMob.X - botX
		dz := newMob.Z - botZ
		dist = float32(math.Sqrt(float64(dx*dx + dz*dz)))
	}

	if fe.bot != nil {
		fe.bot.emitLog("target", fmt.Sprintf("[TARGET] reason=%s old=#%d(hp=%d,chase=%t) new=#%d(hp=%d,chase=%t,dist=%.1fm)",
			reason, oldID, oldHP, oldChase, newID, newHP, newChase, dist), "info")
	}
	return true
}

func (fe *FarmEngine) clearTarget(reason string) {
	fe.mu.Lock()
	oldMob := fe.currentTarget
	if oldMob == nil {
		fe.mu.Unlock()
		return
	}
	fe.currentTarget = nil
	fe.attackPhase = 0
	fe.swingTargetID = 0
	fe.targetSwingsWithoutHit = 0
	fe.targetStartTime = time.Time{}
	fe.postKillWait = fe.now().Add(500 * time.Millisecond)
	coordEnabled := fe.config.CoordinatedFarm
	fe.mu.Unlock()

	botID := fe.getBotID()
	if coordEnabled && botID != "" {
		mapName := fe.getMap()
		chID := fe.getChannelID()
		coord := GetGlobalCoordinator()
		if reason == "d" || reason == "despawn" {
			coord.ReleaseMobAllOwners(mapName, chID, oldMob.ID, reason)
		} else {
			coord.Release(mapName, chID, oldMob.ID, botID, reason)
		}
	}

	fe.setState("Looting")
	if fe.bot != nil {
		fe.bot.ClearWaypoint()

		fe.bot.mu.Lock()
		isLegit := fe.bot.Profile.Preferences.LegitMode
		fe.bot.mu.Unlock()

		if isLegit {
			fe.bot.BroadcastState("astand", 0)
		}

		fe.bot.emitLog("target", fmt.Sprintf("[TARGET] reason=%s old=#%d(hp=%d,chase=%t) new=none",
			reason, oldMob.ID, oldMob.HP, oldMob.Chase), "info")
	}
}

func (fe *FarmEngine) onTargetKilled() {
	fe.clearTarget("d")
}

// OnMobKilledByID handles monster death confirmed by server loot or event packet
func (fe *FarmEngine) OnMobKilledByID(mobID int) {
	fe.recordHpProgress()
	fe.mobsMu.Lock()
	if mob, exists := fe.mobs[mobID]; exists {
		mob.Dead = true
		mob.HP = 0
		mob.Chase = false
	}
	fe.mobsMu.Unlock()

	botID := fe.getBotID()
	fe.mu.RLock()
	coordEnabled := fe.config.CoordinatedFarm
	fe.mu.RUnlock()
	if coordEnabled && botID != "" {
		GetGlobalCoordinator().ReleaseMobAllOwners(fe.getMap(), fe.getChannelID(), mobID, "killed_by_id")
	}

	fe.mu.Lock()
	if fe.currentTarget != nil && fe.currentTarget.ID == mobID {
		fe.mu.Unlock()
		fe.clearTarget("d")
		return
	}
	fe.mu.Unlock()
}

// pruneMobs cleans up dead mobs, stale unverified mobs, and expired shadow mobs.
func (fe *FarmEngine) pruneMobs(now time.Time) {
	fe.mobsMu.Lock()
	defer fe.mobsMu.Unlock()

	leadInst, pbCfg, isMember := fe.getPartyLeaderInstance()
	var leadTgtID int
	if isMember && pbCfg.FocusFire && leadInst != nil && leadInst.Farm != nil {
		leadInst.Farm.mu.RLock()
		if leadInst.Farm.currentTarget != nil && !leadInst.Farm.currentTarget.Dead && leadInst.Farm.currentTarget.HP > 0 {
			leadTgtID = leadInst.Farm.currentTarget.ID
		}
		leadInst.Farm.mu.RUnlock()
	}

	fe.mu.RLock()
	curTargetID := 0
	if fe.currentTarget != nil {
		curTargetID = fe.currentTarget.ID
	}
	fe.mu.RUnlock()

	for id, m := range fe.mobs {
		// 1. Shadow mobs: prune immediately if leader dropped or switched target
		if m.IsShadow && !m.ServerVerified {
			if !isMember || !pbCfg.FocusFire || leadTgtID != id {
				delete(fe.mobs, id)
				continue
			}
		}
		// 2. Confirmed dead mobs: prune after 3 seconds
		if m.Dead && now.Sub(m.LastSeen) > 3*time.Second {
			delete(fe.mobs, id)
			continue
		}
		// 3. Stale mobs not seen in server packets ("m", "j", "s") for > 15 seconds
		if !m.IsShadow && !m.LastSeen.IsZero() && now.Sub(m.LastSeen) > 15*time.Second {
			if curTargetID != id {
				delete(fe.mobs, id)
				continue
			}
		}
	}
}

// thinkHunt executes attacker hunting loop
func (fe *FarmEngine) thinkHunt(cfg models.FarmConfig) {
	fe.bot.mu.Lock()
	botX := fe.bot.Stats.PosX
	botY := fe.bot.Stats.PosY
	botZ := fe.bot.Stats.PosZ
	botLevel := int(fe.bot.Stats.Level)
	botJob := strings.ToLower(fe.bot.Stats.Job)
	fe.bot.mu.Unlock()

	now := fe.now()
	fe.pruneMobs(now)

	// Pause briefly after kill to loot and let animations show naturally
	if now.Before(fe.postKillWait) {
		return
	}

	// Level Up check for Leader / Solo bots in Smart Mode: re-evaluate optimal +14 spawn and transition
	if fe.lastBotLevel == 0 {
		fe.lastBotLevel = botLevel
	} else if botLevel > fe.lastBotLevel {
		oldLv := fe.lastBotLevel
		fe.lastBotLevel = botLevel
		if cfg.SmartMode && (fe.bot.IsPartyLeader() || !fe.bot.IsInParty()) {
			effLv := fe.getEffectiveLevel(cfg)
			if sp := fe.findOptimalSpawn(effLv, botX, botZ, cfg, now); sp != nil {
				fe.setAnchor(sp.CX, sp.CZ)
				fe.bot.SetWaypoint(sp.CX, sp.CZ)
				fe.clearTarget("level-up-roam")
				fe.bot.emitLog("farm", fmt.Sprintf("Level Up! (Lv %d -> %d) Roaming to optimal +14 spawn (%s, Lv %d) at (%.1f, %.1f)",
					oldLv, botLevel, farmMonsters[sp.MobID].Name, farmMonsters[sp.MobID].Level, sp.CX, sp.CZ), "info")
				return
			}
		}
	}

	// Support Regroup Guard on Leader Bot: pause up to 3s if support is 16m-60m away
	if fe.bot.IsPartyLeader() {
		if pbCfg, ok := fe.bot.getPartyBoostConfig(); ok && pbCfg.Enabled && pbCfg.AutoSupport {
			var laggingSupportName string
			var laggingDist float32
			fe.bot.partyMu.RLock()
			pbMgr := fe.bot.partyBoost
			fe.bot.partyMu.RUnlock()
			if pbMgr != nil && pbMgr.botManager != nil {
				for _, mName := range pbCfg.SelectedMembers {
					if strings.EqualFold(mName, pbCfg.LeaderName) {
						continue
					}
					memInst := pbMgr.botManager.FindInstanceByName(mName)
					if memInst == nil || memInst.Status != "online" || memInst.Farm == nil {
						continue
					}
					memInst.mu.Lock()
					mJob := strings.ToLower(memInst.Stats.Job)
					mx, mz := memInst.Stats.PosX, memInst.Stats.PosZ
					memInst.mu.Unlock()
					isAssist := strings.Contains(mJob, "assist") || strings.Contains(mJob, "ringmaster") || strings.Contains(mJob, "billposter")
					if isAssist || memInst.Farm.GetConfig().Mode == "support" {
						d := dist2D(botX, botZ, mx, mz)
						if d > 16.0 && d <= 60.0 {
							laggingSupportName = mName
							laggingDist = d
							break
						}
					}
				}
			}
			if laggingSupportName != "" {
				if fe.supportWaitStart.IsZero() {
					fe.supportWaitStart = now
					fe.setState(fmt.Sprintf("Support Regroup: Waiting for support %s (%.1fm away)", laggingSupportName, laggingDist))
					fe.bot.ClearWaypoint()
					return
				} else if now.Sub(fe.supportWaitStart) < 3*time.Second {
					fe.setState(fmt.Sprintf("Support Regroup: Waiting for support %s (%.1fm away)", laggingSupportName, laggingDist))
					fe.bot.ClearWaypoint()
					return
				}
			} else {
				fe.supportWaitStart = time.Time{}
			}
		}
	}

	// Party Boost Member Synchronization: Dynamic Anchor, Hard Focus Fire, & Formation Follow
	leadInst, pbCfg, isMember := fe.getPartyLeaderInstance()
	if isMember {
		leadSummary := leadInst.Summary()
		lx := leadSummary.Stats.PosX
		ly := leadSummary.Stats.PosY
		lz := leadSummary.Stats.PosZ
		cfg.AnchorX, cfg.AnchorY, cfg.AnchorZ = lx, ly, lz
		fe.mu.Lock()
		fe.config.AnchorX, fe.config.AnchorY, fe.config.AnchorZ = lx, ly, lz
		fe.mu.Unlock()

		leadInst.Farm.mu.RLock()
		leadTgt := leadInst.Farm.currentTarget
		leadInst.Farm.mu.RUnlock()
		leadHasTarget := (leadTgt != nil && !leadTgt.Dead && leadTgt.HP > 0)

		// Hard Focus-Fire Target Override & Cross-Instance Target Shadowing
		if pbCfg.FocusFire && leadHasTarget {
			fe.mu.RLock()
			cur := fe.currentTarget
			fe.mu.RUnlock()
			if cur == nil || cur.ID != leadTgt.ID {
				fe.mobsMu.Lock()
				shadowMob, exists := fe.mobs[leadTgt.ID]
				if !exists || shadowMob == nil {
					shadowMob = &TrackedMob{
						ID:             leadTgt.ID,
						Name:           leadTgt.Name,
						Level:          leadTgt.Level,
						X:              leadTgt.X,
						Y:              leadTgt.Y,
						Z:              leadTgt.Z,
						HP:             leadTgt.HP,
						MaxHP:          leadTgt.MaxHP,
						Dead:           false,
						IsShadow:       true,
						ServerVerified: false,
						LastSeen:       now,
					}
					fe.mobs[leadTgt.ID] = shadowMob
				} else {
					shadowMob.X = leadTgt.X
					shadowMob.Y = leadTgt.Y
					shadowMob.Z = leadTgt.Z
					shadowMob.HP = leadTgt.HP
					shadowMob.MaxHP = leadTgt.MaxHP
					shadowMob.Dead = false
					shadowMob.LastSeen = now
				}
				fe.mobsMu.Unlock()
				fe.switchTarget(shadowMob, "focus-fire", botX, botZ)
			}
		}

		// When Focus Fire is active and Leader has no combat target:
		// Member MUST NOT engage phantom/solo mobs - follow leader in tight formation and wait!
		if pbCfg.FocusFire && !leadHasTarget {
			if fe.currentTarget != nil {
				fe.clearTarget("leader-idle")
			}
			distToLead := dist2D(botX, botZ, lx, lz)
			if distToLead > 4.0 {
				fe.setState(fmt.Sprintf("Following Leader (%s - %.1fm)", pbCfg.LeaderName, distToLead))
				fe.bot.SetWaypoint(lx, lz)
			} else {
				if _, _, hasWp := fe.bot.GetWaypoint(); hasWp {
					fe.bot.ClearWaypoint()
				}
				fe.setState(fmt.Sprintf("With Leader (%s)", pbCfg.LeaderName))
			}
			return
		}

		// Tight Formation Follow When Leader Moves (Leader has no combat target)
		if !leadHasTarget {
			distToLead := dist2D(botX, botZ, lx, lz)
			if distToLead > 6.0 {
				if fe.currentTarget != nil {
					fe.clearTarget("follow-leader")
				}
				fe.setState(fmt.Sprintf("Following Leader (%s - %.1fm)", pbCfg.LeaderName, distToLead))
				fe.bot.SetWaypoint(lx, lz)
				return
			} else if distToLead <= 4.0 {
				if _, _, hasWp := fe.bot.GetWaypoint(); hasWp {
					fe.bot.ClearWaypoint()
				}
			}
		}
	}

	// Multi-bot watchdog: detect zero HP progress / stall
	watchdogSec := cfg.WatchdogSec
	if watchdogSec <= 0 {
		watchdogSec = 90
	}
	watchdogThreshold := time.Duration(watchdogSec) * time.Second

	if cfg.CoordinatedFarm && fe.getBotID() != "" {
		lastProgress := fe.getLastHpProgressTime()
		if lastProgress.IsZero() {
			fe.recordHpProgress()
		} else if now.Sub(lastProgress) > watchdogThreshold {
			if fe.bot != nil {
				fe.bot.emitLog("coord", fmt.Sprintf("[COORD] Watchdog triggered: 0 HP progress for %v. Forcing rebalance.",
					now.Sub(lastProgress).Round(time.Second)), "warn")
			}

			fe.mu.RLock()
			curTarget := fe.currentTarget
			fe.mu.RUnlock()
			if curTarget != nil {
				fe.blacklistMob(curTarget.ID, 45*time.Second)
				fe.clearTarget("watchdog_stall")
			}

			if cfg.SmartMode {
				mapName := fe.getMap()
				chID := fe.getChannelID()
				if sp, changed := GetGlobalCoordinator().AllocateZone(mapName, chID, fe.getBotID(), botLevel, botX, botZ, cfg.SmartMinLevelOffset, cfg.SmartMaxLevelOffset, true); sp != nil && changed {
					fe.mu.Lock()
					fe.config.AnchorX = sp.CX
					fe.config.AnchorZ = sp.CZ
					fe.mu.Unlock()
					if fe.bot != nil {
						fe.bot.SetWaypoint(sp.CX, sp.CZ)
						meta := farmMonsters[sp.MobID]
						fe.bot.emitLog("coord", fmt.Sprintf("[COORD] Watchdog rebalanced bot to %s (Lv %d) at (%.1f, %.1f)",
							meta.Name, meta.Level, sp.CX, sp.CZ), "info")
					}
				}
			}

			fe.recordHpProgress()
			return
		}
	}

	// Leash check: Return to anchor if pulled too far, or continue travelling to target spawn
	dx := botX - cfg.AnchorX
	dz := botZ - cfg.AnchorZ
	distFromAnchor := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	if distFromAnchor > cfg.Radius+8.0 && (!isMember || fe.currentTarget == nil) {
		fe.noTargetStartTime = time.Time{}
		if cfg.SmartMode && now.Before(fe.relocatePausedUntil) {
			fe.setState("Searching paused (max relocation attempts reached, waiting for respawn)")
			fe.bot.ClearWaypoint()
			return
		}

		if cfg.SmartMode && distFromAnchor > 45.0 {
			fe.setState(fmt.Sprintf("Travelling to spawn (%.0fm left)", distFromAnchor))
		} else {
			fe.setState("Returning to Anchor")
		}

		if cfg.SmartMode {
			if fe.travelStartTime.IsZero() {
				fe.travelStartTime = now
				fe.lastTravelDist = distFromAnchor
				fe.lastTravelCheckTime = now
			} else if now.Sub(fe.lastTravelCheckTime) >= ClientSmartTravelStuckTimeout {
				if fe.lastTravelDist-distFromAnchor < 2.0 {
					fe.cooldownAreaSpawns(cfg.AnchorX, cfg.AnchorZ, cfg.Radius, now)
					fe.travelStartTime = time.Time{}
					fe.bot.emitLog("farm", fmt.Sprintf("Smart Hunt: Travel stuck towards (%.1f, %.1f) (< 2m in %v). Blacklisting area and relocating.",
						cfg.AnchorX, cfg.AnchorZ, ClientSmartTravelStuckTimeout), "warn")
					if nextSpawn := fe.findRelocationSpawn(botLevel, botX, botZ, cfg, now); nextSpawn != nil {
						fe.setAnchor(nextSpawn.CX, nextSpawn.CZ)
						fe.bot.SetWaypoint(nextSpawn.CX, nextSpawn.CZ)
						return
					}
					fe.relocateAttempts = ClientSmartRelocateMaxAttempts
					fe.relocatePausedUntil = now.Add(ClientSmartEmptyAreaCooldown)
					fe.setState("Searching paused (all candidate areas empty, retrying soon)")
					fe.bot.ClearWaypoint()
					return
				}
				fe.lastTravelDist = distFromAnchor
				fe.lastTravelCheckTime = now
			}
		}

		fe.bot.SetWaypoint(cfg.AnchorX, cfg.AnchorZ)
		return
	}
	fe.travelStartTime = time.Time{}

	// 1. Aggro-switch mirroring bot.gd:136-137:
	// attackers = living mobs with Chase && horizontal dist < 20.0
	var attackers []*TrackedMob
	fe.mobsMu.RLock()
	for _, m := range fe.mobs {
		if m.Dead || m.HP <= 0 || !m.Chase || m.IsShadow {
			continue
		}
		if _, blacklisted := fe.blacklist[m.ID]; blacklisted {
			continue
		}
		if math.Abs(float64(m.Y-botY)) > 5.0 {
			continue
		}
		adx := m.X - botX
		adz := m.Z - botZ
		hDist := float32(math.Sqrt(float64(adx*adx + adz*adz)))
		if hDist < 20.0 {
			attackers = append(attackers, m)
		}
	}
	fe.mobsMu.RUnlock()

	targetInAttackers := false
	if fe.currentTarget != nil && !fe.currentTarget.Dead {
		for _, att := range attackers {
			if att.ID == fe.currentTarget.ID {
				targetInAttackers = true
				break
			}
		}
	}

	// bot.gd:136: if not attackers.is_empty() and (p.target_mon == null or not p.target_mon in attackers):
	//     _engage(_nearest(attackers))
	if len(attackers) > 0 && (fe.currentTarget == nil || !targetInAttackers) {
		var nearest *TrackedMob
		nearestDist := float32(math.MaxFloat32)
		for _, att := range attackers {
			adx := att.X - botX
			adz := att.Z - botZ
			d := float32(math.Sqrt(float64(adx*adx + adz*adz)))
			if d < nearestDist {
				nearestDist = d
				nearest = att
			}
		}
		if nearest != nil && (fe.currentTarget == nil || nearest.ID != fe.currentTarget.ID) {
			fe.switchTarget(nearest, "aggro-switch", botX, botZ)
		}
	}

	// Focus Fire Leader's target if enabled in Party Boost
	if pbCfg, ok := fe.bot.getPartyBoostConfig(); ok && pbCfg.Enabled && pbCfg.FocusFire && !fe.bot.IsPartyLeader() {
		if leadMob := fe.findLeaderTargetMob(pbCfg.LeaderName, botX, botZ, cfg.Radius); leadMob != nil {
			if fe.currentTarget == nil || (fe.currentTarget.ID != leadMob.ID && fe.currentTarget.HP == fe.currentTarget.MaxHP && fe.attackPhase == 0) {
				fe.switchTarget(leadMob, "focus-fire", botX, botZ)
			}
		}
	}

	// 2. Process current target
	if fe.currentTarget != nil {
		target := fe.currentTarget
		if target.Dead || target.HP <= 0 {
			fe.clearTarget("dead")
			return
		}

		// KS avoidance: if another player claimed or damaged target while we were approaching
		if cfg.AvoidKillSteal && !target.BotDamaged {
			myID := fe.bot.GetEntityID()
			if (target.ClaimedByPlayerID != 0 && target.ClaimedByPlayerID != myID) ||
				(target.TargetPlayerID != 0 && target.TargetPlayerID != myID) ||
				(target.HP < target.MaxHP && target.ClaimedByPlayerID != myID) {
				fe.bot.emitLog("farm", fmt.Sprintf("Target %s engaged by another player - skipping to avoid KS", target.Name), "info")
				fe.blacklistMob(target.ID, 15*time.Second)
				fe.clearTarget("ks")
				fe.bot.ClearWaypoint()
				return
			}
		}

		// Consecutive miss / unresponsive server check:
		// If swung 6 times at target with zero server damage packets or confirmations, target is invalid/ghost
		if fe.targetSwingsWithoutHit >= 6 {
			fe.blacklistMob(target.ID, 45*time.Second)
			fe.bot.emitLog("farm", fmt.Sprintf("Target %s (#%d) rejected by server (6 swings, 0 hits) - blacklisting ghost mob", target.Name, target.ID), "warn")
			fe.mobsMu.Lock()
			delete(fe.mobs, target.ID)
			fe.mobsMu.Unlock()
			fe.clearTarget("ghost_unresponsive")
			return
		}

		// Anti-stuck check: if fighting target for > 12s without damage change, blacklist (bot.gd:321-323)
		if fe.targetStartTime.IsZero() {
			fe.targetStartTime = now
			fe.targetLastHp = target.HP
		} else if target.HP < fe.targetLastHp {
			fe.targetLastHp = target.HP
			fe.targetStartTime = now
		} else if now.Sub(fe.targetStartTime) > 12*time.Second {
			fe.blacklistMob(target.ID, 40*time.Second)
			fe.bot.emitLog("farm", fmt.Sprintf("Can't damage %s - blacklisting for 40s.", target.Name), "warn")
			fe.clearTarget("stuck")
			return
		}

		// Calculate horizontal distance to target (matching player.gd:419-420: to.y = 0)
		tdx := target.X - botX
		tdz := target.Z - botZ
		dist2D := float32(math.Sqrt(float64(tdx*tdx + tdz*tdz)))

		// Dynamic combat reach matching player.gd:409 and stats.gd:221-226: 1.5 + radius + reach
		reach := CombatReach(botJob, 0.8)

		// Slope fix: horizontal arrival check only (player.gd:419-420).
		// Never block on 3D distance or vertical delta tdy!
		if dist2D > reach {
			fe.setState(fmt.Sprintf("Approaching %s (%.1fm)", target.Name, dist2D))
			if cfg.CoordinatedFarm && fe.getBotID() != "" {
				GetGlobalCoordinator().Renew(fe.getMap(), fe.getChannelID(), target.ID, fe.getBotID())
			}
			fe.bot.SetWaypoint(target.X, target.Z)
			return
		}

		// Elevation guard: If within horizontal reach but vertical distance is too large
		// (e.g. monster on cliff or flying above), cannot hit with ground attacks - skip
		tdy := float32(math.Abs(float64(target.Y - botY)))
		if tdy > 3.2 {
			fe.blacklistMob(target.ID, 30*time.Second)
			fe.bot.emitLog("farm", fmt.Sprintf("Target %s (#%d) is vertically unreachable (dy=%.1fm > 3.2m) - skipping", target.Name, target.ID, tdy), "warn")
			fe.clearTarget("elevation-unreachable")
			return
		}

		// Target is within attack reach: clear waypoint if still moving, face monster, and attack
		if _, _, hasWp := fe.bot.GetWaypoint(); hasWp {
			fe.bot.ClearWaypoint()
		}
		fe.setState(fmt.Sprintf("Fighting %s (HP: %d/%d)", target.Name, target.HP, target.MaxHP))
		if cfg.CoordinatedFarm && fe.getBotID() != "" {
			GetGlobalCoordinator().Renew(fe.getMap(), fe.getChannelID(), target.ID, fe.getBotID())
		}
		fe.executeAttack(cfg, target)
		return
	}

	// 3. Process ground loot (matching bot.gd:142 & 360-383)
	if cfg.AutoLoot && fe.tickLoot(cfg, botX, botY, botZ) {
		return
	}

	// 4. Pick next best monster (only runs when currentTarget == nil)
	var mob *TrackedMob
	if pbCfg, ok := fe.bot.getPartyBoostConfig(); ok && pbCfg.Enabled && pbCfg.FocusFire && !fe.bot.IsPartyLeader() {
		mob = fe.findLeaderTargetMob(pbCfg.LeaderName, botX, botZ, cfg.Radius)
		if mob == nil {
			// In Focus Fire, if leader has no combat target, member bot must wait and not pick solo mobs
			return
		}
	}
	if mob == nil {
		mob = fe.pickBestMob(cfg, botX, botY, botZ, botLevel)
	}
	if mob != nil {
		fe.noTargetStartTime = time.Time{}
		fe.relocateAttempts = 0
		fe.relocatePausedUntil = time.Time{}
		reason := "pick"
		if mob.Chase {
			reason = "aggro-switch"
		}
		if fe.switchTarget(mob, reason, botX, botZ) {
			fe.setState(fmt.Sprintf("Approaching %s", mob.Name))
			fe.bot.SetWaypoint(mob.X, mob.Z)
		}
	} else {
		if cfg.SmartMode {
			if fe.relocateAttempts >= ClientSmartRelocateMaxAttempts {
				if fe.relocatePausedUntil.IsZero() {
					fe.relocatePausedUntil = now.Add(ClientSmartEmptyAreaCooldown)
					fe.setState("Searching paused (max relocation attempts reached, waiting for respawn)")
					fe.bot.ClearWaypoint()
					return
				}
				if now.Before(fe.relocatePausedUntil) {
					fe.setState("Searching paused (max relocation attempts reached, waiting for respawn)")
					fe.bot.ClearWaypoint()
					return
				}
				fe.relocateAttempts = 0
				fe.relocatePausedUntil = time.Time{}
				fe.mu.Lock()
				fe.emptyAreaCooldown = make(map[string]time.Time)
				fe.mu.Unlock()
			}

			if distFromAnchor > cfg.Radius {
				if sp := fe.findOptimalSpawn(botLevel, botX, botZ, cfg, now); sp != nil {
					fe.setAnchor(sp.CX, sp.CZ)
					fe.bot.SetWaypoint(sp.CX, sp.CZ)
					meta := farmMonsters[sp.MobID]
					fe.bot.emitLog("farm", fmt.Sprintf("Smart Hunt: Relocating to %s (Lv %d) spawn at (%.1f, %.1f)",
						meta.Name, meta.Level, sp.CX, sp.CZ), "info")
					return
				}
			}

			if fe.noTargetStartTime.IsZero() {
				fe.noTargetStartTime = now
			} else if now.Sub(fe.noTargetStartTime) >= ClientSmartIdleRelocateTimeout {
				fe.cooldownAreaSpawns(cfg.AnchorX, cfg.AnchorZ, cfg.Radius, now)
				fe.relocateAttempts++
				nextSpawn := fe.findRelocationSpawn(botLevel, botX, botZ, cfg, now)
				if nextSpawn != nil {
					fe.setAnchor(nextSpawn.CX, nextSpawn.CZ)
					fe.bot.SetWaypoint(nextSpawn.CX, nextSpawn.CZ)
					fe.noTargetStartTime = time.Time{}
					meta := farmMonsters[nextSpawn.MobID]
					fe.bot.emitLog("farm", fmt.Sprintf("Smart Hunt: Area empty for %v (attempt %d/%d). Relocating to %s (Lv %d) at (%.1f, %.1f)",
						ClientSmartIdleRelocateTimeout, fe.relocateAttempts, ClientSmartRelocateMaxAttempts,
						meta.Name, meta.Level, nextSpawn.CX, nextSpawn.CZ), "info")
					return
				}

				fe.relocateAttempts = ClientSmartRelocateMaxAttempts
				fe.relocatePausedUntil = now.Add(ClientSmartEmptyAreaCooldown)
				fe.noTargetStartTime = time.Time{}
				fe.setState("Searching paused (all candidate areas empty, retrying soon)")
				fe.bot.ClearWaypoint()
				fe.bot.emitLog("farm", fmt.Sprintf("Smart Hunt: All eligible areas on cooldown. Pausing relocation for %v.",
					ClientSmartEmptyAreaCooldown), "warn")
				return
			}
			fe.setState("Searching for monsters...")
			fe.bot.ClearWaypoint()
			return
		}

		anchorX := cfg.AnchorX
		anchorZ := cfg.AnchorZ
		if anchorX == 0 && anchorZ == 0 {
			anchorX = botX
			anchorZ = botZ
		}
		adx := botX - anchorX
		adz := botZ - anchorZ
		if float32(math.Sqrt(float64(adx*adx+adz*adz))) > ClientLeashReturnDist {
			fe.setState("Returning to Anchor")
			fe.bot.SetWaypoint(anchorX, anchorZ)
		} else {
			fe.setState("Waiting for monsters...")
			fe.bot.ClearWaypoint()
		}
	}
}

// tickLoot navigates to and picks up nearby dropped loot matching bot.gd:360-383
func (fe *FarmEngine) tickLoot(cfg models.FarmConfig, botX, botY, botZ float32) bool {
	fe.dropsMu.Lock()
	defer fe.dropsMu.Unlock()

	now := fe.now()

	// 1. Process active drop
	if fe.currentDrop != nil {
		drop := fe.currentDrop
		if _, exists := fe.drops[drop.Key]; !exists {
			fe.currentDrop = nil
		} else {
			// Stuck timeout: 6.0s -> 60s blacklist (bot.gd:366)
			if now.Sub(fe.dropStartTime) > ClientLootStuckTimeout {
				fe.dropBlacklist[drop.Key] = now.Add(ClientLootBlacklistDuration)
				fe.currentDrop = nil
				delete(fe.drops, drop.Key)
				fe.bot.ClearWaypoint()
			} else {
				dx := drop.X - botX
				dz := drop.Z - botZ
				dist2D := float32(math.Sqrt(float64(dx*dx + dz*dz)))

				// Arrival reach: 1.2m (player.gd:414)
				if dist2D <= ClientLootArrivalReach {
					fe.bot.ClearWaypoint()
					fe.bot.BroadcastState("pickup", 0)
					delete(fe.drops, drop.Key)
					fe.currentDrop = nil
					return true
				}

				fe.setState("Looting")
				fe.bot.SetWaypoint(drop.X, drop.Z)
				return true
			}
		}
	}

	// 2. Pick nearest eligible drop (bot.gd:372-379)
	var bestDrop *TrackedDrop
	bestDist := float32(ClientLootPlayerDist) // 14.0m max player distance (bot.gd:372)

	anchorX := cfg.AnchorX
	anchorZ := cfg.AnchorZ
	if anchorX == 0 && anchorZ == 0 {
		anchorX = botX
		anchorZ = botZ
	}
	maxAnchorDist := cfg.Radius + ClientLootAnchorExtra // cfg.range + 6.0m (bot.gd:378)

	for _, drop := range fe.drops {
		if _, blacklisted := fe.dropBlacklist[drop.Key]; blacklisted {
			continue
		}
		// Skip drops older than 120s
		if now.Sub(drop.CreatedAt) > 120*time.Second {
			delete(fe.drops, drop.Key)
			continue
		}
		// Anchor range check (horizontal distance)
		adx := drop.X - anchorX
		adz := drop.Z - anchorZ
		anchorDist := float32(math.Sqrt(float64(adx*adx + adz*adz)))
		if anchorDist > maxAnchorDist {
			continue
		}

		// Player distance check (horizontal distance)
		pdx := drop.X - botX
		pdz := drop.Z - botZ
		playerDist := float32(math.Sqrt(float64(pdx*pdx + pdz*pdz)))
		if playerDist < bestDist {
			bestDist = playerDist
			bestDrop = drop
		}
	}

	if bestDrop != nil {
		fe.currentDrop = bestDrop
		fe.dropStartTime = now
		fe.bot.SetWaypoint(bestDrop.X, bestDrop.Z)
		fe.setState("Looting")
		return true
	}

	return false
}

// pickBestMob selects the most optimal target based on level offset, range, elevation, and blacklist
func (fe *FarmEngine) pickBestMob(cfg models.FarmConfig, botX, botY, botZ float32, botLevel int) *TrackedMob {
	now := fe.now()
	mapName := fe.getMap()
	chID := fe.getChannelID()
	botID := fe.getBotID()
	coord := GetGlobalCoordinator()

	fe.mobsMu.RLock()
	defer fe.mobsMu.RUnlock()

	myID := 0
	if fe.bot != nil {
		myID = fe.bot.GetEntityID()
	}
	avoidKS := cfg.AvoidKillSteal

	// Priority 1: Defend against aggressive mobs chasing player within 20m (matching bot.gd:332-337)
	var bestAttacker *TrackedMob
	bestAttackerDist := float32(math.MaxFloat32)
	for _, mob := range fe.mobs {
		if mob.Dead || mob.HP <= 0 || !mob.Chase || mob.IsShadow {
			continue
		}
		if _, blacklisted := fe.blacklist[mob.ID]; blacklisted {
			continue
		}
		// Multi-bot coordinator: if claimed by another bot and targeting that other bot, don't KS
		if cfg.CoordinatedFarm && botID != "" && coord.IsClaimedByOther(mapName, chID, mob.ID, botID) {
			if mob.TargetPlayerID != 0 && mob.TargetPlayerID != myID {
				continue
			}
		}
		if avoidKS {
			// Don't defend against mobs attacking another player
			if mob.TargetPlayerID != 0 && mob.TargetPlayerID != myID {
				continue
			}
			// Don't steal mobs claimed by another player
			if mob.ClaimedByPlayerID != 0 && mob.ClaimedByPlayerID != myID {
				continue
			}
		}
		// Vertical elevation check: skip mobs > 5.0m vertical distance
		if math.Abs(float64(mob.Y-botY)) > 5.0 {
			continue
		}
		dx := mob.X - botX
		dz := mob.Z - botZ
		dist2D := float32(math.Sqrt(float64(dx*dx + dz*dz)))
		if dist2D < ClientAggroRadius && dist2D < bestAttackerDist {
			bestAttackerDist = dist2D
			bestAttacker = mob
		}
	}
	if bestAttacker != nil {
		return bestAttacker
	}

	// Priority 2: Pick best target monster within level offset, anchor, and elevation
	var bestMob *TrackedMob
	bestDist := float32(math.MaxFloat32)

	effLevel := fe.getEffectiveLevel(cfg)
	minOffset := cfg.SmartMinLevelOffset
	if minOffset < 1 {
		minOffset = 1
	}
	maxOffset := cfg.SmartMaxLevelOffset
	if maxOffset < 1 {
		maxOffset = 14
	}
	targetLv := effLevel + maxOffset

	minLv1 := effLevel + minOffset
	maxLv1 := effLevel + maxOffset
	minLv2 := effLevel + 10
	maxLv2 := effLevel + 16
	minLv3 := effLevel + 1
	maxLv3 := effLevel + 16
	if effLevel <= 2 {
		minLv1 = 1
		minLv2 = 1
		minLv3 = 1
	}

	var bestB1Mob, bestB2Mob, bestB3Mob *TrackedMob
	bestB1Score := float32(math.MaxFloat32)
	bestB2Score := float32(math.MaxFloat32)
	bestB3Score := float32(math.MaxFloat32)

	minLv := botLevel - 5
	maxLv := botLevel + 5
	if minLv < 1 {
		minLv = 1
	}

	anchorX := cfg.AnchorX
	anchorZ := cfg.AnchorZ
	radius := cfg.Radius
	if anchorX == 0 && anchorZ == 0 {
		anchorX = botX
		anchorZ = botZ
	}
	if radius <= 0 {
		radius = 45.0
	}

	for _, mob := range fe.mobs {
		if mob.Dead || mob.HP <= 0 || mob.IsShadow {
			continue
		}
		if _, blacklisted := fe.blacklist[mob.ID]; blacklisted {
			continue
		}
		// Multi-bot coordinator check: skip mobs claimed by other bots on this channel
		if cfg.CoordinatedFarm && botID != "" && coord.IsClaimedByOther(mapName, chID, mob.ID, botID) {
			continue
		}
		if avoidKS {
			// Skip mobs claimed or damaged by another player within the last 12s
			if mob.ClaimedByPlayerID != 0 && mob.ClaimedByPlayerID != myID {
				if now.Sub(mob.ClaimedTime) < 12*time.Second {
					continue
				}
			}
			// Skip mobs currently chasing or targeting another player
			if mob.TargetPlayerID != 0 && mob.TargetPlayerID != myID {
				continue
			}
			// Skip mobs that have already taken damage from someone else
			if mob.HP < mob.MaxHP && !mob.BotDamaged {
				continue
			}
		}
		// Vertical elevation check: skip mobs > 5.0m vertical distance (matching reachable cliff/terrain limits)
		if math.Abs(float64(mob.Y-botY)) > 5.0 {
			continue
		}

		// Anchor distance check
		adx := mob.X - anchorX
		adz := mob.Z - anchorZ
		if float32(math.Sqrt(float64(adx*adx+adz*adz))) > radius {
			continue
		}

		// Player 3D distance check (accounts for height and slopes)
		dx := mob.X - botX
		dy := mob.Y - botY
		dz := mob.Z - botZ
		dist3D := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))

		if cfg.SmartMode {
			lvDiff := float32(math.Abs(float64(mob.Level - targetLv)))
			score := dist3D + lvDiff*40.0
			if mob.Level >= minLv1 && mob.Level <= maxLv1 {
				if score < bestB1Score {
					bestB1Score = score
					bestB1Mob = mob
				}
			}
			if mob.Level >= minLv2 && mob.Level <= maxLv2 {
				if score < bestB2Score {
					bestB2Score = score
					bestB2Mob = mob
				}
			}
			if mob.Level >= minLv3 && mob.Level <= maxLv3 {
				if score < bestB3Score {
					bestB3Score = score
					bestB3Mob = mob
				}
			}
		} else {
			if mob.Level >= minLv && mob.Level <= maxLv {
				if dist3D < bestDist {
					bestDist = dist3D
					bestMob = mob
				}
			}
		}
	}

	if cfg.SmartMode {
		if bestB1Mob != nil {
			return bestB1Mob
		}
		if bestB2Mob != nil {
			return bestB2Mob
		}
		return bestB3Mob
	}
	return bestMob
}

// executeAttack sends mhit or skill packets to damage the monster with authentic swing animations
func (fe *FarmEngine) executeAttack(cfg models.FarmConfig, mob *TrackedMob) {
	fe.bot.mu.Lock()
	isLegit := fe.bot.Profile.Preferences.LegitMode
	botX := fe.bot.Stats.PosX
	botZ := fe.bot.Stats.PosZ
	botLevel := fe.bot.Stats.Level
	fe.bot.mu.Unlock()

	// Direction and facing rotation towards target monster
	dx := mob.X - botX
	dz := mob.Z - botZ
	rot := float32(math.Atan2(float64(dx), float64(dz)))

	now := fe.now()

	// Phase 0: Start swing animation
	if fe.attackPhase == 0 {
		fe.swingTargetID = mob.ID
		fe.swingCounter = (fe.swingCounter % 4) + 1
		anim := fmt.Sprintf("atk%d", fe.swingCounter)
		fe.bot.BroadcastState(anim, rot)
		fe.swingStartTime = now
		fe.attackPhase = 1
		return
	}

	// Phase 1: Wait for weapon wind-up (~250ms, matching 45% hit fraction of animation)
	windup := 250 * time.Millisecond
	if !isLegit {
		windup = 100 * time.Millisecond
	}
	if now.Sub(fe.swingStartTime) < windup {
		return
	}

	// player.gd:457-458: if hit_at < 0.0 and target_mon and not target_mon.dead: _weapon_hit(...)
	fe.mu.RLock()
	curTarget := fe.currentTarget
	fe.mu.RUnlock()

	if curTarget == nil || curTarget.Dead || curTarget.HP <= 0 || curTarget.ID != fe.swingTargetID {
		fe.attackPhase = 0
		fe.swingTargetID = 0
		return
	}

	// Verify target is still within combat reach before sending mhit
	botJob := strings.ToLower(fe.bot.Stats.Job)
	wtdx := curTarget.X - botX
	wtdz := curTarget.Z - botZ
	wdist2D := float32(math.Sqrt(float64(wtdx*wtdx + wtdz*wtdz)))
	wreach := CombatReach(botJob, 0.8)
	if wdist2D > wreach+1.2 {
		fe.attackPhase = 0
		fe.swingTargetID = 0
		return
	}

	// EXPLOIT 1: Instant Kill Mobs (onekill)
	// Matches official Godot client movers.gd:704: dmg = maxi(dmg, mo.hp)
	// Sends exact remaining monster HP instead of 99999999 to bypass server anti-cheat ceiling
	if cfg.ExploitOneKill && (fe.bot == nil || !fe.bot.IsLevelSyncActive()) {
		killDmg := mob.HP
		if killDmg <= 0 {
			killDmg = mob.MaxHP
		}
		if killDmg <= 0 {
			killDmg = 120
		}
		pkt := map[string]interface{}{
			"t":  "mhit",
			"id": mob.ID,
			"d":  killDmg,
			"c":  0,
		}
		if ob, err := json.Marshal(pkt); err == nil {
			_ = fe.bot.ws.Send(ob)
			fe.bot.emitLog("combat", fmt.Sprintf("-> WS OUT [mhit] (1-Hit Kill) target=%d (%s) dmg=%d", mob.ID, mob.Name, killDmg), "info")
		}
		fe.targetSwingsWithoutHit++
		fe.bot.emitLog("combat", fmt.Sprintf("[SWING] mhit sent to #%d (currentTarget=#%d, lockedTarget=#%d)",
			mob.ID, curTarget.ID, fe.swingTargetID), "info")
		mob.BotDamaged = true
		mob.Chase = true
		mob.HP = 1
		fe.attackPhase = 0
		fe.swingTargetID = 0
		return
	}

	// EXPLOIT 2: Multi-Mob AoE Sweep (Damage all monsters in range simultaneously)
	if cfg.ExploitMultiSweep {
		fe.mobsMu.RLock()
		for _, m := range fe.mobs {
			if !m.Dead && m.HP > 0 {
				mdx := m.X - botX
				mdz := m.Z - botZ
				if float32(math.Sqrt(float64(mdx*mdx+mdz*mdz))) <= 16.0 {
					sweepDmg := m.HP
					if sweepDmg <= 0 {
						sweepDmg = m.MaxHP
					}
					pkt := map[string]interface{}{
						"t":  "mhit",
						"id": m.ID,
						"d":  sweepDmg,
						"c":  0,
					}
					if ob, err := json.Marshal(pkt); err == nil {
						_ = fe.bot.ws.Send(ob)
					}
				}
			}
		}
		fe.mobsMu.RUnlock()
	}

	// Normal Combat: calculate hit damage aligned with real client (175-180 for low levels)
	baseDmg := 150 + (int(botLevel)-1)*15 + rand.Intn(35)
	if baseDmg < 100 {
		baseDmg = 100
	}
	dmg := baseDmg
	critVal := 0
	if rand.Float32() < 0.20 {
		critVal = 1
		dmg = int(float32(dmg) * 1.5)
	}

	if fe.bot != nil && fe.bot.IsLevelSyncActive() {
		syncLv, isSync := fe.bot.GetLevelSync()
		if isSync && syncLv > 0 && syncLv < int(botLevel) {
			syncFactor := math.Pow(float64(syncLv)/float64(botLevel), 1.6)
			dmg = int(float64(dmg) * syncFactor)
			if dmg < 1 {
				dmg = 1
			}
		}
	}

	pkt := map[string]interface{}{
		"t":  "mhit",
		"id": mob.ID,
		"d":  dmg,
		"c":  critVal,
	}
	if ob, err := json.Marshal(pkt); err == nil {
		_ = fe.bot.ws.Send(ob)
		fe.bot.emitLog("combat", fmt.Sprintf("-> WS OUT [mhit] target=%d (%s) dmg=%d crit=%d", mob.ID, mob.Name, dmg, critVal), "info")
	}
	fe.targetSwingsWithoutHit++

	fe.bot.emitLog("combat", fmt.Sprintf("[SWING] mhit sent to #%d (currentTarget=#%d, lockedTarget=#%d)",
		mob.ID, curTarget.ID, fe.swingTargetID), "info")

	// movers.gd:661 rage(mo): set Chase = true locally on mhit
	mob.BotDamaged = true
	mob.Chase = true
	if myID := fe.bot.GetEntityID(); myID > 0 {
		mob.ClaimedByPlayerID = myID
	}

	fe.lastAttackTick = now

	// movers.gd:465: predicted HP clamped to >= 1 (keep swinging until server event 'd' arrives)
	if mob.HP > dmg {
		mob.HP -= dmg
	} else {
		mob.HP = 1
	}
	fe.attackPhase = 0
	fe.swingTargetID = 0
}

func (fe *FarmEngine) blacklistMob(id int, duration time.Duration) {
	fe.mobsMu.Lock()
	defer fe.mobsMu.Unlock()
	fe.blacklist[id] = fe.now().Add(duration)
}

// getPartyLeaderInstance checks if this bot is an active non-leader member in Party Boost,
// returning the online leader instance and active party configuration.
func (fe *FarmEngine) getPartyLeaderInstance() (*BotInstance, models.PartyBoostConfig, bool) {
	if fe.bot == nil {
		return nil, models.PartyBoostConfig{}, false
	}
	fe.bot.partyMu.RLock()
	mgr := fe.bot.partyBoost
	fe.bot.partyMu.RUnlock()
	if mgr == nil || mgr.botManager == nil {
		return nil, models.PartyBoostConfig{}, false
	}
	cfg := mgr.GetConfig()
	if !cfg.Enabled || cfg.LeaderName == "" || fe.bot.IsPartyLeader() {
		return nil, cfg, false
	}
	sum := fe.bot.Summary()
	charName := sum.CharacterName
	accName := sum.Name

	isMember := false
	for _, m := range cfg.SelectedMembers {
		if (charName != "" && strings.EqualFold(m, charName)) || (accName != "" && strings.EqualFold(m, accName)) {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, cfg, false
	}

	leadInst := mgr.botManager.FindInstanceByName(cfg.LeaderName)
	if leadInst == nil || leadInst.Status != "online" || leadInst.Farm == nil {
		return nil, cfg, false
	}
	return leadInst, cfg, true
}

// getEffectiveLevel calculates the target level for Smart Mode considering LevelSync and EXP share floor.
func (fe *FarmEngine) getEffectiveLevel(cfg models.FarmConfig) int {
	if fe.bot == nil {
		return 1
	}
	fe.bot.mu.Lock()
	botLevel := int(fe.bot.Stats.Level)
	fe.bot.mu.Unlock()

	fe.bot.partyMu.RLock()
	mgr := fe.bot.partyBoost
	fe.bot.partyMu.RUnlock()

	if mgr == nil || !mgr.GetConfig().Enabled || !fe.bot.IsPartyLeader() {
		return botLevel
	}

	// 1. LevelSync active: use Leader's synced level
	_, _, _, _, _, _, syncActive, syncLevel := fe.bot.GetPartyDetails()
	if syncActive && syncLevel > 0 {
		return syncLevel
	}

	// 2. LevelSync off: ensure chosen monster level respects party EXP share floor (maxPartyMemberLevel - 20)
	members := fe.bot.GetPartyMembers()
	if len(members) > 1 {
		minLevel := botLevel
		for _, m := range members {
			if m.Level > 0 && m.Level < minLevel {
				minLevel = m.Level
			}
		}

		maxOffset := cfg.SmartMaxLevelOffset
		if maxOffset < 1 {
			maxOffset = 14
		}
		maxAllowedMonsterLv := minLevel + 19
		if botLevel+maxOffset > maxAllowedMonsterLv {
			adjusted := maxAllowedMonsterLv - maxOffset
			if adjusted < minLevel {
				adjusted = minLevel
			}
			if adjusted < 1 {
				adjusted = 1
			}
			return adjusted
		}
	}

	return botLevel
}

// findLeaderTargetMob locates the party leader's current combat target mob.
// Cross-instance target shadowing: if the mob is not yet in member's radar or local radius, injects it.
func (fe *FarmEngine) findLeaderTargetMob(leaderName string, botX, botZ, radius float32) *TrackedMob {
	if fe.bot == nil {
		return nil
	}
	fe.bot.partyMu.RLock()
	mgr := fe.bot.partyBoost
	fe.bot.partyMu.RUnlock()
	if mgr == nil || mgr.botManager == nil {
		return nil
	}

	leaderInst := mgr.botManager.FindInstanceByName(leaderName)
	if leaderInst == nil || leaderInst.Status != "online" || leaderInst.Farm == nil {
		return nil
	}

	leaderInst.Farm.mu.RLock()
	leadTgt := leaderInst.Farm.currentTarget
	leaderInst.Farm.mu.RUnlock()
	if leadTgt == nil || leadTgt.Dead || leadTgt.HP <= 0 {
		if leadTgt != nil {
			fe.mobsMu.Lock()
			if m, ok := fe.mobs[leadTgt.ID]; ok && m.IsShadow && !m.ServerVerified {
				delete(fe.mobs, leadTgt.ID)
			}
			fe.mobsMu.Unlock()
		}
		return nil
	}

	leadID := leadTgt.ID
	now := fe.now()
	fe.mobsMu.Lock()
	defer fe.mobsMu.Unlock()
	mob, exists := fe.mobs[leadID]
	if !exists || mob == nil {
		// Cross-instance shadowing: inject mob
		mob = &TrackedMob{
			ID:             leadTgt.ID,
			Name:           leadTgt.Name,
			Level:          leadTgt.Level,
			X:              leadTgt.X,
			Y:              leadTgt.Y,
			Z:              leadTgt.Z,
			HP:             leadTgt.HP,
			MaxHP:          leadTgt.MaxHP,
			Dead:           false,
			IsShadow:       true,
			ServerVerified: false,
			LastSeen:       now,
		}
		fe.mobs[leadID] = mob
	} else {
		mob.X = leadTgt.X
		mob.Y = leadTgt.Y
		mob.Z = leadTgt.Z
		mob.HP = leadTgt.HP
		mob.MaxHP = leadTgt.MaxHP
		mob.Dead = leadTgt.Dead
		mob.LastSeen = now
	}

	return mob
}

// checkPlayerProximity inspects nearby players and relocates if an outsider is detected
func (fe *FarmEngine) checkPlayerProximity(cfg models.FarmConfig) bool {
	fe.bot.playersMu.RLock()
	players := make([]*RemotePlayer, 0, len(fe.bot.nearbyPlayers))
	for _, p := range fe.bot.nearbyPlayers {
		players = append(players, p)
	}
	fe.bot.playersMu.RUnlock()

	botX := fe.bot.Stats.PosX
	botZ := fe.bot.Stats.PosZ
	botLevel := int(fe.bot.Stats.Level)

	for _, p := range players {
		// Ignore party members / own bots
		if fe.bot.isPartyMember(p.ID) || strings.EqualFold(p.Name, fe.bot.Profile.CharacterName) {
			continue
		}

		dx := p.X - botX
		dz := p.Z - botZ
		dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))

		if dist <= cfg.PlayerDetectionRadius {
			fe.setState(fmt.Sprintf("Evacuating (Player nearby: %s, %.0fm)", p.Name, dist))

			// Send Discord Alert (throttled to 1 per 60s)
			if cfg.WebhookAlertOnPlayer && cfg.DiscordWebhookURL != "" && fe.now().Sub(fe.lastEvacAlert) > 60*time.Second {
				fe.lastEvacAlert = fe.now()
				go fe.sendDiscordWebhook(fmt.Sprintf("⚠️ **Player Detected in Vicinity!**\nBot: `%s`\nPlayer: `%s` (~%.0fm away)\nAction: Evacuating area.",
					fe.bot.Profile.CharacterName, p.Name, dist), 0xf1c40f)
			}

			// Find alternative spawn zone matching level bracket
			fe.relocateToFreeSpawn(botLevel, botX, botZ)
			return true
		}
	}

	return false
}

// findOptimalSpawn finds the nearest and most appropriate monster spawn on the Madrigal world map
// matching the target level offset (+1 to +14 above bot level).
func (fe *FarmEngine) findOptimalSpawn(botLevel int, botX, botZ float32, cfg models.FarmConfig, now time.Time) *SpawnMeta {
	fe.mu.RLock()
	cooldowns := make(map[string]time.Time, len(fe.emptyAreaCooldown))
	for k, v := range fe.emptyAreaCooldown {
		cooldowns[k] = v
	}
	fe.mu.RUnlock()

	if cfg.CoordinatedFarm && fe.getBotID() != "" {
		minOffset := cfg.SmartMinLevelOffset
		maxOffset := cfg.SmartMaxLevelOffset
		mapName := fe.getMap()
		chID := fe.getChannelID()
		force := false
		if !fe.lastSawSpawnMob.IsZero() && now.Sub(fe.lastSawSpawnMob) > 30*time.Second {
			force = true
		}
		if sp, _ := GetGlobalCoordinator().AllocateZone(mapName, chID, fe.getBotID(), botLevel, botX, botZ, minOffset, maxOffset, force); sp != nil {
			key := fe.spawnKey(sp)
			if until, onCooldown := cooldowns[key]; !onCooldown || !now.Before(until) {
				return sp
			}
		}
	}

	minOffset := cfg.SmartMinLevelOffset
	if minOffset < 1 {
		minOffset = 1
	}
	maxOffset := cfg.SmartMaxLevelOffset
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

	var bestB1, bestB2, bestB3 *SpawnMeta
	bestB1Score := float32(math.MaxFloat32)
	bestB2Score := float32(math.MaxFloat32)
	bestB3Score := float32(math.MaxFloat32)

	for i := range farmSpawns {
		sp := &farmSpawns[i]
		meta, exists := farmMonsters[sp.MobID]
		if !exists || meta.Fly || meta.Rank == "RANK_BOSS" {
			continue
		}

		key := fe.spawnKey(sp)
		if until, onCooldown := cooldowns[key]; onCooldown && now.Before(until) {
			continue
		}

		dx := sp.CX - botX
		dz := sp.CZ - botZ
		dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))

		lvDiff := float32(math.Abs(float64(meta.Level - targetLv)))
		score := dist + lvDiff*60.0

		if meta.Level >= minLv1 && meta.Level <= maxLv1 {
			if score < bestB1Score {
				bestB1Score = score
				bestB1 = sp
			}
		}
		if meta.Level >= minLv2 && meta.Level <= maxLv2 {
			if score < bestB2Score {
				bestB2Score = score
				bestB2 = sp
			}
		}
		if meta.Level >= minLv3 && meta.Level <= maxLv3 {
			if score < bestB3Score {
				bestB3Score = score
				bestB3 = sp
			}
		}
	}

	if bestB1 != nil {
		return bestB1
	}
	if bestB2 != nil {
		return bestB2
	}
	return bestB3
}

func (fe *FarmEngine) spawnKey(sp *SpawnMeta) string {
	return fmt.Sprintf("%s_%.0f_%.0f", sp.MobID, sp.CX, sp.CZ)
}

func (fe *FarmEngine) currentSpawnKey(anchorX, anchorZ float32) string {
	var bestSp *SpawnMeta
	bestDist := float32(math.MaxFloat32)
	for i := range farmSpawns {
		sp := &farmSpawns[i]
		dx := sp.CX - anchorX
		dz := sp.CZ - anchorZ
		d := float32(math.Sqrt(float64(dx*dx + dz*dz)))
		if d <= sp.Radius+15.0 && d < bestDist {
			bestDist = d
			bestSp = sp
		}
	}
	if bestSp != nil {
		return fe.spawnKey(bestSp)
	}
	return fmt.Sprintf("custom_%.0f_%.0f", anchorX, anchorZ)
}

func (fe *FarmEngine) cooldownAreaSpawns(anchorX, anchorZ, radius float32, now time.Time) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	cooldownExpiry := now.Add(ClientSmartEmptyAreaCooldown)
	matched := 0
	for i := range farmSpawns {
		sp := &farmSpawns[i]
		dx := sp.CX - anchorX
		dz := sp.CZ - anchorZ
		if float32(math.Sqrt(float64(dx*dx+dz*dz))) <= radius+sp.Radius+15.0 {
			fe.emptyAreaCooldown[fe.spawnKey(sp)] = cooldownExpiry
			matched++
		}
	}
	if matched == 0 {
		fe.emptyAreaCooldown[fmt.Sprintf("custom_%.0f_%.0f", anchorX, anchorZ)] = cooldownExpiry
	}
}

func (fe *FarmEngine) findRelocationSpawn(botLevel int, botX, botZ float32, cfg models.FarmConfig, now time.Time) *SpawnMeta {
	fe.mu.RLock()
	cooldowns := make(map[string]time.Time, len(fe.emptyAreaCooldown))
	for k, v := range fe.emptyAreaCooldown {
		cooldowns[k] = v
	}
	fe.mu.RUnlock()

	mapName := fe.getMap()
	chID := fe.getChannelID()
	botID := fe.getBotID()

	if cfg.CoordinatedFarm && botID != "" {
		sp, _ := GetGlobalCoordinator().AllocateZone(
			mapName, chID, botID, botLevel, botX, botZ,
			cfg.SmartMinLevelOffset, cfg.SmartMaxLevelOffset, true,
		)
		if sp != nil {
			adx := sp.CX - cfg.AnchorX
			adz := sp.CZ - cfg.AnchorZ
			distAnchor := float32(math.Sqrt(float64(adx*adx + adz*adz)))
			key := fe.spawnKey(sp)
			until, onCooldown := cooldowns[key]
			if distAnchor > cfg.Radius+30.0 && (!onCooldown || !now.Before(until)) {
				return sp
			}
		}
	}

	minOffset := cfg.SmartMinLevelOffset
	if minOffset < 1 {
		minOffset = 1
	}
	maxOffset := cfg.SmartMaxLevelOffset
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

	var bestB1, bestB2, bestB3 *SpawnMeta
	bestB1Score := float32(math.MaxFloat32)
	bestB2Score := float32(math.MaxFloat32)
	bestB3Score := float32(math.MaxFloat32)

	for i := range farmSpawns {
		sp := &farmSpawns[i]
		meta, exists := farmMonsters[sp.MobID]
		if !exists || meta.Fly || meta.Rank == "RANK_BOSS" {
			continue
		}

		adx := sp.CX - cfg.AnchorX
		adz := sp.CZ - cfg.AnchorZ
		if float32(math.Sqrt(float64(adx*adx+adz*adz))) <= cfg.Radius+30.0 {
			continue
		}

		key := fe.spawnKey(sp)
		if until, onCooldown := cooldowns[key]; onCooldown && now.Before(until) {
			continue
		}

		dx := sp.CX - botX
		dz := sp.CZ - botZ
		dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
		lvDiff := float32(math.Abs(float64(meta.Level - targetLv)))
		score := dist + lvDiff*60.0

		if meta.Level >= minLv1 && meta.Level <= maxLv1 {
			if score < bestB1Score {
				bestB1Score = score
				bestB1 = sp
			}
		}
		if meta.Level >= minLv2 && meta.Level <= maxLv2 {
			if score < bestB2Score {
				bestB2Score = score
				bestB2 = sp
			}
		}
		if meta.Level >= minLv3 && meta.Level <= maxLv3 {
			if score < bestB3Score {
				bestB3Score = score
				bestB3 = sp
			}
		}
	}

	if bestB1 != nil {
		return bestB1
	}
	if bestB2 != nil {
		return bestB2
	}
	return bestB3
}

// relocateToFreeSpawn finds another spawn zone for +1..+14 mobs with 0 nearby players
func (fe *FarmEngine) relocateToFreeSpawn(botLevel int, botX, botZ float32) {
	minLv := botLevel + 1
	maxLv := botLevel + 14

	var bestSpawn *SpawnMeta
	bestDist := float32(math.MaxFloat32)

	for i := range farmSpawns {
		sp := &farmSpawns[i]
		meta, exists := farmMonsters[sp.MobID]
		if !exists || meta.Level < minLv || meta.Level > maxLv {
			continue
		}

		dx := sp.CX - botX
		dz := sp.CZ - botZ
		dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))

		// Pick nearest suitable spot that isn't the current one (> 100m away)
		if dist > 100 && dist < bestDist {
			bestDist = dist
			bestSpawn = sp
		}
	}

	if bestSpawn != nil {
		fe.mu.Lock()
		fe.config.AnchorX = bestSpawn.CX
		fe.config.AnchorZ = bestSpawn.CZ
		fe.mu.Unlock()
		fe.bot.SetWaypoint(bestSpawn.CX, bestSpawn.CZ)
		fe.bot.emitLog("farm", fmt.Sprintf("Smart Evacuation: Moving anchor to spawn area (%s, Level %d) at (%.1f, %.1f)",
			farmMonsters[bestSpawn.MobID].Name, farmMonsters[bestSpawn.MobID].Level, bestSpawn.CX, bestSpawn.CZ), "info")
	}
}

// sendLuckPacket injects the {"t": "luck"} packet every 2 seconds
func (fe *FarmEngine) sendLuckPacket() {
	loops := 1
	chance := 50
	gold := 20
	fortune := true

	pkt := map[string]interface{}{
		"t":       "luck",
		"loops":   loops,
		"chance":  chance,
		"gold":    gold,
		"fortune": fortune,
	}

	if ob, err := json.Marshal(pkt); err == nil {
		_ = fe.bot.ws.Send(ob)
	}
}

// checkSupplies drinks potions or eats food when below thresholds (matching bot.gd:409-447)
func (fe *FarmEngine) checkSupplies(cfg models.FarmConfig) {
	fe.bot.mu.Lock()
	hp := int(fe.bot.Stats.HP)
	mp := int(fe.bot.Stats.MP)
	fp := int(fe.bot.Stats.FP)
	lvl := int(fe.bot.Stats.Level)
	sta := int(fe.bot.Stats.STA)
	intStat := int(fe.bot.Stats.INT)
	fe.bot.mu.Unlock()

	maxHp := CalcMaxHP(lvl, sta)
	maxMp := CalcMaxMP(lvl, intStat)
	maxFp := CalcMaxFP(lvl, sta)

	now := fe.now()

	// 1. HP Supply check (bot.gd:410-414)
	if maxHp > 0 {
		hpPct := float32(hp) / float32(maxHp) * 100.0
		if hpPct < cfg.HpThreshold && now.Sub(fe.lastPotionTick) >= ClientItemCooldown {
			missingHp := maxHp - hp
			if missingHp > 0 {
				if used, _ := fe.bot.UseOptimalSupply("DST_HP", missingHp); used {
					fe.lastPotionTick = now
				}
			}
		}
	}

	// 2. MP Supply check (bot.gd:415)
	if maxMp > 0 && cfg.MpThreshold > 0 {
		mpPct := float32(mp) / float32(maxMp) * 100.0
		if mpPct < cfg.MpThreshold && now.Sub(fe.lastMpTick) >= ClientItemCooldown {
			missingMp := maxMp - mp
			if missingMp > 0 {
				if used, _ := fe.bot.UseOptimalSupply("DST_MP", missingMp); used {
					fe.lastMpTick = now
				}
			}
		}
	}

	// 3. FP Supply check (bot.gd:416)
	if maxFp > 0 && cfg.FpThreshold > 0 {
		fpPct := float32(fp) / float32(maxFp) * 100.0
		if fpPct < cfg.FpThreshold && now.Sub(fe.lastFpTick) >= ClientItemCooldown {
			missingFp := maxFp - fp
			if missingFp > 0 {
				if used, _ := fe.bot.UseOptimalSupply("DST_FP", missingFp); used {
					fe.lastFpTick = now
				}
			}
		}
	}
}

// hasAttackers reports whether any living monster has Chase aggro within 20m (matching bot.gd:122)
func (fe *FarmEngine) hasAttackers() bool {
	if fe.bot == nil {
		return false
	}
	fe.bot.mu.Lock()
	botX := fe.bot.Stats.PosX
	botZ := fe.bot.Stats.PosZ
	fe.bot.mu.Unlock()

	fe.mobsMu.RLock()
	defer fe.mobsMu.RUnlock()
	for _, m := range fe.mobs {
		if m.Dead || !m.Chase {
			continue
		}
		adx := m.X - botX
		adz := m.Z - botZ
		hDist := float32(math.Sqrt(float64(adx*adx + adz*adz)))
		if hDist < 20.0 {
			return true
		}
	}
	return false
}

// checkRestState sits/rests when out of food or below rest threshold (matching bot.gd:124-129)
func (fe *FarmEngine) checkRestState(cfg models.FarmConfig) bool {
	if !cfg.AutoRest {
		fe.lastResting = false
		return false
	}

	// Exploits: Cap HP 1 and God Mode intentionally run at low/1 HP without resting
	if cfg.ExploitCapHp1 || cfg.ExploitGodMode {
		fe.lastResting = false
		return false
	}

	// Bot Preferences override: Disable Auto Rest
	if fe.bot != nil {
		fe.bot.mu.Lock()
		prefs := fe.bot.Profile.Preferences
		fe.bot.mu.Unlock()
		if prefs.DisableAutoRest {
			fe.lastResting = false
			return false
		}
	}

	fe.bot.mu.Lock()
	hp := int(fe.bot.Stats.HP)
	lvl := int(fe.bot.Stats.Level)
	sta := int(fe.bot.Stats.STA)
	fe.bot.mu.Unlock()

	maxHp := CalcMaxHP(lvl, sta)
	if maxHp <= 0 {
		return false
	}

	hpPct := float32(hp) / float32(maxHp) * 100.0
	hasAtt := fe.hasAttackers()

	if fe.lastResting {
		// Recovery threshold: 95.0% or attacked (bot.gd:126: if hp_pct >= 95.0 or not attackers.is_empty(): _resting = false)
		if hpPct >= ClientRestRecoveryThreshold || hasAtt {
			fe.lastResting = false
			return false
		}
		fe.setState(fmt.Sprintf("Resting (HP %.0f%%)", hpPct))
		return true
	}

	// Only enter resting if no attackers around (bot.gd:128)
	if !hasAtt && hpPct < cfg.RestHpThreshold {
		fe.lastResting = true
		fe.bot.ClearWaypoint()
		fe.setState(fmt.Sprintf("Resting (HP %.0f%%)", hpPct))
		return true
	}

	return false
}

// thinkSupport executes Assist / Ringmaster healer and buffer logic
func (fe *FarmEngine) thinkSupport(cfg models.FarmConfig) {
	fe.setState(fmt.Sprintf("Support Mode: Following %s", cfg.FollowTarget))

	// Follow party leader
	if cfg.FollowTarget != "" {
		fe.bot.playersMu.RLock()
		var leader *RemotePlayer
		for _, p := range fe.bot.nearbyPlayers {
			if strings.EqualFold(p.Name, cfg.FollowTarget) {
				leader = p
				break
			}
		}
		fe.bot.playersMu.RUnlock()

		if leader != nil {
			dx := leader.X - fe.bot.Stats.PosX
			dz := leader.Z - fe.bot.Stats.PosZ
			dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))

			if dist > cfg.FollowDist+1.5 {
				fe.bot.SetWaypoint(leader.X, leader.Z)
			} else {
				fe.bot.ClearWaypoint()
			}
		}
	}
}

func (fe *FarmEngine) setState(state string) {
	fe.mu.Lock()
	fe.stats.State = state
	bot := fe.bot
	fe.mu.Unlock()
	if bot != nil {
		bot.mu.Lock()
		bot.StatusMessage = state
		bot.mu.Unlock()
	}
}

func (fe *FarmEngine) updateStats() {
	mapName := fe.getMap()
	chID := fe.getChannelID()
	botID := fe.getBotID()

	fe.mu.Lock()
	defer fe.mu.Unlock()

	elapsedSec := int64(time.Since(fe.sessionStartTime).Seconds())
	if elapsedSec <= 0 {
		elapsedSec = 1
	}

	fe.stats.Running = fe.running
	fe.stats.SessionSec = elapsedSec
	fe.stats.Kills = fe.kills
	fe.stats.KillsPerHour = float32(fe.kills) / float32(elapsedSec) * 3600.0

	if fe.currentTarget != nil {
		fe.stats.TargetName = fe.currentTarget.Name
		fe.stats.TargetHp = fe.currentTarget.HP
		fe.stats.TargetMaxHp = fe.currentTarget.MaxHP
	} else {
		fe.stats.TargetName = ""
		fe.stats.TargetHp = 0
		fe.stats.TargetMaxHp = 0
	}

	if fe.config.CoordinatedFarm && botID != "" {
		coord := GetGlobalCoordinator()
		fe.stats.CoordClaimedMob = coord.GetClaimedMob(mapName, chID, botID)
		if asgn := coord.GetAssignedZone(mapName, chID, botID); asgn != nil {
			fe.stats.CoordZone = fmt.Sprintf("%s (%.0f, %.0f)", asgn.SpawnMobName, asgn.AnchorX, asgn.AnchorZ)
			fe.stats.CoordZoneMobs = asgn.SpawnMobID
			fe.stats.CoordAssignedAt = asgn.AssignedAt.Format("15:04:05")
		} else {
			fe.stats.CoordZone = ""
			fe.stats.CoordZoneMobs = ""
			fe.stats.CoordAssignedAt = ""
		}
	}
}

// GetStats returns current live farm statistics
func (fe *FarmEngine) GetStats() models.FarmStats {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	return fe.stats
}

// sendDiscordWebhook dispatches an embed notification to Discord
func (fe *FarmEngine) sendDiscordWebhook(content string, color int) {
	url := fe.config.DiscordWebhookURL
	if url == "" {
		return
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       "Mortis Farm Alert",
				"description": content,
				"color":       color,
				"footer": map[string]string{
					"text": "Mortis Automation Engine",
				},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(data)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := fe.bot.newGameHTTPClient().HTTPClient
	resp, err := client.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

// GhostModeBlockRecPut determines if rec_put should be dropped
func (fe *FarmEngine) GhostModeBlockRecPut() bool {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	return fe.config.GhostModeBlockRecPut
}
