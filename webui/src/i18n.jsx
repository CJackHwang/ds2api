import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import en from './locales/en.json'
import zh from './locales/zh.json'
import vi from './locales/vi.json'

const STORAGE_KEY = 'ds2api_lang'
const translations = { en, zh, vi }

const I18nContext = createContext({
    lang: 'zh',
    setLang: () => {},
    t: (key) => key,
})

const getBrowserLang = () => {
    if (typeof navigator === 'undefined') return 'zh'
    const browserLang = navigator.language?.toLowerCase()
    if (browserLang.startsWith('zh')) return 'zh'
    if (browserLang.startsWith('vi')) return 'vi'
    return 'en'
}

const getValue = (obj, key) => {
    if (!obj) return undefined
    return key.split('.').reduce((acc, part) => (acc ? acc[part] : undefined), obj)
}

const formatMessage = (message, vars) => {
    if (!vars) return message
    return message.replace(/\{(\w+)\}/g, (match, key) => {
        if (Object.prototype.hasOwnProperty.call(vars, key)) {
            return vars[key]
        }
        return match
    })
}

export const I18nProvider = ({ children }) => {
    const [lang, setLang] = useState(() => {
        if (typeof localStorage === 'undefined') return getBrowserLang()
        return localStorage.getItem(STORAGE_KEY) || getBrowserLang()
    })

    useEffect(() => {
        if (typeof localStorage !== 'undefined') {
            localStorage.setItem(STORAGE_KEY, lang)
        }
        if (typeof document !== 'undefined') {
            document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en'
        }
    }, [lang])

    const t = useMemo(() => {
        return (key, vars) => {
            const value = getValue(translations[lang], key) ?? getValue(translations.en, key) ?? key
            if (typeof value !== 'string') return value
            return formatMessage(value, vars)
        }
    }, [lang])

    const contextValue = useMemo(() => ({ lang, setLang, t }), [lang, t])

    return <I18nContext.Provider value={contextValue}>{children}</I18nContext.Provider>
}

export const useI18n = () => useContext(I18nContext)
