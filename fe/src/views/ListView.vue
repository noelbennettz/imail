<template>
  <div class="list-view-container">
    <div class="list-header">
      <div class="header-title">
        <h2>{{ keyword ? `${lang.search}: ${keyword}` : groupStore.name }}</h2>
      </div>
      <div class="header-actions">
        <el-button class="action-btn view-toggle" plain @click="toggleConversation">
          {{ conversationMode ? lang.messages : lang.conversations }}
        </el-button>
        <div class="selection-actions" v-if="hasSelection">
          <span class="selection-count">{{ selectedRows.length }}</span>
          <el-button @click="selectAll" class="action-btn selection-btn" plain>
            <el-icon><CircleCheck /></el-icon> {{ lang.select_all_btn }}
          </el-button>
          <el-button @click="invertSelection" class="action-btn selection-btn" plain>
            <el-icon><Switch /></el-icon> {{ lang.invert_selection_btn }}
          </el-button>
          <el-button @click="del" class="action-btn" plain>
            <el-icon><Delete /></el-icon> {{ lang.del_btn }}
          </el-button>
          <el-button @click="markRead" class="action-btn" plain>
            <el-icon><View /></el-icon> {{ lang.read_btn }}
          </el-button>
          <el-dropdown class="move-dropdown">
            <el-button class="action-btn" plain>
              <el-icon><Folder /></el-icon> {{ lang.move_btn }}
              <el-icon class="el-icon--right"><EpArrowDownBold/></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="move(group.id,group.name)" v-for="group in groupList" :key="group.id">
                  {{ group.name }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
        <el-button type="primary" class="compose-btn" @click="router.push('/editer')">
          <el-icon><EditPen /></el-icon> {{ lang.compose }}
        </el-button>
      </div>
    </div>

    <div class="list-content">
      <div v-if="loading && data.length === 0" class="skeleton-list" role="status" aria-live="polite">
        <span class="visually-hidden">{{ lang.loading }}</span>
        <div v-for="n in 8" :key="n" class="skeleton-row" aria-hidden="true">
          <span class="skeleton-dot"/>
          <span class="skeleton-line w-20"/>
          <span class="skeleton-line w-45"/>
          <span class="skeleton-line w-10"/>
        </div>
      </div>

      <div v-else-if="loadError" class="list-state" role="alert">
        <el-icon class="state-icon"><WarningFilled/></el-icon>
        <p class="state-title">{{ loadError }}</p>
        <el-button @click="refresh" class="state-action">{{ lang.retry }}</el-button>
      </div>

      <div v-else-if="data.length === 0" class="list-state">
        <el-icon class="state-icon"><Message/></el-icon>
        <p class="state-title">{{ keyword ? lang.empty_search : lang.empty_folder }}</p>
        <el-button v-if="keyword" text class="state-action" @click="clearSearch">{{ lang.clear_search }}</el-button>
      </div>

      <el-table v-else height="100%"
                ref="taskTableDataRef"
                :data="data"
                :show-header="false"
                class="modern-mail-table"
                @row-click="rowClick"
                @selection-change="selectionChange"
                :row-style="rowStyle"
      >
        <el-table-column type="selection" width="40" class-name="selection-col"/>

        <el-table-column width="30" class-name="status-col">
          <template #default="scope">
            <div class="status-indicator">
              <el-badge is-dot class="unread-dot" type="primary" v-if="!scope.row.is_read"></el-badge>
              <el-tooltip effect="dark" :content="lang.dangerous" placement="top-start" v-if="scope.row.dangerous">
                <el-icon color="#ef4444"><Warning /></el-icon>
              </el-tooltip>
              <el-tooltip effect="dark" :content="scope.row.error" placement="top-start" v-if="scope.row.error !== ''">
                <el-icon color="#ef4444"><Warning /></el-icon>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>

        <el-table-column min-width="250">
          <template #default="scope">
            <div class="mail-row-content" :class="{'is-unread': !scope.row.is_read}">
              <div class="mail-main-info">
                <div class="mail-sender">
                  {{ scope.row.sender.Name !== '' ? scope.row.sender.Name : scope.row.sender.EmailAddress }}
                </div>
                <div class="mail-subject">
                  {{ scope.row.title || lang.no_subject }}
                  <span v-if="scope.row.thread_count > 1" class="thread-count">{{ scope.row.thread_count }}</span>
                </div>
                <div class="mail-snippet">{{ scope.row.desc }}</div>
              </div>
              <div class="mail-meta">
                <div class="mail-date">{{ formatShortDate(scope.row.datetime) }}</div>
              </div>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div class="pagination-wrapper" v-if="totalPage > 1">
      <el-pagination
          background
          layout="prev, pager, next"
          :page-count="totalPage"
          :current-page="currentPage"
          @current-change="pageChange"
      />
    </div>
  </div>
</template>

<script setup>
import {EpArrowDownBold} from "vue-icons-plus/ep";
import {Delete, View, Folder, EditPen, Warning, WarningFilled, CircleCheck, Switch, Message} from "@element-plus/icons-vue";
import {useRoute, useRouter} from 'vue-router'
import {ref, watch, computed} from 'vue'
import useGroupStore from '../stores/group'
import lang from '../i18n/i18n';
import {http} from "@/utils/axios";
import {confirmAction} from "@/utils/confirm";
import {ElMessage} from "element-plus";

const router = useRouter();
const route = useRoute();
const groupStore = useGroupStore()
const groupList = ref([])
const taskTableDataRef = ref(null)
const selectedRows = ref([])
const data = ref([])
const rawData = ref([])
const totalPage = ref(0)
const currentPage = ref(1)
const pageSize = 15
const loading = ref(false)
const loadError = ref('')
const conversationMode = ref(localStorage.getItem('imail-view-mode') !== 'list')
const keyword = computed(() => String(route.query.q || ''))
const hasSelection = computed(() => selectedRows.value.length > 0)

const defaultTag = () => (groupStore.tag || '{"type":0,"status":-1}')

// Conversation grouping is a client-side view of the current page, so it keys on
// the normalized subject *and* the other party. Subject alone merged unrelated
// mail from different senders.
const threadKey = (row) => {
  const subject = (row.title || '')
      .replace(/^((re|fwd|fw|回复|回覆|答复|转寄|轉寄|转发)\s*:\s*)+/i, '')
      .trim()
      .toLowerCase()
  const sender = String(row.sender?.EmailAddress || '').toLowerCase()
  return `${subject}|${sender}` || ('id:' + row.id)
}

const applyView = (list) => {
  if (!conversationMode.value) {
    return list
  }
  const groups = new Map()
  for (const item of list) {
    const key = threadKey(item)
    if (!groups.has(key)) {
      groups.set(key, {...item, thread_count: 1, thread_ids: [item.id]})
      continue
    }
    const group = groups.get(key)
    group.thread_count += 1
    group.thread_ids.push(item.id)
    if (item.id > group.id) {
      const count = group.thread_count
      const ids = group.thread_ids
      Object.assign(group, item)
      group.thread_count = count
      group.thread_ids = ids
    }
  }
  return Array.from(groups.values()).sort((a, b) => b.id - a.id)
}

const setData = (list) => {
  rawData.value = list || []
  data.value = applyView(rawData.value)
  selectedRows.value = []
}

const toggleConversation = () => {
  conversationMode.value = !conversationMode.value
  localStorage.setItem('imail-view-mode', conversationMode.value ? 'conversation' : 'list')
  data.value = applyView(rawData.value)
}

// An empty page after deleting or moving the last rows is normal, not an error:
// step back one page instead of showing an empty folder.
const fetchList = function (page = currentPage.value) {
  loading.value = true
  loadError.value = ''
  http.post("/api/email/list", {
    tag: defaultTag(),
    page_size: pageSize,
    current_page: page,
    keyword: keyword.value
  }).then(res => {
    if (res.errorNo !== 0) {
      loadError.value = res.errorMsg || lang.load_failed
      return
    }
    currentPage.value = res.data.current_page || page
    totalPage.value = res.data.total_page || 0
    setData(res.data.list)
    if (data.value.length === 0 && currentPage.value > 1) {
      return fetchList(currentPage.value - 1)
    }
  }).catch(err => {
    loadError.value = err?.userMessage || lang.load_failed
  }).finally(() => {
    loading.value = false
  })
}

const refresh = () => fetchList()

const updateGroupList = function () {
  http.post("/api/group/list", {}, {silent: true}).then(res => {
    if (res.errorNo === 0) groupList.value = res.data || []
  }).catch(() => {})
}

fetchList()
updateGroupList()

watch(() => groupStore.tag, () => {
  currentPage.value = 1
  setData([])
  fetchList(1)
})

watch(() => route.query.q, () => {
  currentPage.value = 1
  setData([])
  fetchList(1)
})

const rowClick = function (row) {
  const query = conversationMode.value ? {thread: '1'} : {}
  router.push({path: "/detail/" + row.id, query})
}

const formatShortDate = (dateStr) => {
  if (!dateStr) return "";
  const d = new Date(dateStr);
  if (Number.isNaN(d.getTime())) return "";
  const now = new Date();
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
  }
  const options = {month: 'short', day: 'numeric'};
  if (d.getFullYear() !== now.getFullYear()) options.year = 'numeric';
  return d.toLocaleDateString([], options);
}

