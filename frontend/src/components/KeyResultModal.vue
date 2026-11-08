<script setup lang="ts">
import type { CreatedKey } from '@/types/api'

defineProps<{ keyData: CreatedKey | null }>()
const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <a-modal
    :visible="!!keyData"
    title="Key 创建成功"
    :footer="false"
    :mask-closable="false"
    @cancel="emit('close')"
  >
    <a-alert type="warning">
      {{ keyData?.notice || '请妥善保存，密钥明文不会再次显示' }}
    </a-alert>
    <a-typography-paragraph class="key-text" copyable :copy-text="keyData?.key">
      {{ keyData?.key }}
    </a-typography-paragraph>
  </a-modal>
</template>

<style scoped>
.key-text {
  margin-top: 12px;
  word-break: break-all;
}
</style>
