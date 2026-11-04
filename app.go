package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/internal/presence"
	"madrigal-multibot/pkg/bot"
	"madrigal-multibot/pkg/client"
	"madrigal-multibot/pkg/models"
	"madrigal-multibot/pkg/version"
	"madrigal-multibot/pkg/vpn"
)

const (
	mortiVPNTunnelID = "morti1"
	vpnEnabled       = false
)

type RegisterAccountRequest struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	Server       string `json:"server"`
	Target1stJob string `json:"target_1st_job,omitempty"`
	Target2ndJob string `json:"target_2nd_job,omitempty"`
}

type botPendingStatUpdate struct {
	Stats         models.BotRuntimeStats `json:"stats"`
	StatusMessage string                 `json:"status_message"`
}

type botPendingLogEntry struct {
	BotID     string `json:"bot_id"`
	Category  string `json:"category"`
	Message   string `json:"message"`
	Level     string `json:"level"`
	Timestamp string `json:"timestamp"`
}

type botPendingChatEntry struct {
	BotID     string `json:"bot_id"`
	Channel   string `json:"channel"`
	Sender    string `json:"sender"`
	Text      string `json:"text"`
	Color     string `json:"color"`
	Timestamp string `json:"timestamp"`
}

type App struct {
	ctx         context.Context
	botManager  *bot.BotManager
	httpCli     *client.GameHttpClient
	vpnManager  *vpn.Manager
	vpnMu       sync.Mutex
	vpnCancel   context.CancelFunc
	vpnDone     chan struct{}
	presenceMu  sync.RWMutex
	presenceTab string
	lastPresenceUpdate time.Time
	settingsMu  sync.RWMutex
	settings    AppSettings
	enableLogs  atomic.Bool
	enableConsole   atomic.Bool
	isShuttingDown  atomic.Bool
	multiDispatcher *multi.Dispatcher
	lastStatusMu    sync.Mutex
	lastStatusMap   map[string]string
	statsThrottleMu sync.Mutex
	lastStatsEmit   map[string]time.Time
	pendingStats    map[string]botPendingStatUpdate
	logThrottleMu   sync.Mutex
	pendingLogs     []botPendingLogEntry
	chatThrottleMu  sync.Mutex
	pendingChats    []botPendingChatEntry
}

type AppSettings struct {
	DiscordRichPresence bool `json:"discordRichPresence"`
	EnableLogs          bool `json:"enableLogs"`
	EnableConsole       bool `json:"enableConsole"`
}

func defaultAppSettings() AppSettings {
	return AppSettings{
		DiscordRichPresence: true,
		EnableLogs:          false,
		EnableConsole:       false,
	}
}

func appSettingsPath() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Morti", "settings.json")
}

func loadAppSettings() AppSettings {
	settings := defaultAppSettings()
	data, err := os.ReadFile(appSettingsPath())
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		slog.Debug("Unable to read app settings; using defaults", "error", err)
		return defaultAppSettings()
	}
	return settings
}

func saveAppSettings(settings AppSettings) error {
	path := appSettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func resolveVPNDir() string {
	checkDir := func(dir string) bool {
		for _, name := range []string{"vpnclient_x64.exe", "vpncmd_x64.exe", "hamcore.se2"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				return false
			}
		}
		return true
	}

	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "third_party", "vpn")
		if checkDir(p) {
			return p
		}
		sub := filepath.Join(p, "SoftEther VPN Client")
		if checkDir(sub) {
			return sub
		}
	}
	p := filepath.Join("third_party", "vpn")
	if checkDir(p) {
		return p
	}
	sub := filepath.Join(p, "SoftEther VPN Client")
	if checkDir(sub) {
		return sub
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "third_party", "vpn")
	}
	return p
}

