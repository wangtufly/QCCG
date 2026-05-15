import { useState, useEffect, useMemo, useCallback } from 'react'
import {
  GetClientConfigs,
  ApplyClientConfig,
  RemoveClientConfig,
  ListQoderModels,
  GetStatus,
  ReadClientConfigFile,
  SaveClientConfigFile,
  GetSettings,
  SaveSettings,
} from '../../wailsjs/go/main/App'
import { account, main } from '../../wailsjs/go/models'
import claudeIcon from '../icons/clients/claude.svg'
import openaiIcon from '../icons/clients/openai.svg'
import geminiIcon from '../icons/clients/gemini.svg'

type ClientConfig = main.ClientConfig

const FALLBACK_QODER_KEYS = ['auto', 'ultimate', 'performance', 'efficient', 'lite']

const ICON_BY_TYPE: Record<string, string> = {
  claude: claudeIcon,
  codex: openaiIcon,
  gemini: geminiIcon,
}

// 默认映射「一键填充」内容，与后端 bridge.go::defaultModelMapping 对齐。
// 按 agent 分组，避免把 gpt 关键字塞进 claude 桶。
const DEFAULT_MAPPING_BY_AGENT: Record<string, Array<[string, string]>> = {
  claude: [['opus', 'ultimate'], ['sonnet', 'performance'], ['haiku', 'lite']],
  codex: [['gpt', 'performance']],
  gemini: [['gemini', 'performance']],
}

// 客户端模型名候选（datalist），按 agent 提供建议
const CLIENT_MODEL_OPTIONS_BY_AGENT: Record<string, string[]> = {
  claude: ['sonnet', 'opus', 'haiku', 'claude-sonnet-4-6', 'claude-opus-4-7', 'claude-haiku-4-5', 'claude-sonnet-4-5', 'claude-3-5-sonnet-latest', 'claude-3-5-haiku-20241022'],
  codex: ['gpt', 'gpt-5', 'gpt-5-mini', 'gpt-5-nano', 'gpt-5-codex', 'gpt-4o', 'gpt-4o-mini', 'gpt-4.1', 'gpt-4.1-mini', 'o3', 'o3-mini', 'o4-mini', 'o1', 'o1-mini'],
  gemini: ['gemini', 'gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-2.5-flash-lite', 'gemini-2.0-flash', 'gemini-1.5-pro-latest', 'gemini-1.5-flash-latest'],
}

