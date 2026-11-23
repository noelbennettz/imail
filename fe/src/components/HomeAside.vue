<template>
  <div class="sidebar-main">
    <div class="brand">
      <router-link to="/" class="brand-link">
        <span class="brand-text">Mail</span>
      </router-link>
    </div>

    <div class="search-box">
      <el-input
        v-model="searchQuery"
        :placeholder="lang.search"
        prefix-icon="Search"
        clearable
        class="custom-search"
        @keyup.enter="doSearch"
        @clear="doSearch"
      />
    </div>

    <nav class="menu-container" :aria-label="lang.menu">
      <button
          v-for="item in menuItems"
          :key="item.tag"
          type="button"
          class="menu-item"
          :class="{active: isActive(item), child: item.depth > 0}"
          :aria-current="isActive(item) ? 'page' : undefined"
          :style="{paddingLeft: `${10 + item.depth * 16}px`}"
          @click="selectGroup(item)"
      >
        <el-icon class="menu-icon"><component :is="item.icon"/></el-icon>
        <span class="menu-label">{{ item.label }}</span>
        <span v-if="item.unread > 0" class="menu-badge">{{ item.unread > 99 ? '99+' : item.unread }}</span>
      </button>
    </nav>

    <div class="sidebar-footer" v-if="isLogin">
      <AccountMenu/>
      <a class="source-offer" :href="sourceUrl" target="_blank" rel="noopener">
        {{ lang.source_code }}
      </a>
    </div>
  </div>
</template>

<script setup>
import {computed, onMounted, ref, watch} from "vue";
import {useRoute, useRouter} from "vue-router";
import {Delete, Document, Folder, Message, Promotion, Warning} from "@element-plus/icons-vue";
import useGroupStore from "../stores/group";
import lang from "../i18n/i18n";
import {useGlobalStatusStore} from "../stores/useGlobalStatusStore";
import AccountMenu from "./AccountMenu.vue";

const groupStore = useGroupStore();
const globalStatus = useGlobalStatusStore();
const isLogin = computed(() => globalStatus.isLogin);
const router = useRouter();
const route = useRoute();
// AGPL-3.0 section 13: the same source offer as on the login page.
const sourceUrl = 'https://github.com/noelbennettz/imail';
const searchQuery = ref(String(route.query.q || ''));

const parseTag = (tag) => {
  try {
    return JSON.parse(tag)
  } catch {
    return {}
  }
}

// Folder icons follow the tag rather than the label, so a renamed folder or a
// different interface language keeps the right glyph.
const iconFor = (tag) => {
  const parsed = parseTag(tag)
  if (parsed.group_id > 0) return Folder
  if (parsed.type === 1) return Promotion
  if (parsed.status === 4) return Document
  if (parsed.status === 5) return Warning
  if (parsed.status === 3) return Delete
  return Message
}

const menuItems = computed(() => {
  const flat = []
  const walk = (items, depth) => {
    for (const item of items) {
      flat.push({...item, depth, icon: iconFor(item.tag)})
      if (item.children?.length) walk(item.children, depth + 1)
    }
  }
  walk(groupStore.items, 0)
  return flat
})

const isEmptyTag = (tag) => !tag
const inboxTag = computed(() => menuItems.value.find(item => {
  const parsed = parseTag(item.tag)
  return parsed.group_id === 0 && parsed.type === 0 && parsed.status === -1
})?.tag || '')

const isActive = (item) => {
  if (isEmptyTag(groupStore.tag)) return item.tag === inboxTag.value
  return item.tag === groupStore.tag
}

const selectGroup = (item) => {
  groupStore.name = item.label
  groupStore.tag = item.tag
  globalStatus.mobileDrawerVisible = false
  router.push({name: "list"})
}

const doSearch = function () {
  const query = searchQuery.value.trim();
  router.push({name: 'list', query: query ? {q: query} : {}});
  globalStatus.mobileDrawerVisible = false;
}

watch(() => route.query.q, (value) => {
  searchQuery.value = String(value || '')
})

onMounted(() => {
  groupStore.loadItems()
})
</script>

<style scoped>
.sidebar-main {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--md-sys-color-surface-container-low);
}

.brand {
  padding: 18px 20px 8px;
}

.brand-link {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.brand-text {
  font-size: 19px;
  font-weight: 600;
  color: var(--pm-text-primary);
  letter-spacing: -0.02em;
}

.search-box {
  padding: 12px 18px 16px;
}

.custom-search :deep(.el-input__wrapper) {
  border-radius: 999px;
  background-color: var(--md-sys-color-surface-container-lowest);
}

.custom-search :deep(.el-input__wrapper:hover) {
  box-shadow: 0 0 0 1px var(--pm-border-color) inset;
}

.custom-search :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 1px var(--pm-primary-color) inset;
}

.menu-container {
  flex-grow: 1;
  overflow-y: auto;
  padding: 0 10px;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  height: 44px;
  margin-bottom: 2px;
  padding-right: 12px;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--pm-text-secondary);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  text-align: left;
}

.menu-item:hover {
  background: var(--md-sys-color-surface-container-high);
  color: var(--pm-text-primary);
}

.menu-item:focus-visible {
  outline: 3px solid var(--pm-primary-color);
  outline-offset: -1px;
}

.menu-item.active {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
  font-weight: 600;
}

.menu-icon {
  font-size: 18px;
}

.menu-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.menu-badge {
  min-width: 20px;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--md-sys-color-primary);
  color: var(--md-sys-color-on-primary);
  font-size: 11px;
  font-weight: 700;
  line-height: 20px;
  text-align: center;
}

.sidebar-footer {
  padding: 12px 12px calc(16px + env(safe-area-inset-bottom));
  border-top: 1px solid var(--pm-border-color);
  margin-top: auto;
}

.source-offer {
  display: block;
  margin-top: 8px;
  padding-left: 8px;
  font-size: 11px;
  color: var(--pm-text-secondary);
  text-decoration: none;
}

.source-offer:hover {
  text-decoration: underline;
}
</style>
