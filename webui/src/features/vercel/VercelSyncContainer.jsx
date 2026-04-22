import { useI18n } from '../../i18n'
import { useVercelSyncState } from './useVercelSyncState'
import VercelSyncForm from './VercelSyncForm'
import VercelSyncStatus from './VercelSyncStatus'
import VercelGuide from './VercelGuide'

export default function VercelSyncContainer({ onMessage, authFetch, isVercel = false, config = null }) {
    const { t } = useI18n()
    const apiFetch = authFetch || fetch

    const {
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
        result,
        preconfig,
        syncStatus,
        handleManualRefresh,
        handleSync,
    } = useVercelSyncState({
        apiFetch,
        onMessage,
        t,
    })

    return (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 max-w-5xl mx-auto h-[calc(100vh-140px)]">
            <VercelSyncForm
                t={t}
                syncStatus={syncStatus}
                onManualRefresh={handleManualRefresh}
                preconfig={preconfig}
                vercelToken={vercelToken}
                setVercelToken={setVercelToken}
                projectId={projectId}
                setProjectId={setProjectId}
                teamId={teamId}
                setTeamId={setTeamId}
                redisURL={redisURL}
                setRedisURL={setRedisURL}
                redisKey={redisKey}
                setRedisKey={setRedisKey}
                loading={loading}
                onSync={handleSync}
            />

            <div className="space-y-6">
                <VercelSyncStatus t={t} result={result} />
                <VercelGuide t={t} syncStatus={syncStatus} preconfig={preconfig} config={config} isVercel={isVercel} />
            </div>
        </div>
    )
}
