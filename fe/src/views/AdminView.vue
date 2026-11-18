<template>
  <div class="admin-shell">
    <header class="admin-app-bar">
      <div class="admin-brand">
        <button class="m3-icon-button" @click="router.push('/')" title="Back to mailbox">
          <el-icon><Back/></el-icon>
        </button>
        <div>
          <h1>{{ lang.settings_title }}</h1>
        </div>
      </div>
      <div class="admin-actions">
        <el-select :model-value="language" @change="setLanguage" size="small" style="width: 140px">
          <el-option v-for="item in languages" :key="item.code" :label="item.label" :value="item.code"/>
        </el-select>
        <ThemeToggle/>
      </div>
    </header>

    <div class="admin-body">
      <nav class="admin-nav">
        <div class="nav-group-title">{{ lang.settings_group }}</div>
        <button
            v-for="tab in settingsTabs"
            :key="tab.name"
            class="admin-nav-item"
            :class="{active: activeTab === tab.name}"
            @click="activeTab = tab.name"
        >
          <el-icon><component :is="tab.icon"/></el-icon>
          <span>{{ tab.label }}</span>
        </button>

        <template v-if="userInfos.is_admin">
          <div class="nav-group-title">{{ lang.administration }}</div>
          <button
              v-for="tab in adminTabs"
              :key="tab.name"
              class="admin-nav-item"
              :class="{active: activeTab === tab.name}"
              @click="activeTab = tab.name"
          >
            <el-icon><component :is="tab.icon"/></el-icon>
            <span>{{ tab.label }}</span>
          </button>
        </template>
      </nav>

      <main class="admin-content">
        <section v-if="activeTab === 'domains'" class="admin-panel">
          <DomainManagement/>
        </section>
        <section v-if="activeTab === 'users'" class="admin-panel">
          <UserManagement/>
        </section>
        <section v-if="activeTab === 'security'" class="admin-panel">
          <SecuritySettings/>
        </section>
        <section v-if="activeTab === 'folders'" class="admin-panel">
          <GroupSettings/>
        </section>
        <section v-if="activeTab === 'rules'" class="admin-panel">
          <RuleSettings/>
        </section>
      </main>
    </div>

  </div>
</template>

<script setup>
import {computed, onMounted, ref} from 'vue'
import {useRouter} from 'vue-router'
import {Back, Folder, Lock, OfficeBuilding, Setting, User} from '@element-plus/icons-vue'
import UserManagement from '@/components/UserManagement.vue'
import DomainManagement from '@/components/DomainManagement.vue'
import SecuritySettings from '@/components/SecuritySettings.vue'
import GroupSettings from '@/components/GroupSettings.vue'
import RuleSettings from '@/components/RuleSettings.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'
import {useGlobalStatusStore} from '@/stores/useGlobalStatusStore'
import lang, {language, languages, setLanguage} from '@/i18n/i18n'

const router = useRouter()
const globalStatus = useGlobalStatusStore()
const userInfos = globalStatus.userInfos
const activeTab = ref('users')

const tabs = [
  {name: 'security', label: lang.admin_password, icon: Lock, group: 'settings'},
  {name: 'folders', label: lang.admin_folders, icon: Folder, group: 'settings'},
  {name: 'rules', label: lang.admin_rules, icon: Setting, group: 'settings'},
  {name: 'users', label: lang.admin_users, icon: User, group: 'administration'},
  {name: 'domains', label: lang.admin_domains, icon: OfficeBuilding, group: 'administration'},
]

const settingsTabs = computed(() => tabs.filter(tab => tab.group === 'settings'))
const adminTabs = computed(() => tabs.filter(tab => tab.group === 'administration'))

onMounted(() => {
  if (Object.keys(userInfos).length === 0) {
    globalStatus.init(() => {
      if (!globalStatus.isLogin) {
        router.push('/login')
      } else if (!userInfos.is_admin) {
        activeTab.value = 'security'
      }
    })
  } else if (!userInfos.is_admin) {
    activeTab.value = 'security'
  }
})
</script>

<style scoped>
.admin-shell {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--md-sys-color-background);
}

.admin-app-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 20px;
  background: var(--md-sys-color-surface-container);
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
}

.admin-brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.admin-brand h1 {
  margin: 0;
  font-size: 20px;
  color: var(--md-sys-color-on-surface);
}

.admin-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.admin-actions :deep(.el-select__wrapper) {
  min-height: 40px;
  height: 40px;
  border-radius: 999px;
  padding: 0 12px;
}


.admin-body {
  flex: 1;
  display: flex;
  min-height: 0;
}

.admin-nav {
  width: 220px;
  padding: 16px 10px;
  border-right: 1px solid var(--md-sys-color-outline-variant);
  background: var(--md-sys-color-surface-container-low);
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nav-group-title {
  padding: 12px 16px 4px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: .08em;
  text-transform: uppercase;
  color: var(--md-sys-color-on-surface-variant);
}

.nav-group-title + .nav-group-title {
  margin-top: 8px;
}

.admin-nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  padding: 12px 16px;
  border: 0;
  border-radius: var(--md-sys-shape-corner-full);
  background: transparent;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
  cursor: pointer;
  text-align: left;
}

.admin-nav-item:hover {
  background: var(--md-sys-color-surface-container-high);
}

.admin-nav-item.active {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
  font-weight: 600;
}

.admin-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
}

.admin-panel {
  max-width: 1080px;
  margin: 0 auto;
}

@media (max-width: 768px) {
  .admin-app-bar {
    padding: 10px 12px;
  }

  .admin-brand h1 {
    font-size: 17px;
  }

  .admin-body {
    flex-direction: column;
  }

  .admin-nav {
    width: 100%;
    flex-direction: row;
    overflow-x: auto;
    padding: 8px;
    border-right: 0;
    border-bottom: 1px solid var(--md-sys-color-outline-variant);
  }

  .admin-nav-item {
    width: auto;
    white-space: nowrap;
    padding: 10px 14px;
  }

  .admin-content {
    padding: 12px;
  }
}

@media (max-width: 768px) {
  .admin-app-bar {
    flex-wrap: wrap;
    gap: 8px;
  }

  .admin-actions {
    gap: 6px;
  }

  .admin-actions :deep(.el-select) {
    width: 110px !important;
  }

  .admin-content {
    padding: 10px 10px calc(10px + env(safe-area-inset-bottom));
  }

  .admin-panel {
    max-width: 100%;
  }
}
</style>
