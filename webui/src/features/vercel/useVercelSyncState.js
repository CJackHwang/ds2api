import { useCallback, useEffect, useState } from 'react'

const POLL_INTERVAL_MS = 8000

export function useVercelSyncState({ apiFetch, onMessage, t }) {
    const [vercelToken, setVercelToken] = useState('')
    const [projectId, setProjectId] = useState('')
    const [teamId, setTeamId] = useState('')
    const [redisURL, setRedisURL] = useState('')
    const [redisKey, setRedisKey] = useState('')
    const [loading, setLoading] = useState(false)
    const [result, setResult] = useState(null)
    const [preconfig, setPreconfig] = useState(null)
    const [syncStatus, setSyncStatus] = useState(null)

    const fetchSyncStatus = useCallback(async ({ manual = false } = {}) => {
        try {
            const res = await apiFetch('/admin/vercel/status')
            if (!res.ok) {
                throw new Error(`status ${res.status}`)
            }
            const data = await res.json()
            setSyncStatus(data)
        } catch (e) {
            if (manual) {
                onMessage('error', t('vercel.networkError'))
            }
            // eslint-disable-next-line no-console
            console.error('Failed to fetch sync status:', e)
        }
    }, [apiFetch, onMessage, t])

    useEffect(() => {
        let mounted = true

        const loadPreconfig = async () => {
            try {
                const res = await apiFetch('/admin/vercel/config')
                if (!res.ok) return
                const data = await res.json()
                if (!mounted) return
                setPreconfig(data)
                if (data.project_id) setProjectId(data.project_id)
                if (data.team_id) setTeamId(data.team_id)
                if (data.redis_key) setRedisKey(data.redis_key)
            } catch (e) {
                // eslint-disable-next-line no-console
                console.error('Failed to load preconfig:', e)
            }
        }

        loadPreconfig()
        fetchSyncStatus()

        const interval = setInterval(() => {
            fetchSyncStatus()
        }, POLL_INTERVAL_MS)

        return () => {
            mounted = false
            clearInterval(interval)
        }
    }, [apiFetch, fetchSyncStatus])

    const handleManualRefresh = useCallback(() => {
        fetchSyncStatus({ manual: true })
    }, [fetchSyncStatus])

    const handleSync = useCallback(async () => {
        const tokenToUse = preconfig?.has_token && !vercelToken ? '__USE_PRECONFIG__' : vercelToken
        const redisURLToUse = preconfig?.has_redis_url && !redisURL ? '' : redisURL
        const redisKeyToUse = redisKey || preconfig?.redis_runtime_key || preconfig?.redis_key || 'ds2api:config'

        if (!tokenToUse && !preconfig?.has_token) {
            onMessage('error', t('vercel.tokenRequired'))
            return
        }
        if (!projectId) {
            onMessage('error', t('vercel.projectRequired'))
            return
        }
        if (!redisURLToUse && !preconfig?.has_redis_url) {
            onMessage('error', t('vercel.redisRequired'))
            return
        }

        setLoading(true)
        setResult(null)
        try {
            const res = await apiFetch('/admin/vercel/sync', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    vercel_token: tokenToUse,
                    project_id: projectId,
                    team_id: teamId || undefined,
                    redis_url: redisURLToUse || undefined,
                    redis_key: redisKeyToUse,
                }),
            })
            const data = await res.json()
            if (res.ok) {
                setResult({ ...data, success: true })
                onMessage('success', data.message)
                fetchSyncStatus()
            } else {
                setResult({ ...data, success: false })
                onMessage('error', data.detail || t('vercel.syncFailed'))
            }
        } catch (_e) {
            onMessage('error', t('vercel.networkError'))
        } finally {
            setLoading(false)
        }
    }, [apiFetch, fetchSyncStatus, onMessage, preconfig, projectId, redisKey, redisURL, t, teamId, vercelToken])

    return {
        vercelToken,
        setVercelToken,
        projectId,
        setProjectId,
        teamId,
        setTeamId,
        redisURL,
        setRedisURL,
        redisKey,
        setRedisKey,
        loading,
        result,
        preconfig,
        syncStatus,
        handleManualRefresh,
        handleSync,
    }
}
