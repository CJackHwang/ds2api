import { useState, useEffect } from 'react'

export default function VercelSync({ onMessage, authFetch }) {
    const [vercelToken, setVercelToken] = useState('')
    const [projectId, setProjectId] = useState('')
    const [teamId, setTeamId] = useState('')
    const [loading, setLoading] = useState(false)
    const [result, setResult] = useState(null)
    const [preconfig, setPreconfig] = useState(null)

    // 使用 authFetch 或回退到普通 fetch
    const apiFetch = authFetch || fetch

    // 自动加载预配置的 Vercel 信息
    useEffect(() => {
        const loadPreconfig = async () => {
            try {
                const res = await apiFetch('/admin/vercel/config')
                if (res.ok) {
                    const data = await res.json()
                    setPreconfig(data)
                    if (data.project_id) setProjectId(data.project_id)
                    if (data.team_id) setTeamId(data.team_id)
                }
            } catch (e) {
                console.error('加载 Vercel 预配置失败:', e)
            }
        }
        loadPreconfig()
    }, [])

    const handleSync = async () => {
        // 如果预配置了 token，使用特殊标记让后端使用预配置的 token
        const tokenToUse = preconfig?.has_token && !vercelToken ? '__USE_PRECONFIG__' : vercelToken

        if (!tokenToUse && !preconfig?.has_token) {
            onMessage('error', '请填写 Vercel Token')
            return
        }
        if (!projectId) {
            onMessage('error', '请填写 Project ID')
            return
        }

        setLoading(true)
        setResult(null)
        try {
            const res = await apiFetch('/admin/vercel/sync', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    vercel_token: vercelToken,
                    project_id: projectId,
                    team_id: teamId || undefined,
                }),
            })
            const data = await res.json()
            if (res.ok) {
                setResult(data)
                onMessage('success', data.message)
            } else {
                onMessage('error', data.detail || '同步失败')
            }
        } catch (e) {
            onMessage('error', '网络错误')
        } finally {
            setLoading(false)
        }
    }

    return (
        <div className="section">
            <div className="card">
                <div className="card-title" style={{ marginBottom: '1rem' }}>☁️ Vercel 同步</div>

                <div className="alert alert-info" style={{ marginBottom: '1rem' }}>
                    <strong>说明：</strong>同步配置到 Vercel 后会自动触发重新部署，约需 30-60 秒生效。
                </div>

                <div className="form-group">
                    <label className="form-label">
                        Vercel Token
                        <a
                            href="https://vercel.com/account/tokens"
                            target="_blank"
                            rel="noopener noreferrer"
                            style={{ marginLeft: '0.5rem', fontSize: '0.8rem' }}
                        >
                            获取 Token →
                        </a>
                    </label>
                    <input
                        type="password"
                        className="form-input"
                        placeholder="输入 Vercel API Token"
                        value={vercelToken}
                        onChange={e => setVercelToken(e.target.value)}
                    />
                </div>

                <div className="form-group">
                    <label className="form-label">
                        Project ID
                        <span style={{ marginLeft: '0.5rem', fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
                            (可在 Vercel 项目设置中找到)
                        </span>
                    </label>
                    <input
                        type="text"
                        className="form-input"
                        placeholder="prj_xxxxxxxxxxxx 或项目名称"
                        value={projectId}
                        onChange={e => setProjectId(e.target.value)}
                    />
                </div>

                <div className="form-group">
                    <label className="form-label">
                        Team ID（可选）
                        <span style={{ marginLeft: '0.5rem', fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
                            (个人项目无需填写)
                        </span>
                    </label>
                    <input
                        type="text"
                        className="form-input"
                        placeholder="team_xxxxxxxxxxxx"
                        value={teamId}
                        onChange={e => setTeamId(e.target.value)}
                    />
                </div>

                <button
                    className="btn btn-primary"
                    onClick={handleSync}
                    disabled={loading}
                    style={{ width: '100%' }}
                >
                    {loading ? (
                        <>
                            <span className="loading"></span>
                            同步中...
                        </>
                    ) : (
                        '🚀 同步到 Vercel 并重新部署'
                    )}
                </button>
            </div>

            {result && (
                <div className="card">
                    <div className="card-title" style={{ marginBottom: '1rem' }}>同步结果</div>
                    <div className={`alert ${result.success ? 'alert-success' : 'alert-error'}`}>
                        {result.message}
                    </div>
                    {result.deployment_url && (
                        <p>
                            部署地址：
                            <a
                                href={`https://${result.deployment_url}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                style={{ color: 'var(--accent)' }}
                            >
                                {result.deployment_url}
                            </a>
                        </p>
                    )}
                    {result.manual_deploy_required && (
                        <p style={{ color: 'var(--warning)' }}>
                            ⚠️ 需要手动在 Vercel 控制台触发重新部署
                        </p>
                    )}
                </div>
            )}

            <div className="card">
                <div className="card-title" style={{ marginBottom: '1rem' }}>📖 使用说明</div>
                <ol style={{ paddingLeft: '1.5rem', color: 'var(--text-secondary)' }}>
                    <li style={{ marginBottom: '0.5rem' }}>
                        前往 <a href="https://vercel.com/account/tokens" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent)' }}>Vercel Token 页面</a> 创建一个新 Token
                    </li>
                    <li style={{ marginBottom: '0.5rem' }}>
                        在 Vercel 项目设置中找到 Project ID（Settings → General → Project ID）
                    </li>
                    <li style={{ marginBottom: '0.5rem' }}>
                        如果是团队项目，还需要填写 Team ID
                    </li>
                    <li style={{ marginBottom: '0.5rem' }}>
                        点击同步按钮，配置将自动更新到 Vercel 环境变量并触发重新部署
                    </li>
                </ol>
            </div>
        </div>
    )
}