func NewApp() *App {
	settings := loadAppSettings()
	a := &App{
		httpCli:       client.NewGameHttpClient(),
		settings:      settings,
		lastStatusMap: make(map[string]string),
		lastStatsEmit: make(map[string]time.Time),
		pendingStats:  make(map[string]botPendingStatUpdate),
		pendingLogs:   make([]botPendingLogEntry, 0, 100),
		pendingChats:  make([]botPendingChatEntry, 0, 50),
	}
	a.enableLogs.Store(settings.EnableLogs)
	a.enableConsole.Store(settings.EnableConsole)
	if a.botManager != nil {
		a.botManager.SetLogsEnabled(settings.EnableLogs)
	}
	a.vpnManager = vpn.New(vpn.Config{Dir: resolveVPNDir()})

	a.botManager = bot.NewBotManager(
		func(id string, stats models.BotRuntimeStats, status string, statusMsg string) {
			if a.isShuttingDown.Load() {
				return
			}
			a.updatePresenceThrottled()
			if a.ctx == nil {
				return
			}

			// Gate status changes: only emit when connection status actually transitions
			statusKey := status
			a.lastStatusMu.Lock()
			statusChanged := a.lastStatusMap[id] != statusKey
			if statusChanged {
				a.lastStatusMap[id] = statusKey
			}
			a.lastStatusMu.Unlock()

			now := time.Now()
			if statusChanged {
				runtime.EventsEmit(a.ctx, "event://bot/status_changed", map[string]interface{}{
					"bot_id":         id,
					"status":         status,
					"status_message": statusMsg,
					"timestamp_ms":   now.UnixMilli(),
				})
			}

			// Buffer stats and live status_message for the 250ms batched flush loop
			a.statsThrottleMu.Lock()
			a.pendingStats[id] = botPendingStatUpdate{
				Stats:         stats,
				StatusMessage: statusMsg,
			}
			a.statsThrottleMu.Unlock()
		},
		func(botID, category, message, level string) {
			if a.ctx != nil && a.enableLogs.Load() {
				a.logThrottleMu.Lock()
				if len(a.pendingLogs) < 500 {
					a.pendingLogs = append(a.pendingLogs, botPendingLogEntry{
						BotID:     botID,
						Category:  category,
						Message:   message,
						Level:     level,
						Timestamp: time.Now().Format("15:04:05.000"),
					})
				}
				a.logThrottleMu.Unlock()
			}
		},
		func(botID, channel, sender, text, color string) {
			if a.ctx != nil && a.enableConsole.Load() {
				a.chatThrottleMu.Lock()
				if len(a.pendingChats) < 200 {
					a.pendingChats = append(a.pendingChats, botPendingChatEntry{
						BotID:     botID,
						Channel:   channel,
						Sender:    sender,
						Text:      text,
						Color:     color,
						Timestamp: time.Now().Format("15:04:05.000"),
					})
				}
				a.chatThrottleMu.Unlock()
			}
		},
	)
	a.botManager.SetVPNManager(a.vpnManager, mortiVPNTunnelID)
	a.botManager.SetLogsEnabled(settings.EnableLogs)

	a.botManager.SetOnInventoryUpdated(func(botID string, invData map[string]interface{}) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "event://bot/inventory_updated", map[string]interface{}{
				"bot_id":    botID,
				"inventory": invData,
			})
		}
	})

	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.multiDispatcher = a.newMultiDispatcher()
	settings := a.GetAppSettings()
	a.presenceMu.Lock()
	a.presenceTab = "Main"
	a.presenceMu.Unlock()
	if err := presence.Start(presence.ApplicationID); err != nil {
		slog.Debug("Unable to start Discord Rich Presence", "error", err)
	}
	presence.SetEnabled(settings.DiscordRichPresence)
	a.updatePresence()
	_, _ = bot.EnsureStatsJsonExists()
	_, _ = a.ImportBotsFile()
	a.reconcileVPN()
	go a.startStatsFlushLoop()
}

func (a *App) startStatsFlushLoop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if a.isShuttingDown.Load() {
			return
		}
		<-ticker.C
		if a.ctx == nil || a.isShuttingDown.Load() {
			continue
		}

		// 1. Flush stats batch
		a.statsThrottleMu.Lock()
		if len(a.pendingStats) > 0 {
			now := time.Now()
			batch := a.pendingStats
			a.pendingStats = make(map[string]botPendingStatUpdate)
			for id := range batch {
				a.lastStatsEmit[id] = now
			}
			a.statsThrottleMu.Unlock()

			updates := make([]map[string]interface{}, 0, len(batch))
			for id, u := range batch {
				updates = append(updates, map[string]interface{}{
					"bot_id":         id,
					"stats":          u.Stats,
					"status_message": u.StatusMessage,
				})
			}

			runtime.EventsEmit(a.ctx, "event://bot/stats_batch", map[string]interface{}{
				"updates": updates,
			})
		} else {
			a.statsThrottleMu.Unlock()
		}

		// 2. Flush logs batch
		a.logThrottleMu.Lock()
		if len(a.pendingLogs) > 0 {
			logsBatch := a.pendingLogs
			a.pendingLogs = make([]botPendingLogEntry, 0, 100)
			a.logThrottleMu.Unlock()

			runtime.EventsEmit(a.ctx, "event://bot/logs_batch", map[string]interface{}{
				"logs": logsBatch,
			})
		} else {
			a.logThrottleMu.Unlock()
		}

		// 3. Flush chat batch
		a.chatThrottleMu.Lock()
		if len(a.pendingChats) > 0 {
			chatsBatch := a.pendingChats
			a.pendingChats = make([]botPendingChatEntry, 0, 50)
			a.chatThrottleMu.Unlock()

			runtime.EventsEmit(a.ctx, "event://bot/chat_batch", map[string]interface{}{
				"chats": chatsBatch,
			})
		} else {
			a.chatThrottleMu.Unlock()
		}
	}
}

func (a *App) GetVPNStatus() vpn.Status {
	if !vpnEnabled {
		return vpn.Status{State: "off", Message: "VPN is disabled."}
	}
	return a.vpnManager.Status(mortiVPNTunnelID)
}

func (a *App) emitVPNStatus() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "event://vpn/status", a.GetVPNStatus())
	}
}

func (a *App) runVPNSetupMode() error {
	if !vpnEnabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := a.vpnManager.Setup(ctx, mortiVPNTunnelID)
	result := "ok"
	if err != nil {
		result = err.Error()
	}
	if writeErr := writeVPNSetupResult(result); writeErr != nil && err == nil {
		err = writeErr
	}
	return err
}

func vpnSetupResultPath() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Morti", "vpn-setup-result.txt")
}

func writeVPNSetupResult(result string) error {
	path := vpnSetupResultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(result), 0600)
}