const selectionChange = (rows) => {
  selectedRows.value = rows
}

const selectAll = () => {
  taskTableDataRef.value?.clearSelection()
  data.value.forEach(row => taskTableDataRef.value?.toggleRowSelection(row, true))
}

const invertSelection = () => {
  const selectedIds = new Set(selectedRows.value.map(row => row.id))
  data.value.forEach(row => {
    taskTableDataRef.value?.toggleRowSelection(row, !selectedIds.has(row.id))
  })
}

const markRead = function () {
  const rows = taskTableDataRef.value?.getSelectionRows()
  if (!rows || rows.length === 0) {
    ElMessage.warning(lang.select_first)
    return
  }
  const ids = rows.map(e => e.id)
  http.post("/api/email/read", {"ids": ids}).then(res => {
    if (res.errorNo === 0) {
      refresh()
      groupStore.refreshItems()
      ElMessage.success(lang.marked_read)
    } else {
      ElMessage.error(res.errorMsg)
    }
  }).catch(() => {})
}

const move = async function (group_id, group_name) {
  const rows = taskTableDataRef.value?.getSelectionRows()
  if (!rows || rows.length === 0) {
    ElMessage.warning(lang.select_first)
    return
  }
  const ids = rows.map(e => e.id)
  if (!await confirmAction(lang.move_email_confirm)) return
  http.post("/api/email/move", {"group_id": group_id, "group_name": group_name, "ids": ids}).then(res => {
    if (res.errorNo === 0) {
      refresh()
      groupStore.refreshItems()
      ElMessage.success(lang.moved)
    } else {
      ElMessage.error(res.errorMsg)
    }
  }).catch(() => {})
}

