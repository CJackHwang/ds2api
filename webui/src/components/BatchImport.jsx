import { useState } from 'react'

// 模板配置
const TEMPLATES = {
    full: {
        name: '完整模板',
        desc: '包含所有配置项',
        config: {
            keys: ["your-api-key-1", "your-api-key-2"],
            accounts: [
                { email: "user1@example.com", password: "password1", token: "" },
                { email: "user2@example.com", password: "password2", token: "" },
                { mobile: "+8613800138001", password: "password3", token: "" }
            ],
            claude_model_mapping: {
                fast: "deepseek-chat",
                slow: "deepseek-reasoner"
            }
        }
    },
    email_only: {
        name: '邮箱账号模板',
        desc: '仅邮箱账号',
        config: {
            keys: ["your-api-key"],
            accounts: [
                { email: "account1@example.com", password: "pass1", token: "" },
                { email: "account2@example.com", password: "pass2", token: "" },
                { email: "account3@example.com", password: "pass3", token: "" }
            ]
        }
    },
    mobile_only: {
        name: '手机号账号模板',
        desc: '仅手机号账号',
        config: {
            keys: ["your-api-key"],
            accounts: [
                { mobile: "+8613800000001", password: "pass1", token: "" },
                { mobile: "+8613800000002", password: "pass2", token: "" },
                { mobile: "+8613800000003", password: "pass3", token: "" }
            ]
        }
    },
    keys_only: {
        name: '仅 API Keys',
        desc: '只添加 API Keys',
        config: {
            keys: ["key-1", "key-2", "key-3"]
        }
    }
}

export default function BatchImport({ onRefresh, onMessage, authFetch }) {
    const [jsonInput, setJsonInput] = useState('')
    const [loading, setLoading] = useState(false)
    const [result, setResult] = useState(null)

    // 使用 authFetch 或回退到普通 fetch
    const apiFetch = authFetch || fetch

    const handleImport = async () => {
        if (!jsonInput.trim()) {
            onMessage('error', '请输入 JSON 配置')
            return
        }

        let config
        try {
            config = JSON.parse(jsonInput)
        } catch (e) {
            onMessage('error', 'JSON 格式无效')
            return
        }

        setLoading(true)
        setResult(null)
        try {
            const res = await apiFetch('/admin/import', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config),
            })
            const data = await res.json()
            if (res.ok) {
                setResult(data)
                onMessage('success', `导入成功: ${data.imported_keys} 个 Key, ${data.imported_accounts} 个账号`)
                onRefresh()
            } else {
                onMessage('error', data.detail || '导入失败')
            }
        } catch (e) {
            onMessage('error', '网络错误')
        } finally {
            setLoading(false)
        }
    }

    const loadTemplate = (key) => {
        const tpl = TEMPLATES[key]
        if (tpl) {
            setJsonInput(JSON.stringify(tpl.config, null, 2))
            onMessage('info', `已加载「${tpl.name}」`)
        }
    }

    const handleExport = async () => {
        try {
            const res = await apiFetch('/admin/export')
            if (res.ok) {
                const data = await res.json()
                setJsonInput(JSON.stringify(JSON.parse(data.json), null, 2))
                onMessage('success', '已加载当前配置')
            }
        } catch (e) {
            onMessage('error', '获取配置失败')
        }
    }

    const copyBase64 = async () => {
        try {
            const res = await apiFetch('/admin/export')
            if (res.ok) {
                const data = await res.json()
                await navigator.clipboard.writeText(data.base64)
                onMessage('success', 'Base64 已复制到剪贴板')
            }
        } catch (e) {
            onMessage('error', '复制失败')
        }
    }

    return (
        <div className="section">
            {/* 模板选择 */}
            <div className="card">
                <div className="card-title" style={{ marginBottom: '1rem' }}>📋 快速模板</div>
                <div className="grid grid-2">
                    {Object.entries(TEMPLATES).map(([key, tpl]) => (
                        <div
                            key={key}
                            style={{
                                padding: '1rem',
                                background: 'var(--bg-tertiary)',
                                borderRadius: 'var(--radius)',
                                cursor: 'pointer',
                                transition: 'all 0.2s',
                                border: '1px solid transparent'
                            }}
                            onClick={() => loadTemplate(key)}
                            onMouseEnter={e => e.currentTarget.style.borderColor = 'var(--accent)'}
                            onMouseLeave={e => e.currentTarget.style.borderColor = 'transparent'}
                        >
                            <div style={{ fontWeight: 600, marginBottom: '0.25rem' }}>{tpl.name}</div>
                            <div style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>{tpl.desc}</div>
                        </div>
                    ))}
                </div>
            </div>

            {/* 导入区域 */}
            <div className="card">
                <div className="card-title" style={{ marginBottom: '1rem' }}>📦 批量导入</div>

                <div className="form-group">
                    <label className="form-label">JSON 配置（点击上方模板快速填充）</label>
                    <textarea
                        className="form-input"
                        style={{ minHeight: '200px' }}
                        value={jsonInput}
                        onChange={e => setJsonInput(e.target.value)}
                        placeholder='{\n  "keys": ["你的API密钥"],\n  "accounts": [\n    {"email": "邮箱", "password": "密码", "token": ""}\n  ]\n}'
                    />
                </div>

                <div className="btn-group" style={{ marginBottom: '1rem' }}>
                    <button className="btn btn-secondary" onClick={handleExport}>
                        ⬇️ 导出当前
                    </button>
                    <button className="btn btn-primary" onClick={handleImport} disabled={loading}>
                        {loading ? <span className="loading"></span> : '📥 导入配置'}
                    </button>
                </div>

                {result && (
                    <div className="alert alert-success">
                        ✅ 导入完成：{result.imported_keys} 个 API Key，{result.imported_accounts} 个账号
                    </div>
                )}
            </div>

            <div className="card">
                <div className="card-title" style={{ marginBottom: '1rem' }}>📤 导出 Base64</div>
                <p style={{ color: 'var(--text-secondary)', marginBottom: '1rem' }}>
                    导出 Base64 格式配置，可直接粘贴到 Vercel 环境变量 <code>DS2API_CONFIG_JSON</code>
                </p>
                <button className="btn btn-success" onClick={copyBase64}>
                    📋 复制 Base64 到剪贴板
                </button>
            </div>
        </div>
    )
}
