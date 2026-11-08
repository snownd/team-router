<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ status: number | undefined }>()

const cfg = computed(() =>
  props.status === 1 ? { color: 'green', text: '启用' } : { color: 'red', text: '禁用' },
)
</script>

<template>
  <a-tag :color="cfg.color">{{ cfg.text }}</a-tag>
</template>
