import { ArrowRight, CheckCircle2, Cloud, ExternalLink, RefreshCw, Database } from 'lucide-react'
import clsx from 'clsx'

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

export default function VercelSyncForm({
    t,
    syncStatus,
    onManualRefresh,
    preconfig,
    vercelToken,
    setVercelToken,
    projectId,
    setProjectId,
    teamId,
    setTeamId,
    redisURL,
    setRedisURL,
    redisKey,
    setRedisKey,
    loading,
    onSync,
}) {
    return (
        <div className="bg-card border border-border rounded-xl shadow-sm p-6 space-y-6">
            <div className="border-b border-border pb-6">
                <div className="flex items-center justify-between gap-3">
                    <h2 className="text-xl font-semibold flex items-center gap-2">
                        <Cloud className="w-6 h-6 text-primary" />
                        {t('vercel.title')}
                    </h2>
                    {syncStatus && (
                        <div className={clsx(
                            'flex items-center gap-1.5 text-xs font-semibold px-2.5 py-1 rounded-full border transition-colors',
                            syncStatus.synced
                                ? 'text-emerald-500 bg-emerald-500/10 border-emerald-500/20'
                                : syncStatus.has_synced_before
                                    ? 'text-amber-500 bg-amber-500/10 border-amber-500/20'
                                    : 'text-muted-foreground bg-muted/50 border-border',
                        )}>
                            <span className={clsx(
                                'w-1.5 h-1.5 rounded-full',
                                syncStatus.synced ? 'bg-emerald-500' : syncStatus.has_synced_before ? 'bg-amber-500 animate-pulse' : 'bg-muted-foreground',
                            )} />
                            {syncStatus.synced
                                ? t('vercel.statusSynced')
                                : syncStatus.has_synced_before
                                    ? t('vercel.statusNotSynced')
                                    : t('vercel.statusNeverSynced')}
                        </div>
                    )}
                </div>
                <p className="text-muted-foreground text-sm mt-1">
                    {t('vercel.description')}
                </p>
                <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">
                    <div className="inline-flex items-center gap-2 rounded-full border border-border bg-secondary/30 px-3 py-1 text-muted-foreground">
                        <Database className="w-3.5 h-3.5" />
                        <span>{t('vercel.currentStorage', { mode: storageLabel(t, syncStatus?.storage_backend || preconfig?.storage_backend) })}</span>
                    </div>
                    <button
                        type="button"
                        onClick={onManualRefresh}
                        className="inline-flex items-center gap-1 rounded-full border border-border px-3 py-1 text-muted-foreground hover:bg-secondary/50"
                    >
                        <RefreshCw className="w-3.5 h-3.5" />
                        {t('vercel.manualRefresh')}
                    </button>
                </div>
                {syncStatus?.last_sync_time && (
                    <p className="text-xs text-muted-foreground/70 mt-3">
                        {t('vercel.lastSyncTime', { time: new Date(syncStatus.last_sync_time * 1000).toLocaleString() })}
                    </p>
                )}
                {syncStatus?.draft_differs && (
                    <p className="text-xs text-amber-500 mt-2">
                        {t('vercel.draftDiffers')}
                    </p>
                )}
            </div>

            <div className="space-y-4">
                <div className="space-y-2">
                    <label className="text-sm font-medium flex items-center justify-between">
                        {t('vercel.tokenLabel')}
                        <a href="https://vercel.com/account/tokens" target="_blank" rel="noopener noreferrer" className="text-xs text-primary hover:underline flex items-center gap-1">
                            {t('vercel.getToken')} <ExternalLink className="w-3 h-3" />
                        </a>
                    </label>
                    <div className="relative">
                        <input
                            type="password"
                            className="w-full h-10 px-3 bg-background border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring transition-all pr-10"
                            placeholder={preconfig?.has_token ? t('vercel.tokenPlaceholderPreconfig') : t('vercel.tokenPlaceholder')}
                            value={vercelToken}
                            onChange={e => setVercelToken(e.target.value)}
                        />
                        {preconfig?.has_token && !vercelToken && (
                            <div className="absolute right-3 top-2.5 text-emerald-500">
                                <CheckCircle2 className="w-5 h-5" />
                            </div>
                        )}
                    </div>
                </div>

                <div className="space-y-2">
                    <label className="text-sm font-medium">{t('vercel.projectIdLabel')}</label>
                    <input
                        type="text"
                        className="w-full h-10 px-3 bg-background border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring transition-all"
                        placeholder="prj_xxxxxxxxxxxx or Project Name"
                        value={projectId}
                        onChange={e => setProjectId(e.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">{t('vercel.projectIdHint')}</p>
                </div>

                <div className="space-y-2">
                    <label className="text-sm font-medium flex items-center gap-2">
                        {t('vercel.teamIdLabel')} <span className="text-xs text-muted-foreground font-normal">({t('vercel.optional')})</span>
                    </label>
                    <input
                        type="text"
                        className="w-full h-10 px-3 bg-background border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring transition-all"
                        placeholder="team_xxxxxxxxxxxx"
                        value={teamId}
                        onChange={e => setTeamId(e.target.value)}
                    />
                </div>

                <div className="space-y-2">
                    <label className="text-sm font-medium">{t('vercel.redisUrlLabel')}</label>
                    <div className="relative">
                        <input
                            type="password"
                            className="w-full h-10 px-3 bg-background border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring transition-all pr-10"
                            placeholder={preconfig?.has_redis_url ? t('vercel.redisUrlPlaceholderPreconfig') : t('vercel.redisUrlPlaceholder')}
                            value={redisURL}
                            onChange={e => setRedisURL(e.target.value)}
                        />
                        {preconfig?.has_redis_url && !redisURL && (
                            <div className="absolute right-3 top-2.5 text-emerald-500">
                                <CheckCircle2 className="w-5 h-5" />
                            </div>
                        )}
                    </div>
                </div>

                <div className="space-y-2">
                    <label className="text-sm font-medium">{t('vercel.redisKeyLabel')}</label>
                    <input
                        type="text"
                        className="w-full h-10 px-3 bg-background border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring transition-all"
                        placeholder="ds2api:config"
                        value={redisKey}
                        onChange={e => setRedisKey(e.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">{t('vercel.redisKeyHint')}</p>
                </div>
            </div>

            <div className="pt-4">
                <button
                    onClick={onSync}
                    disabled={loading}
                    className="w-full flex items-center justify-center gap-2 py-3 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-all font-medium text-sm shadow-sm hover:shadow-md disabled:opacity-50 disabled:shadow-none"
                >
                    {loading ? (
                        <span className="flex items-center gap-2">
                            <span className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
                            {t('vercel.syncing')}
                        </span>
                    ) : (
                        <span className="flex items-center gap-2">
                            {t('vercel.syncRedeploy')} <ArrowRight className="w-4 h-4" />
                        </span>
                    )}
                </button>
                <p className="text-xs text-center text-muted-foreground mt-4">
                    {t('vercel.redeployHint')}
                </p>
            </div>
        </div>
    )
}
