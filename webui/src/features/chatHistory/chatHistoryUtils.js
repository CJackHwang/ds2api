export const LIMIT_OPTIONS = [0, 10, 20, 50]
export const DISABLED_LIMIT = 0
export const MESSAGE_COLLAPSE_AT = 700
export const VIEW_MODE_KEY = 'ds2api_chat_history_view_mode'

const BEGIN_SENTENCE_MARKER = '<|begin▁of▁sentence|>'
const SYSTEM_MARKER = '<|System|>'
const USER_MARKER = '<|User|>'
const ASSISTANT_MARKER = '<|Assistant|>'
const TOOL_MARKER = '<|Tool|>'
const END_INSTRUCTIONS_MARKER = '<|end▁of▁instructions|>'
const END_SENTENCE_MARKER = '<|end▁of▁sentence|>'
const END_TOOL_RESULTS_MARKER = '<|end▁of▁toolresults|>'
const CURRENT_INPUT_FILE_PROMPT = 'Continue from the latest state in the attached DS2API_HISTORY.txt context. Treat it as the current working state and answer the latest user request directly.'
const LEGACY_CURRENT_INPUT_FILE_PROMPTS = new Set([
    'The current request and prior conversation context have already been provided. Answer the latest user request directly.',
])
const HISTORY_TRANSCRIPT_TITLE = '# DS2API_HISTORY.txt'
const HISTORY_TRANSCRIPT_ENTRY_RE = /^===\s+\d+\.\s+([A-Z][A-Z_ -]*)\s+===\s*$/gm

function isCurrentInputFilePrompt(value) {
    const text = String(value || '').trim()
    return text === CURRENT_INPUT_FILE_PROMPT || LEGACY_CURRENT_INPUT_FILE_PROMPTS.has(text)
}

function normalizeHistoryRole(role) {
    const normalized = String(role || '').trim().toLowerCase()
    if (normalized === 'function') return 'tool'
    if (normalized === 'developer') return 'system'
    if (normalized === 'system' || normalized === 'user' || normalized === 'assistant' || normalized === 'tool') {
        return normalized
    }
    return normalized || 'system'
}

export function formatDateTime(value, lang) {
    if (!value) return '-'
    try {
        return new Intl.DateTimeFormat(lang === 'zh' ? 'zh-CN' : 'en-US', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
        }).format(new Date(value))
    } catch {
        return '-'
    }
}

