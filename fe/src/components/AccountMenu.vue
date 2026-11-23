<template>
  <el-popover
      v-model:visible="open"
      :placement="compact ? 'bottom-end' : 'top-start'"
      :width="296"
      trigger="click"
      popper-class="account-popover"
  >
    <template #reference>
      <button type="button" class="account-trigger" :class="{compact}" :aria-label="lang.account">
        <span class="avatar">{{ initial }}</span>
        <span v-if="!compact" class="account-text">
          <span class="account-name">{{ displayName }}</span>
          <span class="account-address">{{ address }}</span>
        </span>
      </button>
    </template>

    <div class="account-card">
      <div class="account-card-head">
        <span class="avatar avatar-lg">{{ initial }}</span>
        <div class="account-card-meta">
          <span class="account-card-label">{{ lang.signed_in_as }}</span>
          <span class="account-name">{{ displayName }}</span>
          <span class="account-address">{{ address }}</span>
        </div>
      </div>

      <button type="button" class="account-row" @click="openSettings">
        <el-icon><Setting/></el-icon>
        <span>{{ lang.settings }}</span>
      </button>

      <div class="account-row account-row-static">
        <el-icon><Brush/></el-icon>
        <span>{{ lang.theme }}</span>
        <div class="theme-seg" role="group" :aria-label="lang.theme">
          <button
              v-for="option in themes"
              :key="option.value"
              type="button"
              :class="{active: mode === option.value}"
              @click="setTheme(option.value)"
          >{{ option.label }}</button>
        </div>
      </div>

      <div class="account-row account-row-static">
        <el-icon><Connection/></el-icon>
        <span>{{ lang.language }}</span>
        <el-select :model-value="language" size="small" class="lang-select" @change="setLanguage">
          <el-option v-for="item in languages" :key="item.code" :label="item.label" :value="item.code"/>
        </el-select>
      </div>

      <button type="button" class="account-row danger" @click="logout">
        <el-icon><SwitchButton/></el-icon>
        <span>{{ lang.logout }}</span>
      </button>
    </div>
  </el-popover>
</template>

<script setup>
import {computed, onMounted, onUnmounted, ref} from 'vue'
import {useRouter} from 'vue-router'
import {Brush, Connection, Setting, SwitchButton} from '@element-plus/icons-vue'
import lang, {language, languages, setLanguage} from '@/i18n/i18n'
import {http} from '@/utils/axios'
import {applyTheme, storedTheme, THEME_EVENT} from '@/utils/theme'
import {useGlobalStatusStore} from '@/stores/useGlobalStatusStore'
import useGroupStore from '@/stores/group'

defineProps({compact: {type: Boolean, default: false}})

const router = useRouter()
const globalStatus = useGlobalStatusStore()
const groupStore = useGroupStore()
const open = ref(false)
const mode = ref(storedTheme())

const themes = computed(() => [
  {value: 'system', label: lang.theme_system},
  {value: 'light', label: lang.theme_light},
  {value: 'dark', label: lang.theme_dark},
])

const displayName = computed(() => globalStatus.userInfos.name || globalStatus.userInfos.account || '')
const address = computed(() => {
  const info = globalStatus.userInfos
  if (!info.account) return ''
  const domain = info.domain || (info.domains || [])[0] || ''
  return domain ? `${info.account}@${domain}` : info.account
})
const initial = computed(() => (displayName.value || address.value || '?').trim().charAt(0).toUpperCase())

const setTheme = (value) => {
  mode.value = value
  applyTheme(value)
}

const openSettings = () => {
  open.value = false
  router.push('/settings')
}

const logout = async () => {
  open.value = false
  await http.post('/api/logout', {}, {silent: true}).catch(() => {})
  globalStatus.reset()
  groupStore.reset()
  document.title = 'iMail'
  await router.replace({name: 'login'})
}

const sync = (event) => {
  mode.value = event.detail.mode
}

onMounted(() => window.addEventListener(THEME_EVENT, sync))
onUnmounted(() => window.removeEventListener(THEME_EVENT, sync))
</script>

<style scoped>
.account-trigger {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--pm-text-primary);
  cursor: pointer;
  text-align: left;
}

.account-trigger:hover {
  background: var(--pm-bg-hover);
}

.account-trigger:focus-visible {
  outline: 3px solid var(--pm-primary-color);
  outline-offset: 2px;
}

.account-trigger.compact {
  width: auto;
  padding: 4px;
}

.avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  min-width: 36px;
  border-radius: 50%;
  background: var(--md-sys-color-primary-container);
  color: var(--md-sys-color-on-primary-container);
  font-size: 15px;
  font-weight: 600;
}

.avatar-lg {
  width: 44px;
  height: 44px;
  min-width: 44px;
  font-size: 18px;
}

.account-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 1.25;
}

.account-name {
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-address {
  font-size: 11px;
  color: var(--pm-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-card {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.account-card-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 8px 14px;
  margin-bottom: 6px;
  border-bottom: 1px solid var(--pm-border-subtle);
}

.account-card-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.account-card-label {
  font-size: 11px;
  color: var(--pm-text-muted);
}

.account-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  min-height: 44px;
  padding: 8px 10px;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: var(--pm-text-primary);
  font-size: 14px;
  cursor: pointer;
  text-align: left;
}

.account-row:hover {
  background: var(--md-sys-color-surface-container-high);
}

.account-row:focus-visible {
  outline: 3px solid var(--pm-primary-color);
  outline-offset: -1px;
}

.account-row-static {
  cursor: default;
}

.account-row-static:hover {
  background: transparent;
}

.account-row.danger {
  color: var(--md-sys-color-error);
}

.account-row.danger:hover {
  background: var(--md-sys-color-error-container);
  color: var(--md-sys-color-on-error-container);
}

.theme-seg {
  display: inline-flex;
  margin-left: auto;
  padding: 2px;
  border-radius: 999px;
  background: var(--md-sys-color-surface-container-high);
}

.theme-seg button {
  padding: 4px 10px;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--pm-text-secondary);
  font-size: 11px;
  cursor: pointer;
}

.theme-seg button.active {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
  font-weight: 600;
}

.lang-select {
  width: 128px;
  margin-left: auto;
}
</style>
