<template>
  <div class="mail-detail-container">
    <div class="mail-detail-header">
      <el-button @click="$router.back()" plain class="back-btn">
        <el-icon><Back /></el-icon>
      </el-button>
      <div class="action-buttons">
        <el-button plain @click="compose('reply')">
          <el-icon><ChatLineRound/></el-icon> {{ lang.reply }}
        </el-button>
        <el-button plain @click="compose('reply_all')">
          <el-icon><ChatDotRound/></el-icon> {{ lang.reply_all }}
        </el-button>
        <el-button plain @click="compose('forward')">
          <el-icon><Promotion/></el-icon> {{ lang.forward }}
        </el-button>
        <el-button plain @click="handleDelete">
          <el-icon><Delete /></el-icon>
        </el-button>
      </div>
    </div>

    <div class="mail-detail-content" ref="contentRef" @scroll.passive="onContentScroll">
      <div class="mail-subject-row">
        <h1 class="mail-subject">{{ isThread ? threadSubject : detailData.subject }}</h1>
        <span v-if="isThread && messages.length > 1" class="thread-count-pill">
          {{ messages.length }} {{ lang.messages }}
        </span>
      </div>

      <div v-for="(message, messageIndex) in shownMessages" :key="message.id || messageIndex" class="thread-message">
        <div v-if="isThread && messageIndex > 0" class="thread-separator"/>

        <div v-if="isThread && message.subject" class="thread-message-subject">
          {{ message.subject }}
        </div>

        <div class="mail-meta-card">
          <div class="meta-left">
            <div class="avatar-placeholder">
              {{ getInitial(message.from_name || message.from_address) }}
            </div>
            <div class="meta-info">
              <div class="sender-line">
                <span class="sender-name">{{ message.from_name !== '' ? message.from_name : message.from_address }}</span>
                <span class="sender-email" v-if="message.from_name !== ''">&lt;{{ message.from_address }}&gt;</span>
              </div>
              <div class="receivers-line">
                <span class="meta-label">{{ lang.to }}:</span>
                <span v-for="(to, index) in message._tos" :key="index" class="receiver-chip">
                  {{ to.Name !== '' ? to.Name : to.EmailAddress }}<span v-if="index < message._tos.length - 1">, </span>
                </span>
                <span v-if="message._ccs.length > 0" class="cc-section">
                  <span class="meta-label">{{ lang.cc }}:</span>
                  <span v-for="(item, index) in message._ccs" :key="'cc'+index" class="receiver-chip">
                    {{ item.Name !== '' ? item.Name : item.EmailAddress }}<span v-if="index < message._ccs.length - 1">, </span>
                  </span>
                </span>
              </div>
            </div>
          </div>
          <div class="meta-right">
            <div class="mail-date">{{ formatDetailDate(message.send_date) }}</div>
            <el-dropdown trigger="click" @command="(command) => handleMessageCommand(command, message)">
              <button type="button" class="message-menu-btn" :aria-label="lang.message_actions">
                <el-icon><MoreFilled/></el-icon>
              </button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="source">{{ lang.view_source }}</el-dropdown-item>
                  <el-dropdown-item command="delete" divided>{{ lang.delete_message }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>

        <el-divider class="custom-divider"/>

        <div class="mail-body">
          <div class="body-text" v-if="!message.html">
            {{ message.text }}
          </div>
          <MailBodyFrame v-else :html="message.html" />
        </div>

        <div v-if="message.attachments && message.attachments.length > 0" class="attachments-section">
          <el-divider class="custom-divider"/>
          <div class="attachments-title">{{ lang.attachment }} ({{ message.attachments.length }})</div>
          <div class="attachments-list">
            <a class="attachment-card" v-for="item in message.attachments" :key="item.Index" :href="'/attachments/download/' + message.id + '/' + item.Index">
              <div class="att-icon"><el-icon><Document/></el-icon></div>
              <div class="att-name">{{ item.Filename }}</div>
              <div class="att-download"><el-icon><Download/></el-icon></div>
            </a>
          </div>
        </div>
      </div>

      <div v-if="isThread && threadHasMore" class="thread-load-more">
        <el-button :loading="loadingMore" @click="loadMore">{{ lang.load_more }}</el-button>
        <span class="thread-progress">{{ messages.length }} / {{ threadTotal }}</span>
      </div>
      <div v-else-if="isThread && threadTotal > threadPageSize" class="thread-load-end">
        {{ lang.all_loaded }} ({{ threadTotal }})
      </div>
    </div>

    <el-dialog v-model="sourceVisible" :title="lang.view_source" width="820px">
      <pre class="source-view">{{ sourceText }}</pre>
      <template #footer>
        <el-button @click="copySource">{{ lang.copy }}</el-button>
        <el-button @click="sourceVisible = false">Close</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import {computed, ref, watch} from 'vue'
import {useRoute, useRouter} from 'vue-router'
import {ChatDotRound, ChatLineRound, Document, Back, Delete, Download, Promotion, MoreFilled} from '@element-plus/icons-vue';
import {ElMessage, ElMessageBox} from 'element-plus';
import lang from '../i18n/i18n';
import {http} from "@/utils/axios";
import useGroupStore from '../stores/group';
import {useGlobalStatusStore} from "@/stores/useGlobalStatusStore";
import MailBodyFrame from '../components/MailBodyFrame.vue';

const route = useRoute()
const router = useRouter()
const groupStore = useGroupStore()
const contentRef = ref(null)
const detailData = ref({
  attachments: []
})
const messages = ref([])
const threadPageSize = 5
const threadTotal = ref(0)
const loadingMore = ref(false)
const sourceVisible = ref(false)
const sourceText = ref('')

const isThread = computed(() => route.query.thread === '1')
const threadHasMore = computed(() => isThread.value && messages.value.length < threadTotal.value)
const shownMessages = computed(() => {
  const list = isThread.value ? messages.value : [detailData.value]
  return list.filter(message => message && (message.id || message.subject))
})
const threadSubject = computed(() => {
  if (messages.value.length === 0) return detailData.value.subject || ''
  return messages.value[messages.value.length - 1].subject || detailData.value.subject || ''
})
const activeMessage = computed(() => {
  if (isThread.value && messages.value.length > 0) {
    return messages.value[messages.value.length - 1]
  }
  return detailData.value
})

const parseAddresses = (raw) => {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

const prepareMessage = (data) => {
  const message = {...(data || {})}
  message.subject = data?.subject || ''
  message.from_name = data?.from_name || ''
  message.from_address = data?.from_address || ''
  message.text = data?.text || ''
  message.html = data?.html || ''
  message.attachments = data?.attachments || []
  message._tos = parseAddresses(data?.to)
  message._ccs = parseAddresses(data?.cc)
  return message
}

const loadThreadPage = (reset) => {
  if (loadingMore.value) return
  const id = parseInt(route.params.id)
  if (!id) return
  if (!reset && !threadHasMore.value) return
  const offset = reset ? 0 : messages.value.length
  loadingMore.value = true
  http.post("/api/email/thread", {id, offset, limit: threadPageSize}).then(res => {
    loadingMore.value = false
    if (res.errorNo !== 0) {
      ElMessage.error(res.errorMsg || 'Unable to load conversation')
      return
    }
    const page = res.data || {}
    const list = (page.list || []).map(prepareMessage)
    if (reset) {
      messages.value = list
    } else {
      const known = new Set(messages.value.map(item => item.id))
      messages.value = messages.value.concat(list.filter(item => !known.has(item.id)))
    }
    threadTotal.value = page.total || messages.value.length
    if (messages.value.length > 0) {
      detailData.value = messages.value[messages.value.length - 1]
    }
  }).catch(() => {
    loadingMore.value = false
  })
}

const loadMore = () => loadThreadPage(false)

const onContentScroll = (event) => {
  if (!threadHasMore.value || loadingMore.value) return
  const el = event.target
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 160) {
    loadMore()
  }
}

const loadDetail = () => {
  const id = parseInt(route.params.id)
  if (!id) return
  messages.value = []
  threadTotal.value = 0
  if (isThread.value) {
    loadThreadPage(true)
    return
  }
  http.post("/api/email/detail", {id}).then(res => {
    if (res.errorNo !== 0) return
    detailData.value = prepareMessage(res.data)
  })
}

loadDetail()
watch(() => [route.params.id, route.query.thread], loadDetail)

const getInitial = (name) => {
  if (!name) return '?';
  return name.charAt(0).toUpperCase();
}

const formatDetailDate = (dateStr) => {
  if (!dateStr) return "";
  const d = new Date(dateStr);
  return d.toLocaleString([], {
    year: 'numeric', month: 'short', day: 'numeric',
    hour: '2-digit', minute:'2-digit'
  });
}

const router2 = router
const globalStatus = useGlobalStatusStore()

const stripHtml = (html) => {
  if (!html) return ''
  return html.replace(/<[^>]*>/g, ' ')
}

const compose = (mode) => {
  const current = activeMessage.value
  const baseSubject = (current.subject || '').replace(/^((re|fwd|aw|wg)\s*:\s*)+/i, '').trim()
  const body = (current.text || stripHtml(current.html) || '').trim()
  const quoted = `\n\n--- ${lang.forward} ---\n` +
      `${lang.from}: ${current.from_name || ''} <${current.from_address || ''}>\n` +
      `${lang.date}: ${formatDetailDate(current.send_date)}\n` +
      `${lang.subject}: ${current.subject || ''}\n\n${body}`
  const query = {
    subject: (mode === 'forward' ? 'Fwd: ' : 'Re: ') + baseSubject,
    body: quoted,
  }
  if (mode === 'reply') {
    query.to = current.from_address || ''
  }
  if (mode === 'reply_all') {
    const own = `${globalStatus.userInfos.account}@${globalStatus.userInfos.domain || (globalStatus.userInfos.domains || [])[0]}`
    const recipients = [current.from_address, ...(current._tos || []).map(item => item.EmailAddress)]
        .filter(item => item && item.toLowerCase() !== own.toLowerCase())
    query.to = recipients.join(',')
    const cc = (current._ccs || []).map(item => item.EmailAddress)
        .filter(item => item && item.toLowerCase() !== own.toLowerCase())
    if (cc.length > 0) query.cc = cc.join(',')
  }
  router2.push({path: '/editer', query})
}

const removeMessage = (id) => {
  const index = messages.value.findIndex(item => item.id === id)
  if (index >= 0) {
    messages.value.splice(index, 1)
  }
  threadTotal.value = Math.max(0, threadTotal.value - 1)
  if (messages.value.length === 0) {
    router.push({name: 'list'})
    return
  }
  if (detailData.value.id === id) {
    detailData.value = messages.value[messages.value.length - 1]
  }
  if (String(route.params.id) === String(id)) {
    router.replace({path: '/detail/' + messages.value[0].id, query: {thread: '1'}})
  }
}

const deleteMessage = (message) => {
  const id = message.id
  if (!id) return
  let forcedDel = false
  try {
    const groupTag = JSON.parse(groupStore.tag || '{"type":0,"status":-1}')
    forcedDel = groupTag.status === 3
  } catch {
    forcedDel = false
  }
  ElMessageBox.confirm(lang.del_email_confirm, 'Warning', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(() => {
    http.post("/api/email/del", {ids: [id], forcedDel}).then(res => {
      if (res.errorNo !== 0) {
        ElMessage.error(res.errorMsg)
        return
      }
      ElMessage.success('Deleted successfully')
      if (isThread.value) {
        removeMessage(id)
      } else {
        router.push({name: 'list'})
      }
    })
  }).catch(() => {})
}

const showSource = (message) => {
  if (!message.id) return
  sourceText.value = ''
  sourceVisible.value = true
  http.post("/api/email/source", {id: message.id}).then(res => {
    if (res.errorNo !== 0) {
      sourceVisible.value = false
      ElMessage.error(res.errorMsg)
      return
    }
    sourceText.value = res.data.source || ''
  })
}

const copySource = async () => {
  try {
    await navigator.clipboard.writeText(sourceText.value)
    ElMessage.success(lang.copied)
  } catch {
    ElMessage.error('Clipboard unavailable')
  }
}

const handleMessageCommand = (command, message) => {
  if (command === 'source') showSource(message)
  if (command === 'delete') deleteMessage(message)
}

const handleDelete = () => {
  const id = activeMessage.value.id || parseInt(route.params.id);
  if (!id) return;

  let tag = groupStore.tag;
  if (!tag) {
    tag = '{"type":0,"status":-1}';
  }

  let forcedDel = false;
  try {
    const groupTag = JSON.parse(tag);
    forcedDel = groupTag.status === 3;
  } catch (e) {
    forcedDel = false;
  }

  ElMessageBox.confirm(lang.del_email_confirm, 'Warning', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(() => {
    http.post("/api/email/del", {ids: [id], forcedDel}).then(res => {
      if (res.errorNo === 0) {
        ElMessage.success('Deleted successfully');
        router.push({name: 'list'});
      } else {
        ElMessage.error(res.errorMsg);
      }
    });
  }).catch(() => {});
}
</script>

<style scoped>
.mail-detail-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--pm-surface-glass);
  border-radius: var(--pm-radius-xl);
  box-shadow: var(--pm-shadow-md);
  border: 1px solid var(--pm-glass-border);
  backdrop-filter: blur(18px);
  overflow: hidden;
}

.mail-detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 24px;
  border-bottom: 1px solid var(--pm-border-color);
  background: var(--pm-surface-glass-soft);
}

