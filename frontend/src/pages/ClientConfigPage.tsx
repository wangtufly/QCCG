import { useState, useEffect, useMemo, useCallback, useRef } from 'react'
import { Wand2 as WandIcon, History, FolderSync, Rocket, Save, AlertTriangle, RefreshCw, CirclePlus } from 'lucide-react'
import ConfigEditor, { ConfigEditorHandle } from '../components/ConfigEditor'
import {
  GetClientConfigs,
  ListQoderModels,
  GetStatus,
  ReadClientConfigFile,
  SaveClientConfigFile,
  SaveAdditionalClientConfigFile,
  GetSettings,
  SaveSettings,
  HasClientConfigBackup,
  RestoreClientConfigFile,
} from '../../bindings/qccg/app'
import * as account from '../../bindings/qccg/account'
import * as main from '../../bindings/qccg/models'
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

const DEFAULT_MAPPING_BY_AGENT: Record<string, Array<[string, string]>> = {
  claude: [['opus', 'ultimate'], ['sonnet', 'performance'], ['haiku', 'lite']],
  codex: [['gpt', 'performance']],
  gemini: [['gemini', 'performance']],
}

const CLIENT_MODEL_OPTIONS_BY_AGENT: Record<string, string[]> = {
  claude: ['sonnet', 'opus', 'haiku', 'claude-sonnet-4-6', 'claude-opus-4-7', 'claude-haiku-4-5', 'claude-sonnet-4-5', 'claude-3-5-sonnet-latest', 'claude-3-5-haiku-20241022'],
  codex: ['gpt', 'gpt-5', 'gpt-5-mini', 'gpt-5-nano', 'gpt-5-codex', 'gpt-4o', 'gpt-4o-mini', 'gpt-4.1', 'gpt-4.1-mini', 'o3', 'o3-mini', 'o4-mini', 'o1', 'o1-mini'],
  gemini: ['gemini', 'gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-2.5-flash-lite', 'gemini-2.0-flash', 'gemini-1.5-pro-latest', 'gemini-1.5-flash-latest'],
}

// claude JSON 中三个模型槽位字段与映射 from-key 的对应关系
const CLAUDE_MODEL_SLOTS: Array<{ envKey: string; fromKey: string }> = [
  { envKey: 'ANTHROPIC_DEFAULT_OPUS_MODEL',   fromKey: 'opus'   },
  { envKey: 'ANTHROPIC_DEFAULT_SONNET_MODEL', fromKey: 'sonnet' },
  { envKey: 'ANTHROPIC_DEFAULT_HAIKU_MODEL',  fromKey: 'haiku'  },
]

const PREVIEW_MAPPING_KEY = '_qccg_model_mapping'
const QODER_API_KEY = 'qccg'

// ============== MappingSelect ==============
interface MappingSelectProps {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  placeholder?: string
}

