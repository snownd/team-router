import { ref } from 'vue'
import type { Ref } from 'vue'

import type { Paged, PageParams } from '@/types/api'

export function usePagination<T>(
  fetcher: (p: PageParams) => Promise<Paged<T>>,
  defaultPageSize = 20,
) {
  const items = ref([]) as Ref<T[]>
  const total = ref(0)
  const loading = ref(false)
  const page = ref(1)
  const pageSize = ref(defaultPageSize)

  async function load() {
    loading.value = true
    try {
      const res = await fetcher({ page: page.value, page_size: pageSize.value })
      items.value = res.items ?? []
      total.value = res.total
    } finally {
      loading.value = false
    }
  }

  function reset() {
    page.value = 1
    return load()
  }

  function onPageChange(next: number) {
    page.value = next
    return load()
  }

  function onPageSizeChange(next: number) {
    pageSize.value = next
    page.value = 1
    return load()
  }

  return { items, total, loading, page, pageSize, load, reset, onPageChange, onPageSizeChange }
}