func (a *App) ensureVPNSetup() error {
	if !vpnEnabled {
		return nil
	}
	if a.vpnManager.IsSetupDone(mortiVPNTunnelID) {
		return nil
	}
	a.vpnManager.SetStatus(mortiVPNTunnelID, vpn.Status{State: "setting_up", Message: "Setting up the SoftEther VPN service and adapter..."})
	a.emitVPNStatus()
	if err := a.vpnManager.ValidatePayload(); err != nil {
		a.vpnManager.SetStatus(mortiVPNTunnelID, vpn.Status{State: "error", Error: err.Error()})
		a.emitVPNStatus()
		return err
	}
	resultPath := vpnSetupResultPath()
	_ = os.Remove(resultPath)
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if err := vpn.RunElevated(executable, "--vpn-setup"); err != nil {
		if errors.Is(err, vpn.ErrUACCancelled) {
			a.vpnManager.SetStatus(mortiVPNTunnelID, vpn.Status{State: "error", Error: "Administrator approval was declined."})
			a.emitVPNStatus()
			return errors.New("administrator approval was declined")
		}
		a.vpnManager.SetStatus(mortiVPNTunnelID, vpn.Status{State: "error", Error: err.Error()})
		a.emitVPNStatus()
		return fmt.Errorf("VPN setup could not start: %w", err)
	}
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if a.vpnManager.IsSetupDone(mortiVPNTunnelID) {
			a.vpnManager.SetStatus(mortiVPNTunnelID, vpn.Status{State: "connecting", Message: "VPN setup complete; selecting a VPN Gate server..."})
			a.emitVPNStatus()
			return nil
		}
		if data, readErr := os.ReadFile(resultPath); readErr == nil {
			result := strings.TrimSpace(string(data))
			if result != "" && result != "ok" {
				a.vpnManager.SetStatus(mortiVPNTunnelID, vpn.Status{State: "error", Error: result})
				a.emitVPNStatus()
				return fmt.Errorf("VPN setup failed: %s", result)
			}
		}
		select {
		case <-timer.C:
			a.vpnManager.SetStatus(mortiVPNTunnelID, vpn.Status{State: "error", Error: "VPN setup did not finish within 90 seconds."})
			a.emitVPNStatus()
			return errors.New("VPN setup did not finish within 90 seconds")
		case <-ticker.C:
		}
	}
}

func (a *App) startVPNMonitor() {
	if !vpnEnabled {
		return
	}
	a.vpnMu.Lock()
	if a.vpnCancel != nil {
		a.vpnMu.Unlock()
		return
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	a.vpnCancel, a.vpnDone = cancel, done
	a.vpnMu.Unlock()

	go func() {
		defer close(done)
		a.vpnManager.Monitor(ctx, mortiVPNTunnelID, func(ctx context.Context) ([]vpn.Server, error) {
			servers, err := vpn.FetchServers(ctx, &http.Client{Timeout: 30 * time.Second})
			if err != nil {
				return nil, err
			}
			return vpn.Pick(servers, "", 8), nil
		}, func(_ string) {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "event://vpn/status", a.GetVPNStatus())
			}
		})
	}()
}

func (a *App) stopVPNMonitor() {
	a.vpnMu.Lock()
	cancel, done := a.vpnCancel, a.vpnDone
	a.vpnCancel, a.vpnDone = nil, nil
	a.vpnMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(20 * time.Second):
		}
	}
	ctx, cancelDisconnect := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelDisconnect()
	if a.vpnManager.IsSetupDone(mortiVPNTunnelID) {
		a.vpnManager.Disconnect(ctx, mortiVPNTunnelID)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "event://vpn/status", a.GetVPNStatus())
	}
}

func (a *App) ensureVPNConnected() error {
	if !vpnEnabled {
		return nil
	}
	if err := a.ensureVPNSetup(); err != nil {
		return err
	}
	a.startVPNMonitor()
	timer := time.NewTimer(4 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if a.vpnManager.IsUp(mortiVPNTunnelID) {
			return nil
		}
		status := a.GetVPNStatus()
		if status.State == "error" && status.Error != "" {
			return errors.New(status.Error)
		}
		select {
		case <-timer.C:
			return errors.New("VPN connection timed out")
		case <-ticker.C:
		}
	}
}

func (a *App) reconcileVPN() {
	if !vpnEnabled {
		return
	}
	for _, summary := range a.botManager.ListBots() {
		if summary.Preferences.BypassLogon {
			a.startVPNMonitor()
			return
		}
	}
	a.stopVPNMonitor()
}

func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	if a.isShuttingDown.Swap(true) {
		return false
	}
	a.doShutdown()
	return false
}

func (a *App) shutdown(ctx context.Context) {
	if a.isShuttingDown.Swap(true) {
		return
	}
	a.doShutdown()
}

func (a *App) doShutdown() {
	if a.botManager != nil {
		a.botManager.StopAllWithTimeout(5 * time.Second)
	}
	if a.vpnManager != nil {
		a.stopVPNMonitor()
	}
	presence.Stop()
	_, _ = a.SaveBotsFile()
}

func (a *App) updatePresenceThrottled() {
	a.presenceMu.Lock()
	now := time.Now()
	if now.Sub(a.lastPresenceUpdate) < 3*time.Second {
		a.presenceMu.Unlock()
		return
	}
	a.lastPresenceUpdate = now
	a.presenceMu.Unlock()
	a.updatePresence()
}

