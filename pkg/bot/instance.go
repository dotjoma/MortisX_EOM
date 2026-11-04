package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"madrigal-multibot/pkg/client"
	"madrigal-multibot/pkg/models"
	"madrigal-multibot/pkg/vpn"
)

type RemotePlayer struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	X         float32   `json:"x"`
	Y         float32   `json:"y"`
	Z         float32   `json:"z"`
	Rot       float32   `json:"rot"`
	LastSeen  time.Time `json:"last_seen"`
	SeenEpoch uint64    `json:"seen_epoch"`
	Source    string    `json:"source"`
}

type BotInstance struct {
	mu                 sync.Mutex
	ID                 string
	Profile            models.BotProfile
	Status             string // "disconnected", "connecting", "online", "error", "channel_full"
	StatusMessage      string
	Stats              models.BotRuntimeStats
	sessionToken       string
	accountKey         string
	accountDP          uint32
	accountVP          uint32
	hasPin             bool
	pinOk              bool
	charFileName       string
	charSlot           int
	saveFiles          map[string]string
	saveDirty          bool
	saveInFlight       bool
	saveTxMu           sync.Mutex
	activeClaims       int32
	lastAckedSave      map[string]string
	sessionBlacklist   map[string]bool
	preMutationInv     []interface{}
	preMutationPenya   float64
	lastMutatedIDs     []string
	lastSavedFiles     map[string]string
	sessionCtx         context.Context
	sessionCancel      context.CancelFunc
	automationRunning  atomic.Bool
	saveStatsRunning   atomic.Bool
	charCacheMu        sync.Mutex
	cachedRawJson      string
	cachedCharData     map[string]interface{}
	resyncing          bool
	saveRefusedShown    string
	saveStopped         bool
	saveFailCount       int
	lastSaveFailureTime time.Time
	resyncCount         int
	lastResyncTime      time.Time
	lastLootTime        time.Time
	combatT             time.Time // player.gd:57 (combat_t, set to 10s on being hit)
	httpCli            *client.GameHttpClient
	followTarget       string
	cancelCh           chan struct{}
	ws                 *client.GameWebSocket
	onUpdate           func(id string, stats models.BotRuntimeStats, status string, statusMsg string)
	onLog              func(botID, category, message, level string)
	onChat             func(botID, channel, sender, text, color string)
	onInventoryUpdated func(botID string, invData map[string]interface{})
	findOtherBot       func(name string) *models.BotSummary

	playersMu     sync.RWMutex
	nearbyPlayers map[int]*RemotePlayer
	playerNames   map[string]int // lowercase name -> ID

	scanEpoch      uint64
	probeWarpTime  time.Time
	isSweeping     bool
	lastSetPos     *models.WaypointCoord
	lastSetPosTime time.Time
	lastSetPosMu   sync.Mutex
	warpPacer      warpPacer

	waypointMu             sync.RWMutex
	waypointX              float32
	waypointZ              float32
	hasWaypoint            bool
	invisToggledByWaypoint bool
	waypointFlyMode        bool
	currentRot             float32

	pingMu      sync.Mutex
	pingSeq     int
	pingSent    map[int]time.Time
	pingHistory []uint32

	tradeMu      sync.Mutex
	tradeSession *TradeSession

	partyMu            sync.RWMutex
	inParty            bool
	partyID            int
	partyName          string
	partyLeaderID      int
	partyLeaderName    string
	partyLevel         int
	partyExp           int
	partyPoints        int
	partyKind          int
	partyShare         int
	partyDist          int
	partyModes         map[string]float64
	partyModesExpiry   map[string]time.Time
	partySkillCD       map[string]float64
	partySkillCDExpiry map[string]time.Time
	partyScrollFull    bool
	partyScrollExpUp   bool
	partyScrollSkillUp bool
	partyMembers       []models.PartyMember
	partyLeashed       bool
	partyBoost         *PartyBoostManager
	levelSyncActive    bool
	syncLevel          int
	lastPSetTime       map[string]time.Time
	lastScrollTime     map[string]time.Time

	entityID    int

	Farm        *FarmEngine
	telemetry   *TelemetryTracker
	vpnManager  *vpn.Manager
	vpnTunnelID string
}

type TradeSession struct {
	PartnerID   int
	PartnerName string
	SlotIndex   int
	Count       int
	Penya       int
	ItemsToGive []interface{}
	StateCh     chan string
}

func NewBotInstance(
	profile models.BotProfile,
	initialStats *models.BotRuntimeStats,
	onUpdate func(id string, stats models.BotRuntimeStats, status string, statusMsg string),
	onLog func(botID, category, message, level string),
	onChat func(botID, channel, sender, text, color string),
) *BotInstance {
	stats := models.DefaultStats()
	if initialStats != nil {
		stats = *initialStats
	}
	if profile.Preferences.AutoReconnectDelay <= 0 {
		profile.Preferences.AutoReconnectDelay = 14
	}
	bot := &BotInstance{
		ID:            profile.ID,
		Profile:       profile,
		Status:        "disconnected",
		StatusMessage: "Disconnected",
		Stats:         stats,
		charSlot:      -1,
		saveFiles:        make(map[string]string),
		lastSavedFiles:   make(map[string]string),
		lastAckedSave:    make(map[string]string),
		sessionBlacklist: make(map[string]bool),
		ws:             client.NewGameWebSocket(),
		onUpdate:      onUpdate,
		onLog:         onLog,
		onChat:        onChat,
		nearbyPlayers: make(map[int]*RemotePlayer),
		playerNames:   make(map[string]int),
		pingSent:      make(map[int]time.Time),
		pingHistory:   make([]uint32, 0, 5),
	}
	bot.telemetry = NewTelemetryTracker(bot.ID, bot.emitLog)
	bot.ws.SetLogger(func(dir string, payload []byte) {
		bot.telemetry.RecordWS(dir, payload)
	})
	bot.Farm = NewFarmEngine(bot)
	return bot
}

func (b *BotInstance) GetEntityID() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.entityID
}

func (b *BotInstance) SetLogsEnabled(enabled bool) {
	if b.telemetry != nil {
		b.telemetry.SetLogsEnabled(enabled)
	}
}

// GetMap returns the current map name for the bot based on current coordinates
func (b *BotInstance) GetMap() string {
	b.mu.Lock()
	x := b.Stats.PosX
	z := b.Stats.PosZ
	b.mu.Unlock()
	return DetermineMap(x, z)
}

// GetChannelID returns the connected channel number (default 2)
func (b *BotInstance) GetChannelID() int {
	b.mu.Lock()
	server := b.Profile.Server
	b.mu.Unlock()
	return ParseChannelID(server)
}

func (b *BotInstance) SetFindOtherBot(fn func(name string) *models.BotSummary) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.findOtherBot = fn
}

func (b *BotInstance) SetVPNManager(manager *vpn.Manager, tunnelID string) {
	b.mu.Lock()
	b.vpnManager = manager
	b.vpnTunnelID = tunnelID
	b.mu.Unlock()
}

// SetCombatTimer updates combat timer if t is later than current timer (player.gd:1391)
func (b *BotInstance) SetCombatTimer(t time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if t.After(b.combatT) {
		b.combatT = t
	}
}

// InCombat returns whether bot is currently in combat (player.gd:57, 1379)
func (b *BotInstance) InCombat(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return now.Before(b.combatT)
}

func (b *BotInstance) usesVPNLocked() (*vpn.Manager, string, bool) {
	// ponytail: VPN temporarily disabled until multi-tunnel routing stabilizes
	return nil, "", false
}

func (b *BotInstance) usesVPN() (*vpn.Manager, string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.usesVPNLocked()
}

type unavailableVPNTransport struct{}

func (unavailableVPNTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, vpn.ErrNotSetup
}

func (b *BotInstance) newGameHTTPClientLocked() *client.GameHttpClient {
	var cli *client.GameHttpClient
	manager, tunnelID, enabled := b.usesVPNLocked()
	if enabled {
		if manager == nil {
			cli = client.NewGameHttpClientWithHTTPClient(&http.Client{Transport: unavailableVPNTransport{}})
		} else {
			cli = client.NewGameHttpClientWithHTTPClient(manager.HTTPClient(tunnelID))
		}
	} else {
		cli = client.NewGameHttpClient()
	}
	if b.telemetry != nil {
		cli.OnSaveTelemetry = func(status int, latency time.Duration, reqSize int, reqHead string, respBody string, err error) {
			b.telemetry.RecordSave(status, latency, reqSize, reqHead, respBody, err)
		}
	}
	return cli
}

func (b *BotInstance) newGameHTTPClient() *client.GameHttpClient {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.newGameHTTPClientLocked()
}

func (b *BotInstance) currentNetworkDialer() (*net.Dialer, error) {
	manager, tunnelID, enabled := b.usesVPN()
	if !enabled {
		return nil, nil
	}
	if manager == nil {
		return nil, vpn.ErrNotSetup
	}
	return manager.Dialer(tunnelID), nil
}

func (b *BotInstance) connectWebSocket(ctx context.Context, endpoint string) error {
	dialer, err := b.currentNetworkDialer()
	if err != nil {
		return err
	}
	return b.ws.ConnectWithDialer(ctx, endpoint, dialer)
}

func (b *BotInstance) getScanConfig() models.ScanConfig {
	return models.DefaultScanConfig()
}

func (b *BotInstance) emitLog(category, message, level string) {
	if b.onLog != nil {
		b.onLog(b.ID, category, message, level)
	}
}

func (b *BotInstance) emitChat(channel, sender, text, color string) {
	if b.onChat != nil {
		b.onChat(b.ID, channel, sender, text, color)
	}
}

func (b *BotInstance) SetOnInventoryUpdated(cb func(botID string, invData map[string]interface{})) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onInventoryUpdated = cb
}

func (b *BotInstance) emitInventoryUpdate(invData map[string]interface{}) {
	b.mu.Lock()
	cb := b.onInventoryUpdated
	b.mu.Unlock()

	if cb == nil {
		return
	}

	if invData == nil {
		var err error
		invData, err = b.GetInventoryData()
		if err != nil {
			return
		}
	}

	cb(b.ID, invData)
}

func (b *BotInstance) emitUpdate(status, statusMsg string) {
	b.mu.Lock()
	b.Status = status
	b.StatusMessage = statusMsg
	stats := b.Stats
	b.mu.Unlock()

	if b.onUpdate != nil {
		b.onUpdate(b.ID, stats, status, statusMsg)
	}
}

func (b *BotInstance) Summary() models.BotSummary {
	b.mu.Lock()
	status := b.Status
	statusMsg := b.StatusMessage
	prof := b.Profile
	stats := b.Stats
	farm := b.Farm
	b.mu.Unlock()

	if status == "online" && farm != nil {
		fs := farm.GetStats()
		if fs.Running && fs.State != "" {
			statusMsg = fs.State
		}
	}

	return models.BotSummary{
		ID:            b.ID,
		Name:          prof.Name,
		Server:        prof.Server,
		CharacterName: prof.CharacterName,
		Status:        status,
		StatusMessage: statusMsg,
		Stats:         stats,
		Preferences:   prof.Preferences,
		Intervals:     prof.Intervals,
		Password:      prof.Password,
	}
}

func (b *BotInstance) GetName() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Profile.Name
}

func (b *BotInstance) IsOnline() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Status == "online"
}

func (b *BotInstance) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Status == "online" || b.Status == "connecting"
}

func (b *BotInstance) Start() error {
	b.mu.Lock()
	if b.Status == "online" || b.Status == "connecting" {
		b.mu.Unlock()
		return nil
	}

	b.Status = "connecting"
	b.StatusMessage = "Connecting..."
	if b.sessionCancel != nil {
		b.sessionCancel()
	}
	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	b.sessionCtx = sessionCtx
	b.sessionCancel = sessionCancel
	if b.cancelCh != nil {
		select {
		case <-b.cancelCh:
		default:
			close(b.cancelCh)
		}
	}
	b.cancelCh = make(chan struct{})
	b.mu.Unlock()

	if b.ws != nil {
		_ = b.ws.Close()
	}

	if b.telemetry != nil {
		b.telemetry.Start()
	}

	b.emitUpdate("connecting", fmt.Sprintf("Connecting to %s...", b.Profile.Server))
	b.emitLog("net", fmt.Sprintf("Connecting bot to server: %s...", b.Profile.Server), "info")

	endpoint := client.ResolveWSEndpoint(b.Profile.Server)
	sessionToken := ""

	// Refresh live credentials and character statistics if not mock
	if endpoint != "mock" && b.Profile.Password != "" {
		httpCli := b.newGameHTTPClient()
		t0 := time.Now()
		b.emitUpdate("connecting", "Authenticating account via HTTP...")
		b.emitLog("net", fmt.Sprintf("HTTP POST /api/login: Authenticating as %s...", b.Profile.Name), "info")
		auth, err := httpCli.LoginWithContext(sessionCtx, b.Profile.Name, b.Profile.Password)
		ping := uint32(time.Since(t0).Milliseconds())
		if ping == 0 {
			ping = 25
		}

		select {
		case <-sessionCtx.Done():
			b.mu.Lock()
			b.Status = "disconnected"
			b.StatusMessage = "Connection cancelled"
			b.mu.Unlock()
			return errors.New("connection cancelled")
		default:
		}

		if err != nil {
			statusType := "error"
			statusText := fmt.Sprintf("Login failed: %v", err)
			errLower := strings.ToLower(err.Error())
			if strings.Contains(errLower, "blocked") || strings.Contains(errLower, "banned") {
				statusType = "account_banned"
				statusText = "Account Banned"
			} else if isServerIssue(err) {
				statusType = "server_issue"
				statusText = "Server Issue (502 Bad Gateway)"
			}
			b.mu.Lock()
			b.Status = statusType
			b.StatusMessage = statusText
			b.mu.Unlock()
			b.emitUpdate(statusType, statusText)
			b.emitLog("system", fmt.Sprintf("Connection failed: %v", err), "error")
			return err
		}

		sessionToken = auth.Token

		b.mu.Lock()
		b.sessionToken = auth.Token
		targetChar := strings.TrimSpace(b.Profile.CharacterName)
		if targetChar == "" {
			targetChar = strings.TrimSpace(b.Profile.Name)
		}
		b.mu.Unlock()

		b.emitUpdate("connecting", fmt.Sprintf("Login OK (%dms). Resolving character...", ping))
		b.emitLog("net", fmt.Sprintf("HTTP Login OK: Session authenticated (latency: %dms)", ping), "info")

		// Find slot with matching character name or first non-empty slot
		foundSlotChar := ""
		charSlot := 0
		for idx, s := range auth.Account.Slots {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				if foundSlotChar == "" {
					foundSlotChar = trimmed
					charSlot = idx
				}
				if strings.EqualFold(trimmed, targetChar) {
					foundSlotChar = trimmed
					charSlot = idx
					break
				}
			}
		}

		// If NO character exists on account at all, create character in slot 0!
		if foundSlotChar == "" {
			b.emitLog("system", fmt.Sprintf("No character in account slots. Creating character '%s' in slot 0...", targetChar), "info")
			b.emitUpdate("connecting", fmt.Sprintf("Creating character '%s'...", targetChar))
			acc, cErr := httpCli.CreateCharacter(sessionToken, 0, targetChar)
			if cErr != nil {
				b.emitLog("system", fmt.Sprintf("Failed to auto-create character: %v", cErr), "warn")
			} else {
				if acc != nil && len(acc.Slots) > 0 && acc.Slots[0] != "" {
					foundSlotChar = acc.Slots[0]
				} else {
					foundSlotChar = targetChar
				}
				accKey := auth.Account.Key
				if accKey == "" {
					accKey = strings.ToLower(b.Profile.Name)
				}
				initJson := models.CreateDefaultCharacterSave(foundSlotChar, "male")
				_, _ = httpCli.SaveFiles(sessionToken, map[string]string{
					fmt.Sprintf("chars/%s_0.json", accKey): initJson,
				})
				if auth.Files == nil {
					auth.Files = make(map[string]string)
				}
				auth.Files[fmt.Sprintf("chars/%s_0.json", accKey)] = initJson
				b.emitLog("system", fmt.Sprintf("Character '%s' created and saved to server.", foundSlotChar), "info")
			}
		}

		if foundSlotChar != "" {
			targetChar = foundSlotChar
		}

		accKey := auth.Account.Key
		if accKey == "" {
			accKey = strings.ToLower(b.Profile.Name)
		}

		b.mu.Lock()
		b.httpCli = httpCli
		b.accountKey = accKey
		b.accountDP = auth.Account.DP
		b.accountVP = auth.Account.VP
		b.hasPin = (auth.Account.Pin == 1)
		b.charSlot = charSlot
		b.saveFiles = auth.Files
		b.charFileName = models.FindCharacterFileName(auth.Files, targetChar, accKey)
		b.Profile.CharacterName = targetChar
		if cs := models.ParseCharacterStats(auth.Files, b.Profile.CharacterName); cs != nil {
			b.Stats.Level = cs.Level
			b.Stats.Exp = cs.Exp
			b.Stats.MaxExp = uint64(models.GetExpNeed(int(cs.Level)))
			b.Stats.Gold = cs.Penya
			b.Stats.Job = models.FormatJobName(cs.Job)
			b.Stats.HP = cs.HP
			b.Stats.MP = cs.MP
			b.Stats.FP = cs.FP
			b.Stats.STR = cs.STR
			if b.Stats.STR == 0 {
				b.Stats.STR = 15
			}
			b.Stats.STA = cs.STA
			if b.Stats.STA == 0 {
				b.Stats.STA = 15
			}
			b.Stats.DEX = cs.DEX
			if b.Stats.DEX == 0 {
				b.Stats.DEX = 15
			}
			b.Stats.INT = cs.INT
			if b.Stats.INT == 0 {
				b.Stats.INT = 15
			}
			b.Stats.StatPoints = cs.Points
			b.Stats.AutoStat = cs.AutoStat
			if len(cs.Pos) >= 3 {
				b.Stats.PosX = cs.Pos[0]
				b.Stats.PosY = cs.Pos[1]
				b.Stats.PosZ = cs.Pos[2]
				b.Stats.Zone = models.DetermineZone(cs.Map, cs.Pos[0], cs.Pos[2])
			}
			if cs.Name != "" {
				b.Profile.CharacterName = cs.Name
			}
			charName := b.Profile.CharacterName
			lvl := b.Stats.Level
			job := b.Stats.Job
			zone := b.Stats.Zone
			b.mu.Unlock()
			b.emitLog("system", fmt.Sprintf("Character profile loaded: %s (Lv %d %s) at %s", charName, lvl, job, zone), "info")
		} else {
			charName := b.Profile.CharacterName
			lvl := b.Stats.Level
			b.mu.Unlock()
			b.emitLog("system", fmt.Sprintf("Using active character: %s (Lv %d)", charName, lvl), "info")
		}
	}

	// Connect WebSocket
	b.emitUpdate("connecting", fmt.Sprintf("Dialing WebSocket (%s)...", b.Profile.Server))
	b.emitLog("net", fmt.Sprintf("Connecting WebSocket endpoint: %s", endpoint), "info")
	if err := b.connectWebSocket(sessionCtx, endpoint); err != nil {
		select {
		case <-sessionCtx.Done():
			b.mu.Lock()
			b.Status = "disconnected"
			b.StatusMessage = "Connection cancelled"
			b.mu.Unlock()
			return errors.New("connection cancelled")
		default:
		}
		statusType := "error"
		statusText := fmt.Sprintf("WebSocket dial failed: %v", err)
		if isServerIssue(err) {
			statusType = "server_issue"
			statusText = "Server Issue (502 Bad Gateway)"
		}
		b.mu.Lock()
		b.Status = statusType
		b.StatusMessage = statusText
		b.mu.Unlock()
		b.emitUpdate(statusType, statusText)
		b.emitLog("net", fmt.Sprintf("WebSocket dial error: %v", err), "error")
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	select {
	case <-sessionCtx.Done():
		b.mu.Lock()
		b.Status = "disconnected"
		b.StatusMessage = "Connection cancelled"
		b.mu.Unlock()
		return errors.New("connection cancelled")
	default:
	}

	b.mu.Lock()
	if b.Status != "connecting" {
		b.mu.Unlock()
		return errors.New("session cancelled or stopped")
	}
	b.Status = "online"
	b.StatusMessage = "Session active"
	currentStats := b.Stats
	b.mu.Unlock()

	b.emitUpdate("online", "Connected â€¢ Session active")
	b.emitLog("net", "WebSocket connection established. Session active.", "info")

	// Send handshake hello packet
	if endpoint != "mock" && sessionToken != "" {
		b.emitUpdate("online", "Sending handshake hello...")
		look := b.buildLook(b.Profile.Preferences.Invisible)
		helloPkt := map[string]interface{}{
			"t":     "hello",
			"token": sessionToken,
			"char":  b.Profile.CharacterName,
			"look":  look,
			"lv":    currentStats.Level,
			"job":   strings.ToLower(currentStats.Job),
			"p":     []float32{currentStats.PosX, currentStats.PosY, currentStats.PosZ},
			"map":   "madrigal",
		}
		if data, err := json.Marshal(helloPkt); err == nil {
			_ = b.ws.Send(data)
			b.emitLog("net", fmt.Sprintf("-> WS OUT [hello] Handshake sent for %s (Lv %d)", b.Profile.CharacterName, currentStats.Level), "info")
		}
	} else if endpoint == "mock" {
		b.emitLog("system", "Mock server session active.", "info")
	}

	b.emitUpdate("online", "Connected â€¢ Synchronized")
	go b.sendPing()

	// Spawn isolated bot execution loop
	go b.runLoop()
	b.startPeriodicSaver()

	return nil
}

func (b *BotInstance) sendPing() {
	if !b.ws.IsConnected() {
		return
	}
	now := time.Now()
	b.pingMu.Lock()
	b.pingSeq++
	seq := b.pingSeq
	b.pingSent[seq] = now
	for s, t := range b.pingSent {
		if now.Sub(t) > 5*time.Second {
			delete(b.pingSent, s)
		}
	}
	b.pingMu.Unlock()

	pkt := map[string]interface{}{
		"t": "ping",
		"c": seq,
	}
	if data, err := json.Marshal(pkt); err == nil {
		_ = b.ws.Send(data)
	}
}

func (b *BotInstance) pruneStaleNearbyPlayers(maxAge time.Duration) {
	b.playersMu.Lock()
	defer b.playersMu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for id, p := range b.nearbyPlayers {
		if p.LastSeen.Before(cutoff) {
			delete(b.playerNames, strings.ToLower(p.Name))
			delete(b.nearbyPlayers, id)
		}
	}
}

