import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { login as loginApi } from '@/api/auth'
import type { LoginUser } from '@/types/api'
import { clearAuth, getStoredUser, setStoredUser, setToken } from '@/utils/token'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<LoginUser | null>(getStoredUser<LoginUser>())
  const isAdmin = computed(() => user.value?.role === 'admin')

  async function login(username: string, password: string) {
    const res = await loginApi({ username, password })
    setToken(res.access_token)
    setStoredUser(res.user)
    user.value = res.user
  }

  function logout() {
    clearAuth()
    user.value = null
  }

  return { user, isAdmin, login, logout }
})