.back-btn {
  border-radius: 12px;
  font-size: 16px;
  padding: 8px 10px;
}

.back-btn:hover {
}

.action-buttons .el-button {
  border-radius: 12px;
  font-size: 16px;
}

.action-buttons .el-button:hover {
}

.mail-detail-content {
  flex-grow: 1;
  overflow-y: auto;
  padding: 28px 30px;
}

.mail-subject {
  font-size: 36px;
  font-weight: 600;
  color: var(--pm-text-primary);
  margin: 0 0 22px 0;
  line-height: 1.3;
  letter-spacing: -0.02em;
}

.mail-subject-row {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
}

.mail-subject-row .mail-subject {
  margin-bottom: 22px;
}

.thread-count-pill {
  margin-bottom: 22px;
  padding: 4px 12px;
  border-radius: 999px;
  border: 1px solid var(--pm-border-color);
  background: var(--pm-surface-muted);
  color: var(--pm-text-secondary);
  font-size: 13px;
  white-space: nowrap;
}

.thread-message {
  position: relative;
}

.thread-separator {
  height: 1px;
  margin: 28px 0;
  background: var(--pm-border-color);
}

.thread-message-subject {
  margin-bottom: 10px;
  color: var(--pm-text-secondary);
  font-size: 14px;
  font-weight: 600;
}

