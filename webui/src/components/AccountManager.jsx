import { useState, useEffect } from 'react'

export default function AccountManager({ config, onRefresh, onMessage, authFetch }) {
    const [showAddKey, setShowAddKey] = useState(false)
    const [showAddAccount, setShowAddAccount] = useState(false)
    const [newKey, setNewKey] = useState('')
    const [newAccount, setNewAccount] = useState({ email: '', mobile: '', password: '' })
    const [loading, setLoading] = useState(false)
    const [validating, setValidating] = useState({})  // 单个账号验证状态
    const [validatingAll, setValidatingAll] = useState(false)
    const [testing, setTesting] = useState({})  // 单个账号测试状态
    const [testingAll, setTestingAll] = useState(false)
    const [batchProgress, setBatchProgress] = useState({ current: 0, total: 0, results: [] })
    const [queueStatus, setQueueStatus] = useState(null)

    // 使用 authFetch 或回退到普通 fetch
    const apiFetch = authFetch || fetch

    // 获取队列状态
    const fetchQueueStatus = async () => {
        try {
            const res = await apiFetch('/admin/queue/status')
            if (res.ok) {
                const data = await res.json()
                setQueueStatus(data)
            }
        } catch (e) {
            console.error('获取队列状态失败:', e)
        }
    }

    useEffect(() => {
        fetchQueueStatus()
        const interval = setInterval(fetchQueueStatus, 5000)  // 每5秒刷新
        return () => clearInterval(interval)
    }, [])

    const addKey = async () => {
        if (!newKey.trim()) return
        setLoading(true)
        try {
            const res = await apiFetch('/admin/keys', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ key: newKey.trim() }),
            })
            if (res.ok) {
                onMessage('success', 'API Key 添加成功')
                setNewKey('')
                setShowAddKey(false)
                onRefresh()
            } else {
                const data = await res.json()
                onMessage('error', data.detail || '添加失败')
            }
        } catch (e) {
            onMessage('error', '网络错误')
        } finally {
            setLoading(false)
        }
    }

    const deleteKey = async (key) => {
        if (!confirm('确定删除此 API Key？')) return
        try {
            const res = await apiFetch(`/admin/keys/${encodeURIComponent(key)}`, { method: 'DELETE' })
            if (res.ok) {
                onMessage('success', '删除成功')
                onRefresh()
            } else {
                onMessage('error', '删除失败')
            }
        } catch (e) {
            onMessage('error', '网络错误')
        }
    }

    const addAccount = async () => {
        if (!newAccount.password || (!newAccount.email && !newAccount.mobile)) {
            onMessage('error', '请填写密码和邮箱/手机号')
            return
        }
        setLoading(true)
        try {
            const res = await apiFetch('/admin/accounts', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(newAccount),
            })
            if (res.ok) {
                onMessage('success', '账号添加成功')
                setNewAccount({ email: '', mobile: '', password: '' })
                setShowAddAccount(false)
                onRefresh()
            } else {
                const data = await res.json()
                onMessage('error', data.detail || '添加失败')
            }
        } catch (e) {
            onMessage('error', '网络错误')
        } finally {
            setLoading(false)
        }
    }

    const deleteAccount = async (id) => {
        if (!confirm('确定删除此账号？')) return
        try {
            const res = await apiFetch(`/admin/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' })
            if (res.ok) {
                onMessage('success', '删除成功')
                onRefresh()
            } else {
                onMessage('error', '删除失败')
            }
        } catch (e) {
            onMessage('error', '网络错误')
        }
    }

    // 验证单个账号
    const validateAccount = async (identifier) => {
        setValidating(prev => ({ ...prev, [identifier]: true }))
        try {
            const res = await apiFetch('/admin/accounts/validate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identifier }),
            })
            const data = await res.json()
            if (data.valid) {
                onMessage('success', `${identifier}: ${data.message}`)
            } else {
                onMessage('error', `${identifier}: ${data.message}`)
            }
            onRefresh()
        } catch (e) {
            onMessage('error', '验证失败: ' + e.message)
        } finally {
            setValidating(prev => ({ ...prev, [identifier]: false }))
        }
    }

    // 批量验证所有账号（带进度）
    const validateAllAccounts = async () => {
        if (!confirm('确定要验证所有账号？')) return
        const accounts = config.accounts || []
        if (accounts.length === 0) return

        setValidatingAll(true)
        setBatchProgress({ current: 0, total: accounts.length, results: [] })

        let validCount = 0
        const results = []

        for (let i = 0; i < accounts.length; i++) {
            const acc = accounts[i]
            const id = acc.email || acc.mobile

            try {
                const res = await apiFetch('/admin/accounts/validate', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ identifier: id }),
                })
                const data = await res.json()
                results.push({ id, success: data.valid, message: data.message })
                if (data.valid) validCount++
            } catch (e) {
                results.push({ id, success: false, message: e.message })
            }

            setBatchProgress({ current: i + 1, total: accounts.length, results: [...results] })
        }

        onMessage('success', `验证完成: ${validCount}/${accounts.length} 个账号有效`)
        onRefresh()
        setValidatingAll(false)
    }

    // 测试单个账号 API
    const testAccount = async (identifier) => {
        setTesting(prev => ({ ...prev, [identifier]: true }))
        try {
            const res = await apiFetch('/admin/accounts/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identifier }),
            })
            const data = await res.json()
            if (data.success) {
                onMessage('success', `${identifier}: API 测试成功 (${data.response_time}ms)`)
            } else {
                onMessage('error', `${identifier}: ${data.message}`)
            }
            onRefresh()
        } catch (e) {
            onMessage('error', 'API 测试失败: ' + e.message)
        } finally {
            setTesting(prev => ({ ...prev, [identifier]: false }))
        }
    }

    // 批量测试所有账号 API（带进度）
    const testAllAccounts = async () => {
        if (!confirm('确定要测试所有账号的 API？')) return
        const accounts = config.accounts || []
        if (accounts.length === 0) return

        setTestingAll(true)
        setBatchProgress({ current: 0, total: accounts.length, results: [] })

        let successCount = 0
        const results = []

        for (let i = 0; i < accounts.length; i++) {
            const acc = accounts[i]
            const id = acc.email || acc.mobile

            try {
                const res = await apiFetch('/admin/accounts/test', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ identifier: id }),
                })
                const data = await res.json()
                results.push({ id, success: data.success, message: data.message, time: data.response_time })
                if (data.success) successCount++
            } catch (e) {
                results.push({ id, success: false, message: e.message })
            }

            setBatchProgress({ current: i + 1, total: accounts.length, results: [...results] })
        }

        onMessage('success', `API 测试完成: ${successCount}/${accounts.length} 个账号可用`)
        onRefresh()
        setTestingAll(false)
    }

    return (
        <div className="section">
            {/* 队列状态监控 */}
            {queueStatus && (
                <div className="card">
                    <div className="card-header">
                        <span className="card-title">📊 轮询队列状态</span>
                        <button className="btn btn-secondary" onClick={fetchQueueStatus}>刷新</button>
                    </div>
                    <div className="queue-status">
                        <div className="stat-row">
                            <span className="stat-label">可用账号:</span>
                            <span className="stat-value stat-success">{queueStatus.available}</span>
                            <span className="stat-label" style={{ marginLeft: '20px' }}>使用中:</span>
                            <span className="stat-value stat-warning">{queueStatus.in_use}</span>
                            <span className="stat-label" style={{ marginLeft: '20px' }}>总计:</span>
                            <span className="stat-value">{queueStatus.total}</span>
                        </div>
                        {queueStatus.in_use > 0 && (
                            <div className="stat-detail">
                                正在使用: {queueStatus.in_use_accounts.join(', ')}
                            </div>
                        )}
                    </div>
                </div>
            )}

            {/* API Keys */}
            <div className="card">
                <div className="card-header">
                    <span className="card-title">🔑 API Keys</span>
                    <button className="btn btn-primary" onClick={() => setShowAddKey(true)}>+ 添加</button>
                </div>

                {config.keys?.length > 0 ? (
                    <div className="list">
                        {config.keys.map((key, i) => (
                            <div key={i} className="list-item">
                                <span className="list-item-text">{key.slice(0, 16)}****</span>
                                <button className="btn btn-danger" onClick={() => deleteKey(key)}>删除</button>
                            </div>
                        ))}
                    </div>
                ) : (
                    <div className="empty-state">暂无 API Key</div>
                )}
            </div>

            {/* Accounts */}
            <div className="card">
                <div className="card-header">
                    <span className="card-title">👤 DeepSeek 账号</span>
                    <div className="btn-group-inline">
                        <button
                            className="btn btn-primary btn-sm"
                            onClick={testAllAccounts}
                            disabled={testingAll || validatingAll || !config.accounts?.length}
                        >
                            {testingAll ? <span className="loading"></span> : '🧪 批量测试'}
                        </button>
                        <button
                            className="btn btn-secondary btn-sm"
                            onClick={validateAllAccounts}
                            disabled={validatingAll || testingAll || !config.accounts?.length}
                        >
                            {validatingAll ? <span className="loading"></span> : '✅ 批量验证'}
                        </button>
                        <button className="btn btn-primary" onClick={() => setShowAddAccount(true)}>+ 添加</button>
                    </div>
                </div>

                {/* 批量操作进度条 */}
                {(testingAll || validatingAll) && batchProgress.total > 0 && (
                    <div className="batch-progress">
                        <div className="progress-header">
                            <span>{testingAll ? '🧪 批量测试中...' : '✅ 批量验证中...'}</span>
                            <span>{batchProgress.current}/{batchProgress.total}</span>
                        </div>
                        <div className="progress-bar">
                            <div
                                className="progress-fill"
                                style={{ width: `${(batchProgress.current / batchProgress.total) * 100}%` }}
                            />
                        </div>
                        {batchProgress.results.length > 0 && (
                            <div className="progress-results">
                                {batchProgress.results.map((r, i) => (
                                    <div key={i} className={`progress-result ${r.success ? 'success' : 'failed'}`}>
                                        {r.success ? '✓' : '✗'} {r.id} {r.time ? `(${r.time}ms)` : ''}
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                )}

                {config.accounts?.length > 0 ? (
                    <div className="list">
                        {config.accounts.map((acc, i) => {
                            const id = acc.email || acc.mobile
                            return (
                                <div key={i} className="list-item">
                                    <div className="list-item-info">
                                        <span className="list-item-text">{id}</span>
                                        <span className={`badge ${acc.has_token ? 'badge-success' : 'badge-warning'}`}>
                                            {acc.has_token ? '已登录' : '未登录'}
                                        </span>
                                        {acc.token_preview && (
                                            <span className="token-preview" title="Token 预览">
                                                🔑 {acc.token_preview}
                                            </span>
                                        )}
                                    </div>
                                    <div className="btn-group-inline">
                                        <button
                                            className="btn btn-primary btn-sm"
                                            onClick={() => testAccount(id)}
                                            disabled={testing[id]}
                                        >
                                            {testing[id] ? <span className="loading"></span> : '测试'}
                                        </button>
                                        <button
                                            className="btn btn-secondary btn-sm"
                                            onClick={() => validateAccount(id)}
                                            disabled={validating[id]}
                                        >
                                            {validating[id] ? <span className="loading"></span> : '验证'}
                                        </button>
                                        <button className="btn btn-danger btn-sm" onClick={() => deleteAccount(id)}>删除</button>
                                    </div>
                                </div>
                            )
                        })}
                    </div>
                ) : (
                    <div className="empty-state">暂无账号</div>
                )}
            </div>

            {/* Add Key Modal */}
            {showAddKey && (
                <div className="modal-overlay" onClick={() => setShowAddKey(false)}>
                    <div className="modal" onClick={e => e.stopPropagation()}>
                        <div className="modal-header">
                            <span className="modal-title">添加 API Key</span>
                            <button className="modal-close" onClick={() => setShowAddKey(false)}>&times;</button>
                        </div>
                        <div className="form-group">
                            <label className="form-label">API Key</label>
                            <input
                                type="text"
                                className="form-input"
                                placeholder="输入你自定义的 API Key"
                                value={newKey}
                                onChange={e => setNewKey(e.target.value)}
                            />
                        </div>
                        <div className="btn-group">
                            <button className="btn btn-secondary" onClick={() => setShowAddKey(false)}>取消</button>
                            <button className="btn btn-primary" onClick={addKey} disabled={loading}>
                                {loading ? <span className="loading"></span> : '添加'}
                            </button>
                        </div>
                    </div>
                </div>
            )}

            {/* Add Account Modal */}
            {showAddAccount && (
                <div className="modal-overlay" onClick={() => setShowAddAccount(false)}>
                    <div className="modal" onClick={e => e.stopPropagation()}>
                        <div className="modal-header">
                            <span className="modal-title">添加 DeepSeek 账号</span>
                            <button className="modal-close" onClick={() => setShowAddAccount(false)}>&times;</button>
                        </div>
                        <div className="form-group">
                            <label className="form-label">Email（可选）</label>
                            <input
                                type="email"
                                className="form-input"
                                placeholder="user@example.com"
                                value={newAccount.email}
                                onChange={e => setNewAccount({ ...newAccount, email: e.target.value })}
                            />
                        </div>
                        <div className="form-group">
                            <label className="form-label">手机号（可选）</label>
                            <input
                                type="text"
                                className="form-input"
                                placeholder="+86..."
                                value={newAccount.mobile}
                                onChange={e => setNewAccount({ ...newAccount, mobile: e.target.value })}
                            />
                        </div>
                        <div className="form-group">
                            <label className="form-label">密码（必填）</label>
                            <input
                                type="password"
                                className="form-input"
                                placeholder="DeepSeek 账号密码"
                                value={newAccount.password}
                                onChange={e => setNewAccount({ ...newAccount, password: e.target.value })}
                            />
                        </div>
                        <div className="btn-group">
                            <button className="btn btn-secondary" onClick={() => setShowAddAccount(false)}>取消</button>
                            <button className="btn btn-primary" onClick={addAccount} disabled={loading}>
                                {loading ? <span className="loading"></span> : '添加'}
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    )
}
