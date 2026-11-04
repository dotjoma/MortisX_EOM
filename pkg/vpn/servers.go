//go:build windows

package vpn

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const serverListURL = "https://www.vpngate.net/api/iphone/"

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

var (
	cacheMu   sync.Mutex
	cacheAt   time.Time
	cacheList []Server
)

func parsePortFromConfig(b64 string) int {
	if b64 == "" {
		return 0
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "remote ") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				if port, err := strconv.Atoi(fields[2]); err == nil && port > 0 && port < 65536 {
					return port
				}
			}
		}
	}
	return 0
}

func FetchServers(ctx context.Context, client *http.Client) ([]Server, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if len(cacheList) > 0 && time.Since(cacheAt) < 10*time.Minute {
		return append([]Server(nil), cacheList...), nil
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, serverListURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("VPN Gate server list request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VPN Gate server list returned HTTP %d", response.StatusCode)
	}

	reader := csv.NewReader(io.LimitReader(response.Body, 12<<20))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("could not parse VPN Gate server list: %w", err)
	}

	indices := map[string]int{}
	var servers []Server
	for _, record := range records {
		if len(record) == 0 || strings.HasPrefix(record[0], "*") {
			continue
		}
		if strings.HasPrefix(record[0], "#") {
			for index, heading := range record {
				indices[strings.TrimPrefix(heading, "#")] = index
			}
			continue
		}
		if len(indices) == 0 {
			continue
		}
		get := func(key string) string {
			if index, ok := indices[key]; ok && index < len(record) {
				return record[index]
			}
			return ""
		}
		number := func(key string) int64 {
			value, _ := strconv.ParseInt(get(key), 10, 64)
			return value
		}
		server := Server{
			Host: get("HostName"), IP: get("IP"),
			Port:     parsePortFromConfig(get("OpenVPN_ConfigData_Base64")),
			Score:    number("Score"), Ping: number("Ping"), Speed: number("Speed"),
			Country:  strings.ToUpper(strings.TrimSpace(get("CountryShort"))), LogType: get("LogType"),
			Sessions: number("NumVpnSessions"),
		}
		if server.Host == "" || net.ParseIP(server.IP) == nil || server.Ping <= 0 || server.Speed <= 0 {
			continue
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("VPN Gate returned no usable servers")
	}
	cacheList, cacheAt = servers, time.Now()
	return append([]Server(nil), servers...), nil
}

func isPreferredCountry(country string) bool {
	upper := strings.ToUpper(strings.TrimSpace(country))
	return upper == "JP" || upper == "KR"
}

func Pick(all []Server, country string, limit int) []Server {
	var pool []Server
	if country != "" {
		for _, server := range all {
			if strings.EqualFold(server.Country, country) {
				pool = append(pool, server)
			}
		}
	} else {
		// Priority 1: Japan (JP) and Korea (KR)
		for _, server := range all {
			if isPreferredCountry(server.Country) {
				pool = append(pool, server)
			}
		}
		// If fewer than limit JP/KR servers, add other countries as backup
		if len(pool) < limit {
			for _, server := range all {
				if !isPreferredCountry(server.Country) {
					pool = append(pool, server)
				}
			}
		}
	}

	sort.Slice(pool, func(i, j int) bool {
		// 1. Japan / Korea always first
		prefI := isPreferredCountry(pool[i].Country)
		prefJ := isPreferredCountry(pool[j].Country)
		if prefI != prefJ {
			return prefI
		}
		// 2. Lowest ping first
		if pool[i].Ping != pool[j].Ping {
			return pool[i].Ping < pool[j].Ping
		}
		// 3. Highest speed
		return pool[i].Speed > pool[j].Speed
	})

	if limit > 0 && len(pool) > limit {
		pool = pool[:limit]
	}
	return pool
}
