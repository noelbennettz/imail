import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import {router} from './router'

import {ElLoading, ElMessage} from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import './assets/main.css'
import { initTheme } from './utils/theme'
import lang from './i18n/i18n'

initTheme()

const app = createApp(App)
app.use(createPinia())
app.use(router)

// Components come from the auto-import resolver per file, so registering the
// whole plugin only to get white-listed components is dead weight. The loading
// directive is the one global Element Plus affordance the views rely on.
app.directive('loading', ElLoading.directive)

// A component that throws should not leave a blank window with only a console
// stack trace; tell the person something went wrong and keep the shell alive.
app.config.errorHandler = (error, instance, info) => {
  console.error('[imail] unhandled error', info, error)
  ElMessage.error(lang.server_error)
}

app.mount('#app')