.mail-meta-card {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 18px;
  padding: 16px 18px;
  border-radius: var(--pm-radius-lg);
  border: 1px solid var(--pm-border-color);
  background: var(--pm-surface-glass);
}

.meta-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.avatar-placeholder {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--pm-primary-light-3), var(--pm-primary-color));
  color: white;
  display: flex;
  justify-content: center;
  align-items: center;
  font-size: 20px;
  font-weight: 600;
  flex-shrink: 0;
}

.sender-line {
  margin-bottom: 4px;
}

.sender-name {
  font-weight: 600;
  font-size: 16px;
  color: var(--pm-text-primary);
  margin-right: 8px;
}

.sender-email {
  color: var(--pm-text-secondary);
  font-size: 14px;
}

.receivers-line {
  font-size: 13px;
  color: var(--pm-text-secondary);
}

.meta-label {
  color: var(--pm-text-muted);
  margin-right: 4px;
}

.cc-section {
  margin-left: 12px;
}

.meta-right {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  color: var(--pm-text-secondary);
  font-size: 14px;
}

.message-menu-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--pm-text-secondary);
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.message-menu-btn:hover {
  background: var(--pm-bg-hover);
  color: var(--pm-text-primary);
}

.thread-load-more {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 24px 0 8px;
}

.thread-progress {
  color: var(--pm-text-muted);
  font-size: 12px;
}

