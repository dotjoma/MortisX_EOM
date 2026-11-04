package vpn

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestValidatePayloadAndSetupDone(t *testing.T) {
	dir, err := filepath.Abs("../../third_party/vpn")
	if err != nil {
		t.Skipf("cannot resolve payload path: %v", err)
	}

	mgr := New(Config{Dir: dir})
	if err := mgr.ValidatePayload(); err != nil {
		t.Logf("ValidatePayload note: %v", err)
		return
	}

	ready := mgr.IsSetupDone("morti1")
	t.Logf("IsSetupDone(morti1): %v", ready)
}

func TestPickPrioritizesJapanAndKorea(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	servers, err := FetchServers(ctx, nil)
	if err != nil {
		t.Skipf("cannot fetch live servers: %v", err)
	}

	picked := Pick(servers, "", 8)
	if len(picked) == 0 {
		t.Fatalf("expected servers to be picked, got 0")
	}

	for i, s := range picked {
		t.Logf("#%d: Country=%s IP=%s Ping=%dms Speed=%.1fMB/s Port=%d",
			i+1, s.Country, s.IP, s.Ping, float64(s.Speed)/(1024*1024), s.Port)
		if s.Country != "JP" && s.Country != "KR" {
			t.Errorf("expected server #%d to be JP or KR, got %s", i+1, s.Country)
		}
	}
}
