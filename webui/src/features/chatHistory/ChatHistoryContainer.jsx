import { ArrowDown, ArrowUp, Bot, ChevronDown, Clock3, Loader2, MessageSquareText, RefreshCcw, Sparkles, Trash2, UserRound, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import clsx from 'clsx'

import { useI18n } from '../../i18n'

const LIMIT_OPTIONS = [0, 10, 20, 50]
const DISABLED_LIMIT = 0
const MESSAGE_COLLAPSE_AT = 700
const VIEW_MODE_KEY = 'ds2api_chat_history_view_mode'

function formatDateTime(value, lang) {
    if (!value) return '-'
    try {
        return new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
        }).format(new Date(value))
    } catch {
        return '-'
    }
}

function formatElapsed(ms, t) {
    if (!ms) return t('chatHistory.metaUnknown')
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(ms < 10_000 ? 2 : 1)}s`
}

function formatBytes(bytes, t) {
    if (!bytes) return t('chatHistory.metaUnknown')
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(bytes < 10 * 1024 ? 2 : 1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

function formatDiagReason(reason, t) {
    if (!reason) return t('chatHistory.metaUnknown')
    return t(`chatHistory.diagReasonValues.${reason}`)
}

function previewText(item) {
    return item?.preview || item?.content || item?.reasoning_content || item?.error || item?.user_input || ''
}

function statusTone(status) {
    switch (status) {
        case 'success':
            return 'border-emerald-500/20 bg-emerald-500/10 text-emerald-600'
        case 'error':
            return 'border-destructive/20 bg-destructive/10 text-destructive'
        case 'stopped':
            return 'border-amber-500/20 bg-amber-500/10 text-amber-600'
        default:
            return 'border-border bg-secondary/60 text-muted-foreground'
    }
}

function ExpandableText({ text = '', threshold = MESSAGE_COLLAPSE_AT, expandLabel, collapseLabel, buttonClassName = 'text-white hover:text-white/80' }) {
    const shouldCollapse = text.length > threshold
    const [expanded, setExpanded] = useState(false)
    const contentRef = useRef(null)
    const [maxHeight, setMaxHeight] = useState('none')

    useEffect(() => {
        setExpanded(false)
    }, [text])

    const visibleText = shouldCollapse && !expanded ? `${text.slice(0, threshold)}...` : text

    useEffect(() => {
        if (!contentRef.current) return
        setMaxHeight(`${contentRef.current.scrollHeight}px`)
    }, [expanded, visibleText])

    return (
        <div>
            <div
                className="overflow-hidden transition-[max-height] duration-300 ease-out"
                style={{ maxHeight }}
            >
                <div ref={contentRef} className="whitespace-pre-wrap break-words">
                    {visibleText}
                </div>
            </div>
            {shouldCollapse && (
                <button
                    type="button"
                    onClick={() => setExpanded(prev => !prev)}
                    className={clsx('mt-3 inline-flex items-center gap-2 text-xs font-medium transition-colors', buttonClassName)}
                >
                    <ChevronDown className={clsx('w-3.5 h-3.5 transition-transform duration-300', expanded && 'rotate-180')} />
                    {expanded ? collapseLabel : expandLabel}
                </button>
            )}
        </div>
    )
}

function ListModeIcon() {
    return (
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16" aria-hidden="true">
            <path d="M3 0h10a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V2a2 2 0 0 1 2-2m0 1a1 1 0 0 0-1 1v3a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V2a1 1 0 0 0-1-1zm0 8h10a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2m0 1a1 1 0 0 0-1 1v3a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1v-3a1 1 0 0 0-1-1z" />
        </svg>
    )
}

function MergeModeIcon() {
    return (
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16" aria-hidden="true">
            <path d="M4 0a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V2a2 2 0 0 0-2-2zm0 1h8a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V2a1 1 0 0 1 1-1" />
        </svg>
    )
}

function RequestMessages({ item, t }) {
    const messages = Array.isArray(item?.messages) && item.messages.length > 0
        ? item.messages
        : [{ role: 'user', content: item?.user_input || t('chatHistory.emptyUserInput') }]

    return (
        <div className="space-y-5 max-w-4xl mx-auto">
            {messages.map((message, index) => {
                const role = message.role || 'user'
                const isUser = role === 'user'
                const isAssistant = role === 'assistant'
                const isTool = role === 'tool'
                const label = isUser
                    ? t('chatHistory.role.user')
                    : (isAssistant ? t('chatHistory.role.assistant') : (isTool ? t('chatHistory.role.tool') : t('chatHistory.role.system')))
                return (
                    <div key={`${role}-${index}`} className="flex gap-4">
                        <div className={clsx(
                            'w-8 h-8 rounded-lg flex items-center justify-center shrink-0 border border-border',
                            isUser
                                ? 'bg-secondary'
                                : (isAssistant ? 'bg-muted' : 'bg-background')
                        )}>
                            {isUser
                                ? <UserRound className="w-4 h-4 text-muted-foreground" />
                                : <Bot className="w-4 h-4 text-foreground" />}
                        </div>
                        <div className="max-w-[88%] lg:max-w-[78%] text-left">
                            <div className="text-[11px] uppercase tracking-[0.12em] text-muted-foreground mb-2 px-1">
                                {label}
                            </div>
                            <div className={clsx(
                                'rounded-2xl px-5 py-3 text-sm leading-relaxed shadow-sm border whitespace-pre-wrap break-words',
                                isUser
                                    ? 'bg-primary text-primary-foreground rounded-tr-sm border-primary/30'
                                    : (isAssistant
                                        ? 'bg-secondary/60 text-foreground rounded-tl-sm border-border'
                                        : 'bg-background text-foreground rounded-tl-sm border-border')
                            )}>
                                <div className="whitespace-pre-wrap break-words">
                                    {message.content || t('chatHistory.emptyUserInput')}
                                </div>
                            </div>
                        </div>
                    </div>
                )
            })}
        </div>
    )
}

function MergedPromptView({ item, t }) {
    const merged = item?.final_prompt || ''
    return (
        <div
            className="max-w-4xl mx-auto rounded-2xl border px-5 py-4"
            style={{
                backgroundColor: 'rgb(231, 176, 8)',
                borderColor: 'rgba(231, 176, 8, 0.45)',
            }}
        >
            <div className="text-[11px] uppercase tracking-[0.12em] text-[#5b4300] mb-3">
                {t('chatHistory.mergedInput')}
            </div>
            <div className="text-sm leading-7 text-[#2f2200] whitespace-pre-wrap break-words font-mono">
                <ExpandableText
                    text={merged || t('chatHistory.emptyMergedPrompt')}
                    expandLabel={t('chatHistory.expand')}
                    collapseLabel={t('chatHistory.collapse')}
                    buttonClassName="text-[#2f2200] hover:text-black"
                />
            </div>
        </div>
    )
}

function HistoryTextView({ item, t }) {
    const historyText = (item?.history_text || '').trim()
    if (!historyText) return null

    return (
        <div className="max-w-4xl mx-auto rounded-2xl border border-border bg-background px-5 py-4">
            <div className="text-[11px] uppercase tracking-[0.12em] text-muted-foreground mb-3 text-left">
                HISTORY
            </div>
            <div className="text-sm leading-7 text-foreground whitespace-pre-wrap break-words font-mono">
                <ExpandableText
                    text={historyText}
                    threshold={Math.floor(MESSAGE_COLLAPSE_AT / 4)}
                    expandLabel={t('chatHistory.expand')}
                    collapseLabel={t('chatHistory.collapse')}
                    buttonClassName="text-foreground hover:text-muted-foreground"
                />
            </div>
        </div>
    )
}

function UploadDiagnosticsView({ item, t }) {
    const diagnostics = item?.diagnostics
    const current = diagnostics?.current_input_upload
    const history = diagnostics?.history_upload
    const currentReason = diagnostics?.current_input_reason || ''
    const historyReason = diagnostics?.history_reason || ''
    const refFileIDs = Array.isArray(diagnostics?.ref_file_ids) ? diagnostics.ref_file_ids : []
    const hasRecordedDiagnostics = !!(current || history || refFileIDs.length)

    const renderUpload = (title, upload, reason, accentClass) => (
        <div className={clsx('rounded-xl border px-4 py-4', accentClass)}>
            <div className="flex items-center justify-between gap-3">
                <div className="text-sm font-semibold text-foreground">{title}</div>
                <span className={clsx(
                    'px-2 py-1 rounded-full border text-[10px] font-semibold uppercase tracking-wide',
                    upload
                        ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-600'
                        : 'border-border bg-background/70 text-muted-foreground'
                )}>
                    {upload ? t('chatHistory.diagUploaded') : t('chatHistory.diagNotUploaded')}
                </span>
            </div>
            <div className="mt-3 space-y-2 text-sm">
                <div className="rounded-lg bg-background/75 px-3 py-2 border border-border/70">
                    <div className="text-[11px] text-muted-foreground">{t('chatHistory.diagFilename')}</div>
                    <div className="font-mono break-all text-foreground">{upload?.filename || '-'}</div>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
                    <div className="rounded-lg bg-background/75 px-3 py-2 border border-border/70">
                        <div className="text-[11px] text-muted-foreground">{t('chatHistory.diagBytes')}</div>
                        <div className="font-medium text-foreground">{formatBytes(upload?.bytes, t)}</div>
                    </div>
                    <div className="rounded-lg bg-background/75 px-3 py-2 border border-border/70">
                        <div className="text-[11px] text-muted-foreground">{t('chatHistory.diagFileID')}</div>
                        <div className="font-mono break-all text-foreground">{upload?.file_id || '-'}</div>
                    </div>
                </div>
                <div className="rounded-lg bg-background/75 px-3 py-2 border border-border/70">
                    <div className="text-[11px] text-muted-foreground">{t('chatHistory.diagReason')}</div>
                    <div className="text-foreground">{formatDiagReason(reason, t)}</div>
                </div>
            </div>
        </div>
    )

    return (
        <div className="max-w-4xl mx-auto rounded-2xl border border-amber-500/35 bg-[rgba(255,243,205,0.92)] px-5 py-4 shadow-sm">
            <div className="flex items-start justify-between gap-4">
                <div>
                    <div className="text-[11px] uppercase tracking-[0.12em] text-[#7a5200]">{t('chatHistory.diagnosticsTitle')}</div>
                    <div className="mt-2 text-sm leading-6 text-[#5d3d00]">{t('chatHistory.diagnosticsDesc')}</div>
                    {!hasRecordedDiagnostics && (
                        <div className="mt-2 text-xs leading-5 text-[#7a5200]">
                            {t('chatHistory.diagUnavailable')}
                        </div>
                    )}
                </div>
                <div className="shrink-0 rounded-xl border border-amber-600/20 bg-white/60 px-3 py-2 text-right">
                    <div className="text-[11px] text-[#7a5200]">{t('chatHistory.diagRefFileIDs')}</div>
                    <div className="text-base font-semibold text-[#2f2200]">{refFileIDs.length || 0}</div>
                </div>
            </div>
            <div className="mt-4 grid grid-cols-1 xl:grid-cols-2 gap-4">
                {renderUpload(t('chatHistory.diagCurrentInput'), current, currentReason, 'border-amber-600/25 bg-white/55')}
                {renderUpload(t('chatHistory.diagHistory'), history, historyReason, 'border-amber-600/25 bg-white/55')}
            </div>
            <div className="mt-4 rounded-xl border border-amber-600/20 bg-white/55 px-4 py-3">
                <div className="text-[11px] uppercase tracking-[0.12em] text-[#7a5200]">{t('chatHistory.diagRefFileIDs')}</div>
                <div className="mt-2 text-sm text-[#2f2200]">
                    {refFileIDs.length
                        ? refFileIDs.map((id, index) => (
                            <div key={`${id}-${index}`} className="font-mono break-all">{id}</div>
                        ))
                        : t('chatHistory.diagNone')}
                </div>
            </div>
        </div>
    )
}

function DetailConversation({ selectedItem, t, viewMode, detailScrollRef, assistantStartRef, bottomButtonClassName }) {
    if (!selectedItem) return null

    return (
        <>
            <UploadDiagnosticsView item={selectedItem} t={t} />

            <HistoryTextView item={selectedItem} t={t} />

            {viewMode === 'list'
                ? <RequestMessages item={selectedItem} t={t} />
                : <MergedPromptView item={selectedItem} t={t} />}

            <div ref={assistantStartRef} className="flex gap-4 max-w-4xl mx-auto">
                <div className={clsx(
                    'w-8 h-8 rounded-lg flex items-center justify-center shrink-0 border border-border',
                    selectedItem.status === 'error' ? 'bg-destructive/10 border-destructive/20' : 'bg-muted'
                )}>
                    <Bot className={clsx('w-4 h-4', selectedItem.status === 'error' ? 'text-destructive' : 'text-foreground')} />
                </div>
                <div className="space-y-4 flex-1 min-w-0">
                    {(selectedItem.reasoning_content || '').trim() && (
                        <div className="text-xs bg-secondary/50 border border-border rounded-lg p-3 space-y-1.5">
                            <div className="flex items-center gap-1.5 text-muted-foreground">
                                <Sparkles className="w-3.5 h-3.5" />
                                <span className="font-medium">{t('chatHistory.reasoningTrace')}</span>
                            </div>
                            <div className="whitespace-pre-wrap leading-relaxed text-muted-foreground font-mono text-[12px] md:text-[13px] max-h-64 overflow-y-auto custom-scrollbar pl-5 border-l-2 border-border/50 break-words">
                                {selectedItem.reasoning_content}
                            </div>
                        </div>
                    )}

                    <div className="text-sm leading-7 text-foreground whitespace-pre-wrap break-words">
                        {selectedItem.status === 'error'
                            ? <span className="text-destructive font-medium">{selectedItem.error || t('chatHistory.failedOutput')}</span>
                            : (selectedItem.content || t('chatHistory.emptyAssistantOutput'))}
                    </div>
                </div>
            </div>

            <div className="max-w-4xl mx-auto rounded-xl border border-border bg-background/70 p-4 space-y-3">
                <div className="text-xs font-semibold uppercase tracking-[0.12em] text-muted-foreground">{t('chatHistory.metaTitle')}</div>
                <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
                    <div className="rounded-lg border border-border bg-card px-3 py-2">
                        <div className="text-[11px] text-muted-foreground">{t('chatHistory.metaAccount')}</div>
                        <div className="text-sm font-medium text-foreground">{selectedItem.account_id || t('chatHistory.metaUnknown')}</div>
                    </div>
                    <div className="rounded-lg border border-border bg-card px-3 py-2">
                        <div className="text-[11px] text-muted-foreground">{t('chatHistory.metaElapsed')}</div>
                        <div className="text-sm font-medium text-foreground flex items-center gap-2">
                            <Clock3 className="w-3.5 h-3.5 text-muted-foreground" />
                            {formatElapsed(selectedItem.elapsed_ms, t)}
                        </div>
                    </div>
                    <div className="rounded-lg border border-border bg-card px-3 py-2">
                        <div className="text-[11px] text-muted-foreground">{t('chatHistory.metaModel')}</div>
                        <div className="text-sm font-medium text-foreground break-all">{selectedItem.model || t('chatHistory.metaUnknown')}</div>
                    </div>
                    <div className="rounded-lg border border-border bg-card px-3 py-2">
                        <div className="text-[11px] text-muted-foreground">{t('chatHistory.metaStatusCode')}</div>
                        <div className="text-sm font-medium text-foreground">{selectedItem.status_code || '-'}</div>
                    </div>
                    <div className="rounded-lg border border-border bg-card px-3 py-2">
                        <div className="text-[11px] text-muted-foreground">{t('chatHistory.metaStream')}</div>
                        <div className="text-sm font-medium text-foreground">{selectedItem.stream ? t('chatHistory.streamMode') : t('chatHistory.nonStreamMode')}</div>
                    </div>
                    <div className="rounded-lg border border-border bg-card px-3 py-2">
                        <div className="text-[11px] text-muted-foreground">{t('chatHistory.metaCaller')}</div>
                        <div className="text-sm font-medium text-foreground break-all">{selectedItem.caller_id || t('chatHistory.metaUnknown')}</div>
                    </div>
                </div>
            </div>

            <button
                type="button"
                onClick={() => detailScrollRef.current?.scrollTo({ top: detailScrollRef.current?.scrollHeight || 0, behavior: 'smooth' })}
                className={clsx('h-12 w-12 rounded-full border border-border bg-card/95 backdrop-blur shadow-lg text-muted-foreground hover:text-foreground hover:bg-secondary/90 flex items-center justify-center', bottomButtonClassName)}
                title={t('chatHistory.backToBottom')}
            >
                <ArrowDown className="w-5 h-5" />
            </button>
        </>
    )
}

export default function ChatHistoryContainer({ authFetch, onMessage }) {
    const { t, lang } = useI18n()
    const apiFetch = authFetch || fetch
    const [items, setItems] = useState([])
    const [limit, setLimit] = useState(20)
    const [loading, setLoading] = useState(true)
    const [refreshing, setRefreshing] = useState(false)
    const [selectedId, setSelectedId] = useState('')
    const [selectedDetail, setSelectedDetail] = useState(null)
    const [savingLimit, setSavingLimit] = useState(false)
    const [clearing, setClearing] = useState(false)
    const [deletingId, setDeletingId] = useState('')
    const [detail, setDetail] = useState('')
    const [confirmClearOpen, setConfirmClearOpen] = useState(false)
    const [autoRefreshReady, setAutoRefreshReady] = useState(false)
    const [viewMode, setViewMode] = useState(() => {
        if (typeof localStorage === 'undefined') return 'list'
        const stored = localStorage.getItem(VIEW_MODE_KEY)
        return stored === 'merged' ? 'merged' : 'list'
    })
    const [isMobileView, setIsMobileView] = useState(() => typeof window !== 'undefined' ? window.innerWidth < 1024 : false)
    const [mobileDetailOpen, setMobileDetailOpen] = useState(false)
    const [mobileDetailVisible, setMobileDetailVisible] = useState(false)
    const [mobileOrigin, setMobileOrigin] = useState({ x: 50, y: 50 })
    const [pendingJumpToAssistant, setPendingJumpToAssistant] = useState(false)

    const inFlightRef = useRef(false)
    const detailInFlightRef = useRef(false)
    const listETagRef = useRef('')
    const detailETagRef = useRef('')
    const assistantStartRef = useRef(null)
    const detailScrollRef = useRef(null)
    const mobileCloseTimerRef = useRef(null)

    const selectedSummary = items.find(item => item.id === selectedId) || items[0] || null
    const selectedItem = selectedDetail && selectedDetail.id === selectedId ? selectedDetail : null

    const syncItems = (nextItems) => {
        setItems(nextItems)
        setSelectedId(prev => {
            if (!nextItems.length) return ''
            if (prev && nextItems.some(item => item.id === prev)) return prev
            return nextItems[0].id
        })
    }

    const loadList = async ({ mode = 'silent', announceError = false } = {}) => {
        if (inFlightRef.current) return
        inFlightRef.current = true
        if (mode === 'manual') {
            setRefreshing(true)
        } else if (mode === 'initial') {
            setLoading(true)
        }
        if (announceError) {
            setDetail('')
        }
        try {
            const headers = {}
            if (listETagRef.current) {
                headers['If-None-Match'] = listETagRef.current
            }
            const res = await apiFetch('/admin/chat-history', { headers })
            if (res.status === 304) {
                return
            }
            const data = await res.json()
            if (!res.ok) {
                throw new Error(data?.detail || t('chatHistory.loadFailed'))
            }
            listETagRef.current = res.headers.get('ETag') || ''
            setLimit(typeof data.limit === 'number' ? data.limit : 20)
            syncItems(Array.isArray(data.items) ? data.items : [])
        } catch (error) {
            setDetail(error.message || t('chatHistory.loadFailed'))
            if (announceError) {
                onMessage?.('error', error.message || t('chatHistory.loadFailed'))
            }
        } finally {
            if (mode === 'initial') {
                setLoading(false)
            }
            if (mode === 'manual') {
                setRefreshing(false)
            }
            inFlightRef.current = false
        }
    }

    const loadDetail = async (id, { announceError = false } = {}) => {
        if (!id || detailInFlightRef.current) return
        detailInFlightRef.current = true
        try {
            const headers = {}
            if (detailETagRef.current) {
                headers['If-None-Match'] = detailETagRef.current
            }
            const res = await apiFetch(`/admin/chat-history/${encodeURIComponent(id)}`, { headers })
            if (res.status === 304) {
                return
            }
            const data = await res.json()
            if (!res.ok) {
                throw new Error(data?.detail || t('chatHistory.loadFailed'))
            }
            detailETagRef.current = res.headers.get('ETag') || ''
            setSelectedDetail(data.item || null)
        } catch (error) {
            if (announceError) {
                onMessage?.('error', error.message || t('chatHistory.loadFailed'))
            }
        } finally {
            detailInFlightRef.current = false
        }
    }

    useEffect(() => {
        loadList({ mode: 'initial', announceError: true }).finally(() => {
            setAutoRefreshReady(true)
        })
    }, [])

    useEffect(() => {
        if (!autoRefreshReady) return undefined
        const timer = window.setInterval(() => {
            loadList({ mode: 'silent', announceError: false })
        }, 5000)
        return () => window.clearInterval(timer)
    }, [autoRefreshReady])

    useEffect(() => {
        if (!autoRefreshReady || !selectedId || selectedSummary?.status !== 'streaming') return undefined
        const timer = window.setInterval(() => {
            loadDetail(selectedId, { announceError: false })
        }, 1000)
        return () => window.clearInterval(timer)
    }, [autoRefreshReady, selectedId, selectedSummary?.status])

    useEffect(() => {
        if (!selectedId) return undefined
        detailETagRef.current = ''
        setSelectedDetail(null)
        loadDetail(selectedId, { announceError: false })
    }, [selectedId, mobileDetailOpen])

    useEffect(() => {
        if (!pendingJumpToAssistant || !selectedItem || selectedItem.id !== selectedId) return undefined
        const frame = window.requestAnimationFrame(() => {
            assistantStartRef.current?.scrollIntoView({ behavior: 'auto', block: 'start' })
            setPendingJumpToAssistant(false)
        })
        return () => window.cancelAnimationFrame(frame)
    }, [pendingJumpToAssistant, selectedId, selectedItem?.id, selectedItem?.revision, mobileDetailOpen, viewMode])

    useEffect(() => {
        if (typeof localStorage === 'undefined') return
        localStorage.setItem(VIEW_MODE_KEY, viewMode)
    }, [viewMode])

    useEffect(() => {
        if (typeof window === 'undefined') return undefined
        const handleResize = () => setIsMobileView(window.innerWidth < 1024)
        handleResize()
        window.addEventListener('resize', handleResize)
        return () => window.removeEventListener('resize', handleResize)
    }, [])

    useEffect(() => {
        if (!isMobileView) {
            setMobileDetailOpen(false)
            setMobileDetailVisible(false)
        }
    }, [isMobileView])

    useEffect(() => {
        return () => {
            if (mobileCloseTimerRef.current) {
                window.clearTimeout(mobileCloseTimerRef.current)
            }
        }
    }, [])

    const handleRefresh = async ({ manual = true } = {}) => {
        await loadList({ mode: manual ? 'manual' : 'silent', announceError: manual })
        if (selectedId) {
            detailETagRef.current = ''
            await loadDetail(selectedId, { announceError: manual })
        }
    }

    const handleLimitChange = async (nextLimit) => {
        if (nextLimit === limit || savingLimit) return
        setSavingLimit(true)
        try {
            const res = await apiFetch('/admin/chat-history/settings', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ limit: nextLimit }),
            })
            const data = await res.json()
            if (!res.ok) {
                throw new Error(data?.detail || t('chatHistory.updateLimitFailed'))
            }
            const resolvedLimit = typeof data.limit === 'number' ? data.limit : nextLimit
            setLimit(resolvedLimit)
            listETagRef.current = ''
            syncItems(Array.isArray(data.items) ? data.items : [])
            onMessage?.('success', t('chatHistory.limitUpdated', { limit: resolvedLimit === DISABLED_LIMIT ? t('chatHistory.off') : resolvedLimit }))
        } catch (error) {
            onMessage?.('error', error.message || t('chatHistory.updateLimitFailed'))
        } finally {
            setSavingLimit(false)
        }
    }

    const handleDeleteItem = async (id) => {
        if (!id || deletingId) return
        setDeletingId(id)
        try {
            const res = await apiFetch(`/admin/chat-history/${encodeURIComponent(id)}`, { method: 'DELETE' })
            const data = await res.json()
            if (!res.ok) {
                throw new Error(data?.detail || t('chatHistory.deleteFailed'))
            }
            if (selectedId === id) {
                detailETagRef.current = ''
                setSelectedDetail(null)
            }
            syncItems(items.filter(item => item.id !== id))
            onMessage?.('success', t('chatHistory.deleteSuccess'))
        } catch (error) {
            onMessage?.('error', error.message || t('chatHistory.deleteFailed'))
        } finally {
            setDeletingId('')
        }
    }

    const handleClear = async () => {
        if (clearing || !items.length) return
        setClearing(true)
        try {
            const res = await apiFetch('/admin/chat-history', { method: 'DELETE' })
            const data = await res.json()
            if (!res.ok) {
                throw new Error(data?.detail || t('chatHistory.clearFailed'))
            }
            listETagRef.current = ''
            detailETagRef.current = ''
            setSelectedDetail(null)
            syncItems([])
            onMessage?.('success', t('chatHistory.clearSuccess'))
        } catch (error) {
            onMessage?.('error', error.message || t('chatHistory.clearFailed'))
        } finally {
            setClearing(false)
        }
    }

    const openMobileDetail = (itemId, event) => {
        const x = typeof window !== 'undefined' && event?.clientX ? (event.clientX / window.innerWidth) * 100 : 50
        const y = typeof window !== 'undefined' && event?.clientY ? (event.clientY / window.innerHeight) * 100 : 50
        setMobileOrigin({ x, y })
        setPendingJumpToAssistant(true)
        setSelectedId(itemId)
        setMobileDetailOpen(true)
        setMobileDetailVisible(false)
        window.requestAnimationFrame(() => {
            window.requestAnimationFrame(() => setMobileDetailVisible(true))
        })
    }

    const closeMobileDetail = () => {
        setMobileDetailVisible(false)
        if (mobileCloseTimerRef.current) {
            window.clearTimeout(mobileCloseTimerRef.current)
        }
        mobileCloseTimerRef.current = window.setTimeout(() => {
            setMobileDetailOpen(false)
        }, 180)
    }

    const handleSelectItem = (itemId, event) => {
        if (isMobileView) {
            openMobileDetail(itemId, event)
            return
        }
        setPendingJumpToAssistant(true)
        setSelectedId(itemId)
    }

    if (loading) {
        return (
            <div className="h-[calc(100vh-140px)] rounded-2xl border border-border bg-card shadow-sm flex items-center justify-center">
                <div className="flex items-center gap-3 text-sm text-muted-foreground">
                    <Loader2 className="w-4 h-4 animate-spin" />
                    {t('chatHistory.loading')}
                </div>
            </div>
        )
    }

    return (
        <div className="space-y-6">
            <div className="rounded-2xl border border-border bg-card shadow-sm p-4 lg:p-5 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div>
                    <div className="text-sm font-semibold text-foreground">{t('chatHistory.retentionTitle')}</div>
                    <div className="text-xs text-muted-foreground mt-1">{t('chatHistory.retentionDesc')}</div>
                </div>
                <div className="flex flex-wrap gap-2 items-center">
                    {LIMIT_OPTIONS.map(option => (
                        <button
                            key={option}
                            type="button"
                            disabled={savingLimit}
                            onClick={() => handleLimitChange(option)}
                            className={clsx(
                                'h-9 px-3 rounded-lg border text-sm transition-colors',
                                option === limit
                                    ? (option === DISABLED_LIMIT
                                        ? 'border-destructive bg-destructive text-destructive-foreground'
                                        : 'border-primary bg-primary text-primary-foreground')
                                    : 'border-border bg-background text-muted-foreground hover:text-foreground hover:bg-secondary/70'
                            )}
                        >
                            {option === DISABLED_LIMIT ? t('chatHistory.off') : option}
                        </button>
                    ))}
                    <button
                        type="button"
                        onClick={() => handleRefresh({ manual: true })}
                        disabled={refreshing}
                        className={clsx(
                            'h-9 rounded-lg border border-border bg-background text-muted-foreground hover:text-foreground hover:bg-secondary/70 flex items-center',
                            isMobileView ? 'w-9 justify-center px-0' : 'gap-2 px-3'
                        )}
                    >
                        {refreshing ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCcw className="w-4 h-4" />}
                        {!isMobileView && t('chatHistory.refresh')}
                    </button>
                    <button
                        type="button"
                        onClick={() => setConfirmClearOpen(true)}
                        disabled={clearing || !items.length}
                        className="h-10 w-10 rounded-xl border border-border bg-[#111214] text-muted-foreground hover:text-destructive hover:bg-[#181a1d] disabled:opacity-50 flex items-center justify-center"
                        title={t('chatHistory.clearAll')}
                    >
                        {clearing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Trash2 className="w-4 h-4" />}
                    </button>
                </div>
            </div>

            {detail && (
                <div className="rounded-xl border border-destructive/20 bg-destructive/10 text-destructive px-4 py-3 text-sm">
                    {detail}
                </div>
            )}

            <div className="grid grid-cols-1 lg:grid-cols-[340px,minmax(0,1fr)] gap-6 h-[calc(100vh-240px)] min-h-[520px]">
                <div className="rounded-2xl border border-border bg-card shadow-sm min-h-0 overflow-hidden flex flex-col">
                    <div className="px-4 py-3 border-b border-border flex items-center justify-between">
                        <div className="text-sm font-semibold">{t('chatHistory.listTitle')}</div>
                        <div className="text-xs text-muted-foreground">{items.length}</div>
                    </div>
                    <div className="flex-1 overflow-y-auto p-3 space-y-3">
                        {!items.length && (
                            <div className="h-full rounded-xl border border-dashed border-border/80 bg-background/50 flex flex-col items-center justify-center gap-2 text-center px-6">
                                <MessageSquareText className="w-8 h-8 text-muted-foreground/50" />
                                <div className="text-sm font-medium text-foreground">{t('chatHistory.emptyTitle')}</div>
                                <div className="text-xs text-muted-foreground leading-6">{t('chatHistory.emptyDesc')}</div>
                            </div>
                        )}

                        {items.map(item => (
                            <button
                                key={item.id}
                                type="button"
                                onClick={(event) => handleSelectItem(item.id, event)}
                                className={clsx(
                                    'w-full text-left rounded-xl border px-4 py-3 transition-colors',
                                    selectedItem?.id === item.id
                                        ? 'border-primary/40 bg-primary/5'
                                        : 'border-border hover:bg-secondary/40'
                                )}
                            >
                                <div className="flex items-start justify-between gap-3">
                                    <div className="min-w-0">
                                        <div className="text-sm font-semibold text-foreground truncate">
                                            {item.user_input || t('chatHistory.untitled')}
                                        </div>
                                        <div className="text-[11px] text-muted-foreground mt-1 truncate">
                                            {item.model || '-'}
                                        </div>
                                    </div>
                                    <div className="flex items-center gap-2 shrink-0">
                                        <span className={clsx('px-2 py-0.5 rounded-full border text-[10px] font-semibold uppercase tracking-wide', statusTone(item.status))}>
                                            {t(`chatHistory.status.${item.status || 'streaming'}`)}
                                        </span>
                                        <button
                                            type="button"
                                            onClick={(event) => {
                                                event.stopPropagation()
                                                handleDeleteItem(item.id)
                                            }}
                                            disabled={deletingId === item.id}
                                            className="p-1.5 rounded-md text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                                        >
                                            {deletingId === item.id ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Trash2 className="w-3.5 h-3.5" />}
                                        </button>
                                    </div>
                                </div>
                                <div className="text-xs text-muted-foreground mt-3 line-clamp-2 whitespace-pre-wrap break-words">
                                    {previewText(item) || t('chatHistory.noPreview')}
                                </div>
                                <div className="text-[11px] text-muted-foreground/80 mt-3">
                                    {formatDateTime(item.completed_at || item.updated_at || item.created_at, lang)}
                                </div>
                            </button>
                        ))}
                    </div>
                </div>

                <div className="hidden lg:flex rounded-2xl border border-border bg-card shadow-sm min-h-0 overflow-hidden flex-col relative">
                    <div className="px-5 py-4 border-b border-border flex items-center justify-between gap-3">
                        <div>
                            <div className="text-sm font-semibold text-foreground">{t('chatHistory.detailTitle')}</div>
                            <div className="text-xs text-muted-foreground mt-1">
                                {selectedSummary ? formatDateTime(selectedSummary.completed_at || selectedSummary.updated_at || selectedSummary.created_at, lang) : t('chatHistory.selectPrompt')}
                            </div>
                        </div>
                        <div className="flex items-center gap-2">
                            <div className="inline-flex items-center rounded-xl border border-border bg-background p-1">
                                <button
                                    type="button"
                                    onClick={() => setViewMode('list')}
                                    className={clsx(
                                        'h-9 w-12 rounded-lg flex items-center justify-center transition-colors',
                                        viewMode === 'list'
                                            ? 'bg-secondary text-foreground'
                                            : 'text-muted-foreground hover:text-foreground hover:bg-secondary/60'
                                    )}
                                    title={t('chatHistory.viewModeList')}
                                >
                                    <ListModeIcon />
                                </button>
                                <button
                                    type="button"
                                    onClick={() => setViewMode('merged')}
                                    className={clsx(
                                        'h-9 w-12 rounded-lg flex items-center justify-center transition-colors',
                                        viewMode === 'merged'
                                            ? 'bg-secondary text-foreground'
                                            : 'text-muted-foreground hover:text-foreground hover:bg-secondary/60'
                                    )}
                                    title={t('chatHistory.viewModeMerged')}
                                >
                                    <MergeModeIcon />
                                </button>
                            </div>
                            <button
                                type="button"
                                onClick={() => detailScrollRef.current?.scrollTo({ top: 0, behavior: 'smooth' })}
                                className="h-8 w-8 rounded-lg border border-border bg-background text-muted-foreground hover:text-foreground hover:bg-secondary/70 flex items-center justify-center"
                                title={t('chatHistory.backToTop')}
                            >
                                <ArrowUp className="w-4 h-4" />
                            </button>
                            {selectedSummary && (
                                <span className={clsx('px-2.5 py-1 rounded-full border text-[10px] font-semibold uppercase tracking-wide', statusTone(selectedSummary.status))}>
                                    {t(`chatHistory.status.${selectedSummary.status || 'streaming'}`)}
                                </span>
                            )}
                        </div>
                    </div>

                    <div ref={detailScrollRef} className="flex-1 overflow-y-auto p-5 lg:p-6 space-y-6">
                        {!selectedItem && (
                            <div className="h-full rounded-xl border border-dashed border-border/80 bg-background/50 flex items-center justify-center text-sm text-muted-foreground">
                                {t('chatHistory.selectPrompt')}
                            </div>
                        )}

                        {selectedItem && (
                            <DetailConversation
                                selectedItem={selectedItem}
                                t={t}
                                viewMode={viewMode}
                                detailScrollRef={detailScrollRef}
                                assistantStartRef={assistantStartRef}
                                bottomButtonClassName="absolute right-5 bottom-5"
                            />
                        )}
                    </div>
                </div>
            </div>

            {isMobileView && mobileDetailOpen && selectedItem && (
                <div
                    className={clsx(
                        'fixed inset-0 z-50 flex items-center justify-center px-3 py-4 bg-background/65 backdrop-blur-sm transition-opacity duration-200',
                        mobileDetailVisible ? 'opacity-100' : 'opacity-0'
                    )}
                    onClick={closeMobileDetail}
                >
                    <div
                        onClick={(event) => event.stopPropagation()}
                        className={clsx(
                            'w-full h-full rounded-2xl border border-border bg-card shadow-2xl overflow-hidden flex flex-col transition-transform duration-200 ease-out',
                            mobileDetailVisible ? 'scale-100' : 'scale-90'
                        )}
                        style={{ transformOrigin: `${mobileOrigin.x}% ${mobileOrigin.y}%` }}
                    >
                        <div className="px-5 py-4 border-b border-border flex items-start justify-between gap-3">
                            <div>
                                <div className="text-sm font-semibold text-foreground">{t('chatHistory.detailTitle')}</div>
                                <div className="text-xs text-muted-foreground mt-1">
                                    {formatDateTime(selectedItem.completed_at || selectedItem.updated_at || selectedItem.created_at, lang)}
                                </div>
                            </div>
                            <div className="flex items-center gap-2">
                                <div className="inline-flex items-center rounded-xl border border-border bg-background p-1">
                                    <button
                                        type="button"
                                        onClick={() => setViewMode('list')}
                                        className={clsx(
                                            'h-9 w-10 rounded-lg flex items-center justify-center transition-colors',
                                            viewMode === 'list'
                                                ? 'bg-secondary text-foreground'
                                                : 'text-muted-foreground hover:text-foreground hover:bg-secondary/60'
                                        )}
                                        title={t('chatHistory.viewModeList')}
                                    >
                                        <ListModeIcon />
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => setViewMode('merged')}
                                        className={clsx(
                                            'h-9 w-10 rounded-lg flex items-center justify-center transition-colors',
                                            viewMode === 'merged'
                                                ? 'bg-secondary text-foreground'
                                                : 'text-muted-foreground hover:text-foreground hover:bg-secondary/60'
                                        )}
                                        title={t('chatHistory.viewModeMerged')}
                                    >
                                        <MergeModeIcon />
                                    </button>
                                </div>
                                <button
                                    type="button"
                                    onClick={() => detailScrollRef.current?.scrollTo({ top: 0, behavior: 'smooth' })}
                                    className="h-9 w-9 rounded-lg border border-border bg-background text-muted-foreground hover:text-foreground hover:bg-secondary/70 flex items-center justify-center"
                                    title={t('chatHistory.backToTop')}
                                >
                                    <ArrowUp className="w-4 h-4" />
                                </button>
                                <button
                                    type="button"
                                    onClick={closeMobileDetail}
                                    className="h-9 w-9 rounded-lg border border-border bg-background text-muted-foreground hover:text-foreground hover:bg-secondary/70 flex items-center justify-center"
                                    title={t('actions.cancel')}
                                >
                                    <X className="w-4 h-4" />
                                </button>
                            </div>
                        </div>

                        <div ref={detailScrollRef} className="flex-1 overflow-y-auto p-5 space-y-6">
                            <DetailConversation
                                selectedItem={selectedItem}
                                t={t}
                                viewMode={viewMode}
                                detailScrollRef={detailScrollRef}
                                assistantStartRef={assistantStartRef}
                                bottomButtonClassName="fixed right-5 bottom-5"
                            />
                        </div>
                    </div>
                </div>
            )}

            {confirmClearOpen && (
                <div className="fixed inset-0 z-50 bg-background/80 backdrop-blur-sm flex items-center justify-center px-4">
                    <div className="w-full max-w-sm rounded-2xl border border-border bg-card shadow-2xl p-5 space-y-4">
                        <div className="flex items-start justify-between gap-3">
                            <div className="flex items-center gap-3">
                                <div className="h-11 w-11 rounded-2xl bg-[#111214] text-muted-foreground flex items-center justify-center">
                                    <Trash2 className="w-5 h-5" />
                                </div>
                                <div>
                                    <div className="text-base font-semibold text-foreground">{t('chatHistory.confirmClearTitle')}</div>
                                    <div className="text-sm text-muted-foreground mt-1">{t('chatHistory.confirmClearDesc')}</div>
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={() => setConfirmClearOpen(false)}
                                className="p-2 rounded-lg text-muted-foreground hover:text-foreground hover:bg-secondary/70"
                            >
                                <X className="w-4 h-4" />
                            </button>
                        </div>
                        <div className="flex justify-end gap-3">
                            <button
                                type="button"
                                onClick={() => setConfirmClearOpen(false)}
                                className="h-10 px-4 rounded-lg border border-border bg-background text-muted-foreground hover:text-foreground hover:bg-secondary/60"
                            >
                                {t('actions.cancel')}
                            </button>
                            <button
                                type="button"
                                onClick={async () => {
                                    setConfirmClearOpen(false)
                                    await handleClear()
                                }}
                                className="h-10 px-4 rounded-lg border border-destructive/20 bg-destructive/10 text-destructive hover:bg-destructive/15 flex items-center gap-2"
                            >
                                <Trash2 className="w-4 h-4" />
                                {t('chatHistory.confirmClearAction')}
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    )
}
