const MODE_STYLES = {
    shadow:  'bg-amber-500/15 text-amber-700 border-amber-400/40',
    enforce: 'bg-emerald-500/15 text-emerald-700 border-emerald-400/40',
    off:     'bg-muted/60 text-muted-foreground border-border',
}

function ModeBadge({ mode }) {
    const style = MODE_STYLES[mode] || MODE_STYLES.off
    return (
        <span className={`inline-flex items-center px-2 py-0.5 rounded-md border text-xs font-mono font-medium ${style}`}>
            {mode || 'off'}
        </span>
    )
}

export default function FeatureFlagsSection({ t, form }) {
    const parserV2 = form.parser_v2 || { mode: 'off', env_override: false }

    return (
        <div className="bg-card border border-border rounded-xl p-5 space-y-4">
            <h3 className="font-semibold">{t('settings.featureFlagsTitle')}</h3>
            <div className="space-y-3">
                <div className="flex items-center justify-between rounded-lg border border-border bg-background/60 px-4 py-3">
                    <div className="space-y-0.5">
                        <span className="text-sm font-medium block">{t('settings.parserV2ModeLabel')}</span>
                        <span className="text-xs text-muted-foreground block">{t('settings.parserV2ModeDesc')}</span>
                    </div>
                    <div className="flex items-center gap-2 shrink-0 ml-4">
                        {parserV2.env_override && (
                            <span className="text-xs text-muted-foreground border border-border rounded px-1.5 py-0.5">
                                {t('settings.envOverride')}
                            </span>
                        )}
                        <ModeBadge mode={parserV2.mode} />
                    </div>
                </div>
            </div>
        </div>
    )
}
