import { Info, Database } from 'lucide-react'

function storageLabel(t, backend) {
    switch (backend) {
        case 'redis':
            return t('vercel.storageRedis')
        case 'env':
            return t('vercel.storageEnv')
        case 'file':
            return t('vercel.storageFile')
        default:
            return t('vercel.storageMemory')
    }
}

export default function VercelGuide({ t, syncStatus, preconfig, config }) {
    const backend = syncStatus?.storage_backend || config?.storage_backend || preconfig?.storage_backend

    return (
        <div className="bg-secondary/20 border border-border rounded-xl p-6">
            <div className="flex items-center justify-between gap-3 mb-4">
                <h3 className="font-semibold flex items-center gap-2">
                    <Info className="w-5 h-5 text-primary" />
                    {t('vercel.howItWorks')}
                </h3>
                <div className="inline-flex items-center gap-2 rounded-full border border-border bg-background/80 px-3 py-1 text-xs text-muted-foreground">
                    <Database className="w-3.5 h-3.5" />
                    <span>{t('vercel.currentStorage', { mode: storageLabel(t, backend) })}</span>
                </div>
            </div>
            <ul className="space-y-4">
                <li className="flex gap-3">
                    <span className="shrink-0 w-6 h-6 rounded-full bg-background border border-border flex items-center justify-center text-xs font-bold text-muted-foreground">1</span>
                    <p className="text-sm text-muted-foreground">{t('vercel.steps.one')}</p>
                </li>
                <li className="flex gap-3">
                    <span className="shrink-0 w-6 h-6 rounded-full bg-background border border-border flex items-center justify-center text-xs font-bold text-muted-foreground">2</span>
                    <p className="text-sm text-muted-foreground">{t('vercel.steps.two')}</p>
                </li>
                <li className="flex gap-3">
                    <span className="shrink-0 w-6 h-6 rounded-full bg-background border border-border flex items-center justify-center text-xs font-bold text-muted-foreground">3</span>
                    <p className="text-sm text-muted-foreground">
                        {t('vercel.steps.three')} <code className="bg-background px-1 py-0.5 rounded border border-border text-xs">DS2API_REDIS_URL</code> / <code className="bg-background px-1 py-0.5 rounded border border-border text-xs">DS2API_REDIS_KEY</code>
                    </p>
                </li>
                <li className="flex gap-3">
                    <span className="shrink-0 w-6 h-6 rounded-full bg-background border border-border flex items-center justify-center text-xs font-bold text-muted-foreground">4</span>
                    <p className="text-sm text-muted-foreground">{t('vercel.steps.four')}</p>
                </li>
            </ul>
        </div>
    )
}
