import { useEffect, useState } from 'react'
import { GetStatus, StartBridge, StopBridge } from '../../bindings/qccg/app'

interface Status { running: boolean; port: number; active_account: string }

async function getStatus(): Promise<Status> {
  return GetStatus() as unknown as Status
}

async function startBridge() {
  try {
    console.log('[StatusBar] Starting bridge...')
    const result = await StartBridge()
    console.log('[StatusBar] Start result:', result)
    return result
  } catch (err) {
    console.error('[StatusBar] Start error:', err)
    throw err
  }
}

async function stopBridge() {
  try {
    console.log('[StatusBar] Stopping bridge...')
    const result = await StopBridge()
    console.log('[StatusBar] Stop result:', result)
    return result
  } catch (err) {
    console.error('[StatusBar] Stop error:', err)
    throw err
  }
}

export default function StatusBar() {
  const [status, setStatus] = useState<Status>({ running: false, port: 8963, active_account: '' })

  const refresh = () => getStatus().then(s => { if (s) setStatus(s) })

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, 3000)
    return () => clearInterval(interval)
  }, [])

  const toggle = async () => {
    try {
      if (status.running) {
        await stopBridge()
      } else {
        await startBridge()
      }
      await refresh()
    } catch (err) {
      console.error('[StatusBar] Toggle error:', err)
      alert('操作失败: ' + err)
    }
  }

  return (
    <div className="status-bar">
      <div className={`status-dot ${status.running ? 'running' : ''}`} />
      <span>{status.running ? `运行中 :${status.port}` : '已停止'}</span>
      {status.running && status.active_account && (
        <span style={{ color: 'var(--text-muted)' }}>· {status.active_account}</span>
      )}
      <button
        onClick={toggle}
        className="btn btn-sm"
        style={{
          marginLeft: 'auto',
          background: 'none',
          border: '1px solid var(--primary)',
          color: 'var(--primary)',
        }}
      >
        {status.running ? '停止' : '启动'}
      </button>
    </div>
  )
}
