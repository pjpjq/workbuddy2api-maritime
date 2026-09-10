// login.go — WorkBuddy OAuth 登录（设备授权流程，支持国际版与国内版）。
//
// 两个子命令，由 login.sh 顺序驱动：
//
//	login url [global|cn]  → POST /v2/plugin/auth/state?platform=CLI 拿 state+authUrl，
//	                         state 落 /tmp/wb2api-login-state.json，stdout 打印授权 URL
//	login poll             → 读 state，GET /v2/plugin/auth/token?state= 一次，
//	                         成功再 GET /v2/plugin/login/account?state= 拿 uid/nickname，
//	                         stdout 打印完整 token+account JSON
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

const (
	clientUA  = "CLI/2.139.0 CodeBuddy/2.139.0"
	stateFile = "/tmp/wb2api-login-state.json"
)

// commonHeaders 通用请求头
func commonHeaders(req *http.Request, origin string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")
	req.Header.Set("User-Agent", clientUA)
}

type apiEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func doJSON(client *http.Client, method, fullURL, origin string, headers func(*http.Request), body io.Reader) (json.RawMessage, int, error) {
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, 0, err
	}
	commonHeaders(req, origin)
	if headers != nil {
		headers(req)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("http_error: upstream %d", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("http_error: upstream redirect %d", resp.StatusCode)
	}
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parse failed: %w", err)
	}
	if env.Code != 0 {
		return nil, resp.StatusCode, fmt.Errorf("code=%d msg=%s", env.Code, env.Msg)
	}
	return env.Data, resp.StatusCode, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "login: "+format+"\n", args...)
	os.Exit(1)
}

type loginState struct {
	State  string `json:"state"`
	Base   string `json:"base"`
	Origin string `json:"origin"`
	Domain string `json:"domain"`
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: login <url|poll> [global|cn]")
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 30 * time.Second, Jar: jar}

	switch os.Args[1] {
	case "url":
		region := "global"
		if len(os.Args) >= 3 && (os.Args[2] == "cn" || os.Args[2] == "domestic") {
			region = "cn"
		}
		if v := os.Getenv("WB_REGION"); v == "cn" || v == "domestic" {
			region = "cn"
		}

		base := "https://www.workbuddy.ai"
		origin := "https://www.workbuddy.ai"
		domain := "workbuddy.ai"
		if region == "cn" {
			base = "https://copilot.tencent.com"
			origin = "https://www.codebuddy.cn"
			domain = "codebuddy.cn"
		}

		endpointAuthState := base + "/v2/plugin/auth/state?platform=CLI"
		data, _, err := doJSON(client, http.MethodPost, endpointAuthState, origin, nil, bytes.NewReader([]byte("{}")))
		if err != nil {
			fatal("auth state failed: %v", err)
		}
		var st struct {
			State   string `json:"state"`
			AuthURL string `json:"authUrl"`
		}
		if err := json.Unmarshal(data, &st); err != nil || st.State == "" || st.AuthURL == "" {
			fatal("auth state: missing state or authUrl")
		}
		raw, _ := json.Marshal(loginState{
			State:  st.State,
			Base:   base,
			Origin: origin,
			Domain: domain,
		})
		if err := os.WriteFile(stateFile, raw, 0o600); err != nil {
			fatal("write state: %v", err)
		}
		fmt.Println(st.AuthURL)

	case "poll":
		raw, err := os.ReadFile(stateFile)
		if err != nil {
			fatal("read state: %v (先跑 login url)", err)
		}
		var ls loginState
		if err := json.Unmarshal(raw, &ls); err != nil {
			fatal("parse state: %v", err)
		}
		if ls.Base == "" {
			ls.Base = "https://www.workbuddy.ai"
			ls.Origin = "https://www.workbuddy.ai"
			ls.Domain = "workbuddy.ai"
		}

		endpointAuthToken := ls.Base + "/v2/plugin/auth/token?state="
		tokRaw, status, errTok := doJSON(client, http.MethodGet, endpointAuthToken+ls.State, ls.Origin, nil, nil)
		if errTok != nil {
			if status == 0 || status >= 500 {
				fatal("token endpoint error: %v", errTok)
			}
			fatal("登录未完成（waiting for login）。请确认已在浏览器完成登录再按 y")
		}
		var tok struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresIn    int64  `json:"expiresIn"`
			Domain       string `json:"domain"`
		}
		if err := json.Unmarshal(tokRaw, &tok); err != nil || tok.AccessToken == "" {
			fatal("登录未完成（waiting for login）。请确认已在浏览器完成登录再按 y")
		}

		endpointLoginAcct := ls.Base + "/v2/plugin/login/account?state="
		var acct struct {
			UID          string `json:"uid"`
			EnterpriseID string `json:"enterpriseId"`
			Nickname     string `json:"nickname"`
		}
		acctHeaders := func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		}
		if acctRaw, _, errAcct := doJSON(client, http.MethodGet, endpointLoginAcct+ls.State, ls.Origin, acctHeaders, nil); errAcct == nil {
			_ = json.Unmarshal(acctRaw, &acct)
		}

		finalDomain := tok.Domain
		if finalDomain == "" || (ls.Domain == "workbuddy.ai" && !strings.Contains(strings.ToLower(finalDomain), "workbuddy")) {
			finalDomain = ls.Domain
		}

		out := map[string]any{
			"access_token":  tok.AccessToken,
			"refresh_token": tok.RefreshToken,
			"expires_in":    tok.ExpiresIn,
			"domain":        finalDomain,
			"uid":           acct.UID,
			"enterprise_id": acct.EnterpriseID,
			"nickname":      acct.Nickname,
		}
		oraw, _ := json.Marshal(out)
		fmt.Println(string(oraw))
		os.Remove(stateFile)

	default:
		fatal("unknown subcommand %q (want url|poll)", os.Args[1])
	}
}
