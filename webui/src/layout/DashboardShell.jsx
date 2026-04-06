import { useCallback, useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
    LayoutDashboard,
    Upload,
    Cloud,
    Settings as SettingsIcon,
    LogOut,
    Menu,
    X,
    Server,
    Users,
    Globe
} from 'lucide-react'
import clsx from 'clsx'

import AccountManagerContainer from '../features/account/AccountManagerContainer'
import ApiTesterContainer from '../features/apiTester/ApiTesterContainer'
import BatchImport from '../components/BatchImport'
import VercelSyncContainer from '../features/vercel/VercelSyncContainer'
import SettingsContainer from '../features/settings/SettingsContainer'
import ProxyManagerContainer from '../features/proxy/ProxyManagerContainer'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'

export default function DashboardShell({ token, onLogout, config, fetchConfig, showMessage, message, onForceLogout, isVercel }) {
    const { t } = useI18n()
    const location = useLocation()
    const navigate = useNavigate()
    const [sidebarOpen, setSidebarOpen] = useState(false)

    const navItems = [
        { id: 'accounts', label: t('nav.accounts.label'), icon: Users, description: t('nav.accounts.desc') },
        { id: 'proxies', label: t('nav.proxies.label'), icon: Globe, description: t('nav.proxies.desc') },
        { id: 'test', label: t('nav.test.label'), icon: Server, description: t('nav.test.desc') },
        { id: 'import', label: t('nav.import.label'), icon: Upload, description: t('nav.import.desc') },
        { id: 'vercel', label: t('nav.vercel.label'), icon: Cloud, description: t('nav.vercel.desc') },
        { id: 'settings', label: t('nav.settings.label'), icon: SettingsIcon, description: t('nav.settings.desc') },
    ]

    const tabIds = new Set(navItems.map(item => item.id))
    const pathSegments = location.pathname.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean)
    const routeSegments = pathSegments[0] === 'admin' ? pathSegments.slice(1) : pathSegments
    const pathTab = routeSegments[0] || ''
    const activeTab = tabIds.has(pathTab) ? pathTab : 'accounts'
    const adminBasePath = pathSegments[0] === 'admin' ? '/admin' : ''

    const navigateToTab = useCallback((tabID) => {
        const nextPath = tabID === 'accounts'
            ? `${adminBasePath || ''}/`
            : `${adminBasePath}/${tabID}`
        navigate(nextPath)
        setSidebarOpen(false)
    }, [adminBasePath, navigate])

    const authFetch = useCallback(async (url, options = {}) => {
        const headers = {
            ...options.headers,
            'Authorization': `Bearer ${token}`
        }
        const res = await fetch(url, { ...options, headers })

        if (res.status === 401) {
            onLogout()
            throw new Error(t('auth.expired'))
        }
        return res
    }, [onLogout, t, token])


    const [versionInfo, setVersionInfo] = useState(null)

    useEffect(() => {
        let disposed = false
        async function loadVersion() {
            try {
                const res = await authFetch('/admin/version')
                const data = await res.json()
                if (!disposed) {
                    setVersionInfo(data)
                }
            } catch (_err) {
                if (!disposed) {
                    setVersionInfo(null)
                }
            }
        }
        loadVersion()
        return () => {
            disposed = true
        }
    }, [authFetch])
    const renderTab = () => {
        switch (activeTab) {
            case 'accounts':
                return <AccountManagerContainer config={config} onRefresh={fetchConfig} onMessage={showMessage} authFetch={authFetch} />
            case 'proxies':
                return <ProxyManagerContainer config={config} onRefresh={fetchConfig} onMessage={showMessage} authFetch={authFetch} />
            case 'test':
                return <ApiTesterContainer config={config} onMessage={showMessage} authFetch={authFetch} />
            case 'import':
                return <BatchImport onRefresh={fetchConfig} onMessage={showMessage} authFetch={authFetch} />
            case 'vercel':
                return <VercelSyncContainer onMessage={showMessage} authFetch={authFetch} isVercel={isVercel} config={config} />
            case 'settings':
                return <SettingsContainer onRefresh={fetchConfig} onMessage={showMessage} authFetch={authFetch} onForceLogout={onForceLogout} isVercel={isVercel} />
            default:
                return null
        }
    }

    return (
        <div className="flex h-screen bg-background overflow-hidden text-foreground">
            {sidebarOpen && (
                <div
                    className="fixed inset-0 bg-background/80 backdrop-blur-sm z-40 lg:hidden"
                    onClick={() => setSidebarOpen(false)}
                />
            )}

            <aside className={clsx(
                "fixed lg:static inset-y-0 left-0 z-50 w-64 bg-card border-r border-border transition-transform duration-300 ease-in-out lg:transform-none flex flex-col shadow-2xl lg:shadow-none",
                sidebarOpen ? "translate-x-0" : "-translate-x-full"
            )}>
                <div className="p-6">
                    <div className="flex items-center gap-2.5 font-bold text-xl text-foreground tracking-tight">
                        <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center text-primary-foreground shadow-lg shadow-primary/20">
                            <LayoutDashboard className="w-5 h-5" />
                        </div>
                        <span>DS2API</span>
                    </div>
                    <div className="flex items-center justify-between mt-2">
                        <p className="text-[10px] text-muted-foreground font-semibold tracking-[0.1em] uppercase opacity-60 px-1">{t('sidebar.onlineAdminConsole')}</p>
                        <LanguageToggle />
                    </div>
                </div>

                <nav className="flex-1 px-3 space-y-1 overflow-y-auto pt-2">
                    {navItems.map((item) => {
                        const Icon = item.icon
                        const isActive = activeTab === item.id
                        return (
                            <button
                                key={item.id}
                                onClick={() => {
                                    navigateToTab(item.id)
                                }}
                                className={clsx(
                                    "w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-all duration-200 group border",
                                    isActive
                                        ? "bg-secondary text-primary border-border shadow-sm"
                                        : "text-muted-foreground border-transparent hover:bg-secondary/80 hover:text-foreground"
                                )}
                            >
                                <Icon className={clsx("w-4 h-4 transition-colors", isActive ? "text-primary" : "text-muted-foreground group-hover:text-foreground")} />
                                <span className="flex-1 text-left">{item.label}</span>
                                {isActive && <div className="w-1.5 h-1.5 rounded-full bg-primary" />}
                            </button>
                        )
                    })}
                </nav>

                <div className="p-4 border-t border-border bg-card">
                    <div className="space-y-4">
                        <div className="flex items-center justify-between text-sm px-1">
                            <span className="text-muted-foreground font-semibold text-[10px] uppercase tracking-wider">{t('sidebar.systemStatus')}</span>
                            <span className="flex items-center gap-1.5 text-[10px] font-bold text-emerald-500 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
                                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
                                {t('sidebar.statusOnline')}
                            </span>
                        </div>
                        <div className="grid grid-cols-2 gap-2">
                            <div className="bg-background rounded-lg p-3 border border-border shadow-sm">
                                <div className="text-[9px] text-muted-foreground font-bold uppercase tracking-wider mb-0.5 opacity-70">{t('sidebar.accounts')}</div>
                                <div className="text-lg font-bold text-foreground leading-tight">{config.accounts?.length || 0}</div>
                            </div>
                            <div className="bg-background rounded-lg p-3 border border-border shadow-sm">
                                <div className="text-[9px] text-muted-foreground font-bold uppercase tracking-wider mb-0.5 opacity-70">{t('sidebar.keys')}</div>
                                <div className="text-lg font-bold text-foreground">{config.keys?.length || 0}</div>
                            </div>
                        </div>
                        <div className="bg-background rounded-lg p-3 border border-border shadow-sm">
                            <div className="text-[9px] text-muted-foreground font-bold uppercase tracking-wider mb-1 opacity-70">{t('sidebar.version')}</div>
                            <div className="text-xs font-semibold text-foreground">{versionInfo?.current_tag || '-'}</div>
                            {versionInfo?.has_update && (
                                <a
                                    className="inline-flex mt-1 text-[10px] text-amber-500 hover:text-amber-400"
                                    href={versionInfo?.release_url || 'https://github.com/CJackHwang/ds2api/releases/latest'}
                                    target="_blank"
                                    rel="noreferrer"
                                >
                                    {t('sidebar.updateAvailable', { latest: versionInfo.latest_tag || '' })}
                                </a>
                            )}
                        </div>
                        <button
                            onClick={onLogout}
                            className="w-full h-10 flex items-center justify-center gap-2 rounded-lg border border-border text-xs font-medium text-muted-foreground hover:bg-destructive/10 hover:text-destructive hover:border-destructive/20 transition-all"
                        >
                            <LogOut className="w-3.5 h-3.5" />
                            {t('sidebar.signOut')}
                        </button>
                    </div>
                </div>
            </aside>

            <main className="flex-1 flex flex-col min-w-0 overflow-hidden relative">
                <header className="lg:hidden h-14 flex items-center justify-between px-4 border-b border-border bg-card">
                    <div className="flex items-center gap-2">
                        <div className="w-6 h-6 rounded bg-primary flex items-center justify-center text-primary-foreground text-[10px]">
                            <LayoutDashboard className="w-3.5 h-3.5" />
                        </div>
                        <span className="font-semibold text-sm">DS2API</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <LanguageToggle />
                        <button
                            onClick={() => setSidebarOpen(true)}
                            className="p-2 -mr-2 text-muted-foreground hover:text-foreground"
                        >
                            <Menu className="w-5 h-5" />
                        </button>
                    </div>
                </header>

                <div className="flex-1 overflow-auto bg-background p-4 lg:p-10">
                    <div className="max-w-6xl mx-auto space-y-4 lg:space-y-6">
                        <div className="hidden lg:block mb-8">
                            <h1 className="text-3xl font-bold tracking-tight mb-2">
                                {navItems.find(n => n.id === activeTab)?.label}
                            </h1>
                            <p className="text-muted-foreground">
                                {navItems.find(n => n.id === activeTab)?.description}
                            </p>
                        </div>

                        {message && (
                            <div className={clsx(
                                "p-4 rounded-lg border flex items-center gap-3 animate-in fade-in slide-in-from-top-2",
                                message.type === 'error' ? "bg-destructive/10 border-destructive/20 text-destructive" :
                                    "bg-emerald-500/10 border-emerald-500/20 text-emerald-500"
                            )}>
                                {message.type === 'error' ? <X className="w-5 h-5" /> : <div className="w-5 h-5 rounded-full border-2 border-emerald-500 flex items-center justify-center text-[10px]">✓</div>}
                                {message.text}
                            </div>
                        )}

                        <div className="animate-in fade-in duration-500">
                            {renderTab()}
                        </div>
                    </div>
                </div>
            </main>
        </div>
    )
}
