import { useCallback, useEffect, useState } from 'react'

export function useUsageStats({ apiFetch, onMessage, t }) {
    const [loading, setLoading] = useState(true)
    const [refreshing, setRefreshing] = useState(false)
    const [stats, setStats] = useState({ summary: {}, rows: [] })

    const refresh = useCallback(async () => {
        const firstLoad = loading
        if (firstLoad) {
            setLoading(true)
        } else {
            setRefreshing(true)
        }
        try {
            const res = await apiFetch('/admin/stats/usage')
            const data = await res.json()
            if (!res.ok) {
                throw new Error(data?.detail || t('usageStats.loadFailed'))
            }
            setStats({
                summary: data?.summary || {},
                rows: Array.isArray(data?.rows) ? data.rows : [],
            })
        } catch (err) {
            onMessage?.('error', err?.message || t('usageStats.loadFailed'))
        } finally {
            setLoading(false)
            setRefreshing(false)
        }
    }, [apiFetch, loading, onMessage, t])

    useEffect(() => {
        refresh()
    }, [refresh])

    return {
        loading,
        refreshing,
        stats,
        refresh,
    }
}
