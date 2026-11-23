import en from './locales/en.js'
import zhCN from './locales/zh-CN.js'
import zhTW from './locales/zh-TW.js'
import ar from './locales/ar.js'
import fr from './locales/fr.js'
import ru from './locales/ru.js'
import es from './locales/es.js'

export const languages = [
  {code: 'en', label: 'English'},
  {code: 'zh-CN', label: '简体中文'},
  {code: 'zh-TW', label: '繁體中文'},
  {code: 'ar', label: 'العربية'},
  {code: 'fr', label: 'Français'},
  {code: 'ru', label: 'Русский'},
  {code: 'es', label: 'Español'},
]

const dictionaries = {'en': en, 'zh-CN': zhCN, 'zh-TW': zhTW, 'ar': ar, 'fr': fr, 'ru': ru, 'es': es}
const supported = languages.map(item => item.code)

function detectLanguage() {
  const stored = localStorage.getItem('imail-lang')
  if (stored && supported.includes(stored)) return stored
  const browser = ((navigator.languages && navigator.languages[0]) || navigator.language || 'en').toLowerCase()
  if (browser.startsWith('zh')) {
    return /(tw|hk|mo)/.test(browser) ? 'zh-TW' : 'zh-CN'
  }
  for (const code of ['ar', 'fr', 'ru', 'es']) {
    if (browser.startsWith(code)) return code
  }
  return 'en'
}

export const language = detectLanguage()

// The <html lang> attribute drives hyphenation, screen-reader pronunciation and
// the :lang() selectors, so it has to follow the interface language rather than
// staying at the value baked into index.html.
const htmlLang = {'en': 'en', 'zh-CN': 'zh-Hans', 'zh-TW': 'zh-Hant', 'ar': 'ar', 'fr': 'fr', 'ru': 'ru', 'es': 'es'}
document.documentElement.lang = htmlLang[language] || language
document.documentElement.dir = language === 'ar' ? 'rtl' : 'ltr'

export function setLanguage(code) {
  if (!supported.includes(code)) return
  localStorage.setItem('imail-lang', code)
  window.location.reload()
}

export default dictionaries[language] || en
