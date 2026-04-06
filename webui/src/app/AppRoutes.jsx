import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import clsx from 'clsx'

import LandingPage from '../components/LandingPage'
import Login from '../components/Login'
import DashboardShell from '../layout/DashboardShell'
import { useI18n } from '../i18n'
import { useAdminAuth } from './useAdminAuth'
import { useAdminConfig } from './useAdminConfig'

export default function AppRoutes() {
    const { t } = useI18n()
    const navigate = useNavigate()
    const location = useLocation()

    const isProduction = import.meta.env.MODE === 'production'
    const {
        token,
        authChecking,
        message,
        isAdminRoute,
        isVercel,
        showMessage,
        handleLogin,
        handleLogout,
    } = useAdminAuth({ isProduction, location, t })

    const {
        config,
        fetchConfig,
    } = useAdminConfig({ token, showMessage, t })

    if (isAdminRoute && authChecking) {
        return (
            <div className="min-h-screen flex items-center justify-center bg-background">
                <div className="flex flex-col items-center gap-4">
                    <div className="w-8 h-8 border-4 border-primary border-t-transparent rounded-full animate-spin"></div>
                    <p className="text-muted-foreground animate-pulse">{t('auth.checking')}</p>
                </div>
            </div>
        )
    }

    return (
        <Routes>
            {!isProduction && (
                <Route path="/" element={<LandingPage onEnter={() => navigate('/admin')} />} />
            )}
            <Route path={isProduction ? "/*" : "/admin/*"} element={
                token ? (
                    <DashboardShell
                        token={token}
                        onLogout={handleLogout}
                        config={config}
                        fetchConfig={fetchConfig}
                        showMessage={showMessage}
                        message={message}
                        onForceLogout={handleLogout}
                        isVercel={isVercel}
                    />
                ) : (
                    <div className="min-h-screen flex flex-col bg-background relative overflow-hidden">
                        <div className="absolute top-0 left-0 w-full h-full overflow-hidden pointer-events-none z-0">
                            <div className="absolute top-[-10%] right-[-10%] w-[50%] h-[50%] bg-primary/5 rounded-full blur-[120px]"></div>
                            <div className="absolute bottom-[-10%] left-[-10%] w-[50%] h-[50%] bg-accent/5 rounded-full blur-[120px]"></div>
                        </div>

                        {message && (
                            <div className={clsx(
                                "fixed top-4 right-4 z-50 px-4 py-3 rounded-lg shadow-lg border animate-in slide-in-from-top-2 fade-in",
                                message.type === 'error' ? "bg-destructive/10 border-destructive/20 text-destructive" :
                                    "bg-primary/10 border-primary/20 text-primary"
                            )}>
                                {message.text}
                            </div>
                        )}
                        <Login onLogin={handleLogin} onMessage={showMessage} />
                    </div>
                )
            } />
            <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
    )
}