func (a *App) updatePresence() {
	if a.botManager == nil {
		return
	}
	a.presenceMu.RLock()
	tab := a.presenceTab
	a.presenceMu.RUnlock()
	if tab == "" {
		tab = "Main"
	}
	running := a.botManager.RunningCount()
	presence.Update(presence.State{Tab: tab, RunningBots: running})
}

func (a *App) UpdatePresenceTab(tab string) {
	switch tab {
	case "Main", "List", "Executor", "Database", "Settings":
	default:
		tab = "Main"
	}
	a.presenceMu.Lock()
	a.presenceTab = tab
	a.presenceMu.Unlock()
	a.updatePresence()
}

func (a *App) GetAppSettings() AppSettings {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.settings
}

func (a *App) SaveAppSettings(settings AppSettings) error {
	if err := saveAppSettings(settings); err != nil {
		return err
	}
	a.settingsMu.Lock()
	a.settings = settings
	a.settingsMu.Unlock()
	a.enableLogs.Store(settings.EnableLogs)
	a.enableConsole.Store(settings.EnableConsole)
	if a.botManager != nil {
		a.botManager.SetLogsEnabled(settings.EnableLogs)
	}

	presence.SetEnabled(settings.DiscordRichPresence)
	if settings.DiscordRichPresence {
		a.updatePresence()
	}
	return nil
}

func (a *App) ListBots() []models.BotSummary {
	return a.botManager.ListBots()
}

func (a *App) GetPartyBoostConfig() models.PartyBoostConfig {
	if a.botManager != nil {
		return a.botManager.GetPartyBoostConfig()
	}
	return models.DefaultPartyBoostConfig()
}

func (a *App) SavePartyBoostConfig(cfg models.PartyBoostConfig) error {
	if a.botManager != nil {
		return a.botManager.SavePartyBoostConfig(cfg)
	}
	return errors.New("bot manager not initialized")
}

func (a *App) GetPartyRoster() []models.PartyRosterEntry {
	if a.botManager != nil {
		return a.botManager.DiscoverPartyRoster()
	}
	return nil
}

func (a *App) SyncPartyBoost() error {
	if a.botManager != nil {
		return a.botManager.SyncPartyBoost()
	}
	return errors.New("bot manager not initialized")
}

func (a *App) CreateBot(profile models.BotProfile) (models.BotSummary, error) {
	if strings.TrimSpace(profile.Name) == "" {
		return models.BotSummary{}, errors.New("bot name cannot be empty")
	}
	s, err := a.botManager.CreateBot(profile, nil)
	if err == nil {
		_, _ = a.SaveBotsFile()
	}
	return s, err
}

func (a *App) StartBot(id string) (*models.BotSummary, error) {
	if a.isShuttingDown.Load() {
		return nil, errors.New("application is shutting down")
	}
	profile, err := a.botManager.GetBot(id)
	if err != nil {
		return nil, err
	}
	if profile.Preferences.BypassLogon {
		if err := a.ensureVPNConnected(); err != nil {
			return nil, err
		}
	}
	s, err := a.botManager.StartBot(id)
	if a.ctx != nil {
		currentBot, _ := a.botManager.GetBot(id)
		if currentBot != nil {
			a.lastStatusMu.Lock()
			a.lastStatusMap[currentBot.ID] = currentBot.Status
			a.lastStatusMu.Unlock()
			runtime.EventsEmit(a.ctx, "event://bot/status_changed", map[string]interface{}{
				"bot_id":         currentBot.ID,
				"status":         currentBot.Status,
				"status_message": currentBot.StatusMessage,
				"timestamp_ms":   time.Now().UnixMilli(),
			})
			if err == nil {
				runtime.EventsEmit(a.ctx, "event://bot/stats_updated", map[string]interface{}{
					"bot_id": currentBot.ID,
					"stats":  currentBot.Stats,
				})
			}
		}
	}
	if err != nil {
		_, _ = a.SaveBotsFile()
	}
	return s, err
}

func (a *App) StopBot(id string) (*models.BotSummary, error) {
	s, err := a.botManager.StopBot(id)
	if err == nil && a.ctx != nil {
		runtime.EventsEmit(a.ctx, "event://bot/status_changed", map[string]interface{}{
			"bot_id":         s.ID,
			"status":         s.Status,
			"status_message": s.StatusMessage,
			"timestamp_ms":   time.Now().UnixMilli(),
		})
	}
	return s, err
}

func (a *App) RemoveBots(ids []string) error {
	a.lastStatusMu.Lock()
	for _, id := range ids {
		delete(a.lastStatusMap, id)
	}
	a.lastStatusMu.Unlock()

	a.statsThrottleMu.Lock()
	for _, id := range ids {
		delete(a.lastStatsEmit, id)
		delete(a.pendingStats, id)
	}
	a.statsThrottleMu.Unlock()

	for _, id := range ids {
		_ = a.botManager.RemoveBot(id)
		if a.multiDispatcher != nil {
			a.multiDispatcher.RemoveBot(id)
		}
	}
	_, _ = a.SaveBotsFile()
	a.reconcileVPN()
	return nil
}

