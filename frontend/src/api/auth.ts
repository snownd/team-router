import { api } from './http'
import type { LoginPayload, LoginResult } from '@/types/api'

export const login = (payload: LoginPayload) => api.post<LoginResult>('/auth/login', payload)