const del = async function () {
  const rows = taskTableDataRef.value?.getSelectionRows()
  if (!rows || rows.length === 0) {
    ElMessage.warning(lang.select_first)
    return
  }
  const ids = rows.map(e => e.id)
  let groupTag = {}
  try {
    groupTag = JSON.parse(defaultTag())
  } catch {
    groupTag = {}
  }
  if (!await confirmAction(lang.del_email_confirm)) return
  http.post("/api/email/del", {"ids": ids, "forcedDel": groupTag.status === 3}).then(res => {
    if (res.errorNo === 0) {
      refresh()
      groupStore.refreshItems()
      ElMessage.success(lang.deleted)
    } else {
      ElMessage.error(res.errorMsg)
    }
  }).catch(() => {})
}

const clearSearch = () => {
  router.push({name: 'list', query: {}})
}

const rowStyle = function () {
  return {'cursor': 'pointer'}
}

const pageChange = function (p) {
  currentPage.value = p
  fetchList(p)
}
</script>

<style scoped>
.list-view-container {
  display: flex;
  flex-direction: column;
  /* [core fix] Prefer the parent's height while keeping the viewport height as a floor */
  height: 100%;
  min-height: 100vh;
  background: var(--pm-surface-glass);
  border: 1px solid var(--pm-glass-border);
  border-radius: var(--pm-radius-xl);
  box-shadow: var(--pm-shadow-md);
  backdrop-filter: blur(16px);
  padding: 18px;
  overflow: hidden;
}

.list-header {
  flex-shrink: 0;
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 14px;
  flex-wrap: wrap;
  gap: 12px;
}

.header-title h2 {
  font-size: 30px;
  font-weight: 600;
  color: var(--pm-text-primary);
  margin: 0;
  letter-spacing: -0.02em;
}

.header-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.selection-actions {
  display: flex;
  padding-right: 8px;
  margin-right: 2px;
  border-right: 1px solid var(--pm-border-color);
}

.selection-actions .selection-btn + .selection-btn {
  margin-left: 8px;
}

.action-btn {
  border-radius: 999px;
  border-color: var(--pm-border-color);
  color: var(--pm-text-secondary);
  background: var(--pm-surface-glass);
  font-weight: 500;
}

.action-btn:hover {
  background-color: var(--pm-bg-hover);
  color: var(--pm-text-primary);
  border-color: var(--pm-border-color);
}

.compose-btn {
  border-radius: 999px;
  font-weight: 600;
  margin-left: 8px;
  padding-inline: 16px;
  box-shadow: 0 8px 16px rgba(0, 113, 227, 0.2);
}

.compose-btn:hover {
  box-shadow: 0 14px 24px rgba(0, 113, 227, 0.24);
}

.list-content {
  min-height: 0;
  flex: 1;
  overflow: hidden;
  background: var(--pm-surface-glass);
  border-radius: var(--pm-radius-lg);
  border: 1px solid var(--pm-border-color);
  box-shadow: inset 0 1px 0 var(--pm-glass-border);
}

