<template>
  <div class="settings-card">
    <div class="settings-header">
      <h3>{{ lang.admin_security }}</h3>
      <p class="settings-desc">{{ lang.security_desc }}</p>
    </div>

    <div class="setting-row">
      <div class="setting-text">
        <span class="setting-label">{{ lang.limit_failed_signins }}</span>
        <span class="setting-hint">{{ lang.limit_failed_signins_desc }}</span>
      </div>
      <el-switch v-model="limitFailedSignins" :loading="saving" @change="save"/>
    </div>
  </div>
</template>

<script setup>
import {onMounted, ref} from 'vue'
import {ElNotification} from 'element-plus'
import lang from '../i18n/i18n'
import {http} from '@/utils/axios'

const limitFailedSignins = ref(true)
const saving = ref(false)

const load = () => {
  http.get('/api/admin/security').then(res => {
    if (res.errorNo !== 0) return
    limitFailedSignins.value = res.data.limitFailedSignins !== false
  })
}

const save = (value) => {
  saving.value = true
  http.post('/api/admin/security/save', {limitFailedSignins: value}).then(res => {
    const ok = res.errorNo === 0
    ElNotification({
      title: ok ? lang.succ : lang.fail,
      message: ok ? lang.saved : res.errorMsg,
      type: ok ? 'success' : 'error',
    })
    if (!ok) limitFailedSignins.value = !value
  }).catch(() => {
    limitFailedSignins.value = !value
  }).finally(() => {
    saving.value = false
  })
}

onMounted(load)
</script>

<style scoped>
.settings-card {
  padding: 0;
}

.settings-header {
  margin-bottom: 24px;
}

.settings-header h3 {
  font-size: 18px;
  font-weight: 600;
  color: var(--pm-text-primary);
  margin: 0 0 8px 0;
}

.settings-desc {
  font-size: 14px;
  color: var(--pm-text-secondary);
  margin: 0;
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 16px;
  border: 1px solid var(--pm-border-color);
  border-radius: var(--pm-radius-sm);
  background: var(--pm-surface-muted);
}

.setting-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.setting-label {
  font-weight: 600;
  color: var(--pm-text-primary);
}

.setting-hint {
  font-size: 13px;
  color: var(--pm-text-secondary);
  line-height: 1.5;
}

@media (max-width: 768px) {
  .setting-row {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
