package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"qccg/account"
	"qccg/internal/bridge"
	"qccg/internal/cosy"
	"qccg/internal/updater"
	"qccg/logger"
)

// QoderModel 定义在 main 包以确保 Wails 生成 main.QoderModel 绑定。
type QoderModel struct {
	Key            string `json:"key"`
	DisplayName    string `json:"display_name"`
	Enable         bool   `json:"enable"`
	IsDefault      bool   `json:"is_default"`
	MaxInputTokens int    `json:"max_input_tokens,omitempty"`
}

// App 持有 Wails v3 的 app 和 window 引用，替代 v2 的 ctx。
type App struct {
	app         *application.App
	window      *application.WebviewWindow
	bridge      *bridge.Bridge
	bridgeSrv   *http.Server
	bridgeMu    sync.Mutex
	bridgePort  int
	bridgeToken string // 自定义鉴权 token，空则使用默认值 "qccg"

	// 托盘刷新回调，由 tray.go 设置
	refreshTray func()
}

func NewApp(app *application.App, window *application.WebviewWindow) *App {
	return &App{app: app, window: window, bridgePort: 8963}
}

// ServiceStartup 实现 v3 service 生命周期钩子，替代 v2 的 startup(ctx)。
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	// 文件日志在 Info("Application started") 之前初始化，确保启动期日志也能落盘
	if home, err := os.UserHomeDir(); err == nil {
		logDir := filepath.Join(home, ".qccg", "logs")
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

	// 启动后异步检查更新，不阻塞主流程
	go a.checkUpdateInBackground()

	return nil
}

// GetVersion 返回当前版本号。
func (a *App) GetVersion() string {
	return updater.Version
}

// checkUpdateInBackground 启动后异步检查更新，有新版本时通过前端回调通知。
func (a *App) checkUpdateInBackground() {
	// 等一秒确保前端加载完成
	time.Sleep(1 * time.Second)

	info, err := updater.Check()
	if err != nil {
		logger.Debug("更新检查失败: %v", err)
		return
	}
	if !info.HasUpdate {
		logger.Debug("当前已是最新版本 (%s)", info.Current)
		return
	}

	logger.Info("发现新版本: %s → %s", info.Current, info.Latest)

	// 通过 Wails 事件通知前端
	if a.app != nil {
		a.app.Event.Emit("update-available", info)
	}
}

// CheckUpdate 前端调用：手动检查更新。
func (a *App) CheckUpdate() *updater.UpdateInfo {
	info, err := updater.Check()
	if err != nil {
		logger.Error("手动检查更新失败: %v", err)
		return &updater.UpdateInfo{HasUpdate: false, Current: updater.Version}
	}
	return info
}

// ApplyUpdate 前端调用：执行更新，通过事件推送进度。
func (a *App) ApplyUpdate() error {
	info, err := updater.Check()
	if err != nil {
		return fmt.Errorf("检查更新失败: %w", err)
	}
	if !info.HasUpdate {
		return fmt.Errorf("没有可用更新")
	}

	logger.Info("开始更新到 %s", info.Latest)

	onProgress := func(pct int) {
		if a.app != nil {
			a.app.Event.Emit("update-progress", pct)
		}
	}

	ok, err := updater.Apply(info.DownloadURL, onProgress)
	if err != nil {
		return err
	}
	if ok {
		// 更新脚本已启动，退出当前 app
		if a.app != nil {
			a.app.Quit()
		}
	}
	return nil
}

func (a *App) ListAccounts() []account.Account {
	accounts, _ := account.List()
	if accounts == nil {
		return []account.Account{}
	}
	return accounts
}

func (a *App) AddAccountByPAT(pat string) (*account.Account, error) {
	defer a.refreshTrayMenu()
	mid := cosy.NewUUID()
	mtoken := cosy.NewBase64Token()
	mtype := cosy.NewHexToken(18)

	jt, err := cosy.ExchangeJobToken(pat, mid, mtoken, mtype)
	if err != nil {
		return nil, fmt.Errorf("验证 PAT 失败: %w", err)
	}
	id := account.SanitizeID(bridge.StrVal(jt, "id") + bridge.StrVal(jt, "name"))
	now := time.Now()
	acct := &account.Account{
		ID:        id,
		Name:      bridge.StrVal(jt, "name"),
		Email:     bridge.StrVal(jt, "email"),
		UserType:  bridge.StrValDefault(jt, "userType", "personal_standard"),
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
			a.window.EmitEvent("oauth:error", err.Error())
			return
		}
		if err := account.Save(acct); err != nil {
			a.window.EmitEvent("oauth:error", err.Error())
			return
		}
		a.refreshTrayMenu()
		a.window.EmitEvent("oauth:success", acct)
	}()
}