func (a *App) LoginAccount(req RegisterAccountRequest) (models.BotSummary, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return models.BotSummary{}, errors.New("username cannot be empty")
	}
	if req.Password == "" {
		return models.BotSummary{}, errors.New("password cannot be empty")
	}

	auth, err := a.httpCli.Login(username, req.Password)
	if err != nil {
		return models.BotSummary{}, err
	}

	charName := ""
	for _, s := range auth.Account.Slots {
		if strings.TrimSpace(s) != "" {
			charName = strings.TrimSpace(s)
			break
		}
	}

	if charName == "" {
		charName = username
		acc, cErr := a.httpCli.CreateCharacter(auth.Token, 0, charName)
		if cErr == nil && acc != nil && len(acc.Slots) > 0 && acc.Slots[0] != "" {
			charName = acc.Slots[0]
		}
		accKey := auth.Account.Key
		if accKey == "" {
			accKey = strings.ToLower(username)
		}
		initJson := models.CreateDefaultCharacterSave(charName, "male")
		_, _ = a.httpCli.SaveFiles(auth.Token, map[string]string{
			fmt.Sprintf("chars/%s_0.json", accKey): initJson,
		})
		if auth.Files == nil {
			auth.Files = make(map[string]string)
		}
		auth.Files[fmt.Sprintf("chars/%s_0.json", accKey)] = initJson
	}

	prefs := models.DefaultPreferences()
	if req.Target1stJob != "" {
		prefs.JobConfig.Target1stJob = req.Target1stJob
	}
	if req.Target2ndJob != "" {
		prefs.JobConfig.Target2ndJob = req.Target2ndJob
	}

	initialStats := models.DefaultStats()
	if cs := models.ParseCharacterStats(auth.Files, charName); cs != nil {
		initialStats.Level = cs.Level
		initialStats.Gold = cs.Penya
		initialStats.Job = models.FormatJobName(cs.Job)
		initialStats.HP = cs.HP
		initialStats.MP = cs.MP
		initialStats.FP = cs.FP
		initialStats.STR = cs.STR
		if initialStats.STR == 0 {
			initialStats.STR = 15
		}
		initialStats.STA = cs.STA
		if initialStats.STA == 0 {
			initialStats.STA = 15
		}
		initialStats.DEX = cs.DEX
		if initialStats.DEX == 0 {
			initialStats.DEX = 15
		}
		initialStats.INT = cs.INT
		if initialStats.INT == 0 {
			initialStats.INT = 15
		}
		initialStats.StatPoints = cs.Points
		initialStats.AutoStat = cs.AutoStat
		if len(cs.Pos) >= 3 {
			initialStats.PosX = cs.Pos[0]
			initialStats.PosY = cs.Pos[1]
			initialStats.PosZ = cs.Pos[2]
			initialStats.Zone = models.DetermineZone(cs.Map, cs.Pos[0], cs.Pos[2])
		}

		cleanJob := strings.ToLower(strings.TrimSpace(cs.Job))
		if cleanJob != "" && cleanJob != "vagrant" {
			if models.IsFirstJob(cleanJob) {
				prefs.JobConfig.Target1stJob = cleanJob
				if secs := models.SecondJobsMap[cleanJob]; len(secs) > 0 {
					prefs.JobConfig.Target2ndJob = secs[0]
				}
			} else if models.IsSecondJob(cleanJob) {
				prefs.JobConfig.Target2ndJob = cleanJob
			}
		}
	}

	// If bot already in list, update stats
	for _, existing := range a.botManager.ListBots() {
		if strings.EqualFold(existing.Name, username) {
			s, err := a.botManager.UpdateStats(existing.ID, initialStats)
			if err == nil {
				_, _ = a.SaveBotsFile()
				return *s, nil
			}
			return existing, nil
		}
	}

	serverSetting := req.Server
	if strings.TrimSpace(serverSetting) == "" {
		serverSetting = "Channel 2 (Safe - No Giants)"
	}

	id := fmt.Sprintf("bot_%d_%s", time.Now().UnixMilli(), username)
	profile := models.BotProfile{
		ID:            id,
		Name:          username,
		Server:        serverSetting,
		CharacterName: charName,
		Preferences:   prefs,
		Intervals:     models.DefaultIntervals(),
		Password:      req.Password,
	}

	s, bErr := a.botManager.CreateBot(profile, &initialStats)
	if bErr == nil {
		_, _ = a.SaveBotsFile()
	}
	return s, bErr
}

