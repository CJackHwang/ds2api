import { useEffect, useState } from 'react'

function matchesAccountQuery(acc, query) {
    const q = String(query || '').trim().toLowerCase()
    if (!q) {
        return true
    }
    const identifier = String(acc?.identifier || acc?.email || acc?.mobile || '').toLowerCase()
    const email = String(acc?.email || '').toLowerCase()
    const mobile = String(acc?.mobile || '').toLowerCase()
    return identifier.includes(q) || email.includes(q) || mobile.includes(q)
}

export function useAccountsData({ apiFetch }) {
    const [queueStatus, setQueueStatus] = useState(null)
    const [keysExpanded, setKeysExpanded] = useState(false)

    const [accounts, setAccounts] = useState([])
    const [page, setPage] = useState(1)
    const [pageSize, setPageSize] = useState(10)
    const [totalPages, setTotalPages] = useState(1)
    const [totalAccounts, setTotalAccounts] = useState(0)
    const [loadingAccounts, setLoadingAccounts] = useState(false)

    const resolveAccountIdentifier = (acc) => {
        if (!acc || typeof acc !== 'object') return ''
        return String(acc.identifier || acc.email || acc.mobile || '').trim()
    }

    const [searchQuery, setSearchQuery] = useState('')

    const fetchAccounts = async (targetPage = page, targetPageSize = pageSize, targetQuery = searchQuery) => {
        setLoadingAccounts(true)
        try {
            let url = `/admin/accounts?page=${targetPage}&page_size=${targetPageSize}`
            if (targetQuery.trim()) url += `&q=${encodeURIComponent(targetQuery.trim())}`
            const res = await apiFetch(url)
            if (res.ok) {
                const data = await res.json()
                setAccounts(data.items || [])
                setTotalPages(data.total_pages || 1)
                setTotalAccounts(data.total || 0)
                setPage(data.page || 1)
            }
        } catch (e) {
            console.error('Failed to fetch accounts:', e)
        } finally {
            setLoadingAccounts(false)
        }
    }

    const changePageSize = (newSize) => {
        setPageSize(newSize)
        fetchAccounts(1, newSize)
    }

    const handleSearchChange = (query) => {
        setSearchQuery(query)
        fetchAccounts(1, pageSize, query)
    }

    const fetchQueueStatus = async () => {
        try {
            const res = await apiFetch('/admin/queue/status')
            if (res.ok) {
                const data = await res.json()
                setQueueStatus(data)
            }
        } catch (e) {
            console.error('Failed to fetch queue status:', e)
        }
    }

    const updateFilteredCount = (delta) => {
        setTotalAccounts(prev => {
            const next = Math.max(0, prev + delta)
            const nextPages = Math.max(1, Math.ceil(next / pageSize))
            setTotalPages(nextPages)
            setPage(current => Math.min(current, nextPages))
            return next
        })
    }

    const updateQueueCount = (delta) => {
        setQueueStatus(prev => {
            if (!prev) {
                return prev
            }
            return {
                ...prev,
                total: Math.max(0, (prev.total || 0) + delta),
                available: Math.max(0, (prev.available || 0) + delta),
            }
        })
    }

    const addAccountLocally = (account) => {
        const identifier = resolveAccountIdentifier(account)
        if (!identifier) {
            return
        }
        updateQueueCount(1)
        if (!matchesAccountQuery(account, searchQuery)) {
            return
        }
        updateFilteredCount(1)
        setPage(1)
        setAccounts(prev => {
            const next = [account, ...prev.filter(item => resolveAccountIdentifier(item) !== identifier)]
            return next.slice(0, pageSize)
        })
    }

    const removeAccountLocally = (identifier) => {
        const accountID = String(identifier || '').trim()
        if (!accountID) {
            return
        }
        updateQueueCount(-1)
        setAccounts(prev => prev.filter(item => resolveAccountIdentifier(item) !== accountID))
        updateFilteredCount(-1)
    }

    useEffect(() => {
        fetchAccounts()
        fetchQueueStatus()
        const interval = setInterval(fetchQueueStatus, 3000)
        return () => clearInterval(interval)
    }, [])

    return {
        queueStatus,
        keysExpanded,
        setKeysExpanded,
        accounts,
        page,
        pageSize,
        totalPages,
        totalAccounts,
        loadingAccounts,
        fetchAccounts,
        fetchQueueStatus,
        changePageSize,
        resolveAccountIdentifier,
        searchQuery,
        handleSearchChange,
        addAccountLocally,
        removeAccountLocally,
    }
}
