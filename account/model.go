package account

import "time"

type Account struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Email     string     `json:"email,omitempty"`
	UserType  string     `json:"user_type,omitempty"`
	Plan      string     `json:"plan,omitempty"`
	AuthMode  string     `json:"auth_mode"` // "pat" | "oauth"
	APIMode   string     `json:"api_mode"`  // "openai" | "anthropic" | "gemini" | "claude-code"
	Tags      []string   `json:"tags"`
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	Active    bool       `json:"active"`
	SortOrder int        `json:"sort_order"`
}

type OAuthSession struct {
	LoginID  string `json:"login_id"`
	LoginURL string `json:"login_url"`
}

type Status struct {
	Running       bool   `json:"running"`
	Port          int    `json:"port"`
	ActiveAccount string `json:"active_account"`
	APIMode       string `json:"api_mode"`
}

type Settings struct {
	Port                 int    `json:"port"`
	AutoStart            bool   `json:"auto_start"`
	LogLevel             string `json:"log_level"`              // "info" | "debug" | "error"
	QuotaRefreshInterval int    `json:"quota_refresh_interval"` // 秒，0=不自动刷新
}

type QuotaInfo struct {
	Plan            string       `json:"plan"`
	UserQuota       *QuotaBucket `json:"user_quota,omitempty"`
	AddonQuota      *QuotaBucket `json:"addon_quota,omitempty"`
	IsQuotaExceeded bool         `json:"is_quota_exceeded"`
}

type QuotaBucket struct {
	Used      float64 `json:"used"`
	Total     float64 `json:"total"`
	Remaining float64 `json:"remaining"`
	ResetTime string  `json:"reset_time,omitempty"`
}
