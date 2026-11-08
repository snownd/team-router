<script setup lang="ts">
import { Message } from '@arco-design/web-vue'
import { onMounted, reactive, ref } from 'vue'

import {
  createUser,
  deleteUser,
  getDepartmentTree,
  listUsers,
  setUserDepartments,
  setUserPassword,
  setUserQuota,
  updateUser,
} from '@/api/admin'
import QuotaText from '@/components/QuotaText.vue'
import StatusTag from '@/components/StatusTag.vue'
import { usePagination } from '@/composables/usePagination'
import type { AdminUser } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { flattenDepts, type DeptOption } from '@/utils/dept'
import { microToUsd, usdToMicro } from '@/utils/quota'

const keyword = ref('')
const { items, total, loading, page, pageSize, load, reset, onPageChange, onPageSizeChange } =
  usePagination<AdminUser>((p) => listUsers({ keyword: keyword.value || undefined, ...p }))

const deptOptions = ref<DeptOption[]>([])

function deptNames(u: AdminUser): string {
  return (u.departments ?? []).map((d) => d.Name).join('、')
}

onMounted(async () => {
  reset()
  try {
    deptOptions.value = flattenDepts((await getDepartmentTree()).tree)
  } catch {
    // 拦截器已提示
  }
})

const columns = [
  { title: 'ID', dataIndex: 'ID', width: 70 },
  { title: '用户名', dataIndex: 'Username', width: 130 },
  { title: '姓名', slotName: 'displayName', width: 110 },
  { title: '邮箱', dataIndex: 'Email', width: 190 },
  { title: '角色', slotName: 'role', width: 90 },
  { title: '状态', slotName: 'status', width: 90 },
  { title: '总额度', slotName: 'quota', width: 120 },
  { title: '已用', slotName: 'used', width: 110, align: 'right' as const },
  { title: '部门', slotName: 'depts' },
  { title: '创建时间', slotName: 'createdAt', width: 180 },
  { title: '操作', slotName: 'actions', width: 240, fixed: 'right' as const },
]

// ===== 创建 =====
const createVisible = ref(false)
const createForm = reactive({
  username: '',
  password: '',
  displayName: '',
  email: '',
  role: 'user' as 'admin' | 'user',
  quota: undefined as number | undefined,
  deptIds: [] as number[],
  primaryDeptId: undefined as number | undefined,
})

async function submitCreate(): Promise<boolean> {
  if (!createForm.username) {
    Message.warning('请输入用户名')
    return false
  }
  try {
    await createUser({
      username: createForm.username,
      password: createForm.password || undefined,
      display_name: createForm.displayName || undefined,
      email: createForm.email || undefined,
      role: createForm.role,
      quota: createForm.quota ? usdToMicro(createForm.quota) : undefined,
      dept_ids: createForm.deptIds.length ? createForm.deptIds : undefined,
      primary_dept_id: createForm.primaryDeptId,
    })
    Message.success('已创建')
    createVisible.value = false
    createForm.username = ''
    createForm.password = ''
    createForm.displayName = ''
    createForm.email = ''
    createForm.role = 'user'
    createForm.quota = undefined
    createForm.deptIds = []
    createForm.primaryDeptId = undefined
    reset()
    return true
  } catch {
    return false
  }
}

// ===== 编辑资料 =====
const editVisible = ref(false)
const editTarget = ref<AdminUser | null>(null)
const editForm = reactive({
  displayName: '',
  email: '',
  role: 'user' as 'admin' | 'user',
  status: 1,
  quota: undefined as number | undefined,
})

function openEdit(u: AdminUser) {
  editTarget.value = u
  editForm.displayName = u.DisplayName ?? ''
  editForm.email = u.Email ?? ''
  editForm.role = u.Role
  editForm.status = u.Status
  editForm.quota = microToUsd(u.Quota)
  editVisible.value = true
}

