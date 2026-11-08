import type { DeptNode } from '@/types/api'

export interface DeptOption {
  id: number
  label: string
}

export function flattenDepts(nodes: DeptNode[], prefix = ''): DeptOption[] {
  return nodes.flatMap((n) => [
    { id: n.id, label: prefix + n.name },
    ...flattenDepts(n.children ?? [], `${prefix}${n.name} / `),
  ])
}
