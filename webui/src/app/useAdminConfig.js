import { useCallback, useEffect, useState } from 'react'

const ENV_DRAFT_KEY = 'ds2api_env_config_draft_v1'

export function useAdminConfig({ token, showMessage, t }) {
    const [config, setConfig] = useState({ keys: [], accounts: [], proxies: [] })

    const fetchConfig = useCallback(async () => {
        if (!token) return
        try {
            const res = await fetch('/admin/config', {
                headers: { 'Authorization': `Bearer ${token}` }
            })
            if (res.ok) {
                const data = await res.json()
                if (data?.env_backed) {
                    localStorage.setItem(ENV_DRAFT_KEY, JSON.stringify(data))
                } else {
                    localStorage.removeItem(ENV_DRAFT_KEY)
                }
                setConfig(data)
            }
        } catch (e) {
            console.error('Failed to fetch config:', e)
            showMessage('error', t('errors.fetchConfig', { error: e.message }))
        }
    }, [showMessage, t, token])

    useEffect(() => {
        if (token) {
            const rawDraft = localStorage.getItem(ENV_DRAFT_KEY)
            if (rawDraft) {
                try {
                    const draft = JSON.parse(rawDraft)
                    if (draft?.env_backed) {
                        setConfig(draft)
                    }
                } catch (_e) {
                    localStorage.removeItem(ENV_DRAFT_KEY)
                }
            }
            fetchConfig()
        }
    }, [fetchConfig, token])

    return {
        config,
        fetchConfig,
    }
}
