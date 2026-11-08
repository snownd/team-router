<script setup lang="ts">
import { Message } from '@arco-design/web-vue'
import { computed, onMounted, ref } from 'vue'

import {
  createDepartment,
  deleteDepartment,
  getDepartmentTree,
  getDepartmentUsers,
  moveDepartment,
  updateDepartment,
} from '@/api/admin'
import QuotaText from '@/components/QuotaText.vue'
import StatusTag from '@/components/StatusTag.vue'
import type { AdminUser, DeptNode } from '@/types/api'
import { flattenDepts, type DeptOption } from '@/utils/dept'

const tree = ref<DeptNode[]>([])
const loading = ref(false)
const selectedKeys = ref<(string | number)[]>([])
const selectedId = computed(() =>
  selectedKeys.value.length ? Number(selectedKeys.value[0]) : null,
)
const selectedNode = computed(() => findNode(tree.value, selectedId.value))
const parentName = computed(() => {
  if (!selectedId.value) return '-'
  let name = '（根部门）'
  const walk = (nodes: DeptNode[]): boolean => {
    for (const n of nodes) {
      if (n.children?.some((c) => c.id === selectedId.value)) {
        name = n.name
        return true
      }
      if (n.children && walk(n.children)) return true
    }
    return false
  }
  walk(tree.value)
  return name
})

function findNode(nodes: DeptNode[], id: number | null): DeptNode | null {
  for (const n of nodes) {
    if (n.id === id) return n
    const found = findNode(n.children ?? [], id)
    if (found) return found
  }
  return null
}

async function load() {
  loading.value = true
  try {
    tree.value = (await getDepartmentTree()).tree
  } catch {
    // 拦截器已提示
  } finally {
    loading.value = false
  }
}

onMounted(load)

const fieldNames = { key: 'id', title: 'name', children: 'children' }

// ===== 创建 =====
const createVisible = ref(false)
const createForm = ref({ parentId: null as number | null, name: '', sortOrder: 0 })

function openCreate(parentId: number | null) {
  createForm.value = { parentId, name: '', sortOrder: 0 }
  createVisible.value = true
}

