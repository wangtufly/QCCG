import { marked } from 'marked'
import DOMPurify from 'dompurify'

interface UpdateInfo {
  has_update: boolean
  force_update?: boolean
  current: string
  latest: string
  body: string
  download_url: string
  file_size: number
}

interface UpdateModalProps {
  updateInfo: UpdateInfo | null
  onDismiss: () => void
  onUpdate: () => void
  updating: boolean
  progress: number
  error: string | null
}

export default function UpdateModal({ updateInfo, onDismiss, onUpdate, updating, progress, error }: UpdateModalProps) {
  if (!updateInfo) return null

  const forceUpdate = !!updateInfo.force_update

  const rawHtml = updateInfo.body
    ? DOMPurify.sanitize(marked.parse(updateInfo.body, { async: false }))
    : ''

  return (
    <div className="modal-overlay" onClick={forceUpdate ? undefined : onDismiss}>
      <div className="modal" style={{ maxWidth: 500 }} onClick={e => e.stopPropagation()}>
        <div style={{ marginBottom: 20 }}>
          <h3 style={{ margin: 0 }}>
            {forceUpdate && <span style={{ color: '#ef4444', marginRight: 6 }}>⚠</span>}
            发现新版本 {updateInfo.latest}
          </h3>
          <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginTop: 4 }}>
            当前版本：{updateInfo.current}
            {forceUpdate && <span style={{ color: '#ef4444', marginLeft: 8, fontWeight: 500 }}>此版本为强制更新</span>}
          </div>
        </div>

        <div style={{
          maxHeight: 280, overflowY: 'auto',
          background: 'var(--bg-app)',
          border: '1px solid var(--border)',
          borderRadius: 8, padding: '12px 16px',
          marginBottom: 16,
          fontSize: 13, lineHeight: 1.7,
          color: 'var(--text-primary)',
        }}>
          {rawHtml ? (
            <div className="update-changelog" dangerouslySetInnerHTML={{ __html: rawHtml }} />
          ) : (
            <span style={{ color: 'var(--text-secondary)', fontStyle: 'italic' }}>暂无更新说明</span>
          )}
        </div>

        {error && (
          <div style={{ color: '#ef4444', fontSize: 12, marginBottom: 12 }}>
            更新失败：{error}
          </div>
        )}

        {updating ? (
          <div>
            <div style={{
              height: 6, background: 'var(--primary-light)',
              borderRadius: 3, overflow: 'hidden', marginBottom: 8,
            }}>
              <div style={{
                height: '100%', background: 'var(--primary)',
                borderRadius: 3, width: `${progress}%`,
                transition: 'width 0.3s ease', minWidth: 4,
              }} />
            </div>
            <div style={{ fontSize: 12, color: 'var(--text-secondary)', textAlign: 'center' }}>
              正在下载更新… {progress}%
            </div>
          </div>
        ) : (
          <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end' }}>
            {!forceUpdate && (
              <button
                onClick={onDismiss}
                style={{
                  padding: '8px 18px', borderRadius: 8,
                  border: '1px solid var(--border)',
                  background: 'var(--bg-app)',
                  color: 'var(--text-secondary)',
                  fontSize: 13, fontWeight: 500, cursor: 'pointer',
                }}
              >关闭</button>
            )}
            <button className="btn btn-primary" onClick={onUpdate}>立即更新</button>
          </div>
        )}
      </div>
    </div>
  )
}
