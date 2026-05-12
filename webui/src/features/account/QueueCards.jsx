import { AlertCircle, CheckCircle2, Server, ShieldCheck } from 'lucide-react'

export default function QueueCards({ queueStatus, t }) {
    if (!queueStatus) {
        return null
    }

    return (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                    <CheckCircle2 className="w-16 h-16" />
                </div>
                <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.available')}</p>
                <div className="mt-2 flex items-baseline gap-2">
                    <span className="text-3xl font-bold text-foreground">{queueStatus.available}</span>
                    <span className="text-xs text-muted-foreground">{t('accountManager.accountsUnit')}</span>
                </div>
            </div>
            <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                    <Server className="w-16 h-16" />
                </div>
                <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.inUse')}</p>
                <div className="mt-2 flex items-baseline gap-2">
                    <span className="text-3xl font-bold text-foreground">{queueStatus.in_use}</span>
                    <span className="text-xs text-muted-foreground">{t('accountManager.threadsUnit')}</span>
                </div>
            </div>
            <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                    <ShieldCheck className="w-16 h-16" />
                </div>
                <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.totalPool')}</p>
                <div className="mt-2 flex items-baseline gap-2">
                    <span className="text-3xl font-bold text-foreground">{queueStatus.total}</span>
                    <span className="text-xs text-muted-foreground">{t('accountManager.accountsUnit')}</span>
                </div>
            </div>
            <div className={`border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group transition-colors ${
                queueStatus.failed > 0
                    ? 'bg-destructive/5 border-destructive/30'
                    : 'bg-card border-border'
            }`}>
                <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                    <AlertCircle className="w-16 h-16" />
                </div>
                <p className={`text-xs font-medium uppercase tracking-widest ${
                    queueStatus.failed > 0 ? 'text-destructive' : 'text-muted-foreground'
                }`}>{t('accountManager.failedPool')}</p>
                <div className="mt-2 flex items-baseline gap-2">
                    <span className={`text-3xl font-bold ${
                        queueStatus.failed > 0 ? 'text-destructive' : 'text-foreground'
                    }`}>{queueStatus.failed ?? 0}</span>
                    <span className="text-xs text-muted-foreground">{t('accountManager.accountsUnit')}</span>
                </div>
                {queueStatus.failed > 0 && (
                    <p className="text-[10px] text-destructive/70 mt-1 truncate">
                        {t('accountManager.failedPoolHint')}
                    </p>
                )}
            </div>
        </div>
    )
}
