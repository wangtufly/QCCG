package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"qoder2api/account"
	"qoder2api/logger"
)

type App struct {
	ctx        context.Context
	bridge     *bridge
	bridgeSrv  *http.Server
	bridgeMu   sync.Mutex
	bridgePort int
}

func NewApp() *App {
	return &App{bridgePort: 8963}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logger.Info("Application started")

	settings, err := account.LoadSettings()
	if err == nil {
		logger.SetLevel(settings.LogLevel)
		logger.Info("Settings loaded: port=%d, log_level=%s", settings.Port, settings.LogLevel)
		a.bridgePort = settings.Port
	} else {
		logger.Error("Failed to load settings: %v", err)
	}

	acct, _ := account.GetActive()
	if acct != nil {
		_ = a.startBridgeWithAccount(acct)
	}
}

func (a *App) ListAccounts() []account.Account {
	accounts, _ := account.List()
	if accounts == nil {
		return []account.Account{}
	}
	return accounts
}

func (a *App) AddAccountByPAT(pat string) (*account.Account, error) {
	defer a.refreshAppMenu()
	mid := newUUID()
	mtoken := newBase64Token()
	mtype := newHexToken(18)

	jt, err := exchangeJobToken(pat, mid, mtoken, mtype)
	if err != nil {
		return nil, fmt.Errorf("验证 PAT 失败: %w", err)
	}
	id := account.SanitizeID(strVal(jt, "id") + strVal(jt, "name"))
	now := time.Now()
	acct := &account.Account{
		ID:        id,
		Name:      strVal(jt, "name"),
		Email:     strVal(jt, "email"),
		UserType:  strValDefault(jt, "userType", "personal_standard"),
		AuthMode:  "pat",
		APIMode:   "openai",
		Tags:      []string{},
		CreatedAt: now,
	}
	if err := account.SaveSecret(acct.ID, pat); err != nil {
		return nil, err
	}
	if err := account.Save(acct); err != nil {
		return nil, err
	}
	return acct, nil
}

func (a *App) StartOAuthLogin() (*account.OAuthSession, error) {
	return account.StartLogin()
}

func (a *App) WaitOAuthLogin(loginID string) {
	go func() {
		acct, err := account.WaitLogin(loginID)
		if err != nil {
			runtime.EventsEmit(a.ctx, "oauth:error", err.Error())
			return
		}
		if err := account.Save(acct); err != nil {
			runtime.EventsEmit(a.ctx, "oauth:error", err.Error())
			return
		}
		a.refreshAppMenu()
		runtime.EventsEmit(a.ctx, "oauth:success", acct)
	}()
}

func (a *App) CancelOAuthLogin(loginID string) {
	account.CancelLogin(loginID)
}

func (a *App) HideWindow() {
	runtime.WindowHide(a.ctx)
}

func (a *App) QuitApp() {
	runtime.Quit(a.ctx)
}

func (a *App) Confirm(title, message string) bool {
	result, _ := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         title,
		Message:       message,
		Buttons:       []string{"确认", "取消"},
		DefaultButton: "取消",
		CancelButton:  "取消",
	})
	return result == "确认"
}

func (a *App) DeleteAccount(id string) error {
	defer a.refreshAppMenu()
	_ = account.DeleteSecret(id)
	return account.Delete(id)
}

func (a *App) SetActiveAccount(id string) error {
	defer a.refreshAppMenu()
	if err := account.SetActive(id); err != nil {
		return err
	}
	acct, err := account.GetActive()
	if err != nil || acct == nil {
		return err
	}
	return a.restartBridge(acct)
}

func (a *App) GetStatus() account.Status {
	a.bridgeMu.Lock()
	defer a.bridgeMu.Unlock()
	running := a.bridgeSrv != nil
	activeID := ""
	apiMode := ""
	if acct, _ := account.GetActive(); acct != nil {
		activeID = acct.ID
		apiMode = acct.APIMode
	}
	return account.Status{Running: running, Port: a.bridgePort, ActiveAccount: activeID, APIMode: apiMode}
}

