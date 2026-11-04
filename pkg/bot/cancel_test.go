package bot

import (
	"fmt"
	"testing"
	"time"

	"madrigal-multibot/pkg/models"
)

func TestConnectingBotCancel(t *testing.T) {
	instance := NewBotInstance(models.BotProfile{
		ID:            "test-cancel-bot",
		Name:          "cancel_test_user",
		Server:        "mock",
		CharacterName: "cancel_char",
		Password:      "testpass",
	}, nil, nil, nil, nil)

	// Simulate connecting state
	instance.mu.Lock()
	instance.Status = "connecting"
	instance.StatusMessage = "Connecting..."
	instance.cancelCh = make(chan struct{})
	instance.sessionToken = "test_token_123"
	instance.mu.Unlock()

	// Calling Stop() while connecting should transition cleanly to disconnected
	err := instance.Stop()
	if err != nil {
		t.Fatalf("Stop() returned unexpected error: %v", err)
	}

	instance.mu.Lock()
	status := instance.Status
	tok := instance.sessionToken
	instance.mu.Unlock()

	if status != "disconnected" {
		t.Fatalf("expected status 'disconnected', got '%s'", status)
	}
	if tok != "" {
		t.Fatalf("expected empty session token after cancel, got '%s'", tok)
	}
}

func TestStopAllGraceful(t *testing.T) {
	mgr := NewBotManager(nil, nil, nil)
	_, _ = mgr.CreateBot(models.BotProfile{
		ID:     "bot-1",
		Name:   "user1",
		Server: "mock",
	}, nil)
	_, _ = mgr.CreateBot(models.BotProfile{
		ID:     "bot-2",
		Name:   "user2",
		Server: "mock",
	}, nil)

	// Calling StopAll should execute cleanly on all bots without deadlock
	done := make(chan struct{})
	go func() {
		mgr.StopAll()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatalf("StopAll() timed out or deadlocked")
	}
}

func TestAccountBannedStatus(t *testing.T) {
	var reportedStatus, reportedMsg string
	instance := NewBotInstance(models.BotProfile{
		ID:            "banned-test-bot",
		Name:          "banned_user",
		Server:        "mock",
		CharacterName: "banned_char",
	}, nil, func(id string, stats models.BotRuntimeStats, status, statusMsg string) {
		reportedStatus = status
		reportedMsg = statusMsg
	}, nil, nil)

	instance.mu.Lock()
	instance.Status = "online"
	instance.mu.Unlock()

	// Simulate receiving blocked error message
	errPkt := []byte(`{"t":"error","text":"This account is blocked."}`)
	instance.handleIncomingPacket(errPkt)

	instance.mu.Lock()
	st := instance.Status
	msg := instance.StatusMessage
	instance.mu.Unlock()

	if st != "account_banned" {
		t.Fatalf("expected status 'account_banned', got '%s'", st)
	}
	if msg != "Account Banned" {
		t.Fatalf("expected status_message 'Account Banned', got '%s'", msg)
	}
	if reportedStatus != "account_banned" {
		t.Fatalf("expected reported status 'account_banned', got '%s'", reportedStatus)
	}
	if reportedMsg != "Account Banned" {
		t.Fatalf("expected reported msg 'Account Banned', got '%s'", reportedMsg)
	}
}

func TestStopFarmingBotNoDeadlock(t *testing.T) {
	instance := NewBotInstance(models.BotProfile{
		ID:            "farming-deadlock-test",
		Name:          "farmer",
		Server:        "mock",
		CharacterName: "farmer_char",
	}, nil, nil, nil, nil)

	if err := instance.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Activate FarmEngine
	instance.Farm.mu.Lock()
	instance.Farm.running = true
	instance.Farm.cancelCh = make(chan struct{})
	instance.Farm.mu.Unlock()

	done := make(chan struct{})
	go func() {
		_ = instance.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success: no deadlock
	case <-time.After(2 * time.Second):
		t.Fatalf("instance.Stop() on farming bot deadlocked!")
	}

	if instance.Farm.IsRunning() {
		t.Fatalf("expected Farm.IsRunning() to be false after Stop()")
	}
}

func TestStopAllWithFarmingBots(t *testing.T) {
	mgr := NewBotManager(nil, nil, nil)
	for i := 0; i < 10; i++ {
		botID := fmt.Sprintf("farm-bot-%d", i)
		_, err := mgr.CreateBot(models.BotProfile{
			ID:     botID,
			Name:   fmt.Sprintf("farmer_%d", i),
			Server: "mock",
		}, nil)
		if err != nil {
			t.Fatalf("CreateBot failed: %v", err)
		}
		_, err = mgr.StartBot(botID)
		if err != nil {
			t.Fatalf("StartBot failed: %v", err)
		}

		mgr.mu.RLock()
		bot := mgr.bots[botID]
		mgr.mu.RUnlock()

		bot.Farm.mu.Lock()
		bot.Farm.running = true
		bot.Farm.cancelCh = make(chan struct{})
		bot.Farm.mu.Unlock()
	}

	done := make(chan struct{})
	go func() {
		mgr.StopAll()
		close(done)
	}()

	select {
	case <-done:
		// Success: all farming bots stopped gracefully
	case <-time.After(3 * time.Second):
		t.Fatalf("mgr.StopAll() with farming bots deadlocked or timed out!")
	}
}

func TestFindBotByNameIndex(t *testing.T) {
	mgr := NewBotManager(nil, nil, nil)
	_, err := mgr.CreateBot(models.BotProfile{
		ID:            "bot-idx-1",
		Name:          "LeaderBot",
		CharacterName: "BladeMaster",
		Server:        "mock",
	}, nil)
	if err != nil {
		t.Fatalf("CreateBot failed: %v", err)
	}

	// Lookup by Name (case-insensitive)
	found := mgr.FindBotByName("leaderbot")
	if found == nil || found.ID != "bot-idx-1" {
		t.Fatalf("expected to find bot by Name 'leaderbot', got %v", found)
	}

	// Lookup by CharacterName (case-insensitive)
	foundChar := mgr.FindBotByName("blademaster")
	if foundChar == nil || foundChar.ID != "bot-idx-1" {
		t.Fatalf("expected to find bot by CharacterName 'blademaster', got %v", foundChar)
	}

	// Remove bot and verify index cleaned up
	_ = mgr.RemoveBot("bot-idx-1")
	if notFound := mgr.FindBotByName("leaderbot"); notFound != nil {
		t.Fatalf("expected nil after RemoveBot, got %v", notFound)
	}
}
