import { useState } from 'react'
import { Key, ArrowRight, ShieldCheck, Lock, Check } from 'lucide-react'
import clsx from 'clsx'
import { useI18n } from '../i18n'
import LanguageToggle from './LanguageToggle'

export default function Login({ onLogin, onMessage }) {
    const { t } = useI18n()
    const [adminKey, setAdminKey] = useState('')
    const [loading, setLoading] = useState(false)
    const [remember, setRemember] = useState(true)

    const handleLogin = async (e) => {
        e.preventDefault()
        if (!adminKey.trim()) return

        setLoading(true)

        try {
            const res = await fetch('/admin/login', {
                method: 'POST',
                cache: 'no-store',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ admin_key: adminKey }),
            })

            const data = await res.json()

            if (res.ok && data.success) {
                const storage = remember ? localStorage : sessionStorage
                storage.setItem('ds2api_token', data.token)
                storage.setItem('ds2api_token_expires', Date.now() + data.expires_in * 1000)

                onLogin(data.token)
                if (data.message) {
                    onMessage('warning', data.message)
                }
            } else {
                onMessage('error', data.detail || t('login.signInFailed'))
            }
        } catch (e) {
            onMessage('error', t('login.networkError', { error: e.message }))
        } finally {
            setLoading(false)
        }
    }

    return (
        <div className="min-h-screen w-full flex flex-col items-center justify-center p-4 bg-background text-foreground">
            <div className="absolute top-6 right-6">
                <LanguageToggle />
            </div>

            <div className="w-full max-w-[400px] relative z-10 animate-in fade-in zoom-in-95 duration-200">
                <div className="w-full bg-card border border-border rounded-xl p-8 shadow-sm">
                    <div className="text-center space-y-2 mb-8 animate-in fade-in slide-in-from-top-4 duration-500">
                        <div className="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-primary/10 text-primary mb-2">
                            <Lock className="w-6 h-6" />
                        </div>
                        <h1 className="text-3xl font-bold tracking-tight text-foreground">{t('login.welcome')}</h1>
                        <p className="text-sm text-muted-foreground/80">{t('login.subtitle')}</p>
                    </div>

                    <form onSubmit={handleLogin} className="space-y-5 animate-in fade-in slide-in-from-bottom-4 duration-700 delay-150">
                        <div className="space-y-2">
                            <label className="text-xs font-semibold text-muted-foreground uppercase tracking-widest ml-1">{t('login.adminKeyLabel')}</label>
                            <div className="relative group">
                                <div className="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-muted-foreground group-focus-within:text-primary transition-colors">
                                    <Key className="w-4 h-4" />
                                </div>
                                <input
                                    type="password"
                                    className="w-full bg-[#09090b] border border-border rounded-xl pl-10 pr-4 py-3 text-sm focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all placeholder:text-muted-foreground/30 text-foreground"
                                    placeholder={t('login.adminKeyPlaceholder')}
                                    value={adminKey}
                                    onChange={e => setAdminKey(e.target.value)}
                                    autoFocus
                                />
                            </div>
                        </div>

                        <div className="flex items-center justify-between px-1">
                            <label className="flex items-center gap-2.5 cursor-pointer group">
                                <div className="relative flex items-center">
                                    <input
                                        type="checkbox"
                                        className="peer sr-only"
                                        checked={remember}
                                        onChange={e => setRemember(e.target.checked)}
                                    />
                                    <div className="w-[18px] h-[18px] bg-secondary border border-border rounded-md peer-checked:bg-primary peer-checked:border-primary transition-all shadow-sm"></div>
                                    <Check className="absolute inset-0 m-auto w-3 h-3 text-primary-foreground opacity-0 peer-checked:opacity-100 transition-opacity stroke-[3]" />
                                </div>
                                <span className="text-xs font-medium text-muted-foreground group-hover:text-foreground transition-colors">{t('login.rememberSession')}</span>
                            </label>
                        </div>

                        <button
                            type="submit"
                            disabled={loading}
                            className="w-full h-12 flex items-center justify-center gap-2 bg-primary text-primary-foreground rounded-xl hover:bg-primary/90 transition-all font-semibold text-sm shadow-lg shadow-primary/20 hover:shadow-primary/30 disabled:opacity-50 disabled:shadow-none"
                        >
                            {loading ? (
                                <div className="w-5 h-5 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
                            ) : (
                                <div className="flex items-center gap-2">
                                    <span>{t('login.signIn')}</span>
                                    <ArrowRight className="w-4 h-4" />
                                </div>
                            )}
                        </button>
                    </form>

                    <div className="mt-6 pt-6 border-t border-border flex justify-center">
                        <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground/60 font-medium tracking-wide uppercase">
                            <ShieldCheck className="w-3 h-3" />
                            <span>{t('login.secureConnection')}</span>
                        </div>
                    </div>
                </div>

                <div className="mt-8 text-center">
                    <p className="text-[10px] text-muted-foreground/30 font-mono text-center">{t('login.adminPortal')}</p>
                </div>
            </div>
        </div>
    )
}
