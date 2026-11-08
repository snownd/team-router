<script setup lang="ts">
import { Message } from '@arco-design/web-vue'
import { onMounted, reactive, ref } from 'vue'

import { createProvider, deleteProvider, fetchProviderModels, getProviderTypes, listProviders, updateProvider } from '@/api/admin'
import StatusTag from '@/components/StatusTag.vue'
import type { Provider, ProviderPayload } from '@/types/api'
import { formatDateTime } from '@/utils/format'

const providers = ref<Provider[]>([])
const types = ref<string[]>([])
const loading = ref(false)

const columns = [
  { title: 'ID', dataIndex: 'id', width: 70 },
  { title: '名称', dataIndex: 'name', width: 130 },
  { title: '类型', slotName: 'type', width: 110 },
  { title: '上游地址', dataIndex: 'base_url', width: 220 },
  { title: '模型', slotName: 'models' },
  { title: '优先级', dataIndex: 'priority', width: 80, align: 'right' as const },
  { title: '权重', dataIndex: 'weight', width: 70, align: 'right' as const },
  { title: '状态', slotName: 'status', width: 90 },
  { title: '更新时间', slotName: 'updatedAt', width: 180 },
  { title: '操作', slotName: 'actions', width: 130, fixed: 'right' as const },
]

async function load() {
  loading.value = true
  try {
    providers.value = (await listProviders()).items
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
const editing = ref<Provider | null>(null)
const form = reactive({
  name: '',
  type: '',
  baseUrl: '',
  apiKey: '',
  models: [] as string[],
  mappingText: '{}',
  priority: 0,
  weight: 1,
  proxyUrl: '',
  status: 1,
})

const modelOptions = ref<{ label: string; value: string }[]>([])
const fetchingModels = ref(false)

async function fetchModels() {
  if (!form.type || !form.baseUrl) {
    Message.warning('请先选择适配器类型并填写上游地址')
    return
  }
  fetchingModels.value = true
  try {
    const { models } = await fetchProviderModels({
      type: form.type,
      base_url: form.baseUrl,
      api_key: form.apiKey || undefined,
      proxy_url: form.proxyUrl || undefined,
      provider_id: editing.value?.id,
    })
    modelOptions.value = models.map((m) => ({ label: m, value: m }))
    if (!models.length) Message.warning('上游返回的模型列表为空')
    else Message.success(`已拉取 ${models.length} 个模型`)
  } catch {
    // 拦截器已提示
  } finally {
    fetchingModels.value = false
  }
}

function openCreate() {
  editing.value = null
  form.name = ''
  form.type = types.value[0] ?? ''
  form.baseUrl = ''
  form.apiKey = ''
  form.models = []
  form.mappingText = '{}'
  form.priority = 0
  form.weight = 1
  form.proxyUrl = ''
  form.status = 1
  modelOptions.value = []
  visible.value = true
}

function openEdit(record: Provider) {
  editing.value = record
  form.name = record.name
  form.type = record.type
  form.baseUrl = record.base_url
  form.apiKey = ''
  form.models = [...record.models]
  form.mappingText = JSON.stringify(record.model_mapping ?? {}, null, 2)
  form.priority = record.priority
  form.weight = record.weight
  form.proxyUrl = record.proxy_url
  form.status = record.status
  modelOptions.value = []
  visible.value = true
}

async function submit(): Promise<boolean> {
  if (!form.name || !form.type || !form.baseUrl) {
    Message.warning('名称、类型、上游地址为必填项')
    return false
  }
  let mapping: Record<string, string> = {}
  if (form.mappingText.trim()) {
    try {
      mapping = JSON.parse(form.mappingText)
    } catch {
      Message.error('模型映射不是合法 JSON')
      return false
    }
  }
  const payload: ProviderPayload = {
    name: form.name,
    type: form.type,
    base_url: form.baseUrl,
    models: form.models,
    model_mapping: mapping,
    priority: form.priority ?? 0,
    weight: form.weight ?? 1,
    status: form.status,
    proxy_url: form.proxyUrl || undefined,
  }
  if (form.apiKey) payload.api_key = form.apiKey
  try {
    if (editing.value) {
      await updateProvider(editing.value.id, payload)
      Message.success('已保存')
    } else {
      await createProvider(payload)
      Message.success('已创建')
    }
    visible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

async function removeProvider(record: Provider) {
  try {
    await deleteProvider(record.id)
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
          渠道列表
          <a-tag size="small" color="gray">{{ providers.length }}</a-tag>
        </div>
        <a-button type="primary" @click="openCreate">
          <template #icon><icon-plus /></template>
          创建渠道
        </a-button>
      </div>
      <a-table
        :data="providers"
        :columns="columns"
        :loading="loading"
        :border="false"
        row-key="id"
        :scroll="{ x: 1300 }"
        :pagination="{ pageSize: 20, showTotal: true }"
      >
        <template #type="{ record }">
          <a-tag size="small" color="cyan">{{ record.type }}</a-tag>
        </template>
        <template #models="{ record }">
          <a-space v-if="record.models?.length" wrap size="mini">
            <a-tag v-for="m in record.models" :key="m" size="small">{{ m }}</a-tag>
          </a-space>
          <span v-else>-</span>
        </template>
        <template #status="{ record }">
          <StatusTag :status="record.status" />
        </template>
        <template #updatedAt="{ record }">{{ formatDateTime(record.updated_at) }}</template>
        <template #actions="{ record }">
          <a-space size="mini">
            <a-button type="text" size="small" @click="openEdit(record)">编辑</a-button>
            <a-popconfirm content="确认删除该渠道？" @ok="removeProvider(record)">
              <a-button type="text" size="small" status="danger">删除</a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:visible="visible"
      :title="editing ? '编辑渠道' : '创建渠道'"
      width="640px"
      @before-ok="submit"
    >
      <a-form :model="form" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="渠道名称" required>
              <a-input v-model="form.name" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="适配器类型" required>
              <a-select
                v-model="form.type"
                :options="types.map((t) => ({ label: t, value: t }))"
                :disabled="!!editing"
              />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="上游地址" required help="不以 /v1 结尾时后端自动补全">
          <a-input v-model="form.baseUrl" placeholder="https://api.openai.com" />
        </a-form-item>
        <a-form-item
          label="上游 API Key"
          :help="editing ? '留空表示保持不变' : '加密落库，不会再次显示'"
        >
          <a-input-password v-model="form.apiKey" />
        </a-form-item>
        <a-form-item
          label="可服务模型"
          help="网关模型别名：从上游拉取列表后选择，或输入自定义值回车添加"
        >
          <div class="models-row">
            <a-select
              v-model="form.models"
              class="models-select"
              multiple
              allow-create
              allow-clear
              :options="modelOptions"
              :loading="fetchingModels"
              placeholder="拉取上游模型或手动输入"
            />
            <a-button :loading="fetchingModels" @click="fetchModels">拉取模型</a-button>
          </div>
        </a-form-item>
        <a-form-item label="模型映射（JSON）" help='请求模型 → 上游模型，如 {"gpt-4o": "gpt-4o-2024-11-20"}'>
          <a-textarea v-model="form.mappingText" :auto-size="{ minRows: 2, maxRows: 6 }" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="8">
            <a-form-item label="优先级" tooltip="数值大者优先">
              <a-input-number v-model="form.priority" :precision="0" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="权重">
              <a-input-number v-model="form.weight" :min="0" :precision="0" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="状态">
              <a-switch v-model="form.status" :checked-value="1" :unchecked-value="2">
                <template #checked>启用</template>
                <template #unchecked>禁用</template>
              </a-switch>
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="HTTP 代理（可选）">
          <a-input v-model="form.proxyUrl" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.models-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.models-select {
  flex: 1;
}
</style>