func (b *BotInstance) runLoop() {
	b.mu.Lock()
	stopCh := b.cancelCh
	b.mu.Unlock()

	// Periodic follow loop, ping heartbeat, and keepalive position broadcast
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		idleCounter := 0
		tickCount := 0

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if !b.ws.IsConnected() {
					continue
				}

				tickCount++
				// Every 2 seconds (8 ticks * 250ms), send live ping to measure accurate latency
				if tickCount%8 == 0 {
					go b.sendPing()
				}

				// Every 500ms (2 ticks * 250ms), run smart gear and inventory automation (overlap guarded)
				if tickCount%2 == 0 {
					if b.automationRunning.CompareAndSwap(false, true) {
						go func() {
							defer b.automationRunning.Store(false)
							b.RunSmartAutomationTick()
						}()
					}
				}

				// Every 5 seconds (20 ticks * 250ms), prune stale nearby players (> 10s old)
				if tickCount%20 == 0 {
					b.pruneStaleNearbyPlayers(10 * time.Second)
				}

				// Every 15 seconds (60 ticks * 250ms), persist character stats (level, exp, penya) to server (overlap guarded)
				if tickCount%60 == 0 {
					if b.saveStatsRunning.CompareAndSwap(false, true) {
						go func() {
							defer b.saveStatsRunning.Store(false)
							_ = b.SaveCurrentStats()
						}()
					}
				}

				// Every 30 minutes (7200 ticks * 250ms), check daily gifts if auto-claim enabled
				if tickCount%7200 == 0 && b.Profile.Preferences.AutoClaimDailyGifts {
					go b.checkAndClaimDailyGift()
				}

				// If actively performing a radar sweep, skip regular movement and keepalive st packets
				// to avoid conflicting with node probe state packets
				b.mu.Lock()
				sweeping := b.isSweeping
				b.mu.Unlock()
				if sweeping {
					continue
				}

				// Priority 1: Navigation to Map Waypoint
				wpX, wpZ, hasWp := b.GetWaypoint()
				if hasWp {
					b.mu.Lock()
					botX := b.Stats.PosX
					botY := b.Stats.PosY
					botZ := b.Stats.PosZ
					level := b.Stats.Level
					moveStepPref := b.Profile.Preferences.MoveStep
					isLegit := b.Profile.Preferences.LegitMode
					b.mu.Unlock()

					b.waypointMu.RLock()
					isFly := b.waypointFlyMode
					b.waypointMu.RUnlock()

					dx := wpX - botX
					dz := wpZ - botZ
					dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))

					stopDist := float32(2.2)
					if b.Farm != nil && b.Farm.IsRunning() {
						stopDist = 1.3
					}
					if dist <= stopDist {
						b.onWaypointArrived(wpX, wpZ)
						continue
					}

					// Use configured MoveStep from MOVEMENT settings (default 1.2)
					step := moveStepPref
					if step <= 0 {
						step = 1.2
					}
					// Natural run step speed in Legit Mode (~6-7m/s => 1.6m per 250ms)
					if isLegit && !isFly && step < 1.6 {
						step = 1.6
					}
					if dist < step {
						step = dist
					}

					var newX, newZ, rot float32
					if isLegit && !isFly {
						newX, newZ, rot = SteerStep(botX, botZ, wpX, wpZ, step)
					} else {
						newX = botX + (dx/dist)*step
						newZ = botZ + (dz/dist)*step
						rot = float32(math.Atan2(float64(dx), float64(dz)))
					}

					var newY float32
					anim := "run"
					if isLegit && b.Farm != nil && b.Farm.IsRunning() {
						anim = "arun"
					}
					if isFly {
						anim = "fly"
						newY = stanceY(newX, newZ, true, 25.0) // 25m clearance to fly over mountains/obstacles
					} else {
						newY = stanceY(newX, newZ, false, 0)
					}
					if newY <= 0 {
						newY = botY
					}

					nowMs := time.Now().UnixMilli()
					stPkt := map[string]interface{}{
						"t":   "st",
						"p":   []float32{float32(math.Round(float64(newX)*100) / 100), float32(math.Round(float64(newY)*100) / 100), float32(math.Round(float64(newZ)*100) / 100)},
						"r":   float32(math.Round(float64(rot)*1000) / 1000),
						"a":   anim,
						"fly": isFly,
						"lv":  level,
						"hp":  1.0,
						"ts":  nowMs,
					}
					if data, err := json.Marshal(stPkt); err == nil {
						_ = b.ws.Send(data)
					}

					b.mu.Lock()
					b.Stats.PosX = newX
					b.Stats.PosY = newY
					b.Stats.PosZ = newZ
					b.Stats.Zone = models.DetermineZone("madrigal", newX, newZ)
					b.Stats.Waypoint = &models.WaypointCoord{X: wpX, Z: wpZ}
					modeLabel := "Walking"
					if isFly {
						modeLabel = "Flying (noclip)"
					}
					statusMsg := fmt.Sprintf("%s to (%.0f, %.0f) â€¢ %.0fm left (spd: %.1f)", modeLabel, wpX, wpZ, dist, step)
					b.StatusMessage = statusMsg
					currentStats := b.Stats
					status := b.Status
					b.mu.Unlock()

					if b.onUpdate != nil {
						b.onUpdate(b.ID, currentStats, status, statusMsg)
					}
					continue
				}

				target := b.GetFollowTarget()
				if target != "" {
					var targetX, targetY, targetZ float32
					found := false

					// Check nearby players in current channel
					b.playersMu.RLock()
					if id, ok := b.playerNames[strings.ToLower(target)]; ok {
						if p, ok := b.nearbyPlayers[id]; ok {
							targetX = p.X
							targetY = p.Y
							targetZ = p.Z
							found = true
						}
					}
					b.playersMu.RUnlock()

					// Check other bots in multibot manager
					if !found && b.findOtherBot != nil {
						if other := b.findOtherBot(target); other != nil {
							targetX = other.Stats.PosX
							targetY = other.Stats.PosY
							targetZ = other.Stats.PosZ
							found = true
						}
					}

					if !found {
						idleCounter++
						if idleCounter%20 == 0 { // every 5s if following missing target
							gotoPkt := map[string]interface{}{
								"t":    "gm",
								"k":    "goto",
								"name": target,
							}
							if data, err := json.Marshal(gotoPkt); err == nil {
								_ = b.ws.Send(data)
							}
						}
						continue
					}

					b.mu.Lock()
					botX := b.Stats.PosX
					botZ := b.Stats.PosZ
					level := b.Stats.Level
					b.mu.Unlock()

					dx := targetX - botX
					dz := targetZ - botZ
					dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))

					// If within 2.5m, stand still
					if dist <= 2.5 {
						idleCounter++
						if idleCounter%4 == 0 {
							nowMs := time.Now().UnixMilli()
							stPkt := map[string]interface{}{
								"t":   "st",
								"p":   []float32{botX, targetY, botZ},
								"r":   float32(0),
								"a":   "stand",
								"fly": false,
								"lv":  level,
								"hp":  1.0,
								"ts":  nowMs,
							}
							if data, err := json.Marshal(stPkt); err == nil {
								_ = b.ws.Send(data)
							}
						}
						continue
					}

					// If extremely far (> 300m), warp closer
					if dist > 300.0 {
						_ = b.Warp(target)
						continue
					}

					// Step towards target (~7 units/sec => 1.75 units per 250ms)
					step := float32(1.75)
					if dist-2.2 < step {
						step = dist - 2.2
					}

					newX := botX + (dx/dist)*step
					newZ := botZ + (dz/dist)*step
					rot := float32(math.Atan2(float64(-dx), float64(-dz)))

					nowMs := time.Now().UnixMilli()
					stPkt := map[string]interface{}{
						"t":   "st",
						"p":   []float32{newX, targetY, newZ},
						"r":   rot,
						"a":   "run",
						"fly": false,
						"lv":  level,
						"hp":  1.0,
						"ts":  nowMs,
					}
					if data, err := json.Marshal(stPkt); err == nil {
						_ = b.ws.Send(data)
					}

					b.mu.Lock()
					b.Stats.PosX = newX
					b.Stats.PosY = targetY
					b.Stats.PosZ = newZ
					currentStats := b.Stats
					b.mu.Unlock()

					if b.onUpdate != nil {
						b.onUpdate(b.ID, currentStats, "online", b.StatusMessage)
					}
				} else {
					// Periodic keepalive position broadcast every 1s (4 ticks * 250ms)
					idleCounter++
					if idleCounter%4 == 0 {
						b.mu.Lock()
						botX := b.Stats.PosX
						botY := b.Stats.PosY
						botZ := b.Stats.PosZ
						level := b.Stats.Level
						rot := b.currentRot
						isFighting := b.Farm != nil && b.Farm.IsFighting()
						if isFighting {
							b.mu.Unlock()
							continue
						}
						isFarming := b.Farm != nil && b.Farm.IsRunning()
						anim := "stand"
						if isFarming {
							anim = "astand"
						}
						b.mu.Unlock()

						nowMs := time.Now().UnixMilli()
						stPkt := map[string]interface{}{
							"t":   "st",
							"p":   []float32{botX, botY, botZ},
							"r":   rot,
							"a":   anim,
							"fly": false,
							"lv":  level,
							"hp":  1.0,
							"ts":  nowMs,
						}
						if data, err := json.Marshal(stPkt); err == nil {
							_ = b.ws.Send(data)
						}
					}

					// Auto job advancement check every 10 seconds
					if idleCounter%40 == 0 {
						b.mu.Lock()
						lvl := b.Stats.Level
						job := strings.ToLower(b.Stats.Job)
						jobCfg := b.Profile.Preferences.JobConfig
						b.mu.Unlock()

						if jobCfg.AutoJobChange {
							if lvl >= 15 && (job == "vagrant" || job == "") {
								go func(target string) {
									_, _ = b.AdvanceJob(target)
								}(jobCfg.Target1stJob)
							} else if lvl >= 60 && models.IsFirstJob(job) {
								go func(target string) {
									_, _ = b.AdvanceJob(target)
								}(jobCfg.Target2ndJob)
							}
						}
					}
				}
			}
		}
	}()

	defer GetGlobalCoordinator().ReleaseAll(b.ID)

	for {
		select {
		case <-stopCh:
			_ = b.ws.Close()
			return
		default:
			msg, err := b.ws.Read()
			if err != nil {
				if b.telemetry != nil {
					b.telemetry.DumpContext(fmt.Sprintf("WebSocket read failed: %v", err))
				}
				b.mu.Lock()
				if b.sessionCancel != nil {
					b.sessionCancel()
					b.sessionCancel = nil
				}
				if b.cancelCh != nil {
					select {
					case <-b.cancelCh:
					default:
						close(b.cancelCh)
					}
				}
				if b.Status == "duplicate_login" || b.Status == "error" || b.Status == "channel_full" {
					statusType := b.Status
					statusMsg := b.StatusMessage
					b.Stats.PingMs = 0
					b.mu.Unlock()

					b.pingMu.Lock()
					b.pingSeq = 0
					b.pingSent = make(map[int]time.Time)
					b.pingHistory = nil
					b.pingMu.Unlock()

					b.emitLog("system", fmt.Sprintf("WebSocket closed: %s", statusMsg), "warn")
					b.emitUpdate(statusType, statusMsg)
					return
				}
				b.Status = "disconnected"
				b.StatusMessage = "Connection closed"
				b.Stats.PingMs = 0
				b.mu.Unlock()

				b.pingMu.Lock()
				b.pingSeq = 0
				b.pingSent = make(map[int]time.Time)
				b.pingHistory = nil
				b.pingMu.Unlock()

				b.emitLog("system", fmt.Sprintf("WebSocket closed / disconnected: %v", err), "warn")
				b.emitUpdate("disconnected", "Disconnected from server")
				return
			}
			tBeforeProcess := time.Now()
			b.handleIncomingPacket(msg)
			procDuration := time.Since(tBeforeProcess)
			if procDuration > 300*time.Millisecond {
				b.emitLog("net", fmt.Sprintf("[WS STALL] Packet handler took %v to process message (blocked read loop)", procDuration), "warn")
				if b.telemetry != nil {
					b.telemetry.RecordWSStall(procDuration)
				}
			}
		}
	}
}

