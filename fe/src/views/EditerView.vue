<template>
  <div class="composer-container">
    <div class="composer-header">
      <h2 class="composer-title">{{ lang.compose }}</h2>
      <div class="composer-actions">
        <el-button v-if="draftStored" class="action-btn" @click="discardDraft">{{ lang.discard_draft }}</el-button>
        <el-button class="action-btn" @click="upload">
          <el-icon><Paperclip /></el-icon> {{ lang.add_att }}
        </el-button>
        <el-button type="primary" class="send-btn" :loading="sending" :disabled="sending" @click="send(ruleFormRef)">
          <el-icon><Position /></el-icon> {{ lang.send }}
        </el-button>
        <input v-show="false" ref="fileRef" type="file" @change="fileChange" multiple>
      </div>
    </div>

    <div class="composer-body">
      <el-form :rules="rules" ref="ruleFormRef" :model="ruleForm" class="composer-form">
        
        <div class="compose-row">
          <div class="field-label">{{ lang.sender }}</div>
          <el-form-item prop="sender" class="flex-grow-item m-0">
            <el-popover trigger="click" :width="400" placement="bottom-start">
              <template #reference>
                <div class="sender-selector">
                  <span class="sender-name">{{ ruleForm.nickName }}</span>
                  <span class="sender-email">&lt;{{ ruleForm.sender }}@{{ ruleForm.pickDomain }}&gt;</span>
                  <el-icon class="arrow-down"><ArrowDown /></el-icon>
                </div>
              </template>
              <template #default>
                <div class="sender-edit-card">
                  <div class="edit-row">
                    <span class="edit-label">{{ lang.sender_prefix }}</span>
                    <el-input 
                      :disabled="!(globalStatus.userInfos.is_admin)"
                      v-model="ruleForm.sender" 
                      :placeholder="lang.sender_desc"
                    />
                  </div>
                  <div class="edit-row">
                    <span class="edit-label">{{ lang.sender_domain }}</span>
                    <el-select v-model="ruleForm.pickDomain" class="w-full">
                      <el-option :value="item" v-for="item in ruleForm.domains" :key="item">{{ item }}</el-option>
                    </el-select>
                  </div>
                  <div class="edit-row">
                    <span class="edit-label">{{ lang.nick_name }}</span>
                    <el-input v-model="ruleForm.nickName"/>
                  </div>
                </div>
              </template>
            </el-popover>
          </el-form-item>
        </div>

        <div class="compose-row">
          <div class="field-label">{{ lang.to }}</div>
          <el-form-item prop="receivers" class="flex-grow-item m-0 border-less-select">
            <el-select 
              v-model="ruleForm.receivers" 
              multiple filterable allow-create :reserve-keyword="false" 
              :placeholder="lang.recipients_ph"
            ></el-select>
          </el-form-item>
        </div>

        <div class="compose-row" v-if="ruleForm.cc.length > 0 || showCcBcc">
          <div class="field-label">{{ lang.cc }}</div>
          <el-form-item prop="cc" class="flex-grow-item m-0 border-less-select">
            <el-select 
              v-model="ruleForm.cc" 
              multiple filterable allow-create :reserve-keyword="false" 
              :placeholder="lang.cc_ph"
            ></el-select>
          </el-form-item>
        </div>

        <div class="compose-row" v-if="ruleForm.bcc.length > 0 || showCcBcc">
          <div class="field-label">{{ lang.bcc }}</div>
          <el-form-item prop="bcc" class="flex-grow-item m-0 border-less-select">
            <el-select 
              v-model="ruleForm.bcc" 
              multiple filterable allow-create :reserve-keyword="false" 
              :placeholder="lang.bcc_ph"
            ></el-select>
          </el-form-item>
        </div>

        <div class="compose-row subject-row">
          <el-form-item prop="subject" class="w-full m-0 border-less-input">
            <el-input v-model="ruleForm.subject" :placeholder="lang.subject_ph"></el-input>
            <div class="cc-bcc-toggle" @click="showCcBcc = !showCcBcc" v-if="!showCcBcc">Cc/Bcc</div>
          </el-form-item>
        </div>

        <div class="editor-wrapper">
          <Toolbar class="editor-toolbar" :editor="editorRef" :defaultConfig="toolbarConfig" :mode="mode"/>
          <Editor class="editor-content" v-model="valueHtml" :defaultConfig="editorConfig" :mode="mode" @onCreated="handleCreated"/>
        </div>

        <div class="attachments-preview" v-if="fileList.length > 0">
          <div class="att-chip" v-for="(item, index) in fileList" :key="index">
            <el-icon class="att-icon"><Document /></el-icon>
            <span class="att-name">{{ item.name }}</span>
            <span class="att-size">{{ formatSize(item.size) }}</span>
            <button type="button" class="att-remove" :aria-label="lang.del_btn" @click="delFile(index)"><el-icon><Close/></el-icon></button>
          </div>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup>
