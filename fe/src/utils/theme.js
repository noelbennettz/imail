// Material 3 theme controller: light / dark / system, persisted locally.
const STORAGE_KEY = 'imail-theme'
const THEME_EVENT = 'imail-theme-change'

export const THEME_MODES = ['system', 'light', 'dark']

const media = window.matchMedia('(prefers-color-scheme: dark)')

export function storedTheme() {
  const value = localStorage.getItem(STORAGE_KEY)
  return THEME_MODES.includes(value) ? value : 'system'
}

export function resolvedTheme(mode = storedTheme()) {
  if (mode === 'system') {
    return media.matches ? 'dark' : 'light'
  }
  return mode
}

export function applyTheme(mode = storedTheme(), persist = true) {
  const resolved = resolvedTheme(mode)
  const root = document.documentElement
  root.dataset.theme = resolved
  root.dataset.themeMode = mode
  root.classList.toggle('dark', resolved === 'dark')
  root.style.colorScheme = resolved
  if (persist) {
    localStorage.setItem(STORAGE_KEY, mode)
  }
  window.dispatchEvent(new CustomEvent(THEME_EVENT, {detail: {mode, resolved}}))
  return resolved
}

export function nextThemeMode() {
  const current = storedTheme()
  const index = THEME_MODES.indexOf(current)
  return THEME_MODES[(index + 1) % THEME_MODES.length]
}

export function initTheme() {
  applyTheme(storedTheme(), false)
  const onSystemChange = () => {
    if (storedTheme() === 'system') {
      applyTheme('system', false)
    }
  }
  if (media.addEventListener) {
    media.addEventListener('change', onSystemChange)
  } else if (media.addListener) {
    media.addListener(onSystemChange)
  }
}

export { THEME_EVENT, STORAGE_KEY }
