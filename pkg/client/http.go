package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"madrigal-multibot/pkg/models"
)

var defaultSharedTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	MaxIdleConns:          1000,
	MaxIdleConnsPerHost:   200,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

var defaultSharedHTTPClient = &http.Client{
	Timeout:   10 * time.Second,
	Transport: defaultSharedTransport,
}

type SaveTelemetryFunc func(status int, latency time.Duration, reqSize int, reqHead string, respBody string, err error)

type GameHttpClient struct {
	BaseURL         string
	HTTPClient      *http.Client
	OnSaveTelemetry SaveTelemetryFunc
}

func NewGameHttpClient() *GameHttpClient {
	return NewGameHttpClientWithHTTPClient(defaultSharedHTTPClient)
}

func NewGameHttpClientWithHTTPClient(httpClient *http.Client) *GameHttpClient {
	if httpClient == nil {
		httpClient = defaultSharedHTTPClient
	}
	return &GameHttpClient{
		BaseURL:    "https://echoesofmadrigal.com",
		HTTPClient: httpClient,
	}
}

func (c *GameHttpClient) GetChannels() ([]models.ChannelInfo, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/api/hello")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch channels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}

	var res models.ServerHelloResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("invalid hello response: %w", err)
	}
	return res.Channels, nil
}

func (c *GameHttpClient) Login(username, password string) (*models.AuthResponse, error) {
	return c.LoginWithContext(context.Background(), username, password)
}

func (c *GameHttpClient) LoginWithContext(ctx context.Context, username, password string) (*models.AuthResponse, error) {
	payload, err := json.Marshal(models.AuthRequest{
		Name:     username,
		Password: password,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/login", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errObj struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errObj) == nil && errObj.Error != "" {
			return nil, fmt.Errorf("%s", errObj.Error)
		}
		return nil, fmt.Errorf("login failed (%d): %s", resp.StatusCode, string(body))
	}

	var auth models.AuthResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, fmt.Errorf("invalid login response format: %w", err)
	}
	return &auth, nil
}

func (c *GameHttpClient) Logout(token string) error {
	return c.LogoutWithContext(context.Background(), token)
}

func (c *GameHttpClient) LogoutWithContext(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/logout", bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("logout network error: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (c *GameHttpClient) Register(username, password string) (*models.AuthResponse, error) {
	payload, err := json.Marshal(models.AuthRequest{
		Name:     username,
		Password: password,
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/api/register", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("registration network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errObj struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errObj) == nil && errObj.Error != "" {
			return nil, fmt.Errorf("%s", errObj.Error)
		}
		return nil, fmt.Errorf("registration failed (%d): %s", resp.StatusCode, string(body))
	}

	var auth models.AuthResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, fmt.Errorf("invalid registration response format: %w", err)
	}
	return &auth, nil
}

func (c *GameHttpClient) CreateCharacter(token string, slot int, name string) (*models.AccountData, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"slot": slot,
		"name": name,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/api/char/create", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("character creation network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var res struct {
		Error   string             `json:"error"`
		Account models.AccountData `json:"account"`
	}
	if err := json.Unmarshal(body, &res); err == nil && res.Error != "" {
		return nil, fmt.Errorf("%s", res.Error)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("character creation failed (%d): %s", resp.StatusCode, string(body))
	}

	return &res.Account, nil
}

type SaveResult struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
	Resync     int    `json:"resync,omitempty"`
	Success    bool   `json:"success"`
	RawBody    string `json:"raw_body,omitempty"`
}

