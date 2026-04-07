import { useMemo } from 'react'

import { useI18n } from '../../i18n'
import { useUsageStats } from './useUsageStats'

function formatTimestamp(value, lang) {
    if (!value) return '-'
    const d = new Date(value * 1000)
    if (Number.isNaN(d.getTime())) return '-'
    return new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
    }).format(d)
}

function resolvedModel(row) {
    return row?.resolved_model || row?.response_model || row?.requested_model || '-'
}

export default function UsageStatsContainer({ authFetch, onMessage }) {
    const { t, lang } = useI18n()
    const apiFetch = authFetch || fetch
    const { loading, refreshing, stats, refresh } = useUsageStats({ apiFetch, onMessage, t })
    const rows = stats?.rows || []
    const summary = stats?.summary || {}

    const summaryCards = useMemo(() => ([
        { key: 'total', label: t('usageStats.summary.totalCalls'), value: summary.total_calls || 0 },
        { key: 'accounts', label: t('usageStats.summary.accounts'), value: summary.account_count || 0 },
        { key: 'models', label: t('usageStats.summary.models'), value: summary.model_count || 0 },
        { key: 'surfaces', label: t('usageStats.summary.surfaces'), value: summary.surface_count || 0 },
    ]), [summary, t])

    return (
        <div className="space-y-6">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div>
                    <h2 className="text-xl font-semibold">{t('usageStats.title')}</h2>
                    <p className="text-sm text-muted-foreground">{t('usageStats.desc')}</p>
                </div>
                <div className="flex items-center gap-3">
                    <div className="text-xs text-muted-foreground">
                        {t('usageStats.lastUpdated')}: {formatTimestamp(summary.last_called_at, lang)}
                    </div>
                    <button
                        onClick={refresh}
                        disabled={refreshing}
                        className="h-10 rounded-lg border border-border px-4 text-sm font-medium hover:bg-secondary disabled:opacity-60"
                    >
                        {refreshing ? t('usageStats.refreshing') : t('usageStats.refresh')}
                    </button>
                </div>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
                {summaryCards.map((card) => (
                    <div key={card.key} className="rounded-xl border border-border bg-card p-4 shadow-sm">
                        <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{card.label}</div>
                        <div className="mt-2 text-3xl font-bold text-foreground">{card.value}</div>
                    </div>
                ))}
            </div>

            <div className="rounded-2xl border border-border bg-card shadow-sm">
                <div className="flex items-center justify-between border-b border-border px-5 py-4">
                    <div>
                        <h3 className="font-semibold">{t('usageStats.table.title')}</h3>
                        <p className="text-sm text-muted-foreground">{t('usageStats.table.desc')}</p>
                    </div>
                    <div className="text-xs text-muted-foreground">{t('usageStats.table.rows', { count: rows.length })}</div>
                </div>

                <div className="overflow-x-auto">
                    <table className="min-w-full text-sm">
                        <thead className="bg-secondary/40 text-muted-foreground">
                            <tr>
                                <th className="px-5 py-3 text-left font-medium">{t('usageStats.columns.account')}</th>
                                <th className="px-5 py-3 text-left font-medium">{t('usageStats.columns.accountType')}</th>
                                <th className="px-5 py-3 text-left font-medium">{t('usageStats.columns.model')}</th>
                                <th className="px-5 py-3 text-left font-medium">{t('usageStats.columns.requestedModel')}</th>
                                <th className="px-5 py-3 text-left font-medium">{t('usageStats.columns.surface')}</th>
                                <th className="px-5 py-3 text-left font-medium">{t('usageStats.columns.count')}</th>
                                <th className="px-5 py-3 text-left font-medium">{t('usageStats.columns.lastCalledAt')}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((row, idx) => (
                                <tr key={`${row.account_id}-${row.surface}-${row.resolved_model}-${idx}`} className="border-t border-border">
                                    <td className="px-5 py-4 font-medium text-foreground">{row.account_id || '-'}</td>
                                    <td className="px-5 py-4 text-muted-foreground">{t(`usageStats.accountType.${row.account_type || 'unknown'}`)}</td>
                                    <td className="px-5 py-4 text-foreground">{resolvedModel(row)}</td>
                                    <td className="px-5 py-4 text-muted-foreground">{row.requested_model || '-'}</td>
                                    <td className="px-5 py-4">
                                        <span className="inline-flex rounded-full border border-border bg-background px-2.5 py-1 text-xs">
                                            {row.surface || '-'}
                                        </span>
                                    </td>
                                    <td className="px-5 py-4 font-semibold text-foreground">{row.count || 0}</td>
                                    <td className="px-5 py-4 text-muted-foreground">{formatTimestamp(row.last_called_at, lang)}</td>
                                </tr>
                            ))}
                            {!loading && rows.length === 0 && (
                                <tr>
                                    <td colSpan={7} className="px-5 py-12 text-center text-muted-foreground">
                                        {t('usageStats.empty')}
                                    </td>
                                </tr>
                            )}
                            {loading && (
                                <tr>
                                    <td colSpan={7} className="px-5 py-12 text-center text-muted-foreground">
                                        {t('usageStats.loading')}
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    )
}
