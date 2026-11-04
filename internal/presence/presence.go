package presence

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hugolgst/rich-go/client"
)

const (
	activityInterval = 15 * time.Second
	maxRetryDelay    = 30 * time.Second
)

type State struct {
	Tab         string
	RunningBots int
}

type Activity struct {
	Details    string
	State      string
	LargeImage string
}

type discordClient interface {
	Login(appID string) error
	SetActivity(Activity) error
	Logout()
}

type richGoClient struct{}

func (richGoClient) Login(appID string) error { return client.Login(appID) }

func (richGoClient) SetActivity(a Activity) error {
	return client.SetActivity(client.Activity{
		Details:    a.Details,
		State:      a.State,
		LargeImage: a.LargeImage,
	})
}

func (richGoClient) Logout() {
	// rich-go's Logout panics when the IPC socket is already gone.
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("Discord Rich Presence logout failed", "error", r)
		}
	}()
	client.Logout()
}

type command struct {
	state   *State
	enabled *bool
}

var defaultService = newService(richGoClient{})

type service struct {
	client       discordClient
	mu           sync.Mutex
	started      bool
	appID        string
	commands     chan command
	cancel       context.CancelFunc
	done         chan struct{}
	sendInterval time.Duration
}

func newService(c discordClient) *service {
	return &service{client: c, commands: make(chan command, 1), sendInterval: activityInterval}
}

func Start(appID string) error { return defaultService.start(appID) }

func (s *service) start(appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	if appID == "" {
		return fmt.Errorf("Discord application ID is empty")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.appID, s.cancel, s.done, s.started = appID, cancel, make(chan struct{}), true
	go s.run(ctx)
	return nil
}

func Update(state State) { defaultService.update(state) }

func (s *service) update(state State) {
	s.enqueue(command{state: &state})
}

// SetEnabled disables Rich Presence by disconnecting from Discord, which clears
// the activity immediately. The latest state is retained for re-enabling.
func SetEnabled(enabled bool) { defaultService.setEnabled(enabled) }

func (s *service) setEnabled(enabled bool) {
	s.enqueue(command{enabled: &enabled})
}

func (s *service) enqueue(cmd command) {
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		return
	}
	select {
	case s.commands <- cmd:
	default:
		merged := cmd
		select {
		case pending := <-s.commands:
			if cmd.state == nil {
				merged.state = pending.state
			}
			if cmd.enabled == nil {
				merged.enabled = pending.enabled
			}
		default:
		}
		select {
		case s.commands <- merged:
		default:
		}
	}
}

func Stop() { defaultService.stop() }

func (s *service) stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	s.started = false
	s.cancel = nil
	s.mu.Unlock()
	// Shutdown hooks run on the Wails lifecycle path. Let the worker perform the
	// final Discord logout without waiting on an IPC read on that thread.
	cancel()
}

func Format(state State) Activity {
	botText := fmt.Sprintf("Running %d bot(s)", state.RunningBots)
	if state.RunningBots == 0 {
		botText = "No bots running"
	}
	return Activity{
		Details:    fmt.Sprintf("Currently in %s Tab", state.Tab),
		State:      botText,
		LargeImage: "app_logo",
	}
}

func (s *service) run(ctx context.Context) {
	defer close(s.done)
	var latest State
	hasState, enabled, connected := false, true, false
	var lastSent *Activity
	backoff := time.Second
	nextRetry := time.Now()
	nextSend := time.Now()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		if connected {
			s.client.Logout()
		}
	}()

	for {
		// Drain queued updates before acting so bursts collapse to their latest value.
	forDrain:
		for {
			select {
			case cmd := <-s.commands:
				if cmd.state != nil {
					latest, hasState = *cmd.state, true
				}
				if cmd.enabled != nil && enabled != *cmd.enabled {
					enabled = *cmd.enabled
					if !enabled && connected {
						s.client.Logout()
						connected, lastSent = false, nil
					}
					if enabled {
						nextRetry = time.Now()
					}
				}
			default:
				break forDrain
			}
		}

		if !enabled {
			select {
			case <-ctx.Done():
				return
			case cmd := <-s.commands:
				if cmd.state != nil {
					latest, hasState = *cmd.state, true
				}
				if cmd.enabled != nil {
					enabled = *cmd.enabled
					nextRetry = time.Now()
				}
			case <-ticker.C:
			}
			continue
		}

		now := time.Now()
		if !connected && !now.Before(nextRetry) {
			if err := safeLogin(s.client, s.appID); err != nil {
				slog.Debug("Discord Rich Presence connection failed", "error", err)
				nextRetry = now.Add(backoff)
				backoff *= 2
				if backoff > maxRetryDelay {
					backoff = maxRetryDelay
				}
			} else {
				connected, lastSent, backoff = true, nil, time.Second
				nextSend = now
			}
		}
		if connected && hasState && !now.Before(nextSend) {
			activity := Format(latest)
			if lastSent == nil || *lastSent != activity {
				if err := safeSetActivity(s.client, activity); err != nil {
					slog.Debug("Discord Rich Presence update failed", "error", err)
					s.client.Logout()
					connected, lastSent = false, nil
					nextRetry = now.Add(backoff)
					backoff *= 2
					if backoff > maxRetryDelay {
						backoff = maxRetryDelay
					}
				} else {
					copy := activity
					lastSent = &copy
					nextSend = now.Add(s.sendInterval)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case cmd := <-s.commands:
			if cmd.state != nil {
				latest, hasState = *cmd.state, true
			}
			if cmd.enabled != nil {
				enabled = *cmd.enabled
				if enabled {
					nextRetry = time.Now()
				}
			}
		case <-ticker.C:
		}
	}
}

func safeLogin(c discordClient, appID string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("login panic: %v", r)
		}
	}()
	return c.Login(appID)
}

func safeSetActivity(c discordClient, activity Activity) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("activity panic: %v", r)
		}
	}()
	return c.SetActivity(activity)
}
