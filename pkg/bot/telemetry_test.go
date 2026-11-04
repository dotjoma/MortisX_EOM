package bot

import (
	"strings"
	"testing"
	"time"
)

func TestTelemetryTracker(t *testing.T) {
	var loggedMessages []string
	emitLog := func(cat, msg, level string) {
		loggedMessages = append(loggedMessages, msg)
	}

	tracker := NewTelemetryTracker("bot_test", emitLog)
	tracker.SetLogsEnabled(true)

	// Test WS message recording
	tracker.RecordWS("OUT", []byte(`{"t":"mhit","id":1001,"d":250,"c":1}`))
	tracker.RecordWS("IN", []byte(`{"t":"count","n":42}`))

	if len(tracker.contextEntries) != 2 {
		t.Fatalf("expected 2 context entries, got %d", len(tracker.contextEntries))
	}
	if !strings.Contains(tracker.contextEntries[0].Message, "WS OUT [mhit] id=1001 d=250 c=1") {
		t.Errorf("unexpected mhit format: %s", tracker.contextEntries[0].Message)
	}

	// Test Save recording
	tracker.RecordSave(200, 45*time.Millisecond, 120, `{"files":{"char.json":"{...}"}}`, `{"ok":true}`, nil)
	if len(tracker.contextEntries) != 3 {
		t.Fatalf("expected 3 context entries, got %d", len(tracker.contextEntries))
	}

	// Test Kill recording
	tracker.RecordKill()
	if len(tracker.killTimestamps) != 1 {
		t.Fatalf("expected 1 kill timestamp, got %d", len(tracker.killTimestamps))
	}

	// Test context dump
	dump := tracker.DumpContext("Test Trigger")
	if !strings.Contains(dump, "30s TELEMETRY CONTEXT DUMP") {
		t.Errorf("dump missing header: %s", dump)
	}
	if !strings.Contains(dump, "WS OUT [mhit]") {
		t.Errorf("dump missing mhit entry: %s", dump)
	}

	// Test start and stop
	tracker.Start()
	time.Sleep(100 * time.Millisecond)
	tracker.Stop()
}
