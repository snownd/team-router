import { api } from './http'
import type {
  AdminUser,
  ApiKey,
  CreateDeptPayload,
  CreateKeyPayload,
  CreateUserPayload,
  CreatedKey,
  DeptNode,
  LogQuery,
  ModelPrice,
  Paged,
  PageParams,
  PricePayload,
  Provider,
  ProviderPayload,
  RequestLog,
  TtftStat,
  UpdateDeptPayload,
  UpdateKeyPayload,
  UpdateUserPayload,
  UsageStat,
} from '@/types/api'

// ===== 用户 =====
export const listUsers = (p: { keyword?: string } & PageParams) =>
  api.get<Paged<AdminUser>>('/admin/users', p)

export const createUser = (p: CreateUserPayload) =>
  api.post<{ id: number; username: string }>('/admin/users', p)

export const updateUser = (id: number, p: UpdateUserPayload) =>
  api.put<{ message: string }>(`/admin/users/${id}`, p)

export const setUserPassword = (id: number, password: string) =>
  api.put<{ message: string }>(`/admin/users/${id}/password`, { password })

export const setUserQuota = (id: number, p: { quota?: number; delta?: number }) =>
  api.post<{ message: string }>(`/admin/users/${id}/quota`, p)

export const setUserDepartments = (id: number, p: { dept_ids: number[]; primary_dept_id?: number }) =>
  api.put<{ message: string }>(`/admin/users/${id}/departments`, p)

export const deleteUser = (id: number) => api.delete<{ message: string }>(`/admin/users/${id}`)

// ===== 部门 =====
export const getDepartmentTree = () =>
  api.get<{ tree: DeptNode[]; total: number }>('/admin/departments')

export const createDepartment = (p: CreateDeptPayload) => api.post<DeptNode>('/admin/departments', p)

export const updateDepartment = (id: number, p: UpdateDeptPayload) =>
  api.put<{ message: string }>(`/admin/departments/${id}`, p)

export const moveDepartment = (id: number, newParentId: number) =>
  api.post<{ message: string }>(`/admin/departments/${id}/move`, { new_parent_id: newParentId })

export const deleteDepartment = (id: number) =>
  api.delete<{ message: string }>(`/admin/departments/${id}`)

export const getDepartmentUsers = (id: number, includeSubtree: boolean) =>
  api.get<Paged<AdminUser>>(`/admin/departments/${id}/users`, {
    include_subtree: includeSubtree ? 1 : undefined,
  })

// ===== Key =====
export const listKeys = (p: { user_id?: number } & PageParams) =>
  api.get<Paged<ApiKey>>('/admin/keys', p)

export const createKey = (p: CreateKeyPayload & { user_id: number }) =>
  api.post<CreatedKey>('/admin/keys', p)

export const updateKey = (id: number, p: UpdateKeyPayload) =>
  api.put<{ message: string }>(`/admin/keys/${id}`, p)

export const deleteKey = (id: number) => api.delete<{ message: string }>(`/admin/keys/${id}`)

// ===== 渠道 =====
export const listProviders = () => api.get<Paged<Provider>>('/admin/providers')

export const getProviderTypes = () => api.get<{ types: string[] }>('/admin/providers/types')

export const fetchProviderModels = (p: {
  type: string
  base_url: string
  api_key?: string
  proxy_url?: string
  provider_id?: number
}) => api.post<{ models: string[] }>('/admin/providers/fetch-models', p)

export const createProvider = (p: ProviderPayload) => api.post<Provider>('/admin/providers', p)

export const updateProvider = (id: number, p: ProviderPayload) =>
  api.put<Provider>(`/admin/providers/${id}`, p)

export const deleteProvider = (id: number) =>
  api.delete<{ message: string }>(`/admin/providers/${id}`)

// ===== 价目 =====
export const listPrices = () => api.get<Paged<ModelPrice>>('/admin/prices')

export const upsertPrice = (p: PricePayload) => api.post<{ message: string }>('/admin/prices', p)

export const deletePrice = (id: number) => api.delete<{ message: string }>(`/admin/prices/${id}`)

// ===== 日志与统计 =====
export const listLogs = (p: LogQuery & PageParams) => api.get<Paged<RequestLog>>('/admin/logs', p)

export const getStats = (p: { start?: string; end?: string; user_id?: number }) =>
  api.get<Paged<UsageStat>>('/admin/stats', p)

export const getTtftStats = (p: { start?: string; end?: string }) =>
  api.get<{ items: TtftStat[] }>('/admin/stats/ttft', p)
