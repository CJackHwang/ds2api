import { useState, useEffect } from 'react'
import AccountManager from './components/AccountManager'
import ApiTester from './components/ApiTester'
import BatchImport from './components/BatchImport'
import VercelSync from './components/VercelSync'
import Login from './components/Login'

const TABS = [
    { id: 'accounts', label: '🔑 账号管理' },
    { id: 'test', label: '🧪 API 测试' },
    { id: 'import', label: '📦 批量导入' },
    { id: 'vercel', label: '☁️ Vercel 同步' },
]

export default function App() {
    const [activeTab, setActiveTab] = useState('accounts')
    const [config, setConfig] = useState({ keys: [], accounts: [] })
    const [loading, setLoading] = useState(true)
    const [message, setMessage] = useState(null)
    const [token, setToken] = useState(null)
    const [authChecking, setAuthChecking] = useState(true)

    // 检查已存储的 Token
    useEffect(() => {
        const checkAuth = async () => {
            // 检查 localStorage 或 sessionStorage
            const storedToken = localStorage.getItem('ds2api_token') || sessionStorage.getItem('ds2api_token')
            const expiresAt = parseInt(localStorage.getItem('ds2api_token_expires') || sessionStorage.getItem('ds2api_token_expires') || '0')

            if (storedToken && expiresAt > Date.now()) {
                // 验证 token 是否有效
                try {
                    const res = await fetch('/admin/verify', {
                        headers: { 'Authorization': `Bearer ${storedToken}` }
                    })
                    if (res.ok) {
                        setToken(storedToken)
                    } else {
                        // Token 无效，清除
                        localStorage.removeItem('ds2api_token')
                        localStorage.removeItem('ds2api_token_expires')
                        sessionStorage.removeItem('ds2api_token')
                        sessionStorage.removeItem('ds2api_token_expires')
                    }
                } catch {
                    // 网络错误，保留 token 重试
                    setToken(storedToken)
                }
            }
            setAuthChecking(false)
        }
        checkAuth()
    }, [])

    // 带认证的 fetch
    const authFetch = async (url, options = {}) => {
        const headers = {
            ...options.headers,
            'Authorization': `Bearer ${token}`
        }
        const res = await fetch(url, { ...options, headers })

        // 401 时自动登出
        if (res.status === 401) {
            handleLogout()
            throw new Error('认证已过期，请重新登录')
        }
        return res
    }

    const fetchConfig = async () => {
        if (!token) return
        try {
            setLoading(true)
            const res = await authFetch('/admin/config')
            if (res.ok) {
                const data = await res.json()
                setConfig(data)
            }
        } catch (e) {
            console.error('获取配置失败:', e)
            showMessage('error', e.message)
        } finally {
            setLoading(false)
        }
    }

    useEffect(() => {
        if (token) {
            fetchConfig()
        }
    }, [token])

    const showMessage = (type, text) => {
        setMessage({ type, text })
        setTimeout(() => setMessage(null), 5000)
    }

    const handleLogin = (newToken) => {
        setToken(newToken)
    }

    const handleLogout = () => {
        setToken(null)
        localStorage.removeItem('ds2api_token')
        localStorage.removeItem('ds2api_token_expires')
        sessionStorage.removeItem('ds2api_token')
        sessionStorage.removeItem('ds2api_token_expires')
    }

    const renderTab = () => {
        switch (activeTab) {
            case 'accounts':
                return <AccountManager config={config} onRefresh={fetchConfig} onMessage={showMessage} authFetch={authFetch} />
            case 'test':
                return <ApiTester config={config} onMessage={showMessage} authFetch={authFetch} />
            case 'import':
                return <BatchImport onRefresh={fetchConfig} onMessage={showMessage} authFetch={authFetch} />
            case 'vercel':
                return <VercelSync onMessage={showMessage} authFetch={authFetch} />
            default:
                return null
        }
    }

    // 认证检查中
    if (authChecking) {
        return (
            <div className="app">
                <div className="login-container">
                    <div className="login-card">
                        <div className="empty-state">
                            <span className="loading"></span> 检查登录状态...
                        </div>
                    </div>
                </div>
            </div>
        )
    }

    // 未登录
    if (!token) {
        return (
            <div className="app">
                {message && (
                    <div className={`alert alert-${message.type}`}>
                        {message.text}
                    </div>
                )}
                <Login onLogin={handleLogin} onMessage={showMessage} />
            </div>
        )
    }

    // 已登录
    return (
        <div className="app">
            <header className="header">
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div>
                        <h1>DS2API Admin</h1>
                        <p>账号管理 · API 测试 · Vercel 部署</p>
                    </div>
                    <button className="btn btn-secondary btn-sm" onClick={handleLogout}>
                        🚪 登出
                    </button>
                </div>
            </header>

            {message && (
                <div className={`alert alert-${message.type}`}>
                    {message.text}
                </div>
            )}

            <div className="stats">
                <div className="stat">
                    <div className="stat-value">{config.keys?.length || 0}</div>
                    <div className="stat-label">API Keys</div>
                </div>
                <div className="stat">
                    <div className="stat-value">{config.accounts?.length || 0}</div>
                    <div className="stat-label">账号</div>
                </div>
            </div>

            <div className="tabs">
                {TABS.map(tab => (
                    <button
                        key={tab.id}
                        className={`tab ${activeTab === tab.id ? 'active' : ''}`}
                        onClick={() => setActiveTab(tab.id)}
                    >
                        {tab.label}
                    </button>
                ))}
            </div>

            {loading ? (
                <div className="card">
                    <div className="empty-state">
                        <span className="loading"></span> 加载中...
                    </div>
                </div>
            ) : (
                renderTab()
            )}
        </div>
    )
}