export function formatElapsed(ms, t) {
    if (!ms) return t('chatHistory.metaUnknown')
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(ms < 10_000 ? 2 : 1)}s`
}

export function previewText(item) {
    return item?.preview || item?.content || item?.reasoning_content || item?.error || item?.user_input || ''
}

export function statusTone(status) {
    switch (status) {
        case 'success':
            return 'border-emerald-500/20 bg-emerald-500/10 text-emerald-600'
        case 'error':
            return 'border-destructive/20 bg-destructive/10 text-destructive'
        case 'stopped':
            return 'border-amber-500/20 bg-amber-500/10 text-amber-600'
        default:
            return 'border-border bg-secondary/60 text-muted-foreground'
    }
}

export function downloadTextFile(filename, text) {
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
}

function fallbackCopyText(text) {
    const textArea = document.createElement('textarea')
    textArea.value = text
    textArea.setAttribute('readonly', '')
    textArea.style.position = 'fixed'
    textArea.style.top = '-9999px'
    textArea.style.left = '-9999px'

    document.body.appendChild(textArea)
    textArea.focus()
    textArea.select()

    let copied = false
    try {
        copied = document.execCommand('copy')
    } finally {
        document.body.removeChild(textArea)
    }

    if (!copied) {
        throw new Error('copy failed')
    }
}

export async function copyTextWithFallback(text) {
    try {
        if (navigator.clipboard?.writeText) {
            await navigator.clipboard.writeText(text)
            return
        }
    } catch {
        // Fall through to execCommand fallback.
    }
    fallbackCopyText(text)
}

function skipWhitespace(text, start) {
    let cursor = start
    while (cursor < text.length && /\s/.test(text[cursor])) {
        cursor += 1
    }
    return cursor
}

export function parseStrictHistoryMessages(historyText) {
    const rawText = String(historyText || '')
    const beginIndex = rawText.indexOf(BEGIN_SENTENCE_MARKER)
    if (beginIndex < 0) return null

    const transcript = rawText.slice(beginIndex)
    let cursor = BEGIN_SENTENCE_MARKER.length
    const parsed = []
    let expectedRole = null
    let trailingAssistantPromptOnly = false

    while (cursor < transcript.length) {
        if (expectedRole === null) {
            if (transcript.startsWith(SYSTEM_MARKER, cursor)) {
                expectedRole = 'system'
            } else if (transcript.startsWith(USER_MARKER, cursor)) {
                expectedRole = 'user'
            } else if (transcript.startsWith(ASSISTANT_MARKER, cursor)) {
                expectedRole = 'assistant'
            } else if (transcript.slice(cursor).trim() === '') {
                break
            } else {
                return null
            }
        }

        if (transcript.startsWith(SYSTEM_MARKER, cursor)) {
            if (expectedRole !== 'system') return null
            cursor += SYSTEM_MARKER.length
            const nextInstructionsEnd = transcript.indexOf(END_INSTRUCTIONS_MARKER, cursor)
            if (nextInstructionsEnd < 0) return null
            parsed.push({ role: 'system', content: transcript.slice(cursor, nextInstructionsEnd) })
            cursor = nextInstructionsEnd + END_INSTRUCTIONS_MARKER.length
            expectedRole = 'user'
            continue
        }

        if (transcript.startsWith(USER_MARKER, cursor)) {
            if (expectedRole !== 'user' && expectedRole !== 'user_or_tool' && expectedRole !== 'assistant_or_user') return null
            cursor += USER_MARKER.length
            const nextAssistant = transcript.indexOf(ASSISTANT_MARKER, cursor)
            const nextTool = transcript.indexOf(TOOL_MARKER, cursor)
            const nextSentenceEnd = transcript.indexOf(END_SENTENCE_MARKER, cursor)
            let nextRoleIndex = nextAssistant
            if (nextRoleIndex < 0 || (nextTool >= 0 && nextTool < nextRoleIndex)) {
                nextRoleIndex = nextTool
            }
            if (nextRoleIndex < 0) return null
            if (nextSentenceEnd >= 0 && nextSentenceEnd < nextRoleIndex) {
                const assistantStart = skipWhitespace(transcript, nextSentenceEnd + END_SENTENCE_MARKER.length)
                if (!transcript.startsWith(ASSISTANT_MARKER, assistantStart)) return null
                parsed.push({ role: 'user', content: transcript.slice(cursor, nextSentenceEnd) })
                cursor = assistantStart
                expectedRole = 'assistant'
                continue
            }
            parsed.push({ role: 'user', content: transcript.slice(cursor, nextRoleIndex) })
            if (transcript.startsWith(TOOL_MARKER, nextRoleIndex)) {
                cursor = nextRoleIndex
                expectedRole = 'tool'
                continue
            }
            const assistantStart = nextRoleIndex + ASSISTANT_MARKER.length
            if (transcript.indexOf(END_SENTENCE_MARKER, assistantStart) < 0) {
                trailingAssistantPromptOnly = true
                cursor = assistantStart
                break
            }
            cursor = nextRoleIndex
            expectedRole = 'assistant'
            continue
        }

        if (transcript.startsWith(ASSISTANT_MARKER, cursor)) {
            if (expectedRole !== 'assistant' && expectedRole !== 'assistant_or_user') return null
            cursor += ASSISTANT_MARKER.length
            const nextSentenceEnd = transcript.indexOf(END_SENTENCE_MARKER, cursor)
            if (nextSentenceEnd < 0) return null
            parsed.push({ role: 'assistant', content: transcript.slice(cursor, nextSentenceEnd) })
            cursor = nextSentenceEnd + END_SENTENCE_MARKER.length
            expectedRole = 'user_or_tool'
            continue
        }

        if (transcript.startsWith(TOOL_MARKER, cursor)) {
            if (expectedRole !== 'tool' && expectedRole !== 'user' && expectedRole !== 'user_or_tool') return null
            cursor += TOOL_MARKER.length
            const nextToolResultsEnd = transcript.indexOf(END_TOOL_RESULTS_MARKER, cursor)
            if (nextToolResultsEnd < 0) return null
            parsed.push({ role: 'tool', content: transcript.slice(cursor, nextToolResultsEnd) })
            cursor = nextToolResultsEnd + END_TOOL_RESULTS_MARKER.length
            expectedRole = 'assistant_or_user'
            continue
        }

        if (parsed.length && (expectedRole === 'user' || expectedRole === 'user_or_tool' || expectedRole === 'assistant_or_user')) break
        if (transcript.slice(cursor).trim() === '') break
        return null
    }

    if (!parsed.length) return null
    if (!trailingAssistantPromptOnly && parsed[parsed.length - 1]?.role !== 'assistant') return null
    return parsed
}

export function parseTranscriptHistoryMessages(historyText) {
    const rawText = String(historyText || '')
    const titleIndex = rawText.indexOf(HISTORY_TRANSCRIPT_TITLE)
    const transcript = titleIndex >= 0 ? rawText.slice(titleIndex) : rawText
    const matches = [...transcript.matchAll(HISTORY_TRANSCRIPT_ENTRY_RE)]
    if (!matches.length) return null

    const parsed = []
    for (let i = 0; i < matches.length; i += 1) {
        const match = matches[i]
        const next = matches[i + 1]
        const role = normalizeHistoryRole(match[1])
        const start = (match.index || 0) + match[0].length
        const end = next ? next.index : transcript.length
        const content = transcript.slice(start, end).replace(/^\r?\n/, '').trim()
        if (!content) continue
        parsed.push({ role, content })
    }

    return parsed.length ? parsed : null
}

export function parseHistoryMessages(historyText) {
    return parseStrictHistoryMessages(historyText) || parseTranscriptHistoryMessages(historyText)
}

export function buildListModeMessages(item, t) {
    const liveMessages = Array.isArray(item?.messages) && item.messages.length > 0
        ? item.messages
        : [{ role: 'user', content: item?.user_input || t('chatHistory.emptyUserInput') }]
    const historyMessages = parseHistoryMessages(item?.history_text)

    if (!historyMessages?.length) {
        return { messages: liveMessages, historyMerged: false }
    }

    const placeholderOnly = liveMessages.length === 1
        && String(liveMessages[0]?.role || '').trim().toLowerCase() === 'user'
        && isCurrentInputFilePrompt(liveMessages[0]?.content)

    if (placeholderOnly) {
        return { messages: historyMessages, historyMerged: true }
    }

    const insertAt = liveMessages.findIndex(message => {
        const role = String(message?.role || '').trim().toLowerCase()
        return role !== 'system' && role !== 'developer'
    })
    const mergedMessages = [...liveMessages]
    mergedMessages.splice(insertAt < 0 ? mergedMessages.length : insertAt, 0, ...historyMessages)

    return { messages: mergedMessages, historyMerged: true }
}
