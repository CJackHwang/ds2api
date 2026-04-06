import { useState } from 'react'
import { FileCode, Download, Upload, Copy, Check, AlertTriangle } from 'lucide-react'
import clsx from 'clsx'
import { useI18n } from '../i18n'

export default function BatchImport({ onRefresh, onMessage, authFetch }) {
    const { t } = useI18n()
    const [jsonInput, setJsonInput] = useState('')
    const [loading, setLoading] = useState(false)
    const [result, setResult] = useState(null)
    const [copied, setCopied] = useState(false)

    const apiFetch = authFetch || fetch
    const templates = {
        full: {
            name: t('batchImport.templates.full.name'),
            desc: t('batchImport.templates.full.desc'),
            config: {
                keys: ["your-api-key-1", "your-api-key-2"],
                proxies: [
                    {
                        id: "proxy-hk-1",
                        name: "Hong Kong Exit",
                        type: "socks5h",
                        host: "127.0.0.1",
                        port: 1080,
                        username: "proxy-user",
                        password: "proxy-password"
                    }
                ],
                accounts: [
                    { email: "user1@example.com", password: "password1", token: "", proxy_id: "proxy-hk-1" },
                    { email: "user2@example.com", password: "password2", token: "" },
                    { mobile: "+8613800138001", password: "password3", token: "" }
                ],
                claude_model_mapping: {
                    fast: "deepseek-chat",
                    slow: "deepseek-reasoner"
                }
            }
        },
        email_only: {
            name: t('batchImport.templates.emailOnly.name'),
            desc: t('batchImport.templates.emailOnly.desc'),
            config: {
                keys: ["your-api-key"],
                accounts: [
                    { email: "account1@example.com", password: "pass1", token: "" },
                    { email: "account2@example.com", password: "pass2", token: "" },
                    { email: "account3@example.com", password: "pass3", token: "" }
                ]
            }
        },
        mobile_only: {
            name: t('batchImport.templates.mobileOnly.name'),
            desc: t('batchImport.templates.mobileOnly.desc'),
            config: {
                keys: ["your-api-key"],
                accounts: [
                    { mobile: "+8613800000001", password: "pass1", token: "" },
                    { mobile: "+8613800000002", password: "pass2", token: "" },
                    { mobile: "+8613800000003", password: "pass3", token: "" }
                ]
            }
        },
        keys_only: {
            name: t('batchImport.templates.keysOnly.name'),
            desc: t('batchImport.templates.keysOnly.desc'),
            config: {
                keys: ["key-1", "key-2", "key-3"]
            }
        }
    }

    const handleImport = async () => {
        if (!jsonInput.trim()) {
            onMessage('error', t('batchImport.enterJson'))
            return
        }

        let config
        try {
            config = JSON.parse(jsonInput)
        } catch (e) {
            onMessage('error', t('messages.invalidJson'))
            return
        }

        setLoading(true)
        setResult(null)
        try {
            const res = await apiFetch('/admin/import', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config),
            })
            const data = await res.json()
            if (res.ok) {
                setResult(data)
                onMessage('success', t('batchImport.importSuccess', { keys: data.imported_keys, accounts: data.imported_accounts }))
                onRefresh()
            } else {
                onMessage('error', data.detail || t('messages.importFailed'))
            }
        } catch (e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setLoading(false)
        }
    }

    const loadTemplate = (key) => {
        const tpl = templates[key]
        if (tpl) {
            setJsonInput(JSON.stringify(tpl.config, null, 2))
            onMessage('info', t('batchImport.templateLoaded', { name: tpl.name }))
        }
    }

    const handleExport = async () => {
        try {
            const res = await apiFetch('/admin/export')
            if (res.ok) {
                const data = await res.json()
                setJsonInput(JSON.stringify(JSON.parse(data.json), null, 2))
                onMessage('success', t('batchImport.currentConfigLoaded'))
            }
        } catch (e) {
            onMessage('error', t('batchImport.fetchConfigFailed'))
        }
    }

    const copyBase64 = async () => {
        try {
            const res = await apiFetch('/admin/export')
            if (res.ok) {
                const data = await res.json()
                await navigator.clipboard.writeText(data.base64)
                setCopied(true)
                setTimeout(() => setCopied(false), 2000)
                onMessage('success', t('batchImport.copySuccess'))
            }
        } catch (e) {
            onMessage('error', t('messages.copyFailed'))
        }
    }

    return (
        <div className="flex flex-col lg:grid lg:grid-cols-3 gap-6 lg:h-[calc(100vh-140px)]">
            {/* Templates Panel */}
            <div className="md:col-span-1 space-y-4">
                <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                    <h3 className="font-semibold flex items-center gap-2 mb-4">
                        <FileCode className="w-4 h-4 text-primary" />
                        {t('batchImport.quickTemplates')}
                    </h3>
                    <div className="space-y-3">
                        {Object.entries(templates).map(([key, tpl]) => (
                            <button
                                key={key}
                                onClick={() => loadTemplate(key)}
                                className="w-full text-left p-3 rounded-lg border border-border bg-secondary/20 hover:bg-secondary/50 hover:border-primary/50 transition-all custom-focus group"
                            >
                                <div className="font-medium text-sm group-hover:text-primary transition-colors">{tpl.name}</div>
                                <div className="text-xs text-muted-foreground mt-0.5">{tpl.desc}</div>
                            </button>
                        ))}
                    </div>
                </div>

                <div className="bg-linear-to-br from-primary/10 to-transparent border border-primary/20 rounded-xl p-5 shadow-sm">
                    <h3 className="font-semibold flex items-center gap-2 mb-2 text-primary">
                        <Download className="w-4 h-4" />
                        {t('batchImport.dataExport')}
                    </h3>
                    <p className="text-sm text-muted-foreground mb-4">
                        {t('batchImport.dataExportDesc')}
                    </p>
                    <button
                        onClick={copyBase64}
                        className="w-full flex items-center justify-center gap-2 py-2.5 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-all font-medium text-sm shadow-sm"
                    >
                        {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                        {copied ? t('batchImport.copied') : t('batchImport.copyBase64')}
                    </button>
                    <p className="text-[10px] text-muted-foreground mt-2 text-center">
                        {t('batchImport.variableName')}: <code className="bg-background px-1 py-0.5 rounded border border-border">DS2API_CONFIG_JSON</code>
                    </p>
                </div>
            </div>

            {/* Editor Panel */}
            <div className="lg:col-span-2 flex flex-col bg-card border border-border rounded-xl shadow-sm overflow-hidden min-h-[400px] lg:h-full">
                <div className="p-4 border-b border-border flex items-center justify-between bg-muted/20">
                    <h3 className="font-semibold flex items-center gap-2">
                        <Upload className="w-4 h-4 text-primary" />
                        {t('batchImport.jsonEditor')}
                    </h3>
                    <div className="flex gap-2">
                        <button onClick={handleExport} className="px-3 py-1.5 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-xs font-medium border border-border">
                            {t('batchImport.loadCurrentConfig')}
                        </button>
                        <button onClick={handleImport} disabled={loading} className="px-3 py-1.5 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-xs font-medium disabled:opacity-50">
                            {loading ? t('batchImport.importing') : t('batchImport.applyConfig')}
                        </button>
                    </div>
                </div>

                <div className="flex-1 relative min-h-[400px]">
                    <textarea
                        className="absolute inset-0 w-full h-full p-4 font-mono text-sm bg-[#09090b] text-foreground resize-none focus:outline-none custom-scrollbar"
                        value={jsonInput}
                        onChange={e => setJsonInput(e.target.value)}
                        placeholder={'{\n  "keys": ["your-api-key"],\n  "accounts": [\n    {"email": "...", "password": "...", "token": ""}\n  ]\n}'}
                        spellCheck={false}
                    />
                </div>

                {result && (
                    <div className={clsx(
                        "p-4 border-t",
                        result.imported_keys || result.imported_accounts ? "bg-emerald-500/10 border-emerald-500/20" : "bg-destructive/10 border-destructive/20"
                    )}>
                        <div className="flex items-start gap-3">
                            {result.imported_keys || result.imported_accounts ? (
                                <Check className="w-5 h-5 text-emerald-500 mt-0.5" />
                            ) : (
                                <AlertTriangle className="w-5 h-5 text-destructive mt-0.5" />
                            )}
                            <div>
                                <h4 className={clsx("font-medium", result.imported_keys || result.imported_accounts ? "text-emerald-500" : "text-destructive")}>
                                    {t('batchImport.importComplete')}
                                </h4>
                                <p className="text-sm opacity-80 mt-1">
                                    {t('batchImport.importSummary', { keys: result.imported_keys, accounts: result.imported_accounts })}
                                </p>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        </div>
    )
}
