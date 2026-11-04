package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LogContextEntry holds a single recent log message for crash/disconnect context dump.
type LogContextEntry struct {
	Timestamp time.Time
	Message   string
}

// TelemetryTracker tracks packet flow, save telemetry, rolling context, and per-second rates.
type TelemetryTracker struct {
	mu sync.Mutex

	botID       string
	emitLog     func(category, message, level string)
	logsEnabled atomic.Bool

	// Rolling 30-second context ring buffer
	contextEntries []LogContextEntry
	maxEntries     int

	// Per-second and rolling rate counters
	mhitsInSec     int
	dmgInSec       int
	killTimestamps []time.Time
	saveTimestamps []time.Time

	// Lifecycle
	stopCh chan struct{}
}

// NewTelemetryTracker creates a new telemetry instance for a bot.
func NewTelemetryTracker(botID string, emitLog func(category, message, level string)) *TelemetryTracker {
	tt := &TelemetryTracker{
		botID:          botID,
		emitLog:        emitLog,
		maxEntries:     100,
		contextEntries: make([]LogContextEntry, 0, 100),
		killTimestamps: make([]time.Time, 0, 50),
		saveTimestamps: make([]time.Time, 0, 50),
		stopCh:         make(chan struct{}),
	}
	tt.logsEnabled.Store(false)
	return tt
}

// SetLogsEnabled toggles detailed packet unmarshaling and logging.
func (t *TelemetryTracker) SetLogsEnabled(enabled bool) {
	t.logsEnabled.Store(enabled)
}

// Start begins the 1-second telemetry counter loop.
func (t *TelemetryTracker) Start() {
	t.mu.Lock()
	if t.stopCh != nil {
		select {
		case <-t.stopCh:
			t.stopCh = make(chan struct{})
		default:
			// already running
			t.mu.Unlock()
			return
		}
	} else {
		t.stopCh = make(chan struct{})
	}
	stop := t.stopCh
	t.mu.Unlock()

	go t.counterLoop(stop)
}

// Stop terminates the telemetry counter loop.
func (t *TelemetryTracker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopCh != nil {
		select {
		case <-t.stopCh:
		default:
			close(t.stopCh)
		}
	}
}

func (t *TelemetryTracker) counterLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			t.mu.Lock()
			mhitSec := t.mhitsInSec
			dmgSec := t.dmgInSec
			t.mhitsInSec = 0
			t.dmgInSec = 0

			// Prune kills older than 60s
			cutoff60s := now.Add(-60 * time.Second)
			validKills := 0
			for _, kt := range t.killTimestamps {
				if kt.After(cutoff60s) {
					t.killTimestamps[validKills] = kt
					validKills++
				}
			}
			t.killTimestamps = t.killTimestamps[:validKills]
			killsMin := validKills

			// Prune saves older than 60s
			validSaves := 0
			for _, st := range t.saveTimestamps {
				if st.After(cutoff60s) {
					t.saveTimestamps[validSaves] = st
					validSaves++
				}
			}
			t.saveTimestamps = t.saveTimestamps[:validSaves]
			savesMin := validSaves

			// Prune ring buffer older than 30s
			cutoff30s := now.Add(-30 * time.Second)
			validEntries := 0
			for _, e := range t.contextEntries {
				if e.Timestamp.After(cutoff30s) {
					t.contextEntries[validEntries] = e
					validEntries++
				}
			}
			t.contextEntries = t.contextEntries[:validEntries]
			t.mu.Unlock()

			// Emit per-second rate line if active or periodically
			if mhitSec > 0 || killsMin > 0 || savesMin > 0 {
				msg := fmt.Sprintf("[RATE %s] mhit/s=%d dmg/s=%d kills/min=%d saves/min=%d",
					now.Format("15:04:05.000"), mhitSec, dmgSec, killsMin, savesMin)
				t.appendContext(now, msg)
				if t.logsEnabled.Load() {
					if t.emitLog != nil {
						t.emitLog("stats", msg, "info")
					}
					log.Println(msg)
				}
			}
		}
	}
}

// RecordKill records a monster defeat timestamp for kills/min calculation.
func (t *TelemetryTracker) RecordKill() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.killTimestamps = append(t.killTimestamps, time.Now())
}

