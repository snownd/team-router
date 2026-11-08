<script setup lang="ts">
import { Message } from '@arco-design/web-vue'
import { onMounted, reactive, ref } from 'vue'

import { createMyKey, deleteMyKey, getMyKeys } from '@/api/user'
import KeyResultModal from '@/components/KeyResultModal.vue'
import QuotaText from '@/components/QuotaText.vue'
import StatusTag from '@/components/StatusTag.vue'
import { usePagination } from '@/composables/usePagination'
import type { ApiKey, CreatedKey } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { usdToMicro } from '@/utils/quota'

const { items, total, loading, page, pageSize, load, onPageChange, onPageSizeChange } =
  usePagination<ApiKey>(getMyKeys)

onMounted(load)

const createVisible = ref(false)
const form = reactive({
  name: '',
  quota: undefined as number | undefined,
  models: [] as string[],
  expiresDays: undefined as number | undefined,
})
const created = ref<CreatedKey | null>(null)

const columns = [
  { title: 'ID', dataIndex: 'id', width: 70 },
  { title: '名称', dataIndex: 'name' },
  { title: 'Key 前缀', dataIndex: 'key_prefix', width: 160 },
  { title: '状态', slotName: 'status', width: 90 },
  { title: 'Key 额度', slotName: 'quota', width: 120 },
  { title: '已用', slotName: 'used', width: 120, align: 'right' as const },
  { title: '模型白名单', slotName: 'models' },
  { title: '过期时间', slotName: 'expires', width: 180 },
  { title: '创建时间', dataIndex: 'created_at', slotName: 'created', width: 180 },
  { title: '操作', slotName: 'actions', width: 100, fixed: 'right' as const },
]

async function submitCreate(): Promise<boolean> {
  try {
    created.value = await createMyKey({
      name: form.name || undefined,
      quota: usdToMicro(form.quota ?? 0),
      allowed_models: form.models.length ? form.models : undefined,
      expires_in_days: form.expiresDays || undefined,
    })
    form.name = ''
    form.quota = undefined
    form.models = []
    form.expiresDays = undefined
    createVisible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

async function removeKey(record: ApiKey) {
  try {
    await deleteMyKey(record.id)
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
          我的 API Key
          <a-tag size="small" color="gray">{{ total }}</a-tag>
        </div>
        <a-button type="primary" @click="createVisible = true">
          <template #icon><icon-plus /></template>
          创建 API Key
        </a-button>
      </div>
      <a-table
        :data="items"
        :columns="columns"
        :loading="loading"
        :border="false"
        row-key="id"
        :scroll="{ x: 1200 }"
        :pagination="{
          total,
          current: page,
          pageSize,
          showTotal: true,
          showPageSize: true,
        }"
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
        <template #created="{ record }">{{ formatDateTime(record.created_at) }}</template>
        <template #actions="{ record }">
          <a-popconfirm content="确认删除该 Key？" @ok="removeKey(record)">
            <a-button type="text" size="small" status="danger">删除</a-button>
          </a-popconfirm>
        </template>
      </a-table>
    </a-card>

    <a-modal v-model:visible="createVisible" title="创建 API Key" @before-ok="submitCreate">
      <a-form :model="form" layout="vertical">
        <a-form-item label="备注名">
          <a-input v-model="form.name" placeholder="可选" />
        </a-form-item>
        <a-form-item label="Key 额度上限（USD）" help="0 表示不限额">
          <a-input-number v-model="form.quota" :min="0" :precision="2" placeholder="0" />
        </a-form-item>
        <a-form-item label="模型白名单" help="留空表示不限制">
          <a-select
            v-model="form.models"
            multiple
            allow-create
            allow-clear
            placeholder="输入模型名后回车"
          />
        </a-form-item>
        <a-form-item label="有效天数" help="0 或留空表示永不过期">
          <a-input-number v-model="form.expiresDays" :min="0" :precision="0" placeholder="0" />
        </a-form-item>
      </a-form>
    </a-modal>

    <KeyResultModal :key-data="created" @close="created = null" />
  </div>
</template>