export default function ClientConfigPage() {
  const [configs, setConfigs] = useState<ClientConfig[]>([])
  const [qoderModels, setQoderModels] = useState<main.QoderModel[]>([])
  const [loading, setLoading] = useState(true)
  const [applying, setApplying] = useState<string | null>(null)
  const [status, setStatus] = useState<{ running: boolean; port: number }>({ running: false, port: 8963 })
  const [activeType, setActiveType] = useState<string>('claude')

  // 配置文件编辑器
  const [fileLoading, setFileLoading] = useState(false)
  const [fileSaving, setFileSaving] = useState(false)
  const [fileContent, setFileContent] = useState('')
  const [fileMeta, setFileMeta] = useState<{ path: string; format: string; existed: boolean } | null>(null)
  const [fileDirty, setFileDirty] = useState(false)
  const [fileError, setFileError] = useState<string | null>(null)
  const [fileToast, setFileToast] = useState<string | null>(null)

  // 客户端模型名 → Qoder model.key 的待保存映射（按 agent 暂存到内存，仅在用户点击"保存"时写后端）
  const [pendingMapping, setPendingMapping] = useState<Record<string, Record<string, string>> | null>(null)
  const [mappingDirty, setMappingDirty] = useState(false)

  const refresh = async () => {
    try {
      const [c, m, s] = await Promise.all([
        GetClientConfigs(),
        ListQoderModels().catch(() => [] as main.QoderModel[]),
        GetStatus().catch(() => ({ running: false, port: 8963 })),
      ])
      setConfigs(c || [])
      setQoderModels(m || [])
      if (s) setStatus(s as any)
    } catch (err) {
      console.error('Failed to load configs:', err)
    } finally {
      setLoading(false)
    }
  }

  const loadFile = useCallback(async (type: string) => {
    setFileLoading(true)
    setFileError(null)
    setFileDirty(false)
    try {
      const r = await ReadClientConfigFile(type)
      setFileContent(r?.content || '')
      setFileMeta({ path: r?.path || '', format: r?.format || '', existed: !!r?.existed })
    } catch (err: any) {
      setFileError(String(err?.message || err))
      setFileContent('')
      setFileMeta(null)
    } finally {
      setFileLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, 5000)
    return () => clearInterval(interval)
  }, [])

  // 切 Tab 时同步加载该 client 的主配置文件原文
  useEffect(() => {
    if (activeType) loadFile(activeType)
  }, [activeType, loadFile])

  const activeCfg = useMemo(
    () => configs.find(c => c.type === activeType),
    [configs, activeType]
  )

  const handleApply = async (cfg: ClientConfig) => {
    setApplying(cfg.type)
    try {
      // 不再传入硬编码 model：让 CLI 自行选择模型，bridge 端 mapModel 兜底处理。
      // 用户的差异化映射通过下方「模型映射」分段配置。
      await ApplyClientConfig(cfg.type, '')
      await refresh()
      await loadFile(cfg.type) // 应用后重新加载文件
    } catch (err: any) {
      alert(`配置失败: ${err}`)
    } finally {
      setApplying(null)
    }
  }

  const handleRemove = async (cfg: ClientConfig) => {
    setApplying(cfg.type)
    try {
      await RemoveClientConfig(cfg.type)
      await refresh()
      await loadFile(cfg.type)
    } catch (err: any) {
      alert(`移除失败: ${err}`)
    } finally {
      setApplying(null)
    }
  }

  // 统一保存：同时写 CLI 配置文件 + 持久化模型映射到 Settings.json
  const handleSaveFile = async () => {
    if (!activeType) return
    setFileSaving(true)
    setFileError(null)
    try {
      // 1. 写文件（仅当文件有改动时）
      if (fileDirty) {
        await SaveClientConfigFile(activeType, fileContent)
        setFileDirty(false)
      }
      // 2. 写 Settings.ModelMappings（仅当映射有改动时）
      if (mappingDirty && pendingMapping) {
        const cur = await GetSettings()
        const merged = new account.Settings({
          ...(cur || {}),
          model_mapping: undefined,
          model_mappings: pendingMapping,
        } as any)
        await SaveSettings(merged)
        setMappingDirty(false)
      }
      setFileToast('已保存')
      setTimeout(() => setFileToast(null), 2000)
      await refresh()
    } catch (err: any) {
      setFileError(String(err?.message || err))
    } finally {
      setFileSaving(false)
    }
  }

  if (loading) return <div className="config-loading">加载中…</div>

  return (
    <div className="client-config-page">
      <div className="page-header">
        <h2>Agent 配置</h2>
        <button className="btn btn-secondary btn-sm" onClick={refresh} title="刷新状态">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="14" height="14" style={{ verticalAlign: 'middle', marginRight: 4 }}>
            <polyline points="23 4 23 10 17 10"/>
            <polyline points="1 20 1 14 7 14"/>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
          </svg>
          刷新
        </button>
      </div>

      {/* 横向 Tab 栏（紧贴页头之下，对齐 Sidebar 风格） */}
      <div className="client-tabs" role="tablist">
        {configs.map(cfg => {
          const iconSrc = ICON_BY_TYPE[cfg.type]
          return (
            <button
              key={cfg.type}
              role="tab"
              aria-selected={activeType === cfg.type}
              className={activeType === cfg.type ? 'active' : ''}
              onClick={() => setActiveType(cfg.type)}
              title={cfg.name}
            >
              {iconSrc ? <img src={iconSrc} alt="" className="tab-icon" /> : <span className="tab-icon" aria-hidden="true">{cfg.icon}</span>}
              <span>{cfg.name}</span>
              {cfg.applied && <span className="tab-applied-dot" aria-label="已配置" />}
            </button>
          )
        })}
      </div>

      {!status.running && (
        <div className="config-warning">
          ⚠️ Bridge 未启动，请先在「账号」页面激活账号并启动 Bridge 服务。
        </div>
      )}

      <div className="config-note">
        <div className="note-icon">💡</div>
        <div>
          <strong>一键配置</strong> — 自动写入对应 CLI 工具的配置文件，让请求经 Qoder2API Bridge 转发。
          <br />
          采用合并模式，仅修改 base_url / api_key / 模型字段，其它配置（hooks/MCP/permissions）原样保留。
          下方编辑器展示真实的配置文件原文，可手动修改后保存。
        </div>
      </div>

      {/* 当前 Tab 配置卡片 */}
      {activeCfg && (
        <div className={`config-card ${activeCfg.applied ? 'applied' : ''}`}>
          <div className="config-card-header">
            {ICON_BY_TYPE[activeCfg.type]
              ? <img src={ICON_BY_TYPE[activeCfg.type]} alt="" className="config-card-icon" />
              : <div className="config-card-icon-emoji">{activeCfg.icon}</div>}
            <div>
              <h3>{activeCfg.name}</h3>
              <p>{activeCfg.applied ? '已配置（由 qoder2api 管理）' : '未配置'}</p>
            </div>
            <div className={`config-status-dot ${activeCfg.applied ? 'active' : ''}`} />
          </div>

          <div className="config-card-body">
            {activeCfg.error && (
              <div className="config-warning">
                ⚠️ {activeCfg.error}
              </div>
            )}

            {/* 模型映射（按 agent 分组），改动暂存，"保存"按钮统一落盘 */}
            <ModelMappingSection
              agent={activeCfg.type}
              qoderModels={qoderModels}
              onMappingChange={(allMappings) => { setPendingMapping(allMappings); setMappingDirty(true) }}
              onMappingClean={() => setMappingDirty(false)}
            />

            {/* 配置文件编辑器（同卡片内分段） */}
            <div className="config-section-divider">
              <span className="section-title">📝 配置文件</span>
              <code className="section-path">{activeCfg.config_path}</code>
              <span className="meta">
                {fileMeta && !fileMeta.existed && '· 文件不存在'}
                {fileDirty ? ' · 未保存' : (fileMeta?.existed ? ' · 已保存' : '')}
              </span>
              <span className="divider-spacer" />
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => loadFile(activeType)}
                disabled={fileLoading}
                title="重新从磁盘加载（丢弃未保存的修改）"
              >{fileLoading ? '加载中…' : '重新加载'}</button>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => handleApply(activeCfg)}
                disabled={!status.running || applying === activeCfg.type}
                title="向 CLI 配置文件注入 base_url / api_key / marker（直接落盘后刷新编辑器）"
              >
                {applying === activeCfg.type ? '配置中…' : activeCfg.applied ? '更新配置' : '一键配置'}
              </button>
              {activeCfg.applied && (
                <button
                  className="btn btn-danger btn-sm"
                  onClick={() => handleRemove(activeCfg)}
                  disabled={applying === activeCfg.type}
                  title="清除 CLI 配置文件中由 qoder2api 注入的字段（直接落盘）"
                >移除</button>
              )}
              <button
                className="btn btn-primary btn-sm"
                onClick={handleSaveFile}
                disabled={(!fileDirty && !mappingDirty) || fileSaving}
                title="保存当前编辑器内容到配置文件，并持久化模型映射"
              >
                {fileSaving ? '保存中…' : '保存'}
              </button>
              {fileToast && <span style={{ color: 'var(--success)', fontSize: 12 }}>{fileToast}</span>}
            </div>
            <textarea
              className="config-editor"
              value={fileContent}
              onChange={e => { setFileContent(e.target.value); setFileDirty(true) }}
              onInput={e => { const t = e.currentTarget; t.style.height = 'auto'; t.style.height = t.scrollHeight + 'px' }}
              ref={el => { if (el) { el.style.height = 'auto'; el.style.height = el.scrollHeight + 'px' } }}
              spellCheck={false}
              placeholder={fileLoading ? '加载中…' : '配置文件内容…'}
              rows={1}
            />
            {fileError && (
              <div className="config-warning" style={{ marginTop: 0 }}>
                ⚠️ {fileError}
              </div>
            )}

          </div>
        </div>
      )}

    </div>
  )
}

