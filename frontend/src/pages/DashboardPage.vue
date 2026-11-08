<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { getStats, getTtftStats } from '@/api/admin'
import { getMe, getMyStats } from '@/api/user'
import { useAuthStore } from '@/stores/useAuthStore'
import type { Me, TtftStat, UsageStat } from '@/types/api'
import { defaultRange } from '@/utils/format'
import { formatUsd, isUnlimitedQuota, quotaText } from '@/utils/quota'

const auth = useAuthStore()
const range = ref<[string, string]>(defaultRange())
const loading = ref(false)
const me = ref<Me | null>(null)
const stats = ref<UsageStat[]>([])
const ttft = ref<TtftStat[]>([])

const summary = computed(() =>
  stats.value.reduce(
    (acc, s) => ({
      requests: acc.requests + s.requests,
      input: acc.input + s.input_tokens + s.cache_read_tokens + s.cache_write_tokens,
      output: acc.output + s.output_tokens,
      cost: acc.cost + s.quota_cost,
    }),
    { requests: 0, input: 0, output: 0, cost: 0 },
  ),
)

const quotaPct = computed(() => {
  if (!me.value || isUnlimitedQuota(me.value.quota) || me.value.quota <= 0) return null
  if (me.value.used_quota <= 0) return 0
  return Math.min(100, Math.round((me.value.used_quota / me.value.quota) * 100))
})

const modelRows = computed(() => {
  const map = new Map<
    string,
    { model: string; requests: number; input: number; output: number; cost: number }
  >()
  for (const s of stats.value) {
    const row = map.get(s.model) ?? { model: s.model, requests: 0, input: 0, output: 0, cost: 0 }
    row.requests += s.requests
    row.input += s.input_tokens
    row.output += s.output_tokens
    row.cost += s.quota_cost
    map.set(s.model, row)
  }
  return [...map.values()].sort((a, b) => b.cost - a.cost)
})

const modelColumns = [
  { title: '模型', dataIndex: 'model' },
  { title: '请求数', slotName: 'requests', width: 110, align: 'right' as const },
  { title: '输入 Tokens', slotName: 'input', width: 130, align: 'right' as const },
  { title: '输出 Tokens', slotName: 'output', width: 130, align: 'right' as const },
  { title: '费用', slotName: 'cost', width: 130, align: 'right' as const },
]

const ttftColumns = [
  { title: '渠道', dataIndex: 'provider_name' },
  { title: '模型', dataIndex: 'model' },
  { title: '请求数', slotName: 'requests', width: 110, align: 'right' as const },
  { title: '平均首 Token (ms)', slotName: 'ttft', width: 160, align: 'right' as const },
  { title: '平均总耗时 (ms)', slotName: 'total', width: 160, align: 'right' as const },
]