async function submitEdit(): Promise<boolean> {
  if (!editTarget.value) return false
  try {
    await updateUser(editTarget.value.ID, {
      display_name: editForm.displayName || undefined,
      email: editForm.email || undefined,
      role: editForm.role,
      status: editForm.status,
      quota: usdToMicro(editForm.quota ?? 0),
    })
    Message.success('已保存')
    editVisible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

// ===== 配额调整 =====
const quotaVisible = ref(false)
const quotaTarget = ref<AdminUser | null>(null)
const quotaForm = reactive({
  mode: 'set' as 'set' | 'delta',
  amount: undefined as number | undefined,
})

function openQuota(u: AdminUser) {
  quotaTarget.value = u
  quotaForm.mode = 'set'
  quotaForm.amount = undefined
  quotaVisible.value = true
}

async function submitQuota(): Promise<boolean> {
  if (!quotaTarget.value || quotaForm.amount == null) {
    Message.warning('请输入金额')
    return false
  }
  const payload =
    quotaForm.mode === 'set'
      ? { quota: usdToMicro(quotaForm.amount) }
      : { delta: usdToMicro(quotaForm.amount) }
  try {
    await setUserQuota(quotaTarget.value.ID, payload)
    Message.success('已更新')
    quotaVisible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

// ===== 重置密码 =====
const pwdVisible = ref(false)
const pwdTarget = ref<AdminUser | null>(null)
const pwdForm = reactive({ password: '' })

function openPassword(u: AdminUser) {
  pwdTarget.value = u
  pwdForm.password = ''
  pwdVisible.value = true
}

async function submitPassword(): Promise<boolean> {
  if (!pwdTarget.value || !pwdForm.password) {
    Message.warning('请输入新密码')
    return false
  }
  try {
    await setUserPassword(pwdTarget.value.ID, pwdForm.password)
    Message.success('已重置')
    pwdVisible.value = false
    return true
  } catch {
    return false
  }
}

// ===== 部门归属 =====
const deptVisible = ref(false)
const deptTarget = ref<AdminUser | null>(null)
const deptForm = reactive({
  deptIds: [] as number[],
  primaryDeptId: undefined as number | undefined,
})

function openDepartments(u: AdminUser) {
  deptTarget.value = u
  deptForm.deptIds = (u.departments ?? []).map((d) => d.ID)
  deptForm.primaryDeptId = u.departments?.find((d) => d.IsPrimary)?.ID
  deptVisible.value = true
}

async function submitDepartments(): Promise<boolean> {
  if (!deptTarget.value) return false
  if (!deptForm.deptIds.length) {
    Message.warning('请至少选择一个部门')
    return false
  }
  try {
    await setUserDepartments(deptTarget.value.ID, {
      dept_ids: deptForm.deptIds,
      primary_dept_id: deptForm.primaryDeptId,
    })
    Message.success('已保存')
    deptVisible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

async function removeUser(u: AdminUser) {
  try {
    await deleteUser(u.ID)
    Message.success('已删除')
    await load()
  } catch {
    // 拦截器已提示
  }
}
</script>

<template>
  <div class="page">
    <a-card :bordered="false">
      <div class="table-head">
        <div class="table-title">
          用户列表
          <a-tag size="small" color="gray">{{ total }}</a-tag>
        </div>
        <a-space>
          <a-input-search
            v-model="keyword"
            placeholder="用户名 / 邮箱"
            style="width: 240px"
            allow-clear
            search-button
            @keyup.enter="reset"
            @search="reset"
          />
          <a-button type="primary" @click="createVisible = true">
            <template #icon><icon-plus /></template>
            创建用户
          </a-button>
        </a-space>
      </div>
      <a-table
        :data="items"
        :columns="columns"
        :loading="loading"
        :border="false"
        row-key="ID"
        :scroll="{ x: 1400 }"
        :pagination="{ total, current: page, pageSize, showTotal: true, showPageSize: true }"
        @page-change="onPageChange"
        @page-size-change="onPageSizeChange"
      >
        <template #displayName="{ record }">{{ record.DisplayName || '-' }}</template>
        <template #role="{ record }">
          <a-tag :color="record.Role === 'admin' ? 'cyan' : 'gray'" size="small">
            {{ record.Role === 'admin' ? '管理员' : '成员' }}
          </a-tag>
        </template>
        <template #status="{ record }">
          <StatusTag :status="record.Status" />
        </template>
        <template #quota="{ record }">
          <QuotaText :value="record.Quota" />
        </template>
        <template #used="{ record }">
          {{ record.UsedQuota > 0 ? `$${(record.UsedQuota / 1e6).toFixed(4)}` : '-' }}
        </template>
        <template #depts="{ record }">{{ deptNames(record) }}</template>
        <template #createdAt="{ record }">{{ formatDateTime(record.CreatedAt) }}</template>
        <template #actions="{ record }">
          <a-space size="mini">
            <a-button type="text" size="small" @click="openEdit(record)">编辑</a-button>
            <a-button type="text" size="small" @click="openQuota(record)">配额</a-button>
            <a-button type="text" size="small" @click="openDepartments(record)">部门</a-button>
            <a-button type="text" size="small" @click="openPassword(record)">密码</a-button>
            <a-popconfirm content="确认删除该用户？" @ok="removeUser(record)">
              <a-button type="text" size="small" status="danger">删除</a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:visible="createVisible"
      title="创建用户"
      width="640px"
      @before-ok="submitCreate"
    >
      <a-form :model="createForm" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="用户名" required>
              <a-input v-model="createForm.username" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="初始密码" tooltip="留空则暂无本地密码">
              <a-input-password v-model="createForm.password" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="姓名">
              <a-input v-model="createForm.displayName" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="邮箱">
              <a-input v-model="createForm.email" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="角色">
              <a-radio-group v-model="createForm.role">
                <a-radio value="user">成员</a-radio>
                <a-radio value="admin">管理员</a-radio>
              </a-radio-group>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="总额度（USD）" tooltip="0 表示不限额">
              <a-input-number v-model="createForm.quota" :min="0" :precision="2" placeholder="0" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="所属部门">
              <a-select
                v-model="createForm.deptIds"
                multiple
                allow-clear
                placeholder="选择部门"
                :options="deptOptions.map((d) => ({ label: d.label, value: d.id }))"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="主部门">
              <a-select
                v-model="createForm.primaryDeptId"
                allow-clear
                placeholder="缺省取第一个部门"
                :options="createForm.deptIds.map((id) => ({
                  label: deptOptions.find((d) => d.id === id)?.label ?? String(id),
                  value: id,
                }))"
              />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="editVisible" title="编辑用户" @before-ok="submitEdit">
      <a-form :model="editForm" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="姓名">
              <a-input v-model="editForm.displayName" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="邮箱">
              <a-input v-model="editForm.email" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="角色">
              <a-radio-group v-model="editForm.role">
                <a-radio value="user">成员</a-radio>
                <a-radio value="admin">管理员</a-radio>
              </a-radio-group>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="状态">
              <a-switch v-model="editForm.status" :checked-value="1" :unchecked-value="2">
                <template #checked>启用</template>
                <template #unchecked>禁用</template>
              </a-switch>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="总额度（USD）" tooltip="0 表示不限额，提交后覆盖">
              <a-input-number v-model="editForm.quota" :min="0" :precision="2" />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="quotaVisible" title="配额调整" @before-ok="submitQuota">
      <a-descriptions v-if="quotaTarget" :column="2" size="small" class="quota-desc">
        <a-descriptions-item label="用户">{{ quotaTarget.Username }}</a-descriptions-item>
        <a-descriptions-item label="当前已用">
          {{ `$${(quotaTarget.UsedQuota / 1e6).toFixed(4)}` }}
        </a-descriptions-item>
      </a-descriptions>
      <a-form :model="quotaForm" layout="vertical">
        <a-form-item label="方式">
          <a-radio-group v-model="quotaForm.mode" type="button">
            <a-radio value="set">设置总额度</a-radio>
            <a-radio value="delta">增量调整</a-radio>
          </a-radio-group>
        </a-form-item>
        <a-form-item :label="quotaForm.mode === 'set' ? '总额度（USD）' : '调整额（USD，可为负）'">
          <a-input-number v-model="quotaForm.amount" :precision="2" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="pwdVisible" title="重置密码" @before-ok="submitPassword">
      <a-form :model="pwdForm" layout="vertical">
        <a-form-item label="新密码" required>
          <a-input-password v-model="pwdForm.password" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="deptVisible"
      title="设置所属部门"
      width="560px"
      @before-ok="submitDepartments"
    >
      <a-form :model="deptForm" layout="vertical">
        <a-form-item label="所属部门" required>
          <a-select
            v-model="deptForm.deptIds"
            multiple
            placeholder="选择部门"
            :options="deptOptions.map((d) => ({ label: d.label, value: d.id }))"
          />
        </a-form-item>
        <a-form-item label="主部门">
          <a-select
            v-model="deptForm.primaryDeptId"
            allow-clear
            placeholder="缺省取第一个部门"
            :options="deptForm.deptIds.map((id) => ({
              label: deptOptions.find((d) => d.id === id)?.label ?? String(id),
              value: id,
            }))"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.quota-desc {
  margin-bottom: 16px;
}
</style>
