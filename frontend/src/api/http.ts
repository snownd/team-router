import { Message } from '@arco-design/web-vue'
import axios from 'axios'

import { clearAuth, getToken } from '@/utils/token'

export const http = axios.create({ baseURL: '/api', timeout: 30_000 })

http.interceptors.request.use((config) => {
  const token = getToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  config.headers['X-Request-Id'] = crypto.randomUUID()
  return config
})

http.interceptors.response.use(
  (response) => response.data,
  (error) => {
    const message = error.response?.data?.error?.message ?? error.message ?? '请求失败'
    if (error.response?.status === 401) {
      clearAuth()
      const redirect = encodeURIComponent(`${window.location.pathname}${window.location.search}`)
      window.location.href = `/login?redirect=${redirect}`
    } else {
      Message.error(message)
    }
    return Promise.reject(new Error(message))
  },
)

// 后端响应体即业务数据（拦截器已解包），第二泛型直接取 T
export const api = {
  get: <T>(url: string, params?: object) => http.get<T, T>(url, { params }),
  post: <T>(url: string, data?: object) => http.post<T, T>(url, data),
  put: <T>(url: string, data?: object) => http.put<T, T>(url, data),
  delete: <T>(url: string) => http.delete<T, T>(url),
}
