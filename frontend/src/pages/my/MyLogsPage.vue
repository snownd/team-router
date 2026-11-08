<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'

import { getMyLogs } from '@/api/user'
import { usePagination } from '@/composables/usePagination'
import type { LogQuery, RequestLog } from '@/types/api'
import { defaultRange, formatDateTime } from '@/utils/format'
import { formatUsd } from '@/utils/quota'

const range = ref<[string, string]>(defaultRange())
const filters = reactive({ model: '', protocol: '', keyword: '', isError: '' })

function buildQuery(): LogQuery {
  const [start, end] = range.value
  const q: LogQuery = { start, end }
  if (filters.model) q.model = filters.model
  if (filters.protocol) q.protocol = filters.protocol
  if (filters.keyword) q.keyword = filters.keyword
  if (filters.isError) q.is_error = filters.isError
  return q
}

const { items, total, loading, page, pageSize, reset, onPageChange, onPageSizeChange } =
  usePagination<RequestLog>((p) => getMyLogs({ ...buildQuery(), ...p }))

onMounted(reset)

function clearFilters() {
  range.value = defaultRange()
  filters.model = ''
  filters.protocol = ''
  filters.keyword = ''
  filters.isError = ''
  reset()
}

const columns = [
  { title: '时间', slotName: 'time', width: 170 },
  { title: '模型', dataIndex: 'model', width: 170 },
  { title: '协议', dataIndex: 'protocol', width: 150 },
  { title: '状态', slotName: 'status', width: 100 },
  { title: '输入', slotName: 'input', width: 90, align: 'right' as const },
  { title: '输出', slotName: 'output', width: 90, align: 'right' as const },
  { title: '费用', slotName: 'cost', width: 120, align: 'right' as const },
  { title: '耗时', slotName: 'duration', width: 110, align: 'right' as const },
  { title: '错误信息', dataIndex: 'err_msg', ellipsis: true, tooltip: true },
]

function detailRows(record: RequestLog) {
  return [
    { label: 'Request ID', value: record.request_id },
    { label: '上游模型', value: record.upstream_model || '-' },
    { label: '渠道', value: record.provider_name || '-' },
    { label: '缓存读', value: (record.cache_read_tokens ?? 0).toLocaleString() },
    { label: '缓存写', value: (record.cache_write_tokens ?? 0).toLocaleString() },
    { label: '推理 Tokens', value: (record.reasoning_tokens ?? 0).toLocaleString() },
    { label: '重试次数', value: String(record.retries ?? 0) },
    { label: '客户端 IP', value: record.client_ip || '-' },
  ]
}
</script>

<template>
  <div class="page">
    <a-card :bordered="false">
      <a-form :model="filters" layout="inline">
        <a-form-item label="时间">
          <a-range-picker v-model="range" value-format="YYYY-MM-DD" style="width: 250px" />
        </a-form-item>
        <a-form-item label="模型">
          <a-input v-model="filters.model" placeholder="模型" style="width: 150px" allow-clear />
        </a-form-item>
        <a-form-item label="协议">
          <a-select
            v-model="filters.protocol"
            placeholder="全部"
            style="width: 170px"
            allow-clear
            :options="[
              { label: 'chat_completions', value: 'chat_completions' },
              { label: 'responses', value: 'responses' },
              { label: 'messages', value: 'messages' },
            ]"
          />
        </a-form-item>
        <a-form-item label="结果">
          <a-select
            v-model="filters.isError"
            placeholder="全部"
            style="width: 110px"
            allow-clear
            :options="[
              { label: '仅成功', value: '0' },
              { label: '仅错误', value: '1' },
            ]"
          />
        </a-form-item>
        <a-form-item label="关键字">
          <a-input v-model="filters.keyword" placeholder="关键字" style="width: 150px" allow-clear />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button type="primary" @click="reset">
              <template #icon><icon-search /></template>
              查询
            </a-button>
            <a-button @click="clearFilters">
              <template #icon><icon-refresh /></template>
              重置
            </a-button>
          </a-space>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card :bordered="false">
      <div class="table-head">
        <div class="table-title">
          调用记录
          <a-tag size="small" color="gray">{{ total }}</a-tag>
        </div>
      </div>
      <a-table
        :data="items"
        :columns="columns"
        :loading="loading"
        :border="false"
        row-key="id"
        :scroll="{ x: 1200 }"
        :expandable="{ width: 60 }"
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
        <template #time="{ record }">{{ formatDateTime(record.created_at) }}</template>
        <template #status="{ record }">
          <a-tag :color="record.is_error ? 'red' : 'green'">{{ record.status_code }}</a-tag>
        </template>
        <template #input="{ record }">{{ record.input_tokens.toLocaleString() }}</template>
        <template #output="{ record }">{{ record.output_tokens.toLocaleString() }}</template>
        <template #cost="{ record }">{{ formatUsd(record.quota_cost) }}</template>
        <template #duration="{ record }">
          {{ record.total_ms != null ? `${record.total_ms} ms` : '-' }}
        </template>
        <template #expand-row="{ record }">
          <a-descriptions :data="detailRows(record)" :column="3" size="small" />
        </template>
      </a-table>
    </a-card>
  </div>
</template>
