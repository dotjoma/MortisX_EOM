package console

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"madrigal-multibot/internal/multi"
)

// ChatSender abstracts chat transmission on a bot instance.
type ChatSender interface {
	SendChat(text string) error
}

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Console" }
func (m *Module) ID() string    { return "chat" }
func (m *Module) Title() string { return "Chat & Console" }
func (m *Module) Order() int    { return 6 }
func (m *Module) Icon() string  { return "chat" }

type SendChatPayload struct {
	Message string `json:"message"`
}

func (m *Module) Register(r multi.Registrar) {
	// 1. Send Chat Command
	r.Command(multi.CommandDef{
		Key:     "console.chat.send",
		Scope:   multi.AllSelected,
		Mode:    multi.Parallel,
		Stagger: 100 * time.Millisecond,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p SendChatPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if strings.TrimSpace(p.Message) == "" {
				return errors.New("chat message cannot be empty")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p SendChatPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(ChatSender)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return nil, inst.SendChat(p.Message)
		},
	})
}