.thread-load-end {
  padding: 18px 0 4px;
  text-align: center;
  color: var(--pm-text-muted);
  font-size: 12px;
}

.source-view {
  max-height: 65vh;
  margin: 0;
  padding: 16px;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  border: 1px solid var(--pm-border-color);
  border-radius: var(--pm-radius-md);
  background: var(--pm-surface-muted);
  color: var(--pm-text-primary);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.55;
}

.custom-divider {
  margin: 10px 0 26px 0;
}

.mail-body {
  font-size: 16px;
  line-height: 1.7;
  color: var(--pm-text-primary);
  min-height: 200px;
}

.body-text {
  white-space: pre-wrap;
}

.attachments-section {
  margin-top: 32px;
}

.attachments-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 16px;
  color: var(--pm-text-primary);
}

.attachments-list {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
}

.attachment-card {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  border: 1px solid var(--pm-border-color);
  border-radius: 14px;
  background-color: var(--pm-surface-muted);
  min-width: 200px;
  max-width: 300px;
}

.attachment-card:hover {
  border-color: var(--pm-primary-color);
  background-color: var(--pm-bg-hover);
  box-shadow: var(--pm-shadow-sm);
}

.att-icon {
  font-size: 24px;
  color: var(--pm-text-secondary);
  margin-right: 12px;
}

.att-name {
  flex-grow: 1;
  font-size: 14px;
  color: var(--pm-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-right: 12px;
}

.att-download {
  color: var(--pm-primary-color);
  font-size: 18px;
}

@media (max-width: 768px) {
  .mail-detail-header {
    padding: 12px 16px;
  }
  .mail-detail-content {
    padding: 20px 16px;
  }
  .mail-subject {
    font-size: 22px;
  }
  .mail-meta-card {
    flex-direction: column;
    gap: 12px;
  }
  .meta-right {
    padding-left: 64px; /* align with text */
  }
  .avatar-placeholder {
    width: 40px;
    height: 40px;
    font-size: 16px;
  }
}

@media (max-width: 768px) {
  .mail-detail-container {
    padding: 10px 10px calc(10px + env(safe-area-inset-bottom));
    border-radius: var(--md-sys-shape-corner-large);
  }

  .mail-detail-header {
    flex-wrap: wrap;
    gap: 8px;
  }

  .action-buttons {
    width: 100%;
    overflow-x: auto;
    padding-bottom: 4px;
  }

  .action-buttons .el-button {
    height: 36px;
    margin: 0;
    padding: 0 12px;
    border-radius: 999px;
  }

  .mail-subject {
    font-size: 20px;
    line-height: 1.3;
  }

  .mail-meta-card {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
    padding: 14px;
  }

  .mail-meta-card .meta-right {
    width: 100%;
  }

  .mail-body {
    padding: 4px 0 12px;
  }
}
</style>