func toFloat32(v interface{}) float32 {
	switch n := v.(type) {
	case float32:
		return n
	case float64:
		return float32(n)
	case int:
		return float32(n)
	case int64:
		return float32(n)
	case uint32:
		return float32(n)
	case uint64:
		return float32(n)
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return float32(f)
		}
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

func (b *BotInstance) handleIncomingPacket(msg []byte) {
	if len(msg) == 0 {
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(msg, &raw); err != nil {
		b.emitLog("net", fmt.Sprintf("<- WS IN [raw] %s", string(msg)), "info")
		return
	}

	pktType, _ := raw["t"].(string)
	switch pktType {
	case "welcome":
		myID := toInt(raw["id"])
		onlineCount := toInt(raw["online"])
		b.mu.Lock()
		b.entityID = myID
		b.mu.Unlock()
		b.emitLog("net", fmt.Sprintf("<- WS IN [welcome] Session authorized! My Entity ID: %d (Online: %d)", myID, onlineCount), "info")
		b.emitUpdate("online", fmt.Sprintf("Online (ID: %d) â€¢ %s", myID, b.Profile.Server))

		// Authentic in-game chatbox welcome system messages (matches Echoes of Madrigal game client):
		charName := b.Profile.CharacterName
		if charName == "" {
			charName = b.Profile.Name
		}
		b.emitChat("system", "", fmt.Sprintf("Welcome to Echoes of Madrigal, %s!  Click a monster to fight it.  I: inventory   T: character   K: skills   Q: quests   `: auto hunt   B: bag   Enter: chat   Esc: menu", charName), "#ffe066")
		b.emitChat("system", "", "Welcome to Echoes of Madrigal! Type /help in chat for commands.", "#ffe066")
		b.emitChat("system", "", fmt.Sprintf("Connected to Echoes of Madrigal. %d player(s) online.", onlineCount), "#75c8ff")

		if b.Profile.Preferences.AutoClaimDailyGifts {
			go b.checkAndClaimDailyGift()
		}
	case "gmtp":
		if pList, ok := raw["p"].([]interface{}); ok && len(pList) >= 3 {
			gx := toFloat32(pList[0])
			gy := toFloat32(pList[1])
			gz := toFloat32(pList[2])
			b.mu.Lock()
			b.Stats.PosX = gx
			b.Stats.PosY = gy
			b.Stats.PosZ = gz
			b.Stats.Zone = models.DetermineZone("madrigal", gx, gz)
			currentStats := b.Stats
			b.mu.Unlock()
			b.emitLog("action", fmt.Sprintf("<- WS IN [gmtp] Teleported by server to (%.1f, %.1f, %.1f)", gx, gy, gz), "info")
			if b.onUpdate != nil {
				b.onUpdate(b.ID, currentStats, "online", b.StatusMessage)
			}
		}
	case "setpos":
		if pList, ok := raw["p"].([]interface{}); ok && len(pList) >= 3 {
			gx := toFloat32(pList[0])
			gy := toFloat32(pList[1])
			gz := toFloat32(pList[2])
			b.emitLog("action", fmt.Sprintf("<- WS IN [setpos] Server setpos to (%.1f, %.1f, %.1f)", gx, gy, gz), "warn")
			b.mu.Lock()
			b.Stats.PosX = gx
			b.Stats.PosY = gy
			b.Stats.PosZ = gz
			b.Stats.Zone = models.DetermineZone("madrigal", gx, gz)
			currentStats := b.Stats
			b.mu.Unlock()

			b.lastSetPosMu.Lock()
			b.lastSetPos = &models.WaypointCoord{X: gx, Z: gz}
			b.lastSetPosTime = time.Now()
			b.lastSetPosMu.Unlock()

			if b.onUpdate != nil {
				b.onUpdate(b.ID, currentStats, "online", b.StatusMessage)
			}
		}
	case "ping", "mock_ping":
		if pktType == "mock_ping" {
			b.mu.Lock()
			b.Stats.PingMs = 28
			currentStats := b.Stats
			status := b.Status
			statusMsg := b.StatusMessage
			b.mu.Unlock()
			if b.onUpdate != nil {
				b.onUpdate(b.ID, currentStats, status, statusMsg)
			}
		}
	case "pong":
		now := time.Now()
		cVal, ok := raw["c"].(float64)
		if ok {
			seq := int(cVal)
			b.pingMu.Lock()
			t0, found := b.pingSent[seq]
			if found {
				delete(b.pingSent, seq)
			}
			b.pingMu.Unlock()

			if found {
				rtt := uint32(now.Sub(t0).Milliseconds())
				if rtt == 0 {
					rtt = 1
				}

				b.pingMu.Lock()
				b.pingHistory = append(b.pingHistory, rtt)
				if len(b.pingHistory) > 5 {
					b.pingHistory = b.pingHistory[1:]
				}
				histCopy := make([]uint32, len(b.pingHistory))
				copy(histCopy, b.pingHistory)
				sort.Slice(histCopy, func(i, j int) bool { return histCopy[i] < histCopy[j] })
				medianRTT := histCopy[len(histCopy)/2]
				b.pingMu.Unlock()

				b.mu.Lock()
				b.Stats.PingMs = medianRTT
				currentStats := b.Stats
				status := b.Status
				statusMsg := b.StatusMessage
				b.mu.Unlock()

				if b.onUpdate != nil {
					b.onUpdate(b.ID, currentStats, status, statusMsg)
				}
			}
		}
	case "chat":
		ch, _ := raw["ch"].(string)
		if ch == "" {
			ch = "say"
		}
		sender, _ := raw["from"].(string)
		txt, _ := raw["text"].(string)
		if txt == "" {
			txt, _ = raw["msg"].(string)
		}
		b.emitLog("chat", fmt.Sprintf("<- WS IN [chat] <%s>: %s", sender, txt), "info")
		b.handleChatMessage(ch, sender, txt, raw)
	case "notice":
		txt, _ := raw["text"].(string)
		if txt == "" {
			txt, _ = raw["msg"].(string)
		}
		b.emitLog("system", fmt.Sprintf("<- WS IN [notice] %s", txt), "info")
		b.emitChat("system", "Notice", fmt.Sprintf("[Notice] %s", txt), "#ffcc66")
	case "join":
		if idVal, ok := raw["id"].(float64); ok {
			id := int(idVal)
			name, _ := raw["name"].(string)
			epoch := atomic.LoadUint64(&b.scanEpoch)
			p := &RemotePlayer{
				ID:        id,
				Name:      name,
				LastSeen:  time.Now(),
				SeenEpoch: epoch,
				Source:    sightJoin,
			}
			if pList, ok := raw["p"].([]interface{}); ok && len(pList) >= 3 {
				p.X = toFloat32(pList[0])
				p.Y = toFloat32(pList[1])
				p.Z = toFloat32(pList[2])
			}
			b.playersMu.Lock()
			b.nearbyPlayers[id] = p
			if name != "" {
				b.playerNames[strings.ToLower(name)] = id
			}
			b.playersMu.Unlock()
			b.emitLog("net", fmt.Sprintf("Player in range: %s (id: %d) at (%.1f, %.1f, %.1f)", name, id, p.X, p.Y, p.Z), "info")
		}
	case "snap":
		if plList, ok := raw["pl"].([]interface{}); ok {
			now := time.Now()
			epoch := atomic.LoadUint64(&b.scanEpoch)
			b.playersMu.Lock()
			for _, item := range plList {
				if entry, ok := item.([]interface{}); ok && len(entry) >= 4 {
					id := toInt(entry[0])
					p, exists := b.nearbyPlayers[id]
					if !exists {
						p = &RemotePlayer{ID: id}
						b.nearbyPlayers[id] = p
					}
					p.X = toFloat32(entry[1])
					p.Y = toFloat32(entry[2])
					p.Z = toFloat32(entry[3])
					if len(entry) > 4 {
						p.Rot = toFloat32(entry[4])
					}
					p.LastSeen = now
					p.SeenEpoch = epoch
					p.Source = sightSnap
				}
			}
			b.playersMu.Unlock()
		}
	case "leave":
		id := toInt(raw["id"])
		if id != 0 {
			b.playersMu.Lock()
			if p, exists := b.nearbyPlayers[id]; exists {
				delete(b.playerNames, strings.ToLower(p.Name))
				delete(b.nearbyPlayers, id)
			}
			b.playersMu.Unlock()
		}
	case "party":
		partyID := toInt(raw["id"])
		b.partyMu.Lock()
		if partyID == 0 {
			b.inParty = false
			b.partyID = 0
			b.partyName = ""
			b.partyLeaderID = 0
			b.partyLeaderName = ""
			b.partyLevel = 1
			b.partyExp = 0
			b.partyPoints = 0
			b.partyKind = 0
			b.partyShare = 0
			b.partyDist = 0
			b.partyModes = nil
			b.partyModesExpiry = nil
			b.partySkillCD = nil
			b.partySkillCDExpiry = nil
			b.partyScrollFull = false
			b.partyScrollExpUp = false
			b.partyScrollSkillUp = false
			b.partyMembers = nil
			b.partyLeashed = false
			b.levelSyncActive = false
			b.syncLevel = 0
		} else {
			b.inParty = true
			b.partyID = partyID
			if pName, ok := raw["name"].(string); ok && pName != "" {
				b.partyName = pName
			}
			if pLv := toInt(raw["lv"]); pLv > 0 {
				b.partyLevel = pLv
			}
			if pExp := toInt(raw["exp"]); pExp >= 0 {
				b.partyExp = pExp
			}
			if pPt := toInt(raw["pt"]); pPt >= 0 {
				b.partyPoints = pPt
			}
			if kVal, ok := raw["kind"]; ok {
				b.partyKind = toInt(kVal)
			}
			if sVal, ok := raw["share"]; ok {
				b.partyShare = toInt(sVal)
			}
			if dVal, ok := raw["dist"]; ok {
				b.partyDist = toInt(dVal)
			}
			b.partyScrollFull = toInt(raw["full"]) == 1
			b.partyScrollExpUp = toInt(raw["expup"]) == 1
			b.partyScrollSkillUp = toInt(raw["skillup"]) == 1
			b.partyLeaderID = toInt(raw["leader"])
			now := time.Now()
			if modesRaw, ok := raw["modes"].(map[string]interface{}); ok {
				b.partyModes = make(map[string]float64)
				b.partyModesExpiry = make(map[string]time.Time)
				for k, v := range modesRaw {
					secs := toFloat64(v)
					b.partyModes[k] = secs
					if secs > 0 {
						b.partyModesExpiry[k] = now.Add(time.Duration(secs * float64(time.Second)))
					}
				}
			}
			if cdRaw, ok := raw["cd"].(map[string]interface{}); ok {
				b.partySkillCD = make(map[string]float64)
				b.partySkillCDExpiry = make(map[string]time.Time)
				for k, v := range cdRaw {
					secs := toFloat64(v)
					b.partySkillCD[k] = secs
					if secs > 0 {
						b.partySkillCDExpiry[k] = now.Add(time.Duration(secs * float64(time.Second)))
					}
				}
			}
			var parsedMembers []models.PartyMember
			if mList, ok := raw["m"].([]interface{}); ok {
				for _, item := range mList {
					if entry, ok := item.([]interface{}); ok && len(entry) >= 2 {
						mID := toInt(entry[0])
						mName, _ := entry[1].(string)
						var mLv, mJob, mHp, mWorld int
						var mX, mZ float32
						if len(entry) >= 3 {
							mLv = toInt(entry[2])
						}
						if len(entry) >= 4 {
							mJob = toInt(entry[3])
						}
						if len(entry) >= 5 {
							mHp = toInt(entry[4])
						}
						if len(entry) >= 7 {
							mX = toFloat32(entry[5])
							mZ = toFloat32(entry[6])
							if mZ > 0 {
								mZ = -mZ
							}
						}
						if len(entry) >= 8 {
							mWorld = toInt(entry[7])
						}
						parsedMembers = append(parsedMembers, models.PartyMember{
							ID:    mID,
							Name:  mName,
							Level: mLv,
							Job:   mJob,
							HP:    mHp,
							X:     mX,
							Z:     mZ,
							World: mWorld,
						})
						if mID == b.partyLeaderID && mName != "" {
							b.partyLeaderName = mName
						}
					}
				}
			}
			b.partyMembers = parsedMembers
		}
		b.partyMu.Unlock()

		b.evaluatePartyRules()
		b.evaluateLevelSync()

		if mList, ok := raw["m"].([]interface{}); ok {
			now := time.Now()
			b.playersMu.Lock()
			for _, item := range mList {
				if entry, ok := item.([]interface{}); ok && len(entry) >= 7 {
					id := toInt(entry[0])
					name, _ := entry[1].(string)
					x := toFloat32(entry[5])
					z := toFloat32(entry[6])
					if z > 0 {
						z = -z
					}
					p, exists := b.nearbyPlayers[id]
					if !exists {
						p = &RemotePlayer{ID: id, Name: name}
						b.nearbyPlayers[id] = p
					}
					p.X = x
					p.Z = z
					p.LastSeen = now
					if p.Source == "" {
						p.Source = sightParty
					}
					if name != "" {
						b.playerNames[strings.ToLower(name)] = id
					}
				}
			}
			b.playersMu.Unlock()
		}
	case "pinvite":
		fromID := toInt(raw["from"])
		fromName, _ := raw["name"].(string)
		if fromID != 0 {
			accepted := false
			// Intercept via Party Boost module
			if cfg, ok := b.getPartyBoostConfig(); ok && cfg.Enabled {
				isLeader := strings.EqualFold(fromName, cfg.LeaderName)
				if !isLeader && b.partyLeaderID != 0 && fromID == b.partyLeaderID {
					isLeader = true
				}
				if !isLeader && b.findOtherBot != nil && cfg.LeaderName != "" {
					if other := b.findOtherBot(cfg.LeaderName); other != nil {
						if strings.EqualFold(other.CharacterName, fromName) || strings.EqualFold(other.Name, fromName) {
							isLeader = true
						}
					}
				}

				if isLeader {
					_ = b.SendPartyAccept(fromID)
					b.emitLog("party", fmt.Sprintf("Party Boost: Auto-accepted party invite from leader %s (id: %d)", fromName, fromID), "info")
					accepted = true
				} else {
					b.emitLog("party", fmt.Sprintf("Party Boost: Ignored party invite from %s (id: %d, not designated leader %s)", fromName, fromID, cfg.LeaderName), "info")
				}
			}

			// Fallback to bot preference AutoAcceptParty
			if !accepted {
				b.mu.Lock()
				autoAccept := b.Profile.Preferences.AutoAcceptParty
				b.mu.Unlock()
				if autoAccept {
					_ = b.SendPartyAccept(fromID)
					b.emitLog("social", fmt.Sprintf("Auto-accepted party invite from %s (id: %d)", fromName, fromID), "info")
				} else {
					b.emitLog("social", fmt.Sprintf("<- WS IN [pinvite] Party invite received from %s (id: %d)", fromName, fromID), "info")
				}
			}
		}
	case "fsumq":
		fromID := toInt(raw["from"])
		fromName, _ := raw["name"].(string)
		b.emitLog("social", fmt.Sprintf("<- WS IN [fsumq] Friendship summon request from %s (id: %d)", fromName, fromID), "info")
		if cfg, ok := b.getPartyBoostConfig(); ok && cfg.Enabled {
			isLeader := strings.EqualFold(fromName, cfg.LeaderName)
			if !isLeader && b.findOtherBot != nil && cfg.LeaderName != "" {
				if other := b.findOtherBot(cfg.LeaderName); other != nil {
					if strings.EqualFold(other.CharacterName, fromName) || strings.EqualFold(other.Name, fromName) {
						isLeader = true
					}
				}
			}
			if isLeader {
				_ = b.SendFriendshipSummonAccept(fromID)
				b.emitLog("party", fmt.Sprintf("Party Boost: Auto-accepted friendship summon from leader %s (id: %d)", fromName, fromID), "info")
			}
		}
	case "pskill":
		skillID, _ := raw["id"].(string)
		dur := toFloat64(raw["time"])
		if dur <= 0 {
			dur = 60
		}
		by, _ := raw["by"].(string)
		now := time.Now()
		b.partyMu.Lock()
		if b.partyModes == nil {
			b.partyModes = make(map[string]float64)
		}
		if b.partyModesExpiry == nil {
			b.partyModesExpiry = make(map[string]time.Time)
		}
		b.partyModes[skillID] = dur
		b.partyModesExpiry[skillID] = now.Add(time.Duration(dur * float64(time.Second)))
		b.partyMu.Unlock()
		b.emitLog("party", fmt.Sprintf("Party skill %s activated by %s (Duration: %.0fs)", skillID, by, dur), "info")
	case "assist":
		skID, _ := raw["sk"].(string)
		fromName, _ := raw["name"].(string)
		healAmt := toInt(raw["heal"])
		if skID == "SI_ASS_HEAL_RESURRECTION" {
			b.mu.Lock()
			isDead := b.Stats.HP == 0
			if isDead {
				if healAmt <= 0 {
					healAmt = 100
				}
				b.Stats.HP = uint32(healAmt)
			}
			b.mu.Unlock()
			if isDead {
				b.emitLog("party", fmt.Sprintf("Auto-accepted Resurrection from %s (Revived with %d HP)", fromName, healAmt), "info")
				b.emitUpdate("online", fmt.Sprintf("Resurrected by %s", fromName))
			}
		}
	case "treq":
		fromID := toInt(raw["from"])
		fromName, _ := raw["name"].(string)
		b.emitLog("social", fmt.Sprintf("<- WS IN [treq] Trade request from %s (id: %d)", fromName, fromID), "info")
		b.mu.Lock()
		autoAccept := b.Profile.Preferences.AutoAcceptTrade
		b.mu.Unlock()
		if autoAccept {
			acceptPkt := map[string]interface{}{
				"t":    "tacc",
				"from": fromID,
			}
			if data, err := json.Marshal(acceptPkt); err == nil {
				_ = b.ws.Send(data)
				b.emitLog("social", fmt.Sprintf("Auto-accepted trade from %s (id: %d)", fromName, fromID), "info")
			}
		}
	case "trade":
		withName, _ := raw["with"].(string)
		withId := toInt(raw["id"])
		b.emitLog("social", fmt.Sprintf("<- WS IN [trade] Trade window opened with %s (id: %d)", withName, withId), "info")
		b.tradeMu.Lock()
		sess := b.tradeSession
		b.tradeMu.Unlock()
		if sess != nil && (sess.PartnerID == withId || sess.PartnerID == 0) {
			sess.PartnerID = withId
			sess.PartnerName = withName
			tsetPkt := map[string]interface{}{
				"t":     "tset",
				"items": sess.ItemsToGive,
				"penya": sess.Penya,
			}
			if tb, err := json.Marshal(tsetPkt); err == nil {
				_ = b.ws.Send(tb)
			}
			tokPkt := map[string]interface{}{"t": "tok"}
			if ob, err := json.Marshal(tokPkt); err == nil {
				_ = b.ws.Send(ob)
			}
		} else {
			tokPkt := map[string]interface{}{"t": "tok"}
			if ob, err := json.Marshal(tokPkt); err == nil {
				_ = b.ws.Send(ob)
			}
		}
	case "tstate":
		b.tradeMu.Lock()
		sess := b.tradeSession
		b.tradeMu.Unlock()
		if sess != nil {
			if okThem, _ := raw["ok_them"].(bool); okThem {
				tokPkt := map[string]interface{}{"t": "tok"}
				if ob, err := json.Marshal(tokPkt); err == nil {
					_ = b.ws.Send(ob)
				}
			}
		}
	case "tdone":
		withName, _ := raw["with"].(string)
		b.emitLog("social", fmt.Sprintf("<- WS IN [tdone] Trade completed successfully with %s!", withName), "info")
		b.emitChat("system", "Trade", fmt.Sprintf("Trade with %s completed.", withName), "#ffd959")
		b.tradeMu.Lock()
		sess := b.tradeSession
		b.tradeSession = nil
		b.tradeMu.Unlock()
		if sess != nil {
			select {
			case sess.StateCh <- "done":
			default:
			}
		}
	case "tclosed":
		why, _ := raw["why"].(string)
		if why == "" {
			why = "cancelled"
		}
		b.emitLog("social", fmt.Sprintf("<- WS IN [tclosed] Trade was %s", why), "warn")
		b.tradeMu.Lock()
		sess := b.tradeSession
		b.tradeSession = nil
		b.tradeMu.Unlock()
		if sess != nil {
			select {
			case sess.StateCh <- why:
			default:
			}
		}
	case "dmg", "combat", "mhit":
		if b.Farm != nil && b.Farm.IsRunning() {
			cfg := b.Farm.GetConfig()
			if cfg.ExploitGodMode {
				return
			}
			if cfg.ExploitCapHp1 {
				b.mu.Lock()
				if b.Stats.HP <= 1 {
					b.Stats.HP = 1
				}
				b.mu.Unlock()
			}
		}
		b.emitLog("combat", fmt.Sprintf("<- WS IN [combat] %s", string(msg)), "info")
	case "loot", "drop", "item":
		b.emitLog("loot", fmt.Sprintf("<- WS IN [loot] %s", string(msg)), "info")
		b.mu.Lock()
		b.lastLootTime = time.Now()
		b.mu.Unlock()
		penya, _ := raw["penya"].(float64)
		itemsRaw, _ := raw["items"].([]interface{})
		if penya > 0 || len(itemsRaw) > 0 {
			b.handleLootReceived(penya, itemsRaw)
			if b.Farm != nil && b.Farm.IsRunning() {
				b.Farm.OnLootPacket(raw)
			}
		}
	case "mob":
		if b.Farm != nil {
			b.Farm.OnMobPacket(raw)
		}
	case "error":
		errText, _ := raw["text"].(string)
		if b.telemetry != nil {
			b.telemetry.DumpContext(fmt.Sprintf("Server WS error: %s", errText))
		}
		b.emitLog("net", fmt.Sprintf("<- WS IN [error] %s", string(msg)), "warn")
		lower := strings.ToLower(errText)
		if strings.Contains(lower, "blocked") || strings.Contains(lower, "banned") {
			statusType := "account_banned"
			statusText := "Account Banned"
			b.mu.Lock()
			b.Status = statusType
			b.StatusMessage = statusText
			b.Stats.PingMs = 0
			currentStats := b.Stats
			b.mu.Unlock()

			b.emitLog("system", fmt.Sprintf("Connection failed: %s", errText), "error")
			b.emitUpdate(statusType, statusText)
			if b.onUpdate != nil {
				b.onUpdate(b.ID, currentStats, statusType, statusText)
			}
			_ = b.ws.Close()
		} else if strings.Contains(lower, "logged in somewhere else") ||
			strings.Contains(lower, "open in another window") ||
			strings.Contains(lower, "opened somewhere else") {
			statusType := "duplicate_login"
			statusText := errText
			if statusText == "" {
				statusText = "Your account logged in somewhere else."
			}
			b.mu.Lock()
			b.Status = statusType
			b.StatusMessage = statusText
			b.Stats.PingMs = 0
			currentStats := b.Stats
			b.mu.Unlock()

			b.emitLog("system", fmt.Sprintf("Duplicate login detected: %s", statusText), "error")
			b.emitUpdate(statusType, statusText)
			if b.onUpdate != nil {
				b.onUpdate(b.ID, currentStats, statusType, statusText)
			}
			_ = b.ws.Close()
		} else if strings.Contains(lower, "is full") || (strings.Contains(lower, "channel") && strings.Contains(lower, "full")) {
			statusType := "channel_full"
			statusText := "Channel is Full"
			b.mu.Lock()
			b.Status = statusType
			b.StatusMessage = statusText
			b.Stats.PingMs = 0
			currentStats := b.Stats
			b.mu.Unlock()

			b.emitLog("system", fmt.Sprintf("Channel full: %s", errText), "warn")
			b.emitUpdate(statusType, statusText)
			if b.onUpdate != nil {
				b.onUpdate(b.ID, currentStats, statusType, statusText)
			}
			_ = b.ws.Close()
		} else {
			statusType := "error"
			statusText := fmt.Sprintf("Server error: %s", errText)
			if errText == "" {
				statusText = "Server error"
			}
			b.mu.Lock()
			b.Status = statusType
			b.StatusMessage = statusText
			currentStats := b.Stats
			b.mu.Unlock()

			b.emitLog("system", statusText, "error")
			b.emitUpdate(statusType, statusText)
			if b.onUpdate != nil {
				b.onUpdate(b.ID, currentStats, statusType, statusText)
			}
			_ = b.ws.Close()
		}
	default:
		if pktType != "" {
			b.emitLog("net", fmt.Sprintf("<- WS IN [%s] %s", pktType, string(msg)), "info")
		} else {
			b.emitLog("net", fmt.Sprintf("<- WS IN %s", string(msg)), "info")
		}
	}
}

func (b *BotInstance) handleChatMessage(ch, sender, txt string, raw map[string]interface{}) {
	var channel string = "general"
	var color string = "#ffffff"
	var formatted string = ""

	switch ch {
	case "say":
		channel = "general"
		color = "#ffffff"
		if sender != "" {
			formatted = fmt.Sprintf("%s: %s", sender, txt)
		} else {
			formatted = txt
		}
	case "shout":
		channel = "shout"
		color = "#ffd959"
		if sender != "" {
			formatted = fmt.Sprintf("[Shout] %s: %s", sender, txt)
		} else {
			formatted = fmt.Sprintf("[Shout] %s", txt)
		}
	case "whisper":
		channel = "whisper"
		color = "#ff8cd9"
		to, _ := raw["to"].(string)
		if sender == b.Profile.CharacterName || sender == b.Profile.Name {
			formatted = fmt.Sprintf("To %s: %s", to, txt)
		} else {
			formatted = fmt.Sprintf("From %s: %s", sender, txt)
		}
	case "party":
		channel = "party"
		color = "#8cff99"
		if sender != "" {
			formatted = fmt.Sprintf("[Party] %s: %s", sender, txt)
		} else {
			formatted = fmt.Sprintf("[Party] %s", txt)
		}
	case "guild":
		channel = "guild"
		color = "#8cd9ff"
		if sender != "" {
			formatted = fmt.Sprintf("[Guild] %s: %s", sender, txt)
		} else {
			formatted = fmt.Sprintf("[Guild] %s", txt)
		}
	case "notice":
		channel = "system"
		color = "#ffcc66"
		formatted = fmt.Sprintf("[Notice] %s", txt)
	default:
		channel = "system"
		color = "#e6edff"
		formatted = txt
	}

	b.emitChat(channel, sender, formatted, color)
}

func (b *BotInstance) SendChat(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if !b.ws.IsConnected() {
		return fmt.Errorf("bot %s is not connected", b.ID)
	}

	charName := b.Profile.CharacterName
	if charName == "" {
		charName = b.Profile.Name
	}

	var pkt map[string]interface{}

	if strings.HasPrefix(text, "/") {
		parts := strings.SplitN(text, " ", 3)
		cmd := strings.ToLower(parts[0])

		if (cmd == "/w" || cmd == "/whisper" || cmd == "/tell") && len(parts) >= 3 {
			target := parts[1]
			msg := parts[2]
			pkt = map[string]interface{}{
				"t":    "w",
				"to":   target,
				"text": msg,
			}
			b.emitChat("whisper", charName, fmt.Sprintf("To %s: %s", target, msg), "#ff8cd9")
		} else if (cmd == "/p" || cmd == "/party") && len(parts) >= 2 {
			msg := strings.TrimSpace(text[len(parts[0]):])
			pkt = map[string]interface{}{
				"t":    "chat",
				"ch":   "party",
				"text": msg,
			}
			// Server echoes party chat back via handleChatMessage
		} else if (cmd == "/g" || cmd == "/guild") && len(parts) >= 2 {
			msg := strings.TrimSpace(text[len(parts[0]):])
			pkt = map[string]interface{}{
				"t":    "chat",
				"ch":   "guild",
				"text": msg,
			}
			// Server echoes guild chat back via handleChatMessage
		} else if (cmd == "/invite" || cmd == "/pinvite") && len(parts) >= 2 {
			pkt = map[string]interface{}{
				"t":    "pinvite",
				"name": parts[1],
			}
			b.emitChat("system", "", fmt.Sprintf("Inviting %s to party...", parts[1]), "#e6edff")
		} else if cmd == "/leave" || cmd == "/pleave" {
			pkt = map[string]interface{}{
				"t": "pleave",
			}
			b.emitChat("system", "", "Leaving party...", "#e6edff")
		} else {
			pkt = map[string]interface{}{
				"t":    "chat",
				"ch":   "say",
				"text": text,
			}
			// Server echoes say chat back via handleChatMessage
		}
	} else if strings.HasPrefix(text, "!") && len(text) > 1 {
		shoutMsg := strings.TrimSpace(text[1:])
		pkt = map[string]interface{}{
			"t":    "chat",
			"ch":   "shout",
			"text": shoutMsg,
		}
		// Server echoes shout back via handleChatMessage
	} else {
		pkt = map[string]interface{}{
			"t":    "chat",
			"ch":   "say",
			"text": text,
		}
		// Server echoes say chat back via handleChatMessage
	}

	if pkt != nil {
		data, err := json.Marshal(pkt)
		if err != nil {
			return err
		}
		return b.ws.Send(data)
	}
	return nil
}

func (b *BotInstance) Stop() error {
	b.mu.Lock()
	wasDisconnected := (b.Status == "disconnected")
	if b.sessionCancel != nil {
		b.sessionCancel()
		b.sessionCancel = nil
	}
	if b.cancelCh != nil {
		select {
		case <-b.cancelCh:
		default:
			close(b.cancelCh)
		}
	}
	if b.telemetry != nil {
		b.telemetry.Stop()
	}
	b.Status = "disconnected"
	b.StatusMessage = "Disconnected"
	b.sessionToken = ""
	b.accountKey = ""
	b.Stats.PingMs = 0
	b.followTarget = ""
	b.waypointMu.Lock()
	b.hasWaypoint = false
	b.waypointFlyMode = false
	b.waypointMu.Unlock()
	b.Stats.Waypoint = nil
	if b.invisToggledByWaypoint {
		b.Profile.Preferences.Invisible = false
		b.invisToggledByWaypoint = false
	}
	b.partyMu.Lock()
	b.inParty = false
	b.partyID = 0
	b.partyName = ""
	b.partyLeaderID = 0
	b.partyLeaderName = ""
	b.partyMu.Unlock()
	farm := b.Farm
	b.mu.Unlock()

	// Stop Farm OUTSIDE b.mu to prevent S-01 self-deadlock
	if farm != nil {
		farm.Stop()
	}

	GetGlobalCoordinator().ReleaseAll(b.ID)

	b.pingMu.Lock()
	b.pingSeq = 0
	b.pingSent = make(map[int]time.Time)
	b.pingHistory = nil
	b.pingMu.Unlock()

	if !wasDisconnected {
		b.emitLog("system", "Disconnecting bot session...", "warn")
		b.emitUpdate("connecting", "Disconnecting session...")
	}

	_ = b.ws.Close()

	if !wasDisconnected {
		b.emitLog("system", "Bot disconnected from server.", "warn")
		b.emitUpdate("disconnected", "Disconnected")
	}

	return nil
}

var (
	petDbOnce      sync.Once
	petLootersMap  = make(map[string][]interface{}) // itemID -> [model, scale]
	petKinds       = make([]map[string]interface{}, 0)
)

func initPetDatabase() {
	petDbOnce.Do(func() {
		gameJsonPath := resolveFilePath("madrigal_dump/world/game.json")
		dataBytes, err := os.ReadFile(gameJsonPath)
		if err != nil {
			return
		}
		var fullGame map[string]interface{}
		if err := json.Unmarshal(dataBytes, &fullGame); err != nil {
			return
		}
		petsObj, _ := fullGame["pets"].(map[string]interface{})
		if petsObj == nil {
			return
		}
		if looters, ok := petsObj["looters"].(map[string]interface{}); ok {
			for k, v := range looters {
				if arr, ok := v.([]interface{}); ok && len(arr) >= 2 {
					petLootersMap[k] = arr
				}
			}
		}
		if kinds, ok := petsObj["kinds"].([]interface{}); ok {
			for _, k := range kinds {
				if kObj, ok := k.(map[string]interface{}); ok {
					petKinds = append(petKinds, kObj)
				}
			}
		}
	})
}

func lookupLooterModel(itemID string) (string, float32) {
	initPetDatabase()
	if arr, exists := petLootersMap[itemID]; exists && len(arr) >= 2 {
		m, _ := arr[0].(string)
		s := toFloat32(arr[1])
		if s <= 0 {
			s = 1.0
		}
		return m, s
	}
	return "PetDog1", 1.0
}

func lookupRaisedPetModel(itemMap map[string]interface{}) (string, float32, string) {
	initPetDatabase()
	petObj, _ := itemMap["pet"].(map[string]interface{})
	if petObj == nil {
		return "PetEgg", 0.8, "Egg"
	}
	lv := int(toFloat32(petObj["lv"]))
	if lv == 0 {
		return "PetEgg", 0.8, "Egg"
	}
	kindIdx := int(toFloat32(petObj["kind"]))
	if kindIdx >= 0 && kindIdx < len(petKinds) {
		kObj := petKinds[kindIdx]
		if shapes, ok := kObj["shapes"].([]interface{}); ok && len(shapes) > 0 {
			shapeIdx := 0
			if lv >= 5 && len(shapes) > 2 {
				shapeIdx = 2
			} else if lv >= 3 && len(shapes) > 1 {
				shapeIdx = 1
			}
			if shapeArr, ok := shapes[shapeIdx].([]interface{}); ok && len(shapeArr) >= 2 {
				m, _ := shapeArr[0].(string)
				s := toFloat32(shapeArr[1])
				return m, s, "Pet"
			}
		}
	}
	return "PetEgg", 0.8, "Pet"
}

func (b *BotInstance) buildLook(isInvis bool) map[string]interface{} {
	b.mu.Lock()
	job := strings.ToLower(b.Stats.Job)
	b.mu.Unlock()

	charData, _, _, err := b.loadCharData()
	if err != nil || charData == nil {
		charData = make(map[string]interface{})
	}

	gender, _ := charData["gender"].(string)
	if gender == "" {
		gender = "male"
	}
	face := 1
	if f := int(toFloat32(charData["face"])); f > 0 {
		face = f
	}
	hair := 1
	if h := int(toFloat32(charData["hair"])); h > 0 {
		hair = h
	}
	hairColor, _ := charData["hair_color"].(string)
	if hairColor == "" {
		hairColor = "5a381f"
	}

	// 1. Equip mapping: {"weapon": "II_...", "shield": "...", ...}
	eqMap := make(map[string]string)
	if rawEq, ok := charData["equip"].(map[string]interface{}); ok && rawEq != nil {
		for slot, itm := range rawEq {
			if itmStr, ok := itm.(string); ok && itmStr != "" {
				eqMap[slot] = itmStr
			}
		}
	}

	// 2. Equip upgrade mapping: {"weapon": 10, ...}
	upMap := make(map[string]int)
	if rawUp, ok := charData["equip_up"].(map[string]interface{}); ok && rawUp != nil {
		for slot, up := range rawUp {
			if upNum, ok := up.(float64); ok {
				upMap[slot] = int(upNum)
			}
		}
	}

	// 3. Equip element mapping: {"weapon": 3, ...}
	elMap := make(map[string]int)
	if rawEl, ok := charData["equip_el"].(map[string]interface{}); ok && rawEl != nil {
		for slot, el := range rawEl {
			if elNum, ok := el.(float64); ok {
				elMap[slot] = int(elNum)
			}
		}
	}

	// 4. Wardrobe mapping
	wdMap := make(map[string]string)
	if rawWd, ok := charData["wardrobe"].(map[string]interface{}); ok && rawWd != nil {
		for slot, wd := range rawWd {
			if wdStr, ok := wd.(string); ok && wdStr != "" {
				wdMap[slot] = wdStr
			}
		}
	}

	look := map[string]interface{}{
		"g":   gender,
		"f":   face,
		"h":   hair,
		"hc":  hairColor,
		"eq":  eqMap,
		"up":  upMap,
		"el":  elMap,
		"wd":  wdMap,
		"job": job,
		"ti":  "",
		"hd":  []string{},
	}

	// 5. Active Looter and Raised Pet mapping
	looterOut := toInt(charData["looter_out"])
	petOut := toInt(charData["pet_out"])

	if invList, ok := charData["inv"].([]interface{}); ok {
		for _, rawItm := range invList {
			itmMap, ok := rawItm.(map[string]interface{})
			if !ok || len(itmMap) == 0 {
				continue
			}
			uid := toInt(itmMap["uid"])
			id, _ := itmMap["id"].(string)

			// Match Looter Pet
			if looterOut != 0 && uid == looterOut {
				m, s := lookupLooterModel(id)
				look["lt"] = []interface{}{m, s, id}
			}

			// Match Raised Pet / Egg
			if petOut != 0 {
				petObj, _ := itmMap["pet"].(map[string]interface{})
				petUid := uid
				if petObj != nil {
					petUid = toInt(petObj["uid"])
				}
				if petUid == petOut {
					m, s, name := lookupRaisedPetModel(itmMap)
					look["pt"] = []interface{}{m, s, name}
				}
			}
		}
	}

	if isInvis {
		look["ih"] = 1
	}
	return look
}

func (b *BotInstance) broadcastLook() {
	b.mu.Lock()
	connected := b.ws != nil && b.ws.IsConnected()
	sessionToken := b.sessionToken
	isInvis := b.Profile.Preferences.Invisible
	b.mu.Unlock()

	if !connected || sessionToken == "" {
		return
	}

	look := b.buildLook(isInvis)

	// Send look packet to sync peer appearance
	lookPkt := map[string]interface{}{
		"t":    "look",
		"look": look,
	}
	if data, err := json.Marshal(lookPkt); err == nil {
		_ = b.ws.Send(data)
	}

	// Also send GM invis packet for monster non-aggro
	gmInvisPkt := map[string]interface{}{
		"t":  "gm",
		"k":  "invis",
		"on": isInvis,
	}
	if data, err := json.Marshal(gmInvisPkt); err == nil {
		_ = b.ws.Send(data)
	}
}

func (b *BotInstance) diffAndApplyPreferences(old, newPrefs models.BotPreferences, online bool) {
	if online && old.Invisible != newPrefs.Invisible {
		b.broadcastLook()
		if newPrefs.Invisible {
			b.emitLog("action", "Invisibility ENABLED: Bot model and collision hidden from other players.", "info")
		} else {
			b.emitLog("action", "Invisibility DISABLED: Bot model visible to other players.", "info")
		}
	}

	if online && old.LegitMode != newPrefs.LegitMode {
		if newPrefs.LegitMode {
			b.emitLog("action", "Legit Mode ENABLED: In-game movement & combat animations active.", "info")
		} else {
			b.emitLog("action", "Legit Mode DISABLED: Instant warp and raw combat active.", "info")
		}
	}

	if online && !old.AutoClaimDailyGifts && newPrefs.AutoClaimDailyGifts {
		b.emitLog("system", "Claim Daily Gifts enabled. Checking for unclaimed gifts...", "info")
		go b.checkAndClaimDailyGift()
	}
}

func (b *BotInstance) GetPreferences() models.BotPreferences {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Profile.Preferences
}

func (b *BotInstance) MutatePreferences(mutate func(p *models.BotPreferences)) models.BotPreferences {
	b.mu.Lock()
	old := b.Profile.Preferences
	newPrefs := old
	mutate(&newPrefs)
	if newPrefs.AutoReconnectDelay <= 0 {
		newPrefs.AutoReconnectDelay = 14
	}
	b.Profile.Preferences = newPrefs
	online := b.Status == "online"
	b.mu.Unlock()

	b.diffAndApplyPreferences(old, newPrefs, online)
	return newPrefs
}

func (b *BotInstance) UpdatePreferences(prefs models.BotPreferences) {
	b.MutatePreferences(func(p *models.BotPreferences) {
		*p = prefs
	})
}

func (b *BotInstance) UpdateIntervals(intervals models.BotIntervals) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Profile.Intervals = intervals
}

func (b *BotInstance) GetPosition() (float32, float32, float32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Stats.PosX, b.Stats.PosY, b.Stats.PosZ
}

func (b *BotInstance) GetFarmConfig() models.FarmConfig {
	if b.Farm == nil {
		return models.DefaultFarmConfig()
	}
	return b.Farm.GetConfig()
}

func (b *BotInstance) SetFarmConfig(cfg models.FarmConfig) error {
	if b.Farm == nil {
		return fmt.Errorf("farm engine not initialized")
	}
	b.Farm.SetConfig(cfg)
	return nil
}

func (b *BotInstance) MutateFarmConfig(mutate func(c *models.FarmConfig)) models.FarmConfig {
	if b.Farm == nil {
		return models.FarmConfig{}
	}
	cfg := b.Farm.GetConfig()
	mutate(&cfg)
	b.Farm.SetConfig(cfg)
	return cfg
}

func (b *BotInstance) StartFarm() error {
	if b.Farm == nil {
		return fmt.Errorf("farm engine not initialized")
	}
	return b.Farm.Start()
}

func (b *BotInstance) StopFarm() error {
	if b.Farm == nil {
		return nil
	}
	b.Farm.Stop()
	return nil
}

func (b *BotInstance) IsFarmRunning() bool {
	if b.Farm == nil {
		return false
	}
	return b.Farm.IsRunning()
}

func (b *BotInstance) UpdateServer(server string) error {
	b.mu.Lock()
	oldServer := b.Profile.Server
	b.Profile.Server = server
	status := b.Status
	sessionToken := b.sessionToken
	b.mu.Unlock()

	b.emitLog("net", fmt.Sprintf("Server profile updated: %s -> %s", oldServer, server), "info")

	// If bot is currently online and channel changed, reconnect WebSocket to new channel
	if status == "online" && !strings.EqualFold(oldServer, server) {
		endpoint := client.ResolveWSEndpoint(server)
		if endpoint != "mock" {
			b.emitLog("net", fmt.Sprintf("Switching channel: Reconnecting to %s (%s)...", server, endpoint), "info")

			// Close old connection
			_ = b.ws.Close()

			// Connect to new endpoint
			if err := b.connectWebSocket(context.Background(), endpoint); err != nil {
				b.mu.Lock()
				b.Status = "error"
				b.StatusMessage = fmt.Sprintf("Channel switch failed: %v", err)
				b.mu.Unlock()
				b.emitLog("net", fmt.Sprintf("Failed to switch channel: %v", err), "error")
				b.emitUpdate("error", b.StatusMessage)
				return fmt.Errorf("reconnect to channel %s failed: %w", server, err)
			}

			// Clear nearby players map and coordinator claims since this is a new channel instance
			GetGlobalCoordinator().ReleaseAll(b.ID)
			b.playersMu.Lock()
			b.nearbyPlayers = make(map[int]*RemotePlayer)
			b.playerNames = make(map[string]int)
			b.playersMu.Unlock()

			// Send handshake hello packet
			b.mu.Lock()
			currentStats := b.Stats
			charName := b.Profile.CharacterName
			isInvis := b.Profile.Preferences.Invisible
			b.mu.Unlock()

			if sessionToken != "" {
				look := b.buildLook(isInvis)
				helloPkt := map[string]interface{}{
					"t":     "hello",
					"token": sessionToken,
					"char":  charName,
					"look":  look,
					"lv":    currentStats.Level,
					"job":   strings.ToLower(currentStats.Job),
					"p":     []float32{currentStats.PosX, currentStats.PosY, currentStats.PosZ},
					"map":   "madrigal",
				}
				if data, err := json.Marshal(helloPkt); err == nil {
					_ = b.ws.Send(data)
					b.emitLog("net", fmt.Sprintf("-> WS OUT [hello] Sent handshake for %s on %s", charName, server), "info")
				}
			}

			b.emitLog("net", fmt.Sprintf("Successfully switched to %s", server), "info")

			b.emitUpdate("online", fmt.Sprintf("Connected â€¢ %s", server))
		}
	}
	return nil
}

func (b *BotInstance) Warp(target string) error {
	b.mu.Lock()
	if b.Status != "online" {
		b.mu.Unlock()
		return fmt.Errorf("bot %s is not connected", b.ID)
	}
	b.mu.Unlock()

	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("target name or coordinates cannot be empty")
	}

	var targetX, targetY, targetZ float32
	var foundName string
	found := false

	// 1. Check if target is coordinate pair e.g. "100, -250" or "6950 3200"
	coords := strings.FieldsFunc(target, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
	if len(coords) >= 2 {
		x, err1 := strconv.ParseFloat(coords[0], 32)
		z, err2 := strconv.ParseFloat(coords[1], 32)
		if err1 == nil && err2 == nil {
			targetX = float32(x)
			targetZ = float32(z)
			if targetZ > 0 {
				targetZ = -targetZ
			}
			targetY = b.Stats.PosY
			foundName = fmt.Sprintf("Coords(%.1f, %.1f)", targetX, targetZ)
			found = true
		}
	}

	// 2. Check known named places (e.g. Flarine, Saint Morning, Darken)
	if !found {
		if nx, nz, ok := models.ResolveNamedPlace(target); ok {
			targetX = nx
			targetZ = nz
			targetY = b.Stats.PosY
			foundName = target
			found = true
		}
	}

	// 3. Check nearby players received from WebSocket in current channel
	if !found {
		b.playersMu.RLock()
		if id, ok := b.playerNames[strings.ToLower(target)]; ok {
			if p, ok := b.nearbyPlayers[id]; ok && time.Since(p.LastSeen) <= 10*time.Second {
				targetX = p.X
				targetY = p.Y
				targetZ = p.Z
				foundName = p.Name
				found = true
			}
		}
		b.playersMu.RUnlock()
	}

	// 4. Check other bots in multibot manager
	if !found && b.findOtherBot != nil {
		if other := b.findOtherBot(target); other != nil {
			targetX = other.Stats.PosX
			targetY = other.Stats.PosY
			targetZ = other.Stats.PosZ
			foundName = other.Name
			found = true
		}
	}

	if !found {
		return b.RadarSweepAndWarp(target)
	}

	return b.warpDirect(targetX, targetY, targetZ, foundName)
}

func (b *BotInstance) sendWarpPacket(x, z float32) {
	b.sendProbe(models.SweepNode{X: x, Z: z, Fly: false})
}

func (b *BotInstance) sendProbe(node models.SweepNode) {
	cfg := b.getScanConfig()
	y := stanceY(node.X, node.Z, node.Fly, cfg.FlightClearanceM)
	anim := "stand"
	if node.Fly {
		anim = "fly"
	}

	warpPkt := map[string]interface{}{
		"t": "warp",
		"x": node.X,
		"z": node.Z,
	}
	if data, err := json.Marshal(warpPkt); err == nil {
		_ = b.ws.Send(data)
	}

	b.mu.Lock()
	level := b.Stats.Level
	b.Stats.PosX = node.X
	b.Stats.PosY = y
	b.Stats.PosZ = node.Z
	b.mu.Unlock()

	stPkt := map[string]interface{}{
		"t":   "st",
		"p":   []float32{node.X, y, node.Z},
		"r":   float32(0),
		"a":   anim,
		"fly": node.Fly,
		"lv":  level,
		"hp":  1.0,
		"ts":  time.Now().UnixMilli(),
	}
	if data, err := json.Marshal(stPkt); err == nil {
		if sendErr := b.ws.Send(data); sendErr != nil {
			b.emitLog("net", fmt.Sprintf("Failed to send warp state packet: %v", sendErr), "warning")
		}
	}
}

func (b *BotInstance) warpDirect(targetX, targetY, targetZ float32, foundName string) error {
	b.sendProbe(models.SweepNode{X: targetX, Z: targetZ, Fly: false})

	b.mu.Lock()
	currentStats := b.Stats
	b.mu.Unlock()

	b.emitLog("action", fmt.Sprintf("-> WARP to %s: Arrived at (%.1f, %.1f, %.1f)", foundName, targetX, currentStats.PosY, targetZ), "info")

	if b.onUpdate != nil {
		b.onUpdate(b.ID, currentStats, "online", b.StatusMessage)
	}

	return nil
}

func (b *BotInstance) RadarSweepAndWarp(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}

	b.mu.Lock()
	if b.Status != "online" {
		b.mu.Unlock()
		return fmt.Errorf("bot %s is not online", b.ID)
	}
	origServer := b.Profile.Server
	token := b.sessionToken
	origX := b.Stats.PosX
	origY := b.Stats.PosY
	origZ := b.Stats.PosZ
	b.mu.Unlock()

	// 1. Quick check if target is already in nearby list with fresh sighting
	b.playersMu.RLock()
	if id, ok := b.playerNames[strings.ToLower(target)]; ok {
		if p, ok := b.nearbyPlayers[id]; ok && time.Since(p.LastSeen) <= 5*time.Second {
			b.playersMu.RUnlock()
			return b.warpDirect(p.X, p.Y, p.Z, p.Name)
		}
	}
	b.playersMu.RUnlock()

	// 2. Opportunistic /api/who check to detect target channel
	b.emitLog("action", fmt.Sprintf("Target '%s' not in render range. Checking /api/who...", target), "info")
	httpCli := b.newGameHTTPClient()
	who, err := httpCli.GetWho(token)
	var targetPlayer *client.WhoPlayer
	if err == nil && who != nil && who.OK {
		for i := range who.List {
			if strings.EqualFold(strings.TrimSpace(who.List[i].Char), target) {
				targetPlayer = &who.List[i]
				break
			}
		}

		if targetPlayer != nil {
			b.emitLog("action", fmt.Sprintf("Target '%s' found in /api/who: Online on Channel %d (Lv.%d %s).", targetPlayer.Char, targetPlayer.Ch, targetPlayer.Lv, targetPlayer.Job), "info")

			targetServer := fmt.Sprintf("Channel %d", targetPlayer.Ch)
			if !strings.EqualFold(origServer, targetServer) {
				b.emitLog("action", fmt.Sprintf("Target is on %s. Switching from %s to locate...", targetServer, origServer), "info")
				if sErr := b.UpdateServer(targetServer); sErr != nil {
					b.emitLog("action", fmt.Sprintf("Channel switch error: %v. Continuing on %s.", sErr, origServer), "warn")
				} else {
					time.Sleep(1500 * time.Millisecond) // Allow channel handshake to finish
				}
			}
		} else {
			b.emitLog("action", fmt.Sprintf("Target '%s' not in /api/who sample (%d players listed). Starting world zone sweep on current channel...", target, len(who.List)), "info")
		}
	} else if err != nil {
		b.emitLog("net", fmt.Sprintf("/api/who lookup skipped (%v). Starting world zone sweep on current channel...", err), "info")
	}

	// 3. Sweep waypoints
	b.pruneStaleNearbyPlayers(2 * time.Second)

	cleanTarget := strings.ToLower(strings.TrimSpace(target))
	var lastKnown *models.WaypointCoord
	b.playersMu.RLock()
	if id, ok := b.playerNames[cleanTarget]; ok {
		if p, ok := b.nearbyPlayers[id]; ok {
			lastKnown = &models.WaypointCoord{X: p.X, Z: p.Z}
		}
	}
	b.playersMu.RUnlock()

	cfg := b.getScanConfig()
	terrain := GetWorldTerrain()
	profile := models.DefaultMainMadrigalProfile()
	nodes := terrain.GenerateSweepNodes(profile, cfg, origX, origZ, lastKnown)

	b.emitLog("action", fmt.Sprintf("Starting %s radar sweep for '%s' across %d nodes (R=%dm, dwell=%dms, burst=%d/%dms)...",
		cfg.SweepMode, target, len(nodes), int(cfg.RGuaranteedM), cfg.DwellTimeoutMs, cfg.BurstSize, cfg.BurstRestMs), "info")

	var foundP *RemotePlayer
	emptyCount := 0
	rejectedCount := 0

	dwellTimeout := time.Duration(cfg.DwellTimeoutMs) * time.Millisecond
	if dwellTimeout <= 0 {
		dwellTimeout = 1000 * time.Millisecond
	}

	b.mu.Lock()
	b.isSweeping = true
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.isSweeping = false
		b.mu.Unlock()
	}()

	for i, node := range nodes {
		b.mu.Lock()
		if b.Status != "online" {
			b.mu.Unlock()
			return fmt.Errorf("radar sweep interrupted: bot disconnected")
		}
		b.mu.Unlock()

		b.warpPacer.Wait(cfg)

		curEpoch := atomic.AddUint64(&b.scanEpoch, 1)
		probeWarpTime := time.Now()

		b.lastSetPosMu.Lock()
		b.lastSetPos = nil
		b.lastSetPosMu.Unlock()

		b.sendProbe(node)

		nodeResult := "empty"
		nodeLatency := int64(0)

		pollTicker := time.NewTicker(25 * time.Millisecond)
		probeTimer := time.NewTimer(dwellTimeout)

	dwellLoop:
		for {
			select {
			case <-probeTimer.C:
				nodeLatency = time.Since(probeWarpTime).Milliseconds()
				nodeResult = "empty"
				emptyCount++
				break dwellLoop
			case <-pollTicker.C:
				elapsed := time.Since(probeWarpTime).Milliseconds()

				// Check setpos rejection
				b.lastSetPosMu.Lock()
				isRejected := b.lastSetPos != nil && b.lastSetPosTime.After(probeWarpTime)
				b.lastSetPosMu.Unlock()

				if isRejected {
					nodeLatency = elapsed
					nodeResult = "rejected"
					rejectedCount++
					b.emitLog("action", fmt.Sprintf("-> Probe Tier %d (%.1f, %.1f): REJECTED by server in %dms (%s)",
						node.Tier, node.X, node.Z, nodeLatency, node.Name), "warn")
					b.lastSetPosMu.Lock()
					anchor := b.lastSetPos
					b.lastSetPosMu.Unlock()
					if anchor != nil && i+1 < len(nodes) {
						rest := NearestNeighborSort(append([]models.SweepNode(nil), nodes[i+1:]...), anchor.X, anchor.Z)
						nodes = append(nodes[:i+1], rest...)
					}
					break dwellLoop
				}

				b.playersMu.RLock()
				if id, ok := b.playerNames[cleanTarget]; ok {
					if p, ok := b.nearbyPlayers[id]; ok && p.IsFreshAOI(curEpoch, probeWarpTime) {
						cp := *p
						foundP = &cp
					}
				}
				if foundP == nil {
					for _, p := range b.nearbyPlayers {
						if strings.EqualFold(strings.TrimSpace(p.Name), cleanTarget) && p.IsFreshAOI(curEpoch, probeWarpTime) {
							cp := *p
							foundP = &cp
							break
						}
					}
				}
				b.playersMu.RUnlock()

				if foundP != nil {
					nodeLatency = elapsed
					nodeResult = "hit"
					b.emitLog("action", fmt.Sprintf("-> Probe Tier %d (%.1f, %.1f): HIT! Target '%s' verified in %dms (%s)",
						node.Tier, node.X, node.Z, foundP.Name, nodeLatency, node.Name), "info")
					break dwellLoop
				}
			}
		}
		pollTicker.Stop()
		probeTimer.Stop()

		b.emitLog("debug", fmt.Sprintf("Probe node (%.1f, %.1f) Tier %d result: %s in %dms",
			node.X, node.Z, node.Tier, nodeResult, nodeLatency), "debug")

		if foundP != nil {
			b.emitLog("action", fmt.Sprintf("RADAR LOCK! Target '%s' detected near %s (%.1f, %.1f) via %s!",
				target, node.Name, foundP.X, foundP.Z, foundP.Source), "info")
			break
		}
	}

	// 4. Lock onto target if found
	if foundP != nil {
		return b.warpDirect(foundP.X, foundP.Y, foundP.Z, foundP.Name)
	}

	// 5. Fallback: Return to starting spot
	b.emitLog("action", fmt.Sprintf("Target '%s' not found across %d nodes (%d empty, %d rejected). Returning to starting position.",
		target, len(nodes), emptyCount, rejectedCount), "warn")
	b.sendWarpPacket(origX, origZ)
	b.mu.Lock()
	b.Stats.PosX = origX
	b.Stats.PosY = origY
	b.Stats.PosZ = origZ
	currentServer := b.Profile.Server
	b.mu.Unlock()

	return fmt.Errorf("target '%s' not found across %d scanned nodes on %s (%d empty, %d rejected). Ensure bot and player are on same channel",
		target, len(nodes), currentServer, emptyCount, rejectedCount)
}

