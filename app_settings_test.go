package main

import (
	"encoding/json"
	"testing"
)

func TestDefaultAppSettings_DisabledLogsAndConsole(t *testing.T) {
	defaults := defaultAppSettings()
	if !defaults.DiscordRichPresence {
		t.Errorf("expected DiscordRichPresence true, got %v", defaults.DiscordRichPresence)
	}
	if defaults.EnableLogs {
		t.Errorf("expected EnableLogs false by default for 50-100 bots optimization, got %v", defaults.EnableLogs)
	}
	if defaults.EnableConsole {
		t.Errorf("expected EnableConsole false by default for 50-100 bots optimization, got %v", defaults.EnableConsole)
	}
}

func TestLegacyAppSettings_UnmarshalPreservesDisabledDefaults(t *testing.T) {
	legacyJSON := []byte(`{"discordRichPresence": true}`)
	settings := defaultAppSettings()
	if err := json.Unmarshal(legacyJSON, &settings); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !settings.DiscordRichPresence {
		t.Errorf("expected DiscordRichPresence true, got %v", settings.DiscordRichPresence)
	}
	if settings.EnableLogs {
		t.Errorf("expected EnableLogs false for legacy JSON, got %v", settings.EnableLogs)
	}
	if settings.EnableConsole {
		t.Errorf("expected EnableConsole false for legacy JSON, got %v", settings.EnableConsole)
	}
}

func TestApp_SettingsAtomicToggles(t *testing.T) {
	_ = saveAppSettings(defaultAppSettings())
	defer func() { _ = saveAppSettings(defaultAppSettings()) }()
	app := NewApp()

	// Initial defaults
	if app.enableLogs.Load() {
		t.Errorf("expected app.enableLogs false initially, got true")
	}
	if app.enableConsole.Load() {
		t.Errorf("expected app.enableConsole false initially, got true")
	}

	// Toggle on
	newSettings := AppSettings{
		DiscordRichPresence: true,
		EnableLogs:          true,
		EnableConsole:       true,
	}
	app.settingsMu.Lock()
	app.settings = newSettings
	app.settingsMu.Unlock()
	app.enableLogs.Store(newSettings.EnableLogs)
	app.enableConsole.Store(newSettings.EnableConsole)

	if !app.enableLogs.Load() {
		t.Errorf("expected app.enableLogs true after update")
	}
	if !app.enableConsole.Load() {
		t.Errorf("expected app.enableConsole true after update")
	}
	got := app.GetAppSettings()
	if !got.EnableLogs || !got.EnableConsole {
		t.Errorf("GetAppSettings did not return updated values: %+v", got)
	}
}
