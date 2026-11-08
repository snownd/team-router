<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { getMe, getMyStats } from '@/api/user'
import type { Me, UsageStat } from '@/types/api'
import { defaultRange, formatDate } from '@/utils/format'
import { formatUsd, isUnlimitedQuota, quotaText } from '@/utils/quota'

const range = ref<[string, string]>(defaultRange())
const loading = ref(false)
const me = ref<Me | null>(null)
const stats = ref<UsageStat[]>([])

const quotaPct = computed(() => {
  if (!me.value || isUnlimitedQuota(me.value.quota) || me.value.quota <= 0) return null
  if (me.value.used_quota <= 0) return 0
  return Math.min(100, Math.round((me.value.used_quota / me.value.quota) * 100))
})

const columns = [
  { title: '日期', slotName: 'date', width: 120 },
  { title: '模型', dataIndex: 'model' },
  { title: '请求数', slotName: 'requests', width: 100, align: 'right' as const },
  { title: '输入 Tokens', slotName: 'input', width: 120, align: 'right' as const },
  { title: '输出 Tokens', slotName: 'output', width: 120, align: 'right' as const },
  { title: '缓存读', slotName: 'cacheRead', width: 110, align: 'right' as const },
  { title: '缓存写', slotName: 'cacheWrite', width: 110, align: 'right' as const },
  { title: '费用', slotName: 'cost', width: 120, align: 'right' as const },
]

async function load() {
  loading.value = true
  try {
    const [start, end] = range.value
    const [m, s] = await Promise.all([getMe(), getMyStats({ start, end })])
    me.value = m
    stats.value = s.items ?? []
  } catch {
    // 拦截器已提示
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <a-grid v-if="me" :cols="{ xs: 1, sm: 2, md: 4 }" :col-gap="16">
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
          用量明细
          <a-tag size="small" color="gray">{{ stats.length }}</a-tag>
        </div>
        <a-space>
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
      </div>
      <a-table
        :data="stats"
        :columns="columns"
        :loading="loading"
        :border="false"
        row-key="model"
        :pagination="false"
      >
        <template #date="{ record }">{{ formatDate(record.bucket) }}</template>
        <template #requests="{ record }">{{ record.requests.toLocaleString() }}</template>
        <template #input="{ record }">{{ record.input_tokens.toLocaleString() }}</template>
        <template #output="{ record }">{{ record.output_tokens.toLocaleString() }}</template>
        <template #cacheRead="{ record }">{{ record.cache_read_tokens.toLocaleString() }}</template>
        <template #cacheWrite="{ record }">{{ record.cache_write_tokens.toLocaleString() }}</template>
        <template #cost="{ record }">{{ formatUsd(record.quota_cost) }}</template>
      </a-table>
    </a-card>
  </div>
</template>
