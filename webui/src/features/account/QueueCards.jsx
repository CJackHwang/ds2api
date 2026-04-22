import { Activity, CheckCircle2, Database, Server, ShieldCheck, XCircle } from 'lucide-react'

function cardClass() {
    return 'bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group'
}

function iconClass() {
    return 'absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity'
}

function resolveStorageLabel(t, backend) {
    switch (backend) {
        case 'redis':
            return t('accountManager.storageRedis')
        case 'env':
            return t('accountManager.storageEnv')
        case 'file':
            return t('accountManager.storageFile')
        default:
            return t('accountManager.storageMemory')
    }
}

export default function QueueCards({ queueStatus, t, config }) {
    if (!queueStatus) {
        return null
    }

    const calls = queueStatus.calls || {}
    const storageLabel = resolveStorageLabel(t, config?.storage_backend)
    const flowSteps = [
        t('accountManager.lifecycleStepOne'),
        t('accountManager.lifecycleStepTwo'),
        t('accountManager.lifecycleStepThree'),
        t('accountManager.lifecycleStepFour'),
    ]

    const cards = [
        {
            key: 'available',
            title: t('accountManager.available'),
            value: queueStatus.available,
            unit: t('accountManager.accountsUnit'),
            icon: CheckCircle2,
        },
        {
            key: 'inUse',
            title: t('accountManager.inUse'),
            value: queueStatus.in_use,
            unit: t('accountManager.threadsUnit'),
            icon: Server,
        },
        {
            key: 'totalPool',
            title: t('accountManager.totalPool'),
            value: queueStatus.total,
            unit: t('accountManager.accountsUnit'),
            icon: ShieldCheck,
        },
        {
            key: 'totalCalls',
            title: t('accountManager.totalCalls'),
            value: calls.total || 0,
            unit: t('accountManager.callsUnit'),
            icon: Activity,
        },
        {
            key: 'successCalls',
            title: t('accountManager.successCalls'),
            value: calls.success || 0,
            unit: t('accountManager.callsUnit'),
            icon: CheckCircle2,
        },
        {
            key: 'failedCalls',
            title: t('accountManager.failedCalls'),
            value: calls.failed || 0,
            unit: t('accountManager.callsUnit'),
            icon: XCircle,
        },
    ]

    return (
        <div className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
                {cards.map((card) => {
                    const Icon = card.icon
                    return (
                        <div key={card.key} className={cardClass()}>
                            <div className={iconClass()}>
                                <Icon className="w-16 h-16" />
                            </div>
                            <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{card.title}</p>
                            <div className="mt-2 flex items-baseline gap-2">
                                <span className="text-3xl font-bold text-foreground">{card.value}</span>
                                <span className="text-xs text-muted-foreground">{card.unit}</span>
                            </div>
                        </div>
                    )
                })}
            </div>

            <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                    <div className="max-w-2xl">
                        <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.lifecycleTitle')}</p>
                        <p className="mt-2 text-sm text-muted-foreground">{t('accountManager.lifecycleDesc')}</p>
                    </div>
                    <div className="inline-flex items-center gap-2 rounded-full border border-border bg-secondary/40 px-3 py-1 text-xs text-muted-foreground">
                        <Database className="w-3.5 h-3.5" />
                        <span>{t('accountManager.storageMode', { mode: storageLabel })}</span>
                    </div>
                </div>

                <div className="mt-5 grid grid-cols-1 gap-3 lg:grid-cols-4">
                    {flowSteps.map((step, index) => (
                        <div key={index} className="rounded-xl border border-border bg-background/70 px-4 py-3">
                            <div className="text-xs font-semibold text-primary">{index + 1}</div>
                            <div className="mt-1 text-sm text-foreground">{step}</div>
                        </div>
                    ))}
                </div>

                {config?.redis_config_key && (
                    <p className="mt-4 text-xs text-muted-foreground">
                        {t('accountManager.redisKeyHint', { key: config.redis_config_key })}
                    </p>
                )}
            </div>
        </div>
    )
}
