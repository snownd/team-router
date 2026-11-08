<script setup lang="ts">
import { computed } from 'vue'

import { quotaText } from '@/utils/quota'

const props = defineProps<{ value: number }>()

const text = computed(() => quotaText(props.value))
</script>

<template>
  <span>{{ text }}</span>
</template>
