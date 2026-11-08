import { api } from './http'
import type {
  ApiKey,
  CreateKeyPayload,
  CreatedKey,
  LogQuery,
  Me,
  Paged,
  PageParams,
  RequestLog,
  UsageStat,
} from '@/types/api'

export const getMe = () => api.get<Me>('/user/me')

export const getMyKeys = (p: PageParams) => api.get<Paged<ApiKey>>('/user/keys', p)

export const createMyKey = (p: CreateKeyPayload) => api.post<CreatedKey>('/user/keys', p)

export const deleteMyKey = (id: number) => api.delete<{ message: string }>(`/user/keys/${id}`)

export const getMyLogs = (p: LogQuery & PageParams) => api.get<Paged<RequestLog>>('/user/logs', p)

export const getMyStats = (p: { start?: string; end?: string }) =>
  api.get<Paged<UsageStat>>('/user/stats', p)