import '@wangeditor/editor/dist/css/style.css'
import {ElMessage} from 'element-plus'
import {computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch} from 'vue'
import {Close, Paperclip, Position, ArrowDown, Document} from '@element-plus/icons-vue';
import lang from '../i18n/i18n';
import {onBeforeRouteLeave, useRoute, useRouter} from 'vue-router'
import {Editor, Toolbar} from '@wangeditor/editor-for-vue'
import {i18nChangeLanguage} from '@wangeditor/editor'
import {http} from "@/utils/axios";
import {confirmAction} from "@/utils/confirm";
import useGroupStore from '../stores/group'
import {useGlobalStatusStore} from "@/stores/useGlobalStatusStore";

const router = useRouter();
const route = useRoute();
const groupStore = useGroupStore()
const globalStatus = useGlobalStatusStore();
const showCcBcc = ref(false);
const sending = ref(false);
const sent = ref(false);
const draftStored = ref(false);
const MAX_FILE_SIZE = 10 * 1024 * 1024;
const MAX_TOTAL_SIZE = 20 * 1024 * 1024;
const DRAFT_PREFIX = 'imail-draft:';

if (lang.lang === "zhCn") {
  i18nChangeLanguage('zh-CN')
} else {
  i18nChangeLanguage('en')
}

// Content HTML
const valueHtml = ref('<p><br></p>')

const toolbarConfig = {}
const editorConfig = {
  MENU_CONF: {},
  placeholder: lang.write_message
}

editorConfig.MENU_CONF['uploadImage'] = {
  base64LimitSize: 10 * 1024 * 1024,
}
const mode = ref()
const fileRef = ref();
const ruleFormRef = ref()
const ruleForm = reactive({
  nickName: '',
  sender: '',
  receivers: [],
  cc: [],
  bcc: [],
  subject: '',
  domains: [],
  pickDomain: ""
})
const fileList = reactive([]);
const draftKey = computed(() => {
  const info = globalStatus.userInfos || {};
  const account = info.account || 'anon';
  const domain = info.domain || (info.domains || [])[0] || '';
  return DRAFT_PREFIX + account + '@' + domain;
});

let hydrated = false
let draftTimer = null

const hasBody = () => Boolean(valueHtml.value && valueHtml.value !== '<p><br></p>')
const hasContent = () => Boolean(
    ruleForm.subject.trim() || ruleForm.receivers.length || ruleForm.cc.length ||
    ruleForm.bcc.length || hasBody() || fileList.length
)

const clearDraft = () => {
  try {
    localStorage.removeItem(draftKey.value)
  } catch {
    // Storage can be unavailable in private modes; a lost draft is not fatal.
  }
  draftStored.value = false
}

const saveDraft = () => {
  if (!draftKey.value) return
  if (!hasContent()) {
    clearDraft()
    return
  }
  try {
    // Attachments are intentionally left out: their base64 payload would blow
    // through the storage quota and the send request re-reads them anyway.
    localStorage.setItem(draftKey.value, JSON.stringify({
      subject: ruleForm.subject,
      receivers: ruleForm.receivers,
      cc: ruleForm.cc,
      bcc: ruleForm.bcc,
      html: valueHtml.value,
      nickName: ruleForm.nickName,
      sender: ruleForm.sender,
      pickDomain: ruleForm.pickDomain,
      savedAt: Date.now(),
    }))
    draftStored.value = true
  } catch {
    draftStored.value = false
  }
}

