import { useState } from 'react'

import { useI18n } from '../../i18n'
import { readApiResponse } from '../../utils/http'
import ProxiesTable from './ProxiesTable'
import ProxyFormModal, { createEmptyProxyForm } from './ProxyFormModal'

export default function ProxyManagerContainer({ config, onRefresh, onMessage, authFetch }) {
    const { t } = useI18n()
    const apiFetch = authFetch || fetch

    const [showModal, setShowModal] = useState(false)
    const [editingProxy, setEditingProxy] = useState(null)
    const [form, setForm] = useState(createEmptyProxyForm())
    const [saving, setSaving] = useState(false)
    const [testing, setTesting] = useState({})
    const [testResults, setTestResults] = useState({})

    const proxies = config?.proxies || []

    const openCreate = () => {
        setEditingProxy(null)
        setForm(createEmptyProxyForm())
        setShowModal(true)
    }

    const openEdit = (proxy) => {
        setEditingProxy(proxy)
        setForm({
            name: proxy.name || '',
            type: proxy.type || 'socks5h',
            host: proxy.host || '',
            port: proxy.port || 1080,
            username: proxy.username || '',
            password: '',
        })
        setShowModal(true)
    }

    const closeModal = () => {
        setShowModal(false)
        setEditingProxy(null)
        setForm(createEmptyProxyForm())
    }

    const saveProxy = async () => {
        if (!form.host || !form.port) {
            onMessage('error', t('proxyManager.requiredFields'))
            return
        }
        setSaving(true)
        try {
            const url = editingProxy?.id
                ? `/admin/proxies/${encodeURIComponent(editingProxy.id)}`
                : '/admin/proxies'
            const method = editingProxy?.id ? 'PUT' : 'POST'
            const res = await apiFetch(url, {
                method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: form.name,
                    type: form.type,
                    host: form.host,
                    port: Number(form.port),
                    username: form.username,
                    password: form.password,
                }),
            })
            const data = await readApiResponse(res, t('settings.nonJsonResponse', { status: res.status }))
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            await onRefresh?.()
            onMessage('success', editingProxy?.id ? t('proxyManager.updateSuccess') : t('proxyManager.addSuccess'))
            closeModal()
        } catch (err) {
            onMessage('error', err?.message || t('messages.networkError'))
        } finally {
            setSaving(false)
        }
    }

    const deleteProxy = async (proxy) => {
        if (!confirm(t('proxyManager.deleteConfirm', { name: proxy.name || `${proxy.host}:${proxy.port}` }))) return
        try {
            const res = await apiFetch(`/admin/proxies/${encodeURIComponent(proxy.id)}`, { method: 'DELETE' })
            const data = await readApiResponse(res, t('settings.nonJsonResponse', { status: res.status }))
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.deleteFailed'))
                return
            }
            await onRefresh?.()
            onMessage('success', t('messages.deleted'))
            setTestResults(prev => {
                const next = { ...prev }
                delete next[proxy.id]
                return next
            })
        } catch (err) {
            onMessage('error', err?.message || t('messages.networkError'))
        }
    }

    const testProxy = async (proxy) => {
        setTesting(prev => ({ ...prev, [proxy.id]: true }))
        try {
            const res = await apiFetch('/admin/proxies/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ proxy_id: proxy.id }),
            })
            const data = await readApiResponse(res, t('settings.nonJsonResponse', { status: res.status }))
            setTestResults(prev => ({ ...prev, [proxy.id]: data }))
            onMessage(data.success ? 'success' : 'error', data.message || t('messages.requestFailed'))
        } catch (err) {
            onMessage('error', err?.message || t('messages.networkError'))
        } finally {
            setTesting(prev => ({ ...prev, [proxy.id]: false }))
        }
    }

    return (
        <div className="space-y-6">
            <div className="grid gap-4 md:grid-cols-3">
                <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                    <div className="text-[10px] text-muted-foreground font-bold uppercase tracking-wider">{t('proxyManager.totalProxies')}</div>
                    <div className="mt-2 text-2xl font-bold">{proxies.length}</div>
                </div>
                <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                    <div className="text-[10px] text-muted-foreground font-bold uppercase tracking-wider">{t('proxyManager.socks5hCount')}</div>
                    <div className="mt-2 text-2xl font-bold">{proxies.filter(proxy => proxy.type === 'socks5h').length}</div>
                </div>
                <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                    <div className="text-[10px] text-muted-foreground font-bold uppercase tracking-wider">{t('proxyManager.authProxyCount')}</div>
                    <div className="mt-2 text-2xl font-bold">{proxies.filter(proxy => proxy.username || proxy.has_password).length}</div>
                </div>
            </div>

            <ProxiesTable
                t={t}
                proxies={proxies}
                testing={testing}
                testResults={testResults}
                onCreate={openCreate}
                onTest={testProxy}
                onEdit={openEdit}
                onDelete={deleteProxy}
            />

            <ProxyFormModal
                show={showModal}
                t={t}
                form={form}
                setForm={setForm}
                editingProxy={editingProxy}
                loading={saving}
                onClose={closeModal}
                onSubmit={saveProxy}
            />
        </div>
    )
}
