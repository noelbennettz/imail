// The one-time setup token of the wizard. It is kept in sessionStorage so a
// reload of the first screen does not ask for it again, and it is gone when the
// tab closes. Nothing is written to localStorage or to a cookie.
const STORAGE_KEY = 'imail-setup-token'
export const SETUP_TOKEN_HEADER = 'X-Setup-Token'

let cached = null

function storage() {
    try {
        return window.sessionStorage
    } catch {
        return null
    }
}

export function getSetupToken() {
    if (cached !== null) return cached
    const store = storage()
    cached = store ? (store.getItem(STORAGE_KEY) || '') : ''
    return cached
}

export function setSetupToken(value) {
    cached = (value || '').trim()
    const store = storage()
    if (!store) return
    if (cached === '') {
        store.removeItem(STORAGE_KEY)
    } else {
        store.setItem(STORAGE_KEY, cached)
    }
}

export function clearSetupToken() {
    setSetupToken('')
}

export function isSetupAPI(url) {
    return typeof url === 'string' && url.startsWith('/api/setup')
}
