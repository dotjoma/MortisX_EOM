//go:build windows

package vpn

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	ipUnicastIf   = 31
	ipv6UnicastIf = 31
)

func htonl(value uint32) uint32 {
	return value<<24 | (value&0xff00)<<8 | (value>>8)&0xff00 | value>>24
}

func (m *Manager) control(id string) func(network, address string, raw syscall.RawConn) error {
	return func(network, address string, raw syscall.RawConn) error {
		ifIndex := m.ifIndex(id)
		if !m.IsUp(id) || ifIndex == 0 {
			return ErrTunnelDown
		}
		var socketErr error
		if err := raw.Control(func(fd uintptr) {
			handle := windows.Handle(fd)
			if strings.HasSuffix(network, "6") {
				socketErr = windows.SetsockoptInt(handle, windows.IPPROTO_IPV6, ipv6UnicastIf, ifIndex)
			} else {
				socketErr = windows.SetsockoptInt(handle, windows.IPPROTO_IP, ipUnicastIf, int(htonl(uint32(ifIndex))))
			}
		}); err != nil {
			return err
		}
		return socketErr
	}
}

func (m *Manager) Dialer(id string) *net.Dialer {
	dialer := &net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   m.control(id),
	}
	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			resolverDialer := &net.Dialer{Timeout: 5 * time.Second, Control: m.control(id)}
			conn, err := resolverDialer.DialContext(ctx, network, "1.1.1.1:53")
			if err != nil {
				return resolverDialer.DialContext(ctx, network, "8.8.8.8:53")
			}
			return conn, nil
		},
	}
	return dialer
}

func (m *Manager) HTTPClient(id string) *http.Client {
	dialer := m.Dialer(id)
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: 15 * time.Second,
			Proxy:               nil,
		},
	}
}

func RunElevated(exe, args string) error {
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	arg, err := syscall.UTF16PtrFromString(args)
	if err != nil {
		return err
	}
	cwd, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := syscall.UTF16PtrFromString(filepath.Dir(cwd))
	if err != nil {
		return err
	}
	err = windows.ShellExecute(0, verb, file, arg, dir, windows.SW_HIDE)
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return ErrUACCancelled
	}
	return err
}
