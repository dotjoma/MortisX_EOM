//go:build windows

// Package vpn manages SoftEther VPN Client tunnels for Morti.
package vpn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNoServer     = errors.New("no VPN Gate server could be connected")
	ErrNotSetup     = errors.New("SoftEther is not set up; administrator approval is required")
	ErrTunnelDown   = errors.New("VPN tunnel is down; refusing to use the normal network route")
	ErrUACCancelled = errors.New("administrator approval was declined")
)

const (
	serviceName        = "MortiVpnClient"
	conflictingService = "SEVPNCLIENT"
	hubName            = "VPNGATE"
	hubUser            = "vpn"
	lowPriority        = "9999"
)

var ports = []int{443, 992, 5555}

type Config struct {
	Dir string
}

type Tunnel struct {
	ID      string
	IfIndex int
	Server  string
	up      bool
}

type Status struct {
	State   string `json:"state"`
	Server  string `json:"server,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type Manager struct {
	cfg     Config
	mu      sync.RWMutex
	tunnels map[string]*Tunnel
	status  map[string]Status
}

func New(cfg Config) *Manager {
	return &Manager{
		cfg:     cfg,
		tunnels: map[string]*Tunnel{},
		status:  map[string]Status{},
	}
}

func (m *Manager) clientPath() string { return filepath.Join(m.cfg.Dir, "vpnclient_x64.exe") }
func (m *Manager) cmdPath() string    { return filepath.Join(m.cfg.Dir, "vpncmd_x64.exe") }

func (m *Manager) ValidatePayload() error {
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("Morti VPN currently supports 64-bit Windows only (architecture: %s)", runtime.GOARCH)
	}
	for _, name := range []string{"vpnclient_x64.exe", "vpncmd_x64.exe", "hamcore.se2", "LICENSE.txt"} {
		if info, err := os.Stat(filepath.Join(m.cfg.Dir, name)); err != nil || info.IsDir() {
			return fmt.Errorf("required SoftEther file is missing: %s", filepath.Join(m.cfg.Dir, name))
		}
	}
	return nil
}

func (m *Manager) vpncmd(ctx context.Context, args ...string) (string, error) {
	full := append([]string{"localhost", "/CLIENT", "/CMD"}, args...)
	cmd := exec.CommandContext(ctx, m.cmdPath(), full...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func hidden(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func serviceExists(name string) bool {
	return hidden("sc.exe", "query", name).Run() == nil
}

func (m *Manager) IsSetupDone(ids ...string) bool {
	if m.ValidatePayload() != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := m.vpncmd(ctx, "VersionGet"); err != nil {
		return false
	}
	for _, id := range ids {
		if _, err := m.adapter(id); err != nil {
			return false
		}
	}
	return true
}

func (m *Manager) Setup(ctx context.Context, ids ...string) error {
	if err := m.ValidatePayload(); err != nil {
		return err
	}
	if _, err := m.vpncmd(ctx, "VersionGet"); err != nil {
		if serviceExists(conflictingService) {
			_ = hidden("sc.exe", "start", conflictingService).Run()
		} else {
			if err := m.ensureService(ctx); err != nil {
				return err
			}
		}
		if err := waitFor(ctx, 20*time.Second, func() bool {
			_, err := m.vpncmd(ctx, "VersionGet")
			return err == nil
		}); err != nil {
			return fmt.Errorf("SoftEther VPN service did not become ready: %w", err)
		}
	}
	for _, id := range ids {
		if err := validateTunnelID(id); err != nil {
			return err
		}
		if _, err := m.adapter(id); err == nil {
			continue
		}
		nicName, err := nicNameForID(id)
		if err != nil {
			return err
		}
		if out, err := m.vpncmd(ctx, "NicCreate", nicName); err != nil {
			msg := strings.TrimSpace(out)
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("could not create SoftEther virtual adapter %q. The adapter driver may be missing from hamcore.se2: %s", nicName, msg)
		}
		if err := waitFor(ctx, 20*time.Second, func() bool {
			_, err := m.findAdapter(id)
			return err == nil
		}); err != nil {
			return fmt.Errorf("SoftEther created no usable Windows network adapter: %w", err)
		}
	}
	if err := m.setAdapterMetric(ids...); err != nil {
		return err
	}
	return nil
}

func (m *Manager) ensureService(ctx context.Context) error {
	clientPath, err := filepath.Abs(m.clientPath())
	if err != nil {
		return err
	}
	if serviceExists(serviceName) {
		out, err := hidden("sc.exe", "qc", serviceName).CombinedOutput()
		if err != nil {
			return fmt.Errorf("could not inspect %s service: %s", serviceName, strings.TrimSpace(string(out)))
		}
		if serviceBinaryPath(string(out)) != normalizedServicePath(clientPath) {
			if out, err := hidden("sc.exe", "config", serviceName, "binPath=", `"`+clientPath+`" /service`).CombinedOutput(); err != nil {
				return fmt.Errorf("could not update %s service path: %s", serviceName, strings.TrimSpace(string(out)))
			}
		}
	} else {
		args := []string{"create", serviceName, "binPath=", `"` + clientPath + `" /service`, "start=", "demand", "DisplayName=", "Morti VPN Client"}
		if out, err := hidden("sc.exe", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("could not create %s service: %s", serviceName, strings.TrimSpace(string(out)))
		}
	}
	if out, err := hidden("sc.exe", "start", serviceName).CombinedOutput(); err != nil && !strings.Contains(strings.ToLower(string(out)), "already running") {
		return fmt.Errorf("could not start %s service: %s", serviceName, strings.TrimSpace(string(out)))
	}
	return ctx.Err()
}

var binPathPattern = regexp.MustCompile(`(?im)^\s*BINARY_PATH_NAME\s*:\s*(.*?)\s*$`)

func serviceBinaryPath(output string) string {
	match := binPathPattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	path := strings.TrimSpace(match[1])
	path = strings.Trim(path, `"`)
	if i := strings.Index(strings.ToLower(path), `" /service`); i >= 0 {
		path = strings.Trim(path[:i], `"`)
	} else if i := strings.Index(strings.ToLower(path), " /service"); i >= 0 {
		path = path[:i]
	}
	return normalizedServicePath(path)
}

