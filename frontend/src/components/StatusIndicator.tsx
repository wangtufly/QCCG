import { useEffect, useState } from 'react'
import { GetStatus, StartBridge, StopBridge } from '../../bindings/qccg/app'

interface Status { running: boolean; port: number; active_account: string; api_mode: string }

async function getStatus(): Promise<Status> {
  return GetStatus() as unknown as Status
}

async function startBridge() {
  return StartBridge()
}

async function stopBridge() {
  return StopBridge()
}

function getBaseUrl(port: number): string {
  return `http://127.0.0.1:${port}`
}

export default function StatusIndicator() {
  const [status, setStatus] = useState<Status>({ running: false, port: 8963, active_account: '', api_mode: '' })

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
      alert('操作失败: ' + err)
    }
  }

  return (
    <div className="status-indicator-bar">
      <button onClick={toggle} className="status-toggle-btn">
        {status.running ? '停止' : '启动'}
      </button>
      <div className={`status-dot-sm ${status.running ? 'running' : ''}`} />
      <span className="status-indicator-text">
        {status.running ? '运行中' : '已停止'}
      </span>
      {status.running && (
        <code className="status-indicator-url">
          {getBaseUrl(status.port)}
        </code>
      )}
    </div>
  )
}