func (b *BotInstance) SetWaypoint(x, z float32) {
	b.waypointMu.Lock()
	oldX := b.waypointX
	oldZ := b.waypointZ
	wasActive := b.hasWaypoint
	b.waypointX = x
	b.waypointZ = z
	b.hasWaypoint = true
	b.waypointFlyMode = false
	b.waypointMu.Unlock()

	// If minor adjustment towards same moving target, update coords without spamming logs
	if wasActive && math.Abs(float64(x-oldX)) < 1.0 && math.Abs(float64(z-oldZ)) < 1.0 {
		b.mu.Lock()
		if b.Stats.Waypoint != nil {
			b.Stats.Waypoint.X = x
			b.Stats.Waypoint.Z = z
		}
		b.mu.Unlock()
		return
	}

	isFarming := b.Farm != nil && b.Farm.IsRunning()

	b.mu.Lock()
	b.followTarget = ""
	b.Stats.Waypoint = &models.WaypointCoord{X: x, Z: z}
	botX := b.Stats.PosX
	botZ := b.Stats.PosZ
	currentStats := b.Stats
	status := b.Status
	isLegit := b.Profile.Preferences.LegitMode
	shouldEnableInvis := !b.Profile.Preferences.Invisible && !isLegit && !isFarming
	if shouldEnableInvis {
		b.Profile.Preferences.Invisible = true
		b.invisToggledByWaypoint = true
	}
	b.mu.Unlock()

	if shouldEnableInvis {
		go b.broadcastLook()
		b.emitLog("action", "Waypoint transit: Auto-invisibility ENABLED", "info")
	}

	dist := float32(math.Sqrt(float64((x-botX)*(x-botX) + (z-botZ)*(z-botZ))))
	statusMsg := fmt.Sprintf("Navigating to waypoint (%.0fm)", dist)
	if !isFarming {
		b.emitLog("action", fmt.Sprintf("Waypoint set to (X: %.1f, Z: %.1f) â€¢ %.0fm away", x, z, dist), "info")
	}
	if b.onUpdate != nil {
		b.onUpdate(b.ID, currentStats, status, statusMsg)
	}

	// Trigger fallback navigation pipeline: Direct Warp -> Portal Warp -> Flight Mode
	// In Legit Mode or during active farm hunting, do not warp! The 250ms movement loop smoothly runs step-by-step
	if !isLegit && !isFarming {
		go b.executeWaypointNavigation(x, z)
	}
}

func (b *BotInstance) ClearWaypoint() {
	b.waypointMu.Lock()
	had := b.hasWaypoint
	b.hasWaypoint = false
	b.waypointFlyMode = false
	b.waypointMu.Unlock()

	if had {
		b.partyMu.Lock()
		b.partyLeashed = false
		b.partyMu.Unlock()

		b.mu.Lock()
		b.Stats.Waypoint = nil
		b.StatusMessage = "Session active"
		revertInvis := b.invisToggledByWaypoint
		if revertInvis {
			b.Profile.Preferences.Invisible = false
			b.invisToggledByWaypoint = false
		}
		currentStats := b.Stats
		status := b.Status
		b.mu.Unlock()

		if revertInvis {
			go b.broadcastLook()
			b.emitLog("action", "Waypoint transit canceled: Invisibility restored to OFF", "info")
		}

		b.StopMove()
		if b.Farm == nil || !b.Farm.IsRunning() {
			b.emitLog("action", "Waypoint cleared â€¢ Stopped", "info")
		}
		if b.onUpdate != nil {
			b.onUpdate(b.ID, currentStats, status, "Waypoint cleared")
		}
	}
}

func (b *BotInstance) executeWaypointNavigation(targetX, targetZ float32) {
	b.mu.Lock()
	if b.Status != "online" || b.ws == nil || !b.ws.IsConnected() {
		b.mu.Unlock()
		return
	}
	botX := b.Stats.PosX
	botZ := b.Stats.PosZ
	b.mu.Unlock()

	currentMap := DetermineMap(botX, botZ)
	targetMap := DetermineMap(targetX, targetZ)

	// Scenario A: Same Map Pathfinding
	if strings.EqualFold(currentMap, targetMap) {
		b.emitLog("action", fmt.Sprintf("Same-map waypoint (%s). Trying Direct Warp...", currentMap), "info")
		if b.attemptWarpTo(targetX, targetZ, "Direct Destination Warp") {
			b.onWaypointArrived(targetX, targetZ)
			return
		}

		// Rejected by server: Fallback to Flight Mode
		b.emitLog("action", "Direct warp rejected by server. Fallback: Engaging Flight Mode (noclip)...", "warn")
		b.waypointMu.Lock()
		b.waypointFlyMode = true
		b.waypointMu.Unlock()
		return
	}

	// Scenario B: Cross-Map / Different Map Pathfinding
	b.emitLog("action", fmt.Sprintf("Cross-map waypoint: %s -> %s. Switching map...", currentMap, targetMap), "info")
	mapPkt := map[string]interface{}{
		"t":   "map",
		"map": targetMap,
	}
	if data, err := json.Marshal(mapPkt); err == nil {
		_ = b.ws.Send(data)
	}
	time.Sleep(300 * time.Millisecond)

	b.emitLog("action", fmt.Sprintf("Trying Direct Warp on %s...", targetMap), "info")
	if b.attemptWarpTo(targetX, targetZ, fmt.Sprintf("%s Direct Warp", targetMap)) {
		b.onWaypointArrived(targetX, targetZ)
		return
	}

	// Direct warp rejected on target map: Find proper teleportation lodestar / portal on target map
	ls := FindClosestLodestar(targetMap, targetX, targetZ)
	if ls != nil {
		b.emitLog("action", fmt.Sprintf("Direct warp rejected. Warping to proper map portal: %s (%.1f, %.1f)...", ls.Name, ls.X, ls.Z), "warn")
		if b.attemptWarpTo(ls.X, ls.Z, ls.Name) {
			time.Sleep(300 * time.Millisecond)
			b.emitLog("action", fmt.Sprintf("At %s. Re-trying Direct Warp to destination...", ls.Name), "info")
			if b.attemptWarpTo(targetX, targetZ, "Destination Warp from Portal") {
				b.onWaypointArrived(targetX, targetZ)
				return
			}
		}
	}

	// Final Fallback: Flight Mode (noclip) straight from portal to destination
	b.emitLog("action", "Map warp rejected. Fallback: Engaging Flight Mode (noclip) straight to destination...", "warn")
	b.waypointMu.Lock()
	b.waypointFlyMode = true
	b.waypointMu.Unlock()
}

func (b *BotInstance) attemptWarpTo(x, z float32, label string) bool {
	t0 := time.Now()

	b.lastSetPosMu.Lock()
	b.lastSetPos = nil
	b.lastSetPosMu.Unlock()

	warpPkt := map[string]interface{}{
		"t": "warp",
		"x": x,
		"z": z,
	}
	if data, err := json.Marshal(warpPkt); err == nil {
		_ = b.ws.Send(data)
	}

	y := stanceY(x, z, false, 0)
	nowMs := time.Now().UnixMilli()

	b.mu.Lock()
	lvl := b.Stats.Level
	b.Stats.PosX = x
	b.Stats.PosY = y
	b.Stats.PosZ = z
	b.Stats.Zone = models.DetermineZone("madrigal", x, z)
	currentStats := b.Stats
	b.mu.Unlock()

	stPkt := map[string]interface{}{
		"t":   "st",
		"p":   []float32{x, y, z},
		"r":   float32(0),
		"a":   "stand",
		"fly": false,
		"lv":  lvl,
		"hp":  1.0,
		"ts":  nowMs,
	}
	if data, err := json.Marshal(stPkt); err == nil {
		_ = b.ws.Send(data)
	}

	if b.onUpdate != nil {
		b.onUpdate(b.ID, currentStats, "online", fmt.Sprintf("Warping to %s", label))
	}

	time.Sleep(350 * time.Millisecond)

	b.lastSetPosMu.Lock()
	rejected := b.lastSetPos != nil && b.lastSetPosTime.After(t0)
	b.lastSetPosMu.Unlock()

	if rejected {
		b.emitLog("action", fmt.Sprintf("Server rejected warp to %s (setpos received)", label), "warn")
		return false
	}

	b.mu.Lock()
	curX := b.Stats.PosX
	curZ := b.Stats.PosZ
	b.mu.Unlock()

	dx := curX - x
	dz := curZ - z
	if dx*dx+dz*dz <= 100.0 {
		b.emitLog("action", fmt.Sprintf("Warp to %s SUCCESSFUL (Arrived at %.1f, %.1f)", label, curX, curZ), "info")
		return true
	}

	return false
}

func (b *BotInstance) onWaypointArrived(x, z float32) {
	b.partyMu.Lock()
	b.partyLeashed = false
	b.partyMu.Unlock()

	b.waypointMu.Lock()
	b.hasWaypoint = false
	b.waypointFlyMode = false
	b.waypointMu.Unlock()

	b.mu.Lock()
	b.Stats.Waypoint = nil
	b.StatusMessage = "Session active"
	revertInvis := b.invisToggledByWaypoint
	if revertInvis {
		b.Profile.Preferences.Invisible = false
		b.invisToggledByWaypoint = false
	}
	currentStats := b.Stats
	b.mu.Unlock()

	if revertInvis {
		go b.broadcastLook()
		b.emitLog("action", "Waypoint transit reached: Invisibility restored to OFF", "info")
	}

	b.StopMove()
	if b.Farm == nil || !b.Farm.IsRunning() {
		b.emitLog("action", fmt.Sprintf("Arrived at waypoint (X: %.1f, Z: %.1f)", x, z), "info")
	}
	if b.onUpdate != nil {
		b.onUpdate(b.ID, currentStats, "online", "Waypoint reached")
	}
}

func (b *BotInstance) GetWaypoint() (float32, float32, bool) {
	b.waypointMu.RLock()
	defer b.waypointMu.RUnlock()
	return b.waypointX, b.waypointZ, b.hasWaypoint
}

func (b *BotInstance) SetFollowTarget(target string) {
	b.mu.Lock()
	b.followTarget = strings.TrimSpace(target)
	if target != "" {
		b.Stats.Waypoint = nil
	}
	b.mu.Unlock()

	if target != "" {
		b.waypointMu.Lock()
		b.hasWaypoint = false
		b.waypointMu.Unlock()

		b.emitLog("action", fmt.Sprintf("Follow target set: %s (auto-tracking enabled)", target), "info")
		go func(t string) {
			b.playersMu.RLock()
			_, isNearby := b.playerNames[strings.ToLower(t)]
			b.playersMu.RUnlock()

			if !isNearby {
				_ = b.RadarSweepAndWarp(t)
			}
		}(target)
	} else {
		b.emitLog("action", "Follow stopped.", "info")
	}
}

func (b *BotInstance) GetFollowTarget() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.followTarget
}

func (b *BotInstance) Move(dx, dz float32) {
	b.mu.Lock()
	if b.Status != "online" || !b.ws.IsConnected() {
		b.mu.Unlock()
		return
	}
	b.Stats.PosX += dx
	b.Stats.PosZ += dz
	posX := b.Stats.PosX
	posZ := b.Stats.PosZ
	posY := b.Stats.PosY
	newY := stanceY(posX, posZ, false, 0)
	if newY > 0 {
		posY = newY
		b.Stats.PosY = newY
	}
	lvl := b.Stats.Level
	b.Stats.Zone = models.DetermineZone("madrigal", posX, posZ)
	currentStats := b.Stats
	status := b.Status
	statusMsg := b.StatusMessage
	b.mu.Unlock()

	rot := float32(math.Atan2(float64(dx), float64(dz)))
	stPkt := map[string]interface{}{
		"t":   "st",
		"p":   []float32{posX, posY, posZ},
		"r":   rot,
		"a":   "run",
		"fly": false,
		"lv":  lvl,
		"hp":  1.0,
		"ts":  time.Now().UnixMilli(),
	}
	if data, err := json.Marshal(stPkt); err == nil {
		if sendErr := b.ws.Send(data); sendErr != nil {
			b.emitLog("net", fmt.Sprintf("Failed to send move packet: %v", sendErr), "warning")
		}
	}

	if b.onUpdate != nil {
		b.onUpdate(b.ID, currentStats, status, statusMsg)
	}
}

func (b *BotInstance) StopMove() {
	b.waypointMu.Lock()
	b.hasWaypoint = false
	b.waypointFlyMode = false
	b.waypointMu.Unlock()

	b.mu.Lock()
	revertInvis := b.invisToggledByWaypoint
	if revertInvis {
		b.Profile.Preferences.Invisible = false
		b.invisToggledByWaypoint = false
	}
	if b.Status != "online" || !b.ws.IsConnected() {
		b.Stats.Waypoint = nil
		b.mu.Unlock()
		if revertInvis {
			go b.broadcastLook()
		}
		return
	}
	b.Stats.Waypoint = nil
	posX := b.Stats.PosX
	posY := b.Stats.PosY
	posZ := b.Stats.PosZ
	lvl := b.Stats.Level
	currentStats := b.Stats
	status := b.Status
	statusMsg := b.StatusMessage
	b.mu.Unlock()

	if revertInvis {
		go b.broadcastLook()
		b.emitLog("action", "Movement stopped: Invisibility restored to OFF", "info")
	}

	if b.Farm != nil && b.Farm.IsFighting() {
		// Active combat: preserve combat swing animation and facing rotation, do not overwrite with astand
		if b.onUpdate != nil {
			b.onUpdate(b.ID, currentStats, status, statusMsg)
		}
		return
	}

	stopAnim := "stand"
	if b.Farm != nil && b.Farm.IsRunning() {
		stopAnim = "astand"
	}

	stPkt := map[string]interface{}{
		"t":   "st",
		"p":   []float32{float32(math.Round(float64(posX)*100) / 100), float32(math.Round(float64(posY)*100) / 100), float32(math.Round(float64(posZ)*100) / 100)},
		"r":   float32(math.Round(float64(b.currentRot)*1000) / 1000),
		"a":   stopAnim,
		"fly": false,
		"lv":  lvl,
		"hp":  1.0,
		"ts":  time.Now().UnixMilli(),
	}
	if data, err := json.Marshal(stPkt); err == nil {
		if sendErr := b.ws.Send(data); sendErr != nil {
			b.emitLog("net", fmt.Sprintf("Failed to send stop move packet: %v", sendErr), "warning")
		}
	}

	if b.onUpdate != nil {
		b.onUpdate(b.ID, currentStats, status, statusMsg)
	}
}

