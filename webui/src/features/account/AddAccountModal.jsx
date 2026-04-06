import { X } from 'lucide-react'

export default function AddAccountModal({
    show,
    t,
    newAccount,
    setNewAccount,
    proxies,
    loading,
    onClose,
    onAdd,
}) {
    if (!show) {
        return null
    }

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4 animate-in fade-in">
            <div className="bg-card w-full max-w-md rounded-xl border border-border shadow-2xl overflow-hidden animate-in zoom-in-95">
                <div className="p-4 border-b border-border flex justify-between items-center">
                    <h3 className="font-semibold">{t('accountManager.modalAddAccountTitle')}</h3>
                    <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
                        <X className="w-5 h-5" />
                    </button>
                </div>
                <div className="p-6 space-y-4">
                    <div>
                        <label className="block text-sm font-medium mb-1.5">{t('accountManager.emailOptional')}</label>
                        <input
                            type="email"
                            className="input-field"
                            placeholder="user@example.com"
                            value={newAccount.email}
                            onChange={e => setNewAccount({ ...newAccount, email: e.target.value })}
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium mb-1.5">{t('accountManager.mobileOptional')}</label>
                        <input
                            type="text"
                            className="input-field"
                            placeholder="+86..."
                            value={newAccount.mobile}
                            onChange={e => setNewAccount({ ...newAccount, mobile: e.target.value })}
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium mb-1.5">{t('accountManager.passwordLabel')} <span className="text-destructive">*</span></label>
                        <input
                            type="password"
                            className="input-field bg-[#09090b]"
                            placeholder={t('accountManager.passwordPlaceholder')}
                            value={newAccount.password}
                            onChange={e => setNewAccount({ ...newAccount, password: e.target.value })}
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium mb-1.5">{t('accountManager.accountProxyLabel')}</label>
                        <select
                            className="input-field"
                            value={newAccount.proxy_id || ''}
                            onChange={e => setNewAccount({ ...newAccount, proxy_id: e.target.value })}
                        >
                            <option value="">{t('accountManager.proxyNone')}</option>
                            {proxies.map((proxy) => (
                                <option key={proxy.id} value={proxy.id}>
                                    {proxy.name || `${proxy.host}:${proxy.port}`} ({proxy.type})
                                </option>
                            ))}
                        </select>
                    </div>
                    <div className="flex justify-end gap-2 pt-2">
                        <button onClick={onClose} className="px-4 py-2 rounded-lg border border-border hover:bg-secondary transition-colors text-sm font-medium">{t('actions.cancel')}</button>
                        <button onClick={onAdd} disabled={loading} className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-sm font-medium disabled:opacity-50">
                            {loading ? t('accountManager.addAccountLoading') : t('accountManager.addAccountAction')}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    )
}
