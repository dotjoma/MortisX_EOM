//go:build !windows

package vpn

import (
	"context"
	"errors"
	"net"
	"net/http"
	"syscall"
)

var (
	ErrNoServer     = errors.New("no VPN Gate server could be connected")
	ErrNotSetup     = errors.New("SoftEther VPN is only supported on 64-bit Windows")
	ErrTunnelDown   = errors.New("VPN tunnel is down; refusing to use the normal network route")
	ErrUACCancelled = errors.New("administrator approval was declined")
)

type Config struct{ Dir string }

type Server struct {
	Host     string
	IP       string
	Port     int
	Score    int64
	Ping     int64
	Speed    int64
	Country  string
	LogType  string
	Sessions int64
}

type Status struct {
	State   string `json:"state"`
	Server  string `json:"server,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type Manager struct{}

func New(Config) *Manager                                        { return &Manager{} }
func (*Manager) ValidatePayload() error                          { return ErrNotSetup }
func (*Manager) IsSetupDone(...string) bool                      { return false }
func (*Manager) Setup(context.Context, ...string) error          { return ErrNotSetup }
func (*Manager) Status(string) Status                            { return Status{State: "error", Error: ErrNotSetup.Error()} }
func (*Manager) SetStatus(string, Status)                        {}
func (*Manager) Connect(context.Context, string, []Server) error { return ErrNotSetup }
func (*Manager) Disconnect(context.Context, string)              {}
func (*Manager) IsUp(string) bool                                { return false }
func (*Manager) Monitor(context.Context, string, func(context.Context) ([]Server, error), func(string)) {
}
func (*Manager) Dialer(string) *net.Dialer {
	return &net.Dialer{Control: func(string, string, syscall.RawConn) error { return ErrNotSetup }}
}
func (m *Manager) HTTPClient(id string) *http.Client {
	dialer := m.Dialer(id)
	return &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext, Proxy: nil}}
}
func RunElevated(string, string) error                             { return ErrNotSetup }
func FetchServers(context.Context, *http.Client) ([]Server, error) { return nil, ErrNotSetup }
func Pick([]Server, string, int) []Server                          { return nil }
