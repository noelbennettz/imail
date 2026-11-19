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

    <el-divider class="settings-divider"/>

    <div class="settings-header">
      <h3>{{ lang.outbox }}</h3>
      <p class="settings-desc">{{ lang.outbox_desc }}</p>
    </div>

    <div class="setting-row">
      <div class="setting-text">
        <span class="setting-label">{{ lang.outbox_pending }}: {{ outbox.pending }}</span>
        <span class="setting-hint">{{ lang.outbox_failed }}: {{ outbox.failed }}</span>
      </div>
      <el-button size="small" :loading="loadingOutbox" @click="load">{{ lang.refresh }}</el-button>
    </div>

    <div class="outbox-toolbar">
      <el-radio-group v-model="filter" size="small" @change="loadEntries(1)">
        <el-radio-button label="pending">{{ lang.outbox_pending }}</el-radio-button>
        <el-radio-button label="failed">{{ lang.outbox_failed }}</el-radio-button>
      </el-radio-group>
    </div>

    <el-table v-loading="loadingEntries" :data="entries" size="small" class="outbox-table">
      <el-table-column prop="id" label="#" width="70"/>
      <el-table-column :label="lang.outbox_recipients">
        <template #default="scope">{{ (scope.row.recipients || []).join(', ') }}</template>
      </el-table-column>
      <el-table-column prop="attempts" :label="lang.outbox_attempts" width="90"/>
      <el-table-column prop="last_error" :label="lang.outbox_last_error" show-overflow-tooltip/>
      <el-table-column :label="lang.outbox_updated" width="170">
        <template #default="scope">{{ formatTime(scope.row.update_time) }}</template>
      </el-table-column>
      <el-table-column :label="lang.actions" width="170">
        <template #default="scope">
          <el-button v-if="scope.row.state === 'failed'" size="small" @click="retry(scope.row.id)">
            {{ lang.outbox_retry }}
          </el-button>
          <el-button v-if="canDelete(scope.row)" size="small" type="danger" plain @click="remove(scope.row.id)">
            {{ lang.delete }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-if="!loadingEntries && entries.length === 0" :description="lang.outbox_empty"/>

    <el-pagination
        v-if="totalEntries > pageSize"
        class="outbox-pagination"
        layout="prev, pager, next"
        :total="totalEntries"
        :page-size="pageSize"
        :current-page="page"
        @current-change="loadEntries"/>
  </div>
</template>

<script setup>
import {onMounted, ref} from 'vue'
import {ElMessageBox, ElNotification} from 'element-plus'
import lang from '../i18n/i18n'
import {http} from '@/utils/axios'

const limitFailedSignins = ref(true)
const saving = ref(false)
const outbox = ref({pending: 0, failed: 0})
const loadingOutbox = ref(false)

// Outbox browser: one page of the waiting or the finally failed rows, with the
// per-row actions the administrator is allowed to take.
const filter = ref('failed')
const entries = ref([])
const totalEntries = ref(0)
const page = ref(1)
const pageSize = 20
const loadingEntries = ref(false)

const load = () => {
  http.get('/api/admin/security').then(res => {
    if (res.errorNo !== 0) return
    limitFailedSignins.value = res.data.limitFailedSignins !== false
  })
  loadOutbox()
}

const loadOutbox = () => {
  loadingOutbox.value = true
  http.get('/api/admin/outbox').then(res => {
    if (res.errorNo !== 0) return
    outbox.value = {pending: res.data.pending || 0, failed: res.data.failed || 0}
  }).finally(() => {
    loadingOutbox.value = false
  })
}

const loadEntries = (target) => {
  if (typeof target === 'number') page.value = target
  loadingEntries.value = true
  http.get('/api/admin/outbox/list', {
    params: {state: filter.value, page: page.value, page_size: pageSize},
  }).then(res => {
    if (res.errorNo !== 0) {
      entries.value = []
      totalEntries.value = 0
      return
    }
    entries.value = res.data.entries || []
    totalEntries.value = res.data.total || 0
  }).finally(() => {
    loadingEntries.value = false
  })
}

// Only a finished row can be deleted: a pending row is still owned by the
// worker. The list shows the failed state, whose rows are deletable once they
// are read.
const canDelete = (row) => row.state === 'failed' || row.state === 'delivered'

const formatTime = (value) => {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

const notify = (ok, message) => {
  ElNotification({
    title: ok ? lang.succ : lang.fail,
    message,
    type: ok ? 'success' : 'error',
  })
}

const retry = (id) => {
  http.post('/api/admin/outbox/retry', {id}).then(res => {
    notify(res.errorNo === 0, res.errorNo === 0 ? lang.saved : res.errorMsg)
    if (res.errorNo === 0) load()
  }).catch(() => notify(false, lang.fail))
}

const remove = (id) => {
  ElMessageBox.confirm(lang.outbox_delete_confirm, lang.delete, {
    confirmButtonText: lang.delete,
    cancelButtonText: lang.cancel,
    type: 'warning',
  }).then(() => {
    http.post('/api/admin/outbox/delete', {id}).then(res => {
      notify(res.errorNo === 0, res.errorNo === 0 ? lang.saved : res.errorMsg)
      if (res.errorNo === 0) load()
    }).catch(() => notify(false, lang.fail))
  }).catch(() => {
    // The administrator cancelled the dialog.
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

onMounted(() => {
  load()
  loadEntries(1)
})
</script>

<style scoped>
.settings-card {
  padding: 0;
}

.settings-divider {
  margin: 32px 0;
  border-color: var(--pm-border-color);
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

.outbox-toolbar {
  display: flex;
  justify-content: flex-end;
  margin: 16px 0 12px 0;
}

.outbox-table {
  width: 100%;
}

.outbox-pagination {
  margin-top: 16px;
  justify-content: flex-end;
}

@media (max-width: 768px) {
  .setting-row {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