const restoreDraft = () => {
  if (route.query.to || route.query.cc || route.query.subject || route.query.body) return
  try {
    const raw = localStorage.getItem(draftKey.value)
    if (!raw) return
    const draft = JSON.parse(raw)
    if (!draft) return
    if (draft.subject) ruleForm.subject = draft.subject
    if (Array.isArray(draft.receivers)) ruleForm.receivers = draft.receivers
    if (Array.isArray(draft.cc)) ruleForm.cc = draft.cc
    if (Array.isArray(draft.bcc)) ruleForm.bcc = draft.bcc
    if (draft.html) valueHtml.value = draft.html
    if (draft.sender) ruleForm.sender = draft.sender
    if (draft.pickDomain && ruleForm.domains.includes(draft.pickDomain)) ruleForm.pickDomain = draft.pickDomain
    if (draft.nickName) ruleForm.nickName = draft.nickName
    showCcBcc.value = ruleForm.cc.length > 0 || ruleForm.bcc.length > 0
    draftStored.value = true
    ElMessage.info(lang.draft_restored)
  } catch {
    clearDraft()
  }
}

const discardDraft = () => {
  clearDraft()
  ruleForm.subject = ''
  ruleForm.receivers = []
  ruleForm.cc = []
  ruleForm.bcc = []
  valueHtml.value = '<p><br></p>'
}

const applyPrefill = () => {
  const query = route.query
  if (query.to) {
    ruleForm.receivers = String(query.to).split(',').map(item => item.trim()).filter(Boolean)
  }
  if (query.cc) {
    ruleForm.cc = String(query.cc).split(',').map(item => item.trim()).filter(Boolean)
    showCcBcc.value = ruleForm.cc.length > 0
  }
  if (query.subject) {
    ruleForm.subject = String(query.subject)
  }
  if (query.body) {
    const escaped = String(query.body)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/\n/g, '<br>')
    valueHtml.value = `<p>${escaped}</p>`
  }
}

const finishInit = () => {
  ruleForm.sender = globalStatus.userInfos.account
  ruleForm.domains = globalStatus.userInfos.domains
  ruleForm.pickDomain = globalStatus.userInfos.domains[0]
  ruleForm.nickName = globalStatus.userInfos.name
  applyPrefill()
  restoreDraft()
  hydrated = true
}

const init = function () {
  if (Object.keys(globalStatus.userInfos).length === 0 || globalStatus.userInfos === null || globalStatus.userInfos === undefined) {
    globalStatus.init(finishInit)
  } else {
    finishInit()
  }
}
init()

watch(
    [() => ruleForm.subject, () => ruleForm.receivers, () => ruleForm.cc, () => ruleForm.bcc,
      valueHtml, () => ruleForm.sender, () => ruleForm.pickDomain, () => ruleForm.nickName],
    () => {
      if (!hydrated) return
      clearTimeout(draftTimer)
      draftTimer = setTimeout(saveDraft, 1200)
    },
    {deep: true}
)

const onBeforeUnload = (event) => {
  if (!hasContent() || sent.value) return
  saveDraft()
  event.preventDefault()
  event.returnValue = ''
}

onMounted(() => window.addEventListener('beforeunload', onBeforeUnload))

onBeforeRouteLeave(async () => {
  if (sent.value || !hasContent()) return true
  saveDraft()
  return await confirmAction(lang.unsaved_title, {
    title: lang.unsaved_title,
    confirmText: lang.leave,
    cancelText: lang.stay,
  })
})


const validateSender = function (rule, value, callback) {
  if (typeof ruleForm.sender === "undefined" || ruleForm.sender === null || ruleForm.sender.trim() === "") {
    callback(new Error(lang.err_sender_must))
  } else if (ruleForm.sender.includes("@")) {
    callback(new Error(lang.only_prefix))
  } else {
    callback()
  }
}

const checkEmail = function (str) {
  const re = /.+@.+\..+/;
  return re.test(str);
}

const validateReceivers = function (rule, value, callback) {
  for (let index = 0; index < ruleForm.receivers.length; index++) {
    let element = ruleForm.receivers[index];
    if (!checkEmail(element)) {
      callback(new Error(lang.err_email_format))
      return
    }
  }
  callback()
}

const validateCc = function (rule, value, callback) {
  for (let index = 0; index < ruleForm.cc.length; index++) {
    let element = ruleForm.cc[index];
    if (!checkEmail(element)) {
      callback(new Error(lang.err_email_format))
      return
    }
  }
  callback()
}

const validateBcc = function (rule, value, callback) {
  for (let index = 0; index < ruleForm.bcc.length; index++) {
    let element = ruleForm.bcc[index];
    if (!checkEmail(element)) {
      callback(new Error(lang.err_email_format))
      return
    }
  }
  callback()
}

