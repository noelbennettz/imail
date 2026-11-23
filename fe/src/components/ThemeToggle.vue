<template>
  <el-tooltip :content="label" placement="bottom" effect="dark">
    <button type="button" class="m3-icon-button theme-toggle" :aria-label="label" @click="cycle">
      <el-icon v-if="mode === 'light'"><Sunny/></el-icon>
      <el-icon v-else-if="mode === 'dark'"><Moon/></el-icon>
      <el-icon v-else><Monitor/></el-icon>
    </button>
  </el-tooltip>
</template>

<script setup>
import {computed, onMounted, onUnmounted, ref} from 'vue'
import {Moon, Monitor, Sunny} from '@element-plus/icons-vue'
import {applyTheme, nextThemeMode, storedTheme, THEME_EVENT} from '@/utils/theme'
import lang from '@/i18n/i18n'

const mode = ref(storedTheme())
const labels = {
  system: `${lang.theme}: ${lang.theme_system}`,
  light: `${lang.theme}: ${lang.theme_light}`,
  dark: `${lang.theme}: ${lang.theme_dark}`,
}
const label = computed(() => labels[mode.value])

const cycle = () => {
  mode.value = nextThemeMode()
  applyTheme(mode.value)
}

const sync = (event) => {
  mode.value = event.detail.mode
}

onMounted(() => {
  window.addEventListener(THEME_EVENT, sync)
  window.addEventListener('storage', () => {
    mode.value = storedTheme()
  })
})
onUnmounted(() => {
  window.removeEventListener(THEME_EVENT, sync)
})
</script>

<style scoped>
.theme-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 20px;
  background: var(--md-sys-color-surface-container);
  color: var(--md-sys-color-on-surface-variant);
  cursor: pointer;
  transition: background-color .15s, color .15s;
}

.theme-toggle:hover {
  background: var(--md-sys-color-surface-container-high);
  color: var(--md-sys-color-primary);
}

.theme-toggle:focus-visible {
  outline: 3px solid var(--md-sys-color-primary);
  outline-offset: 2px;
}
</style>
