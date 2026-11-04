package multi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Scope defines whether a control applies to all selected bots or only the focused bot.
type Scope int

const (
	AllSelected Scope = iota
	FocusedOnly
)

func (s Scope) String() string {
	switch s {
	case AllSelected:
		return "AllSelected"
	case FocusedOnly:
		return "FocusedOnly"
	default:
		return "Unknown"
	}
}

// Mode defines execution mode: Parallel (worker pool) or Sequential.
type Mode int

const (
	Parallel Mode = iota
	Sequential
)

func (m Mode) String() string {
	switch m {
	case Parallel:
		return "Parallel"
	case Sequential:
		return "Sequential"
	default:
		return "Unknown"
	}
}

// Risk defines confirmation requirements.
type Risk int

const (
	Low Risk = iota
	Medium
	High
)

func (r Risk) String() string {
	switch r {
	case Low:
		return "low"
	case Medium:
		return "medium"
	case High:
		return "high"
	default:
		return "low"
	}
}

func ParseRisk(s string) Risk {
	switch s {
	case "high":
		return High
	case "medium":
		return Medium
	default:
		return Low
	}
}

// PayloadSource defines where a command's payload is sourced.
type PayloadSource int

const (
	PayloadStatic PayloadSource = iota
	PayloadPerBot
)

// Status represents the per-bot outcome.
type Status int

const (
	StatusOK Status = iota
	StatusSkipped
	StatusFailed
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusSkipped:
		return "Skipped"
	case StatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// Target encapsulates the selected bot IDs and the currently focused bot ID.
type Target struct {
	IDs       []string `json:"ids"`
	FocusedID string   `json:"focusedId"`
}

// Opts passes execution options to ApplySetting, ApplySettings, and RunCommand.
type Opts struct {
	Confirm bool `json:"confirm"` // true => bypass confirmation gate for Risk=High
}

// BotRef references another bot by ID and/or character/profile name.
type BotRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Bot represents the minimal interface required by internal/multi for a bot instance.
type Bot interface {
	ID() string
	Name() string
	Online() bool
	Raw() any // Underlying instance; modules type-assert in their own packages
}

// BotProvider abstracts bot lookup and listing for dependency injection.
type BotProvider interface {
	Get(id string) (Bot, bool)
	ByName(name string) (Bot, bool)
	All() []Bot
}

// Persister abstracts bot persistence.
type Persister interface {
	Save(b Bot) error
	Flush() error
}

// Clock provides time abstraction for test deterministic delays and timeouts.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// RealClock implements Clock using the standard time package.
type RealClock struct{}

func (RealClock) Now() time.Time                         { return time.Now() }
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Deps groups the dependencies for the multi Dispatcher.
type Deps struct {
	Bots      BotProvider
	Emitter   Emitter
	Persister Persister
	Workers   int
	Clock     Clock
}

// ConfirmRequest is returned as a value when an action requires explicit user confirmation.
type ConfirmRequest struct {
	Entry string   `json:"entry"`           // setting or command key
	Bots  []BotRef `json:"bots"`            // affected bots (used for preview dialog)
	Items []string `json:"items,omitempty"` // affected items/refine details if applicable
}

// BotResult represents the outcome for an individual bot.
type BotResult struct {
	BotID  string `json:"botId"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
	Data   any    `json:"data,omitempty"`
}

// Summary aggregates the outcome of a multi-bot dispatch.
type Summary struct {
	Total   int         `json:"total"`
	OK      int         `json:"ok"`
	Skipped int         `json:"skipped"`
	Failed  int         `json:"failed"`
	Results []BotResult `json:"results"`
}

// Result is the primary return value for multi-bot operations.
// Structured failures and confirmation requests are returned as VALUES, not Go errors.
type Result struct {
	Summary Summary         `json:"summary"`
	Confirm *ConfirmRequest `json:"confirm,omitempty"`
	Error   string          `json:"error,omitempty"` // Stale selection, validation errors, invalid target
}

// MixedValue represents read settings across selected bots.
type MixedValue struct {
	Mixed  bool           `json:"mixed"`
	Value  any            `json:"value,omitempty"`  // Focused bot value (unmasked if not mixed)
	PerBot map[string]any `json:"perBot,omitempty"` // Per-bot values; sensitive values masked as "***"
}

// SettingDef defines a configurable setting.
type SettingDef struct {
	Key         string
	Scope       Scope
	Validate    func(b Bot, v any) error
	ValidateRef func(sender Bot, ref Bot, v any) error
	RefsBots    func(v any) []BotRef
	SerializeBy func(b Bot) string
	Get         func(b Bot) (any, error)
	Set         func(b Bot, v any) error
	MutateInPlace func(target any, v any) error
	Persist     bool
	Risk        Risk
	RiskFor     func(b Bot, v any) Risk // Dynamic risk evaluated per bot before execution
	Sensitive   bool
	Kind        string
	ConfigSpec  *ConfigSpec // Non-nil if generated by a ConfigSpec
}

// CommandDef defines an executable command.
type CommandDef struct {
	Key           string
	Scope         Scope
	AllowOffline  bool // true => command may execute when bot is offline (e.g. connect)
	CanRun        func(b Bot) error
	Validate      func(b Bot, payload json.RawMessage) error
	ValidateRef   func(sender Bot, ref Bot, payload json.RawMessage) error
	RefsBots      func(payload json.RawMessage) []BotRef
	LockRefs      bool // true => acquire activity lock on referenced bots (trade: true, mail: false)
	SerializeBy   func(b Bot) string
	Dedupe        func(b Bot, payload json.RawMessage) string
	Execute       func(ctx context.Context, b Bot, payload json.RawMessage) (any, error)
	Mode          Mode
	Timeout       time.Duration
	Stagger       time.Duration
	Risk          Risk
	RiskFor       func(b Bot, payload any) Risk // Dynamic risk evaluated per bot before execution
	PayloadSource PayloadSource
	Exclusive     string // Activity lock name (e.g. "farm", "transfer")
	Kind          string
}

// CollectionDef defines operations on collection fields (e.g., add/remove/update buyList).
type CollectionDef struct {
	Key        string
	Ops        []string
	ValidateOp func(b Bot, op string, payload json.RawMessage) error
	Apply      func(b Bot, op string, payload json.RawMessage) error
	Risk       Risk
	Kind       string
}

// ReaderDef defines read-only computed or aggregated data.
type ReaderDef struct {
	Key       string
	Scope     Scope
	Read      func(b Bot) (any, error)
	Sensitive bool
	Kind      string
}

// FieldSchema provides UI metadata for auto-discovery and forms.
type FieldSchema struct {
	Key        string   `json:"key"`
	Kind       string   `json:"kind"` // "setting" | "command" | "collection" | "reader"
	Type       string   `json:"type"` // "bool" | "string" | "number" | "enum"
	Default    any      `json:"default,omitempty"`
	Options    []string `json:"options,omitempty"`
	Min        *float64 `json:"min,omitempty"`
	Max        *float64 `json:"max,omitempty"`
	Risk       string   `json:"risk"`
	Scope      string   `json:"scope"`
	Sensitive  bool     `json:"sensitive"`
	Label      string   `json:"label"`
}

// ModuleInfo describes a module and its registered fields.
type ModuleInfo struct {
	Tab    string        `json:"tab"`
	ID     string        `json:"id"`
	Title  string        `json:"title"`
	Order  int           `json:"order"`
	Icon   string        `json:"icon"`
	Fields []FieldSchema `json:"fields"`
}

// Schema aggregates metadata for all modules in the application.
type Schema struct {
	Modules []ModuleInfo `json:"modules"`
}

func FormatReason(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
