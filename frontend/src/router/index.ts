import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

import { getToken, getStoredUser } from '@/utils/token'

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    requiresAdmin?: boolean
  }
}

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/pages/LoginPage.vue'),
    meta: { title: '登录' },
  },
  {
    path: '/',
    component: () => import('@/layouts/AdminLayout.vue'),
    children: [
      {
        path: '',
        name: 'dashboard',
        component: () => import('@/pages/DashboardPage.vue'),
        meta: { title: '概览' },
      },
      {
        path: 'my/keys',
        name: 'my-keys',
        component: () => import('@/pages/my/MyKeysPage.vue'),
        meta: { title: '我的 API Key' },
      },
      {
        path: 'my/logs',
        name: 'my-logs',
        component: () => import('@/pages/my/MyLogsPage.vue'),
        meta: { title: '调用日志' },
      },
      {
        path: 'my/usage',
        name: 'my-usage',
        component: () => import('@/pages/my/MyUsagePage.vue'),
        meta: { title: '用量统计' },
      },
      {
        path: 'admin/users',
        name: 'admin-users',
        component: () => import('@/pages/admin/UsersPage.vue'),
        meta: { title: '用户管理', requiresAdmin: true },
      },
      {
        path: 'admin/departments',
        name: 'admin-departments',
        component: () => import('@/pages/admin/DepartmentsPage.vue'),
        meta: { title: '部门管理', requiresAdmin: true },
      },
      {
        path: 'admin/keys',
        name: 'admin-keys',
        component: () => import('@/pages/admin/KeysPage.vue'),
        meta: { title: 'Key 管理', requiresAdmin: true },
      },
      {
        path: 'admin/providers',
        name: 'admin-providers',
        component: () => import('@/pages/admin/ProvidersPage.vue'),
        meta: { title: '渠道管理', requiresAdmin: true },
      },
      {
        path: 'admin/prices',
        name: 'admin-prices',
        component: () => import('@/pages/admin/PricesPage.vue'),
        meta: { title: '价目管理', requiresAdmin: true },
      },
      {
        path: 'admin/logs',
        name: 'admin-logs',
        component: () => import('@/pages/admin/LogsPage.vue'),
        meta: { title: '日志查询', requiresAdmin: true },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  if (to.path === '/login') return true
  if (!getToken()) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.requiresAdmin && getStoredUser<{ role: string }>()?.role !== 'admin') {
    return { name: 'dashboard' }
  }
  return true
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} · Team Router` : 'Team Router 管理台'
})

export default router
