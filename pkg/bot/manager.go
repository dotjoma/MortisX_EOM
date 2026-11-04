package bot

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"madrigal-multibot/pkg/models"
	"madrigal-multibot/pkg/vpn"
)

type BotManager struct {
	mu                 sync.RWMutex
	bots               map[string]*BotInstance
	nameToID           map[string]string
	logsEnabled        atomic.Bool
	onUpdate           func(id string, stats models.BotRuntimeStats, status string, statusMsg string)
	onLog              func(botID, category, message, level string)
	onChat             func(botID, channel, sender, text, color string)
	onInventoryUpdated func(botID string, invData map[string]interface{})
	vpnManager         *vpn.Manager
	vpnTunnelID        string
	partyBoost         *PartyBoostManager
}

func NewBotManager(
	onUpdate func(id string, stats models.BotRuntimeStats, status string, statusMsg string),
	onLog func(botID, category, message, level string),
	onChat func(botID, channel, sender, text, color string),
) *BotManager {
	GetGlobalCoordinator().SetLogger(onLog)
	bm := &BotManager{
		bots:     make(map[string]*BotInstance),
		nameToID: make(map[string]string),
		onUpdate: onUpdate,
		onLog:    onLog,
		onChat:   onChat,
	}
	bm.logsEnabled.Store(true)
	bm.partyBoost = NewPartyBoostManager(bm)
	return bm
}

func (m *BotManager) ListBots() []models.BotSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summaries := make([]models.BotSummary, 0, len(m.bots))
	for _, b := range m.bots {
		summaries = append(summaries, b.Summary())
	}
	return summaries
}

func (m *BotManager) GetBot(id string) (*models.BotSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	b, ok := m.bots[id]
	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}
	s := b.Summary()
	return &s, nil
}

func (m *BotManager) SetOnInventoryUpdated(cb func(botID string, invData map[string]interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onInventoryUpdated = cb
	for _, b := range m.bots {
		b.SetOnInventoryUpdated(cb)
	}
}

func (m *BotManager) SetVPNManager(manager *vpn.Manager, tunnelID string) {
	m.mu.Lock()
	m.vpnManager = manager
	m.vpnTunnelID = tunnelID
	for _, instance := range m.bots {
		instance.SetVPNManager(manager, tunnelID)
	}
	m.mu.Unlock()
}

func (m *BotManager) SetLogsEnabled(enabled bool) {
	m.logsEnabled.Store(enabled)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, b := range m.bots {
		b.SetLogsEnabled(enabled)
	}
}

func (m *BotManager) CreateBot(profile models.BotProfile, initialStats *models.BotRuntimeStats) (models.BotSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.bots[profile.ID]; ok {
		return models.BotSummary{}, fmt.Errorf("bot %s already exists", profile.ID)
	}

	instance := NewBotInstance(profile, initialStats, m.onUpdate, m.onLog, m.onChat)
	instance.SetLogsEnabled(m.logsEnabled.Load())
	instance.SetVPNManager(m.vpnManager, m.vpnTunnelID)
	instance.SetFindOtherBot(m.FindBotByName)
	if m.partyBoost != nil {
		instance.SetPartyBoostManager(m.partyBoost)
	}
	if m.onInventoryUpdated != nil {
		instance.SetOnInventoryUpdated(m.onInventoryUpdated)
	}
	m.bots[profile.ID] = instance

	// Fast index for FindBotByName (F-02)
	if n := strings.ToLower(strings.TrimSpace(profile.Name)); n != "" {
		m.nameToID[n] = profile.ID
	}
	if cn := strings.ToLower(strings.TrimSpace(profile.CharacterName)); cn != "" {
		m.nameToID[cn] = profile.ID
	}

	return instance.Summary(), nil
}

func (m *BotManager) StartBot(id string) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	if err := instance.Start(); err != nil {
		return nil, err
	}
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) StopBot(id string) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	if err := instance.Stop(); err != nil {
		return nil, err
	}
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) RunningCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, b := range m.bots {
		if b.IsRunning() {
			count++
		}
	}
	return count
}

func (m *BotManager) StopAll() {
	m.StopAllWithTimeout(8 * time.Second)
}

