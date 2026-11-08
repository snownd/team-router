<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/useAuthStore'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const collapsed = ref(false)

const selectedKey = computed(() => route.path)
const pageTitle = computed(() => route.meta.title ?? '')
const groupTitle = computed(() => (route.path.startsWith('/admin') ? '管理' : '工作台'))
const userName = computed(() => auth.user?.display_name || auth.user?.username || '')

function onMenuClick(key: string | number | undefined) {
  router.push(String(key))
}

function onLogout() {
  auth.logout()
  router.replace('/login')
}
</script>

<template>
  <a-layout class="layout">
    <a-layout-sider
      class="sider"
      theme="dark"
      :width="220"
      collapsible
      breakpoint="lg"
      hide-trigger
      v-model:collapsed="collapsed"
    >
      <div class="logo">
        <span class="logo-mark"><icon-thunderbolt /></span>
        <span class="logo-name">Team Router</span>
      </div>
      <a-menu
        :selected-keys="[selectedKey]"
        :collapsed="collapsed"
        auto-scroll-into-view
        @menu-item-click="onMenuClick"
      >
        <a-menu-item key="/">
          <template #icon><icon-dashboard /></template>
          概览
        </a-menu-item>
        <a-menu-item key="/my/keys">
          <template #icon><icon-lock /></template>
          我的 API Key
        </a-menu-item>
        <a-menu-item key="/my/logs">
          <template #icon><icon-list /></template>
          调用日志
        </a-menu-item>
        <a-menu-item key="/my/usage">
          <template #icon><icon-bar-chart /></template>
          用量统计
        </a-menu-item>
        <template v-if="auth.isAdmin">
          <a-menu-item-group title="管理">
            <a-menu-item key="/admin/users">
              <template #icon><icon-user /></template>
              用户管理
            </a-menu-item>
            <a-menu-item key="/admin/departments">
              <template #icon><icon-apps /></template>
              部门管理
            </a-menu-item>
            <a-menu-item key="/admin/keys">
              <template #icon><icon-safe /></template>
              Key 管理
            </a-menu-item>
            <a-menu-item key="/admin/providers">
              <template #icon><icon-cloud /></template>
              渠道管理
            </a-menu-item>
            <a-menu-item key="/admin/prices">
              <template #icon><icon-tag /></template>
              价目管理
            </a-menu-item>
            <a-menu-item key="/admin/logs">
              <template #icon><icon-search /></template>
              日志查询
            </a-menu-item>
          </a-menu-item-group>
        </template>
      </a-menu>
    </a-layout-sider>
    <a-layout class="main">
      <a-layout-header class="header">
        <a-button type="text" class="collapse-btn" @click="collapsed = !collapsed">
          <template #icon>
            <icon-menu-unfold v-if="collapsed" />
            <icon-menu-fold v-else />
          </template>
        </a-button>
        <a-space size="medium">
          <a-tag v-if="auth.isAdmin" color="cyan" size="small">管理员</a-tag>
          <a-dropdown trigger="hover">
            <a-button type="text" class="user-btn">
              <a-avatar :size="26" class="user-avatar"><icon-user /></a-avatar>
              <span class="user-name">{{ userName }}</span>
              <icon-caret-down class="user-caret" />
            </a-button>
            <template #content>
              <a-doption @click="onLogout">
                <template #icon><icon-export /></template>
                退出登录
              </a-doption>
            </template>
          </a-dropdown>
        </a-space>
      </a-layout-header>
      <a-layout-content class="content">
        <div class="page-head">
          <a-breadcrumb class="breadcrumb">
            <a-breadcrumb-item>{{ groupTitle }}</a-breadcrumb-item>
            <a-breadcrumb-item>{{ pageTitle }}</a-breadcrumb-item>
          </a-breadcrumb>
          <h1 class="page-title">{{ pageTitle }}</h1>
        </div>
        <router-view />
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<style scoped>
.layout {
  min-height: 100vh;
}

.sider {
  display: flex;
  flex-direction: column;
}

.sider :deep(.arco-layout-sider-children) {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sider :deep(.arco-menu) {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}

.logo {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 60px;
  padding: 0 16px;
  flex-shrink: 0;
  border-bottom: 1px solid rgb(var(--sider-line) / 0.08);
}

.logo-mark {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  border-radius: 8px;
  font-size: 16px;
  background: rgb(var(--primary-6));
  color: var(--color-white);
}

.logo-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-white);
  white-space: nowrap;
}

:global(.arco-layout-sider-collapsed) .logo {
  padding: 0;
  justify-content: center;
}

:global(.arco-layout-sider-collapsed) .logo-name {
  display: none;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 60px;
  padding: 0 20px 0 8px;
  background: var(--color-bg-2);
  border-bottom: 1px solid var(--color-border);
}

.user-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 8px;
  color: var(--color-text-1);
}

.user-avatar {
  background: rgb(var(--primary-6));
  color: var(--color-white);
}

.user-name {
  font-size: 14px;
}

.user-caret {
  font-size: 12px;
  color: var(--color-text-3);
}

.content {
  padding: 16px 20px 24px;
  min-height: calc(100vh - 60px);
  background: var(--color-fill-2);
}

.breadcrumb {
  font-size: 12px;
}

.page-title {
  margin: 4px 0 0;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
  color: var(--color-text-1);
  overflow-wrap: break-word;
  min-width: 0;
}
</style>
