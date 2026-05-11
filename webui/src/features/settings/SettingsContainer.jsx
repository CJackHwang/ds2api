import { AlertTriangle, Save } from 'lucide-react'

import { useI18n } from '../../i18n'
import { useSettingsForm } from './useSettingsForm'
import SecuritySection from './SecuritySection'
import RuntimeSection from './RuntimeSection'
import BehaviorSection from './BehaviorSection'
import CurrentInputFileSection from './CurrentInputFileSection'
import AutoDeleteSection from './AutoDeleteSection'
import FeatureFlagsSection from './FeatureFlagsSection'
import ModelSection from './ModelSection'
import BackupSection from './BackupSection'

export default function SettingsContainer({ onRefresh, onMessage, authFetch, onForceLogout, isVercel = false }) {
    const { t } = useI18n()
    const apiFetch = authFetch || fetch

    const {
        form,
        setForm,
        loading,
        saving,
        changingPassword,
        importing,
        exportData,
        importMode,
        setImportMode,
        importText,
        setImportText,
        newPassword,
        setNewPassword,
        consecutiveFailures,
        autoFetchPaused,
        lastError,
        settingsMeta,
        syncHintVisible,
        retryLoadSettings,
        saveSettings,
        updatePassword,
        loadExportData,
        downloadExportFile,
        loadImportFile,
        doImport,
    } = useSettingsForm({
        apiFetch,
        t,
        onMessage,
        onRefresh,
        onForceLogout,
        isVercel,
    })

    return (
        <div className="space-y-6">
            {autoFetchPaused && (
                <div className="p-4 rounded-lg border border-destructive/30 bg-destructive/10 text-destructive flex items-center justify-between gap-4">
                    <div className="flex items-center gap-2">
                        <AlertTriangle className="w-4 h-4" />
                        <span className="text-sm">
                            {t('settings.autoFetchPaused', { count: consecutiveFailures, error: lastError || t('settings.loadFailed') })}
                        </span>
                    </div>
                    <button
                        type="button"
                        onClick={retryLoadSettings}
                        className="px-3 py-1.5 text-xs rounded-md border border-destructive/40 hover:bg-destructive/10"
                    >
                        {t('settings.retryLoad')}
                    </button>
                </div>
            )}
            {settingsMeta.default_password_warning && (
                <div className="p-4 rounded-lg border border-amber-300/30 bg-amber-500/10 text-amber-700 flex items-center gap-2">
                    <AlertTriangle className="w-4 h-4" />
                    <span className="text-sm">{t('settings.defaultPasswordWarning')}</span>
                </div>
            )}
            {syncHintVisible && (
                <div className="p-4 rounded-lg border border-amber-300/30 bg-amber-500/10 text-amber-700 flex items-center gap-2">
                    <AlertTriangle className="w-4 h-4" />
                    <span className="text-sm">{t('settings.vercelSyncHint')}</span>
                </div>
            )}

            <SecuritySection
                t={t}
                form={form}
                setForm={setForm}
                newPassword={newPassword}
                setNewPassword={setNewPassword}
                changingPassword={changingPassword}
                onUpdatePassword={updatePassword}
            />

            <RuntimeSection t={t} form={form} setForm={setForm} />

            <BehaviorSection t={t} form={form} setForm={setForm} />

            <CurrentInputFileSection t={t} form={form} setForm={setForm} />

            <AutoDeleteSection t={t} form={form} setForm={setForm} />

            <FeatureFlagsSection t={t} form={form} />

            <ModelSection t={t} form={form} setForm={setForm} />

            <BackupSection
                t={t}
                importMode={importMode}
                setImportMode={setImportMode}
                importing={importing}
                onLoadExportData={loadExportData}
                onDownloadExportFile={downloadExportFile}
                onImport={doImport}
                onImportFileChange={loadImportFile}
                importText={importText}
                setImportText={setImportText}
                exportData={exportData}
            />

            <div className="flex justify-end">
                <button
                    type="button"
                    onClick={saveSettings}
                    disabled={loading || saving}
                    className="px-4 py-2 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 flex items-center gap-2"
                >
                    <Save className="w-4 h-4" />
                    {saving ? t('settings.saving') : t('settings.save')}
                </button>
            </div>
        </div>
    )
}