func (a *App) RegisterAccount(req RegisterAccountRequest) (models.BotSummary, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return models.BotSummary{}, errors.New("username cannot be empty")
	}
	if req.Password == "" {
		return models.BotSummary{}, errors.New("password cannot be empty")
	}

	auth, err := a.httpCli.Register(username, req.Password)
	if err != nil {
		return models.BotSummary{}, err
	}

	charName := ""
	for _, s := range auth.Account.Slots {
		if strings.TrimSpace(s) != "" {
			charName = strings.TrimSpace(s)
			break
		}
	}

	if charName == "" {
		charName = username
		acc, cErr := a.httpCli.CreateCharacter(auth.Token, 0, charName)
		if cErr == nil && acc != nil && len(acc.Slots) > 0 && acc.Slots[0] != "" {
			charName = acc.Slots[0]
		}
		accKey := auth.Account.Key
		if accKey == "" {
			accKey = strings.ToLower(username)
		}
		initJson := models.CreateDefaultCharacterSave(charName, "male")
		_, _ = a.httpCli.SaveFiles(auth.Token, map[string]string{
			fmt.Sprintf("chars/%s_0.json", accKey): initJson,
		})
	}

	id := fmt.Sprintf("bot_%d_%s", time.Now().UnixMilli(), username)
	serverSetting := req.Server
	if strings.TrimSpace(serverSetting) == "" {
		serverSetting = "Channel 2 (Safe - No Giants)"
	}

	prefs := models.DefaultPreferences()
	if req.Target1stJob != "" {
		prefs.JobConfig.Target1stJob = req.Target1stJob
	}
	if req.Target2ndJob != "" {
		prefs.JobConfig.Target2ndJob = req.Target2ndJob
	}

	profile := models.BotProfile{
		ID:            id,
		Name:          username,
		Server:        serverSetting,
		CharacterName: charName,
		Preferences:   prefs,
		Intervals:     models.DefaultIntervals(),
		Password:      req.Password,
	}

	s, bErr := a.botManager.CreateBot(profile, nil)
	if bErr == nil {
		_, _ = a.SaveBotsFile()
	}
	return s, bErr
}

func (a *App) GetServerChannels() ([]models.ChannelInfo, error) {
	return a.httpCli.GetChannels()
}

func (a *App) SaveBotsFile() (string, error) {
	bots := a.botManager.ListBots()
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(filepath.Dir(exePath), "bot.json")

	data, err := json.MarshalIndent(bots, "", "  ")
	if err != nil {
		return "", err
	}

	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return "", err
	}
	_ = os.Remove(targetPath)
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func (a *App) ImportBotsFile() ([]models.BotSummary, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	targetPath := filepath.Join(filepath.Dir(exePath), "bot.json")

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, err
	}

	var profiles []models.BotProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}

	for _, p := range profiles {
		_, _ = a.botManager.CreateBot(p, nil)
	}
	return a.botManager.ListBots(), nil
}

func (a *App) UpdateBotPreferences(id string, prefs models.BotPreferences) (*models.BotSummary, error) {
	previous, err := a.botManager.GetBot(id)
	if err != nil {
		return nil, err
	}
	changedVPN := previous.Preferences.BypassLogon != prefs.BypassLogon
	wasActive := previous.Status == "online" || previous.Status == "connecting"
	if changedVPN && wasActive {
		if _, err := a.botManager.StopBot(id); err != nil {
			return nil, err
		}
	}
	if prefs.BypassLogon && !previous.Preferences.BypassLogon {
		if err := a.ensureVPNSetup(); err != nil {
			if wasActive {
				_, _ = a.botManager.StartBot(id)
			}
			return nil, err
		}
	}
	s, err := a.botManager.UpdatePreferences(id, prefs)
	if err != nil {
		if wasActive {
			_, _ = a.botManager.StartBot(id)
		}
		return nil, err
	}
	_, _ = a.SaveBotsFile()
	if prefs.BypassLogon {
		a.startVPNMonitor()
	} else {
		a.reconcileVPN()
	}
	if changedVPN && wasActive {
		if prefs.BypassLogon {
			if err := a.ensureVPNConnected(); err != nil {
				return s, err
			}
		}
		return a.StartBot(id)
	}
	return s, nil
}

func (a *App) UpdateBotIntervals(id string, intervals models.BotIntervals) (*models.BotSummary, error) {
	return a.botManager.UpdateIntervals(id, intervals)
}

func (a *App) UpdateBotServer(id string, server string) (*models.BotSummary, error) {
	return a.botManager.UpdateServer(id, server)
}

func (a *App) WarpBot(id string, target string) (*models.BotSummary, error) {
	return a.botManager.WarpBot(id, target)
}

func (a *App) FollowTarget(id string, target string) (*models.BotSummary, error) {
	return a.botManager.FollowTarget(id, target)
}

func (a *App) MoveBot(id string, dx, dz float32) error {
	return a.botManager.MoveBot(id, dx, dz)
}

func (a *App) StopMoveBot(id string) error {
	return a.botManager.StopMoveBot(id)
}

func (a *App) SetBotWaypoint(id string, x, z float32) (*models.BotSummary, error) {
	s, err := a.botManager.SetBotWaypoint(id, x, z)
	if err == nil && a.ctx != nil && s != nil {
		runtime.EventsEmit(a.ctx, "event://bot/status_changed", map[string]interface{}{
			"bot_id":         s.ID,
			"status":         s.Status,
			"status_message": s.StatusMessage,
			"timestamp_ms":   time.Now().UnixMilli(),
		})
		runtime.EventsEmit(a.ctx, "event://bot/stats_updated", map[string]interface{}{
			"bot_id": s.ID,
			"stats":  s.Stats,
		})
	}
	return s, err
}

func (a *App) ClearBotWaypoint(id string) (*models.BotSummary, error) {
	s, err := a.botManager.ClearBotWaypoint(id)
	if err == nil && a.ctx != nil && s != nil {
		runtime.EventsEmit(a.ctx, "event://bot/status_changed", map[string]interface{}{
			"bot_id":         s.ID,
			"status":         s.Status,
			"status_message": s.StatusMessage,
			"timestamp_ms":   time.Now().UnixMilli(),
		})
		runtime.EventsEmit(a.ctx, "event://bot/stats_updated", map[string]interface{}{
			"bot_id": s.ID,
			"stats":  s.Stats,
		})
	}
	return s, err
}

