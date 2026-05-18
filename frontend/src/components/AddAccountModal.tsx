import { useState } from 'react'

type Mode = 'select' | 'pat' | 'oauth-waiting'

interface Props { onClose: () => void }

export default function AddAccountModal({ onClose }: Props) {
  const [mode, setMode] = useState<Mode>('select')
  const [pat, setPat] = useState('')
  const [loginID, setLoginID] = useState('')
  const [loginURL, setLoginURL] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handlePAT = async () => {
    if (!pat.trim()) return
    setLoading(true)
    setError('')
    try {
      await (window as any).go?.main?.App?.AddAccountByPAT(pat.trim())
      onClose()
    } catch (e: any) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleOAuth = async () => {
    setLoading(true)
    setError('')
    try {
      const session = await (window as any).go?.main?.App?.StartOAuthLogin()
      if (!session) throw new Error('启动 OAuth 失败')
      setLoginID(session.login_id)
      setLoginURL(session.login_url)
      setMode('oauth-waiting')

      ;(window as any).runtime?.BrowserOpenURL(session.login_url)
      ;(window as any).runtime?.EventsOn('oauth:success', () => onClose())
      ;(window as any).runtime?.EventsOn('oauth:error', (msg: string) => {
        setError(msg)
        setMode('select')
      })
      ;(window as any).go?.main?.App?.WaitOAuthLogin(session.login_id)
    } catch (e: any) {
      setError(String(e))
      setLoading(false)
    }
  }

  const handleCancel = () => {
    if (loginID) (window as any).go?.main?.App?.CancelOAuthLogin(loginID)
    onClose()
  }

  return (
    <div className="modal-overlay" onClick={handleCancel}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <h3>添加账号</h3>

        {mode === 'select' && (
          <div className="add-account-options">
            <button className="add-option-card" onClick={() => setMode('pat')}>
              <div className="add-option-icon">🔑</div>
              <div className="add-option-info">
                <span className="add-option-title">PAT 导入</span>
                <span className="add-option-desc">在 Qoder 设置页获取 Personal Access Token 粘贴导入</span>
              </div>
            </button>
            <button className="add-option-card primary" onClick={handleOAuth} disabled={loading} style={{ display: 'none' }} aria-hidden="true" tabIndex={-1}>
              <div className="add-option-icon">🌐</div>
              <div className="add-option-info">
                <span className="add-option-title">{loading ? '启动中...' : 'OAuth 登录'}</span>
                <span className="add-option-desc">通过浏览器授权自动登录，推荐使用</span>
              </div>
            </button>
          </div>
        )}

        {mode === 'pat' && (
          <div>
            <p className="modal-hint">
              前往 Qoder 设置页 → API → Personal Access Token，复制后粘贴至下方
            </p>
            <input
              value={pat}
              onChange={e => setPat(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handlePAT()}
              placeholder="粘贴 PAT Token..."
            />
            <div className="actions actions-split">
              <button onClick={() => setMode('select')} className="btn btn-secondary">返回</button>
              <button
                onClick={handlePAT}
                disabled={loading || !pat.trim()}
                className="btn btn-primary"
                style={{ opacity: (loading || !pat.trim()) ? 0.5 : 1 }}
              >
                {loading ? '验证中...' : '导入'}
              </button>
            </div>
          </div>
        )}

        {mode === 'oauth-waiting' && (
          <div className="oauth-waiting">
            <div className="oauth-waiting-icon">🌐</div>
            <p className="oauth-waiting-title">浏览器已打开授权页面</p>
            <p className="oauth-waiting-hint">在浏览器中完成授权后将自动返回</p>
            <p className="oauth-waiting-url">{loginURL}</p>
            <div className="oauth-waiting-status">
              <span className="oauth-waiting-dot" />
              等待授权中...
            </div>
          </div>
        )}

        {error && <p className="modal-error">{error}</p>}

        <button onClick={handleCancel} className="modal-cancel">取消</button>
      </div>
    </div>
  )
}
