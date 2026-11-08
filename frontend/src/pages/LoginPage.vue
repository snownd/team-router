<script setup lang="ts">
import { Message } from '@arco-design/web-vue'
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/useAuthStore'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const form = reactive({ username: '', password: '' })
const loading = ref(false)

async function submit() {
  if (!form.username || !form.password) {
    Message.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    await auth.login(form.username, form.password)
    const raw = route.query.redirect
    const redirect = typeof raw === 'string' && raw.startsWith('/') ? raw : '/'
    router.replace(redirect)
  } catch {
    // 错误提示已由 http 拦截器统一展示
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="brand-pane">
      <div class="brand">
        <span class="brand-mark"><icon-thunderbolt /></span>
        <div class="brand-name">Team Router</div>
        <div class="brand-tagline">团队 AI Token 管理平台</div>
      </div>
    </div>
    <div class="form-pane">
      <div class="form-box">
        <h1 class="form-title">登录 Team Router</h1>
        <p class="form-subtitle">团队 AI Token 管理平台</p>
        <a-form :model="form" layout="vertical">
          <a-form-item label="用户名">
            <a-input v-model="form.username" placeholder="用户名" size="large" @keyup.enter="submit" />
          </a-form-item>
          <a-form-item label="密码">
            <a-input-password
              v-model="form.password"
              placeholder="密码"
              size="large"
              @keyup.enter="submit"
            />
          </a-form-item>
          <a-button type="primary" long size="large" :loading="loading" @click="submit">
            登录
          </a-button>
        </a-form>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  display: grid;
  grid-template-columns: minmax(0, 5fr) minmax(0, 7fr);
  min-height: 100vh;
}

.brand-pane {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px;
  background: rgb(var(--primary-9));
}

.brand {
  max-width: 320px;
}

.brand-mark {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 16px;
  font-size: 28px;
  background: rgb(var(--primary-6));
  color: var(--color-white);
}

.brand-name {
  margin-top: 24px;
  font-size: 28px;
  font-weight: 600;
  color: var(--color-white);
}

.brand-tagline {
  margin-top: 8px;
  font-size: 15px;
  color: rgb(var(--primary-2));
}

.form-pane {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--color-bg-2);
}

.form-box {
  width: 100%;
  max-width: 360px;
}

.form-title {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
  color: var(--color-text-1);
}

.form-subtitle {
  margin: 8px 0 24px;
  font-size: 14px;
  color: var(--color-text-3);
}

@media (max-width: 900px) {
  .login-page {
    grid-template-columns: minmax(0, 1fr);
  }

  .brand-pane {
    display: none;
  }
}
</style>
