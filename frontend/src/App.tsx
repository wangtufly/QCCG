import { useState, useEffect } from 'react'
import { Events } from '@wailsio/runtime'
import appLogo from './assets/qoder.png'
import AccountsPage from './pages/AccountsPage'
import SettingsPage from './pages/SettingsPage'
import ClientConfigPage from './pages/ClientConfigPage'
import LogsPage from './pages/LogsPage'
import StatusIndicator from './components/StatusIndicator'
import { ApplyUpdate, GetVersion, CheckUpdate } from '../bindings/qccg/app'
import './App.css'

type Page = 'accounts' | 'settings' | 'clients' | 'logs'

interface UpdateInfo {
  has_update: boolean
  current: string
  latest: string
  body: string
  download_url: string
  file_size: number
}

export default function App() {
  const [page, setPage] = useState<Page>('accounts')
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null)
  const [updating, setUpdating] = useState(false)
  const [updateProgress, setUpdateProgress] = useState(0)
  const [updateError, setUpdateError] = useState<string | null>(null)
  const [version, setVersion] = useState<string>('')
  const [checkingUpdate, setCheckingUpdate] = useState(false)
  const [upToDate, setUpToDate] = useState(false)

  useEffect(() => {
    GetVersion().then(setVersion).catch(() => {})
  }, [])

  useEffect(() => {
    const unsubAvail = Events.On('update-available', (event: any) => {
      const data = event?.data
      if (data?.has_update) setUpdateInfo(data)
    })
    const unsubProgress = Events.On('update-progress', (event: any) => {
      const pct = typeof event?.data === 'number' ? event.data : 0
      setUpdateProgress(pct)
    })
    return () => { unsubAvail(); unsubProgress() }
  }, [])

  async function handleCheckUpdate() {
    if (checkingUpdate) return
    setCheckingUpdate(true)
    setUpToDate(false)
    try {
      const info = await CheckUpdate()
      if (info?.has_update) {
        setUpdateInfo(info as UpdateInfo)
      } else {
        setUpToDate(true)
        setTimeout(() => setUpToDate(false), 3000)
      }
    } catch (_) {}
    setCheckingUpdate(false)
  }

  async function handleUpdate() {
    if (!updateInfo) return
    setUpdating(true)
    setUpdateProgress(0)
    setUpdateError(null)
    try {
      await ApplyUpdate()
      // 更新脚本已启动， app 即将退出
    } catch (e: any) {
      setUpdateError(e?.message ?? String(e))
      setUpdating(false)
      setUpdateProgress(0)
    }
  }

  return (
    <div className="app">
      <header className="topbar">
        <div className="topbar-left">
          <span className="topbar-title">QCCG</span>
          <StatusIndicator />
        </div>
        <div className="topbar-right">
          {version && (
            <span
              className={`topbar-version${checkingUpdate ? ' topbar-version--checking' : ''}${upToDate ? ' topbar-version--ok' : ''}`}
              onClick={handleCheckUpdate}
              title="点击检查更新"
            >
              {upToDate ? '✓ 已是最新' : `v${version}`}
            </span>
          )}
          {updateInfo && (
            <div className="update-banner">
              <span className="update-banner-version">{updateInfo.latest}</span>
              <span className="update-banner-label">可用</span>
              {updateError && <span className="update-banner-error">{updateError}</span>}
              {updating ? (
                <div className="update-progress-wrap">
                  <div className="update-progress-bar" style={{width: `${updateProgress}%`}} />
                  <span className="update-progress-text">{updateProgress}%</span>
                </div>
              ) : (
                <>
                  <button className="update-btn-install" onClick={handleUpdate}>更新</button>
                  <button className="update-btn-dismiss" onClick={() => setUpdateInfo(null)}>×</button>
                </>
              )}
            </div>
          )}
        </div>
      </header>
      <nav className="sidebar">
        <div className="sidebar-brand" aria-hidden="true">
          <img src={appLogo} alt="" className="sidebar-logo" />
        </div>
        <button className={page === 'accounts' ? 'active' : ''} onClick={() => setPage('accounts')}>
          <svg className="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
            <circle cx="12" cy="7" r="4"/>
          </svg>
          <span>账号</span>
        </button>
        <button className={page === 'clients' ? 'active' : ''} onClick={() => setPage('clients')}>
          <svg className="nav-icon" viewBox="0 0 24 24" fill="currentColor" stroke="none">
            <path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zM13.26 22.43a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.795.795 0 0 0 .392-.681v-6.737l2.02 1.168a.071.071 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.494 4.494zM3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085 4.783 2.759a.771.771 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.766.766 0 0 0 .388.676l5.815 3.355-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.843-3.371 2.019-1.168a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.676 8.105v-5.678a.79.79 0 0 0-.4-.679zm2.01-3.023l-.141-.085-4.774-2.782a.776.776 0 0 0-.785 0L9.409 9.23V6.897a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.68 4.66zm-12.64 4.135l-2.02-1.164a.08.08 0 0 1-.038-.057V6.075a4.5 4.5 0 0 1 7.375-3.453l-.142.08L8.704 5.46a.795.795 0 0 0-.393.681zm1.097-2.365l2.602-1.5 2.607 1.5v2.999l-2.597 1.5-2.607-1.5z"/>
          </svg>
          <span>客户端</span>
        </button>
        <button className={page === 'settings' ? 'active' : ''} onClick={() => setPage('settings')}>
          <svg className="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="3"/>
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
          </svg>
          <span>设置</span>
        </button>
        <button className={page === 'logs' ? 'active' : ''} onClick={() => setPage('logs')}>
          <svg className="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
            <polyline points="14 2 14 8 20 8"/>
            <line x1="16" y1="13" x2="8" y2="13"/>
            <line x1="16" y1="17" x2="8" y2="17"/>
            <polyline points="10 9 9 9 8 9"/>
          </svg>
          <span>日志</span>
        </button>
      </nav>
      <main className="content">
        {page === 'accounts' && <AccountsPage />}
        {page === 'settings' && <SettingsPage />}
        {page === 'clients' && <ClientConfigPage />}
        {page === 'logs' && <LogsPage />}
      </main>
    </div>
  )
}