const rules = reactive({
  sender: [{validator: validateSender, trigger: 'change'}],
  receivers: [{validator: validateReceivers, trigger: 'change'}],
  cc: [{validator: validateCc, trigger: 'change'}],
  bcc: [{validator: validateBcc, trigger: 'change'}],
  subject: [{required: true, message: lang.err_title_must, trigger: 'change'}],
})

const editorRef = shallowRef()
onBeforeUnmount(() => {
  clearTimeout(draftTimer)
  window.removeEventListener('beforeunload', onBeforeUnload)
  const editor = editorRef.value
  if (editor == null) return
  editor.destroy()
})

const handleCreated = (editor) => {
  editorRef.value = editor
}

const send = function (formEl) {
  if (!formEl || sending.value) return
  formEl.validate((valid) => {
    if (!valid) return false
    if (ruleForm.receivers.length === 0 && ruleForm.cc.length === 0 && ruleForm.bcc.length === 0) {
      ElMessage.warning(lang.please_recipient)
      return
    }
    const objectTos = ruleForm.receivers.map(e => ({name: "", email: e}));
    const objectCcs = ruleForm.cc.map(e => ({name: "", email: e}));
    const objectBccs = ruleForm.bcc.map(e => ({name: "", email: e}));

    const text = editorRef.value.getText()

    sending.value = true
    http.post("/api/email/send", {
      from: {name: ruleForm.nickName, email: ruleForm.sender + "@" + ruleForm.pickDomain},
      to: objectTos,
      cc: objectCcs,
      bcc: objectBccs,
      subject: ruleForm.subject,
      text: text,
      html: valueHtml.value,
      attrs: fileList
    }).then(res => {
      sending.value = false
      if (res.errorNo === 0) {
        sent.value = true
        clearTimeout(draftTimer)
        clearDraft()
        ElMessage.success(lang.succ_send)
        groupStore.name = lang.outbox
        groupStore.tag = '{"type":1,"status":-1}'
        router.replace({name: 'list'})
      } else {
        ElMessage.error(res.data || res.errorMsg || lang.send_failed)
      }
    }).catch(() => {
      sending.value = false
    })
  })
}

const formatSize = (bytes) => {
  if (!bytes) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

const totalAttachmentSize = () => fileList.reduce((sum, item) => sum + (item.size || 0), 0)

const upload = function () {
  fileRef.value.dispatchEvent(new MouseEvent('click'))
}

const fileChange = function (e) {
  const files = e.target.files || e.dataTransfer.files;
  if (!files.length) return;
  let total = totalAttachmentSize()
  const limitLabel = formatSize(MAX_FILE_SIZE)
  for (let i = 0; i < files.length; i++) {
    const file = files[i]
    if (file.size > MAX_FILE_SIZE) {
      ElMessage.warning(lang.attachment_too_large.replace('{name}', file.name).replace('{limit}', limitLabel))
      continue
    }
    if (total + file.size > MAX_TOTAL_SIZE) {
      ElMessage.warning(lang.attachment_total_too_large.replace('{limit}', formatSize(MAX_TOTAL_SIZE)))
      break
    }
    total += file.size
    const reader = new FileReader();
    reader.onload = function fileReadCompleted() {
      fileList.push({
        name: file.name,
        data: this.result,
        size: file.size
      })
    };
    reader.readAsDataURL(file);
  }
  // Let the same file be picked again after a rejection.
  e.target.value = ''
}

const delFile = function (index) {
  fileList.splice(index, 1)
}

</script>

<style scoped>
.composer-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--md-sys-color-surface-container-lowest);
  border-radius: var(--md-sys-shape-corner-large);
  border: 1px solid var(--md-sys-color-outline-variant);
  overflow: hidden;
}

.composer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
  background: var(--md-sys-color-surface-container-low);
}

.composer-title {
  font-size: 28px;
  font-weight: 600;
  color: var(--pm-text-primary);
  margin: 0;
  letter-spacing: -0.02em;
}

.composer-actions {
  display: flex;
  gap: 12px;
}

.action-btn {
  background: var(--pm-surface-glass);
  border: 1px solid var(--pm-border-color);
  color: var(--pm-text-secondary);
}

.action-btn:hover {
  background: var(--pm-surface-solid);
  color: var(--pm-text-primary);
  border-color: var(--pm-border-strong);
}

.send-btn {
  box-shadow: 0 8px 16px rgba(0, 113, 227, 0.2);
}

/* Material 3 composer surface */
.composer-container {
  background: var(--md-sys-color-surface-container-low);
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: var(--md-sys-shape-corner-extra-large);
  box-shadow: none;
  backdrop-filter: none;
}

