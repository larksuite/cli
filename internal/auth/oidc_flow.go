package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// OIDCFlowOptions 包含 OIDC 认证选项
type OIDCFlowOptions struct {
	AuthProtocol string
	OpenBrowser  func(string) error // Optional interactive browser adapter.
	AppID        string
	AppSecret    string
	Domain       string // Accounts domain (for authorization)
	OpenDomain   string // Open domain (for token exchange)
	RedirectURI  string
	Timeout      time.Duration
	Scope        string // 可选的 scope 参数，用于请求 API 权限
}

// OIDCFlowResult 包含 OIDC 认证结果
type OIDCFlowResult struct {
	AuthProtocol     string
	Authority        string
	AccessToken      string
	RefreshToken     string
	IDToken          string // OIDC 特有
	ExpiresIn        int
	RefreshExpiresIn int
	Scope            string
	OpenID           string
	UserName         string
	Email            string                 // 用户邮箱
	Claims           map[string]interface{} // JWT 声明
}

// StartOIDCFlow 启动 OIDC 认证流程
func StartOIDCFlow(ctx context.Context, opts *OIDCFlowOptions, httpClient *http.Client, errOut io.Writer) (*OIDCFlowResult, error) {
	// 生成 CSRF 保护的 state 参数
	state := generateState()

	// 从 redirect_uri 解析监听地址
	redirectURL, err := url.Parse(opts.RedirectURI)
	if err != nil {
		return nil, fmt.Errorf("invalid redirect URI: %w", err)
	}

	// 启动本地 HTTP 服务器来接收回调
	listener, err := net.Listen("tcp", redirectURL.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener on %s: %w", redirectURL.Host, err)
	}
	defer listener.Close()

	// 用于接收授权码的通道
	codeChan := make(chan string, 1)
	errorChan := make(chan string, 1)

	var consume sync.Once
	callbackHandler := func(w http.ResponseWriter, r *http.Request) {
		expectedPath := redirectURL.Path
		if expectedPath == "" {
			expectedPath = "/"
		}
		if r.URL.Path != expectedPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		query := r.URL.Query()
		if query.Get("state") != state {
			consume.Do(func() { errorChan <- "invalid callback state" })
			http.Error(w, "Invalid callback state", http.StatusForbidden)
			return
		}
		if query.Get("error") != "" {
			consume.Do(func() { errorChan <- "authorization rejected" })
			http.Error(w, "Authorization rejected", http.StatusBadRequest)
			return
		}
		code := query.Get("code")
		if code == "" {
			consume.Do(func() { errorChan <- "missing authorization code" })
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}
		accepted := false
		consume.Do(func() { codeChan <- code; accepted = true })
		if !accepted {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("回调已收到，请返回终端查看最终结果。"))
	}

	// 在 goroutine 中启动 HTTP 服务器
	server := &http.Server{
		Handler: http.HandlerFunc(callbackHandler),
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			fmt.Fprintf(errOut, "[WARN] callback server error: %v\n", serveErr)
		}
	}()

	authRequestURL, err := BuildOIDCAuthorizationURL(opts, state)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(errOut, "Opening browser for authentication...\n")
	fmt.Fprintf(errOut, "If the browser does not open, visit the following URL:\n%s\n", authRequestURL)

	// 尝试打开浏览器
	opener := opts.OpenBrowser
	if opener == nil {
		opener = openBrowser
	}
	if err := opener(authRequestURL); err != nil {
		return nil, fmt.Errorf("failed to open authentication browser")
	}

	// 等待授权码或超时
	var authCode string
	select {
	case authCode = <-codeChan:
		// 成功收到授权码
		fmt.Fprintf(errOut, "Received authorization code.\n")
	case errorStr := <-errorChan:
		return nil, fmt.Errorf("authentication error: %s", errorStr)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(opts.Timeout):
		return nil, fmt.Errorf("authentication timed out after %v", opts.Timeout)
	}

	// 停止 HTTP 服务器
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = server.Shutdown(shutdownCtx)
	shutdownCancel()

	// 使用新的 OIDC API 客户端交换 Token
	client := NewOidcAccessTokenClient(httpClient, opts.OpenDomain)
	client.SetAppCredentials(opts.AppID, opts.AppSecret)
	tokenResp, err := client.CreateAccessToken(ctx, CreateAccessTokenParams{
		AppID:       opts.AppID,
		AppSecret:   opts.AppSecret,
		Code:        authCode,
		RedirectURI: opts.RedirectURI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// 飞书 OIDC API 不直接返回用户信息，需要额外调用用户信息 API
	var openID, userName, email string
	var claims map[string]interface{}

	// ID-token claims are non-authoritative metadata, never account identifiers.
	if tokenResp.IDToken != "" {
		claims, _ = parseIDTokenClaims(tokenResp.IDToken)
	}
	userInfo, err := client.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("trusted user_info required: %w", err)
	}
	if tokenResp.OpenID != "" && tokenResp.OpenID != userInfo.OpenID {
		return nil, fmt.Errorf("identity_conflict")
	}
	openID, userName, email = userInfo.OpenID, userInfo.Name, userInfo.Email

	return &OIDCFlowResult{
		AuthProtocol: "legacy_oidc", Authority: opts.OpenDomain,
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     tokenResp.RefreshToken,
		IDToken:          tokenResp.IDToken,
		ExpiresIn:        tokenResp.ExpiresIn,
		RefreshExpiresIn: tokenResp.RefreshExpiresIn,
		Scope:            tokenResp.Scope,
		OpenID:           openID,
		UserName:         userName,
		Email:            email,
		Claims:           claims,
	}, nil
}

// openBrowser tries to open the URL in the default browser
func openBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		return err
	}
	return nil
}

// generateState 生成 CSRF 保护的 state 参数
func generateState() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if random fails
		return fmt.Sprintf("state_%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// BuildOIDCAuthorizationURL constructs only parameters belonging to the selected protocol.
func BuildOIDCAuthorizationURL(opts *OIDCFlowOptions, state string) (string, error) {
	base, err := url.Parse(opts.Domain)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "https" && base.Scheme != "http") {
		return "", fmt.Errorf("invalid authorization base")
	}
	path := "/open-apis/authen/v1/user_auth_page_beta"
	if opts.AuthProtocol == "legacy_oidc" {
		path = "/open-apis/authen/v1/index"
	} else if opts.AuthProtocol != "" {
		return "", fmt.Errorf("unsupported OIDC authorization protocol")
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	params := url.Values{"app_id": {opts.AppID}, "redirect_uri": {opts.RedirectURI}, "state": {state}}
	if opts.AuthProtocol != "legacy_oidc" && opts.Scope != "" {
		params.Set("scope", opts.Scope)
	}
	base.RawQuery = params.Encode()
	return base.String(), nil
}
