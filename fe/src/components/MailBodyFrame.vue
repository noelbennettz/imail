<template>
  <div class="mail-body">
    <div v-if="blockedCount > 0 && !showImages" class="image-gate">
      <el-icon><Picture/></el-icon>
      <span>{{ lang.images_blocked.replace('{count}', blockedCount) }}</span>
      <button type="button" class="image-gate-action" @click="showImages = true">{{ lang.load_images }}</button>
    </div>
    <iframe
      ref="frame"
      class="mail-body-frame"
      :title="lang.email_body"
      :srcdoc="srcdoc"
      :style="{ height: `${height}px` }"
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      referrerpolicy="no-referrer"
      @load="syncHeight"
    />
  </div>
</template>

<script setup>
import {computed, onBeforeUnmount, ref, watch} from 'vue'
import {Picture} from '@element-plus/icons-vue'
import lang from '../i18n/i18n'

const props = defineProps({
  html: {type: String, default: ''}
})

// Tracking pixels are the reason mail clients ask before loading remote images.
// A broken transparent GIF keeps the layout while the request stays local.
const REMOTE_IMG = /(<img\b[^>]*?\bsrc\s*=\s*["'])(?:https?:)?\/\/[^"']*/gi
const BLANK_IMG = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7'

const frame = ref(null)
const maxHeight = 1200
const height = ref(200)
const showImages = ref(false)
let resizeObserver

const blockedCount = computed(() => (props.html.match(REMOTE_IMG) || []).length)

const srcdoc = computed(() => {
  const html = showImages.value
      ? props.html
      : props.html.replace(REMOTE_IMG, `$1${BLANK_IMG}"`)
  return `<!DOCTYPE html>
<base target="_blank">
<style>
  html, body {
    margin: 0;
    font-size: 16px;
    line-height: 1.7;
    color: #1d1d1f;
    background: transparent;
    overflow-wrap: anywhere;
  }
  img { max-width: 100%; height: auto; }
  a { color: #0071e3; }
  @media (prefers-color-scheme: dark) {
    html, body { color: #f5f7fa; }
  }
</style>
${html}`
})

function disconnect() {
  resizeObserver?.disconnect()
  resizeObserver = null
}

function syncHeight() {
  disconnect()
  const doc = frame.value?.contentDocument
  // A message link can navigate the frame to a different origin.
  if (!doc?.documentElement || !doc.body) return
  const updateHeight = () => {
    if (frame.value?.contentDocument !== doc) return
    // Viewport-relative email CSS can grow with the frame on every resize.
    // Bound auto-sizing and let longer content scroll inside the iframe.
    height.value = Math.min(maxHeight, Math.max(200, doc.documentElement.scrollHeight))
  }
  updateHeight()
  resizeObserver = new ResizeObserver(updateHeight)
  resizeObserver.observe(doc.body)
}

watch(() => props.html, () => {
  showImages.value = false
  disconnect()
  height.value = 200
})

watch(srcdoc, () => {
  disconnect()
  height.value = 200
})

onBeforeUnmount(disconnect)
</script>

<style scoped>
.mail-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.image-gate {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container-high);
  color: var(--md-sys-color-on-surface-variant);
  font-size: 13px;
}

.image-gate-action {
  margin-left: auto;
  white-space: nowrap;
  flex-shrink: 0;
  padding: 6px 14px;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 999px;
  background: transparent;
  color: var(--md-sys-color-primary);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.image-gate-action:hover {
  background: var(--md-sys-color-surface-container-highest);
}

.mail-body-frame {
  display: block;
  width: 100%;
  min-height: 200px;
  border: 0;
  background: transparent;
}
</style>