func normalizedServicePath(path string) string {
	path = strings.TrimSpace(strings.Trim(path, `"`))
	return strings.ToLower(filepath.Clean(path))
}

var tunnelIDPattern = regexp.MustCompile(`^morti([1-9][0-9]{0,2})$`)

func validateTunnelID(id string) error {
	if !tunnelIDPattern.MatchString(id) {
		return fmt.Errorf("invalid VPN tunnel ID %q", id)
	}
	return nil
}

func nicNameForID(id string) (string, error) {
	if err := validateTunnelID(id); err != nil {
		return "", err
	}
	match := tunnelIDPattern.FindStringSubmatch(id)
	index, err := strconv.Atoi(match[1])
	if err != nil || index < 1 || index > 127 {
		return "", fmt.Errorf("VPN tunnel ID %q must end in a number from 1 to 127", id)
	}
	if index == 1 {
		return "VPN", nil
	}
	return fmt.Sprintf("VPN%d", index), nil
}

func adapterAliasForID(id string) (string, error) {
	nicName, err := nicNameForID(id)
	if err != nil {
		return "", err
	}
	return nicName + " - VPN client", nil
}

func (m *Manager) findAdapter(id string) (*net.Interface, error) {
	alias, err := adapterAliasForID(id)
	if err != nil {
		return nil, err
	}
	if i, err := net.InterfaceByName(alias); err == nil {
		return i, nil
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for i := range interfaces {
		name := strings.ToLower(interfaces[i].Name)
		nicName, _ := nicNameForID(id)
		deviceName := strings.ToLower("VPN Client Adapter - " + nicName)
		if name == strings.ToLower(alias) || name == deviceName {
			return &interfaces[i], nil
		}
	}
	return nil, fmt.Errorf("SoftEther adapter %q was not found", alias)
}

func (m *Manager) adapter(id string) (*net.Interface, error) { return m.findAdapter(id) }

func (m *Manager) setAdapterMetric(ids ...string) error {
	for _, id := range ids {
		ifi, err := m.findAdapter(id)
		if err != nil {
			return err
		}
		for _, family := range []string{"ipv4", "ipv6"} {
			out, err := hidden("netsh", "interface", family, "set", "interface", "name="+ifi.Name, "metric="+lowPriority).CombinedOutput()
			if err != nil {
				return fmt.Errorf("could not set %s adapter metric: %s", family, strings.TrimSpace(string(out)))
			}
		}
	}
	return nil
}

func (m *Manager) setStatus(id string, status Status) {
	m.mu.Lock()
	m.status[id] = status
	if t := m.tunnels[id]; t != nil {
		t.up = status.State == "connected"
		if t.up {
			t.Server = status.Server
		}
	}
	m.mu.Unlock()
}

func (m *Manager) Status(id string) Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.status[id]; ok {
		return s
	}
	return Status{State: "off"}
}

func (m *Manager) SetStatus(id string, status Status) { m.setStatus(id, status) }

func (m *Manager) tunnel(id string) *Tunnel {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tunnels[id]
	if t == nil {
		t = &Tunnel{ID: id}
		m.tunnels[id] = t
	}
	return t
}

