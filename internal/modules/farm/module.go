package farm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/models"
)

type FarmController interface {
	GetFarmConfig() models.FarmConfig
	SetFarmConfig(cfg models.FarmConfig) error
	MutateFarmConfig(mutate func(c *models.FarmConfig)) models.FarmConfig
	StartFarm() error
	StopFarm() error
	IsFarmRunning() bool
	GetPosition() (float32, float32, float32)
}

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Automation" }
func (m *Module) ID() string    { return "farm" }
func (m *Module) Title() string { return "Farm Engine" }
func (m *Module) Order() int    { return 1 }
func (m *Module) Icon() string  { return "agriculture" }

type WebhookTestPayload struct {
	URL string `json:"url"`
}

func (m *Module) Register(r multi.Registrar) {
	// 1. Config registration for in-memory FarmConfig struct
	r.Config(multi.ConfigSpec{
		Prefix:  "automation.farm.",
		Target:  &models.FarmConfig{},
		Persist: false,
		Get: func(b multi.Bot) any {
			if b == nil {
				return nil
			}
			inst, ok := b.Raw().(FarmController)
			if !ok || inst == nil {
				return nil
			}
			cfg := inst.GetFarmConfig()
			return &cfg
		},
		Update: func(b multi.Bot, mutate func(cfg any)) error {
			if b == nil {
				return errors.New("bot is nil")
			}
			inst, ok := b.Raw().(FarmController)
			if !ok || inst == nil {
				return fmt.Errorf("bot %s instance unavailable", b.ID())
			}
			inst.MutateFarmConfig(func(c *models.FarmConfig) {
				mutate(c)
			})
			return nil
		},
	})

	// 2. Start Farm Command
	r.Command(multi.CommandDef{
		Key:          "automation.farm.start",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Timeout:      10 * time.Second,
		AllowOffline: false,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(FarmController)
			if !ok || inst == nil {
				return nil, errors.New("instance unavailable")
			}
			if len(payload) > 0 && string(payload) != "{}" && string(payload) != "null" {
				var cfg models.FarmConfig
				if err := json.Unmarshal(payload, &cfg); err == nil {
					inst.MutateFarmConfig(func(c *models.FarmConfig) {
						anchorX := c.AnchorX
						anchorY := c.AnchorY
						anchorZ := c.AnchorZ
						*c = cfg
						if cfg.AnchorX == 0 && cfg.AnchorY == 0 && cfg.AnchorZ == 0 && (anchorX != 0 || anchorZ != 0) {
							c.AnchorX = anchorX
							c.AnchorY = anchorY
							c.AnchorZ = anchorZ
						}
					})
				}
			}
			if inst.IsFarmRunning() {
				return map[string]interface{}{"status": "already_running"}, nil
			}
			return nil, inst.StartFarm()
		},
	})

	// 3. Stop Farm Command
	r.Command(multi.CommandDef{
		Key:          "automation.farm.stop",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Timeout:      10 * time.Second,
		AllowOffline: true,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(FarmController)
			if !ok || inst == nil {
				return nil, errors.New("instance unavailable")
			}
			if !inst.IsFarmRunning() {
				return map[string]interface{}{"status": "already_stopped"}, nil
			}
			return nil, inst.StopFarm()
		},
	})

	// 4. Set Current Pos (Anchor) Command
	r.Command(multi.CommandDef{
		Key:           "automation.farm.set_anchor",
		Scope:         multi.AllSelected,
		Mode:          multi.Parallel,
		PayloadSource: multi.PayloadPerBot,
		AllowOffline:  true,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(FarmController)
			if !ok || inst == nil {
				return nil, errors.New("instance unavailable")
			}
			x, y, z := inst.GetPosition()
			cfg := inst.MutateFarmConfig(func(c *models.FarmConfig) {
				c.AnchorX = x
				c.AnchorY = y
				c.AnchorZ = z
			})
			return map[string]interface{}{"anchor_x": cfg.AnchorX, "anchor_y": cfg.AnchorY, "anchor_z": cfg.AnchorZ}, nil
		},
	})

	// 5. Test Discord Webhook Command
	r.Command(multi.CommandDef{
		Key:          "automation.farm.test_webhook",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		AllowOffline: true,
		Dedupe: func(b multi.Bot, payload json.RawMessage) string {
			var p WebhookTestPayload
			_ = json.Unmarshal(payload, &p)
			return strings.TrimSpace(p.URL)
		},
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p WebhookTestPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if strings.TrimSpace(p.URL) == "" {
				return errors.New("webhook URL cannot be empty")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p WebhookTestPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			url := strings.TrimSpace(p.URL)
			if url == "" {
				return nil, errors.New("webhook URL cannot be empty")
			}

			alertPayload := map[string]interface{}{
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
			data, err := json.Marshal(alertPayload)
			if err != nil {
				return nil, err
			}

			req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(data)))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("webhook request failed: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return nil, fmt.Errorf("discord returned status %d", resp.StatusCode)
			}
			return map[string]interface{}{"status": "ok"}, nil
		},
	})
}
