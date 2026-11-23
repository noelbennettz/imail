<template>
  <div class="settings-card">
    <div class="settings-header domain-header">
      <div>
        <h3>{{ lang.admin_domains }}</h3>
        <p class="settings-desc">{{ lang.domain_management_desc }}</p>
      </div>
      <div class="domain-header-actions">
        <el-button type="primary" @click="openAdd">{{ lang.add_domain }}</el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" :title="lang.domain_restart_note" class="domain-note"/>

    <el-table :data="domains" class="modern-table" style="width: 100%">
      <el-table-column :label="lang.domain_label" prop="domain" min-width="220"/>
      <el-table-column :label="lang.primary_domain" width="140">
        <template #default="scope">
          <el-tag v-if="scope.row.primary" type="success" effect="plain">{{ lang.primary_domain }}</el-tag>
          <span v-else>—</span>
        </template>
      </el-table-column>
      <el-table-column :label="lang.domain_users" width="130" prop="users"/>
      <el-table-column width="340" align="right" class-name="action-cell">
        <template #default="scope">
          <el-button size="small" type="primary" text @click="openTLS(scope.row)">{{ lang.tls_certificate }}</el-button>
          <el-button size="small" type="primary" text @click="showDNS(scope.row)">{{ lang.domain_dns }}</el-button>
          <el-button size="small" type="danger" text :disabled="scope.row.primary || scope.row.users > 0"
                     @click="remove(scope.row)">{{ lang.remove_domain }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dnsVisible" :title="`${lang.domain_dns}: ${dnsDomain}`" width="760px">
      <el-table :data="dnsRecords" class="modern-table" style="width: 100%">
        <el-table-column prop="type" label="Type" width="90"/>
        <el-table-column prop="host" label="Host" min-width="160" show-overflow-tooltip/>
        <el-table-column prop="value" label="Value" min-width="260" show-overflow-tooltip/>
        <el-table-column prop="ttl" label="TTL" width="80"/>
        <el-table-column width="120" align="right" class-name="action-cell">
          <template #default="scope">
            <el-button size="small" text type="primary" @click="copyRecord(scope.row)">{{ lang.copy }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="copyAll">{{ lang.copy_all }}</el-button>
        <el-button @click="dnsVisible = false">Close</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="tlsVisible" :title="`${lang.tls_certificate}: ${tlsDomain}`" width="760px">
      <el-alert v-if="tlsInfo.shared" type="info" :closable="false" class="tls-alert"
                :title="lang.tls_shared"/>
      <el-descriptions :column="1" border>
        <el-descriptions-item :label="lang.tls_current">
          <span v-if="tlsInfo.configured">
            {{ lang.tls_expires }} {{ tlsInfo.not_after }} ({{ tlsInfo.days }} {{ lang.tls_days_left }})
          </span>
          <span v-else>{{ lang.tls_not_configured }}</span>
        </el-descriptions-item>
        <el-descriptions-item v-if="tlsInfo.configured" :label="lang.tls_domains">
          {{ (tlsInfo.domains || []).join(', ') }}
        </el-descriptions-item>
        <el-descriptions-item v-if="tlsInfo.configured" :label="lang.tls_issuer">
          {{ tlsInfo.issuer }}
        </el-descriptions-item>
      </el-descriptions>

      <el-alert v-if="tlsStatus.state === 'error'" type="error" :closable="false" class="tls-alert"
                :title="tlsStatus.message || 'Certificate request failed'"/>
      <el-alert v-else-if="tlsStatus.state === 'pending'" type="info" :closable="false" class="tls-alert"
                :title="tlsStatus.message || lang.tls_pending"/>

      <el-form label-position="top" class="tls-form">
        <el-form-item :label="lang.tls_mode">
          <el-select v-model="tlsMode" style="width: 100%">
            <el-option :label="lang.tls_http" value="http"/>
            <el-option :label="lang.dns_manual" value="dns-manual"/>
            <el-option :label="lang.dns_cloudflare" value="dns-cloudflare"/>
          </el-select>
        </el-form-item>
        <el-form-item v-if="tlsMode === 'dns-cloudflare'" :label="lang.dns_api_token">
          <el-input v-model="tlsToken" type="password" show-password :placeholder="lang.dns_api_token_hint"/>
        </el-form-item>
      </el-form>

      <div class="tls-actions">
        <el-button type="primary" :loading="issuing" @click="issue">{{ lang.tls_issue }}</el-button>
        <el-button @click="refreshTLS">{{ lang.refresh }}</el-button>
      </div>

      <div v-if="tlsRecords.length > 0" class="tls-records">
        <h4>{{ lang.dns_records }}</h4>
        <el-table :data="tlsRecords" class="modern-table" style="width: 100%">
          <el-table-column prop="type" label="Type" width="80"/>
          <el-table-column prop="host" label="Host" min-width="180" show-overflow-tooltip/>
          <el-table-column prop="value" label="Value" min-width="260" show-overflow-tooltip/>
          <el-table-column width="100" align="right" class-name="action-cell">
            <template #default="scope">
              <el-button size="small" text type="primary" @click="copyRecord(scope.row)">{{ lang.copy }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <el-divider/>
      <h4>{{ lang.tls_upload }}</h4>
      <div class="tls-upload-grid">
        <div>
          <label class="upload-label">{{ lang.tls_cert_pem }}</label>
          <input type="file" accept=".crt,.pem" @change="readCertFile"/>
        </div>
        <div>
          <label class="upload-label">{{ lang.tls_key_pem }}</label>
          <input type="file" accept=".key,.pem" @change="readKeyFile"/>
        </div>
      </div>
      <el-button class="tls-upload-btn" @click="uploadTLS">{{ lang.tls_upload_btn }}</el-button>
      <template #footer>
        <el-button @click="tlsVisible = false">Close</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="dialogVisible" :title="lang.add_domain" width="420px">
      <el-form label-position="top">
        <el-form-item :label="lang.domain_label">
          <el-input v-model="newDomain" placeholder="example.com"/>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="saving" @click="add">{{ lang.add_domain }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import {onMounted, onUnmounted, ref, watch} from 'vue'
import {ElMessage, ElMessageBox, ElNotification} from 'element-plus'
import lang from '@/i18n/i18n'
import {http} from '@/utils/axios'

const domains = ref([])
const dialogVisible = ref(false)
const saving = ref(false)
const newDomain = ref('')
const dnsVisible = ref(false)
const dnsDomain = ref('')
const dnsRecords = ref([])

const tlsVisible = ref(false)
const tlsDomain = ref('')
const tlsInfo = ref({})
const tlsStatus = ref({})
const tlsMode = ref('dns-manual')
const tlsToken = ref('')
const tlsRecords = ref([])
const issuing = ref(false)
const uploadKey = ref('')
const uploadCert = ref('')
let tlsTimer = null
let recordsTimer = null

const load = () => {
  http.post('/api/domain/list', {}).then(res => {
    if (res.errorNo !== 0) {
      ElMessage.error(res.errorMsg)
      return
    }
    domains.value = res.data.list || []
  })
}

const openAdd = () => {
  newDomain.value = ''
  dialogVisible.value = true
}

const add = () => {
  const domain = newDomain.value.trim().toLowerCase()
  if (!domain) return
  saving.value = true
  http.post('/api/domain/add', {domain}).then(res => {
    saving.value = false
    if (res.errorNo !== 0) {
      ElMessage.error(res.errorMsg)
      return
    }
    dialogVisible.value = false
    ElNotification({title: lang.succ, message: lang.domain_added, type: 'success'})
    setTimeout(load, 4000)
  }).catch(() => {
    saving.value = false
  })
}

const remove = (row) => {
  ElMessageBox.confirm(lang.delete_domain_confirm.replace('{domain}', row.domain), lang.remove_domain, {
    type: 'warning',
  }).then(() => {
    http.post('/api/domain/del', {domain: row.domain}).then(res => {
      if (res.errorNo !== 0) {
        ElMessage.error(res.errorMsg)
        return
      }
      ElNotification({title: lang.succ, message: lang.domain_removed, type: 'success'})
      setTimeout(load, 4000)
    })
  }).catch(() => {})
}

const showDNS = (row) => {
  dnsDomain.value = row.domain
  dnsRecords.value = []
  http.post('/api/domain/dns', {domain: row.domain}).then(res => {
    if (res.errorNo !== 0) {
      ElMessage.error(res.errorMsg)
      return
    }
    dnsRecords.value = res.data.records || []
    dnsVisible.value = true
  })
}

const copyText = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(lang.copied)
  } catch {
    ElMessage.error(lang.clipboard_unavailable)
  }
}

const copyRecord = (row) => {
  copyText(`${row.type}\t${row.host}\t${row.value}\t${row.ttl}`)
}

const copyAll = () => {
  const lines = dnsRecords.value.map(row => `${row.type}\t${row.host}\t${row.value}\t${row.ttl}`)
  copyText(lines.join('\n'))
}

const refreshTLS = (syncMode = false) => {
  http.post('/api/domain/ssl/info', {domain: tlsDomain.value}).then(res => {
    if (res.errorNo !== 0) return
    tlsInfo.value = res.data.certificate || {}
    tlsStatus.value = res.data.status || {}
    if (syncMode && ['http', 'dns-manual', 'dns-cloudflare'].includes(tlsInfo.value.mode)) {
      tlsMode.value = tlsInfo.value.mode
    }
    if (tlsStatus.value.state === 'pending' && tlsMode.value === 'dns-manual') {
      pollTLSRecords()
    }
  })
}

const pollTLSRecords = () => {
  if (recordsTimer) return
  const tick = () => {
    http.post('/api/domain/ssl/records', {}).then(res => {
      tlsRecords.value = (res.errorNo === 0 && res.data) ? res.data : []
      if (tlsRecords.value.length > 0 || tlsStatus.value.state === 'error') {
        recordsTimer = null
        return
      }
      recordsTimer = setTimeout(tick, 2000)
    }).catch(() => {
      recordsTimer = setTimeout(tick, 2000)
    })
  }
  tick()
}

const openTLS = (row) => {
  tlsDomain.value = row.domain
  tlsRecords.value = []
  tlsVisible.value = true
  refreshTLS(true)
  if (!tlsTimer) {
    tlsTimer = setInterval(refreshTLS, 4000)
  }
}

const stopTLSPolling = () => {
  if (tlsTimer) clearInterval(tlsTimer)
  if (recordsTimer) clearTimeout(recordsTimer)
  tlsTimer = null
  recordsTimer = null
}

const issue = () => {
  issuing.value = true
  tlsRecords.value = []
  http.post('/api/domain/ssl/issue', {domain: tlsDomain.value, mode: tlsMode.value, token: tlsToken.value}).then(res => {
    if (res.errorNo !== 0) {
      ElMessage.error(res.errorMsg)
      issuing.value = false
      return
    }
    ElNotification({title: lang.succ, message: lang.tls_issue_started, type: 'success'})
    issuing.value = false
    refreshTLS()
    if (tlsMode.value === 'dns-manual') {
      pollTLSRecords()
    }
  }).catch(() => {
    issuing.value = false
  })
}

const readPEM = (file, target) => {
  const reader = new FileReader()
  reader.onload = () => { target.value = String(reader.result || '') }
  reader.readAsText(file)
}

const readCertFile = (event) => {
  if (event.target.files && event.target.files[0]) readPEM(event.target.files[0], uploadCert)
}

const readKeyFile = (event) => {
  if (event.target.files && event.target.files[0]) readPEM(event.target.files[0], uploadKey)
}

const uploadTLS = () => {
  if (!uploadCert.value || !uploadKey.value) {
    ElMessage.error(lang.tls_upload_required)
    return
  }
  http.post('/api/domain/ssl/upload', {domain: tlsDomain.value, cert: uploadCert.value, key: uploadKey.value}).then(res => {
    if (res.errorNo !== 0) {
      ElMessage.error(res.errorMsg)
      return
    }
    ElNotification({title: lang.succ, message: lang.tls_uploaded, type: 'success'})
    uploadCert.value = ''
    uploadKey.value = ''
    setTimeout(refreshTLS, 5000)
  })
}

watch(tlsVisible, (visible) => {
  if (!visible) stopTLSPolling()
})

onMounted(load)
onUnmounted(stopTLSPolling)
</script>

<style scoped>
.domain-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.domain-note {
  margin-bottom: 16px;
}

.domain-header-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.tls-alert {
  margin: 12px 0;
}

.tls-actions {
  display: flex;
  gap: 10px;
  margin: 12px 0;
}

.tls-records h4,
.tls-upload-grid + h4 {
  margin: 8px 0;
}

.tls-upload-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  margin-bottom: 12px;
}

.upload-label {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--md-sys-color-on-surface-variant);
}

.tls-upload-grid input[type="file"] {
  width: 100%;
}

@media (max-width: 768px) {
  .domain-header {
    flex-direction: column;
  }

  .tls-upload-grid {
    grid-template-columns: 1fr;
  }
}
</style>