func (a *App) StartBridge() error {
	defer a.refreshAppMenu()
	logger.Info("StartBridge called")
	acct, err := account.GetActive()
	if err != nil || acct == nil {
		logger.Error("StartBridge: no active account (err=%v)", err)
		return fmt.Errorf("no active account")
	}
	logger.Info("StartBridge: using account %s", acct.Name)
	return a.startBridgeWithAccount(acct)
}

func (a *App) StopBridge() error {
	defer a.refreshAppMenu()
	a.bridgeMu.Lock()
	defer a.bridgeMu.Unlock()
	if a.bridgeSrv == nil {
		return nil
	}
	err := a.bridgeSrv.Close()
	a.bridgeSrv = nil
	a.bridge = nil
	return err
}

func (a *App) restartBridge(acct *account.Account) error {
	_ = a.StopBridge()
	return a.startBridgeWithAccount(acct)
}

func (a *App) startBridgeWithAccount(acct *account.Account) error {
	logger.Info("startBridgeWithAccount: account=%s", acct.Name)

	pat, err := account.GetSecret(acct.ID)
	if err != nil {
		logger.Error("Failed to get secret for account %s: %v", acct.ID, err)
		return fmt.Errorf("failed to get secret: %w", err)
	}
	logger.Debug("Got secret for account %s", acct.ID)

	b, err := newBridge(pat)
	if err != nil {
		logger.Error("Failed to create bridge: %v", err)
		return fmt.Errorf("failed to create bridge: %w", err)
	}
	logger.Info("Bridge created successfully")

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", b.handleChatCompletions)
	mux.HandleFunc("/v1/messages", b.handleClaudeMessages)
	mux.HandleFunc("/v1/responses", b.handleCodexResponses)
	logger.Info("Bridge: all endpoints registered")

	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", a.bridgePort),
		Handler: mux,
	}

	a.bridgeMu.Lock()
	a.bridge = b
	a.bridgeSrv = srv
	a.bridgeMu.Unlock()

	logger.Info("Bridge started on port %d", a.bridgePort)
	go srv.ListenAndServe()
	return nil
}

func (a *App) GetSettings() (*account.Settings, error) {
	return account.LoadSettings()
}

func (a *App) SaveSettings(s *account.Settings) error {
	logger.SetLevel(s.LogLevel)
	a.bridgePort = s.Port
	if s.QuotaRefreshInterval > 0 && s.QuotaRefreshInterval < 10 {
		s.QuotaRefreshInterval = 10
	}
	return account.SaveSettings(s)
}

// UpdateAccountAPIMode 更新账号的 API 模式
func (a *App) UpdateAccountAPIMode(accountID, apiMode string) error {
	defer a.refreshAppMenu()
	acct, err := account.Get(accountID)
	if err != nil {
		return err
	}
	acct.APIMode = apiMode
	if err := account.Save(acct); err != nil {
		return err
	}
	// 如果是当前活跃账号，重启 bridge
	if acct.Active {
		return a.restartBridge(acct)
	}
	return nil
}

func (a *App) GetLogs(limit int) []logger.Entry {
	return logger.GetLogs(limit)
}

func (a *App) ReorderAccounts(ids []string) error {
	return account.Reorder(ids)
}

func (a *App) ClearLogs() {
	logger.Clear()
}

func (a *App) GetAccountQuota(accountID string) (*account.QuotaInfo, error) {
	acct, err := account.Get(accountID)
	if err != nil {
		return nil, err
	}
	token, err := account.GetSecret(acct.ID)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	// PAT 需要先换取 securityOauthToken 才能调用 Qoder OpenAPI
	if !strings.HasPrefix(token, "dt-") {
		jt, err := exchangeJobToken(token, newUUID(), newBase64Token(), newHexToken(18))
		if err != nil {
			return nil, fmt.Errorf("exchange token: %w", err)
		}
		oauthToken := strVal(jt, "securityOauthToken")
		if oauthToken == "" {
			return nil, fmt.Errorf("no securityOauthToken in response")
		}
		token = oauthToken
	}
	return account.FetchQuota(token)
}
