import { useI18n } from '../i18n'

export default function LanguageToggle({ className = '' }) {
    const { lang, setLang, t } = useI18n()
    const languages = ['zh', 'en', 'vi']
    const currentIndex = languages.indexOf(lang)
    const nextLang = languages[(currentIndex + 1) % languages.length]
    
    const labelMap = {
        zh: t('language.chinese'),
        en: t('language.english'),
        vi: t('language.vietnamese'),
    }

    return (
        <button
            type="button"
            onClick={() => setLang(nextLang)}
            className={`text-xs font-semibold px-2 py-1 rounded-md border border-border bg-secondary/50 text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors ${className}`}
            title={t('language.label')}
        >
            {labelMap[nextLang]}
        </button>
    )
}
