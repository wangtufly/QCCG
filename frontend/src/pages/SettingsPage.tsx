import { useState, useEffect } from 'react'
import { GetSettings, SaveSettings, ListQoderModels } from '../../wailsjs/go/main/App'
import { account, main } from '../../wailsjs/go/models'

// 默认映射「一键填充」内容，与后端 bridge.go::defaultModelMapping 对齐。
// 默认表只负责让请求落到合法 SKU 不出错；想分档（如 gpt-5→ultimate / gpt-5-mini→efficient）
// 请用户手动加行。下面的关键字依赖后端模糊匹配自动覆盖所有版本号变体。
const DEFAULT_MAPPING_HINT: Array<[string, string]> = [
  ['opus', 'ultimate'],
  ['sonnet', 'performance'],
  ['haiku', 'lite'],
  ['gpt', 'performance'],
  ['gemini', 'performance'],
]

// 客户端模型名候选清单：作为 datalist 提示，让用户既能选具体型号、也能直接输入关键字。
// 后端模糊匹配是「精确 → 长 source 优先 → 双向 substring」，所以两种粒度都生效：
//   写完整名（claude-sonnet-4-6 → ultimate）会优先于关键字（sonnet → performance）
//
// 排序原则：先放推荐关键字（短、覆盖广），再按使用频率排具体型号。
// 浏览器原生 datalist 不展示 label，仅作输入候选词，保持 UI 简洁。
const CLIENT_MODEL_OPTIONS: string[] = [
  // 关键字（短，靠后端模糊匹配自动覆盖所有版本号）
  'sonnet', 'opus', 'haiku',
  'gpt-5', 'gpt-4o', 'o3', 'gemini-2.5-pro',
  // Claude 主力
  'claude-sonnet-4-6', 'claude-opus-4-7', 'claude-haiku-4-5',
  'claude-sonnet-4-5', 'claude-opus-4-1-20250805',
  'claude-3-5-sonnet-latest', 'claude-3-5-haiku-20241022',
  // OpenAI 主力
  'gpt-5-mini', 'gpt-5-nano', 'gpt-5-codex',
  'gpt-4.1', 'gpt-4.1-mini', 'gpt-4o-mini',
  'o3-mini', 'o4-mini', 'o1', 'o1-mini',
  // Gemini 主力
  'gemini-2.5-flash', 'gemini-2.5-flash-lite',
  'gemini-2.0-flash', 'gemini-1.5-pro-latest', 'gemini-1.5-flash-latest',
]

type MappingRow = { id: number; from: string; to: string }

let nextRowId = 1
const newRowId = () => nextRowId++

function mappingToRows(m?: Record<string, string>): MappingRow[] {
  if (!m) return []
  return Object.entries(m).map(([from, to]) => ({ id: newRowId(), from, to }))
}

function rowsToMapping(rows: MappingRow[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const r of rows) {
    const k = r.from.trim()
    const v = r.to.trim()
    if (k && v) out[k] = v
  }
  return out
}

