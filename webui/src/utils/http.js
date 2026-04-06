export async function readApiResponse(res, nonJsonMessage) {
    const contentType = String(res.headers.get('content-type') || '').toLowerCase()
    const raw = await res.text()
    const trimmed = raw.trim()

    if (!trimmed) {
        return {}
    }

    if (contentType.includes('application/json')) {
        try {
            return JSON.parse(trimmed)
        } catch (_err) {
            if (!res.ok) {
                return { detail: trimmed }
            }
            throw new Error(nonJsonMessage)
        }
    }

    if (!res.ok) {
        return { detail: trimmed }
    }

    throw new Error(nonJsonMessage)
}
