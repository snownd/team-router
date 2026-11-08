<script setup lang="ts">
import { Message } from '@arco-design/web-vue'
import { onMounted, reactive, ref } from 'vue'

import { deletePrice, getProviderTypes, listPrices, upsertPrice } from '@/api/admin'
import StatusTag from '@/components/StatusTag.vue'
import type { ModelPrice } from '@/types/api'
import { microToUsd, usdToMicro } from '@/utils/quota'

const prices = ref<ModelPrice[]>([])
const types = ref<string[]>([])
const loading = ref(false)

const columns = [
  { title: '模型', dataIndex: 'model_name' },
  { title: '渠道类型', slotName: 'providerType', width: 120 },
  { title: '输入 $/M', slotName: 'input', width: 110, align: 'right' as const },
  { title: '输出 $/M', slotName: 'output', width: 110, align: 'right' as const },
  { title: '缓存读 $/M', slotName: 'cacheRead', width: 120, align: 'right' as const },
  { title: '缓存写 $/M', slotName: 'cacheWrite', width: 120, align: 'right' as const },
  { title: '币种', dataIndex: 'currency', width: 80 },
  { title: '状态', slotName: 'status', width: 90 },
  { title: '操作', slotName: 'actions', width: 100, fixed: 'right' as const },
]

async function load() {
  loading.value = true
  try {
    prices.value = (await listPrices()).items
  } catch {
    // 拦截器已提示
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  load()
  try {
    types.value = (await getProviderTypes()).types
  } catch {
    // 拦截器已提示
  }
})

const visible = ref(false)
const form = reactive({
  modelName: '',
  providerType: '',
  inputPerM: undefined as number | undefined,
  outputPerM: undefined as number | undefined,
  cacheReadPerM: undefined as number | undefined,
  cacheWritePerM: undefined as number | undefined,
  status: 1,
})

function openCreate() {
  form.modelName = ''
  form.providerType = ''
  form.inputPerM = undefined
  form.outputPerM = undefined
  form.cacheReadPerM = undefined
  form.cacheWritePerM = undefined
  form.status = 1
  visible.value = true
}

function openEdit(record: ModelPrice) {
  form.modelName = record.model_name
  form.providerType = record.provider_type
  form.inputPerM = microToUsd(record.input_per_m)
  form.outputPerM = microToUsd(record.output_per_m)
  form.cacheReadPerM = microToUsd(record.cache_read_per_m)
  form.cacheWritePerM = microToUsd(record.cache_write_per_m)
  form.status = record.status
  visible.value = true
}

async function submit(): Promise<boolean> {
  if (!form.modelName) {
    Message.warning('请输入模型名')
    return false
  }
  try {
    await upsertPrice({
      model_name: form.modelName,
      provider_type: form.providerType || undefined,
      input_per_m: usdToMicro(form.inputPerM ?? 0),
      output_per_m: usdToMicro(form.outputPerM ?? 0),
      cache_read_per_m: usdToMicro(form.cacheReadPerM ?? 0),
      cache_write_per_m: usdToMicro(form.cacheWritePerM ?? 0),
      status: form.status,
    })
    Message.success('已保存')
    visible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

async function removePrice(record: ModelPrice) {
  try {
    await deletePrice(record.id)
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
          价目列表
          <a-tag size="small" color="gray">{{ prices.length }}</a-tag>
        </div>
        <a-button type="primary" @click="openCreate">
          <template #icon><icon-plus /></template>
          新增价目
        </a-button>
      </div>
      <a-table
        :data="prices"
        :columns="columns"
        :loading="loading"
        :border="false"
        row-key="id"
        :scroll="{ x: 1100 }"
        :pagination="{ pageSize: 20, showTotal: true }"
      >
        <template #providerType="{ record }">
          <a-tag v-if="record.provider_type" size="small">{{ record.provider_type }}</a-tag>
          <a-tag v-else color="cyan" size="small">通配</a-tag>
        </template>
        <template #input="{ record }">{{ microToUsd(record.input_per_m) }}</template>
        <template #output="{ record }">{{ microToUsd(record.output_per_m) }}</template>
        <template #cacheRead="{ record }">{{ microToUsd(record.cache_read_per_m) }}</template>
        <template #cacheWrite="{ record }">{{ microToUsd(record.cache_write_per_m) }}</template>
        <template #status="{ record }">
          <StatusTag :status="record.status" />
        </template>
        <template #actions="{ record }">
          <a-space size="mini">
            <a-button type="text" size="small" @click="openEdit(record)">编辑</a-button>
            <a-popconfirm content="确认删除该价目？" @ok="removePrice(record)">
              <a-button type="text" size="small" status="danger">删除</a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </a-table>
    </a-card>

    <a-modal v-model:visible="visible" title="价目配置" width="640px" @before-ok="submit">
      <a-form :model="form" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="模型名" required>
              <a-input v-model="form.modelName" placeholder="如 gpt-4o" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="渠道类型" tooltip="留空为通配价，作用于所有渠道类型">
              <a-select
                v-model="form.providerType"
                allow-clear
                :options="types.map((t) => ({ label: t, value: t }))"
                placeholder="通配（全部渠道类型）"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="输入 $/M">
              <a-input-number v-model="form.inputPerM" :min="0" :precision="4" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="输出 $/M">
              <a-input-number v-model="form.outputPerM" :min="0" :precision="4" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="缓存读 $/M">
              <a-input-number v-model="form.cacheReadPerM" :min="0" :precision="4" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="缓存写 $/M">
              <a-input-number v-model="form.cacheWritePerM" :min="0" :precision="4" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="状态">
          <a-switch v-model="form.status" :checked-value="1" :unchecked-value="2">
            <template #checked>启用</template>
            <template #unchecked>禁用</template>
          </a-switch>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>