func (a *App) AdvanceJob(id string, targetJob string) (*models.BotSummary, error) {
	s, err := a.botManager.AdvanceJob(id, targetJob)
	if err == nil && a.ctx != nil && s != nil {
		runtime.EventsEmit(a.ctx, "event://bot/stats_updated", map[string]interface{}{
			"bot_id": s.ID,
			"stats":  s.Stats,
		})
	}
	return s, err
}

func (a *App) GetStatsJson() (models.StatsJsonMap, error) {
	return bot.LoadStatsJson()
}

func (a *App) SaveStatsJson(cfg models.StatsJsonMap) error {
	return bot.SaveStatsJson(cfg)
}

func (a *App) AllocateCharacterStats(id string, addStr, addSta, addDex, addInt, autoStat int) (*models.BotSummary, error) {
	s, err := a.botManager.AllocateStats(id, addStr, addSta, addDex, addInt, autoStat)
	if err == nil && a.ctx != nil && s != nil {
		runtime.EventsEmit(a.ctx, "event://bot/stats_updated", map[string]interface{}{
			"bot_id": s.ID,
			"stats":  s.Stats,
		})
	}
	return s, err
}

func (a *App) GetCharacterInventory(id string) (map[string]interface{}, error) {
	return a.botManager.GetInventory(id)
}

func (a *App) ToggleItemLock(id string, slotIndex int) (map[string]interface{}, error) {
	return a.botManager.ToggleItemLock(id, slotIndex)
}

func (a *App) ToggleItemJunk(id string, slotIndex int) (map[string]interface{}, error) {
	return a.botManager.ToggleItemJunk(id, slotIndex)
}

func (a *App) EquipItem(id string, slotIndex int, targetSlot string) (map[string]interface{}, error) {
	return a.botManager.EquipItem(id, slotIndex, targetSlot)
}

func (a *App) UnequipItem(id string, slotKey string) (map[string]interface{}, error) {
	return a.botManager.UnequipItem(id, slotKey)
}

func (a *App) DropItem(id string, slotIndex int) (map[string]interface{}, error) {
	return a.botManager.DropItem(id, slotIndex)
}

func (a *App) UseInventoryItem(id string, slotIndex int) (map[string]interface{}, error) {
	return a.botManager.UseInventoryItem(id, slotIndex)
}

func (a *App) SortInventory(id string, by string, keepLocked bool) (map[string]interface{}, error) {
	return a.botManager.SortInventory(id, by, keepLocked)
}

func (a *App) SendChat(id string, message string) error {
	return a.botManager.SendChat(id, message)
}

func (a *App) RedeemCode(id string, code string) (map[string]interface{}, error) {
	return a.botManager.RedeemCode(id, code)
}

func (a *App) ClaimDailyGift(id string) (map[string]interface{}, error) {
	return a.botManager.ClaimDailyGift(id)
}

func (a *App) GetDailyRewardStatus(id string) (map[string]interface{}, error) {
	return a.botManager.GetDailyRewardStatus(id)
}

func (a *App) GetAchievements(id string) (map[string]interface{}, error) {
	return a.botManager.GetAchievements(id)
}

func (a *App) SetTitle(id string, title string) (map[string]interface{}, error) {
	return a.botManager.SetTitle(id, title)
}

func (a *App) GetMonsterAlbum(id string) (map[string]interface{}, error) {
	return a.botManager.GetMonsterAlbum(id)
}

func (a *App) UseMonsterAlbumPiece(id string, mobID string) (map[string]interface{}, error) {
	return a.botManager.UseMonsterAlbumPiece(id, mobID)
}

func (a *App) GetAccountCash(id string) (map[string]interface{}, error) {
	return a.botManager.GetAccountCash(id)
}

func (a *App) BuyCashShopItem(id string, itemID string, n int, amount int, kind string, giftTo string) (map[string]interface{}, error) {
	return a.botManager.BuyCashShopItem(id, itemID, n, amount, kind, giftTo)
}

func (a *App) BuyShopItem(id string, itemID string, itemName string, count int, unitCost int) (map[string]interface{}, error) {
	return a.botManager.BuyShopItem(id, itemID, itemName, count, unitCost)
}

func (a *App) SellShopItem(id string, slotIndex int, count int, unitPrice int) (map[string]interface{}, error) {
	return a.botManager.SellShopItem(id, slotIndex, count, unitPrice)
}

func (a *App) SellJunkItems(id string) (map[string]interface{}, error) {
	return a.botManager.SellJunkItems(id)
}

func (a *App) GetNearbyPlayers(id string) ([]models.NearbyPlayerInfo, error) {
	return a.botManager.GetNearbyPlayers(id)
}

func (a *App) SendMail(id string, to string, title string, text string, slotIndex int, count int, penya int) (map[string]interface{}, error) {
	return a.botManager.SendMail(id, to, title, text, slotIndex, count, penya)
}

func (a *App) StartTrade(id string, target string, slotIndex int, count int, penya int) (map[string]interface{}, error) {
	return a.botManager.StartTrade(id, target, slotIndex, count, penya)
}

func (a *App) GetAppVersion() map[string]interface{} {
	return version.Details()
}