// ============== ModelMappingSection ==============
// 仅本 agent 的模型映射卡片（嵌入 ClientConfigPage 的 config-card-body 中）
//
// 数据形态：Settings.ModelMappings = { claude: {sonnet: performance, ...}, codex: {...}, gemini: {...} }
// 兼容老数据：Settings.ModelMapping (扁平 map) 仅当 ModelMappings[claude] 不存在时迁移到 claude 桶。
//
// 行为：
//   - 改动行 → 调用 onMappingChange(allMappings) 通知父组件 dirty + 完整映射快照
//   - 不直接调用 SaveSettings；落盘由父组件的"保存"按钮统一处理（同时保存 CLI 配置文件 + Settings）
//   - 模型映射是 bridge 内部转换层，不写入 CLI 配置文件本体

type Row = { id: number; from: string; to: string }
let _rid = 1
const newRid = () => _rid++

interface MappingProps {
  agent: string
  qoderModels: main.QoderModel[]
  onMappingChange: (allMappings: Record<string, Record<string, string>>) => void
  onMappingClean: () => void
}

function ModelMappingSection({ agent, qoderModels, onMappingChange, onMappingClean }: MappingProps) {
  const [allMappings, setAllMappings] = useState<Record<string, Record<string, string>>>({})
  const [rows, setRows] = useState<Row[]>([])
  const [error, setError] = useState<string | null>(null)
  const [touched, setTouched] = useState(false)

  // 加载 / agent 切换时重新构造 rows
  useEffect(() => {
    let cancelled = false
    GetSettings().then(s => {
      if (cancelled) return
      const mappings = ((s?.model_mappings || {}) as Record<string, Record<string, string>>)
      let bucket: Record<string, string> | undefined = mappings[agent]
      if (!bucket && agent === 'claude' && s?.model_mapping && Object.keys(s.model_mapping).length > 0) {
        bucket = s.model_mapping
      }
      const list = bucket ? Object.entries(bucket).map(([from, to]) => ({ id: newRid(), from, to })) : []
      setAllMappings(mappings)
      setRows(list)
      setError(null)
      setTouched(false)
      onMappingClean()
    }).catch(err => setError(String(err?.message || err)))
    return () => { cancelled = true }
  }, [agent])

  // rows 变化 → 重组 allMappings 并通知父
  useEffect(() => {
    if (!touched) return
    const next: Record<string, string> = {}
    for (const r of rows) {
      const k = r.from.trim(), v = r.to.trim()
      if (k && v) next[k] = v
    }
    const merged = { ...allMappings }
    if (Object.keys(next).length === 0) delete merged[agent]
    else merged[agent] = next
    onMappingChange(merged)
  }, [rows, touched])

  const datalistId = `mapping-${agent}-options`
  const candidates = CLIENT_MODEL_OPTIONS_BY_AGENT[agent] || []

  const markTouched = () => { if (!touched) setTouched(true) }
  const addRow = () => { setRows([...rows, { id: newRid(), from: '', to: '' }]); markTouched() }
  const updateRow = (id: number, key: 'from' | 'to', v: string) => {
    setRows(rows.map(r => r.id === id ? { ...r, [key]: v } : r)); markTouched()
  }
  const removeRow = (id: number) => { setRows(rows.filter(r => r.id !== id)); markTouched() }
  const fillDefaults = () => {
    const existing = new Set(rows.map(r => r.from.trim()))
    const adds = (DEFAULT_MAPPING_BY_AGENT[agent] || [])
      .filter(([from]) => !existing.has(from))
      .map(([from, to]) => ({ id: newRid(), from, to }))
    if (adds.length > 0) { setRows([...rows, ...adds]); markTouched() }
  }

  return (
    <div className="mapping-section">
      <div className="mapping-section-header">
        <span className="section-title">🔀 模型映射</span>
        <span className="meta">客户端模型名 → Qoder model.key</span>
        <span className="divider-spacer" />
        <button className="btn btn-secondary btn-sm" onClick={fillDefaults} title="按家族关键字一键填充默认条目">默认</button>
        <button className="btn btn-secondary btn-sm" onClick={addRow}>+ 加一行</button>
      </div>

      {error && <div className="config-warning">⚠️ {error}</div>}

      {rows.length === 0 ? (
        <div className="mapping-empty">未配置自定义映射，将使用内置默认表（{(DEFAULT_MAPPING_BY_AGENT[agent] || []).map(([f, t]) => `${f}→${t}`).join(' / ') || '无'}）</div>
      ) : (
        <table className="mapping-table">
          <tbody>
            {rows.map(r => (
              <tr key={r.id}>
                <td>
                  <input
                    type="text"
                    className="setting-input"
                    placeholder="例如 sonnet 或 claude-sonnet-4-6"
                    list={datalistId}
                    value={r.from}
                    onChange={e => updateRow(r.id, 'from', e.target.value)}
                  />
                </td>
                <td className="arrow">→</td>
                <td>
                  {qoderModels.length > 0 ? (
                    <select
                      className="setting-select"
                      value={r.to}
                      onChange={e => updateRow(r.id, 'to', e.target.value)}
                    >
                      <option value="">请选择…</option>
                      {qoderModels.map(m => (
                        <option key={m.key} value={m.key}>{m.display_name} ({m.key}){m.is_default ? ' · 默认' : ''}</option>
                      ))}
                      {r.to && !qoderModels.some(m => m.key === r.to) && (
                        <option value={r.to}>{r.to}（自定义/已下线）</option>
                      )}
                    </select>
                  ) : (
                    <select
                      className="setting-select"
                      value={r.to}
                      onChange={e => updateRow(r.id, 'to', e.target.value)}
                    >
                      <option value="">请选择…</option>
                      {FALLBACK_QODER_KEYS.map(k => <option key={k} value={k}>{k}</option>)}
                    </select>
                  )}
                </td>
                <td className="row-actions">
                  <button className="btn-icon-sm" onClick={() => removeRow(r.id)} title="删除此行" aria-label="删除">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="14" height="14">
                      <line x1="18" y1="6" x2="6" y2="18"/>
                      <line x1="6" y1="6" x2="18" y2="18"/>
                    </svg>
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <datalist id={datalistId}>
        {candidates.map(m => <option key={m} value={m} />)}
      </datalist>
    </div>
  )
}
