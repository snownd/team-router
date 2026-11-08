<script setup lang="ts">
import { Message } from '@arco-design/web-vue'
import { onMounted, reactive, ref } from 'vue'

import { createKey, deleteKey, listKeys, updateKey } from '@/api/admin'
import KeyResultModal from '@/components/KeyResultModal.vue'
import QuotaText from '@/components/QuotaText.vue'
import StatusTag from '@/components/StatusTag.vue'
import { usePagination } from '@/composables/usePagination'
import type { ApiKey, CreatedKey } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { microToUsd, usdToMicro } from '@/utils/quota'

const filterUserId = ref<number | undefined>()
const { items, total, loading, page, pageSize, load, reset, onPageChange, onPageSizeChange } =
  usePagination<ApiKey>((p) => listKeys({ user_id: filterUserId.value, ...p }))

onMounted(reset)

const columns = [
  { title: 'ID', dataIndex: 'id', width: 70 },
  { title: '归属用户', dataIndex: 'user_id', width: 90 },
  { title: '名称', dataIndex: 'name' },
  { title: 'Key 前缀', dataIndex: 'key_prefix', width: 150 },
  { title: '状态', slotName: 'status', width: 90 },
  { title: 'Key 额度', slotName: 'quota', width: 120 },
  { title: '已用', slotName: 'used', width: 110, align: 'right' as const },
  { title: '模型白名单', slotName: 'models' },
  { title: '过期时间', slotName: 'expires', width: 180 },
  { title: '操作', slotName: 'actions', width: 130, fixed: 'right' as const },
]

// ===== 创建 =====
const createVisible = ref(false)
const createForm = reactive({
  userId: undefined as number | undefined,
  name: '',
  quota: undefined as number | undefined,
  models: [] as string[],
  expiresDays: undefined as number | undefined,
})
const created = ref<CreatedKey | null>(null)

async function submitCreate(): Promise<boolean> {
  if (!createForm.userId) {
    Message.warning('请输入归属用户 ID')
    return false
  }
  try {
    created.value = await createKey({
      user_id: createForm.userId,
      name: createForm.name || undefined,
      quota: usdToMicro(createForm.quota ?? 0),
      allowed_models: createForm.models.length ? createForm.models : undefined,
      expires_in_days: createForm.expiresDays || undefined,
    })
    createVisible.value = false
    createForm.userId = undefined
    createForm.name = ''
    createForm.quota = undefined
    createForm.models = []
    createForm.expiresDays = undefined
    await load()
    return true
  } catch {
    return false
  }
}

// ===== 编辑 =====
const editVisible = ref(false)
const editTarget = ref<ApiKey | null>(null)
const editForm = reactive({
  name: '',
  status: 1,
  quota: undefined as number | undefined,
})

function openEdit(record: ApiKey) {
  editTarget.value = record
  editForm.name = record.name
  editForm.status = record.status
  editForm.quota = microToUsd(record.quota)
  editVisible.value = true
}

async function submitEdit(): Promise<boolean> {
  if (!editTarget.value) return false
  try {
    await updateKey(editTarget.value.id, {
      name: editForm.name || undefined,
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

async function removeKey(record: ApiKey) {
  try {
    await deleteKey(record.id)
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
          Key 列表
          <a-tag size="small" color="gray">{{ total }}</a-tag>
        </div>
        <a-space>
          <a-input-number
            v-model="filterUserId"
            placeholder="用户 ID"
            :precision="0"
            style="width: 150px"
            allow-clear
            hide-button
          />
          <a-button type="primary" @click="reset">查询</a-button>
          <a-button type="primary" @click="createVisible = true">
            <template #icon><icon-plus /></template>
            为用户创建 Key
          </a-button>
        </a-space>
      </div>
      <a-table
        :data="items"
        :columns="columns"
        :loading="loading"
        :border="false"
        row-key="id"
        :scroll="{ x: 1300 }"
        :pagination="{ total, current: page, pageSize, showTotal: true, showPageSize: true }"
        @page-change="onPageChange"
        @page-size-change="onPageSizeChange"
      >
        <template #status="{ record }">
          <StatusTag :status="record.status" />
        </template>
        <template #quota="{ record }">
          <QuotaText :value="record.quota" />
        </template>
        <template #used="{ record }">
          {{ record.used_quota > 0 ? `$${(record.used_quota / 1e6).toFixed(4)}` : '-' }}
        </template>
        <template #models="{ record }">
          <a-space v-if="record.allowed_models?.length" wrap size="mini">
            <a-tag v-for="m in record.allowed_models" :key="m" size="small">{{ m }}</a-tag>
          </a-space>
          <span v-else>不限制</span>
        </template>
        <template #expires="{ record }">{{ formatDateTime(record.expires_at) }}</template>
        <template #actions="{ record }">
          <a-space size="mini">
            <a-button type="text" size="small" @click="openEdit(record)">编辑</a-button>
            <a-popconfirm content="确认删除该 Key？" @ok="removeKey(record)">
              <a-button type="text" size="small" status="danger">删除</a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:visible="createVisible"
      title="为用户创建 Key"
      width="560px"
      @before-ok="submitCreate"
    >
      <a-form :model="createForm" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="归属用户 ID" required>
              <a-input-number v-model="createForm.userId" :precision="0" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="备注名">
              <a-input v-model="createForm.name" placeholder="可选" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Key 额度上限（USD）" tooltip="0 表示不限额">
              <a-input-number v-model="createForm.quota" :min="0" :precision="2" placeholder="0" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="有效天数" tooltip="0 或留空表示永不过期">
              <a-input-number v-model="createForm.expiresDays" :min="0" :precision="0" placeholder="0" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="模型白名单" help="留空表示不限制">
          <a-select
            v-model="createForm.models"
            multiple
            allow-create
            allow-clear
            placeholder="输入模型名后回车"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="editVisible" title="编辑 Key" @before-ok="submitEdit">
      <a-form :model="editForm" layout="vertical">
        <a-form-item label="名称">
          <a-input v-model="editForm.name" />
        </a-form-item>
        <a-form-item label="状态">
          <a-switch v-model="editForm.status" :checked-value="1" :unchecked-value="2">
            <template #checked>启用</template>
            <template #unchecked>禁用</template>
          </a-switch>
        </a-form-item>
        <a-form-item label="Key 额度上限（USD）" help="0 表示不限额，提交后覆盖">
          <a-input-number v-model="editForm.quota" :min="0" :precision="2" />
        </a-form-item>
      </a-form>
    </a-modal>

    <KeyResultModal :key-data="created" @close="created = null" />
  </div>
</template>