var tokenRegex = regexp.MustCompile(`"(?:token|auth|password)"\s*:\s*"[^"]+"`)

// RecordSave logs an /api/save call and records save rate.
func (t *TelemetryTracker) RecordSave(status int, latency time.Duration, reqSize int, reqHead string, respBody string, err error) {
	now := time.Now()
	t.mu.Lock()
	t.saveTimestamps = append(t.saveTimestamps, now)
	t.mu.Unlock()

	safeHead := tokenRegex.ReplaceAllString(reqHead, `"[redacted]"`)
	var msg string
	if err != nil {
		msg = fmt.Sprintf("[%s] HTTP /api/save -> ERROR=%v latency=%dms size=%d req=%s",
			now.Format("15:04:05.000"), err, latency.Milliseconds(), reqSize, safeHead)
	} else {
		// Truncate response body if overly large (>500 chars)
		trimmedBody := respBody
		if len(trimmedBody) > 500 {
			trimmedBody = trimmedBody[:500] + "..."
		}
		msg = fmt.Sprintf("[%s] HTTP /api/save -> status=%d latency=%dms size=%d req=%s resp=%s",
			now.Format("15:04:05.000"), status, latency.Milliseconds(), reqSize, safeHead, trimmedBody)
	}

	t.appendContext(now, msg)
	if t.logsEnabled.Load() {
		if t.emitLog != nil {
			t.emitLog("net", msg, "info")
		}
		log.Println(msg)
	}
}

// RecordWS parses and logs all WS incoming and outgoing messages with millisecond timestamps and key fields.
func (t *TelemetryTracker) RecordWS(direction string, payload []byte) {
	now := time.Now()
	nowStr := now.Format("15:04:05.000")

	if direction == "CLOSE" {
		msg := fmt.Sprintf("[%s] WS DISCONNECTED: %s", nowStr, string(payload))
		t.appendContext(now, msg)
		if t.emitLog != nil && t.logsEnabled.Load() {
			t.emitLog("net", msg, "warn")
		}
		log.Println(msg)
		t.DumpContext("WebSocket Connection Closed")
		return
	}

	// Fast path when logging is disabled: bypass JSON parsing and string formatting
	if !t.logsEnabled.Load() {
		if direction == "IN" && bytes.Contains(payload, []byte(`"error"`)) {
			formatted := t.formatWSPacket(direction, payload)
			fullMsg := fmt.Sprintf("[%s] %s", nowStr, formatted)
			t.appendContext(now, fullMsg)
			if strings.Contains(formatted, "WS IN [error]") {
				t.DumpContext("WS Server Error Packet Received: " + formatted)
			}
		}
		return
	}

	formatted := t.formatWSPacket(direction, payload)
	fullMsg := fmt.Sprintf("[%s] %s", nowStr, formatted)

	t.appendContext(now, fullMsg)
	if t.emitLog != nil {
		t.emitLog("net", fullMsg, "info")
	}

	// If WS IN error packet received, automatically dump 30-second context
	if direction == "IN" && strings.Contains(formatted, "WS IN [error]") {
		t.DumpContext("WS Server Error Packet Received: " + formatted)
	}
}