func (a *App) CancelOAuthLogin(loginID string) {
	account.CancelLogin(loginID)
}

func (a *App) HideWindow() {
	a.window.Hide()
}

func (a *App) QuitApp() {
	a.app.Quit()
}

func (a *App) Confirm(title, message string) bool {
	result := false
	dlg := a.app.Dialog.Question().SetMessage(message).SetTitle(title)
	dlg.AddButton("确认").OnClick(func() { result = true })
	cancelBtn := dlg.AddButton("取消").OnClick(func() { result = false })
	dlg.SetCancelButton(cancelBtn)
	dlg.Show()
	return result
}

func (a *App) DeleteAccount(id string) error {
	defer a.refreshTrayMenu()
	_ = account.DeleteSecret(id)
	return account.Delete(id)
}

func (a *App) SetActiveAccount(id string) error {
	defer a.refreshTrayMenu()
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
	defer a.refreshTrayMenu()
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
	defer a.refreshTrayMenu()
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

	// Parse baseprompt template
	tmpl := string(basePromptRaw)
	for _, ukey := range []string{"{UUID1}", "{UUID2}", "{UUID3}", "{UUID4}", "{UUID5}"} {
		tmpl = strings.ReplaceAll(tmpl, ukey, cosy.NewUUID())
	}
	tmpl = strings.ReplaceAll(tmpl, "{TIME1}", fmt.Sprintf("%d", cosy.UnixMs()))
	var templateBase map[string]interface{}
	json.Unmarshal([]byte(tmpl), &templateBase)

	b, err := bridge.NewBridge(pat, templateBase)
	if err != nil {
		logger.Error("Failed to create bridge: %v", err)
		return fmt.Errorf("failed to create bridge: %w", err)
	}
	logger.Info("Bridge created successfully")

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", b.HandleChatCompletions)
	mux.HandleFunc("/v1/messages", b.HandleClaudeMessages)
	mux.HandleFunc("/v1/models", b.HandleListModels)
	mux.HandleFunc("/v1/responses", b.HandleCodexResponses)
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
	defer a.refreshTrayMenu()
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
	deviceToken, _ := bridge.ParseOAuthSecret(token)
	if strings.HasPrefix(deviceToken, "dt-") {
		token = deviceToken
	} else {
		jt, err := cosy.ExchangeJobToken(token, cosy.NewUUID(), cosy.NewBase64Token(), cosy.NewHexToken(18))
		if err != nil {
			return nil, fmt.Errorf("exchange token: %w", err)
		}
		oauthToken := bridge.StrVal(jt, "securityOauthToken")
		if oauthToken == "" {
			return nil, fmt.Errorf("no securityOauthToken in response")
		}
		token = oauthToken
	}
	return account.FetchQuota(token)
}

// ListQoderModels 通过当前激活账号的 bridge 拉取 Qoder 上游可用模型列表。
func (a *App) ListQoderModels() ([]QoderModel, error) {
	a.bridgeMu.Lock()
	b := a.bridge
	a.bridgeMu.Unlock()
	if b == nil {
		return nil, fmt.Errorf("BRIDGE_NOT_RUNNING: bridge 未启动，请先激活账号并启动 bridge")
	}
	models, err := b.ListAvailableModels()
	if err != nil {
		return nil, err
	}
	out := make([]QoderModel, len(models))
	for i, m := range models {
		out[i] = QoderModel{
			Key:            m.Key,
			DisplayName:    m.DisplayName,
			Enable:         m.Enable,
			IsDefault:      m.IsDefault,
			MaxInputTokens: m.MaxInputTokens,
		}
	}
	return out, nil
}

// CleanupAllData 清理 qccg 产生的本地数据与注入配置。
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
	dataDir := filepath.Join(home, ".qccg")
	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("remove %s: %w", dataDir, err)
	}
	logger.Info("CleanupAllData completed: removed %s", dataDir)
	return nil
}

// refreshTrayMenu 回调 tray.go 的重建方法；nil-safe。
func (a *App) refreshTrayMenu() {
	if a.refreshTray != nil {
		a.refreshTray()
	}
}