// BroadcastState sends a "st" state packet syncing position, rotation, and animation to all nearby players
func (b *BotInstance) BroadcastState(anim string, rot float32) {
	b.mu.Lock()
	if b.ws == nil || !b.ws.IsConnected() {
		b.mu.Unlock()
		return
	}
	botX := b.Stats.PosX
	botY := b.Stats.PosY
	botZ := b.Stats.PosZ
	level := b.Stats.Level
	hpRatio := float32(1.0)
	if b.Status == "dead" {
		hpRatio = 0.0
	}
	b.currentRot = rot
	b.mu.Unlock()

	nowMs := time.Now().UnixMilli()
	stPkt := map[string]interface{}{
		"t":   "st",
		"p":   []float32{float32(math.Round(float64(botX)*100) / 100), float32(math.Round(float64(botY)*100) / 100), float32(math.Round(float64(botZ)*100) / 100)},
		"r":   float32(math.Round(float64(rot)*1000) / 1000),
		"a":   anim,
		"fly": false,
		"lv":  level,
		"hp":  float32(math.Round(float64(hpRatio)*100) / 100),
		"ts":  nowMs,
	}
	if data, err := json.Marshal(stPkt); err == nil {
		_ = b.ws.Send(data)
	}
}

// AddExp awards EXP for defeating a monster and checks for level up
func (b *BotInstance) AddExp(amount, monLevel int) {
	if b.telemetry != nil {
		b.telemetry.RecordKill()
	}
	b.mu.Lock()
	if b.Status != "online" {
		b.mu.Unlock()
		return
	}
	curLevel := int(b.Stats.Level)
	if curLevel <= 0 {
		curLevel = 1
	}
	curJob := strings.ToLower(b.Stats.Job)
	b.mu.Unlock()

	// Level gap penalty matching stats.gd:362
	gap := curLevel - monLevel
	v := float64(amount)
	if curJob != "vagrant" && curJob != "" {
		v *= 0.5
	}
	if gap > 0 {
		if gap <= 2 {
			v *= 0.7
		} else if gap <= 4 {
			v *= 0.4
		} else {
			v *= 0.1
		}
	}
	// Exp rate: base 1.0, lord event +10%, etc.
	got := int(math.Round(v * 1.1))
	if got < 1 && amount > 0 {
		got = 1
	}

	b.mu.Lock()
	b.Stats.Exp += uint64(got)
	leveledUp := false
	newLevel := b.Stats.Level
	pointsGot := uint32(0)

	// Level up loop matching stats.gd:383
	for {
		need := models.GetExpNeed(int(newLevel))
		if need <= 0 || b.Stats.Exp < uint64(need) || newLevel >= 120 {
			break
		}
		b.Stats.Exp -= uint64(need)
		newLevel++
		pointsGot += 2 // 2 stat points per level
		leveledUp = true
	}

	b.Stats.Level = newLevel
	b.Stats.StatPoints += pointsGot
	b.Stats.MaxExp = uint64(models.GetExpNeed(int(newLevel)))
	currentStats := b.Stats
	charName := b.Profile.CharacterName
	b.mu.Unlock()

	if b.Farm != nil && b.Farm.IsRunning() {
		b.Farm.OnExpGained(int64(got))
	}

	if leveledUp {
		b.emitLog("system", fmt.Sprintf("ðŸŽ‰ LEVEL UP! %s reached Level %d! (+%d Stat Points)", charName, newLevel, pointsGot), "success")
		b.BroadcastState("astand", b.currentRot)
		go b.SaveCurrentStats()
	} else if b.onUpdate != nil {
		b.onUpdate(b.ID, currentStats, b.Status, b.StatusMessage)
	}
}

// SaveCurrentStats saves the bot's runtime EXP, Level, Penya, and Points to server save file
func (b *BotInstance) SaveCurrentStats() error {
	b.mu.Lock()
	if b.Status != "online" {
		b.mu.Unlock()
		return nil
	}
	charFile := b.charFileName
	token := b.sessionToken
	cli := b.httpCli
	if cli == nil {
		cli = b.newGameHTTPClientLocked()
	}
	curExp := b.Stats.Exp
	curLvl := b.Stats.Level
	curPenya := float64(b.Stats.Gold)
	curPoints := b.Stats.StatPoints
	rawJson := b.saveFiles[charFile]
	b.mu.Unlock()

	if token == "" || charFile == "" || rawJson == "" {
		return nil
	}

	var charData map[string]interface{}
	if err := json.Unmarshal([]byte(rawJson), &charData); err != nil {
		return err
	}

	charData["exp"] = curExp
	charData["level"] = curLvl
	charData["penya"] = curPenya
	charData["points"] = curPoints

	return b.persistCharDataWithInvOption(charData, charFile, token, false)
}

func (b *BotInstance) AdvanceJob(targetJob string) (*models.BotSummary, error) {
	b.mu.Lock()
	charName := b.Profile.CharacterName
	curLvl := b.Stats.Level
	curJob := b.Stats.Job
	jobCfg := b.Profile.Preferences.JobConfig
	cli := b.httpCli
	token := b.sessionToken
	accKey := b.accountKey
	charFile := b.charFileName
	password := b.Profile.Password
	b.mu.Unlock()

	target := strings.ToLower(strings.TrimSpace(targetJob))
	if target == "" {
		if strings.EqualFold(curJob, "vagrant") || curJob == "" {
			target = strings.ToLower(strings.TrimSpace(jobCfg.Target1stJob))
			if target == "" {
				target = "assist"
			}
		} else if models.IsFirstJob(curJob) {
			target = strings.ToLower(strings.TrimSpace(jobCfg.Target2ndJob))
			if target == "" {
				target = "ringmaster"
			}
		} else {
			target = strings.ToLower(curJob)
		}
	}

	if cli == nil {
		cli = b.newGameHTTPClient()
	}

	if token == "" || len(b.saveFiles) == 0 {
		if password == "" {
			return nil, fmt.Errorf("bot %s has no credentials to load character save", b.ID)
		}
		auth, err := cli.Login(b.Profile.Name, password)
		if err != nil {
			errLower := strings.ToLower(err.Error())
			if strings.Contains(errLower, "blocked") || strings.Contains(errLower, "banned") {
				b.mu.Lock()
				b.Status = "account_banned"
				b.StatusMessage = "Account Banned"
				b.mu.Unlock()
				b.emitUpdate("account_banned", "Account Banned")
			}
			return nil, fmt.Errorf("failed to authenticate for job advancement: %w", err)
		}
		token = auth.Token
		b.mu.Lock()
		b.sessionToken = token
		b.httpCli = cli
		b.accountKey = auth.Account.Key
		if b.accountKey == "" {
			b.accountKey = strings.ToLower(b.Profile.Name)
		}
		accKey = b.accountKey
		b.saveFiles = auth.Files
		b.mu.Unlock()
	}

	if accKey == "" {
		accKey = strings.ToLower(b.Profile.Name)
	}

	if charFile == "" {
		b.mu.Lock()
		charFile = models.FindCharacterFileName(b.saveFiles, charName, accKey)
		b.charFileName = charFile
		b.mu.Unlock()
	}

	b.mu.Lock()
	rawJson, exists := b.saveFiles[charFile]
	b.mu.Unlock()

	if !exists || strings.TrimSpace(rawJson) == "" {
		return nil, fmt.Errorf("character save file %s not found", charFile)
	}

	var charData map[string]interface{}
	if err := json.Unmarshal([]byte(rawJson), &charData); err != nil {
		return nil, fmt.Errorf("failed to parse character save json: %w", err)
	}

	curStr := int(toFloat32(charData["str"]))
	curSta := int(toFloat32(charData["sta"]))
	curDex := int(toFloat32(charData["dex"]))
	curInt := int(toFloat32(charData["int"]))
	curPoints := int(toFloat32(charData["points"]))

	if curStr < 15 {
		curStr = 15
	}
	if curSta < 15 {
		curSta = 15
	}
	if curDex < 15 {
		curDex = 15
	}
	if curInt < 15 {
		curInt = 15
	}

	// Refund spent points and reset base stats to 15 (matching stats.gd:554-556)
	spent := (curStr - 15) + (curSta - 15) + (curDex - 15) + (curInt - 15)
	if spent > 0 {
		curPoints += spent
		curStr = 15
		curSta = 15
		curDex = 15
		curInt = 15
	}

	// Try loading build rule from stats.json
	classBuilds, _ := LoadStatsJson()
	matchedBuild, hasBuild := classBuilds[target]

	var addStr, addSta, addDex, addInt int
	autoStatVal := models.DetermineAutoStat(jobCfg)

	if hasBuild {
		autoStatVal = matchedBuild.AutoStat
		if matchedBuild.Mode == "cap" {
			custom := models.CustomStatRatio{
				CapStat:  matchedBuild.CapStat,
				CapValue: matchedBuild.CapValue,
				ThenStat: matchedBuild.ThenStat,
			}
			addStr, addSta, addDex, addInt = models.CalculateStatDistribution(
				models.StatModeThreshold,
				"",
				custom,
				curStr, curSta, curDex, curInt,
				curPoints,
			)
		} else if matchedBuild.Mode == "pure" {
			addStr, addSta, addDex, addInt = models.CalculateStatDistribution(
				models.StatModePreset,
				"pure_"+matchedBuild.CapStat,
				models.CustomStatRatio{},
				curStr, curSta, curDex, curInt,
				curPoints,
			)
		} else {
			addStr, addSta, addDex, addInt = models.CalculateStatDistribution(
				jobCfg.StatMode,
				jobCfg.SelectedPreset,
				jobCfg.CustomRatio,
				curStr, curSta, curDex, curInt,
				curPoints,
			)
		}
	} else {
		addStr, addSta, addDex, addInt = models.CalculateStatDistribution(
			jobCfg.StatMode,
			jobCfg.SelectedPreset,
			jobCfg.CustomRatio,
			curStr, curSta, curDex, curInt,
			curPoints,
		)
	}

	newStr := curStr + addStr
	newSta := curSta + addSta
	newDex := curDex + addDex
	newInt := curInt + addInt
	remPoints := curPoints - (addStr + addSta + addDex + addInt)
	if remPoints < 0 {
		remPoints = 0
	}

	charData["job"] = target
	charData["str"] = newStr
	charData["sta"] = newSta
	charData["dex"] = newDex
	charData["int"] = newInt
	charData["points"] = remPoints
	charData["auto_stat"] = autoStatVal

	starterWpn := models.StarterClassWeapons[target]
	if jobCfg.AutoEquipStarter && starterWpn != "" {
		equipMap, ok := charData["equip"].(map[string]interface{})
		if !ok || equipMap == nil {
			equipMap = make(map[string]interface{})
			charData["equip"] = equipMap
		}
		equipMap["weapon"] = starterWpn

		invList, _ := charData["inv"].([]interface{})
		hasWpn := false
		for _, item := range invList {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemMap["id"] == starterWpn {
					hasWpn = true
					break
				}
			}
		}
		if !hasWpn {
			invList = append(invList, map[string]interface{}{
				"id": starterWpn,
				"n":  1,
			})
			charData["inv"] = invList
		}
	}

	newBytes, err := json.Marshal(charData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode updated character json: %w", err)
	}
	newStrData := string(newBytes)

	if _, err := cli.SaveFiles(token, map[string]string{charFile: newStrData}); err != nil {
		return nil, fmt.Errorf("game server rejected character save: %w", err)
	}

	b.mu.Lock()
	b.saveFiles[charFile] = newStrData
	b.Stats.Job = models.FormatJobName(target)
	b.Stats.STR = uint32(newStr)
	b.Stats.STA = uint32(newSta)
	b.Stats.DEX = uint32(newDex)
	b.Stats.INT = uint32(newInt)
	b.Stats.StatPoints = uint32(remPoints)
	b.Stats.AutoStat = uint32(autoStatVal)
	b.mu.Unlock()

	if b.ws != nil && b.ws.IsConnected() {
		nowMs := time.Now().UnixMilli()
		stPkt := map[string]interface{}{
			"t":   "st",
			"p":   []float32{b.Stats.PosX, b.Stats.PosY, b.Stats.PosZ},
			"r":   float32(0),
			"a":   "stand",
			"fly": false,
			"lv":  curLvl,
			"hp":  1.0,
			"ts":  nowMs,
			"job": target,
		}
		if data, err := json.Marshal(stPkt); err == nil {
			_ = b.ws.Send(data)
		}
	}

	b.emitLog(
		"system",
		fmt.Sprintf(
			"Job change successful: %s promoted to %s! Stats: STR=%d (+%d), STA=%d (+%d), DEX=%d (+%d), INT=%d (+%d). Points left: %d",
			charName,
			models.FormatJobName(target),
			newStr, addStr,
			newSta, addSta,
			newDex, addDex,
			newInt, addInt,
			remPoints,
		),
		"info",
	)

	b.emitUpdate(b.Status, fmt.Sprintf("Promoted to %s", models.FormatJobName(target)))

	summary := b.Summary()
	return &summary, nil
}

func (b *BotInstance) AllocateStats(addStr, addSta, addDex, addInt, autoStat int) (*models.BotSummary, error) {
	b.mu.Lock()
	charName := b.Profile.CharacterName
	cli := b.httpCli
	token := b.sessionToken
	accKey := b.accountKey
	charFile := b.charFileName
	password := b.Profile.Password
	b.mu.Unlock()

	if cli == nil {
		cli = b.newGameHTTPClient()
	}

	if token == "" || len(b.saveFiles) == 0 {
		if password == "" {
			return nil, fmt.Errorf("bot %s has no credentials to load character save", b.ID)
		}
		auth, err := cli.Login(b.Profile.Name, password)
		if err != nil {
			errLower := strings.ToLower(err.Error())
			if strings.Contains(errLower, "blocked") || strings.Contains(errLower, "banned") {
				b.mu.Lock()
				b.Status = "account_banned"
				b.StatusMessage = "Account Banned"
				b.mu.Unlock()
				b.emitUpdate("account_banned", "Account Banned")
			}
			return nil, fmt.Errorf("failed to authenticate for stat allocation: %w", err)
		}
		token = auth.Token
		b.mu.Lock()
		b.sessionToken = token
		b.httpCli = cli
		b.accountKey = auth.Account.Key
		if b.accountKey == "" {
			b.accountKey = strings.ToLower(b.Profile.Name)
		}
		accKey = b.accountKey
		b.saveFiles = auth.Files
		b.mu.Unlock()
	}

	if accKey == "" {
		accKey = strings.ToLower(b.Profile.Name)
	}

	if charFile == "" {
		b.mu.Lock()
		charFile = models.FindCharacterFileName(b.saveFiles, charName, accKey)
		b.charFileName = charFile
		b.mu.Unlock()
	}

	b.mu.Lock()
	rawJson, exists := b.saveFiles[charFile]
	b.mu.Unlock()

	if !exists || strings.TrimSpace(rawJson) == "" {
		return nil, fmt.Errorf("character save file %s not found", charFile)
	}

	var charData map[string]interface{}
	if err := json.Unmarshal([]byte(rawJson), &charData); err != nil {
		return nil, fmt.Errorf("failed to parse character save json: %w", err)
	}

	curStr := int(toFloat32(charData["str"]))
	curSta := int(toFloat32(charData["sta"]))
	curDex := int(toFloat32(charData["dex"]))
	curInt := int(toFloat32(charData["int"]))
	curPoints := int(toFloat32(charData["points"]))

	totalAdd := addStr + addSta + addDex + addInt
	if totalAdd > curPoints {
		return nil, fmt.Errorf("cannot allocate %d points; only %d points available", totalAdd, curPoints)
	}

	newStr := curStr + addStr
	newSta := curSta + addSta
	newDex := curDex + addDex
	newInt := curInt + addInt
	newPoints := curPoints - totalAdd

	charData["str"] = newStr
	charData["sta"] = newSta
	charData["dex"] = newDex
	charData["int"] = newInt
	charData["points"] = newPoints
	if autoStat >= 0 {
		charData["auto_stat"] = autoStat
	}

	newBytes, err := json.Marshal(charData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode updated character json: %w", err)
	}
	newStrData := string(newBytes)

	if _, err := cli.SaveFiles(token, map[string]string{charFile: newStrData}); err != nil {
		return nil, fmt.Errorf("game server rejected character save: %w", err)
	}

	b.mu.Lock()
	b.saveFiles[charFile] = newStrData
	b.Stats.STR = uint32(newStr)
	b.Stats.STA = uint32(newSta)
	b.Stats.DEX = uint32(newDex)
	b.Stats.INT = uint32(newInt)
	b.Stats.StatPoints = uint32(newPoints)
	if autoStat >= 0 {
		b.Stats.AutoStat = uint32(autoStat)
	}
	b.mu.Unlock()

	b.emitLog(
		"action",
		fmt.Sprintf(
			"Stats allocated for %s: STR=%d (+%d), STA=%d (+%d), DEX=%d (+%d), INT=%d (+%d). Remaining Points=%d, AutoStat=%d",
			charName, newStr, addStr, newSta, addSta, newDex, addDex, newInt, addInt, newPoints, autoStat,
		),
		"info",
	)

	b.emitUpdate(b.Status, fmt.Sprintf("Stats updated: %d P left", newPoints))
	summary := b.Summary()
	return &summary, nil
}

func isServerIssue(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "502") ||
		strings.Contains(s, "bad gateway") ||
		strings.Contains(s, "503") ||
		strings.Contains(s, "504") ||
		strings.Contains(s, "bad handshake") ||
		strings.Contains(s, "gateway") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connection reset")
}

func (b *BotInstance) getCharSlot() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.charSlot >= 0 {
		return b.charSlot
	}
	parts := strings.Split(b.charFileName, "_")
	if len(parts) >= 2 {
		slotPart := strings.TrimSuffix(parts[len(parts)-1], ".json")
		if s, err := strconv.Atoi(slotPart); err == nil {
			return s
		}
	}
	return 0
}

func (b *BotInstance) loadCharData() (map[string]interface{}, string, string, error) {
	b.mu.Lock()
	cli := b.httpCli
	token := b.sessionToken
	charFile := b.charFileName
	password := b.Profile.Password
	accKey := b.accountKey
	charName := b.Profile.CharacterName
	b.mu.Unlock()

	if cli == nil {
		cli = b.newGameHTTPClient()
	}

	b.mu.Lock()
	if token == "" || len(b.saveFiles) == 0 {
		if password != "" {
			b.mu.Unlock()
			auth, err := cli.Login(b.Profile.Name, password)
			if err == nil {
				b.mu.Lock()
				b.sessionToken = auth.Token
				token = auth.Token
				if auth.Account.Key != "" {
					b.accountKey = auth.Account.Key
					accKey = auth.Account.Key
				}
				b.saveFiles = auth.Files
				b.mu.Unlock()
			} else {
				errLower := strings.ToLower(err.Error())
				if strings.Contains(errLower, "blocked") || strings.Contains(errLower, "banned") {
					b.mu.Lock()
					b.Status = "account_banned"
					b.StatusMessage = "Account Banned"
					b.mu.Unlock()
					b.emitUpdate("account_banned", "Account Banned")
				}
				b.mu.Lock()
			}
		} else {
			b.mu.Unlock()
		}
	} else {
		b.mu.Unlock()
	}

	if accKey == "" {
		accKey = strings.ToLower(b.Profile.Name)
	}

	if charFile == "" {
		b.mu.Lock()
		charFile = models.FindCharacterFileName(b.saveFiles, charName, accKey)
		b.charFileName = charFile
		b.mu.Unlock()
	}

	b.mu.Lock()
	rawJson, exists := b.saveFiles[charFile]
	b.mu.Unlock()

	if !exists || strings.TrimSpace(rawJson) == "" {
		defaultJson := models.CreateDefaultCharacterSave(charName, "male")
		var d map[string]interface{}
		_ = json.Unmarshal([]byte(defaultJson), &d)
		return d, charFile, token, nil
	}

	b.charCacheMu.Lock()
	if rawJson == b.cachedRawJson && b.cachedCharData != nil {
		cached := cloneMap(b.cachedCharData)
		b.charCacheMu.Unlock()
		return cached, charFile, token, nil
	}
	b.charCacheMu.Unlock()

	var charData map[string]interface{}
	if err := json.Unmarshal([]byte(rawJson), &charData); err != nil {
		return nil, charFile, token, err
	}

	invList, _ := charData["inv"].([]interface{})
	charData["inv"] = NormalizeInventory(invList)
	if normInv, ok := charData["inv"].([]interface{}); ok {
		models.ValidateAndRepairInventory(normInv)
		charData["inv"] = NormalizeInventory(normInv)
	}

	b.charCacheMu.Lock()
	b.cachedRawJson = rawJson
	b.cachedCharData = charData
	cloned := cloneMap(charData)
	b.charCacheMu.Unlock()

	return cloned, charFile, token, nil
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			dst[k] = cloneMap(val)
		case []interface{}:
			dst[k] = cloneSlice(val)
		default:
			dst[k] = val
		}
	}
	return dst
}

func cloneSlice(src []interface{}) []interface{} {
	if src == nil {
		return nil
	}
	dst := make([]interface{}, len(src))
	for i, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			dst[i] = cloneMap(val)
		case []interface{}:
			dst[i] = cloneSlice(val)
		default:
			dst[i] = val
		}
	}
	return dst
}

func (b *BotInstance) persistCharData(charData map[string]interface{}, charFile, token string) error {
	return b.persistCharDataWithInvOption(charData, charFile, token, true)
}

func (b *BotInstance) persistCharDataWithInvOption(charData map[string]interface{}, charFile, token string, emitInv bool) error {
	if invList, ok := charData["inv"].([]interface{}); ok {
		models.ValidateAndRepairInventory(invList)
		charData["inv"] = NormalizeInventory(invList)
	}

	newBytes, err := json.Marshal(charData)
	if err != nil {
		return err
	}
	newStr := string(newBytes)

	b.mu.Lock()
	if b.saveFiles == nil {
		b.saveFiles = make(map[string]string)
	}
	b.saveFiles[charFile] = newStr
	b.saveDirty = true
	b.mu.Unlock()

	effPenya := b.getEffectivePenya(charData)
	b.mu.Lock()
	b.Stats.Gold = uint64(effPenya)
	b.mu.Unlock()

	b.charCacheMu.Lock()
	b.cachedRawJson = newStr
	b.cachedCharData = charData
	b.charCacheMu.Unlock()

	if emitInv {
		if _, ok := charData["inv"]; ok {
			b.emitInventoryUpdate(nil)
		}
	}
	return nil
}

func (b *BotInstance) getEffectivePenyaLocked(charData map[string]interface{}) float64 {
	var filePenya float64
	if charData != nil {
		if pVal, exists := charData["penya"]; exists && pVal != nil {
			switch v := pVal.(type) {
			case float64:
				filePenya = v
			case float32:
				filePenya = float64(v)
			case int:
				filePenya = float64(v)
			case int64:
				filePenya = float64(v)
			case uint64:
				filePenya = float64(v)
			case uint:
				filePenya = float64(v)
			case int32:
				filePenya = float64(v)
			case uint32:
				filePenya = float64(v)
			case json.Number:
				if n, err := v.Float64(); err == nil {
					filePenya = n
				}
			}
		}
	}

	statsGold := float64(b.Stats.Gold)
	if statsGold > filePenya {
		return statsGold
	}
	if filePenya > 0 {
		return filePenya
	}
	return statsGold
}

func (b *BotInstance) getEffectivePenya(charData map[string]interface{}) float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.getEffectivePenyaLocked(charData)
}

func (b *BotInstance) setEffectivePenya(charData map[string]interface{}, newPenya float64) {
	if newPenya < 0 {
		newPenya = 0
	}
	if charData != nil {
		charData["penya"] = newPenya
	}
	b.mu.Lock()
	b.Stats.Gold = uint64(newPenya)
	b.mu.Unlock()
}

func (b *BotInstance) startPeriodicSaver() {
	b.mu.Lock()
	stopCh := b.cancelCh
	b.mu.Unlock()

	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				b.flushSave(true)
				return
			case <-ticker.C:
				b.flushSave(false)
			}
		}
	}()
}

func (b *BotInstance) flushSave(force bool) {
	b.saveTxMu.Lock()
	defer b.saveTxMu.Unlock()

	b.mu.Lock()
	if b.saveStopped || atomic.LoadInt32(&b.activeClaims) > 0 {
		b.mu.Unlock()
		return
	}
	if !force && (b.Status != "online" || !b.saveDirty || b.saveInFlight) {
		b.mu.Unlock()
		return
	}
	if b.saveInFlight {
		b.mu.Unlock()
		return
	}
	token := b.sessionToken
	charFile := b.charFileName
	cli := b.httpCli
	if cli == nil {
		cli = b.newGameHTTPClientLocked()
	}
	if token == "" || charFile == "" || len(b.saveFiles) == 0 {
		b.mu.Unlock()
		return
	}

	curData, exists := b.saveFiles[charFile]
	if !exists || curData == "" {
		b.mu.Unlock()
		return
	}

	if b.lastSavedFiles == nil {
		b.lastSavedFiles = make(map[string]string)
	}
	if !force && curData == b.lastSavedFiles[charFile] {
		b.saveDirty = false
		b.mu.Unlock()
		return
	}

	b.saveInFlight = true
	b.mu.Unlock()

	// Execute HTTP POST OUTSIDE b.mu
	payload := map[string]string{charFile: curData}
	res, err := cli.SaveFiles(token, payload)

	b.mu.Lock()
	b.saveInFlight = false
	if err == nil && (res == nil || res.Error == "") {
		b.saveDirty = false
		b.saveFailCount = 0
		b.lastSavedFiles[charFile] = curData
		if b.lastAckedSave == nil {
			b.lastAckedSave = make(map[string]string)
		}
		b.lastAckedSave[charFile] = curData
		b.saveRefusedShown = ""
		b.preMutationInv = nil
		b.lastMutatedIDs = nil
		b.mu.Unlock()
		return
	}

	// Error handling
	b.saveFailCount++
	b.lastSaveFailureTime = time.Now()
	failCount := b.saveFailCount

	errMsg := ""
	resync := 0
	if res != nil {
		errMsg = res.Error
		resync = res.Resync
	} else if err != nil {
		errMsg = err.Error()
	}
	b.mu.Unlock()

	// Surgical rollback check: revert only inventory mutation, preserve WebSocket state
	if (resync == 1 || strings.Contains(errMsg, "Save refused")) && b.trySurgicalRollback(charFile, errMsg) {
		return
	}

	if failCount >= 3 {
		b.emitLog("net", fmt.Sprintf("[SAVE ALARM] %d consecutive saves failed/timed out! Last error: %s", failCount, errMsg), "error")
		b.emitUpdate(b.Status, fmt.Sprintf("Save warning: %d consecutive failures", failCount))
		if b.telemetry != nil {
			b.telemetry.DumpContext(fmt.Sprintf("Save alarm: %d consecutive save failures (last: %s)", failCount, errMsg))
		}
	} else {
		b.emitLog("net", fmt.Sprintf("[SAVE ERROR] /api/save returned: %s (resync=%d, failures=%d)", errMsg, resync, failCount), "warn")
	}

	if resync == 1 {
		b.handleSaveResync(token, cli)
	}
}

