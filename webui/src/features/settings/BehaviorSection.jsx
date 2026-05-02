export default function BehaviorSection({ t, form, setForm }) {
    return (
        <div className="bg-card border border-border rounded-xl p-6 space-y-6">
            <h3 className="text-base font-semibold">{t('settings.behaviorTitle')}</h3>

            <section className="space-y-4">
                <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wider">{t('settings.multiTurnSection')}</h4>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <label className="flex items-start gap-3 rounded-lg border border-border bg-background/60 p-4 hover:bg-accent/5 transition-colors cursor-pointer">
                        <input
                            type="checkbox"
                            checked={Boolean(form.keep_session?.enabled ?? false)}
                            onChange={(e) => setForm((prev) => ({
                                ...prev,
                                keep_session: { ...prev.keep_session, enabled: e.target.checked },
                            }))}
                            className="mt-0.5 h-4 w-4 rounded border-border accent-primary"
                        />
                        <div className="space-y-1 min-w-0">
                            <span className="text-sm font-medium block leading-tight">{t('settings.keepSessionEnabled')}</span>
                            <span className="text-xs text-muted-foreground block leading-relaxed">{t('settings.keepSessionDesc')}</span>
                        </div>
                    </label>

                    <label className="flex items-start gap-3 rounded-lg border border-border bg-background/60 p-4 hover:bg-accent/5 transition-colors cursor-pointer">
                        <input
                            type="checkbox"
                            checked={Boolean(form.keep_session?.supplement_file_enabled ?? true)}
                            onChange={(e) => setForm((prev) => ({
                                ...prev,
                                keep_session: { ...prev.keep_session, supplement_file_enabled: e.target.checked },
                            }))}
                            className="mt-0.5 h-4 w-4 rounded border-border accent-primary"
                        />
                        <div className="space-y-1 min-w-0">
                            <span className="text-sm font-medium block leading-tight">{t('settings.supplementFileEnabled')}</span>
                            <span className="text-xs text-muted-foreground block leading-relaxed">{t('settings.supplementFileEnabledDesc')}</span>
                        </div>
                    </label>
                </div>
            </section>

            <section className="space-y-4">
                <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wider">{t('settings.fileConfigSection')}</h4>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <label className="space-y-2 block">
                        <span className="text-sm font-medium block">{t('settings.historyFilename')}</span>
                        <input
                            type="text"
                            value={form.keep_session?.history_filename || 'DS2API_HISTORY.txt'}
                            onChange={(e) => setForm((prev) => ({
                                ...prev,
                                keep_session: { ...prev.keep_session, history_filename: e.target.value },
                            }))}
                            placeholder="DS2API_HISTORY.txt"
                            className="w-full bg-background border border-border rounded-lg px-3 py-2 text-sm focus:border-primary/50 focus:outline-none transition-colors"
                        />
                        <p className="text-xs text-muted-foreground">{t('settings.historyFilenameDesc')}</p>
                    </label>

                    <label className="space-y-2 block">
                        <span className="text-sm font-medium block">{t('settings.supplementFilename')}</span>
                        <input
                            type="text"
                            value={form.keep_session?.supplement_filename || 'supplement.txt'}
                            onChange={(e) => setForm((prev) => ({
                                ...prev,
                                keep_session: { ...prev.keep_session, supplement_filename: e.target.value },
                            }))}
                            placeholder="supplement.txt"
                            className="w-full bg-background border border-border rounded-lg px-3 py-2 text-sm focus:border-primary/50 focus:outline-none transition-colors"
                        />
                        <p className="text-xs text-muted-foreground">{t('settings.supplementFilenameDesc')}</p>
                    </label>
                </div>
            </section>

            <section className="space-y-4">
                <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wider">{t('settings.otherSettingsSection')}</h4>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <label className="space-y-2 block">
                        <span className="text-sm font-medium block">{t('settings.responsesTTL')}</span>
                        <input
                            type="number"
                            min={30}
                            value={form.responses.store_ttl_seconds}
                            onChange={(e) => setForm((prev) => ({
                                ...prev,
                                responses: { ...prev.responses, store_ttl_seconds: Number(e.target.value || 30) },
                            }))}
                            className="w-full bg-background border border-border rounded-lg px-3 py-2 text-sm focus:border-primary/50 focus:outline-none transition-colors"
                        />
                    </label>

                    <label className="space-y-2 block">
                        <span className="text-sm font-medium block">{t('settings.embeddingsProvider')}</span>
                        <input
                            type="text"
                            value={form.embeddings.provider}
                            onChange={(e) => setForm((prev) => ({
                                ...prev,
                                embeddings: { ...prev.embeddings, provider: e.target.value },
                            }))}
                            placeholder="default / openai / local"
                            className="w-full bg-background border border-border rounded-lg px-3 py-2 text-sm focus:border-primary/50 focus:outline-none transition-colors"
                        />
                    </label>

                    <label className="flex items-start gap-3 rounded-lg border border-border bg-background/60 p-4 hover:bg-accent/5 transition-colors cursor-pointer md:col-span-2">
                        <input
                            type="checkbox"
                            checked={Boolean(form.thinking_injection?.enabled ?? true)}
                            onChange={(e) => setForm((prev) => ({
                                ...prev,
                                thinking_injection: { ...prev.thinking_injection, enabled: e.target.checked },
                            }))}
                            className="mt-0.5 h-4 w-4 rounded border-border accent-primary"
                        />
                        <div className="space-y-1 flex-1 min-w-0">
                            <span className="text-sm font-medium block leading-tight">{t('settings.thinkingInjectionEnabled')}</span>
                            <span className="text-xs text-muted-foreground block leading-relaxed">{t('settings.thinkingInjectionDesc')}</span>
                        </div>
                    </label>

                    <label className="space-y-2 block md:col-span-2">
                        <span className="text-sm font-medium block">{t('settings.thinkingInjectionPrompt')}</span>
                        <textarea
                            rows={4}
                            value={form.thinking_injection?.prompt || ''}
                            placeholder={form.thinking_injection?.default_prompt || ''}
                            onChange={(e) => setForm((prev) => ({
                                ...prev,
                                thinking_injection: { ...prev.thinking_injection, prompt: e.target.value },
                            }))}
                            className="w-full bg-background border border-border rounded-lg px-3 py-2 resize-y min-h-[100px] text-sm focus:border-primary/50 focus:outline-none transition-colors"
                        />
                        <p className="text-xs text-muted-foreground">{t('settings.thinkingInjectionPromptHelp')}</p>
                    </label>
                </div>
            </section>
        </div>
    )
}