async function submitCreate(): Promise<boolean> {
  if (!createForm.value.name) {
    Message.warning('请输入部门名称')
    return false
  }
  try {
    await createDepartment({
      parent_id: createForm.value.parentId ?? undefined,
      name: createForm.value.name,
      sort_order: createForm.value.sortOrder,
    })
    Message.success('已创建')
    createVisible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

// ===== 编辑 =====
const editVisible = ref(false)
const editForm = ref({ name: '', sortOrder: 0, status: 1 })

function openEdit() {
  if (!selectedNode.value) return
  editForm.value = {
    name: selectedNode.value.name,
    sortOrder: selectedNode.value.sort_order,
    status: selectedNode.value.status,
  }
  editVisible.value = true
}

async function submitEdit(): Promise<boolean> {
  if (!selectedId.value) return false
  try {
    await updateDepartment(selectedId.value, { ...editForm.value })
    Message.success('已保存')
    editVisible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

// ===== 移动 =====
const moveVisible = ref(false)
const moveForm = ref<{ target: number | undefined }>({ target: undefined })
const allOptions = ref<DeptOption[]>([])

function openMove() {
  if (!selectedId.value) return
  allOptions.value = flattenDepts(tree.value).filter((d) => d.id !== selectedId.value)
  moveForm.value = { target: undefined }
  moveVisible.value = true
}

async function submitMove(): Promise<boolean> {
  if (!selectedId.value || moveForm.value.target == null) {
    Message.warning('请选择目标父部门')
    return false
  }
  try {
    await moveDepartment(selectedId.value, moveForm.value.target)
    Message.success('已移动')
    moveVisible.value = false
    await load()
    return true
  } catch {
    return false
  }
}

async function removeDept() {
  if (!selectedId.value) return
  try {
    await deleteDepartment(selectedId.value)
    Message.success('已删除')
    selectedKeys.value = []
    await load()
  } catch {
    // 拦截器已提示
  }
}

// ===== 成员 =====
const membersVisible = ref(false)
const membersLoading = ref(false)
const members = ref<AdminUser[]>([])
const includeSubtree = ref(false)

const memberColumns = [
  { title: 'ID', dataIndex: 'ID', width: 70 },
  { title: '用户名', dataIndex: 'Username' },
  { title: '姓名', slotName: 'displayName' },
  { title: '角色', slotName: 'role', width: 90 },
  { title: '状态', slotName: 'status', width: 90 },
  { title: '已用额度', slotName: 'used', width: 110 },
]

async function loadMembers() {
  if (!selectedId.value) return
  membersLoading.value = true
  try {
    members.value = (await getDepartmentUsers(selectedId.value, includeSubtree.value)).items
  } catch {
    // 拦截器已提示
  } finally {
    membersLoading.value = false
  }
}

function openMembers() {
  if (!selectedId.value) return
  membersVisible.value = true
  loadMembers()
}
</script>

<template>
  <div class="page">
    <div class="dept-grid">
      <a-card :bordered="false" class="dept-tree-card">
        <div class="table-head">
          <div class="table-title">部门树</div>
          <a-space size="mini">
            <a-tooltip content="新建根部门">
              <a-button type="text" @click="openCreate(null)">
                <template #icon><icon-plus /></template>
              </a-button>
            </a-tooltip>
            <a-tooltip content="刷新">
              <a-button type="text" @click="load">
                <template #icon><icon-refresh /></template>
              </a-button>
            </a-tooltip>
          </a-space>
        </div>
        <a-spin :loading="loading" class="tree-spin">
          <a-empty v-if="!tree.length" description="暂无部门，点击「新建根部门」创建" />
          <a-tree
            v-else
            :data="tree"
            :field-names="fieldNames"
            :selected-keys="selectedKeys"
            block-node
            default-expand-all
            @select="selectedKeys = $event"
          />
        </a-spin>
      </a-card>

      <a-card :bordered="false">
        <template v-if="selectedNode">
          <div class="table-head">
            <div class="table-title">{{ selectedNode.name }}</div>
            <StatusTag :status="selectedNode.status" />
          </div>
          <a-descriptions :column="2" size="small" class="dept-desc">
            <a-descriptions-item label="部门 ID">{{ selectedNode.id }}</a-descriptions-item>
            <a-descriptions-item label="排序值">{{ selectedNode.sort_order }}</a-descriptions-item>
            <a-descriptions-item label="父部门">{{ parentName }}</a-descriptions-item>
            <a-descriptions-item label="直属子部门">
              {{ selectedNode.children?.length ?? 0 }}
            </a-descriptions-item>
          </a-descriptions>
          <a-space wrap>
            <a-button type="primary" @click="openCreate(selectedId)">
              <template #icon><icon-plus /></template>
              新建子部门
            </a-button>
            <a-button @click="openEdit">
              <template #icon><icon-edit /></template>
              编辑
            </a-button>
            <a-button @click="openMove">
              <template #icon><icon-swap /></template>
              移动
            </a-button>
            <a-button @click="openMembers">
              <template #icon><icon-user-group /></template>
              成员
            </a-button>
            <a-popconfirm
              type="warning"
              content="将删除该部门及其整棵子树与成员关联，确认？"
              @ok="removeDept"
            >
              <a-button status="danger">
                <template #icon><icon-delete /></template>
                删除
              </a-button>
            </a-popconfirm>
          </a-space>
        </template>
        <a-empty v-else description="在左侧选择部门查看详情" />
      </a-card>
    </div>

    <a-modal v-model:visible="createVisible" title="创建部门" @before-ok="submitCreate">
      <a-form :model="createForm" layout="vertical">
        <a-form-item label="父部门">
          <a-input :model-value="createForm.parentId ? selectedNode?.name : '（根部门）'" disabled />
        </a-form-item>
        <a-form-item label="部门名称" required>
          <a-input v-model="createForm.name" />
        </a-form-item>
        <a-form-item label="排序值">
          <a-input-number v-model="createForm.sortOrder" :precision="0" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="editVisible" title="编辑部门" @before-ok="submitEdit">
      <a-form :model="editForm" layout="vertical">
        <a-form-item label="部门名称" required>
          <a-input v-model="editForm.name" />
        </a-form-item>
        <a-form-item label="排序值">
          <a-input-number v-model="editForm.sortOrder" :precision="0" />
        </a-form-item>
        <a-form-item label="状态">
          <a-switch v-model="editForm.status" :checked-value="1" :unchecked-value="2">
            <template #checked>启用</template>
            <template #unchecked>禁用</template>
          </a-switch>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:visible="moveVisible" title="移动部门" @before-ok="submitMove">
      <a-form :model="moveForm" layout="vertical">
        <a-form-item label="目标父部门" required help="移动到顶层请先移到某父部门下">
          <a-select
            v-model="moveForm.target"
            :options="allOptions.map((d) => ({ label: d.label, value: d.id }))"
            placeholder="选择目标父部门"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-drawer
      v-model:visible="membersVisible"
      :title="`成员 · ${selectedNode?.name ?? ''}`"
      width="720px"
    >
      <a-space class="member-toolbar">
        <a-checkbox v-model="includeSubtree" @change="loadMembers">包含子部门成员</a-checkbox>
      </a-space>
      <a-table
        :data="members"
        :columns="memberColumns"
        :loading="membersLoading"
        :border="false"
        row-key="ID"
        :pagination="false"
      >
        <template #displayName="{ record }">{{ record.DisplayName || '-' }}</template>
        <template #role="{ record }">
          <a-tag :color="record.Role === 'admin' ? 'cyan' : 'gray'" size="small">
            {{ record.Role === 'admin' ? '管理员' : '成员' }}
          </a-tag>
        </template>
        <template #status="{ record }">
          <StatusTag :status="record.Status" />
        </template>
        <template #used="{ record }">
          <QuotaText :value="record.UsedQuota" />
        </template>
      </a-table>
    </a-drawer>
  </div>
</template>

<style scoped>
.dept-grid {
  display: grid;
  grid-template-columns: 340px minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.tree-spin {
  display: block;
}

.dept-desc {
  margin-bottom: 16px;
}

.member-toolbar {
  margin-bottom: 12px;
}

@media (max-width: 900px) {
  .dept-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