.composer-header {
  padding: 18px 24px;
  background: var(--md-sys-color-surface-container);
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
}

.composer-title {
  font-size: 22px;
  font-weight: 600;
}

.composer-actions .el-button {
  height: 40px;
  border-radius: 999px;
  padding: 0 18px;
}

.send-btn {
  box-shadow: none;
}

.composer-body {
  padding: 8px 24px 20px;
}

.compose-row {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 0;
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
}

.field-label {
  min-width: 72px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 13px;
}

.sender-selector {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border-radius: 999px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}

.composer-form :deep(.el-input__wrapper),
.composer-form :deep(.el-select__wrapper) {
  box-shadow: none !important;
  background: transparent;
}

.editor-content :deep(.w-e-toolbar),
.editor-content :deep(.w-e-text-container) {
  background: var(--md-sys-color-surface-container-lowest) !important;
  border: 1px solid var(--md-sys-color-outline-variant) !important;
  border-radius: var(--md-sys-shape-corner-large);
}

.editor-content :deep(.w-e-text-container) {
  min-height: 320px;
}


.composer-body {
  flex-grow: 1;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
}

.composer-form {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.compose-row {
  display: flex;
  align-items: center;
  border-bottom: 1px solid var(--pm-border-subtle);
  padding: 6px 20px;
}

.field-label {
  width: 70px;
  color: var(--pm-text-secondary);
  font-weight: 600;
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.flex-grow-item {
  flex-grow: 1;
}

.m-0 { margin: 0; }
.w-full { width: 100%; }

.sender-selector {
  display: inline-flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 12px;
  cursor: pointer;
  background-color: var(--pm-surface-muted);
  border: 1px solid var(--pm-border-color);
  transition: background 0.2s;
}

.sender-selector:hover {
  background-color: var(--pm-surface-solid);
}

.sender-name {
  font-weight: 600;
  margin-right: 6px;
  color: var(--pm-text-primary);
}

.sender-email {
  color: var(--pm-text-secondary);
  margin-right: 8px;
}

.arrow-down {
  font-size: 12px;
  color: var(--pm-text-muted);
}

.sender-edit-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.edit-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.edit-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--pm-text-secondary);
}

/* Borderless inputs to make it look like integrated fields */
.border-less-select :deep(.el-input__wrapper),
.border-less-input :deep(.el-input__wrapper) {
  box-shadow: none !important;
  background: transparent;
  padding-left: 0;
}

.border-less-select :deep(.el-select .el-input__wrapper) {
  background: var(--pm-surface-muted) !important;
  box-shadow: 0 0 0 1px var(--pm-border-color) inset !important;
  border-radius: 10px;
  padding: 2px 10px;
}

.border-less-select :deep(.el-select .el-input__wrapper:hover),
.border-less-select :deep(.el-select .el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 1px var(--pm-primary-color) inset !important;
  background: var(--pm-surface-solid) !important;
}

.border-less-select :deep(.el-select__tags .el-tag) {
  background: var(--pm-surface-glass);
  border-color: var(--pm-border-color);
  color: var(--pm-text-primary);
}

.border-less-select :deep(.el-input__inner),
.border-less-input :deep(.el-input__inner) {
  font-size: 15px;
}

.subject-row {
  position: relative;
}

.cc-bcc-toggle {
  position: absolute;
  right: 0;
  top: 50%;
  transform: translateY(-50%);
  font-size: 13px;
  color: var(--pm-primary-color);
  cursor: pointer;
  font-weight: 500;
}

.cc-bcc-toggle:hover {
  text-decoration: underline;
}

.editor-wrapper {
  flex-grow: 1;
  display: flex;
  flex-direction: column;
  min-height: 300px;
  --w-e-toolbar-bg-color: var(--pm-surface-glass-soft);
  --w-e-toolbar-color: var(--pm-text-primary);
  --w-e-toolbar-active-color: var(--pm-primary-color);
  --w-e-toolbar-active-bg-color: var(--pm-row-hover);
  --w-e-textarea-bg-color: transparent;
  --w-e-textarea-color: var(--pm-text-primary);
  --w-e-textarea-slight-bg-color: transparent;
  --w-e-textarea-slight-color: var(--pm-text-muted);
  --w-e-border-color: var(--pm-border-subtle);
}

.editor-toolbar {
  border-bottom: 1px solid var(--pm-border-subtle);
  background-color: var(--pm-surface-glass-soft) !important;
}

