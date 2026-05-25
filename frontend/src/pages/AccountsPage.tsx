import { useEffect, useState } from 'react'
import { DndContext, closestCenter, DragEndEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable'
import { Globe, Server } from 'lucide-react'
import AccountCard from '../components/AccountCard'
import AddAccountModal from '../components/AddAccountModal'
import {
  ListAccounts,
  SetActiveAccount,
  DeleteAccount,
  ReorderAccounts,
  GetSettings,
  Confirm,
} from '../../bindings/qccg/app'

interface Account {
  id: string
  name: string
  email?: string
  auth_mode?: string
  api_mode?: string
  plan?: string
  region?: string
  active: boolean
}

async function listAccounts(): Promise<Account[]> {
  const accounts = await ListAccounts()
  return (accounts as unknown as Account[]) ?? []
}

async function setActiveAccount(id: string) {
  return SetActiveAccount(id)
}

async function deleteAccount(id: string) {
  return DeleteAccount(id)
}

async function reorderAccounts(ids: string[]) {
  return ReorderAccounts(ids)
}

export default function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [refreshInterval, setRefreshInterval] = useState(300)
  const [activeRegion, setActiveRegion] = useState<'global' | 'cn'>('global')

  const refresh = () => listAccounts().then(list => {
    setAccounts(list)
    return list
  })

  // 拖拽至少移动 8px 才激活，避免吞掉卡片内按钮的 click 事件
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }))

  useEffect(() => {
    refresh().then(list => {
      const active = list.find(a => a.active)
      if (active) setActiveRegion((active.region || 'global') as 'global' | 'cn')
    })
    GetSettings()
      .then((s: any) => { if (s?.quota_refresh_interval != null) setRefreshInterval(s.quota_refresh_interval) })
      .catch(() => {})
  }, [])

  const handleActivate = (id: string) => setActiveAccount(id).then(refresh)
  const handleDelete = async (id: string) => {
    const target = accounts.find(a => a.id === id)
    if (target?.active) return
    const ok = await Confirm('删除账号', '确认删除此账号？此操作不可撤销。')
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

  const filteredAccounts = accounts.filter(a => (a.region || 'global') === activeRegion)

  return (
    <div>
      <div className="page-header">
        <h2>账号管理</h2>
        <button onClick={() => setShowAdd(true)} className="btn btn-primary">
          添加账号
        </button>
      </div>

      <div className="region-tabs">
        <button
          className={`region-tab ${activeRegion === 'global' ? 'active' : ''}`}
          onClick={() => setActiveRegion('global')}
        >
          <Globe size={14} />
          国际站
        </button>
        <button
          className={`region-tab ${activeRegion === 'cn' ? 'active' : ''}`}
          onClick={() => setActiveRegion('cn')}
        >
          <Server size={14} />
          国内站
        </button>
      </div>

      {filteredAccounts.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon">👤</div>
          <p>暂无{activeRegion === 'cn' ? '国内站' : '国际站'}账号，点击右上角添加</p>
        </div>
      ) : (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={filteredAccounts.map(a => a.id)} strategy={verticalListSortingStrategy}>
            <div className="accounts-grid">
              {filteredAccounts.map(a => (
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