.modern-mail-table {
  width: 100%;
}

.modern-mail-table :deep(tr) {
  transition: background-color .15s ease, border-color .15s ease;
}

.modern-mail-table :deep(tr:hover > td) {
  background-color: var(--pm-row-hover) !important;
}

.modern-mail-table :deep(td) {
  padding: 13px 0;
}

.status-indicator {
  display: flex;
  justify-content: center;
  align-items: center;
}

.mail-row-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.mail-main-info {
  display: flex;
  align-items: center;
  width: 100%;
  gap: 12px;
}

.mail-sender {
  width: 172px;
  min-width: 172px;
  font-weight: 600;
  color: var(--pm-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 14px;
}

.mail-subject {
  flex: 0 1 auto;
  max-width: 40%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 500;
  color: var(--pm-text-primary);
}

.thread-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  margin-left: 8px;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
  font-size: 11px;
  font-weight: 600;
}

.header-actions :deep(.el-button),
.move-dropdown :deep(.el-button) {
  height: 40px;
  padding: 0 16px;
  border-radius: 999px;
}

.compose-btn {
  height: 40px;
  padding: 0 18px;
  border-radius: 999px;
  box-shadow: none;
}

.mail-snippet {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--pm-text-muted);
}

.mail-meta {
  min-width: 80px;
  text-align: right;
  padding-right: 16px;
}

.mail-date {
  font-size: 12px;
  color: var(--pm-text-secondary);
  font-weight: 500;
}

.is-unread .mail-sender,
.is-unread .mail-subject {
  font-weight: 700;
  color: var(--pm-text-primary);
}

.is-unread .mail-date {
  color: var(--pm-primary-color);
  font-weight: 700;
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
}

.skeleton-list {
  padding: 8px 20px;
}

.skeleton-row {
  display: flex;
  align-items: center;
  gap: 16px;
  height: 58px;
  border-bottom: 1px solid var(--pm-border-subtle);
}

.skeleton-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--md-sys-color-surface-container-high);
}

.skeleton-line {
  height: 12px;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--md-sys-color-surface-container-high) 25%, var(--md-sys-color-surface-container-highest) 37%, var(--md-sys-color-surface-container-high) 63%);
  background-size: 400% 100%;
  animation: skeleton-shimmer 1.4s ease infinite;
}

.skeleton-line.w-20 { width: 20%; }
.skeleton-line.w-45 { width: 45%; }
.skeleton-line.w-10 { width: 10%; }

@keyframes skeleton-shimmer {
  0% { background-position: 100% 50%; }
  100% { background-position: 0 50%; }
}

@media (prefers-reduced-motion: reduce) {
  .skeleton-line { animation: none; }
}

.list-state {
  height: 100%;
  min-height: 280px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 32px;
  text-align: center;
  color: var(--pm-text-secondary);
}

.state-icon {
  font-size: 44px;
  color: var(--md-sys-color-outline);
}

.state-title {
  font-size: 15px;
  line-height: 1.6;
  max-width: 34ch;
}

.state-action {
  margin-top: 4px;
}

.selection-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  height: 24px;
  margin-right: 8px;
  border-radius: 999px;
  background: var(--md-sys-color-primary);
  color: var(--md-sys-color-on-primary);
  font-size: 12px;
  font-weight: 700;
}

.pagination-wrapper {
  flex-shrink: 0;
  padding: 10px 0;
  padding-bottom: calc(10px + env(safe-area-inset-bottom)); /* [core fix] Accommodate the iPhone home indicator */
  background-color: transparent; /* [core fix] Drop the white background so dark mode does not look off */
  margin-top: 14px;
  display: flex;
  justify-content: center;
}

:root[data-theme="dark"] .list-content {
  background: var(--pm-surface-glass-soft);
}

@media (max-width: 768px) {
  .list-view-container {
    /* [core fix] On mobile the height must be computed explicitly, subtracting the Mobile Header height (usually about 56px); use dvh so scrolling is not clipped */
    height: calc(100dvh - 56px);
    min-height: auto;
    padding: 12px; /* Reduce padding on mobile to leave more visible space */
  }

  .list-header {
    flex-direction: column;
    align-items: flex-start;
  }
  .header-actions {
    width: 100%;
    overflow-x: auto;
    padding-bottom: 4px;
  }
  .mail-main-info {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }
  .mail-sender {
    width: 100%;
  }
  .mail-subject {
    max-width: 100%;
  }
  .mail-snippet {
    display: none;
  }
  .mail-row-content {
    align-items: flex-start;
  }
  .mail-meta {
    padding-top: 2px;
  }
}

@media (max-width: 768px) {
  .modern-mail-table :deep(.selection-col) {
    display: none;
  }
}
</style>