func (m *BotManager) StopAllWithTimeout(timeout time.Duration) {
	if m.partyBoost != nil {
		m.partyBoost.Stop()
	}

	m.mu.RLock()
	instances := make([]*BotInstance, 0, len(m.bots))
	for _, b := range m.bots {
		instances = append(instances, b)
	}
	m.mu.RUnlock()

	done := make(chan struct{})
	var wg sync.WaitGroup
	for _, inst := range instances {
		wg.Add(1)
		go func(b *BotInstance) {
			defer wg.Done()
			_ = b.Stop()
		}(inst)
	}
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (m *BotManager) RemoveBot(id string) error {
	m.mu.Lock()
	instance, ok := m.bots[id]
	if ok {
		if n := strings.ToLower(strings.TrimSpace(instance.Profile.Name)); n != "" {
			delete(m.nameToID, n)
		}
		if cn := strings.ToLower(strings.TrimSpace(instance.Profile.CharacterName)); cn != "" {
			delete(m.nameToID, cn)
		}
		delete(m.bots, id)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("bot %s not found", id)
	}

	_ = instance.Stop()
	return nil
}

func (m *BotManager) UpdateStats(id string, stats models.BotRuntimeStats) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	instance.mu.Lock()
	instance.Stats = stats
	instance.mu.Unlock()
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) GetInstance(id string) (*BotInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.bots[id]
	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}
	return inst, nil
}

func (m *BotManager) MutatePreferences(id string, mutate func(*models.BotPreferences)) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	instance.MutatePreferences(mutate)
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) UpdatePreferences(id string, prefs models.BotPreferences) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	instance.UpdatePreferences(prefs)
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) UpdateIntervals(id string, intervals models.BotIntervals) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	instance.UpdateIntervals(intervals)
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) FindBotByName(name string) *models.BotSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	targetLower := strings.ToLower(strings.TrimSpace(name))
	if id, ok := m.nameToID[targetLower]; ok {
		if b, exists := m.bots[id]; exists {
			s := b.Summary()
			return &s
		}
	}
	return nil
}

func (m *BotManager) FindInstanceByName(name string) *BotInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	targetLower := strings.ToLower(strings.TrimSpace(name))
	if targetLower == "" {
		return nil
	}
	if id, ok := m.nameToID[targetLower]; ok {
		if b, exists := m.bots[id]; exists {
			return b
		}
	}
	for _, inst := range m.bots {
		s := inst.Summary()
		if strings.EqualFold(s.CharacterName, targetLower) || strings.EqualFold(s.Name, targetLower) {
			return inst
		}
	}
	return nil
}

func (m *BotManager) GetAllInstances() []*BotInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*BotInstance, 0, len(m.bots))
	for _, inst := range m.bots {
		res = append(res, inst)
	}
	return res
}

func (m *BotManager) UpdateServer(id string, server string) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	if err := instance.UpdateServer(server); err != nil {
		return nil, err
	}
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) WarpBot(id string, target string) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	if err := instance.Warp(target); err != nil {
		return nil, err
	}
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) FollowTarget(id string, target string) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	instance.SetFollowTarget(target)
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) AdvanceJob(id string, targetJob string) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.AdvanceJob(targetJob)
}

func (m *BotManager) AllocateStats(id string, addStr, addSta, addDex, addInt, autoStat int) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.AllocateStats(addStr, addSta, addDex, addInt, autoStat)
}

func (m *BotManager) MoveBot(id string, dx, dz float32) error {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("bot %s not found", id)
	}

	instance.Move(dx, dz)
	return nil
}

func (m *BotManager) StopMoveBot(id string) error {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("bot %s not found", id)
	}

	instance.StopMove()
	return nil
}

func (m *BotManager) SetBotWaypoint(id string, x, z float32) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	instance.SetWaypoint(x, z)
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) ClearBotWaypoint(id string) (*models.BotSummary, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	instance.ClearWaypoint()
	s := instance.Summary()
	return &s, nil
}

func (m *BotManager) GetInventory(id string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.GetInventoryData()
}

func (m *BotManager) ToggleItemLock(id string, slotIndex int) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.ToggleItemLock(slotIndex)
}

func (m *BotManager) ToggleItemJunk(id string, slotIndex int) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.ToggleItemJunk(slotIndex)
}

func (m *BotManager) EquipItem(id string, slotIndex int, targetSlot string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.EquipItem(slotIndex, targetSlot)
}

func (m *BotManager) UnequipItem(id string, slotKey string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.UnequipItem(slotKey)
}

func (m *BotManager) DropItem(id string, slotIndex int) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.DropItem(slotIndex)
}

func (m *BotManager) UseInventoryItem(id string, slotIndex int) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.UseInventoryItem(slotIndex)
}

func (m *BotManager) SortInventory(id string, by string, keepLocked bool) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.SortInventory(by, keepLocked)
}