async function load() {
  loading.value = true
  try {
    const [start, end] = range.value
    if (auth.isAdmin) {
      const [s, t] = await Promise.all([getStats({ start, end }), getTtftStats({ start, end })])
      stats.value = s.items ?? []
      ttft.value = t.items ?? []
    } else {
      const [m, s] = await Promise.all([getMe(), getMyStats({ start, end })])
      me.value = m
      stats.value = s.items ?? []
    }
  } catch {
    // 错误提示已由 http 拦截器统一展示
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <a-space class="toolbar">
      <a-range-picker
        v-model="range"
        value-format="YYYY-MM-DD"
        style="width: 260px"
        @change="load"
      />
      <a-button @click="load">
        <template #icon><icon-refresh /></template>
        刷新
      </a-button>
    </a-space>

    <a-grid v-if="auth.isAdmin" :cols="{ xs: 1, sm: 3 }" :col-gap="16">
      <a-grid-item>
        <a-card class="stat-card" :bordered="false">
          <div class="stat-main">
            <span class="stat-chip"><icon-thunderbolt /></span>
            <div class="stat-body">
              <div class="stat-label">请求数</div>
              <div class="stat-value">{{ summary.requests.toLocaleString() }}</div>
            </div>
          </div>
        </a-card>
      </a-grid-item>
      <a-grid-item>
        <a-card class="stat-card" :bordered="false">
          <div class="stat-main">
            <span class="stat-chip is-blue"><icon-storage /></span>
            <div class="stat-body">
              <div class="stat-label">Tokens（含缓存 入/出）</div>
              <div class="stat-value">
                {{ summary.input.toLocaleString() }} / {{ summary.output.toLocaleString() }}
              </div>
            </div>
          </div>
        </a-card>
      </a-grid-item>
      <a-grid-item>
        <a-card class="stat-card" :bordered="false">
          <div class="stat-main">
            <span class="stat-chip is-gold"><icon-fire /></span>
            <div class="stat-body">
              <div class="stat-label">总费用</div>
              <div class="stat-value">{{ formatUsd(summary.cost) }}</div>
            </div>
          </div>
        </a-card>
      </a-grid-item>
    </a-grid>

    <a-grid v-else-if="me" :cols="{ xs: 1, sm: 2, md: 4 }" :col-gap="16">
      <a-grid-item>
        <a-card class="stat-card" :bordered="false">
          <div class="stat-main">
            <span class="stat-chip"><icon-safe /></span>
            <div class="stat-body">
              <div class="stat-label">总额度</div>
              <div class="stat-value">{{ quotaText(me.quota) }}</div>
            </div>
          </div>
        </a-card>
      </a-grid-item>
      <a-grid-item>
        <a-card class="stat-card" :bordered="false">
          <div class="stat-main">
            <span class="stat-chip is-gold"><icon-fire /></span>
            <div class="stat-body">
              <div class="stat-label">已使用</div>
              <div class="stat-value">{{ formatUsd(me.used_quota) }}</div>
            </div>
          </div>
        </a-card>
      </a-grid-item>
      <a-grid-item>
        <a-card class="stat-card" :bordered="false">
          <div class="stat-main">
            <span class="stat-chip is-green"><icon-check-circle /></span>
            <div class="stat-body">
              <div class="stat-label">剩余额度</div>
              <div class="stat-value">{{ quotaText(me.quota_remaining) }}</div>
            </div>
          </div>
          <a-progress
            v-if="quotaPct !== null"
            :percent="quotaPct / 100"
            :show-text="false"
            class="stat-progress"
          />
        </a-card>
      </a-grid-item>
      <a-grid-item>
        <a-card class="stat-card" :bordered="false">
          <div class="stat-main">
            <span class="stat-chip is-purple"><icon-user-group /></span>
            <div class="stat-body">
              <div class="stat-label">所属部门</div>
              <div class="stat-value stat-value-sm">
                {{ (me.departments ?? []).map((d) => d.Name).join('、') || '-' }}
              </div>
            </div>
          </div>
        </a-card>
      </a-grid-item>
    </a-grid>

    <a-card :bordered="false">
      <div class="table-head">
        <div class="table-title">
          模型用量
          <a-tag size="small" color="gray">{{ modelRows.length }}</a-tag>
        </div>
      </div>
      <a-table
        :data="modelRows"
        :columns="modelColumns"
        :loading="loading"
        :border="false"
        :pagination="false"
        row-key="model"
      >
        <template #requests="{ record }">{{ record.requests.toLocaleString() }}</template>
        <template #input="{ record }">{{ record.input.toLocaleString() }}</template>
        <template #output="{ record }">{{ record.output.toLocaleString() }}</template>
        <template #cost="{ record }">{{ formatUsd(record.cost) }}</template>
      </a-table>
    </a-card>

    <a-card v-if="auth.isAdmin" :bordered="false">
      <div class="table-head">
        <div class="table-title">首 Token 延迟（TTFT）</div>
      </div>
      <a-table
        :data="ttft"
        :columns="ttftColumns"
        :loading="loading"
        :border="false"
        :pagination="false"
        row-key="model"
      >
        <template #requests="{ record }">{{ record.requests.toLocaleString() }}</template>
        <template #ttft="{ record }">{{ record.avg_ttft_ms > 0 ? record.avg_ttft_ms : '-' }}</template>
        <template #total="{ record }">{{ record.avg_total_ms > 0 ? record.avg_total_ms : '-' }}</template>
      </a-table>
    </a-card>
  </div>
</template>

<style scoped>
.toolbar {
  justify-content: flex-end;
}
</style>
