<template>
  <div class="settings-card">
    <div class="settings-header">
      <h3>{{ lang.account_management }}</h3>
      <p class="settings-desc">{{ lang.account_management_desc }}</p>
    </div>

    <div class="domain-filter">
      <span class="filter-label">{{ lang.domain_label }}</span>
      <el-select v-model="selectedDomain" style="width: 240px" @change="onDomainChange">
        <el-option v-for="domain in domains" :key="domain" :label="domain" :value="domain"/>
      </el-select>
    </div>

    <div class="table-container">
      <el-table :data="userList" class="modern-table" style="width: 100%">
        <el-table-column label="ID" prop="ID" width="80"/>
        <el-table-column :label="lang.account" prop="Account" min-width="150" show-overflow-tooltip/>
        <el-table-column :label="lang.user_name" prop="Name" min-width="120" show-overflow-tooltip/>
        <el-table-column label="Quota" min-width="170">
          <template #default="scope">
            <div class="quota-cell">
              <el-progress
                  :percentage="quotaPercent(scope.row)"
                  :stroke-width="6"
                  :show-text="false"
                  :status="quotaPercent(scope.row) >= 90 ? 'exception' : ''"
              />
              <span class="quota-text">
                {{ formatBytes(scope.row.quota_used) }} / {{ scope.row.quota > 0 ? formatBytes(scope.row.quota) : '∞' }}
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="lang.disabled" prop="Disabled" width="120">
          <template #default="scope">
            <el-tag :type="scope.row.Disabled === 1 ? 'info' : 'success'" size="small" effect="plain" class="status-tag">
              {{ scope.row.Disabled === 1 ? lang.disabled : lang.enabled }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column align="right" width="140" class-name="action-cell">
          <template #header>
            <el-button type="primary" size="small" @click="createUser" class="new-btn" plain>
              <el-icon><Plus/></el-icon> New
            </el-button>
          </template>
          <template #default="scope">
            <el-button size="small" type="primary" text bg @click="handleEdit(scope.$index, scope.row)" class="action-btn">
              Edit
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div class="pagination-wrapper">
      <el-pagination 
        v-model:current-page="currentPage" 
        small 
        background 
        layout="prev, pager, next"
        :page-count="totalPage" 
        @current-change="reflushList"
      />
    </div>

    <el-dialog v-model="userInfoDialog" :title="title" width="450px" class="premium-dialog">
      <div class="dialog-content">
        <el-form label-position="top">
          <el-form-item :label="lang.account">
            <el-input :disabled="editModel === 'edit'" v-model="editUserInfo.account"/>
          </el-form-item>

          <el-form-item :label="lang.user_name">
            <el-input v-model="editUserInfo.name"/>
          </el-form-item>

          <el-form-item label="Domain">
            <el-select v-model="editUserInfo.domain" placeholder="Primary domain" style="width: 100%">
              <el-option v-for="domain in domains" :key="domain" :label="domain" :value="domain"/>
            </el-select>
          </el-form-item>

          <el-form-item label="Quota (MiB, 0 = unlimited)">
            <el-input-number v-model="editUserInfo.quotaMB" :min="0" :step="256" controls-position="right" style="width: 100%"/>
          </el-form-item>

          <el-form-item :label="lang.password">
            <el-input :placeholder="lang.resetPwd" v-model="editUserInfo.password" type="password" show-password/>
          </el-form-item>

          <el-form-item>
            <div class="status-switch">
              <span class="switch-label">Status</span>
              <el-switch 
                v-model="editUserInfo.disabled" 
                class="ml-2" 
                :active-text="lang.disabled"
                :inactive-text="lang.enabled"
                active-color="#ef4444"
                inactive-color="#10b981"
              />
            </div>
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="userInfoDialog = false">Cancel</el-button>
          <el-button type="primary" @click="submit">Confirm</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import {onMounted, reactive, ref} from 'vue'
import lang from '../i18n/i18n';
import {http} from "@/utils/axios";
import {ElNotification} from "element-plus";
import {Plus} from "@element-plus/icons-vue";
import {useGlobalStatusStore} from "@/stores/useGlobalStatusStore";

const globalStatus = useGlobalStatusStore();
const domains = ref([]);
const selectedDomain = ref("");

const loadDomains = () => {
  http.post('/api/domain/list', {}).then(res => {
    if (res.errorNo === 0 && res.data.list) {
      domains.value = res.data.list.map(item => item.domain)
    } else {
      domains.value = globalStatus.userInfos.domains || []
    }
    if (!selectedDomain.value && domains.value.length > 0) {
      selectedDomain.value = domains.value[0]
    }
    reflushList()
  }).catch(() => {
    domains.value = globalStatus.userInfos.domains || []
    selectedDomain.value = domains.value[0] || ""
    reflushList()
  })
}

const onDomainChange = () => {
  currentPage.value = 1
  reflushList()
}

const userList = reactive([])
const currentPage = ref(1)
const totalPage = ref(1)
const userInfoDialog = ref(false)
const editModel = ref("edit")
const editUserInfo = reactive({
  "id": 0,
  "account": "",
  "name": "",
  "domain": "",
  "quotaMB": 0,
  "password": "",
  "disabled": false
})
const title = ref(lang.editUser)

const formatBytes = (bytes) => {
  const value = Number(bytes || 0)
  if (value <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / Math.pow(1024, index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

const quotaPercent = (row) => {
  if (!row.quota || row.quota <= 0) return 0
  return Math.min(100, Math.round((row.quota_used / row.quota) * 100))
}

const reflushList = function () {
  http.post('/api/user/list', {"current_page": currentPage.value, "page_size": 10, "domain": selectedDomain.value}).then(res => {
    userList.length = 0
    totalPage.value = res.data.total_page
    if (res.data["list"]) {
      userList.push(...res.data["list"])
    }
  })
}

const handleEdit = function (idx, row) {
  editUserInfo.id = row.ID
  editUserInfo.account = row.Account
  editUserInfo.name = row.Name
  editUserInfo.domain = row.domain || selectedDomain.value || (domains.value[0] || "")
  editUserInfo.quotaMB = row.quota > 0 ? Math.round(row.quota / (1024 * 1024)) : 0
  editUserInfo.disabled = row.Disabled === 1
  editUserInfo.password = ""
  editModel.value = "edit"
  title.value = lang.editUser
  userInfoDialog.value = true
}

const createUser = function () {
  editUserInfo.id = 0
  editUserInfo.account = ""
  editUserInfo.name = ""
  editUserInfo.domain = selectedDomain.value || domains.value[0] || ""
  editUserInfo.quotaMB = 0
  editUserInfo.disabled = false
  editUserInfo.password = ""
  editModel.value = "create"
  title.value = lang.newUser
  userInfoDialog.value = true
}

const submit = function () {
  if (editModel.value === 'edit') {
    let newData = {
      "id": editUserInfo.id,
      "account": editUserInfo.account,
      "username": editUserInfo.name,
      "domain": editUserInfo.domain,
      "quota_mb": editUserInfo.quotaMB,
      "disabled": editUserInfo.disabled ? 1 : 0
    }
    if (editUserInfo.password !== "") {
      newData["password"] = editUserInfo.password
    }

    http.post('/api/user/edit', newData).then(res => {
      ElNotification({
        title: res.errorNo === 0 ? lang.succ : lang.fail,
        message: res.errorNo === 0 ? "" : res.data,
        type: res.errorNo === 0 ? 'success' : 'error',
      })
      if (res.errorNo === 0) {
        reflushList()
        userInfoDialog.value = false
      }
    })
  } else {
    let newData = {
      "account": editUserInfo.account,
      "username": editUserInfo.name,
      "domain": editUserInfo.domain,
      "quota_mb": editUserInfo.quotaMB,
      "disabled": editUserInfo.disabled ? 1 : 0,
      "password": editUserInfo.password
    }

    http.post('/api/user/create', newData).then(res => {
      ElNotification({
        title: res.errorNo === 0 ? lang.succ : lang.fail,
        message: res.errorNo === 0 ? "" : res.data,
        type: res.errorNo === 0 ? 'success' : 'error',
      })
      if (res.errorNo === 0) {
        reflushList()
        userInfoDialog.value = false
      }
    })
  }
}

onMounted(loadDomains)
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

.table-container {
  border: 1px solid var(--pm-border-color);
  border-radius: var(--pm-radius-sm);
  overflow: hidden;
  margin-bottom: 24px;
}

.modern-table :deep(th.el-table__cell) {
  background-color: var(--pm-bg-secondary);
  color: var(--pm-text-secondary);
  font-weight: 600;
}

.status-tag {
  border-radius: var(--pm-radius-sm);
}

.new-btn {
  border-radius: 999px;
  height: 34px;
}

.action-btn {
  border-radius: 999px;
  height: 34px;
  padding: 0 14px;
}

.pagination-wrapper {
  margin-top: 20px;
  display: flex;
  justify-content: center;
}

/* Dialog Styles */
.premium-dialog :deep(.el-dialog__header) {
  border-bottom: 1px solid var(--pm-border-color);
  padding-bottom: 16px;
  margin-bottom: 20px;
}

.dialog-content {
  padding: 0 8px;
}

.status-switch {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;
  padding: 8px 12px;
  background: var(--pm-bg-secondary);
  border-radius: var(--pm-radius-sm);
  border: 1px solid var(--pm-border-color);
}

.switch-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--pm-text-primary);
}

.quota-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-right: 8px;
}

.quota-text {
  font-size: 11px;
  color: var(--pm-text-secondary);
  white-space: nowrap;
}

.domain-filter {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.filter-label {
  font-size: 13px;
  color: var(--md-sys-color-on-surface-variant);
}

@media (max-width: 768px) {
  .table-container {
    overflow-x: auto;
  }

  .modern-table {
    min-width: 720px;
  }
}
</style>