// snapshotPreMutation captures pre-mutation inventory and penya for surgical rollback.
func (b *BotInstance) snapshotPreMutation(charData map[string]interface{}, affectedIDs []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	invList, _ := charData["inv"].([]interface{})
	b.preMutationInv = make([]interface{}, len(invList))
	for i, raw := range invList {
		if m, ok := raw.(map[string]interface{}); ok {
			b.preMutationInv[i] = cloneSlotMap(m)
		} else {
			b.preMutationInv[i] = NewEmptySlot()
		}
	}
	b.preMutationPenya = b.getEffectivePenyaLocked(charData)
	b.lastMutatedIDs = append([]string{}, affectedIDs...)
}

// trySurgicalRollback restores inventory and penya on Save refused without resetting WebSocket state.
func (b *BotInstance) trySurgicalRollback(charFile, errMsg string) bool {
	b.mu.Lock()
	if b.preMutationInv == nil {
		b.mu.Unlock()
		return false
	}

	charData := make(map[string]interface{})
	if raw, exists := b.saveFiles[charFile]; exists && raw != "" {
		_ = json.Unmarshal([]byte(raw), &charData)
	}

	// 1. Revert inventory to pre-mutation snapshot
	revertedInv := make([]interface{}, len(b.preMutationInv))
	for i, slot := range b.preMutationInv {
		if m, ok := slot.(map[string]interface{}); ok {
			revertedInv[i] = cloneSlotMap(m)
		} else {
			revertedInv[i] = NewEmptySlot()
		}
	}

	// Preserve any items that arrived from WS loot between mutation and save refusal
	if currentInv, ok := charData["inv"].([]interface{}); ok {
		mutatedSet := make(map[string]bool)
		for _, id := range b.lastMutatedIDs {
			mutatedSet[id] = true
		}
		for _, rawSlot := range currentInv {
			if m, ok := rawSlot.(map[string]interface{}); ok && !IsEmptySlot(m) {
				id, _ := m["id"].(string)
				if !mutatedSet[id] {
					alreadyPresent := false
					for _, preRaw := range b.preMutationInv {
						if pm, ok := preRaw.(map[string]interface{}); ok && pm["id"] == id {
							alreadyPresent = true
							break
						}
					}
					if !alreadyPresent {
						for fi := 0; fi < len(revertedInv); fi++ {
							if IsEmptySlot(revertedInv[fi]) {
								revertedInv[fi] = cloneSlotMap(m)
								break
							}
						}
					}
				}
			}
		}
	}

	charData["inv"] = NormalizeInventory(revertedInv)
	charData["penya"] = b.preMutationPenya
	b.Stats.Gold = uint64(b.preMutationPenya)

	// Blacklist offending item IDs for this session
	if b.sessionBlacklist == nil {
		b.sessionBlacklist = make(map[string]bool)
	}
	for _, id := range b.lastMutatedIDs {
		if id != "" {
			b.sessionBlacklist[id] = true
			b.emitLog("action", fmt.Sprintf("[ROLLBACK] Item '%s' caused save refusal (%s). Blacklisted for session.", id, errMsg), "warn")
		}
	}

	b.preMutationInv = nil
	b.lastMutatedIDs = nil
	b.saveFailCount = 0

	newBytes, _ := json.Marshal(charData)
	b.saveFiles[charFile] = string(newBytes)
	b.saveDirty = true
	b.mu.Unlock()

	b.emitLog("net", fmt.Sprintf("[ROLLBACK] Surgical rollback applied on save refusal: %s. Preserved WebSocket combat/position state.", errMsg), "warn")
	b.emitInventoryUpdate(nil)
	return true
}

// executeClaimTransaction executes /api/claim with pre-claim flush barrier and mutex serialization.
func (b *BotInstance) executeClaimTransaction(token string, payload map[string]interface{}) (map[string]interface{}, error) {
	// Step 1: Pre-claim flush barrier
	b.mu.Lock()
	dirty := b.saveDirty
	b.mu.Unlock()
	if dirty {
		b.flushSave(true)
	}

	// Step 2: Strict Mutex serialization - blocks if flushSave is currently in flight
	b.saveTxMu.Lock()
	defer b.saveTxMu.Unlock()

	atomic.AddInt32(&b.activeClaims, 1)
	defer atomic.AddInt32(&b.activeClaims, -1)

	b.mu.Lock()
	cli := b.httpCli
	if cli == nil {
		cli = b.newGameHTTPClientLocked()
	}
	b.mu.Unlock()

	res, err := cli.Claim(token, payload)
	if err != nil {
		return nil, err
	}
	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	return res, nil
}

func (b *BotInstance) handleSaveResync(token string, cli *client.GameHttpClient) {
	b.mu.Lock()
	if b.resyncing {
		b.mu.Unlock()
		return
	}
	if !b.lastResyncTime.IsZero() && time.Since(b.lastResyncTime) < 30*time.Second {
		b.mu.Unlock()
		b.emitLog("net", "[RESYNC] Cooldown active (<30s since last resync), skipping resync attempt.", "warn")
		return
	}
	if b.resyncCount >= 3 {
		b.saveStopped = true
		b.saveDirty = false
		b.Status = "error"
		b.StatusMessage = "Save halted: max resync attempts (3) exceeded"
		msg := b.StatusMessage
		b.mu.Unlock()
		b.emitLog("net", "[RESYNC ERROR] Max resync attempts (3) reached. Halting auto-save to prevent loop.", "error")
		b.emitUpdate("error", msg)
		return
	}

	b.resyncing = true
	b.resyncCount++
	b.lastResyncTime = time.Now()
	b.saveDirty = false // stop retrying rejected payload
	attempt := b.resyncCount
	charFile := b.charFileName
	b.mu.Unlock()

	b.emitLog("net", fmt.Sprintf("[RESYNC] Out of step with server (resync: 1, attempt %d/3). Reloading character state via /api/session...", attempt), "warn")

	auth, err := cli.ResumeSession(token)
	if err == nil && auth != nil && len(auth.Files) > 0 {
		b.mu.Lock()
		// Validate that equip and inventory are intact in server response
		if serverCharJson, exists := auth.Files[charFile]; exists && strings.TrimSpace(serverCharJson) != "" {
			var charData map[string]interface{}
			if jsonErr := json.Unmarshal([]byte(serverCharJson), &charData); jsonErr == nil {
				if invList, ok := charData["inv"].([]interface{}); ok && len(invList) > 0 {
					models.ValidateAndRepairInventory(invList)
					charData["inv"] = invList
					if repairedBytes, mErr := json.Marshal(charData); mErr == nil {
						auth.Files[charFile] = string(repairedBytes)
					}
				}
				if penya, ok := charData["penya"].(float64); ok {
					b.Stats.Gold = uint64(penya)
				}
			}
		}
		b.saveFiles = auth.Files
		b.lastSavedFiles = make(map[string]string)
		for k, v := range auth.Files {
			b.lastSavedFiles[k] = v
		}
		b.saveDirty = false
		b.resyncing = false
		b.saveFailCount = 0
		b.mu.Unlock()
		b.emitLog("net", fmt.Sprintf("[RESYNC] Session state restored from server. Equip and inventory preserved (attempt %d/3).", attempt), "info")
		b.emitInventoryUpdate(nil)
		return
	}

	b.emitLog("net", fmt.Sprintf("[RESYNC] /api/session failed (%v), terminating session...", err), "warn")
	b.mu.Lock()
	b.resyncing = false
	b.mu.Unlock()
	_ = b.Stop()
}

func (b *BotInstance) GetInventoryData() (map[string]interface{}, error) {
	charData, _, _, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	res := map[string]interface{}{
		"penya":        charData["penya"],
		"inv":          charData["inv"],
		"equip":        charData["equip"],
		"equip_up":     charData["equip_up"],
		"pockets":      charData["pockets"],
		"pocket_until": charData["pocket_until"],
	}
	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	return res, nil
}

func (b *BotInstance) ToggleItemLock(slotIndex int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok || slotIndex < 0 || slotIndex >= len(invList) {
		return nil, fmt.Errorf("invalid slot index %d", slotIndex)
	}

	itemMap, ok := invList[slotIndex].(map[string]interface{})
	if !ok || len(itemMap) == 0 {
		return nil, fmt.Errorf("slot %d is empty", slotIndex)
	}

	curLock, _ := itemMap["lk"].(bool)
	itemMap["lk"] = !curLock

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	return b.GetInventoryData()
}

func (b *BotInstance) ToggleItemJunk(slotIndex int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok || slotIndex < 0 || slotIndex >= len(invList) {
		return nil, fmt.Errorf("invalid slot index %d", slotIndex)
	}

	itemMap, ok := invList[slotIndex].(map[string]interface{})
	if !ok || len(itemMap) == 0 {
		return nil, fmt.Errorf("slot %d is empty", slotIndex)
	}

	curJunk, _ := itemMap["jk"].(bool)
	itemMap["jk"] = !curJunk

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	return b.GetInventoryData()
}

func (b *BotInstance) EquipItem(slotIndex int, targetSlot string) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	actualSlot, err := EquipFromInventory(charData, slotIndex, targetSlot)
	if err != nil {
		return nil, err
	}

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	b.emitLog("action", fmt.Sprintf("Equipped into %s", actualSlot), "info")
	go b.broadcastLook()
	return b.GetInventoryData()
}

func (b *BotInstance) UnequipItem(slotKey string) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	if err := UnequipToInventory(charData, slotKey); err != nil {
		return nil, err
	}

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	b.emitLog("action", fmt.Sprintf("Unequipped from %s", slotKey), "info")
	go b.broadcastLook()
	return b.GetInventoryData()
}

func (b *BotInstance) DropItem(slotIndex int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok || slotIndex < 0 || slotIndex >= len(invList) {
		return nil, fmt.Errorf("invalid slot index %d", slotIndex)
	}

	itemMap, _ := invList[slotIndex].(map[string]interface{})
	if itemMap != nil && itemMap["lk"] == true {
		return nil, fmt.Errorf("item is locked")
	}

	invList[slotIndex] = map[string]interface{}{}
	charData["inv"] = invList

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	b.emitLog("action", fmt.Sprintf("Destroyed item in slot %d", slotIndex), "info")
	return b.GetInventoryData()
}

func (b *BotInstance) SortInventory(by string, keepLocked bool) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok || len(invList) == 0 {
		return b.GetInventoryData()
	}

	var unlocked []map[string]interface{}
	fixed := make(map[int]map[string]interface{})

	for i, it := range invList {
		im, ok := it.(map[string]interface{})
		if !ok || len(im) == 0 || im["id"] == "" {
			continue
		}
		if keepLocked && im["lk"] == true {
			fixed[i] = im
		} else {
			unlocked = append(unlocked, im)
		}
	}

	sort.SliceStable(unlocked, func(i, j int) bool {
		idA, _ := unlocked[i]["id"].(string)
		idB, _ := unlocked[j]["id"].(string)
		return idA < idB
	})

	newInv := make([]interface{}, 112)
	for i := range newInv {
		newInv[i] = map[string]interface{}{}
	}

	uIdx := 0
	for i := 0; i < 112; i++ {
		if it, isFixed := fixed[i]; isFixed {
			newInv[i] = it
		} else if uIdx < len(unlocked) {
			newInv[i] = unlocked[uIdx]
			uIdx++
		}
	}

	charData["inv"] = newInv
	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	b.emitLog("action", fmt.Sprintf("Sorted inventory (%s)", by), "info")
	return b.GetInventoryData()
}

func (b *BotInstance) RedeemCode(code string) (map[string]interface{}, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("redeem code cannot be empty")
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("bot session token unavailable; please log in or reconnect first")
	}

	// Ensure inventory has at least 240 slots with {} empty slots matching official client
	invList, _ := charData["inv"].([]interface{})
	if invList == nil {
		invList = make([]interface{}, 0, 240)
	}
	for len(invList) < 240 {
		invList = append(invList, map[string]interface{}{})
	}
	charData["inv"] = invList

	// Synchronize inventory slots to server before redeem so server can find a free slot
	_ = b.persistCharData(charData, charFile, token)

	charSlot := b.getCharSlot()
	cli := b.newGameHTTPClient()
	res, err := cli.RedeemCode(token, code, charSlot)
	if err != nil {
		b.emitLog("system", fmt.Sprintf("Redeem failed for code '%s': %v", code, err), "error")
		b.emitChat("error", "", fmt.Sprintf("Failed to redeem code '%s': %v", code, err), "#ff7366")
		return nil, err
	}

	if okVal, exists := res["ok"].(bool); exists && !okVal {
		errMsg, _ := res["error"].(string)
		if errMsg == "" {
			errMsg = "Server rejected redeem code"
		}
		b.emitLog("system", fmt.Sprintf("Redeem failed for code '%s': %s", code, errMsg), "error")
		b.emitChat("error", "", fmt.Sprintf("Failed to redeem code '%s': %s", code, errMsg), "#ff7366")
		return nil, fmt.Errorf("%s", errMsg)
	}

	var addedItems []string
	if itemsRaw, ok := res["items"].([]interface{}); ok {
		for _, itRaw := range itemsRaw {
			if itArr, ok := itRaw.([]interface{}); ok && len(itArr) >= 2 {
				itemID, _ := itArr[0].(string)
				itemCount := int(toFloat32(itArr[1]))
				if itemID != "" {
					if itemCount <= 0 {
						itemCount = 1
					}
					addItemToInventory(invList, itemID, itemCount)
					addedItems = append(addedItems, fmt.Sprintf("%s x%d", itemID, itemCount))
				}
			}
		}
	}

	if penya, ok := res["penya"].(float64); ok && penya > 0 {
		curPenya := b.getEffectivePenya(charData)
		b.setEffectivePenya(charData, curPenya+penya)
		addedItems = append(addedItems, fmt.Sprintf("%d Penya", int64(penya)))
	}

	charData["inv"] = invList
	_ = b.persistCharData(charData, charFile, token)

	b.emitLog("system", fmt.Sprintf("Successfully redeemed code '%s'!", code), "info")
	if len(addedItems) > 0 {
		b.emitChat("notice", "", fmt.Sprintf("Gift code '%s' redeemed! Received: %s", code, strings.Join(addedItems, ", ")), "#ffcc66")
	} else {
		b.emitChat("notice", "", fmt.Sprintf("Gift code '%s' successfully redeemed!", code), "#ffcc66")
	}
	b.emitInventoryUpdate(nil)
	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	return res, nil
}

func (b *BotInstance) handleLootReceived(penya float64, itemsRaw []interface{}) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return
	}

	invList, _ := charData["inv"].([]interface{})
	if invList == nil {
		invList = make([]interface{}, 0, 240)
	}
	for len(invList) < 240 {
		invList = append(invList, map[string]interface{}{})
	}

	penyaAdded := false
	if penya > 0 {
		curPenya := b.getEffectivePenya(charData)
		b.setEffectivePenya(charData, curPenya+penya)
		b.emitChat("battle", "Battle", fmt.Sprintf("You got %d Penya.", int64(penya)), "#d9d9d9")
		penyaAdded = true
	}

	itemsAdded := false
	for _, itRaw := range itemsRaw {
		if itArr, ok := itRaw.([]interface{}); ok && len(itArr) >= 2 {
			itemID, _ := itArr[0].(string)
			count := int(toFloat32(itArr[1]))
			up := 0
			if len(itArr) >= 3 {
				up = int(toFloat32(itArr[2]))
			}
			if itemID != "" {
				if count <= 0 {
					count = 1
				}
				if addItemToInventoryWithUp(invList, itemID, count, up) {
					itemsAdded = true
					itemDesc := itemID
					if up > 0 {
						itemDesc = fmt.Sprintf("%s +%d", itemID, up)
					}
					if count > 1 {
						itemDesc = fmt.Sprintf("%s x%d", itemDesc, count)
					}
					b.emitChat("battle", "Battle", fmt.Sprintf("You picked up %s.", itemDesc), "#70f270")
				}
			}
		}
	}

	if penyaAdded || itemsAdded {
		charData["inv"] = invList
		_ = b.persistCharData(charData, charFile, token)
		if penyaAdded {
			b.mu.Lock()
			st := b.Stats
			b.mu.Unlock()
			b.emitUpdate(b.Status, b.StatusMessage)
			if b.onUpdate != nil {
				b.onUpdate(b.ID, st, b.Status, b.StatusMessage)
			}
		}
	}
}

func addItemToInventoryWithUp(invList []interface{}, itemID string, count int, up int) bool {
	if count <= 0 {
		count = 1
	}
	stack := models.GetItemStackLimit(itemID)
	if stack <= 0 {
		stack = 1
	}

	// 1. Stackable only if up == 0 and stack > 1 (matching stats.gd:586)
	if up == 0 && stack > 1 {
		for i, raw := range invList {
			im, ok := raw.(map[string]interface{})
			if !ok || len(im) == 0 {
				continue
			}
			curUp := toInt(im["up"])
			if id, _ := im["id"].(string); id == itemID && curUp == 0 {
				curN := toInt(im["n"])
				if curN <= 0 {
					curN = 1
				}
				if curN < stack {
					put := count
					if put > (stack - curN) {
						put = stack - curN
					}
					im["n"] = curN + put
					invList[i] = im
					count -= put
					if count <= 0 {
						return true
					}
				}
			}
		}
	}

	// 2. Put remaining count into free slots, at most stack per slot (matching stats.gd:591-596)
	for count > 0 {
		freeIdx := -1
		for i, raw := range invList {
			im, ok := raw.(map[string]interface{})
			if !ok || len(im) == 0 || im["id"] == nil || im["id"] == "" {
				freeIdx = i
				break
			}
		}
		put := count
		if put > stack {
			put = stack
		}
		newSlot := map[string]interface{}{
			"id": itemID,
			"n":  put,
		}
		if up > 0 {
			newSlot["up"] = up
		}
		if freeIdx >= 0 {
			invList[freeIdx] = newSlot
		} else if len(invList) < 240 {
			invList = append(invList, newSlot)
		} else {
			return false
		}
		count -= put
	}
	return true
}

func addItemToInventory(invList []interface{}, itemID string, count int) bool {
	return addItemToInventoryWithUp(invList, itemID, count, 0)
}

func (b *BotInstance) checkAndClaimDailyGift() {
	// Stagger daily gift claim checks with 3s-32s jitter to prevent thundering herd
	jitter := 3*time.Second + time.Duration(time.Now().UnixNano()%30)*time.Second
	time.Sleep(jitter)
	b.mu.Lock()
	autoClaim := b.Profile.Preferences.AutoClaimDailyGifts
	online := b.Status == "online"
	b.mu.Unlock()
	if !autoClaim || !online {
		return
	}

	_, _ = b.ClaimDailyGift()
}

func (b *BotInstance) ClaimDailyGift() (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		}, nil
	}
	if token == "" {
		return map[string]interface{}{
			"ok":    false,
			"error": "bot session token unavailable; please log in or reconnect first",
		}, nil
	}

	todayStr := time.Now().Format("2006-01-02")
	rewardsFile := strings.TrimSuffix(charFile, ".json") + "_rewards.json"
	b.mu.Lock()
	rawRewards, hasRewards := b.saveFiles[rewardsFile]
	b.mu.Unlock()
	if hasRewards && rawRewards != "" {
		var rState map[string]interface{}
		if json.Unmarshal([]byte(rawRewards), &rState) == nil {
			if lastDate, _ := rState["last"].(string); lastDate == todayStr {
				dayNum := 1
				if d, ok := rState["day"].(float64); ok {
					dayNum = int(d)
				}
				return map[string]interface{}{
					"ok":             true,
					"claimed":        true,
					"alreadyClaimed": true,
					"day":            dayNum,
					"message":        fmt.Sprintf("Daily login gift already claimed today (Day %d). Come back tomorrow!", dayNum),
				}, nil
			}
		}
	}

	charSlot := b.getCharSlot()
	payload := map[string]interface{}{
		"k":    "daily",
		"char": charSlot,
	}

	res, err := b.executeClaimTransaction(token, payload)
	if err != nil {
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "already") || strings.Contains(errLower, "claimed") || strings.Contains(errLower, "today") || strings.Contains(errLower, "wait") {
			rewardsState := map[string]interface{}{
				"last": todayStr,
			}
			if rBytes, mErr := json.Marshal(rewardsState); mErr == nil {
				b.mu.Lock()
				b.saveFiles[rewardsFile] = string(rBytes)
				b.mu.Unlock()
			}
			b.emitLog("system", "Daily login gift already claimed today.", "info")
			return map[string]interface{}{
				"ok":             true,
				"claimed":        true,
				"alreadyClaimed": true,
				"message":        "Daily login gift already claimed today.",
			}, nil
		}
		b.emitLog("system", fmt.Sprintf("Daily gift claim failed: %v", err), "warn")
		return map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		}, nil
	}

	if okVal, exists := res["ok"].(bool); exists && !okVal {
		errMsg, _ := res["error"].(string)
		if errMsg == "" {
			errMsg = "Server rejected daily gift claim"
		}
		errLower := strings.ToLower(errMsg)
		if strings.Contains(errLower, "already") || strings.Contains(errLower, "claimed") || strings.Contains(errLower, "today") || strings.Contains(errLower, "wait") {
			rewardsState := map[string]interface{}{
				"last": todayStr,
			}
			if rBytes, mErr := json.Marshal(rewardsState); mErr == nil {
				b.mu.Lock()
				b.saveFiles[rewardsFile] = string(rBytes)
				b.mu.Unlock()
			}
			b.emitLog("system", "Daily login gift already claimed today.", "info")
			return map[string]interface{}{
				"ok":             true,
				"claimed":        true,
				"alreadyClaimed": true,
				"message":        "Daily login gift already claimed today.",
			}, nil
		}
		b.emitLog("system", fmt.Sprintf("Daily gift claim failed: %s", errMsg), "warn")
		return map[string]interface{}{
			"ok":    false,
			"error": errMsg,
		}, nil
	}

	dayNum := 1
	if d, ok := res["day"].(float64); ok {
		dayNum = int(d)
	}

	var addedItems []string
	invList, _ := charData["inv"].([]interface{})
	if invList == nil {
		invList = make([]interface{}, 112)
		for i := range invList {
			invList[i] = map[string]interface{}{}
		}
	}

	if itemsRaw, ok := res["items"].([]interface{}); ok {
		for _, itRaw := range itemsRaw {
			if itArr, ok := itRaw.([]interface{}); ok && len(itArr) >= 2 {
				itemID, _ := itArr[0].(string)
				itemCount := int(toFloat32(itArr[1]))
				if itemID != "" {
					if itemCount <= 0 {
						itemCount = 1
					}
					addItemToInventory(invList, itemID, itemCount)
					addedItems = append(addedItems, fmt.Sprintf("%s x%d", itemID, itemCount))
				}
			}
		}
	}

	if penya, ok := res["penya"].(float64); ok && penya > 0 {
		curPenya := b.getEffectivePenya(charData)
		b.setEffectivePenya(charData, curPenya+penya)
		addedItems = append(addedItems, fmt.Sprintf("%d Penya", int64(penya)))
	}

	charData["inv"] = invList
	_ = b.persistCharData(charData, charFile, token)

	// Save _rewards.json state matching official Echoes of Madrigal client
	todayStr = time.Now().Format("2006-01-02")
	rewardsFile = strings.TrimSuffix(charFile, ".json") + "_rewards.json"
	rewardsState := map[string]interface{}{
		"day":  dayNum,
		"last": todayStr,
	}
	if rBytes, err := json.Marshal(rewardsState); err == nil {
		b.mu.Lock()
		b.saveFiles[rewardsFile] = string(rBytes)
		b.mu.Unlock()
		cli := b.newGameHTTPClient()
		_, _ = cli.SaveFiles(token, map[string]string{rewardsFile: string(rBytes)})
	}

	summary := strings.Join(addedItems, ", ")
	if summary == "" {
		summary = "Daily Reward"
	}
	successMsg := fmt.Sprintf("ðŸŽ Claimed daily login gift for Day %d: %s!", dayNum, summary)
	b.emitLog("system", successMsg, "info")
	b.emitChat("notice", "", successMsg, "#ffcc66")

	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	res["claimed"] = true
	res["alreadyClaimed"] = false
	res["message"] = successMsg
	return res, nil
}

func (b *BotInstance) GetAccountCash() (map[string]interface{}, error) {
	_, _, token, _ := b.loadCharData()
	if token != "" {
		cli := b.newGameHTTPClient()
		if acc, err := cli.GetAccount(token); err == nil && acc != nil {
			b.mu.Lock()
			b.accountDP = acc.DP
			b.accountVP = acc.VP
			b.mu.Unlock()
			return map[string]interface{}{
				"dp": acc.DP,
				"vp": acc.VP,
			}, nil
		}
	}

	b.mu.Lock()
	dp := b.accountDP
	vp := b.accountVP
	b.mu.Unlock()

	return map[string]interface{}{
		"dp": dp,
		"vp": vp,
	}, nil
}