.editor-content {
  flex-grow: 1;
  padding: 14px 20px;
  overflow-y: hidden;
}

.editor-content :deep(.w-e-text-container),
.editor-content :deep(.w-e-scroll),
.editor-content :deep(.w-e-text) {
  background: transparent !important;
  color: var(--pm-text-primary) !important;
}

.editor-content :deep(.w-e-text-placeholder) {
  color: var(--pm-text-muted) !important;
}

.attachments-preview {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  padding: 14px 20px;
  border-top: 1px solid var(--pm-border-color);
  background: var(--pm-surface-glass-soft);
}

.att-chip {
  display: flex;
  align-items: center;
  background: var(--pm-bg-secondary);
  border: 1px solid var(--pm-border-color);
  padding: 6px 12px;
  border-radius: 999px;
  font-size: 13px;
  box-shadow: var(--pm-shadow-sm);
}

.att-chip:hover {
  box-shadow: var(--pm-shadow-md);
}

.att-icon {
  margin-right: 6px;
  color: var(--pm-text-secondary);
}

.att-name {
  max-width: 150px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-right: 8px;
}

.att-size {
  margin-right: 8px;
  font-size: 11px;
  color: var(--pm-text-muted);
}

.att-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border: 0;
  background: transparent;
  cursor: pointer;
  color: var(--pm-text-muted);
  border-radius: 999px;
}

.att-remove:focus-visible {
  outline: 2px solid var(--pm-primary-color);
  outline-offset: 2px;
}

.att-remove:hover {
  color: #ef4444;
}

:root[data-theme="dark"] .editor-wrapper {
  background: var(--pm-surface-glass-soft);
}

:root[data-theme="dark"] .editor-content :deep(.w-e-text-container) {
  border-left-color: transparent !important;
  border-right-color: transparent !important;
}

:root[data-theme="dark"] .editor-content :deep(.w-e-bar) {
  background: var(--pm-surface-glass-soft) !important;
  color: var(--pm-text-primary) !important;
}

:root[data-theme="dark"] .editor-content :deep(.w-e-bar-item button),
:root[data-theme="dark"] .editor-content :deep(.w-e-menu-tooltip-v5) {
  color: var(--pm-text-primary) !important;
}

@media (max-width: 768px) {
  .sender-selector {
    padding: 6px;
    max-width: 100%;
    overflow: hidden;
  }
  .sender-email {
    display: none;
  }
  .composer-header {
    padding: 12px 16px;
  }
}
</style>
<style scoped>
/* Material 3 editor toolbar */
.editor-content :deep(.w-e-toolbar) {
  padding: 8px;
  gap: 2px;
  flex-wrap: wrap;
  background: var(--md-sys-color-surface-container) !important;
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
  border-radius: var(--md-sys-shape-corner-large) var(--md-sys-shape-corner-large) 0 0;
}

.editor-content :deep(.w-e-bar-item) {
  padding: 2px;
}

.editor-content :deep(.w-e-bar-item button) {
  width: 32px;
  height: 32px;
  border-radius: var(--md-sys-shape-corner-small);
  color: var(--md-sys-color-on-surface-variant);
}

.editor-content :deep(.w-e-bar-item button:hover) {
  background: var(--md-sys-color-surface-container-highest);
  color: var(--md-sys-color-on-surface);
}

.editor-content :deep(.w-e-bar-divider) {
  background: var(--md-sys-color-outline-variant);
  margin: 4px;
}

.editor-content :deep(.w-e-text-container [contenteditable]) {
  padding: 16px;
  color: var(--md-sys-color-on-surface);
}

@media (max-width: 768px) {
  .composer-container {
    border-radius: var(--md-sys-shape-corner-large);
  }

  .composer-header {
    padding: 12px 14px;
  }

  .composer-title {
    font-size: 18px;
  }

  .composer-actions .el-button {
    height: 36px;
    padding: 0 12px;
  }

  .composer-body {
    padding: 6px 14px 14px;
  }

  .compose-row {
    flex-direction: column;
    align-items: stretch;
    gap: 6px;
    padding: 10px 0;
  }

  .field-label {
    min-width: 0;
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: .04em;
  }

  .editor-content :deep(.w-e-toolbar) {
    flex-wrap: nowrap;
    overflow-x: auto;
  }

  .editor-content :deep(.w-e-text-container) {
    min-height: 260px;
  }
}
</style>
