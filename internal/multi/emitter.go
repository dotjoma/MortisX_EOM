package multi

// Emitter abstracts runtime event emissions to the frontend.
type Emitter interface {
	Emit(eventName string, data any)
}

// SettingChangedPayload is the payload shape for "multi:setting-changed" events.
type SettingChangedPayload struct {
	Keys   []string `json:"keys"`
	BotIDs []string `json:"botIds"`
}

// NoopEmitter is an Emitter implementation that does nothing.
type NoopEmitter struct{}

func (NoopEmitter) Emit(eventName string, data any) {}