func (m *Manager) Connect(ctx context.Context, id string, servers []Server) error {
	if !m.IsSetupDone(id) {
		return ErrNotSetup
	}
	if err := validateTunnelID(id); err != nil {
		return err
	}
	m.setStatus(id, Status{State: "connecting"})
	m.reset(ctx, id)
	for _, server := range servers {
		portsToTry := []int{}
		if server.Port > 0 {
			portsToTry = append(portsToTry, server.Port)
		}
		for _, p := range ports {
			if p != server.Port {
				portsToTry = append(portsToTry, p)
			}
		}

		for _, port := range portsToTry {
			if err := ctx.Err(); err != nil {
				m.setStatus(id, Status{State: "error", Error: err.Error()})
				return err
			}
			address := fmt.Sprintf("%s:%d", server.IP, port)
			m.setStatus(id, Status{State: "connecting", Message: fmt.Sprintf("Connecting to %s (%s)...", server.Country, address)})
			nicName, err := nicNameForID(id)
			if err != nil {
				return err
			}
			if out, err := m.vpncmd(ctx, "AccountCreate", id,
				"/SERVER:"+address, "/HUB:"+hubName, "/USERNAME:"+hubUser, "/NICNAME:"+nicName); err != nil {
				_ = out
				continue
			}
			// Disable routing table adjustments so whole PC internet is NOT routed to the VPN.
			// Only bots with Bypass Logon enabled bind their sockets to this virtual adapter.
			_, _ = m.vpncmd(ctx, "AccountDetailSet", id,
				"/MAXTCP:1", "/INTERVAL:1", "/TTL:0", "/HALF:no", "/BRIDGE:no", "/MONITOR:no", "/NOTRACK:yes", "/NOQOS:no")
			if _, err := m.vpncmd(ctx, "AccountPasswordSet", id, "/PASSWORD:vpn", "/TYPE:standard"); err != nil {
				m.reset(ctx, id)
				continue
			}
			_, _ = m.vpncmd(ctx, "AccountConnect", id)
			if m.waitConnected(ctx, id, 8*time.Second) {
				ifi, err := m.waitIPv4(ctx, id, 10*time.Second)
				if err == nil {
					m.mu.Lock()
					t := m.tunnels[id]
					if t == nil {
						t = &Tunnel{ID: id}
						m.tunnels[id] = t
					}
					t.IfIndex, t.Server, t.up = ifi.Index, address, true
					m.status[id] = Status{State: "connected", Server: server.Country + " / " + server.IP}
					m.mu.Unlock()
					return nil
				}
			}
			m.reset(ctx, id)
		}
	}
	m.setStatus(id, Status{State: "error", Error: ErrNoServer.Error()})
	return ErrNoServer
}

func (m *Manager) reset(ctx context.Context, id string) {
	_, _ = m.vpncmd(ctx, "AccountDisconnect", id)
	_, _ = m.vpncmd(ctx, "AccountDelete", id)
}

func (m *Manager) Disconnect(ctx context.Context, id string) {
	m.setStatus(id, Status{State: "off"})
	m.reset(ctx, id)
}

func (m *Manager) connected(ctx context.Context, id string) bool {
	out, err := m.vpncmd(ctx, "AccountStatusGet", id)
	return err == nil && strings.Contains(strings.ToLower(out), "connection completed")
}

func (m *Manager) waitConnected(ctx context.Context, id string, d time.Duration) bool {
	return waitFor(ctx, d, func() bool { return m.connected(ctx, id) }) == nil
}

func (m *Manager) IsUp(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t := m.tunnels[id]
	return t != nil && t.up
}

func (m *Manager) ifIndex(id string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t := m.tunnels[id]; t != nil {
		return t.IfIndex
	}
	return 0
}

func (m *Manager) Monitor(ctx context.Context, id string, list func(context.Context) ([]Server, error), onEvent func(string)) {
	emit := func(s string) {
		if onEvent != nil {
			onEvent(s)
		}
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if !m.IsUp(id) || !m.connected(ctx, id) {
			previous := m.Status(id)
			state := "connecting"
			message := "Connecting to VPN Gate..."
			if previous.State == "connected" || previous.State == "reconnecting" {
				state = "reconnecting"
				message = "VPN connection dropped; reconnecting..."
			}
			m.setStatus(id, Status{State: state, Message: message})
			emit(state)
			servers, err := list(ctx)
			if err == nil {
				err = m.Connect(ctx, id, servers)
			}
			if err != nil {
				if ctx.Err() == nil {
					m.setStatus(id, Status{State: "error", Error: err.Error()})
					emit("error: " + err.Error())
				}
			} else {
				emit("connected")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func waitFor(ctx context.Context, d time.Duration, ok func() bool) error {
	deadline := time.Now().Add(d)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ok() {
			return nil
		}
		if time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (m *Manager) waitIPv4(ctx context.Context, id string, d time.Duration) (*net.Interface, error) {
	var found *net.Interface
	err := waitFor(ctx, d, func() bool {
		ifi, err := m.findAdapter(id)
		if err != nil {
			return false
		}
		addrs, _ := ifi.Addrs()
		for _, addr := range addrs {
			if ipn, ok := addr.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLinkLocalUnicast() {
				found = ifi
				return true
			}
		}
		return false
	})
	return found, err
}