export default function SettingsPage() {
  const [settings, setSettings] = useState<account.Settings>(new account.Settings({
    port: 8963,
    auto_start: false,
    log_level: 'info'
  }))
  const [mappingRows, setMappingRows] = useState<MappingRow[]>([])
  const [saved, setSaved] = useState(false)
  const [qoderModels, setQoderModels] = useState<main.QoderModel[]>([])
  const [modelsLoading, setModelsLoading] = useState(false)
  const [modelsError, setModelsError] = useState<string | null>(null)

  const loadQoderModels = async () => {
    setModelsLoading(true)
    setModelsError(null)
    try {
      const list = await ListQoderModels()
      setQoderModels(list || [])
    } catch (err: any) {
      setModelsError(String(err?.message || err))
    } finally {
      setModelsLoading(false)
    }
  }

  useEffect(() => {
    GetSettings().then(s => {
      if (s) {
        setSettings(s)
        setMappingRows(mappingToRows(s.model_mapping))
      }
    }).catch(err => {
      console.error('Failed to load settings:', err)
    })
    loadQoderModels()
  }, [])

  const handleSave = async () => {
    try {
      const merged = new account.Settings({
        ...settings,
        model_mapping: rowsToMapping(mappingRows),
      })
      await SaveSettings(merged)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (err) {
      console.error('Failed to save settings:', err)
    }
  }

  const addRow = () => setMappingRows([...mappingRows, { id: newRowId(), from: '', to: '' }])
  const updateRow = (id: number, key: 'from' | 'to', value: string) =>
    setMappingRows(mappingRows.map(r => r.id === id ? { ...r, [key]: value } : r))
  const removeRow = (id: number) => setMappingRows(mappingRows.filter(r => r.id !== id))
  const fillDefaults = () => {
    const existing = new Set(mappingRows.map(r => r.from.trim()))
    const additions = DEFAULT_MAPPING_HINT
      .filter(([from]) => !existing.has(from))
      .map(([from, to]) => ({ id: newRowId(), from, to }))
    setMappingRows([...mappingRows, ...additions])
  }

  return (
    <div className="settings-page">
      <div className="page-header">
        <h2>设置</h2>
        <button
          className="btn btn-primary"
          onClick={handleSave}
          disabled={saved}
        >
          {saved ? '已保存' : '保存设置'}
        </button>
      </div>

      <div className="settings-grid">
        {/* Bridge 配置 */}
        <div className="settings-card">
          <div className="card-header">
            <div className="card-icon">🌐</div>
            <div>
              <h3>Bridge 服务</h3>
              <p>配置 API 代理服务器</p>
            </div>
          </div>

          <div className="setting-item">
            <div className="setting-label">
              <span>监听端口</span>
              <span className="setting-hint">Bridge 服务监听的本地端口</span>
            </div>
            <input
              type="number"
              value={settings.port}
              onChange={e => setSettings({...settings, port: parseInt(e.target.value)} as account.Settings)}
              className="setting-input"
              min="1024"
              max="65535"
            />
          </div>

          <div className="setting-note">
            <div className="note-icon">💡</div>
            <div>
              <strong>提示：</strong>API 转换格式现在可以在账号管理页面为每个账号单独设置
            </div>
          </div>
        </div>

        {/* 应用配置 */}
        <div className="settings-card">
          <div className="card-header">
            <div className="card-icon">⚙️</div>
            <div>
              <h3>应用配置</h3>
              <p>自动启动和日志设置</p>
            </div>
          </div>

          <div className="setting-item">
            <div className="setting-label">
              <span>配额刷新间隔</span>
              <span className="setting-hint">账号配额自动刷新频率</span>
            </div>
            <select
              value={settings.quota_refresh_interval ?? 300}
              onChange={e => setSettings({...settings, quota_refresh_interval: parseInt(e.target.value)} as account.Settings)}
              className="setting-select"
            >
              <option value="0">不自动刷新</option>
              <option value="10">每 10 秒</option>
              <option value="30">每 30 秒</option>
              <option value="60">每 1 分钟</option>
              <option value="300">每 5 分钟</option>
              <option value="600">每 10 分钟</option>
              <option value="1800">每 30 分钟</option>
              <option value="3600">每 1 小时</option>
            </select>
          </div>

          <div className="setting-item">
            <div className="setting-label">
              <span>开机自启</span>
              <span className="setting-hint">应用随系统启动</span>
            </div>
            <label className="toggle">
              <input
                type="checkbox"
                checked={settings.auto_start}
                onChange={e => setSettings({...settings, auto_start: e.target.checked} as account.Settings)}
              />
              <span className="toggle-slider"></span>
            </label>
          </div>

          <div className="setting-item">
            <div className="setting-label">
              <span>日志级别</span>
              <span className="setting-hint">控制日志详细程度</span>
            </div>
            <select
              value={settings.log_level}
              onChange={e => setSettings({...settings, log_level: e.target.value} as account.Settings)}
              className="setting-select"
            >
              <option value="error">Error - 仅错误</option>
              <option value="info">Info - 常规信息</option>
              <option value="debug">Debug - 调试详情</option>
            </select>
          </div>
        </div>

        {/* 模型映射 */}
        <div className="settings-card" style={{ gridColumn: '1 / -1' }}>
          <div className="card-header">
            <div className="card-icon">🔀</div>
            <div>
              <h3>模型映射</h3>
              <p>客户端模型名 → Qoder 上游 model.key（用于使用量正确上报）</p>
            </div>
          </div>

          <div className="setting-note">
            <div className="note-icon">💡</div>
            <div>
              客户端发送的模型名（左侧）会映射成 Qoder 内部 key（右侧）才会计入用量。
              <strong>左侧支持精确名（如 <code>claude-sonnet-4-6</code>）或关键字（如 <code>sonnet</code>）</strong>，
              匹配规则：精确命中 &gt; 长 source 优先 &gt; 双向 substring 模糊匹配。
              {modelsLoading && <> 正在拉取可用模型…</>}
              {!modelsLoading && qoderModels.length > 0 && (
                <> 当前账号可用 key：<code>{qoderModels.map(m => m.key).join(' / ')}</code>。</>
              )}
              {modelsError && (
                <> <span style={{ color: 'var(--danger, #c33)' }}>拉取模型失败：{modelsError}</span></>
              )}
              {!modelsLoading && qoderModels.length === 0 && !modelsError && (
                <> 模型列表暂未加载，可手动输入 key。</>
              )}
              <br />未配置任何映射时使用内置默认表：
              <code>opus→ultimate / sonnet→performance / haiku→lite / gpt→performance / gemini→performance</code>。
              想分档（如 <code>gpt-5→ultimate</code>）请手动加行。
            </div>
          </div>

          <div style={{ display: 'flex', gap: 8, margin: '12px 0' }}>
            <button className="btn btn-secondary" onClick={addRow}>+ 添加映射</button>
            <button className="btn btn-secondary" onClick={fillDefaults}>使用默认映射</button>
            <button className="btn btn-secondary" onClick={loadQoderModels} disabled={modelsLoading}>
              {modelsLoading ? '刷新中…' : '刷新模型列表'}
            </button>
          </div>

          {mappingRows.length === 0 ? (
            <div style={{ padding: 16, color: 'var(--muted, #888)', textAlign: 'center' }}>
              当前未配置自定义映射，将使用内置默认映射
            </div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ textAlign: 'left', fontSize: 12, color: 'var(--muted, #888)' }}>
                  <th style={{ padding: '6px 8px' }}>客户端模型名</th>
                  <th style={{ padding: '6px 8px', width: 24 }}></th>
                  <th style={{ padding: '6px 8px' }}>Qoder model.key</th>
                  <th style={{ padding: '6px 8px', width: 64 }}></th>
                </tr>
              </thead>
              <tbody>
                {mappingRows.map(r => (
                  <tr key={r.id}>
                    <td style={{ padding: '4px 8px' }}>
                      <input
                        type="text"
                        className="setting-input"
                        placeholder="精确名或关键字，例如 sonnet 或 claude-sonnet-4-6"
                        list="client-model-options"
                        value={r.from}
                        onChange={e => updateRow(r.id, 'from', e.target.value)}
                      />
                    </td>
                    <td style={{ textAlign: 'center', color: 'var(--muted, #888)' }}>→</td>
                    <td style={{ padding: '4px 8px' }}>
                      {qoderModels.length > 0 ? (
                        <select
                          className="setting-select"
                          value={r.to}
                          onChange={e => updateRow(r.id, 'to', e.target.value)}
                        >
                          <option value="">请选择 Qoder 模型…</option>
                          {qoderModels.map(m => (
                            <option key={m.key} value={m.key}>
                              {m.display_name} ({m.key}){m.is_default ? ' · 默认' : ''}
                            </option>
                          ))}
                          {/* 回显当前已配置但不在列表中的旧 key */}
                          {r.to && !qoderModels.some(m => m.key === r.to) && (
                            <option value={r.to}>{r.to}（自定义/已下线）</option>
                          )}
                        </select>
                      ) : (
                        <input
                          type="text"
                          className="setting-input"
                          placeholder="例如 performance"
                          value={r.to}
                          onChange={e => updateRow(r.id, 'to', e.target.value)}
                        />
                      )}
                    </td>
                    <td style={{ textAlign: 'right' }}>
                      <button
                        className="btn btn-secondary"
                        style={{ padding: '4px 10px' }}
                        onClick={() => removeRow(r.id)}
                      >删除</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          <datalist id="client-model-options">
            {CLIENT_MODEL_OPTIONS.map(m => (
              <option key={m} value={m} />
            ))}
          </datalist>
        </div>

        {/* 存储位置 */}
        <div className="settings-card">
          <div className="card-header">
            <div className="card-icon">📁</div>
            <div>
              <h3>数据存储</h3>
              <p>账号和配置文件位置</p>
            </div>
          </div>

          <div className="storage-info">
            <div className="storage-item">
              <span className="storage-label">账号数据</span>
              <code className="storage-path">~/.qoder2api/accounts/</code>
            </div>
            <div className="storage-item">
              <span className="storage-label">密钥存储</span>
              <code className="storage-path">macOS Keychain</code>
            </div>
            <div className="storage-item">
              <span className="storage-label">配置文件</span>
              <code className="storage-path">~/.qoder2api/settings.json</code>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
