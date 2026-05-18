import React, { useRef, useEffect, useMemo, useImperativeHandle, forwardRef } from 'react'
import { EditorView, basicSetup } from 'codemirror'
import { json } from '@codemirror/lang-json'
import { EditorState } from '@codemirror/state'
import { placeholder } from '@codemirror/view'
import { linter, Diagnostic } from '@codemirror/lint'
import { StreamLanguage } from '@codemirror/language'
import { toml as tomlMode } from '@codemirror/legacy-modes/mode/toml'

export interface ConfigEditorHandle {
  formatContent: () => boolean
}

interface ConfigEditorProps {
  value: string
  onChange: (value: string) => void
  format?: 'json' | 'toml' | 'dotenv'
  placeholderText?: string
  minLines?: number
}

const baseTheme = EditorView.baseTheme({
  '&': {
    border: '1px solid var(--bs-border-color, #dee2e6)',
    borderRadius: '6px',
    fontSize: '13px',
    background: 'var(--bs-body-bg, #fff)',
  },
  '&.cm-focused': {
    outline: 'none',
    borderColor: '#86b7fe',
    boxShadow: '0 0 0 0.25rem rgba(13,110,253,.25)',
  },
  '.cm-scroller': {
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Courier New', monospace",
    overflow: 'auto',
  },
  '.cm-gutters': {
    background: 'var(--bs-tertiary-bg, #f8f9fa)',
    borderRight: '1px solid var(--bs-border-color, #dee2e6)',
    color: '#6c757d',
  },
  '.cm-activeLine': { background: 'rgba(13,110,253,.04)' },
  '.cm-activeLineGutter': { background: 'rgba(13,110,253,.04)' },
  '.cm-selectionBackground, .cm-content ::selection': {
    background: 'rgba(13,110,253,.15) !important',
  },
})

const ConfigEditor = forwardRef<ConfigEditorHandle, ConfigEditorProps>(({
  value,
  onChange,
  format = 'json',
  placeholderText = '',
  minLines = 8,
}, ref) => {
  const editorRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  useEffect(() => { onChangeRef.current = onChange }, [onChange])

  useImperativeHandle(ref, () => ({
    formatContent() {
      const view = viewRef.current
      if (!view || format !== 'json') return false
      const raw = view.state.doc.toString()
      if (!raw.trim()) return false
      try {
        let formatted = raw
        if (format === 'json') {
          formatted = JSON.stringify(JSON.parse(raw), null, 2) + '\n'
        }
        view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: formatted } })
        onChangeRef.current(formatted)
        return true
      } catch {
        return false
      }
    },
  }), [format])

  const syntaxLinter = useMemo(() => linter((view) => {
    const doc = view.state.doc.toString()
    if (!doc.trim()) return []
    const diagnostics: Diagnostic[] = []
    try {
      if (format === 'json') {
        JSON.parse(doc)
      } else if (format === 'toml') {
        return []
      }
    } catch (e) {
      diagnostics.push({
        from: 0,
        to: doc.length,
        severity: 'error',
        message: e instanceof Error ? e.message : `Invalid ${format.toUpperCase()}`,
      })
    }
    return diagnostics
  }), [format])

  const suppressRef = useRef(false)

  useEffect(() => {
    if (!editorRef.current) return
    const minHeightPx = minLines * 20
    const extensions = [
      basicSetup,
      ...(format === 'json' ? [json()] : []),
      ...(format === 'toml' ? [StreamLanguage.define(tomlMode)] : []),
      placeholder(placeholderText),
      baseTheme,
      EditorView.theme({ '&': { minHeight: `${minHeightPx}px` }, '.cm-scroller': { overflow: 'auto' } }),
      syntaxLinter,
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !suppressRef.current) onChangeRef.current(update.state.doc.toString())
      }),
    ]
    const state = EditorState.create({ doc: value, extensions })
    const view = new EditorView({ state, parent: editorRef.current })
    viewRef.current = view
    return () => { view.destroy(); viewRef.current = null }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [format, minLines, syntaxLinter])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    if (view.state.doc.toString() === value) return
    suppressRef.current = true
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })
    suppressRef.current = false
  }, [value])

  return <div ref={editorRef} style={{ width: '100%' }} />
})

ConfigEditor.displayName = 'ConfigEditor'
export default ConfigEditor
