package om

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type mailClient struct {
	mu      sync.Mutex
	token   string
	expires time.Time
	account string
}

var mail = &mailClient{}

func (m *mailClient) login() error {
	if Cfg.SkyMailEmail == "" || Cfg.SkyMailPassword == "" {
		return errors.New("邮件服务未配置：请设置 OM_SKYMAIL_EMAIL / OM_SKYMAIL_PASSWORD")
	}
	body, _ := json.Marshal(map[string]string{"email": Cfg.SkyMailEmail, "password": Cfg.SkyMailPassword})
	req, _ := http.NewRequest(http.MethodPost, Cfg.SkyMailBase+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("skymail login: %s %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
		Token string `json:"token"`
		Msg   string `json:"message"`
	}
	_ = json.Unmarshal(raw, &out)
	tok := out.Data.Token
	if tok == "" {
		tok = out.Token
	}
	if tok == "" {
		return fmt.Errorf("skymail login: no token in response: %s", strings.TrimSpace(string(raw)))
	}
	m.token = tok
	m.expires = time.Now().Add(6 * time.Hour)
	return nil
}

func (m *mailClient) authToken() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token != "" && time.Now().Before(m.expires) {
		return m.token, nil
	}
	if err := m.login(); err != nil {
		return "", err
	}
	return m.token, nil
}

func (m *mailClient) resolveAccount(token string) (string, error) {
	if Cfg.SkyMailAccount != "" {
		return Cfg.SkyMailAccount, nil
	}
	req, _ := http.NewRequest(http.MethodGet, Cfg.SkyMailBase+"/api/account/list", nil)
	req.Header.Set("Authorization", token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	var arr []struct {
		AccountID any    `json:"accountId"`
		Email     string `json:"email"`
	}
	if err := json.Unmarshal(out.Data, &arr); err != nil {
		var obj struct {
			List []struct {
				AccountID any    `json:"accountId"`
				Email     string `json:"email"`
			} `json:"list"`
		}
		if err2 := json.Unmarshal(out.Data, &obj); err2 != nil {
			return "", fmt.Errorf("skymail accounts: %s", strings.TrimSpace(string(raw)))
		}
		arr = obj.List
	}
	if len(arr) == 0 {
		return "", errors.New("skymail: no sender account")
	}
	if v, ok := arr[0].AccountID.(float64); ok {
		return fmt.Sprintf("%.0f", v), nil
	}
	if v, ok := arr[0].AccountID.(string); ok {
		return v, nil
	}
	return "", errors.New("skymail: accountId missing")
}

func SendMail(to, subject, html, text string) error {
	token, err := mail.authToken()
	if err != nil {
		return err
	}
	account, err := mail.resolveAccount(token)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"accountId":    account,
		"receiveEmail": []string{to},
		"subject":      subject,
		"content":      html,
		"text":         text,
		"name":         "ourmemory",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, Cfg.SkyMailBase+"/api/email/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("skymail send: %s %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	}
	if json.Unmarshal(raw, &out) == nil && out.Code != 0 && out.Code != 200 {
		return fmt.Errorf("skymail send: %s", out.Msg)
	}
	return nil
}