func (c *GameHttpClient) SaveFiles(token string, files map[string]string) (*SaveResult, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"files": files,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/save", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	t0 := time.Now()
	resp, err := c.HTTPClient.Do(req)
	latency := time.Since(t0)

	var respBody []byte
	var statusCode int
	if resp != nil {
		statusCode = resp.StatusCode
		respBody, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	if c.OnSaveTelemetry != nil {
		head := string(payload)
		if len(head) > 200 {
			head = head[:200] + "..."
		}
		c.OnSaveTelemetry(statusCode, latency, len(payload), head, string(respBody), err)
	}

	result := &SaveResult{
		StatusCode: statusCode,
		RawBody:    string(respBody),
		Success:    statusCode == http.StatusOK,
	}

	if len(respBody) > 0 {
		var errData struct {
			Error  string `json:"error"`
			Resync int    `json:"resync"`
		}
		if jsonErr := json.Unmarshal(respBody, &errData); jsonErr == nil {
			result.Error = errData.Error
			result.Resync = errData.Resync
		}
	}

	if err != nil {
		return result, fmt.Errorf("save network error: %w", err)
	}

	if statusCode != http.StatusOK {
		return result, fmt.Errorf("save failed (%d): %s", statusCode, string(respBody))
	}
	return result, nil
}

func (c *GameHttpClient) ResumeSession(token string) (*models.AuthResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/session", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resume session network error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resume session failed (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var auth models.AuthResponse
	if err := json.Unmarshal(bodyBytes, &auth); err != nil {
		return nil, fmt.Errorf("unmarshal resume response error: %w", err)
	}
	return &auth, nil
}

type WhoPlayer struct {
	Char string `json:"char"`
	Lv   int    `json:"lv"`
	Job  string `json:"job"`
	Ch   int    `json:"ch"`
}

type WhoResponse struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	List  []WhoPlayer `json:"list"`
	N     int         `json:"n"`
}

func (c *GameHttpClient) GetWho(token string) (*WhoResponse, error) {
	req, err := http.NewRequest("POST", c.BaseURL+"/api/who", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("who network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("who request failed (%d): %s", resp.StatusCode, string(body))
	}

	var res WhoResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("invalid who response: %w", err)
	}
	return &res, nil
}

func (c *GameHttpClient) RedeemCode(token string, code string, charSlot int) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"code": code,
	}
	if charSlot >= 0 {
		reqBody["char"] = charSlot
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/api/redeem", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("redeem network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read redeem response: %w", err)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("invalid redeem response (%d): %s", resp.StatusCode, string(body))
	}

	if errMsg, ok := res["error"].(string); ok && errMsg != "" {
		return nil, fmt.Errorf("%s", errMsg)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("redeem failed (%d): %s", resp.StatusCode, string(body))
	}

	return res, nil
}

func (c *GameHttpClient) Claim(token string, payload map[string]interface{}) (map[string]interface{}, error) {
	// Ghost Mode: Block rec_put submissions to prevent bot from appearing in server leaderboards
	if k, _ := payload["k"].(string); k == "rec_put" {
		return map[string]interface{}{"ok": true, "ghost_mode": true}, nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/api/claim", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claim network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read claim response: %w", err)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("invalid claim response (%d): %s", resp.StatusCode, string(body))
	}

	if errMsg, ok := res["error"].(string); ok && errMsg != "" {
		return res, fmt.Errorf("%s", errMsg)
	}

	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("claim failed (%d): %s", resp.StatusCode, string(body))
	}

	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	return res, nil
}

func (c *GameHttpClient) GetAccount(token string) (*models.AccountData, error) {
	req, err := http.NewRequest("POST", c.BaseURL+"/api/account", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("account network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read account response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("account request failed (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Error   string              `json:"error"`
		Account *models.AccountData `json:"account"`
		models.AccountData
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("invalid account response: %w", err)
	}

	if res.Error != "" {
		return nil, fmt.Errorf("%s", res.Error)
	}

	if res.Account != nil {
		return res.Account, nil
	}
	return &res.AccountData, nil
}

func (c *GameHttpClient) CheckPin(token string, pin string) (map[string]interface{}, error) {
	data, err := json.Marshal(map[string]string{"pin": pin})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/api/pin/check", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pin check network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res map[string]interface{}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("invalid pin response: %s", string(body))
	}
	if errMsg, ok := res["error"].(string); ok && errMsg != "" {
		return res, fmt.Errorf("%s", errMsg)
	}
	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	return res, nil
}

func (c *GameHttpClient) SetPin(token string, password string, pin string, oldPin string) (map[string]interface{}, error) {
	payload := map[string]string{
		"pin": pin,
	}
	if password != "" {
		payload["password"] = password
	}
	if oldPin != "" {
		payload["old_pin"] = oldPin
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/api/pin/set", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pin set network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res map[string]interface{}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("invalid pin response: %s", string(body))
	}
	if errMsg, ok := res["error"].(string); ok && errMsg != "" {
		return res, fmt.Errorf("%s", errMsg)
	}
	if res == nil {
		res = make(map[string]interface{})
	}
	res["ok"] = true
	return res, nil
}
