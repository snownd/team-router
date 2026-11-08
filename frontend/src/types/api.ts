export interface Paged<T> {
  items: T[]
  total: number
}

export interface PageParams {
  page: number
  page_size: number
}

export type Role = 'admin' | 'user'

export const STATUS_ENABLED = 1
export const STATUS_DISABLED = 2

// ===== 认证（docs/api/auth.md）=====
export interface LoginPayload {
  username: string
  password: string
}

export interface LoginUser {
  id: number
  username: string
  display_name?: string
  role: Role
  quota: number
  used_quota: number
}

export interface LoginResult {
  access_token: string
  user: LoginUser
}

// ===== 用户自助（docs/api/user.md）=====
// 注意：departments[] 按 Go 字段名序列化（PascalCase）
export interface UserDept {
  ID: number
  Name: string
  ParentID: number
  IsPrimary: boolean
}

export interface Me {
  id: number
  username: string
  display_name?: string
  email?: string
  role: Role
  status: number
  quota: number
  used_quota: number
  quota_remaining: number
  departments: UserDept[]
}

export interface ApiKey {
  id: number
  user_id?: number
  name: string
  key_prefix: string
  status: number
  quota: number
  used_quota: number
  allowed_models: string[] | null
  expires_at: string | null
  created_at: string
  updated_at: string
}

export interface CreateKeyPayload {
  name?: string
  quota?: number
  allowed_models?: string[]
  expires_in_days?: number
}

export interface CreatedKey {
  id: number
  name: string
  key: string
  key_prefix: string
  quota: number
  expires_at: string | null
  notice?: string
}

export interface UpdateKeyPayload {
  name?: string
  status?: number
  quota?: number
}

export interface UsageStat {
  bucket: string
  model: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  quota_cost: number
}

export interface RequestLog {
  id: number
  created_at: string
  request_id: string
  user_id: number
  username?: string
  api_key_id: number
  provider_id?: number
  provider_name?: string
  upstream_type?: string
  protocol: string
  model: string
  upstream_model?: string
  stream: boolean
  status_code: number
  is_error: boolean
  err_msg?: string
  input_tokens: number
  output_tokens: number
  reasoning_tokens?: number
  cache_read_tokens?: number
  cache_write_tokens?: number
  ttft_ms?: number
  total_ms?: number
  retries?: number
  quota_cost: number
  client_ip?: string
}

export interface LogQuery {
  start?: string
  end?: string
  user_id?: number
  api_key_id?: number
  provider_id?: number
  model?: string
  protocol?: string
  keyword?: string
  is_error?: string
}

// ===== 管理台：用户（docs/api/admin-users.md）=====
// 注意：user 对象按 Go 导出字段名序列化（PascalCase）；PasswordHash 存在于响应中但前端不声明、不展示
export interface AdminUser {
  ID: number
  Username: string
  DisplayName?: string
  Email?: string
  Role: Role
  Status: number
  Quota: number
  UsedQuota: number
  CreatedAt?: string
  UpdatedAt?: string
  departments?: UserDept[]
}

export interface CreateUserPayload {
  username: string
  password?: string
  display_name?: string
  email?: string
  role?: Role
  quota?: number
  dept_ids?: number[]
  primary_dept_id?: number
}

export interface UpdateUserPayload {
  display_name?: string
  email?: string
  role?: Role
  status?: number
  quota?: number
}

// ===== 管理台：部门（docs/api/admin-departments.md）=====
export interface DeptNode {
  id: number
  parent_id: number | null
  name: string
  sort_order: number
  status: number
  children?: DeptNode[]
}

export interface CreateDeptPayload {
  parent_id?: number
  name: string
  sort_order?: number
}

export interface UpdateDeptPayload {
  name?: string
  sort_order?: number
  status?: number
}

// ===== 管理台：渠道（docs/api/admin-providers.md）=====
export interface Provider {
  id: number
  name: string
  type: string
  base_url: string
  models: string[]
  model_mapping: Record<string, string>
  priority: number
  weight: number
  status: number
  proxy_url: string
  created_at: string
  updated_at: string
}

export interface ProviderPayload {
  name?: string
  type?: string
  base_url?: string
  api_key?: string
  models?: string[]
  model_mapping?: Record<string, string>
  priority?: number
  weight?: number
  status?: number
  proxy_url?: string
}

// ===== 管理台：价目（docs/api/admin-prices.md）=====
// *_per_m 均为每百万 token 的微美元数
export interface ModelPrice {
  id: number
  model_name: string
  provider_type: string
  input_per_m: number
  output_per_m: number
  cache_read_per_m: number
  cache_write_per_m: number
  currency: string
  status: number
  created_at: string
  updated_at: string
}

export interface PricePayload {
  model_name: string
  provider_type?: string
  input_per_m?: number
  output_per_m?: number
  cache_read_per_m?: number
  cache_write_per_m?: number
  currency?: string
  status?: number
}

// ===== 管理台：统计（docs/api/admin-logs-stats.md）=====
export interface TtftStat {
  provider_name: string
  model: string
  requests: number
  avg_ttft_ms: number
  avg_total_ms: number
}
