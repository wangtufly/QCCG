import { useState } from 'react'
import appLogo from './assets/qoder.png'
import AccountsPage from './pages/AccountsPage'
import SettingsPage from './pages/SettingsPage'
import ClientConfigPage from './pages/ClientConfigPage'
import LogsPage from './pages/LogsPage'
import StatusIndicator from './components/StatusIndicator'
import './App.css'

type Page = 'accounts' | 'settings' | 'clients' | 'logs'

export default function App() {
  const [page, setPage] = useState<Page>('accounts')

  return (
    <div className="app">
      <header className="topbar">
        <div className="topbar-left">
          <span className="topbar-title">Qoder2API</span>
          <StatusIndicator />
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
        <button className={page === 'settings' ? 'active' : ''} onClick={() => setPage('settings')}>
          <svg className="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="3"/>
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
          </svg>
          <span>设置</span>
        </button>
        <button className={page === 'clients' ? 'active' : ''} onClick={() => setPage('clients')}>
          <svg className="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
            <line x1="8" y1="21" x2="16" y2="21"/>
            <line x1="12" y1="17" x2="12" y2="21"/>
          </svg>
          <span>客户端</span>
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
