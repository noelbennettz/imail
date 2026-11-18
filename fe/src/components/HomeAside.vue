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
    
    <div class="menu-container">
      <el-menu
        :default-active="activeGroup"
        class="sidebar-menu"
        @select="handleMenuSelect"
      >
        <el-menu-item v-for="item in data" :key="item.tag" :index="item.tag">
          <span class="menu-label">{{ item.label }}</span>
        </el-menu-item>
      </el-menu>
    </div>
    <div class="sidebar-footer" v-if="isLogin">
      <div class="footer-actions">
        <ThemeToggle/>
        <button class="m3-icon-button" title="Settings" @click="router.push('/settings')">
          <el-icon><Setting/></el-icon>
        </button>
      </div>
      <a class="source-offer" :href="sourceUrl" target="_blank" rel="noopener">
        {{ lang.source_code }}
      </a>
    </div>
  </div>
</template>

<script setup>
import { useRouter } from "vue-router";
import { Setting } from "@element-plus/icons-vue";
import { ref, watch, computed } from "vue";
import useGroupStore from "../stores/group";
import lang from "../i18n/i18n";
import { http } from "@/utils/axios";
import { useGlobalStatusStore } from "../stores/useGlobalStatusStore";
import ThemeToggle from "./ThemeToggle.vue";

const groupStore = useGroupStore();
const globalStatus = useGlobalStatusStore();
const isLogin = computed(() => globalStatus.isLogin);
const router = useRouter();
// AGPL-3.0 section 13: the same source offer as on the login page.
const sourceUrl = 'https://github.com/noelbennettz/imail';
const data = ref([]);
const searchQuery = ref("");
const activeGroup = ref(groupStore.tag);

// Keep active menu synced with store
watch(() => groupStore.tag, (newVal) => {
  activeGroup.value = newVal;
});

http.get("/api/group").then((res) => {
  if (res.data) {
    // Attempting to flatten the tree for a simpler premium menu or keep it, el-tree is less beautiful.
    // For now we assume res.data is an array of items. If it's a tree, we flatten it.
    let list = [];
    const traverse = (items) => {
      items.forEach(node => {
        list.push(node);
        if(node.children) traverse(node.children);
      });
    }
    traverse(res.data);
    data.value = list;
  }
});

const handleMenuSelect = function (index) {
  const selected = data.value.find(d => d.tag === index);
  if (selected) {
    groupStore.name = selected.label;
    groupStore.tag = selected.tag;
    router.push({ name: "list" });
  }
};

const doSearch = function () {
  const query = searchQuery.value.trim();
  router.push({name: 'list', query: query ? {q: query} : {}});
  globalStatus.mobileDrawerVisible = false;
};

</script>

<style scoped>
.sidebar-main {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: linear-gradient(180deg, var(--pm-sidebar-grad-start) 0%, var(--pm-sidebar-grad-end) 100%);
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
  background-color: var(--pm-surface-solid);
}

.custom-search :deep(.el-input__wrapper:hover) {
  background-color: var(--pm-bg-secondary);
  box-shadow: 0 0 0 1px var(--pm-border-color) inset;
}

.custom-search :deep(.el-input__wrapper.is-focus) {
  background-color: var(--pm-bg-secondary);
  box-shadow: 0 0 0 1px var(--pm-primary-color) inset;
}

.menu-container {
  flex-grow: 1;
  overflow-y: auto;
  padding: 0 10px;
}

.sidebar-menu {
  border-right: none;
  background: transparent;
}

.sidebar-menu :deep(.el-menu-item) {
  height: 42px;
  line-height: 42px;
  padding-left: 10px !important;
  border-radius: 999px;
  margin-bottom: 4px;
  color: var(--pm-text-secondary);
  font-weight: 500;
}

.sidebar-menu :deep(.el-menu-item:hover) {
  background-color: var(--pm-bg-hover);
  color: var(--pm-text-primary);
}

.sidebar-menu :deep(.el-menu-item.is-active) {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
  font-weight: 600;
}

.menu-label {
  font-size: 14px;
}

.sidebar-footer {
  padding: 14px 16px 18px;
  border-top: 1px solid var(--pm-border-color);
  margin-top: auto;
}

.settings-btn {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  font-size: 14px;
  font-weight: 500;
  color: var(--pm-text-secondary);
  border-radius: 14px;
  background: var(--pm-surface-glass);
  border: 1px solid var(--pm-border-color);
  cursor: pointer;
  transition: background-color .15s ease, border-color .15s ease;
  flex: 1;
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.source-offer {
  display: block;
  margin-top: 10px;
  font-size: 11px;
  color: var(--pm-text-secondary);
  text-decoration: none;
}

.source-offer:hover {
  text-decoration: underline;
}

.settings-btn .el-icon {
  font-size: 18px;
}

.settings-btn:hover {
  background-color: var(--pm-bg-hover);
  color: var(--pm-text-primary);
}
</style>