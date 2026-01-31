import { useState } from 'react'

export default function Login({ onLogin, onMessage }) {
    const [adminKey, setAdminKey] = useState('')
    const [loading, setLoading] = useState(false)
    const [remember, setRemember] = useState(true)

    const handleLogin = async (e) => {
        e.preventDefault()
        setLoading(true)

        try {
            const res = await fetch('/admin/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ admin_key: adminKey }),
            })

            const data = await res.json()

            if (res.ok && data.success) {
                // 存储 token
                const storage = remember ? localStorage : sessionStorage
                storage.setItem('ds2api_token', data.token)
                storage.setItem('ds2api_token_expires', Date.now() + data.expires_in * 1000)

                onLogin(data.token)
                if (data.message) {
                    onMessage('warning', data.message)
                }
            } else {
                onMessage('error', data.detail || '登录失败')
            }
        } catch (e) {
            onMessage('error', '网络错误: ' + e.message)
        } finally {
            setLoading(false)
        }
    }

    return (
        <div className="login-container">
            <div className="login-card">
                <div className="login-header">
                    <h1>🔐 DS2API Admin</h1>
                    <p>请输入管理密钥登录</p>
                </div>

                <form onSubmit={handleLogin}>
                    <div className="form-group">
                        <label className="form-label">管理密钥</label>
                        <input
                            type="password"
                            className="form-input"
                            placeholder="输入 DS2API_ADMIN_KEY..."
                            value={adminKey}
                            onChange={e => setAdminKey(e.target.value)}
                            autoFocus
                        />
                    </div>

                    <div className="form-group" style={{ flexDirection: 'row', alignItems: 'center', gap: '0.5rem' }}>
                        <input
                            type="checkbox"
                            id="remember"
                            checked={remember}
                            onChange={e => setRemember(e.target.checked)}
                        />
                        <label htmlFor="remember" style={{ cursor: 'pointer' }}>
                            记住登录状态
                        </label>
                    </div>

                    <button
                        type="submit"
                        className="btn btn-primary"
                        disabled={loading}
                        style={{ width: '100%', justifyContent: 'center' }}
                    >
                        {loading ? <span className="loading"></span> : '🚀 登录'}
                    </button>
                </form>

                <div className="login-footer">
                    <p>Session 有效期 24 小时</p>
                </div>
            </div>
        </div>
    )
}
