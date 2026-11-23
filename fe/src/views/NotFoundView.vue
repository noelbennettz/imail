<template>
  <div class="not-found">
    <div class="not-found-card">
      <el-icon class="not-found-icon"><Compass/></el-icon>
      <h1>{{ lang.not_found_title }}</h1>
      <p>{{ lang.not_found_desc }}</p>
      <el-button type="primary" @click="router.replace({name: 'home'})">{{ lang.go_home }}</el-button>
    </div>
  </div>
</template>

<script setup>
import {Compass} from '@element-plus/icons-vue'
import {useRouter} from 'vue-router'
import lang from '@/i18n/i18n'

const router = useRouter()
</script>

<style scoped>
.not-found {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100dvh;
  padding: 24px;
  background: var(--md-sys-color-background);
}

.not-found-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  max-width: 420px;
  padding: 40px 32px;
  border-radius: var(--pm-radius-xl);
  border: 1px solid var(--pm-border-color);
  background: var(--md-sys-color-surface-container-low);
  text-align: center;
}

.not-found-icon {
  font-size: 48px;
  color: var(--md-sys-color-outline);
}

h1 {
  font-size: 22px;
  font-weight: 600;
}

p {
  color: var(--pm-text-secondary);
  line-height: 1.6;
}
</style>