func (a *App) GetFarmConfig(id string) (models.FarmConfig, error) {
	return a.botManager.GetFarmConfig(id)
}

func (a *App) SaveFarmConfig(id string, cfg models.FarmConfig) error {
	return a.botManager.SaveFarmConfig(id, cfg)
}

func (a *App) StartFarm(id string) error {
	return a.botManager.StartFarm(id)
}

func (a *App) StopFarm(id string) error {
	return a.botManager.StopFarm(id)
}

func (a *App) GetFarmStats(id string) (models.FarmStats, error) {
	return a.botManager.GetFarmStats(id)
}

func (a *App) TestDiscordWebhook(webhookURL string) error {
	if strings.TrimSpace(webhookURL) == "" {
		return fmt.Errorf("webhook URL is empty")
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       "Mortis Farm Alert Test",
				"description": "✅ Discord webhook connection verified successfully from Mortis!",
				"color":       0x2ecc71,
				"footer": map[string]string{
					"text": "Mortis Automation Engine",
				},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", webhookURL, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// ============================================================================
// CENTRAL ACTION MENU & INTERACTIVE DIALOG WAILS BINDINGS
// ============================================================================

// 1. MAIL SERVICE
func (a *App) GetMailInbox(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.GetMailInbox()
}

func (a *App) ReadMail(id string, mailID int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.ReadMail(mailID)
}

func (a *App) CollectMailAttachment(id string, mailID int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.CollectMailAttachment(mailID)
}

func (a *App) CollectAllMail(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.CollectAllMail()
}

func (a *App) CollectAllMailMulti(botIDs []string) (map[string]interface{}, error) {
	totalCollected := 0
	totalPenya := int64(0)
	botSuccess := 0
	botErrors := make(map[string]string)

	for _, id := range botIDs {
		inst, err := a.botManager.GetInstance(id)
		if err != nil {
			botErrors[id] = err.Error()
			continue
		}
		res, err := inst.CollectAllMail()
		if err != nil {
			botErrors[id] = err.Error()
			continue
		}
		botSuccess++
		if c, ok := res["collected_items"].(int); ok {
			totalCollected += c
		}
		if p, ok := res["collected_penya"].(int64); ok {
			totalPenya += p
		}
	}

	return map[string]interface{}{
		"success":         true,
		"bots_processed":  botSuccess,
		"total_collected": totalCollected,
		"total_penya":     totalPenya,
		"errors":          botErrors,
	}, nil
}

func (a *App) DeleteMail(id string, mailID int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.DeleteMail(mailID)
}

func (a *App) SendMailFull(id, to, title, text string, slotIndex, count, penya, cod int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.SendMailFull(to, title, text, slotIndex, count, penya, cod)
}

// 2. BOUNTY & CONTRACTS
func (a *App) GetBountyContracts(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.GetBountyContracts()
}

func (a *App) ClaimBountyContract(id string, index int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.ClaimBountyContract(index)
}

func (a *App) GetBountyBoard(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.GetBountyBoard()
}

func (a *App) TakeBounty(id string, bountyID int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.TakeBounty(bountyID)
}

func (a *App) DropBounty(id string, bountyID int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.DropBounty(bountyID)
}

func (a *App) TriggerDailyTrial(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.TriggerDailyTrial()
}

// 3. REWARDS, BATTLE PASS & GACHA
func (a *App) GetBattlePass(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.GetBattlePass()
}

func (a *App) PullGacha(id string, wheelIndex int, count int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.PullGacha(wheelIndex, count)
}

// 4. BANK & PIN
func (a *App) GetBankData(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.GetBankData()
}

func (a *App) CheckBankPin(id string, pin string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.CheckBankPin(pin)
}

func (a *App) SetBankPin(id string, pin string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.SetBankPin(pin)
}

func (a *App) DepositBankItem(id string, slotIndex int, count int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.DepositBankItem(slotIndex, count)
}

func (a *App) WithdrawBankItem(id string, bankSlotIndex int, count int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.WithdrawBankItem(bankSlotIndex, count)
}

func (a *App) DepositBankPenya(id string, amount int64) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.DepositBankPenya(amount)
}

func (a *App) WithdrawBankPenya(id string, amount int64) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.WithdrawBankPenya(amount)
}

// 5. SOCIAL & COUPLE
func (a *App) GetCoupleInfo(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.GetCoupleInfo()
}

func (a *App) ClaimCoupleReward(id string, level int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.ClaimCoupleReward(level)
}

func (a *App) ProposeCouple(id string, targetName string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.ProposeCouple(targetName)
}

func (a *App) BreakCouple(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.BreakCouple()
}

func (a *App) SendPartyInvite(id string, targetName string) error {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return err
	}
	return inst.SendPartyInvite(targetName)
}

func (a *App) AcceptPartyInvite(id string, fromID int) error {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return err
	}
	return inst.AcceptPartyInvite(fromID)
}

func (a *App) LeaveParty(id string) error {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return err
	}
	return inst.LeaveParty()
}

// 6. PETS & MILL
func (a *App) HatchPetEgg(id string) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.HatchPetEgg()
}

func (a *App) MillPetFeed(id string, itemID string, count int) (map[string]interface{}, error) {
	inst, err := a.botManager.GetInstance(id)
	if err != nil {
		return nil, err
	}
	return inst.MillPetFeed(itemID, count)
}