func (t *TelemetryTracker) formatWSPacket(direction string, payload []byte) string {
	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		// Non-object payload, check if array (e.g., mob sync array or raw)
		var arr []interface{}
		if errArr := json.Unmarshal(payload, &arr); errArr == nil {
			return fmt.Sprintf("WS %s [array] items=%d: %s", direction, len(arr), truncatePayload(payload, 150))
		}
		return fmt.Sprintf("WS %s [raw] %s", direction, truncatePayload(payload, 120))
	}

	pktType, _ := raw["t"].(string)

	// Format based on packet type
	switch pktType {
	case "mhit":
		id := toInt(raw["id"])
		d := toInt(raw["d"])
		c := toInt(raw["c"])
		b := toInt(raw["b"])
		kb := raw["kb"]

		if direction == "OUT" {
			t.mu.Lock()
			t.mhitsInSec++
			t.dmgInSec += d
			t.mu.Unlock()
		}

		if kb != nil {
			return fmt.Sprintf("WS %s [mhit] id=%d d=%d c=%d b=%d kb=%v", direction, id, d, c, b, kb)
		}
		return fmt.Sprintf("WS %s [mhit] id=%d d=%d c=%d b=%d", direction, id, d, c, b)

	case "st":
		p, _ := raw["p"].([]interface{})
		s, _ := raw["s"].(float64)
		sp, _ := raw["sp"].(float64)
		return fmt.Sprintf("WS %s [st] p=%v yaw=%.2f speed=%.2f", direction, p, s, sp)

	case "hello":
		return fmt.Sprintf("WS %s [hello] token=[redacted] id=%v", direction, raw["id"])

	case "welcome":
		return fmt.Sprintf("WS %s [welcome] my_id=%v online=%v", direction, raw["id"], raw["online"])

	case "error":
		return fmt.Sprintf("WS %s [error] text=%q", direction, raw["text"])

	case "count":
		return fmt.Sprintf("WS %s [count] n=%v", direction, raw["n"])

	case "goal":
		return fmt.Sprintf("WS %s [goal] done=%v bonus=%v", direction, raw["done"], raw["bonus_exp"])

	case "ann":
		return fmt.Sprintf("WS %s [ann] text=%q", direction, raw["text"])

	case "snap":
		return fmt.Sprintf("WS %s [snap] count=%v", direction, raw["count"])

	case "loot":
		return fmt.Sprintf("WS %s [loot] %s", direction, truncatePayload(payload, 180))

	case "d":
		return fmt.Sprintf("WS %s [d] target=%v killer=%v shares=%v", direction, raw["id"], raw["k"], raw["s"])

	default:
		// Check for mob sync packet format where key "s" has mob states
		if mobs, ok := raw["s"].([]interface{}); ok && len(mobs) > 0 {
			return fmt.Sprintf("WS %s [mob_sync] mobs=%d %s", direction, len(mobs), truncatePayload(payload, 150))
		}
		if pktType != "" {
			return fmt.Sprintf("WS %s [%s] %s", direction, pktType, truncatePayload(payload, 150))
		}
		return fmt.Sprintf("WS %s %s", direction, truncatePayload(payload, 150))
	}
}

func truncatePayload(payload []byte, maxLen int) string {
	s := strings.TrimSpace(string(payload))
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func (t *TelemetryTracker) appendContext(ts time.Time, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.contextEntries = append(t.contextEntries, LogContextEntry{
		Timestamp: ts,
		Message:   msg,
	})
	if len(t.contextEntries) > t.maxEntries {
		t.contextEntries = t.contextEntries[len(t.contextEntries)-t.maxEntries:]
	}
}

// RecordWSStall logs a detected WebSocket read-loop stall (>300 ms).
func (t *TelemetryTracker) RecordWSStall(gap time.Duration) {
	t.appendContext(time.Now(), fmt.Sprintf("[WS STALL] Read loop delayed by %v (blocked/slow processing)", gap))
}

// RecordLootToD records the duration from loot packet arrival to mob death ("d") event.
func (t *TelemetryTracker) RecordLootToD(latency time.Duration) {
	t.appendContext(time.Now(), fmt.Sprintf("[LOOT-TO-D] Latency: %v", latency))
}

// DumpContext prints the recent 30 seconds of telemetry logs.
func (t *TelemetryTracker) DumpContext(triggerReason string) string {
	t.mu.Lock()
	now := time.Now()
	cutoff := now.Add(-30 * time.Second)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n==================== 30s TELEMETRY CONTEXT DUMP [%s] ====================\n", triggerReason))
	sb.WriteString(fmt.Sprintf("Trigger Time: %s | Bot ID: %s\n", now.Format("15:04:05.000"), t.botID))

	count := 0
	for _, e := range t.contextEntries {
		if e.Timestamp.After(cutoff) {
			sb.WriteString(e.Message)
			sb.WriteString("\n")
			count++
		}
	}
	sb.WriteString(fmt.Sprintf("==================== END CONTEXT DUMP (%d entries) ====================\n", count))
	dumpStr := sb.String()
	t.mu.Unlock()

	log.Print(dumpStr)
	if t.emitLog != nil {
		t.emitLog("system", fmt.Sprintf("RECENT 30s TELEMETRY: Triggered by '%s' (%d events logged)", triggerReason, count), "error")
	}
	return dumpStr
}