func (m *BotManager) SendChat(id string, message string) error {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("bot %s not found", id)
	}

	return instance.SendChat(message)
}

func (m *BotManager) RedeemCode(id string, code string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.RedeemCode(code)
}

func (m *BotManager) ClaimDailyGift(id string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.ClaimDailyGift()
}

func (m *BotManager) GetDailyRewardStatus(id string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.GetDailyRewardStatus()
}

func (m *BotManager) GetAchievements(id string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.GetAchievements()
}

func (m *BotManager) SetTitle(id string, title string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.SetTitle(title)
}

func (m *BotManager) GetMonsterAlbum(id string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.GetMonsterAlbum()
}

func (m *BotManager) UseMonsterAlbumPiece(id string, mobID string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.UseMonsterAlbumPiece(mobID)
}

func (m *BotManager) GetAccountCash(id string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.GetAccountCash()
}

func (m *BotManager) BuyCashShopItem(id string, itemID string, n int, amount int, kind string, giftTo string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.BuyCashShopItem(itemID, n, amount, kind, giftTo)
}

func (m *BotManager) BuyShopItem(id string, itemID string, itemName string, count int, unitCost int) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.BuyShopItem(itemID, itemName, count, unitCost)
}

func (m *BotManager) SellShopItem(id string, slotIndex int, count int, unitPrice int) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.SellShopItem(slotIndex, count, unitPrice)
}

func (m *BotManager) SellJunkItems(id string) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.SellJunkItems()
}

func (m *BotManager) CleanInventory(id string, rules CleanRules) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.CleanInventory(rules)
}

func (m *BotManager) GetNearbyPlayers(id string) ([]models.NearbyPlayerInfo, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.GetNearbyPlayers(), nil
}

func (m *BotManager) SendMail(id string, to string, title string, text string, slotIndex int, count int, penya int) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.SendMail(to, title, text, slotIndex, count, penya)
}

func (m *BotManager) StartTrade(id string, target string, slotIndex int, count int, penya int) (map[string]interface{}, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("bot %s not found", id)
	}

	return instance.StartTrade(target, slotIndex, count, penya)
}

func (m *BotManager) GetFarmConfig(id string) (models.FarmConfig, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return models.DefaultFarmConfig(), fmt.Errorf("bot %s not found", id)
	}

	if instance.Farm == nil {
		return models.DefaultFarmConfig(), fmt.Errorf("farm engine not initialized")
	}
	return instance.Farm.GetConfig(), nil
}

func (m *BotManager) SaveFarmConfig(id string, cfg models.FarmConfig) error {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("bot %s not found", id)
	}

	if instance.Farm == nil {
		return fmt.Errorf("farm engine not initialized")
	}
	instance.Farm.SetConfig(cfg)
	return nil
}

func (m *BotManager) StartFarm(id string) error {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("bot %s not found", id)
	}

	if instance.Farm == nil {
		return fmt.Errorf("farm engine not initialized")
	}
	return instance.Farm.Start()
}

func (m *BotManager) StopFarm(id string) error {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("bot %s not found", id)
	}

	if instance.Farm == nil {
		return nil
	}
	instance.Farm.Stop()
	return nil
}

func (m *BotManager) GetFarmStats(id string) (models.FarmStats, error) {
	m.mu.RLock()
	instance, ok := m.bots[id]
	m.mu.RUnlock()

	if !ok {
		return models.FarmStats{}, fmt.Errorf("bot %s not found", id)
	}

	if instance.Farm == nil {
		return models.FarmStats{}, nil
	}
	return instance.Farm.GetStats(), nil
}

func (m *BotManager) GetPartyBoost() *PartyBoostManager {
	return m.partyBoost
}

func (m *BotManager) GetPartyBoostConfig() models.PartyBoostConfig {
	if m.partyBoost != nil {
		return m.partyBoost.GetConfig()
	}
	return models.DefaultPartyBoostConfig()
}

func (m *BotManager) SavePartyBoostConfig(cfg models.PartyBoostConfig) error {
	if m.partyBoost != nil {
		return m.partyBoost.UpdateConfig(cfg)
	}
	return fmt.Errorf("party boost manager not initialized")
}

func (m *BotManager) DiscoverPartyRoster() []models.PartyRosterEntry {
	if m.partyBoost != nil {
		return m.partyBoost.DiscoverRoster()
	}
	return nil
}

func (m *BotManager) SyncPartyBoost() error {
	if m.partyBoost != nil {
		return m.partyBoost.SyncNow()
	}
	return fmt.Errorf("party boost manager not initialized")
}
