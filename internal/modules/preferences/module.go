package preferences

import (
	"errors"
	"fmt"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/bot"
	"madrigal-multibot/pkg/models"
)

// Module implements the multi.Module interface for Main Tab Preferences.
type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Main" }
func (m *Module) ID() string    { return "preferences" }
func (m *Module) Title() string { return "Preferences" }
func (m *Module) Order() int    { return 1 }
func (m *Module) Icon() string  { return "settings" }

func (m *Module) Register(r multi.Registrar) {
	r.Config(multi.ConfigSpec{
		Prefix:  "main.preferences.",
		Target:  &models.BotPreferences{},
		Persist: true,
		Get: func(b multi.Bot) any {
			if b == nil {
				return nil
			}
			if inst, ok := b.Raw().(*bot.BotInstance); ok && inst != nil {
				prefs := inst.GetPreferences()
				return &prefs
			}
			type prefGetter interface {
				GetPreferences() models.BotPreferences
			}
			if pg, ok := b.Raw().(prefGetter); ok {
				prefs := pg.GetPreferences()
				return &prefs
			}
			return nil
		},
		Update: func(b multi.Bot, mutate func(cfg any)) error {
			if b == nil {
				return errors.New("bot is nil")
			}
			if inst, ok := b.Raw().(*bot.BotInstance); ok && inst != nil {
				inst.MutatePreferences(func(p *models.BotPreferences) {
					mutate(p)
				})
				return nil
			}
			type prefMutator interface {
				MutatePreferences(func(*models.BotPreferences)) models.BotPreferences
			}
			if pm, ok := b.Raw().(prefMutator); ok {
				pm.MutatePreferences(func(p *models.BotPreferences) {
					mutate(p)
				})
				return nil
			}
			return fmt.Errorf("bot %s instance unavailable", b.ID())
		},
		Validate: func(old, new any) error {
			p, ok := new.(*models.BotPreferences)
			if !ok || p == nil {
				return errors.New("invalid preferences payload")
			}
			return models.ValidateJobProgressionConfig(p.JobConfig)
		},
	})
}
