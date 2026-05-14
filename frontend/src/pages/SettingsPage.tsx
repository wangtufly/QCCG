import { useState, useEffect } from 'react'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'
import { account } from '../../wailsjs/go/models'

export default function SettingsPage() {
  const [settings, setSettings] = useState<account.Settings>(new account.Settings({
    port: 8963,
    auto_start: false,
    log_level: 'info'
  }))
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    GetSettings().then(s => {
      if (s) setSettings(s)
    }).catch(err => {
      console.error('Failed to load settings:', err)
    })
  }, [])

  const handleSave = async () => {
    try {
      await SaveSettings(settings)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (err) {
      console.error('Failed to save settings:', err)
    }
  }

  return (
    <div className="settings-page">
      <div className="page-header">
        <h2>设置</h2>
        <button
          className="btn btn-primary"
          onClick={handleSave}
          disabled={saved}
        >
          {saved ? '已保存' : '保存设置'}
        </button>
      </div>

      <div className="settings-grid">
        {/* Bridge 配置 */}
        <div className="settings-card">
          <div className="card-header">
            <div className="card-icon">🌐</div>
            <div>
              <h3>Bridge 服务</h3>
              <p>配置 API 代理服务器</p>
            </div>
          </div>

          <div className="setting-item">
            <div className="setting-label">
              <span>监听端口</span>
              <span className="setting-hint">Bridge 服务监听的本地端口</span>
            </div>
            <input
              type="number"
              value={settings.port}
              onChange={e => setSettings({...settings, port: parseInt(e.target.value)})}
              className="setting-input"
              min="1024"
              max="65535"
            />
          </div>

          <div className="setting-note">
            <div className="note-icon">💡</div>
            <div>
              <strong>提示：</strong>API 转换格式现在可以在账号管理页面为每个账号单独设置
            </div>
          </div>
        </div>

        {/* 应用配置 */}
        <div className="settings-card">
          <div className="card-header">
            <div className="card-icon">⚙️</div>
            <div>
              <h3>应用配置</h3>
              <p>自动启动和日志设置</p>
            </div>
          </div>

          <div className="setting-item">
            <div className="setting-label">
              <span>配额刷新间隔</span>
              <span className="setting-hint">账号配额自动刷新频率</span>
            </div>
            <select
              value={settings.quota_refresh_interval ?? 300}
              onChange={e => setSettings({...settings, quota_refresh_interval: parseInt(e.target.value)})}
              className="setting-select"
            >
              <option value="0">不自动刷新</option>
              <option value="10">每 10 秒</option>
              <option value="30">每 30 秒</option>
              <option value="60">每 1 分钟</option>
              <option value="300">每 5 分钟</option>
              <option value="600">每 10 分钟</option>
              <option value="1800">每 30 分钟</option>
              <option value="3600">每 1 小时</option>
            </select>
          </div>

          <div className="setting-item">
            <div className="setting-label">
              <span>开机自启</span>
              <span className="setting-hint">应用随系统启动</span>
            </div>
            <label className="toggle">
              <input
                type="checkbox"
                checked={settings.auto_start}
                onChange={e => setSettings({...settings, auto_start: e.target.checked})}
              />
              <span className="toggle-slider"></span>
            </label>
          </div>

          <div className="setting-item">
            <div className="setting-label">
              <span>日志级别</span>
              <span className="setting-hint">控制日志详细程度</span>
            </div>
            <select
              value={settings.log_level}
              onChange={e => setSettings({...settings, log_level: e.target.value})}
              className="setting-select"
            >
              <option value="error">Error - 仅错误</option>
              <option value="info">Info - 常规信息</option>
              <option value="debug">Debug - 调试详情</option>
            </select>
          </div>
        </div>

        {/* 存储位置 */}
        <div className="settings-card">
          <div className="card-header">
            <div className="card-icon">📁</div>
            <div>
              <h3>数据存储</h3>
              <p>账号和配置文件位置</p>
            </div>
          </div>

          <div className="storage-info">
            <div className="storage-item">
              <span className="storage-label">账号数据</span>
              <code className="storage-path">~/.qoder2api/accounts/</code>
            </div>
            <div className="storage-item">
              <span className="storage-label">密钥存储</span>
              <code className="storage-path">macOS Keychain</code>
            </div>
            <div className="storage-item">
              <span className="storage-label">配置文件</span>
              <code className="storage-path">~/.qoder2api/settings.json</code>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