func (b *BotInstance) BuyCashShopItem(itemID string, n int, amount int, kind string, giftTo string) (map[string]interface{}, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("item ID cannot be empty")
	}
	if amount <= 0 {
		amount = 1
	}
	if n <= 0 {
		n = 1
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "vp" {
		kind = "dp"
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("bot session token unavailable; please log in or reconnect first")
	}

	invList, _ := charData["inv"].([]interface{})
	if invList == nil {
		invList = make([]interface{}, 0, 240)
	}
	for len(invList) < 240 {
		invList = append(invList, map[string]interface{}{})
	}

	freeSlots := 0
	for _, sl := range invList {
		if m, ok := sl.(map[string]interface{}); ok && len(m) == 0 {
			freeSlots++
		}
	}
	if freeSlots < 1 {
		return nil, fmt.Errorf("inventory is full (no free slots)")
	}

	charData["inv"] = invList
	_ = b.persistCharData(charData, charFile, token)

	charSlot := b.getCharSlot()
	payload := map[string]interface{}{
		"k":      "cash",
		"id":     itemID,
		"n":      n,
		"amount": amount,
		"kind":   kind,
		"char":   charSlot,
	}
	if strings.TrimSpace(giftTo) != "" {
		payload["to"] = strings.TrimSpace(giftTo)
	}

	res, err := b.executeClaimTransaction(token, payload)
	if err != nil {
		b.emitLog("system", fmt.Sprintf("Cash shop purchase failed for '%s': %v", itemID, err), "error")
		b.emitChat("error", "", fmt.Sprintf("Cash shop purchase failed: %v", err), "#ff7366")
		return nil, err
	}

	if okVal, exists := res["ok"].(bool); exists && !okVal {
		errMsg, _ := res["error"].(string)
		if errMsg == "" {
			errMsg = "Server rejected cash shop purchase"
		}
		b.emitLog("system", fmt.Sprintf("Cash shop purchase failed for '%s': %s", itemID, errMsg), "error")
		b.emitChat("error", "", fmt.Sprintf("Cash shop purchase failed: %s", errMsg), "#ff7366")
		return nil, fmt.Errorf("%s", errMsg)
	}

	var addedItems []string
	if itemsRaw, ok := res["items"].([]interface{}); ok && len(itemsRaw) > 0 {
		for _, itRaw := range itemsRaw {
			if itArr, ok := itRaw.([]interface{}); ok && len(itArr) >= 2 {
				id, _ := itArr[0].(string)
				cnt := int(toFloat32(itArr[1]))
				if id != "" {
					if cnt <= 0 {
						cnt = 1
					}
					addItemToInventory(invList, id, cnt)
					addedItems = append(addedItems, fmt.Sprintf("%s x%d", id, cnt))
				}
			}
		}
	} else {
		totalCnt := n * amount
		addItemToInventory(invList, itemID, totalCnt)
		addedItems = append(addedItems, fmt.Sprintf("%s x%d", itemID, totalCnt))
	}

	if accMap, ok := res["account"].(map[string]interface{}); ok {
		b.mu.Lock()
		if dpVal, ok := accMap["dp"].(float64); ok {
			b.accountDP = uint32(dpVal)
		}
		if vpVal, ok := accMap["vp"].(float64); ok {
			b.accountVP = uint32(vpVal)
		}
		b.mu.Unlock()
	}

	charData["inv"] = invList
	_ = b.persistCharData(charData, charFile, token)

	curName := "E-Tokens"
	if kind == "vp" {
		curName = "VP"
	}
	cost := 0
	if costVal, ok := res["cost"].(float64); ok {
		cost = int(costVal)
	}

	b.emitLog("system", fmt.Sprintf("Cash shop purchase: bought %s for %d %s", itemID, cost, curName), "info")
	if len(addedItems) > 0 {
		b.emitChat("notice", "", fmt.Sprintf("Premium Shop: received %s (%d %s)", strings.Join(addedItems, ", "), cost, curName), "#ffcc66")
	}

	b.emitInventoryUpdate(nil)
	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	return res, nil
}

