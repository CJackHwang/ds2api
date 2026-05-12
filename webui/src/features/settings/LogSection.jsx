export default function LogSection({ t, form, setForm }) {
    return (
        <div className="bg-card border border-border rounded-xl p-5 space-y-4">
            <h3 className="font-semibold">{t('settings.logTitle')}</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                <label className="text-sm space-y-2">
                    <span className="text-muted-foreground">{t('settings.logLevel')}</span>
                    <select
                        value={form.log.level}
                        onChange={(e) => setForm((prev) => ({
                            ...prev,
                            log: { ...prev.log, level: e.target.value },
                        }))}
                        className="w-full bg-background border border-border rounded-lg px-3 py-2"
                    >
                        <option value="debug">{t('settings.logLevelDebug')}</option>
                        <option value="info">{t('settings.logLevelInfo')}</option>
                        <option value="warn">{t('settings.logLevelWarn')}</option>
                        <option value="error">{t('settings.logLevelError')}</option>
                    </select>
                </label>
                <label className="text-sm space-y-2">
                    <span className="text-muted-foreground">{t('settings.logFile')}</span>
                    <input
                        type="text"
                        value={form.log.file}
                        onChange={(e) => setForm((prev) => ({
                            ...prev,
                            log: { ...prev.log, file: e.target.value },
                        }))}
                        placeholder="/var/log/ds2api.log"
                        className="w-full bg-background border border-border rounded-lg px-3 py-2"
                    />
                </label>
                <label className="text-sm space-y-2">
                    <span className="text-muted-foreground">{t('settings.logMaxSize')}</span>
                    <input
                        type="number"
                        min={1}
                        value={form.log.max_size_mb}
                        onChange={(e) => setForm((prev) => ({
                            ...prev,
                            log: { ...prev.log, max_size_mb: Number(e.target.value || 1) },
                        }))}
                        className="w-full bg-background border border-border rounded-lg px-3 py-2"
                    />
                </label>
                <label className="text-sm space-y-2">
                    <span className="text-muted-foreground">{t('settings.logMaxBackups')}</span>
                    <input
                        type="number"
                        min={0}
                        value={form.log.max_backups}
                        onChange={(e) => setForm((prev) => ({
                            ...prev,
                            log: { ...prev.log, max_backups: Number(e.target.value || 0) },
                        }))}
                        className="w-full bg-background border border-border rounded-lg px-3 py-2"
                    />
                </label>
                <label className="text-sm space-y-2 flex items-center gap-2">
                    <input
                        type="checkbox"
                        checked={form.log.file_enabled}
                        onChange={(e) => setForm((prev) => ({
                            ...prev,
                            log: { ...prev.log, file_enabled: e.target.checked },
                        }))}
                        className="w-4 h-4"
                    />
                    <span className="text-muted-foreground">{t('settings.logFileEnabled')}</span>
                </label>
            </div>
        </div>
    )
}