function MappingSelect({ value, onChange, options, placeholder = '请选择…' }: MappingSelectProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const selected = options.find(o => o.value === value)

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div ref={ref} className="mapping-custom-select" onClick={() => setOpen(o => !o)}>
      <span className="mapping-custom-select-value">
        {selected ? selected.label : <span className="mapping-custom-select-placeholder">{placeholder}</span>}
      </span>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="12" height="12" style={{ flexShrink: 0, color: 'var(--text-muted)' }}>
        <polyline points="6 9 12 15 18 9"/>
      </svg>
      {open && (
        <div className="mapping-custom-dropdown">
          {options.map(o => (
            <div
              key={o.value}
              className={`mapping-custom-option${o.value === value ? ' selected' : ''}`}
              onMouseDown={e => { e.stopPropagation(); onChange(o.value); setOpen(false) }}
            >
              {o.label}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ============== 配置文件文本工具 ==============

function applyConfigToContent(content: string, format: string, agentType: string, port: number, apiKey: string): string {
  const baseURL = `http://127.0.0.1:${port}`
  if (format === 'json') {
    try {
      const obj = content.trim() ? JSON.parse(content) : {}
      if (!obj['env'] || typeof obj['env'] !== 'object') obj['env'] = {}
      const env = obj['env'] as Record<string, string>
      if (agentType === 'claude') {
        env['ANTHROPIC_BASE_URL'] = baseURL
        env['ANTHROPIC_AUTH_TOKEN'] = apiKey
      }
      return JSON.stringify(obj, null, 2) + '\n'
    } catch { return content }
  }
  if (format === 'toml') {
    try {
      const lines = content.trimEnd().split('\n')
      const filtered = lines.filter(l => !l.startsWith('model_provider'))
      filtered.push(`model_provider = "qccg"`)
      return filtered.join('\n') + '\n'
    } catch { return content }
  }
  if (format === 'dotenv') {
    const lines = content.trimEnd().split('\n').filter(l =>
      !l.startsWith('GOOGLE_GEMINI_BASE_URL=') && !l.startsWith('GEMINI_API_KEY=')
    )
    lines.push(`GOOGLE_GEMINI_BASE_URL=${baseURL}`)
    lines.push(`GEMINI_API_KEY=${apiKey}`)
    return lines.join('\n') + '\n'
  }
  return content
}

function applyAdditionalConfigToContent(content: string, format: string, agentType: string, path: string, apiKey: string): string {
  if (agentType === 'codex' && format === 'json' && path.endsWith('/.codex/auth.json')) {
    try {
      const obj = content.trim() ? JSON.parse(content) : {}
      obj['OPENAI_API_KEY'] = apiKey
      return JSON.stringify(obj, null, 2) + '\n'
    } catch {
      return content
    }
  }
  return content
}

const TOP_LEVEL_KEY_ORDER = ['env', 'enabledPlugins', 'permissions', 'model', 'extensions', 'hooks']
const ENV_KEY_ORDER = [
  'ANTHROPIC_BASE_URL', 'ANTHROPIC_AUTH_TOKEN',
  'ANTHROPIC_DEFAULT_OPUS_MODEL', 'ANTHROPIC_DEFAULT_SONNET_MODEL', 'ANTHROPIC_DEFAULT_HAIKU_MODEL',
  'ANTHROPIC_MODEL',
]

function sortJSONKeys(obj: unknown): unknown {
  if (Array.isArray(obj)) return obj.map(sortJSONKeys)
  if (obj !== null && typeof obj === 'object') {
    const o = obj as Record<string, unknown>
    const sortWithPriority = (keys: string[], priorityList: string[]) => {
      const priority = keys.filter(k => priorityList.includes(k)).sort((a, b) => priorityList.indexOf(a) - priorityList.indexOf(b))
      const rest = keys.filter(k => !priorityList.includes(k)).sort()
      return [...priority, ...rest]
    }
    const isEnvObj = Object.keys(o).some(k => ENV_KEY_ORDER.includes(k))
    const ordered = sortWithPriority(Object.keys(o), isEnvObj ? ENV_KEY_ORDER : TOP_LEVEL_KEY_ORDER)
    const result: Record<string, unknown> = {}
    for (const k of ordered) result[k] = sortJSONKeys(o[k])
    return result
  }
  return obj
}

function formatContent(content: string, format: string): string {
  const trimmed = content.trim()
  if (!trimmed) return ''
  if (format === 'json') {
    return JSON.stringify(sortJSONKeys(JSON.parse(trimmed)), null, 2) + '\n'
  }
  throw new Error(`${format} 暂不支持自动整理`)
}

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function stripMappingPreview(content: string, format: string): string {
  if (!content) return content
  if (format === 'json') {
    try {
      const obj = JSON.parse(content)
      if (obj && typeof obj === 'object' && PREVIEW_MAPPING_KEY in obj) {
        delete obj[PREVIEW_MAPPING_KEY]
        return JSON.stringify(obj, null, 2) + '\n'
      }
    } catch {}
    return content
  }
  if (format === 'toml') {
    const re = new RegExp(`(?:^|\\n)\\[${escapeRe(PREVIEW_MAPPING_KEY)}\\][^\\n]*(?:\\n(?!\\[)[^\\n]*)*`, 'g')
    return content.replace(re, '').replace(/\n{3,}/g, '\n\n').replace(/\s+$/, '') + '\n'
  }
  if (format === 'dotenv') {
    const re = new RegExp(`(?:^|\\n)# ${escapeRe(PREVIEW_MAPPING_KEY)}=[^\\n]*`, 'g')
    return content.replace(re, '').replace(/\n{3,}/g, '\n\n').replace(/\s+$/, '') + '\n'
  }
  return content
}

function patchMappingPreview(content: string, format: string, mapping: Record<string, string> | undefined, agent?: string): string {
  const cleaned = stripMappingPreview(content, format)
  if (format === 'json' && agent === 'claude') {
    try {
      const obj = cleaned.trim() ? JSON.parse(cleaned) : {}
      if (obj['env'] && typeof obj['env'] === 'object') {
        const env = obj['env'] as Record<string, string>
        for (const { envKey } of CLAUDE_MODEL_SLOTS) delete env[envKey]
        if (Object.keys(env).length === 0) delete obj['env']
      }
      if (mapping && Object.keys(mapping).length > 0) {
        if (!obj['env'] || typeof obj['env'] !== 'object') obj['env'] = {}
        const env = obj['env'] as Record<string, string>
        for (const { envKey, fromKey } of CLAUDE_MODEL_SLOTS) {
          const mappedTo = mapping[fromKey]
          if (mappedTo) env[envKey] = mappedTo
        }
      }
      return JSON.stringify(obj, null, 2) + '\n'
    } catch { return content }
  }
  if (!mapping || Object.keys(mapping).length === 0) return cleaned
  if (format === 'json') {
    try {
      const obj = cleaned.trim() ? JSON.parse(cleaned) : {}
      obj[PREVIEW_MAPPING_KEY] = mapping
      return JSON.stringify(obj, null, 2) + '\n'
    } catch { return content }
  }
  if (format === 'toml') {
    const head = cleaned.replace(/\s+$/, '')
    const lines: string[] = []
    lines.push(`[${PREVIEW_MAPPING_KEY}]  # qccg 模型映射预览（仅展示，由 Settings.json 管理；保存时不会写入此文件）`)
    for (const [k, v] of Object.entries(mapping)) {
      const key = /^[A-Za-z0-9_-]+$/.test(k) ? k : JSON.stringify(k)
      lines.push(`${key} = ${JSON.stringify(v)}`)
    }
    return (head ? head + '\n\n' : '') + lines.join('\n') + '\n'
  }
  if (format === 'dotenv') {
    const head = cleaned.replace(/\s+$/, '')
    return (head ? head + '\n' : '') + `# ${PREVIEW_MAPPING_KEY}=${JSON.stringify(mapping)}\n`
  }
  return cleaned
}

// ============== ClientConfigPage ==============

export default function ClientConfigPage() {
  const [configs, setConfigs] = useState<ClientConfig[]>([])
  const [qoderModels, setQoderModels] = useState<main.QoderModel[]>([])
  const [loading, setLoading] = useState(true)
  const [status, setStatus] = useState<{ running: boolean; port: number }>({ running: false, port: 8963 })
  const [activeType, setActiveType] = useState<string>('claude')

  // 从 Settings 拉取的数据，统一在父组件管理，避免子组件重复 IPC
  const [savedMappings, setSavedMappings] = useState<Record<string, Record<string, string>>>({})
  const [bridgeToken, setBridgeToken] = useState(QODER_API_KEY)

  // 配置文件编辑器
  const [fileLoading, setFileLoading] = useState(false)
  const [fileSaving, setFileSaving] = useState(false)
  const [fileContent, setFileContent] = useState('')
  const [fileMeta, setFileMeta] = useState<{ path: string; format: string; existed: boolean } | null>(null)
  const [extraFiles, setExtraFiles] = useState<Array<{ path: string; format: string; existed: boolean; content: string }>>([])
  const [fileDirty, setFileDirty] = useState(false)
  const [fileError, setFileError] = useState<string | null>(null)
  const [formatStatus, setFormatStatus] = useState<'idle' | 'done' | 'error'>('idle')
  const [hasBackup, setHasBackup] = useState(false)
  const editorRef = useRef<ConfigEditorHandle>(null)

  const [pendingMapping, setPendingMapping] = useState<Record<string, Record<string, string>> | null>(null)
  const [mappingDirty, setMappingDirty] = useState(false)
  const [modelsLoading, setModelsLoading] = useState(false)
  const [modelsError, setModelsError] = useState<string | null>(null)

  const refreshQoderModels = useCallback(async () => {
    setModelsLoading(true)
    setModelsError(null)
    try {
      const models = await ListQoderModels()
      setQoderModels(models || [])
      return models || []
    } catch (err: any) {
      const msg = String(err?.message || err)
      if (msg.includes('BRIDGE_NOT_RUNNING')) {
        setModelsError('Bridge 未启动，当前显示的是兜底模型列表。请先激活账号并启动 Bridge 后刷新。')
      } else {
        setModelsError(`拉取模型列表失败：${msg}`)
      }
      setQoderModels([])
      return [] as main.QoderModel[]
    } finally {
      setModelsLoading(false)
    }
  }, [])

  // 初始加载：并行拉取所有数据
  useEffect(() => {
    Promise.all([
      GetClientConfigs(),
      refreshQoderModels(),
      GetStatus().catch(() => ({ running: false, port: 8963 })),
      GetSettings().catch(() => null),
    ]).then(([c, _m, s, settings]) => {
      setConfigs(c || [])
      if (s) setStatus(s as any)
      if (settings) {
        const mappings = (settings.model_mappings || {}) as Record<string, Record<string, string>>
        setSavedMappings(mappings)
        setBridgeToken(settings.bridge_token || QODER_API_KEY)
      }
    }).catch(err => {
      console.error('Failed to load configs:', err)
    }).finally(() => setLoading(false))
  }, [refreshQoderModels])

  const loadFile = useCallback(async (type: string) => {
    setFileLoading(true)
    setFileError(null)
    setFileDirty(false)
    try {
      const r = await ReadClientConfigFile(type)
      setFileContent(r?.content || '')
      setFileMeta({ path: r?.path || '', format: r?.format || '', existed: !!r?.existed })
      const extras = (r?.extra_files || []).map((f: any) => ({
        path: f.path || '',
        format: f.format || '',
        existed: !!f.existed,
        content: f.content || '',
      }))
      setExtraFiles(extras)
      setHasBackup(await HasClientConfigBackup(type))
    } catch (err: any) {
      setFileError(String(err?.message || err))
      setFileContent('')
      setFileMeta(null)
      setExtraFiles([])
    } finally {
      setFileLoading(false)
    }
  }, [])

  // 切 Tab 时加载配置文件
  useEffect(() => {
    if (activeType) loadFile(activeType)
  }, [activeType, loadFile])

  // 文件就绪后注入映射预览
  useEffect(() => {
    if (!fileMeta?.format || !activeType) return
    const bucket = savedMappings[activeType]
    setFileContent(prev => patchMappingPreview(prev, fileMeta.format, bucket || {}, activeType))
  }, [fileMeta?.path, activeType])

  // bridge 从未运行->运行时，自动刷新一次模型列表
  useEffect(() => {
    if (!status?.running) return
    refreshQoderModels()
  }, [status?.running, refreshQoderModels])

  const activeCfg = useMemo(
    () => configs.find(c => c.type === activeType),
    [configs, activeType]
  )

  const handleApply = async (cfg: ClientConfig) => {
    if (!fileMeta?.format) return
    const bucket = pendingMapping?.[cfg.type] ?? savedMappings[cfg.type]
    let applied = applyConfigToContent(fileContent, fileMeta.format, cfg.type, status.port, bridgeToken)
    applied = patchMappingPreview(applied, fileMeta.format, bucket || {}, cfg.type)
    setFileContent(applied)
    if (extraFiles.length > 0) {
      setExtraFiles(prev => prev.map(extra => ({
        ...extra,
        content: applyAdditionalConfigToContent(extra.content, extra.format, cfg.type, extra.path, bridgeToken),
      })))
    }
    setFileDirty(true)
  }

  const handleRestore = async () => {
    if (!activeType) return
    try {
      await RestoreClientConfigFile(activeType)
      setFileDirty(false)
      setMappingDirty(false)
      // 还原后只需重新加载文件和 configs（不需要重拉模型列表）
      const [c, s] = await Promise.all([GetClientConfigs(), GetStatus().catch(() => null)])
      setConfigs(c || [])
      if (s) setStatus(s as any)
      await loadFile(activeType)
    } catch (err: any) {
      setFileError(String(err?.message || err))
    }
  }

  const handleReload = () => {
    setFileDirty(false)
    setMappingDirty(false)
    loadFile(activeType)
  }

  const handleSaveFile = async () => {
    if (!activeType) return
    setFileSaving(true)
    setFileError(null)
    try {
      if (fileDirty) {
        const cleaned = stripMappingPreview(fileContent, fileMeta?.format || '')
        await SaveClientConfigFile(activeType, cleaned)
        for (const extra of extraFiles) {
          await SaveAdditionalClientConfigFile(activeType, extra.path, extra.format, extra.content)
        }
        setFileDirty(false)
      }
      if (mappingDirty && pendingMapping) {
        const cur = await GetSettings()
        const merged = new account.Settings({
          ...(cur || {}),
          model_mapping: undefined,
          model_mappings: pendingMapping,
        } as any)
        await SaveSettings(merged)
        setSavedMappings(pendingMapping)
        setBridgeToken(cur?.bridge_token || QODER_API_KEY)
        setMappingDirty(false)
      }
      // 只刷新 configs（applied 状态），不重拉模型列表
      const [c, s] = await Promise.all([GetClientConfigs(), GetStatus().catch(() => null)])
      setConfigs(c || [])
      if (s) setStatus(s as any)
      await loadFile(activeType)
    } catch (err: any) {
      setFileError(String(err?.message || err))
    } finally {
      setFileSaving(false)
    }
  }

  const handleFormatFile = () => {
    if (!fileMeta?.format) return
    try {
      const formatted = formatContent(fileContent, fileMeta.format)
      if (formatted !== fileContent) {
        setFileContent(formatted)
        setFileDirty(true)
      }
      setFileError(null)
      setFormatStatus('done')
      setTimeout(() => setFormatStatus('idle'), 1500)
    } catch (err: any) {
      setFileError(`格式化失败: ${String(err?.message || err)}`)
      setFormatStatus('error')
      setTimeout(() => setFormatStatus('idle'), 1500)
    }
  }

  const handleMappingPatchPreview = useCallback((agentBucket: Record<string, string>) => {
    if (!fileMeta?.format) return
    const fmt = fileMeta.format
    setFileContent(prev => {
      const next = patchMappingPreview(prev, fmt, agentBucket, activeType)
      if (next !== prev) setFileDirty(true)
      return next
    })
  }, [fileMeta?.format, activeType])

  if (loading) return <div className="config-loading">加载中…</div>

  return (
    <div className="client-config-page">
      <div className="page-header">
        <h2>Agent 配置</h2>
      </div>

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

      {activeCfg && (
        <div className={`config-card ${activeCfg.applied ? 'applied' : ''}`}>
          <div className="config-card-header">
            {ICON_BY_TYPE[activeCfg.type]
              ? <img src={ICON_BY_TYPE[activeCfg.type]} alt="" className="config-card-icon" />
              : <div className="config-card-icon-emoji">{activeCfg.icon}</div>}
            <div>
              <h3>{activeCfg.name}</h3>
              <p>{activeCfg.applied ? '已配置（由 qccg 管理）' : '未配置'}</p>
            </div>
            <div className={`config-status-dot ${activeCfg.applied ? 'active' : ''}`} />
          </div>

          <div className="config-card-body">
            {activeCfg.error && (
              <div className="config-warning">
                <AlertTriangle size={15} style={{ flexShrink: 0, marginTop: 1 }} />
                <span>{activeCfg.error}</span>
              </div>
            )}

            <ModelMappingSection
              agent={activeCfg.type}
              qoderModels={qoderModels}
              initialMappings={savedMappings}
              modelsLoading={modelsLoading}
              modelsError={modelsError}
              onRefreshModels={refreshQoderModels}
              onMappingChange={(allMappings) => { setPendingMapping(allMappings); setMappingDirty(true) }}
              onMappingClean={() => setMappingDirty(false)}
              onPatchPreview={handleMappingPatchPreview}
            />

            <div className="config-section-divider">
              <span className="section-title">📝 配置文件</span>
              <code className="section-path">{activeCfg.config_path}</code>
              <span className="meta" style={!fileDirty && fileMeta?.existed ? { color: 'var(--bs-success, #198754)' } : undefined}>
                {fileMeta && !fileMeta.existed && '· 文件不存在'}
                {fileDirty ? ' · 未保存' : (fileMeta?.existed ? ' · 已保存' : '')}
              </span>
              <span className="divider-spacer" />
              {fileMeta?.format === 'json' && (
                <button
                  key={formatStatus}
                  className={`icon-btn icon-btn-format${formatStatus === 'error' ? ' icon-btn-danger' : ''}${formatStatus === 'done' ? ' btn-format-flash' : ''}`}
                  onClick={handleFormatFile}
                  disabled={fileLoading || !fileContent.trim() || formatStatus !== 'idle'}
                  title='格式化（JSON）'
                >
                  <WandIcon key={formatStatus} size={18} className={formatStatus === 'done' ? 'wand-icon-animate' : ''} />
                </button>
              )}
              <button
                className="icon-btn icon-btn-apply"
                onClick={() => handleApply(activeCfg)}
                title={activeCfg.applied ? '更新配置' : '一键配置'}
              ><Rocket size={18} /></button>
              <button
                className="icon-btn icon-btn-reload"
                onClick={handleReload}
                disabled={!fileDirty && !mappingDirty}
                title="丢弃未保存的编辑，从磁盘重新加载"
              ><FolderSync size={18} /></button>
              <button
                className="icon-btn btn-primary"
                onClick={handleSaveFile}
                disabled={(!fileDirty && !mappingDirty) || fileSaving}
                title="保存"
              ><Save size={18} /></button>
              <button
                className="icon-btn icon-btn-restore"
                onClick={handleRestore}
                disabled={!(activeCfg.applied && hasBackup)}
                title="还原到首次保存前的配置（恢复备份）"
              ><History size={18} /></button>
            </div>
            {fileError && (
              <div className="config-warning" style={{ marginTop: 0, marginBottom: 6 }}>
                <AlertTriangle size={15} style={{ flexShrink: 0, marginTop: 1 }} />
                <span>{fileError}</span>
              </div>
            )}
            <div style={{ marginTop: 10 }}>
              <ConfigEditor
                ref={editorRef}
                value={fileContent}
                onChange={v => { setFileContent(v); setFileDirty(true) }}
                format={fileMeta?.format as 'json' | 'toml' | 'dotenv' | undefined}
                placeholderText={fileLoading ? '加载中…' : '配置文件内容…'}
                minLines={16}
              />
            </div>

            {extraFiles.length > 0 && (
              <div style={{ marginTop: 16 }}>
                {extraFiles.map((extra, idx) => (
                  <div key={extra.path || idx} style={{ marginTop: idx === 0 ? 0 : 14 }}>
                    <div className="config-section-divider">
                      <span className="section-title">🧩 附加配置文件</span>
                      <code className="section-path">{extra.path}</code>
                      <span className="meta" style={extra.existed ? { color: 'var(--bs-success, #198754)' } : undefined}>
                        {extra.existed ? ' · 已加载' : ' · 文件不存在'}
                      </span>
                    </div>
                    <div style={{ marginTop: 8 }}>
                      <ConfigEditor
                        value={extra.content}
                        onChange={v => {
                          setExtraFiles(prev => prev.map((f, i) => i === idx ? { ...f, content: v } : f))
                          setFileDirty(true)
                        }}
                        format={extra.format as 'json' | 'toml' | 'dotenv' | undefined}
                        placeholderText={fileLoading ? '加载中…' : '附加配置文件内容…'}
                        minLines={8}
                      />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

// ============== ModelMappingSection ==============

type Row = { id: number; from: string; to: string }
let _rid = 1
const newRid = () => _rid++

interface MappingProps {
  agent: string
  qoderModels: main.QoderModel[]
  initialMappings: Record<string, Record<string, string>>
  modelsLoading: boolean
  modelsError: string | null
  onRefreshModels: () => Promise<main.QoderModel[]>
  onMappingChange: (allMappings: Record<string, Record<string, string>>) => void
  onMappingClean: () => void
  onPatchPreview: (agentBucket: Record<string, string>) => void
}

function ModelMappingSection({ agent, qoderModels, initialMappings, modelsLoading, modelsError, onRefreshModels, onMappingChange, onMappingClean, onPatchPreview }: MappingProps) {
  const [rows, setRows] = useState<Row[]>([])
  const [touched, setTouched] = useState(false)

  // agent 切换或 initialMappings 更新时重建 rows（无 IPC）
  useEffect(() => {
    let bucket = initialMappings[agent]
    if (!bucket && agent === 'claude') {
      // 兼容旧扁平映射（已在父组件加载时处理，这里不再重复拉取）
    }
    const list = bucket ? Object.entries(bucket).map(([from, to]) => ({ id: newRid(), from, to })) : []
    setRows(list)
    setTouched(false)
    const merged = { ...initialMappings }
    if (bucket && Object.keys(bucket).length > 0) merged[agent] = bucket
    else delete merged[agent]
    onMappingChange(merged)
    onMappingClean()
  }, [agent, initialMappings])

  useEffect(() => {
    if (!touched) return
    const next: Record<string, string> = {}
    for (const r of rows) {
      const k = r.from.trim(), v = r.to.trim()
      if (k && v) next[k] = v
    }
    const merged = { ...initialMappings }
    if (Object.keys(next).length === 0) delete merged[agent]
    else merged[agent] = next
    onMappingChange(merged)
    onPatchPreview(next)
  }, [rows, touched])

  const candidates = CLIENT_MODEL_OPTIONS_BY_AGENT[agent] || []
  const markTouched = () => { if (!touched) setTouched(true) }
  const addRow = () => { setRows(prev => [...prev, { id: newRid(), from: '', to: '' }]); markTouched() }
  const updateRow = (id: number, key: 'from' | 'to', v: string) => {
    setRows(prev => prev.map(r => r.id === id ? { ...r, [key]: v } : r)); markTouched()
  }
  const removeRow = (id: number) => { setRows(prev => prev.filter(r => r.id !== id)); markTouched() }
  const fillDefaults = () => {
    const existing = new Set(rows.map(r => r.from.trim()))
    const adds = (DEFAULT_MAPPING_BY_AGENT[agent] || [])
      .filter(([from]) => !existing.has(from))
      .map(([from, to]) => ({ id: newRid(), from, to }))
    if (adds.length > 0) { setRows(prev => [...prev, ...adds]); markTouched() }
  }

  return (
    <div className="mapping-section">
      <div className="mapping-section-header">
        <span className="section-title">🔀 模型映射</span>
        <span className="meta">客户端模型名 → Qoder model.key</span>
        <span className="divider-spacer" />
        <button className="btn btn-secondary btn-sm" onClick={fillDefaults} title="按家族关键字一键填充默认条目">默认</button>
        <button className="icon-btn icon-btn-apply" onClick={addRow} title="加一行" aria-label="加一行">
          <CirclePlus size={16} strokeWidth={2.2} />
        </button>
        <button className="icon-btn icon-btn-reload" onClick={onRefreshModels} disabled={modelsLoading} title="刷新上游模型列表">
          <RefreshCw size={16} className={modelsLoading ? 'spin' : ''} />
        </button>
      </div>
      {modelsError && <div className="mapping-hint-error">{modelsError}</div>}

      {rows.length === 0 ? (
        <div className="mapping-empty">未配置自定义映射，将使用内置默认表（{(DEFAULT_MAPPING_BY_AGENT[agent] || []).map(([f, t]) => `${f}→${t}`).join(' / ') || '无'}）</div>
      ) : (
        <div className="mapping-list">
          {rows.map(r => (
            <div key={r.id} className="mapping-row">
              <MappingSelect
                value={r.from}
                onChange={v => updateRow(r.id, 'from', v)}
                options={candidates.map(c => ({ value: c, label: c }))}
                placeholder="客户端模型名"
              />
              <svg className="mapping-arrow-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="14" height="14">
                <line x1="5" y1="12" x2="19" y2="12"/>
                <polyline points="12 5 19 12 12 19"/>
              </svg>
              <MappingSelect
                value={r.to}
                onChange={v => updateRow(r.id, 'to', v)}
                options={qoderModels.length > 0
                  ? [
                      ...qoderModels.map(m => ({ value: m.key, label: `${m.display_name} (${m.key})${m.is_default ? ' · 默认' : ''}` })),
                      ...(r.to && !qoderModels.some(m => m.key === r.to) ? [{ value: r.to, label: `${r.to}（自定义/已下线）` }] : [])
                    ]
                  : FALLBACK_QODER_KEYS.map(k => ({ value: k, label: k }))
                }
              />
              <button className="mapping-delete-btn" onClick={() => removeRow(r.id)} title="删除此行" aria-label="删除">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
                  <line x1="18" y1="6" x2="6" y2="18"/>
                  <line x1="6" y1="6" x2="18" y2="18"/>
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