func (b *BotInstance) BuyShopItem(itemID string, itemName string, count int, unitCost int) (map[string]interface{}, error) {
	if count <= 0 {
		count = 1
	}
	if unitCost < 0 {
		unitCost = 0
	}
	totalCost := float64(unitCost * count)

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	currentPenya := b.getEffectivePenya(charData)
	if currentPenya < totalCost {
		return nil, fmt.Errorf("not enough Penya (have: %d, required: %d)", int64(currentPenya), int64(totalCost))
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok {
		invList = make([]interface{}, 112)
		for i := range invList {
			invList[i] = map[string]interface{}{}
		}
	}

	// Try to stack if item already in inventory
	added := false
	for _, rawSlot := range invList {
		slotMap, isMap := rawSlot.(map[string]interface{})
		if isMap && len(slotMap) > 0 {
			if id, hasID := slotMap["id"].(string); hasID && id == itemID {
				curN, _ := slotMap["n"].(float64)
				if curN > 0 && curN+float64(count) <= 9999 {
					slotMap["n"] = curN + float64(count)
					added = true
					break
				}
			}
		}
	}

	// If not stacked, place in first empty slot
	if !added {
		emptySlot := -1
		for i, rawSlot := range invList {
			slotMap, isMap := rawSlot.(map[string]interface{})
			if !isMap || len(slotMap) == 0 {
				emptySlot = i
				break
			}
		}
		if emptySlot == -1 {
			if len(invList) < 112 {
				invList = append(invList, map[string]interface{}{"id": itemID, "n": count})
				emptySlot = len(invList) - 1
			} else {
				return nil, fmt.Errorf("inventory is full (no empty slots available)")
			}
		} else {
			invList[emptySlot] = map[string]interface{}{"id": itemID, "n": count}
		}
	}

	charData["inv"] = invList
	b.setEffectivePenya(charData, currentPenya-totalCost)

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	dispName := itemName
	if dispName == "" {
		dispName = itemID
	}
	b.emitLog("action", fmt.Sprintf("Bought %s x%d for %d Penya", dispName, count, int64(totalCost)), "info")
	b.emitChat("system", "", fmt.Sprintf("Bought %s x%d for %d Penya.", dispName, count, int64(totalCost)), "#8cd9ff")

	return b.GetInventoryData()
}

func (b *BotInstance) SellShopItem(slotIndex int, count int, unitPrice int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok || slotIndex < 0 || slotIndex >= len(invList) {
		return nil, fmt.Errorf("invalid slot index %d", slotIndex)
	}

	itemMap, ok := invList[slotIndex].(map[string]interface{})
	if !ok || IsEmptySlot(itemMap) {
		return nil, fmt.Errorf("slot %d is empty", slotIndex)
	}

	itemID, _ := itemMap["id"].(string)
	meta := models.GetItem(itemID)

	// Ruta 1: Protected items cannot be sold
	if IsProtectedSlot(itemMap, meta, charData, nil) {
		return nil, fmt.Errorf("item '%s' in slot %d is protected and cannot be sold", itemID, slotIndex)
	}

	// Check session blacklist
	b.mu.Lock()
	blacklisted := b.sessionBlacklist != nil && b.sessionBlacklist[itemID]
	b.mu.Unlock()
	if blacklisted {
		return nil, fmt.Errorf("item '%s' previously triggered save refusal and is blacklisted for this session", itemID)
	}

	curN := 1.0
	if nVal, exists := itemMap["n"]; exists {
		switch v := nVal.(type) {
		case float64:
			curN = v
		case int:
			curN = float64(v)
		}
	}
	if curN <= 0 {
		curN = 1
	}

	if count <= 0 || float64(count) >= curN {
		count = int(curN)
	}

	// Calculate server-accurate unit price using ItemDB (Cost / 4)
	cost := 0
	if meta != nil {
		cost = meta.Cost
	}
	unitSellPrice := cost / 4
	isBound, _ := itemMap["bd"].(bool)
	isQuest := meta != nil && meta.K3 == "IK3_QUEST"

	currentPenya := b.getEffectivePenya(charData)
	totalGained := float64(unitSellPrice * count)

	effectiveCap := DefaultMaxPenya
	if b.Profile.Preferences.MaxPenyaCap > 0 {
		effectiveCap = b.Profile.Preferences.MaxPenyaCap
	}
	if effectiveCap > HardMaxPenya {
		effectiveCap = HardMaxPenya
	}

	// Ruta 2 condition: if cost < 4, bound, quest, or penya overflow, 0 Penya gain
	if cost < 4 || isBound || isQuest || (currentPenya+totalGained > float64(effectiveCap)) {
		totalGained = 0
		unitSellPrice = 0
	}

	// Snapshot pre-mutation state for surgical rollback
	b.snapshotPreMutation(charData, []string{itemID})

	if float64(count) >= curN {
		invList[slotIndex] = NewEmptySlot()
	} else {
		itemMap["n"] = curN - float64(count)
	}

	b.setEffectivePenya(charData, currentPenya+totalGained)
	charData["inv"] = invList

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	if totalGained > 0 {
		b.emitLog("action", fmt.Sprintf("Sold item (%s) x%d for %d Penya", itemID, count, int64(totalGained)), "info")
		b.emitChat("system", "", fmt.Sprintf("Sold item x%d for %d Penya.", count, int64(totalGained)), "#ffd959")
	} else {
		b.emitLog("action", fmt.Sprintf("Disposed item (%s) x%d (0 Penya delta)", itemID, count), "info")
		b.emitChat("system", "", fmt.Sprintf("Disposed item x%d (0 Penya).", count), "#ffd959")
	}

	return b.GetInventoryData()
}

func (b *BotInstance) SellJunkItems() (map[string]interface{}, error) {
	capVal := b.Profile.Preferences.MaxPenyaCap
	if capVal <= 0 {
		capVal = DefaultMaxPenya
	}
	rules := CleanRules{
		AutoCleanJunk:       true,
		AutoSellBelowRarity: 120,
		MaxPenyaCap:         capVal,
	}
	return b.CleanInventory(rules)
}

func (b *BotInstance) CleanInventoryWithCap(maxCap int) (map[string]interface{}, error) {
	rules := CleanRules{
		AutoCleanJunk:       true,
		AutoSellBelowRarity: 120,
		MaxPenyaCap:         maxCap,
	}
	return b.CleanInventory(rules)
}

func (b *BotInstance) CleanInventory(rules CleanRules) (map[string]interface{}, error) {
	if rules.MaxPenyaCap <= 0 {
		if b.Profile.Preferences.MaxPenyaCap > 0 {
			rules.MaxPenyaCap = b.Profile.Preferences.MaxPenyaCap
		} else {
			rules.MaxPenyaCap = DefaultMaxPenya
		}
	}
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	blacklist := make(map[string]bool)
	for k, v := range b.sessionBlacklist {
		blacklist[k] = v
	}
	b.mu.Unlock()

	// Peek pass on a clone to find all affected item IDs for pre-mutation snapshot
	peekCharData := make(map[string]interface{}, len(charData))
	for k, v := range charData {
		peekCharData[k] = v
	}
	peekSummary := ProcessAutoCleanPass(peekCharData, rules, blacklist)
	if peekSummary.SoldCount == 0 && peekSummary.TrashedCount == 0 {
		return b.GetInventoryData()
	}

	affectedIDs := append(peekSummary.SoldItems, peekSummary.TrashedItems...)
	b.snapshotPreMutation(charData, affectedIDs)

	// Execute actual clean pass
	summary := ProcessAutoCleanPass(charData, rules, blacklist)

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	if summary.SoldCount > 0 || summary.TrashedCount > 0 {
		b.emitLog("action", fmt.Sprintf("Cleaned inventory: %d sold (+%d Penya), %d trashed (0 Penya)",
			summary.SoldCount, summary.PenyaGained, summary.TrashedCount), "info")
		b.emitChat("system", "", fmt.Sprintf("Cleaned inventory: %d sold (+%d Penya), %d trashed.",
			summary.SoldCount, summary.PenyaGained, summary.TrashedCount), "#ffd959")
	}

	return b.GetInventoryData()
}

func (b *BotInstance) GetNearbyPlayers() []models.NearbyPlayerInfo {
	b.playersMu.RLock()
	defer b.playersMu.RUnlock()

	cutoff := time.Now().Add(-30 * time.Second)
	var list []models.NearbyPlayerInfo
	for _, p := range b.nearbyPlayers {
		if p.LastSeen.After(cutoff) && p.Name != "" {
			list = append(list, models.NearbyPlayerInfo{
				ID:   p.ID,
				Name: p.Name,
				X:    p.X,
				Y:    p.Y,
				Z:    p.Z,
			})
		}
	}
	return list
}

func (b *BotInstance) SendMail(to string, title string, text string, slotIndex int, count int, penya int) (map[string]interface{}, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return nil, fmt.Errorf("recipient character name cannot be empty")
	}
	if title == "" {
		title = "Transfer"
	}
	if penya < 0 {
		penya = 0
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	curPenya := b.getEffectivePenya(charData)
	postage := 500
	neededPenya := float64(penya + postage)
	if curPenya < neededPenya {
		return nil, fmt.Errorf("insufficient Penya (have %d, need %d including %d postage fee)",
			int64(curPenya), int64(neededPenya), postage)
	}

	var itemToSend interface{} = nil
	itemDesc := "None"
	invList, _ := charData["inv"].([]interface{})

	if slotIndex >= 0 && slotIndex < len(invList) {
		slotMap, ok := invList[slotIndex].(map[string]interface{})
		if !ok || len(slotMap) == 0 {
			return nil, fmt.Errorf("selected inventory slot %d is empty", slotIndex)
		}
		if lk, _ := slotMap["lk"].(bool); lk {
			return nil, fmt.Errorf("selected item is locked")
		}
		if bd, _ := slotMap["bd"].(bool); bd {
			return nil, fmt.Errorf("selected item is bound and cannot be transferred")
		}

		avail := int(toFloat32(slotMap["n"]))
		if avail <= 0 {
			avail = 1
		}
		if count <= 0 || count > avail {
			count = avail
		}

		cloned := make(map[string]interface{})
		for k, v := range slotMap {
			cloned[k] = v
		}
		cloned["n"] = count
		itemToSend = cloned

		itemID, _ := slotMap["id"].(string)
		itemDesc = fmt.Sprintf("%s x%d", itemID, count)
	}

	b.mu.Lock()
	cli := b.httpCli
	if cli == nil {
		cli = b.newGameHTTPClientLocked()
	}
	b.mu.Unlock()

	payload := map[string]interface{}{
		"k":     "m_send",
		"to":    to,
		"title": title,
		"text":  text,
		"item":  itemToSend,
		"penya": penya,
		"cod":   0,
		"char":  0,
	}

	res, err := b.executeClaimTransaction(token, payload)
	if err != nil {
		b.emitLog("action", fmt.Sprintf("Failed to send mail to %s: %v", to, err), "error")
		return nil, fmt.Errorf("mail server error: %w", err)
	}

	actualPostage := postage
	if pVal, ok := res["postage"].(float64); ok && int(pVal) > 0 {
		actualPostage = int(pVal)
	}
	b.setEffectivePenya(charData, curPenya-float64(penya+actualPostage))

	if slotIndex >= 0 && slotIndex < len(invList) {
		slotMap := invList[slotIndex].(map[string]interface{})
		avail := int(toFloat32(slotMap["n"]))
		if avail <= 0 {
			avail = 1
		}
		if count >= avail {
			invList[slotIndex] = map[string]interface{}{}
		} else {
			slotMap["n"] = avail - count
		}
	}

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	successMsg := fmt.Sprintf("Mail sent successfully to '%s'! (Penya: %d, Item: %s, Postage: %d)",
		to, penya, itemDesc, actualPostage)
	b.emitLog("action", successMsg, "info")
	b.emitChat("system", "", successMsg, "#66ff99")

	return b.GetInventoryData()
}

func (b *BotInstance) StartTrade(target string, slotIndex int, count int, penya int) (map[string]interface{}, error) {
	b.mu.Lock()
	if b.Status != "online" {
		b.mu.Unlock()
		return nil, fmt.Errorf("bot is not online")
	}
	b.mu.Unlock()

	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("target player name cannot be empty")
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	curPenya := b.getEffectivePenya(charData)
	if float64(penya) > curPenya {
		return nil, fmt.Errorf("insufficient Penya (have %d, offered %d)", int64(curPenya), penya)
	}

	var itemsToSend []interface{}
	itemDesc := "None"
	invList, _ := charData["inv"].([]interface{})
	if slotIndex >= 0 && slotIndex < len(invList) {
		slotMap, ok := invList[slotIndex].(map[string]interface{})
		if !ok || len(slotMap) == 0 {
			return nil, fmt.Errorf("selected inventory slot %d is empty", slotIndex)
		}
		if lk, _ := slotMap["lk"].(bool); lk {
			return nil, fmt.Errorf("selected item is locked")
		}
		if bd, _ := slotMap["bd"].(bool); bd {
			return nil, fmt.Errorf("selected item is bound and cannot be traded")
		}

		avail := int(toFloat32(slotMap["n"]))
		if avail <= 0 {
			avail = 1
		}
		if count <= 0 || count > avail {
			count = avail
		}

		cloned := make(map[string]interface{})
		for k, v := range slotMap {
			cloned[k] = v
		}
		cloned["n"] = count
		itemsToSend = append(itemsToSend, cloned)

		itemID, _ := slotMap["id"].(string)
		itemDesc = fmt.Sprintf("%s x%d", itemID, count)
	}

	// Find target in nearby render range
	b.playersMu.RLock()
	var targetID int
	var targetName string
	for id, p := range b.nearbyPlayers {
		if strings.EqualFold(strings.TrimSpace(p.Name), target) {
			targetID = id
			targetName = p.Name
			break
		}
	}
	b.playersMu.RUnlock()

	if targetID == 0 {
		return nil, fmt.Errorf("target '%s' not found in nearby render range. Move closer or ensure bot and target are on same channel", target)
	}

	session := &TradeSession{
		PartnerID:   targetID,
		PartnerName: targetName,
		SlotIndex:   slotIndex,
		Count:       count,
		Penya:       penya,
		ItemsToGive: itemsToSend,
		StateCh:     make(chan string, 5),
	}

	b.tradeMu.Lock()
	b.tradeSession = session
	b.tradeMu.Unlock()

	// Send trade request packet
	treq := map[string]interface{}{"t": "treq", "id": targetID}
	tb, _ := json.Marshal(treq)
	_ = b.ws.Send(tb)
	b.emitLog("action", fmt.Sprintf("Trade request sent to %s (id: %d). Waiting for partner to accept...", targetName, targetID), "info")

	select {
	case state := <-session.StateCh:
		if state == "done" {
			b.setEffectivePenya(charData, curPenya-float64(penya))
			if slotIndex >= 0 && slotIndex < len(invList) {
				slotMap := invList[slotIndex].(map[string]interface{})
				avail := int(toFloat32(slotMap["n"]))
				if avail <= 0 {
					avail = 1
				}
				if count >= avail {
					invList[slotIndex] = map[string]interface{}{}
				} else {
					slotMap["n"] = avail - count
				}
			}
			_ = b.persistCharData(charData, charFile, token)
			successMsg := fmt.Sprintf("Trade completed successfully with %s! (Penya: %d, Item: %s)", targetName, penya, itemDesc)
			b.emitLog("action", successMsg, "info")
			return b.GetInventoryData()
		} else if state == "rejected" || state == "cancelled" {
			return nil, fmt.Errorf("trade was %s by partner", state)
		}
	case <-time.After(15 * time.Second):
		b.tradeMu.Lock()
		b.tradeSession = nil
		b.tradeMu.Unlock()
		tclose := map[string]interface{}{"t": "tclose"}
		cb, _ := json.Marshal(tclose)
		_ = b.ws.Send(cb)
		return nil, fmt.Errorf("trade timed out waiting for partner confirmation")
	}

	return b.GetInventoryData()
}

func (b *BotInstance) isPartyMember(playerID int) bool {
	b.playersMu.RLock()
	defer b.playersMu.RUnlock()
	p, exists := b.nearbyPlayers[playerID]
	return exists && p.Source == sightParty
}

func (b *BotInstance) UseInventoryItem(slotIndex int) (map[string]interface{}, error) {
	charData, charFile, token, err := b.loadCharData()
	if err != nil {
		return nil, err
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok || slotIndex < 0 || slotIndex >= len(invList) {
		return nil, fmt.Errorf("invalid slot index %d", slotIndex)
	}

	itemMap, _ := invList[slotIndex].(map[string]interface{})
	if itemMap == nil || len(itemMap) == 0 {
		return nil, fmt.Errorf("slot %d is empty", slotIndex)
	}

	itemID, _ := itemMap["id"].(string)
	upper := strings.ToUpper(itemID)

	// Pet handling (pets are toggled, not consumed)
	isPet := strings.Contains(upper, "PET") || strings.Contains(upper, "EGG")
	if isPet {
		uid := toInt(itemMap["uid"])
		petObj, _ := itemMap["pet"].(map[string]interface{})
		if petObj != nil || strings.Contains(upper, "EGG") {
			// Raised Pet toggle
			petUid := uid
			if petObj != nil {
				petUid = toInt(petObj["uid"])
			}
			curPet := toInt(charData["pet_out"])
			if curPet == petUid {
				charData["pet_out"] = 0
				b.emitLog("action", fmt.Sprintf("Unsummoned pet %s", itemID), "info")
			} else {
				charData["pet_out"] = petUid
				b.emitLog("action", fmt.Sprintf("Summoned pet %s", itemID), "info")
			}
		} else {
			// Looter Pet toggle
			curLooter := toInt(charData["looter_out"])
			if curLooter == uid {
				charData["looter_out"] = 0
				b.emitLog("action", fmt.Sprintf("Unsummoned looter pet %s", itemID), "info")
			} else {
				charData["looter_out"] = uid
				b.emitLog("action", fmt.Sprintf("Summoned looter pet %s", itemID), "info")
			}
		}
	} else {
		// Consumable item handling
		count := int(toFloat32(itemMap["n"]))
		if count <= 1 {
			invList[slotIndex] = map[string]interface{}{}
		} else {
			itemMap["n"] = count - 1
		}
		charData["inv"] = invList

		if strings.Contains(upper, "POT") || strings.Contains(upper, "FOO") || strings.Contains(upper, "MEAT") || strings.Contains(upper, "BREAD") || strings.Contains(upper, "EEL") || strings.Contains(upper, "REF") || strings.Contains(upper, "VITAL") {
			b.mu.Lock()
			b.Stats.HP = b.Stats.HP + 200
			b.Stats.MP = b.Stats.MP + 150
			b.Stats.FP = b.Stats.FP + 150
			b.mu.Unlock()
		}
	}

	if err := b.persistCharData(charData, charFile, token); err != nil {
		return nil, err
	}

	if isPet {
		go b.broadcastLook()
	}

	b.emitLog("action", fmt.Sprintf("Used item %s from slot %d", itemID, slotIndex), "info")
	return b.GetInventoryData()
}

func (b *BotInstance) UseOptimalSupply(dst string, missing int) (bool, error) {
	charData, _, _, err := b.loadCharData()
	if err != nil {
		return false, err
	}
	invList, ok := charData["inv"].([]interface{})
	if !ok {
		return false, fmt.Errorf("no inventory")
	}

	bestSlot := -1
	bestScore := -1e9

	for i, raw := range invList {
		item, ok := raw.(map[string]interface{})
		if !ok || len(item) == 0 {
			continue
		}
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		v := GetFoodValue(id, dst)
		if v <= 0 {
			continue
		}
		sc := ScoreSupplyItem(v, missing)
		if sc > bestScore {
			bestScore = sc
			bestSlot = i
		}
	}

	if bestSlot >= 0 {
		_, err := b.UseInventoryItem(bestSlot)
		return err == nil, err
	}
	return false, fmt.Errorf("no optimal supply found for %s", dst)
}

func (b *BotInstance) UseFirstPotion(dst string) error {
	charData, _, _, err := b.loadCharData()
	if err != nil {
		return err
	}
	invList, ok := charData["inv"].([]interface{})
	if !ok {
		return fmt.Errorf("no inventory")
	}

	for i, raw := range invList {
		item, ok := raw.(map[string]interface{})
		if !ok || len(item) == 0 {
			continue
		}
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		upper := strings.ToUpper(id)
		switch dst {
		case "DST_HP":
			if strings.Contains(upper, "POT") || strings.Contains(upper, "FOO") || strings.Contains(upper, "MEAT") || strings.Contains(upper, "BREAD") || strings.Contains(upper, "EEL") {
				_, err := b.UseInventoryItem(i)
				return err
			}
		case "DST_MP":
			if strings.Contains(upper, "REF") || strings.Contains(upper, "MANA") {
				_, err := b.UseInventoryItem(i)
				return err
			}
		case "DST_FP":
			if strings.Contains(upper, "VITAL") || strings.Contains(upper, "DRI") {
				_, err := b.UseInventoryItem(i)
				return err
			}
		}
	}
	return fmt.Errorf("no potion found for %s", dst)
}

func (b *BotInstance) UseItemByID(targetID string) error {
	charData, _, _, err := b.loadCharData()
	if err != nil {
		return err
	}
	invList, ok := charData["inv"].([]interface{})
	if !ok {
		return fmt.Errorf("no inventory")
	}
	for i, raw := range invList {
		item, ok := raw.(map[string]interface{})
		if !ok || len(item) == 0 {
			continue
		}
		id, _ := item["id"].(string)
		if id == targetID {
			_, err := b.UseInventoryItem(i)
			return err
		}
	}
	return fmt.Errorf("item %s not found in inventory", targetID)
}

// GetPartyState returns the thread-safe party membership state of the bot instance.
func (b *BotInstance) GetPartyState() (inParty bool, partyID int, partyName string, leaderName string) {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	b.mu.Lock()
	online := b.Status == "online"
	b.mu.Unlock()
	if !online {
		return false, 0, "", ""
	}
	return b.inParty, b.partyID, b.partyName, b.partyLeaderName
}

// IsInParty returns true if the bot instance is currently joined in an online party.
func (b *BotInstance) IsInParty() bool {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	return b.inParty
}

// SetPartyState updates the bot instance's cached party membership state.
func (b *BotInstance) SetPartyState(inParty bool, partyID int, partyName string, leaderName string) {
	b.partyMu.Lock()
	defer b.partyMu.Unlock()
	b.inParty = inParty
	b.partyID = partyID
	b.partyName = partyName
	b.partyLeaderName = leaderName
}

// SetPartyBoostManager binds the central PartyBoostManager to this bot instance.
func (b *BotInstance) SetPartyBoostManager(mgr *PartyBoostManager) {
	b.partyMu.Lock()
	defer b.partyMu.Unlock()
	b.partyBoost = mgr
}

func (b *BotInstance) getPartyBoostConfig() (models.PartyBoostConfig, bool) {
	b.partyMu.RLock()
	mgr := b.partyBoost
	b.partyMu.RUnlock()
	if mgr == nil {
		return models.PartyBoostConfig{}, false
	}
	return mgr.GetConfig(), true
}

// SendWSPacket encodes and transmits a JSON packet over the active game websocket.
func (b *BotInstance) SendWSPacket(pkt map[string]interface{}) error {
	b.mu.Lock()
	ws := b.ws
	online := b.Status == "online"
	b.mu.Unlock()

	if !online || ws == nil {
		return fmt.Errorf("bot not online or websocket not connected")
	}
	data, err := json.Marshal(pkt)
	if err != nil {
		return err
	}
	return ws.Send(data)
}

// SendPartyAccept sends a paccept packet replying to invitation from fromID.
func (b *BotInstance) SendPartyAccept(fromID int) error {
	return b.AcceptPartyInvite(fromID)
}

// SendFriendshipSummonAccept sends fsumok replying to summon request from fromID.
func (b *BotInstance) SendFriendshipSummonAccept(fromID int) error {
	return b.SendWSPacket(map[string]interface{}{
		"t":    "fsumok",
		"from": fromID,
	})
}

// GetPartyMembers returns a thread-safe slice of current party members.
func (b *BotInstance) GetPartyMembers() []models.PartyMember {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	res := make([]models.PartyMember, len(b.partyMembers))
	copy(res, b.partyMembers)
	return res
}

// HasPartyMember checks if member is in the bot's party by character name.
func (b *BotInstance) HasPartyMember(name string) bool {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	clean := strings.TrimSpace(name)
	for _, m := range b.partyMembers {
		if strings.EqualFold(m.Name, clean) {
			return true
		}
	}
	return false
}

// PartyMemberCount returns the number of members in the bot's party.
func (b *BotInstance) PartyMemberCount() int {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	return len(b.partyMembers)
}

// IsPartyLeashed returns true if this bot is currently following the party leader due to leash limit.
func (b *BotInstance) IsPartyLeashed() bool {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	return b.partyLeashed
}

// SetPartyLeashed updates the leash active flag.
func (b *BotInstance) SetPartyLeashed(leashed bool) {
	b.partyMu.Lock()
	defer b.partyMu.Unlock()
	b.partyLeashed = leashed
}

// IsPartyLeader returns true if this bot instance is currently the leader of its active party.
func (b *BotInstance) IsPartyLeader() bool {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	if !b.inParty || b.partyID == 0 {
		return false
	}
	b.mu.Lock()
	myEntityID := b.entityID
	charName := b.Profile.CharacterName
	accName := b.Profile.Name
	b.mu.Unlock()

	if myEntityID != 0 && b.partyLeaderID != 0 && b.partyLeaderID == myEntityID {
		return true
	}
	if b.partyLeaderName != "" {
		if strings.EqualFold(b.partyLeaderName, charName) || strings.EqualFold(b.partyLeaderName, accName) {
			return true
		}
	}
	if cfg, ok := b.getPartyBoostConfig(); ok && cfg.Enabled && cfg.LeaderName != "" {
		if strings.EqualFold(cfg.LeaderName, charName) || strings.EqualFold(cfg.LeaderName, accName) {
			return true
		}
	}
	return false
}

// SendPartySet sends a pset packet with key k and value v.
func (b *BotInstance) SendPartySet(k string, v interface{}) error {
	return b.SendWSPacket(map[string]interface{}{
		"t": "pset",
		"k": k,
		"v": v,
	})
}

// evaluatePartyRules evaluates auto-rename, auto-advance, exp share, and loot distribution rules on leader.
func (b *BotInstance) evaluatePartyRules() {
	if !b.IsPartyLeader() {
		return
	}

	cfg, ok := b.getPartyBoostConfig()
	if !ok || !cfg.Enabled {
		return
	}

	b.partyMu.Lock()
	curName := b.partyName
	curLevel := b.partyLevel
	curKind := b.partyKind
	curShare := b.partyShare
	curDist := b.partyDist
	if b.lastPSetTime == nil {
		b.lastPSetTime = make(map[string]time.Time)
	}
	now := time.Now()
	b.partyMu.Unlock()

	// 1. Auto-Rename: If party_name is configured and differs from current party name
	desiredName := strings.TrimSpace(cfg.PartyName)
	if desiredName != "" && curName != desiredName {
		b.partyMu.Lock()
		lastSent := b.lastPSetTime["name"]
		canSend := now.Sub(lastSent) >= 3*time.Second
		if canSend {
			b.lastPSetTime["name"] = now
		}
		b.partyMu.Unlock()
		if canSend {
			_ = b.SendPartySet("name", desiredName)
			b.emitLog("party", fmt.Sprintf("Party Boost: Renamed party to '%s'", desiredName), "info")
		}
	}

	// 2. Auto-Advance Party: When auto_advance is enabled, level >= 10, and kind == 0
	if cfg.AutoAdvance && curLevel >= 10 && curKind == 0 {
		b.partyMu.Lock()
		lastSent := b.lastPSetTime["kind"]
		canSend := now.Sub(lastSent) >= 3*time.Second
		if canSend {
			b.lastPSetTime["kind"] = now
		}
		b.partyMu.Unlock()
		if canSend {
			_ = b.SendPartySet("kind", 1)
			b.emitLog("party", "Party Boost: Auto-upgraded party to Advanced Party at Lv. 10", "info")
		}
	}

	// 3. Smart EXP Share Mode:
	// If exp_share_mode == 1 (Contribution), wait until kind == 1 (Advanced Party), then set share = 1
	// If exp_share_mode == 0 (Level), ensure share = 0
	if cfg.ExpShareMode == 1 {
		if curKind == 1 && curShare != 1 {
			b.partyMu.Lock()
			lastSent := b.lastPSetTime["share"]
			canSend := now.Sub(lastSent) >= 3*time.Second
			if canSend {
				b.lastPSetTime["share"] = now
			}
			b.partyMu.Unlock()
			if canSend {
				_ = b.SendPartySet("share", 1)
				b.emitLog("party", "Party Boost: Switched EXP share to Contribution (+10% extra EXP)", "info")
			}
		}
	} else if cfg.ExpShareMode == 0 {
		if curShare != 0 {
			b.partyMu.Lock()
			lastSent := b.lastPSetTime["share"]
			canSend := now.Sub(lastSent) >= 3*time.Second
			if canSend {
				b.lastPSetTime["share"] = now
			}
			b.partyMu.Unlock()
			if canSend {
				_ = b.SendPartySet("share", 0)
				b.emitLog("party", "Party Boost: Switched EXP share to Level weighting", "info")
			}
		}
	}

	// 4. Loot Distribution Mode: If current dist != item_dist_mode
	if curDist != cfg.ItemDistMode && cfg.ItemDistMode >= 0 && cfg.ItemDistMode <= 3 {
		b.partyMu.Lock()
		lastSent := b.lastPSetTime["dist"]
		canSend := now.Sub(lastSent) >= 3*time.Second
		if canSend {
			b.lastPSetTime["dist"] = now
		}
		b.partyMu.Unlock()
		if canSend {
			_ = b.SendPartySet("dist", cfg.ItemDistMode)
			distNames := []string{"Individually", "Sequentially", "Manually", "Randomly"}
			name := fmt.Sprintf("%d", cfg.ItemDistMode)
			if cfg.ItemDistMode < len(distNames) {
				name = distNames[cfg.ItemDistMode]
			}
			b.emitLog("party", fmt.Sprintf("Party Boost: Set item distribution to %s", name), "info")
		}
	}
}

// evaluateLevelSync checks if any nearby party member is 15+ levels below and syncs damage accordingly.
func (b *BotInstance) evaluateLevelSync() {
	cfg, ok := b.getPartyBoostConfig()
	if !ok || !cfg.Enabled || !cfg.LevelSync {
		b.partyMu.Lock()
		wasActive := b.levelSyncActive
		b.levelSyncActive = false
		b.syncLevel = 0
		b.partyMu.Unlock()
		if wasActive {
			b.emitLog("party", "Level Sync is off - fighting at normal level", "info")
		}
		return
	}

	b.partyMu.RLock()
	inParty := b.inParty
	members := b.partyMembers
	b.partyMu.RUnlock()

	if !inParty || len(members) == 0 {
		b.partyMu.Lock()
		b.levelSyncActive = false
		b.syncLevel = 0
		b.partyMu.Unlock()
		return
	}

	b.mu.Lock()
	botLevel := int(b.Stats.Level)
	botX := b.Stats.PosX
	botZ := b.Stats.PosZ
	myEntityID := b.entityID
	charName := b.Profile.CharacterName
	b.mu.Unlock()

	if botLevel <= 1 {
		return
	}

	// Find lowest member level within 80m (matching social.gd:680)
	lowestLevel := 9999
	for _, m := range members {
		if (myEntityID != 0 && m.ID == myEntityID) || strings.EqualFold(m.Name, charName) {
			continue
		}
		dx := float64(m.X - botX)
		dz := float64(m.Z - botZ)
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 80.0 {
			continue
		}
		if m.Level > 0 && m.Level < lowestLevel {
			lowestLevel = m.Level
		}
	}

	b.partyMu.Lock()
	defer b.partyMu.Unlock()

	// Condition: party member is 15+ levels below the bot
	if lowestLevel < 9999 && (botLevel-lowestLevel >= 15) {
		targetSync := lowestLevel + 5 // SYNC_GAP = 5 (social.gd:672)
		if targetSync >= botLevel {
			targetSync = botLevel - 1
		}
		if targetSync < 1 {
			targetSync = 1
		}

		if !b.levelSyncActive || b.syncLevel != targetSync {
			b.levelSyncActive = true
			b.syncLevel = targetSync
			syncFactor := math.Pow(float64(targetSync)/float64(botLevel), 1.6)
			b.emitLog("party", fmt.Sprintf("Level Sync active: synced to Lv %d (member Lv %d, dmg x%.2f, Mentor points enabled)", targetSync, lowestLevel, syncFactor), "info")
		}
	} else {
		if b.levelSyncActive {
			b.levelSyncActive = false
			b.syncLevel = 0
			b.emitLog("party", "Level Sync deactivated: no nearby party members 15+ levels below", "info")
		}
	}
}

// IsLevelSyncActive returns true if level sync is active on this bot.
func (b *BotInstance) IsLevelSyncActive() bool {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	return b.levelSyncActive
}

// GetLevelSync returns current sync level and active status.
func (b *BotInstance) GetLevelSync() (syncLevel int, active bool) {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	return b.syncLevel, b.levelSyncActive
}

// SetLevelSync updates level sync active state and target level.
func (b *BotInstance) SetLevelSync(active bool, syncLevel int) {
	b.partyMu.Lock()
	defer b.partyMu.Unlock()
	b.levelSyncActive = active
	b.syncLevel = syncLevel
}

// GetPartyDetails returns detailed party mechanics state for testing and status reports.
func (b *BotInstance) GetPartyDetails() (level, exp, points, kind, share, dist int, syncActive bool, syncLevel int) {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	return b.partyLevel, b.partyExp, b.partyPoints, b.partyKind, b.partyShare, b.partyDist, b.levelSyncActive, b.syncLevel
}

// PartySkillDefinition defines the mechanics, costs, and priority of a party skill.
type PartySkillDefinition struct {
	ID          string
	Name        string
	ReqLevel    int
	ReqPoints   int
	Duration    float64
	Cooldown    float64
	AdvRequired bool
}

// PartySkillPriorities defines party skills in strict maintainer priority order.
var PartySkillPriorities = []PartySkillDefinition{
	{
		ID:          models.PartySkillLinkAttack,
		Name:        "Linked Attack",
		ReqLevel:    10, // Unlocks with Advanced Party at Lv 10
		ReqPoints:   3,
		Duration:    60.0,
		Cooldown:    13.0,
		AdvRequired: true,
	},
	{
		ID:          models.PartySkillFortuneCircle,
		Name:        "Lucky Drop",
		ReqLevel:    25,
		ReqPoints:   5,
		Duration:    300.0,
		Cooldown:    200.0,
		AdvRequired: true,
	},
	{
		ID:          models.PartySkillGiftBox,
		Name:        "Gift Box",
		ReqLevel:    40,
		ReqPoints:   8,
		Duration:    300.0,
		Cooldown:    200.0,
		AdvRequired: true,
	},
	{
		ID:          models.PartySkillSphereCircle,
		Name:        "Precision",
		ReqLevel:    17,
		ReqPoints:   1,
		Duration:    60.0,
		Cooldown:    40.0,
		AdvRequired: false,
	},
	{
		ID:          models.PartySkillStretching,
		Name:        "Resting",
		ReqLevel:    30,
		ReqPoints:   4,
		Duration:    240.0,
		Cooldown:    80.0,
		AdvRequired: true,
	},
}

// SendPartySkill sends a pskill packet for skill id.
func (b *BotInstance) SendPartySkill(id string) error {
	return b.SendWSPacket(map[string]interface{}{
		"t":  "pskill",
		"id": id,
	})
}

// SendPartyScroll sends a pscroll packet for party scroll base ID and duration in seconds.
func (b *BotInstance) SendPartyScroll(id string, secs int) error {
	return b.SendWSPacket(map[string]interface{}{
		"t":    "pscroll",
		"id":   id,
		"secs": secs,
	})
}

// GetPartySkillRemaining returns the remaining active duration in seconds for skill id.
func (b *BotInstance) GetPartySkillRemaining(id string) float64 {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	if b.partyModesExpiry != nil {
		if exp, ok := b.partyModesExpiry[id]; ok {
			rem := exp.Sub(time.Now()).Seconds()
			if rem > 0 {
				return rem
			}
			return 0.0
		}
	}
	if b.partyModes != nil {
		return b.partyModes[id]
	}
	return 0.0
}

// IsPartySkillOnCooldown returns true if party skill id is currently on cooldown.
func (b *BotInstance) IsPartySkillOnCooldown(id string) bool {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	if b.partySkillCDExpiry != nil {
		if exp, ok := b.partySkillCDExpiry[id]; ok {
			return time.Now().Before(exp)
		}
	}
	if b.partySkillCD != nil {
		return b.partySkillCD[id] > 0
	}
	return false
}

// GetPartyScrollsState returns active state of party scrolls.
func (b *BotInstance) GetPartyScrollsState() (full, expUp, skillUp bool) {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	return b.partyScrollFull, b.partyScrollExpUp, b.partyScrollSkillUp
}

// GetPartyModesAndCD returns active party skill durations and cooldowns with second-level accuracy.
func (b *BotInstance) GetPartyModesAndCD() (map[string]float64, map[string]float64) {
	b.partyMu.RLock()
	defer b.partyMu.RUnlock()
	now := time.Now()
	modes := make(map[string]float64)
	if b.partyModesExpiry != nil {
		for k, exp := range b.partyModesExpiry {
			rem := exp.Sub(now).Seconds()
			if rem > 0 {
				modes[k] = math.Round(rem*10) / 10
			}
		}
	} else if b.partyModes != nil {
		for k, v := range b.partyModes {
			if v > 0 {
				modes[k] = v
			}
		}
	}

	cd := make(map[string]float64)
	if b.partySkillCDExpiry != nil {
		for k, exp := range b.partySkillCDExpiry {
			rem := exp.Sub(now).Seconds()
			if rem > 0 {
				cd[k] = math.Round(rem*10) / 10
			}
		}
	} else if b.partySkillCD != nil {
		for k, v := range b.partySkillCD {
			if v > 0 {
				cd[k] = v
			}
		}
	}
	return modes, cd
}

// evaluatePartySkills runs every second on the party leader to maintain party skills in priority order.
func (b *BotInstance) evaluatePartySkills(now time.Time, cfg models.PartyBoostConfig) {
	if !b.IsPartyLeader() {
		return
	}

	b.partyMu.RLock()
	inParty := b.inParty
	pLevel := b.partyLevel
	pKind := b.partyKind
	pPoints := b.partyPoints
	hasSkillUp := b.partyScrollSkillUp
	b.partyMu.RUnlock()

	if !inParty || pLevel <= 0 {
		return
	}

	for _, sk := range PartySkillPriorities {
		// 1. Is skill enabled in config?
		if !cfg.SkillsEnabled[sk.ID] {
			continue
		}

		// 2. Party level requirement
		if pLevel < sk.ReqLevel {
			continue
		}

		// 3. Advanced party requirement
		if sk.AdvRequired && pKind != 1 {
			continue
		}

		// 4. Party points requirement
		if pPoints < sk.ReqPoints {
			continue
		}

		// 5. Cooldown check
		if b.IsPartySkillOnCooldown(sk.ID) {
			continue
		}

		// 6. Active check & renewal:
		// Active if remaining > 0. If online_skillup is active, early renewal allowed when remaining < 5.0s.
		rem := b.GetPartySkillRemaining(sk.ID)
		canCast := false
		if rem <= 0.0 {
			canCast = true
		} else if hasSkillUp && rem < 5.0 {
			canCast = true
		}

		if !canCast {
			continue
		}

		// All conditions met: cast skill
		err := b.SendPartySkill(sk.ID)
		if err != nil {
			b.emitLog("party", fmt.Sprintf("Failed to cast party skill %s: %v", sk.Name, err), "warn")
			continue
		}

		// Optimistic updates
		b.partyMu.Lock()
		b.partyPoints -= sk.ReqPoints
		pPoints = b.partyPoints
		if b.partyModes == nil {
			b.partyModes = make(map[string]float64)
		}
		if b.partyModesExpiry == nil {
			b.partyModesExpiry = make(map[string]time.Time)
		}
		b.partyModes[sk.ID] = sk.Duration
		b.partyModesExpiry[sk.ID] = now.Add(time.Duration(sk.Duration * float64(time.Second)))

		if b.partySkillCD == nil {
			b.partySkillCD = make(map[string]float64)
		}
		if b.partySkillCDExpiry == nil {
			b.partySkillCDExpiry = make(map[string]time.Time)
		}
		b.partySkillCD[sk.ID] = sk.Cooldown
		b.partySkillCDExpiry[sk.ID] = now.Add(time.Duration(sk.Cooldown * float64(time.Second)))
		b.partyMu.Unlock()

		b.emitLog("party", fmt.Sprintf("Party Boost: Cast %s (%d pt, %.0fs dur)", sk.Name, sk.ReqPoints, sk.Duration), "info")

		// Brief delay before checking/casting next skill
		time.Sleep(150 * time.Millisecond)
	}
}

// evaluatePartyScrolls maintains party scrolls from inventory on leader.
func (b *BotInstance) evaluatePartyScrolls(now time.Time, cfg models.PartyBoostConfig) {
	b.partyMu.RLock()
	inParty := b.inParty
	pLevel := b.partyLevel
	fullActive := b.partyScrollFull
	expUpActive := b.partyScrollExpUp
	skillUpActive := b.partyScrollSkillUp
	b.partyMu.RUnlock()

	if !inParty {
		return
	}

	isLeader := b.IsPartyLeader()

	// Load leader character data (inventory + buffs)
	charData, charFile, token, err := b.loadCharData()
	if err != nil || charData == nil {
		return
	}

	invList, _ := charData["inv"].([]interface{})
	if invList == nil || len(invList) == 0 {
		return
	}

	buffsMap := getOrCreateMap(charData, "buffs")
	dirty := false

	// Helper to check if any active buff starts with prefix
	hasActiveBuff := func(prefix string) bool {
		for k, v := range buffsMap {
			if strings.HasPrefix(k, prefix) && isBuffActive(v) {
				return true
			}
		}
		return false
	}

	// Helper to find and consume a scroll by prefix from inventory
	findAndConsumeScroll := func(prefix string) (baseID string, timeSecs float64, found bool) {
		b.partyMu.Lock()
		if b.lastScrollTime == nil {
			b.lastScrollTime = make(map[string]time.Time)
		}
		if lastUsed, exists := b.lastScrollTime[prefix]; exists && now.Sub(lastUsed) < 5*time.Second {
			b.partyMu.Unlock()
			return "", 0, false
		}
		b.partyMu.Unlock()

		for i := 0; i < len(invList); i++ {
			im, ok := invList[i].(map[string]interface{})
			if !ok || len(im) == 0 {
				continue
			}
			idStr := toString(im["id"])
			if strings.HasPrefix(idStr, prefix) {
				base := strings.Split(idStr, "#")[0]
				tSecs := toFloat64(im["time"])
				if tSecs <= 0 {
					if strings.HasSuffix(base, "_01") || strings.HasSuffix(base, "1") {
						tSecs = 86400.0 // 1 day
					} else if strings.Contains(base, "02") || strings.Contains(base, "15") {
						tSecs = 86400.0 * 15.0 // 15 days
					} else if strings.Contains(base, "30") {
						tSecs = 86400.0 * 30.0 // 30 days
					} else {
						tSecs = 86400.0 * 7.0 // 7 days
					}
				}
				consumeOneFromSlot(invList, i)

				b.partyMu.Lock()
				b.lastScrollTime[prefix] = now
				b.partyMu.Unlock()

				return prefix, tSecs, true
			}
		}
		return "", 0, false
	}

	// 1. II_SYS_SYS_SCR_SELPAR: Scroll of Selected Party (Instant Party Lv. 10)
	// Condition: in party and party.level < 10
	if cfg.ScrollsEnabled[models.PartyScrollSelectedParty] && pLevel < 10 {
		if base, _, found := findAndConsumeScroll(models.PartyScrollSelectedParty); found {
			_ = b.SendPartyScroll(base, 0)
			dirty = true
			b.emitLog("party", "[SCROLL] Used Scroll of Selected Party (Instant Lv. 10)", "info")
		}
	}

	// For the remaining scrolls, bot must be the Party Leader
	if !isLeader {
		if dirty {
			_ = b.persistCharData(charData, charFile, token)
		}
		return
	}

	// 2. II_CHR_SYS_SCR_PSKILLFULL: Enhanced Party Skill Range (Infinite map range)
	// Condition: !online_full and not in st.buffs
	if cfg.ScrollsEnabled[models.PartyScrollSkillFull] && !fullActive && !hasActiveBuff(models.PartyScrollSkillFull) {
		if base, secs, found := findAndConsumeScroll(models.PartyScrollSkillFull); found {
			buffsMap[base] = map[string]interface{}{
				"left":   secs,
				"time":   secs,
				"until":  float64(now.Unix()) + secs,
				"params": []interface{}{},
			}
			_ = b.SendPartyScroll(base, int(secs))
			b.partyMu.Lock()
			b.partyScrollFull = true
			b.partyMu.Unlock()
			dirty = true
			b.emitLog("party", fmt.Sprintf("[SCROLL] Activated Enhanced Party Skill Range (%.0f hrs)", secs/3600.0), "info")
		}
	}

	// 3. II_SYS_SYS_SCR_PARTYEXPUP: Party EXP +50%
	// Condition: !online_expup and not in st.buffs
	if cfg.ScrollsEnabled[models.PartyScrollExpUp] && !expUpActive && !hasActiveBuff(models.PartyScrollExpUp) {
		if base, secs, found := findAndConsumeScroll(models.PartyScrollExpUp); found {
			buffsMap[base] = map[string]interface{}{
				"left":   secs,
				"time":   secs,
				"until":  float64(now.Unix()) + secs,
				"params": []interface{}{},
			}
			_ = b.SendPartyScroll(base, int(secs))
			b.partyMu.Lock()
			b.partyScrollExpUp = true
			b.partyMu.Unlock()
			dirty = true
			b.emitLog("party", fmt.Sprintf("[SCROLL] Activated Party EXP Up +50%% (%.0f hrs)", secs/3600.0), "info")
		}
	}

	// 4. II_SYS_SYS_SCR_PARTYSKILLUP: Party Skill Renew
	// Condition: !online_skillup and not in st.buffs
	if cfg.ScrollsEnabled[models.PartyScrollSkillUp] && !skillUpActive && !hasActiveBuff(models.PartyScrollSkillUp) {
		if base, secs, found := findAndConsumeScroll(models.PartyScrollSkillUp); found {
			buffsMap[base] = map[string]interface{}{
				"left":   secs,
				"time":   secs,
				"until":  float64(now.Unix()) + secs,
				"params": []interface{}{},
			}
			_ = b.SendPartyScroll(base, int(secs))
			b.partyMu.Lock()
			b.partyScrollSkillUp = true
			b.partyMu.Unlock()
			dirty = true
			b.emitLog("party", fmt.Sprintf("[SCROLL] Activated Party Skill Renew (%.0f hrs)", secs/3600.0), "info")
		}
	}

	// 5. II_SYS_SYS_SCR_SUPERLEADERPARTY: Super Leader Party
	// Condition: IsPartyLeader() and not in st.buffs
	if cfg.ScrollsEnabled[models.PartyScrollSuperLeader] && !hasActiveBuff(models.PartyScrollSuperLeader) {
		if base, secs, found := findAndConsumeScroll(models.PartyScrollSuperLeader); found {
			buffsMap[base] = map[string]interface{}{
				"left":   secs,
				"time":   secs,
				"until":  float64(now.Unix()) + secs,
				"params": []interface{}{},
			}
			_ = b.SendPartyScroll(base, int(secs))
			dirty = true
			b.emitLog("party", fmt.Sprintf("[SCROLL] Activated Super Leader Party (%.0f hrs)", secs/3600.0), "info")
		}
	}

	if dirty {
		_ = b.persistCharData(charData, charFile, token)
	}
}

func isBuffActive(buffEntry interface{}) bool {
	if buffEntry == nil {
		return false
	}
	bm, ok := buffEntry.(map[string]interface{})
	if !ok || len(bm) == 0 {
		return false
	}
	nowUnix := float64(time.Now().Unix())
	if untilVal, ok := bm["until"]; ok {
		untilSec := toFloat64(untilVal)
		if untilSec > 0 {
			return untilSec > nowUnix
		}
	}
	if leftVal, ok := bm["left"]; ok {
		return toFloat64(leftVal) > 0
	}
	return true
}
