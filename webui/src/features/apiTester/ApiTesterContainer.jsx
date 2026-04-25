import clsx from 'clsx'

import { useI18n } from '../../i18n'
import { useApiTesterState } from './useApiTesterState'
import { useChatStreamClient } from './useChatStreamClient'
import ConfigPanel from './ConfigPanel'
import ChatPanel from './ChatPanel'

export default function ApiTesterContainer({ config, onMessage, authFetch }) {
    const { t } = useI18n()

    const {
        model,
        setModel,
        message,
        setMessage,
        attachedFiles,
        setAttachedFiles,
        apiKey,
        setApiKey,
        selectedAccount,
        setSelectedAccount,
        response,
        setResponse,
        loading,
        setLoading,
        streamingContent,
        setStreamingContent,
        streamingThinking,
        setStreamingThinking,
        isStreaming,
        setIsStreaming,
        streamingMode,
        setStreamingMode,
        configExpanded,
        setConfigExpanded,
        abortControllerRef,
    } = useApiTesterState({ t })

    const accounts = config.accounts || []
    const resolveAccountIdentifier = (acc) => {
        if (!acc || typeof acc !== 'object') return ''
        return String(acc.identifier || acc.email || acc.mobile || '').trim()
    }
    const configuredKeys = config.keys || []
    const trimmedApiKey = apiKey.trim()
    const defaultKey = configuredKeys[0] || ''
    const effectiveKey = trimmedApiKey || defaultKey
    const customKeyActive = trimmedApiKey !== ''
    const customKeyManaged = customKeyActive && configuredKeys.includes(trimmedApiKey)

    const models = [
        { id: 'deepseek-v4-flash', name: 'deepseek-v4-flash', icon: 'MessageSquare', desc: t('apiTester.models.chat'), color: 'text-amber-500' },
        { id: 'deepseek-reasoner', name: 'deepseek-reasoner', icon: 'Cpu', desc: t('apiTester.models.reasoner'), color: 'text-amber-600' },
        { id: 'deepseek-chat-search', name: 'deepseek-chat-search', icon: 'SearchIcon', desc: t('apiTester.models.chatSearch'), color: 'text-cyan-500' },
        { id: 'deepseek-reasoner-search', name: 'deepseek-reasoner-search', icon: 'SearchIcon', desc: t('apiTester.models.reasonerSearch'), color: 'text-cyan-600' },
        { id: 'deepseek-v4-pro', name: 'deepseek-v4-pro', icon: 'MessageSquare', desc: t('apiTester.models.expertChat'), color: 'text-emerald-500' },
        { id: 'deepseek-expert-reasoner', name: 'deepseek-expert-reasoner', icon: 'Cpu', desc: t('apiTester.models.expertReasoner'), color: 'text-emerald-600' },
        { id: 'deepseek-expert-chat-search', name: 'deepseek-expert-chat-search', icon: 'SearchIcon', desc: t('apiTester.models.expertChatSearch'), color: 'text-teal-500' },
        { id: 'deepseek-expert-reasoner-search', name: 'deepseek-expert-reasoner-search', icon: 'SearchIcon', desc: t('apiTester.models.expertReasonerSearch'), color: 'text-teal-600' },
        { id: 'deepseek-vision-chat', name: 'deepseek-vision-chat', icon: 'MessageSquare', desc: t('apiTester.models.visionChat'), color: 'text-violet-500' },
        { id: 'deepseek-vision-reasoner', name: 'deepseek-vision-reasoner', icon: 'Cpu', desc: t('apiTester.models.visionReasoner'), color: 'text-violet-600' },
        { id: 'deepseek-vision-chat-search', name: 'deepseek-vision-chat-search', icon: 'SearchIcon', desc: t('apiTester.models.visionChatSearch'), color: 'text-fuchsia-500' },
        { id: 'deepseek-vision-reasoner-search', name: 'deepseek-vision-reasoner-search', icon: 'SearchIcon', desc: t('apiTester.models.visionReasonerSearch'), color: 'text-fuchsia-600' },
    ]

    const { runTest, stopGeneration } = useChatStreamClient({
        t,
        onMessage,
        model,
        message,
        effectiveKey,
        selectedAccount,
        streamingMode,
        attachedFiles,
        abortControllerRef,
        setLoading,
        setIsStreaming,
        setResponse,
        setStreamingContent,
        setStreamingThinking,
    })

    return (
        <div className={clsx('flex flex-col lg:grid lg:grid-cols-12 gap-6 h-[calc(100vh-140px)] min-h-0')}>
            <ConfigPanel
                t={t}
                configExpanded={configExpanded}
                setConfigExpanded={setConfigExpanded}
                models={models}
                model={model}
                setModel={setModel}
                streamingMode={streamingMode}
                setStreamingMode={setStreamingMode}
                selectedAccount={selectedAccount}
                setSelectedAccount={setSelectedAccount}
                accounts={accounts}
                resolveAccountIdentifier={resolveAccountIdentifier}
                apiKey={apiKey}
                setApiKey={setApiKey}
                config={config}
                customKeyActive={customKeyActive}
                customKeyManaged={customKeyManaged}
            />

            <ChatPanel
                t={t}
                message={message}
                setMessage={setMessage}
                attachedFiles={attachedFiles}
                setAttachedFiles={setAttachedFiles}
                setSelectedAccount={setSelectedAccount}
                effectiveKey={effectiveKey}
                selectedAccount={selectedAccount}
                onMessage={onMessage}
                response={response}
                isStreaming={isStreaming}
                loading={loading}
                streamingThinking={streamingThinking}
                streamingContent={streamingContent}
                onRunTest={runTest}
                onStopGeneration={stopGeneration}
            />
        </div>
    )
}
