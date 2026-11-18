<script setup>
import {RouterView, useRoute} from 'vue-router'
import HomeHeader from '@/components/HomeHeader.vue'
import HomeAside from '@/components/HomeAside.vue';
import ThemeToggle from '@/components/ThemeToggle.vue'
import {ref, watch, onMounted} from 'vue'
import {useGlobalStatusStore} from "@/stores/useGlobalStatusStore";
import {language, languages, setLanguage} from '@/i18n/i18n'

const route = useRoute()
const pageName = ref(route.name)
const globalStatus = useGlobalStatusStore();

onMounted(() => {
  globalStatus.init(() => {});
});

watch(
    () => route.fullPath,
    () => {
      pageName.value = route.name
    }
)

</script>

<template>
  <div id="main">
    <HomeHeader v-if="pageName !== 'login' && pageName !== 'setup' && pageName !== 'settings'"/>
    <div id="content">
      <div id="aside" v-if="pageName !== 'login' && pageName !== 'setup' && pageName !== 'settings'">
        <HomeAside/>
      </div>
      <el-drawer
          v-if="pageName !== 'login' && pageName !== 'setup' && pageName !== 'settings'"
          v-model="globalStatus.mobileDrawerVisible"
          direction="ltr"
          size="243px"
          :with-header="false"
          class="mobile-aside-drawer"
      >
        <HomeAside/>
      </el-drawer>
      <div id="body" :class="{ 'full-bleed': pageName === 'login' || pageName === 'setup' || pageName === 'settings' }">
        <div v-if="pageName === 'login' || pageName === 'setup'" class="auth-theme-toggle">
          <el-select :model-value="language" @change="setLanguage" size="small" style="width: 130px">
            <el-option v-for="item in languages" :key="item.code" :label="item.label" :value="item.code"/>
          </el-select>
          <ThemeToggle/>
        </div>
        <RouterView/>
      </div>
    </div>
  </div>
</template>


<style scoped>
#aside {
  width: 264px;
  min-width: 264px;
  max-width: 264px;
  margin: 16px 0 16px 16px;
  background: var(--pm-bg-sidebar);
  border: 1px solid var(--pm-border-color);
  border-radius: var(--pm-radius-xl);
  box-shadow: var(--pm-shadow-sm);
  overflow: hidden;
}

#body {
  width: 100%;
  height: 100%;
  padding: 16px;
  box-sizing: border-box;
  overflow: hidden;
}

#body.full-bleed {
  padding: 0;
}

#content {
  display: flex;
  flex-grow: 1;
  overflow: hidden;
  min-height: 0;
}

#main {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.mobile-aside-drawer :deep(.el-drawer__body) {
  padding: 0;
}

.auth-theme-toggle {
  position: fixed;
  top: 16px;
  right: 16px;
  z-index: 2100;
  display: flex;
  align-items: center;
  gap: 8px;
}

.auth-theme-toggle :deep(.el-select) {
  width: 140px;
  height: 40px;
}

.auth-theme-toggle :deep(.el-select__wrapper) {
  min-height: 40px !important;
  height: 40px !important;
  border-radius: 999px;
  padding: 0 12px;
}


@media (max-width: 768px) {
  #aside {
    display: none !important;
  }
  #body {
    padding: 10px;
  }
  #body.full-bleed {
    padding: 0;
  }
  .auth-theme-toggle {
    top: 10px;
    right: 10px;
  }
}
</style>
