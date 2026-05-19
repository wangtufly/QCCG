import { useState, useEffect, useRef } from 'react'
import { GetLogsSince, ClearLogs } from '../../bindings/qccg/app'

interface LogEntry {
  seq: number
  time: string
  level: string
  message: string
}

export default function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [filter, setFilter] = useState<string>('all')
  const lastSeqRef = useRef(0)
  const MAX_DISPLAY = 500

  const fetchIncremental = async (reset = false) => {
    try {
      const afterSeq = reset ? 0 : lastSeqRef.current
      const page = await GetLogsSince(afterSeq, reset ? 200 : 100)
      if (!page?.entries?.length) return
      lastSeqRef.current = page.last_seq
      setLogs(prev => {
        const next = reset ? page.entries : [...prev, ...page.entries]
        return next.length > MAX_DISPLAY ? next.slice(next.length - MAX_DISPLAY) : next
      })
    } catch (e) {
      console.error('[LogsPage] fetch failed:', e)
    }
  }

  useEffect(() => {
    lastSeqRef.current = 0
    fetchIncremental(true)
  }, [])

  useEffect(() => {
    if (!autoRefresh) return
    const id = setInterval(() => fetchIncremental(false), 2000)
    return () => clearInterval(id)
  }, [autoRefresh])

  const handleClear = async () => {
    await ClearLogs()
    lastSeqRef.current = 0
    setLogs([])
  }

  const filteredLogs = filter === 'all' ? logs : logs.filter(l => l.level === filter)

  const getLevelColor = (level: string) => {
    switch (level) {
      case 'debug': return 'var(--text-muted)'
      case 'info': return 'var(--primary)'
      case 'error': return 'var(--danger)'
      default: return 'var(--text-secondary)'
    }
  }

  return (
    <div className="logs-page">
      <div className="logs-header">
        <h2>日志</h2>
        <div className="logs-controls">
          <select value={filter} onChange={e => setFilter(e.target.value)}>
            <option value="all">全部</option>
            <option value="debug">Debug</option>
            <option value="info">Info</option>
            <option value="error">Error</option>
          </select>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={e => setAutoRefresh(e.target.checked)}
            />
            <span>自动刷新</span>
          </label>
          <button onClick={() => fetchIncremental(false)} className="btn btn-secondary">刷新</button>
          <button onClick={handleClear} className="btn btn-danger">清空</button>
        </div>
      </div>

      <div className="logs-container">
        {filteredLogs.length === 0 ? (
          <div className="logs-empty">暂无日志</div>
        ) : (
          filteredLogs.map(log => (
            <div key={log.seq} className="log-entry">
              <span className="log-time">{new Date(log.time).toLocaleTimeString()}</span>
              <span className="log-level" style={{ color: getLevelColor(log.level) }}>
                [{log.level.toUpperCase()}]
              </span>
              <span className="log-message">{log.message}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
