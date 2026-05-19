import { useEffect, useState } from 'react'
import { GetStatus, StartBridge, StopBridge } from '../../bindings/qccg/app'

interface Status { running: boolean; port: number; active_account: string }

async function getStatus(): Promise<Status> {
  return GetStatus() as unknown as Status
}

async function startBridge() {
  return StartBridge()
}

async function stopBridge() {
  return StopBridge()
}

export default function StatusCard() {
  const [status, setStatus] = useState<Status>({ running: false, port: 8963, active_account: '' })

  const refresh = () => getStatus().then(s => { if (s) setStatus(s) })

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, 3000)
    return () => clearInterval(interval)
  }, [])

  const toggle = async () => {
    try {
      if (status.running) await stopBridge()
      else await startBridge()
      await refresh()
    } catch (err) {
      console.error('[StatusCard] Toggle error:', err)
      alert('操作失败: ' + err)
    }
  }

  return (
    <div className="status-card">
      <div className="status-header">
        <div className="status-indicator">
          <div className={`status-dot ${status.running ? 'running' : ''}`} />
          <span className="status-text">
            {status.running ? '服务运行中' : '服务已停止'}
          </span>
        </div>
        <button
          onClick={toggle}
          className={`btn btn-sm ${status.running ? 'btn-danger' : 'btn-primary'}`}
        >
          {status.running ? '停止' : '启动'}
        </button>
      </div>

      {status.running && (
        <div className="status-details">
          <div className="status-item">
            <span className="status-label">API 地址</span>
            <code className="status-value">http://127.0.0.1:{status.port}</code>
          </div>
          <div className="status-item">
            <span className="status-label">端口</span>
            <code className="status-value">{status.port}</code>
          </div>
          {status.active_account && (
            <div className="status-item">
              <span className="status-label">活跃账号</span>
              <span className="status-value">{status.active_account}</span>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
