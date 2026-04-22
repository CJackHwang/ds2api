import { CheckCircle2, Database, ExternalLink, XCircle } from 'lucide-react'

export default function VercelSyncStatus({ t, result }) {
    if (!result) {
        return null
    }

    return (
        <div className={`p-6 rounded-xl border ${result.success ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-destructive/10 border-destructive/20'} animate-in fade-in slide-in-from-right-4`}>
            <div className="flex items-start gap-4">
                {result.success ? (
                    <div className="p-2 bg-emerald-500 text-white rounded-full shadow-lg shadow-emerald-500/30">
                        <CheckCircle2 className="w-6 h-6" />
                    </div>
                ) : (
                    <div className="p-2 bg-destructive text-white rounded-full shadow-lg shadow-destructive/30">
                        <XCircle className="w-6 h-6" />
                    </div>
                )}
                <div className="space-y-2">
                    <h3 className={`font-semibold text-lg ${result.success ? 'text-emerald-500' : 'text-destructive'}`}>
                        {result.success ? t('vercel.syncSucceeded') : t('vercel.syncFailedLabel')}
                    </h3>
                    <p className="text-sm opacity-90">{result.message}</p>

                    {result.storage_backend === 'redis' && (
                        <div className="inline-flex items-center gap-2 rounded-full border border-emerald-500/20 bg-background/80 px-3 py-1 text-xs text-muted-foreground">
                            <Database className="w-3.5 h-3.5" />
                            <span>{t('vercel.syncedToRedis', { key: result.redis_key || 'ds2api:config' })}</span>
                        </div>
                    )}

                    {result.deployment_url && (
                        <div className="pt-3 mt-3 border-t border-emerald-500/20">
                            <a href={`https://${result.deployment_url}`} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-sm font-medium hover:underline">
                                {t('vercel.openDeployment')} <ExternalLink className="w-3 h-3" />
                            </a>
                        </div>
                    )}
                </div>
            </div>
        </div>
    )
}
