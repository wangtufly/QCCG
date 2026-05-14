import { useEffect, useState } from 'react'
import AccountCard from '../components/AccountCard'
import AddAccountModal from '../components/AddAccountModal'

interface Account {
  id: string
  name: string
  email?: string
  auth_mode?: string
  api_mode?: string
  plan?: string
  active: boolean
}

async function listAccounts(): Promise<Account[]> {
  if ((window as any).go?.main?.App?.ListAccounts) {
    return (window as any).go.main.App.ListAccounts()
  }
  return []
}

async function setActiveAccount(id: string) {
  if ((window as any).go?.main?.App?.SetActiveAccount) {
    return (window as any).go.main.App.SetActiveAccount(id)
  }
}

async function deleteAccount(id: string) {
  if ((window as any).go?.main?.App?.DeleteAccount) {
    return (window as any).go.main.App.DeleteAccount(id)
  }
}

async function updateAccountAPIMode(id: string, apiMode: string) {
  if ((window as any).go?.main?.App?.UpdateAccountAPIMode) {
    return (window as any).go.main.App.UpdateAccountAPIMode(id, apiMode)
  }
}

export default function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [refreshInterval, setRefreshInterval] = useState(300)

  const refresh = () => listAccounts().then(setAccounts)

  useEffect(() => {
    refresh()
    ;(window as any).go?.main?.App?.GetSettings()
      .then((s: any) => { if (s?.quota_refresh_interval != null) setRefreshInterval(s.quota_refresh_interval) })
      .catch(() => {})
  }, [])

  const handleActivate = (id: string) => setActiveAccount(id).then(refresh)
  const handleDelete = async (id: string) => {
    const ok = await (window as any).go?.main?.App?.Confirm('删除账号', '确认删除此账号？此操作不可撤销。')
    if (ok) deleteAccount(id).then(refresh)
  }
  const handleAPIMode = (id: string, apiMode: string) => updateAccountAPIMode(id, apiMode).then(refresh)

  return (
    <div>
      <div className="page-header">
        <h2>账号管理</h2>
        <button onClick={() => setShowAdd(true)} className="btn btn-primary">
          添加账号
        </button>
      </div>

      {accounts.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon">👤</div>
          <p>暂无账号，点击右上角添加</p>
        </div>
      ) : (
        <div className="accounts-grid">
          {accounts.map(a => (
            <AccountCard key={a.id} account={a} onActivate={handleActivate} onDelete={handleDelete} onAPIMode={handleAPIMode} refreshInterval={refreshInterval} />
          ))}
        </div>
      )}
      {showAdd && <AddAccountModal onClose={() => { setShowAdd(false); refresh() }} />}
    </div>
  )
}
