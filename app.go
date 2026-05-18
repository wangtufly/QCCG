package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"qoder2api/account"
	"qoder2api/logger"
)

type App struct {
	ctx         context.Context
	bridge      *bridge
	bridgeSrv   *http.Server
	bridgeMu    sync.Mutex
	bridgePort  int
	bridgeToken string // 自定义鉴权 token，空则使用默认值 "qoder2api"
}

func NewApp() *App {
	return &App{bridgePort: 8963}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 文件日志在 Info("Application started") 之前初始化，确保启动期日志也能落盘
	if home, err := os.UserHomeDir(); err == nil {
		logDir := filepath.Join(home, ".qoder2api", "logs")
		if err := logger.InitFile(logDir); err != nil {
			fmt.Fprintf(os.Stderr, "[logger] init file sink failed: %v (logs will only be in memory + stdout)\n", err)
		}
	}

	logger.Info("Application started")

	settings, err := account.LoadSettings()
	if err == nil {
		logger.SetLevel(settings.LogLevel)
		logger.Info("Settings loaded: port=%d, log_level=%s", settings.Port, settings.LogLevel)
		a.bridgePort = settings.Port
		if settings.BridgeToken != "" {
			a.bridgeToken = settings.BridgeToken
		}
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
	mux.HandleFunc("/v1/models", b.handleListModels)
	mux.HandleFunc("/v1/responses", b.handleCodexResponses)
	logger.Info("Bridge: all endpoints registered")

	// 全局请求日志中间件
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("[HTTP] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", a.bridgePort),
		Handler: handler,
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
	if s.BridgeToken != "" {
		a.bridgeToken = s.BridgeToken
	} else {
		a.bridgeToken = ""
	}
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

func (a *App) GetLogsSince(afterSeq, limit int) logger.LogPage {
	return logger.GetLogsSince(afterSeq, limit)
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

// ListQoderModels 通过当前激活账号的 bridge 拉取 Qoder 上游可用模型列表，
// 供「模型映射」配置的下拉选择使用。如果 bridge 未启动则返回错误。
func (a *App) ListQoderModels() ([]QoderModel, error) {
	a.bridgeMu.Lock()
	b := a.bridge
	a.bridgeMu.Unlock()
	if b == nil {
		return nil, fmt.Errorf("BRIDGE_NOT_RUNNING: bridge 未启动，请先激活账号并启动 bridge")
	}
	return b.listAvailableModels()
}

// CleanupAllData 清理 qoder2api 产生的本地数据与注入配置。
func (a *App) CleanupAllData() error {
	if err := a.StopBridge(); err != nil {
		logger.Error("CleanupAllData stop bridge failed: %v", err)
	}

	for _, clientType := range []string{"claude", "codex", "gemini"} {
		if err := a.RemoveClientConfig(clientType); err != nil {
			logger.Error("CleanupAllData remove client config failed (%s): %v", clientType, err)
		}
	}

	accounts, err := account.List()
	if err == nil {
		for _, acct := range accounts {
			if err := account.DeleteSecret(acct.ID); err != nil {
				logger.Error("CleanupAllData delete secret failed (%s): %v", acct.ID, err)
			}
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	dataDir := filepath.Join(home, ".qoder2api")
	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("remove %s: %w", dataDir, err)
	}
	logger.Info("CleanupAllData completed: removed %s", dataDir)
	return nil
}
