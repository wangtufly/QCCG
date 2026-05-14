import { useEffect, useState } from 'react'
import { DndContext, closestCenter, DragEndEvent } from '@dnd-kit/core'
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable'
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

async function reorderAccounts(ids: string[]) {
  if ((window as any).go?.main?.App?.ReorderAccounts) {
    return (window as any).go.main.App.ReorderAccounts(ids)
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

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = accounts.findIndex(a => a.id === active.id)
    const newIndex = accounts.findIndex(a => a.id === over.id)
    const reordered = arrayMove(accounts, oldIndex, newIndex)
    setAccounts(reordered)
    reorderAccounts(reordered.map(a => a.id))
  }

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
        <DndContext collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={accounts.map(a => a.id)} strategy={verticalListSortingStrategy}>
            <div className="accounts-grid">
              {accounts.map(a => (
                <AccountCard key={a.id} account={a} onActivate={handleActivate} onDelete={handleDelete} refreshInterval={refreshInterval} />
              ))}
            </div>
          </SortableContext>
        </DndContext>
      )}
      {showAdd && <AddAccountModal onClose={() => { setShowAdd(false); refresh() }} />}
    </div>
  )
}
