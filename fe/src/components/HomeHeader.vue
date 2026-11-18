<template>
  <div class="header-container">
    <div class="header-left">
      <div class="mobile-menu-btn" @click="globalStatus.mobileDrawerVisible = true">
        <el-icon><EpMenu /></el-icon>
      </div>
      <div class="mobile-logo">
        <router-link to="/">
          <h1>Mail</h1>
        </router-link>
      </div>
    </div>

    <div class="header-right">
      <button class="m3-icon-button" title="Settings" @click="router.push('/settings')">
        <el-icon><Setting/></el-icon>
      </button>
    </div>
  </div>
</template>

<script setup>
import {EpMenu} from "vue-icons-plus/ep";
import {Setting} from "@element-plus/icons-vue";
import {useRouter} from "vue-router";
import {useGlobalStatusStore} from "@/stores/useGlobalStatusStore";

const router = useRouter();
const globalStatus = useGlobalStatusStore();
</script>

<style scoped>
.header-container {
  display: none;
}

.header-left, .header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-menu-btn {
  display: none;
  width: 36px;
  height: 36px;
  justify-content: center;
  align-items: center;
  font-size: 18px;
  color: var(--pm-text-secondary);
  cursor: pointer;
  border-radius: 12px;
  border: 1px solid var(--pm-border-color);
  background: var(--pm-surface-glass);
  transition: background-color .15s ease, border-color .15s ease;
}

.mobile-menu-btn:hover {
  background-color: var(--pm-surface-solid);
  color: var(--pm-text-primary);
}

.mobile-logo {
  display: none;
  margin-left: 12px;
}

.mobile-logo h1 {
  font-size: 20px;
  font-weight: 600;
  color: var(--pm-primary-color);
  margin: 0;
  letter-spacing: -0.02em;
}

.mobile-logo a {
  text-decoration: none;
}

@media (max-width: 768px) {
  .header-container {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin: 10px 10px 0;
    padding: 12px 14px;
    border-radius: var(--pm-radius-lg);
    border: 1px solid var(--pm-border-color);
    background: var(--pm-surface-glass);
    backdrop-filter: blur(12px);
  }
  .mobile-menu-btn {
    display: flex;
  }
  .mobile-logo {
    display: block;
  }
}
</style>
