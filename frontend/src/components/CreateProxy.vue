<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Delete, Setting } from '@element-plus/icons-vue'
import {
  CreateProxy,
  DeleteProxy,
  ListProxies,
  SetProxyEnabled,
  UpdateProxy,
} from '../../wailsjs/go/main/App'

const proxies = ref([])
const pageLoading = ref(false)

const createForm = reactive({
  ip: '127.0.0.1',
  port: '',
  loading: false,
})
const createDialogVisible = ref(false)
const editDialogVisible = ref(false)
const editForm = reactive({
  id: null,
  ip: '',
  port: '',
  loading: false,
})

onMounted(async () => {
  await loadProxies()
})

function normalizeProxy(row) {
  return {
    ...row,
    switchLoading: false,
    deleteLoading: false,
  }
}

function buildProxyPayload(form) {
  return {
    id: form.id || 0,
    ip: form.ip.trim(),
    port: form.port.trim(),
  }
}

function isProxyFormInvalid(form) {
  return !form.ip.trim() || !form.port.trim()
}

async function loadProxies() {
  pageLoading.value = true
  try {
    const data = await ListProxies()
    proxies.value = data.map(normalizeProxy)
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    pageLoading.value = false
  }
}

function openCreateDialog() {
  createDialogVisible.value = true
}

function resetCreateForm() {
  if (createForm.loading) return

  createForm.ip = '127.0.0.1'
  createForm.port = ''
}

function openEditDialog(row) {
  if (row.enabled) return

  editForm.id = row.id
  editForm.ip = row.ip
  editForm.port = row.port
  editDialogVisible.value = true
}

function resetEditForm() {
  if (editForm.loading) return

  editForm.id = null
  editForm.ip = ''
  editForm.port = ''
}

async function updateProxy() {
  if (!editForm.id || isProxyFormInvalid(editForm)) return

  editForm.loading = true
  try {
    const updated = await UpdateProxy(buildProxyPayload(editForm))
    const index = proxies.value.findIndex((item) => item.id === editForm.id)
    if (index >= 0) {
      proxies.value[index] = normalizeProxy(updated)
    }
    editDialogVisible.value = false
    ElMessage.success('代理已更新')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    editForm.loading = false
  }
}

async function toggleProxy(row, nextStatus) {
  row.switchLoading = true
  try {
    const updated = await SetProxyEnabled(row.id, nextStatus)
    Object.assign(row, normalizeProxy(updated))
    ElMessage.success(nextStatus ? '代理已启用' : '代理已停用')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    row.switchLoading = false
  }
}

async function deleteProxy(row) {
  if (row.enabled) return

  row.deleteLoading = true
  try {
    await DeleteProxy(row.id)
    proxies.value = proxies.value.filter((item) => item.id !== row.id)
    ElMessage.success('代理已删除')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    row.deleteLoading = false
  }
}

async function addProxy() {
  if (isProxyFormInvalid(createForm)) return

  createForm.loading = true
  try {
    const created = await CreateProxy(buildProxyPayload(createForm))
    proxies.value.unshift(normalizeProxy(created))
    createDialogVisible.value = false
    ElMessage.success('代理已创建')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    createForm.loading = false
  }
}
</script>

<template>
  <section class="proxy-section">
    <el-card class="card-proxy">
      <template #header>
        <div class="card-header">
          <span>代理配置</span>
          <el-button type="primary" size="small" @click="openCreateDialog">
            新增代理
          </el-button>
        </div>
      </template>

      <el-table v-loading="pageLoading" :data="proxies" class="proxy-table" empty-text="暂无代理配置" border>
        <el-table-column label="监听地址" min-width="220">
          <template #default="{ row }">
            <span class="proxy-text">{{ row.ip }}:{{ row.port }}</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'">
              {{ row.enabled ? '已启用' : '已停用' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="200" align="center">
          <template #default="{ row }">
            <div class="operation-cell">
              <el-switch :model-value="row.enabled" :loading="row.switchLoading"
                :disabled="row.deleteLoading || editForm.loading" @change="(value) => toggleProxy(row, value)" />
              <el-button class="icon-action settings" size="small" text :icon="Setting"
                :disabled="row.enabled || row.switchLoading || row.deleteLoading || editForm.loading"
                @click="openEditDialog(row)" />
              <el-button class="icon-action danger" size="small" text :icon="Delete" :loading="row.deleteLoading"
                :disabled="row.enabled || row.switchLoading || editForm.loading" @click="deleteProxy(row)" />
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="createDialogVisible" title="新增代理" width="420px" :close-on-click-modal="!createForm.loading"
      :close-on-press-escape="!createForm.loading" :show-close="!createForm.loading" @closed="resetCreateForm">
      <el-form class="create-form" :model="createForm" label-width="82px" @submit.prevent>
        <el-form-item label="监听 IP">
          <el-input v-model="createForm.ip" clearable :disabled="createForm.loading" />
        </el-form-item>
        <el-form-item label="监听端口">
          <el-input v-model="createForm.port" placeholder="例如 18080" clearable :disabled="createForm.loading"
            @keyup.enter="addProxy" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button size="small" :disabled="createForm.loading" @click="createDialogVisible = false">
          取消
        </el-button>
        <el-button size="small" type="primary" :loading="createForm.loading" :disabled="isProxyFormInvalid(createForm)"
          @click="addProxy">
          确定
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editDialogVisible" title="编辑代理" width="420px" :close-on-click-modal="!editForm.loading"
      :close-on-press-escape="!editForm.loading" :show-close="!editForm.loading" @closed="resetEditForm">
      <el-form class="create-form" :model="editForm" label-width="82px" @submit.prevent>
        <el-form-item label="监听 IP">
          <el-input v-model="editForm.ip" clearable :disabled="editForm.loading" />
        </el-form-item>
        <el-form-item label="监听端口">
          <el-input v-model="editForm.port" clearable :disabled="editForm.loading" @keyup.enter="updateProxy" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button size="small" :disabled="editForm.loading" @click="editDialogVisible = false">
          取消
        </el-button>
        <el-button size="small" type="primary" :loading="editForm.loading" :disabled="isProxyFormInvalid(editForm)"
          @click="updateProxy">
          确定
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.proxy-section {
  width: 100%;
  box-sizing: border-box;
}

.card-proxy {
  margin: 0 auto 16px;
  border: none;
  background-color: #243447;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.create-form {
  padding: 4px 0;
}

.create-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.create-form :deep(.el-form-item:last-child) {
  margin-bottom: 0;
}

.proxy-table {
  width: 100%;
  --el-table-bg-color: #243447;
  --el-table-tr-bg-color: #243447;
  --el-table-header-bg-color: #1f2f3f;
  --el-table-header-text-color: #ffffff;
  --el-table-text-color: #ffffff;
  --el-table-row-hover-bg-color: #2d4054;
  --el-table-border-color: #32475b;
}

.proxy-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.operation-cell {
  display: flex;
  justify-self: center;
  align-items: center;
  gap: 10px;
  min-height: 32px;
}

.operation-cell :deep(.el-switch) {
  width: 44px;
  flex: 0 0 44px;
}

.operation-cell :deep(.el-button) {
  margin-left: 0;
  flex: 0 0 24px;
}

.icon-action {
  width: 24px;
  height: 24px;
  padding: 0;
  color: #b6c3d1;
  --el-button-hover-bg-color: #1b2636;
  --el-button-active-bg-color: #1b2636;
  --el-button-disabled-bg-color: transparent;
}

.icon-action:hover,
.icon-action:focus {
  background: #2d3644 !important;
}

.icon-action.settings {
  color: #64b5ff;
  --el-button-hover-text-color: #ffffff;
  --el-button-active-text-color: #ffffff;
}

.icon-action.settings:hover,
.icon-action.settings:focus {
  color: #ffffff;
  background: #1b2636;
}

.icon-action.danger {
  color: #ff7a7a;
  --el-button-hover-text-color: #ffffff;
  --el-button-active-text-color: #ffffff;
}

.icon-action.danger:hover,
.icon-action.danger:focus {
  color: #ffffff;
  background: #1b2636;
}

.icon-action.is-disabled,
.icon-action.is-disabled:hover,
.icon-action.is-disabled:focus {
  color: #607184;
  background: transparent;
  opacity: 0.7;
}

:deep(.el-card__header) {
  background: #243447;
  color: white;
  padding: 15px;
  border-bottom: 1px solid #32475b;
}

:deep(.el-card__body) {
  padding: 0;
  color: white;
}

:deep(.el-form-item__label) {
  color: white;
}

:deep(.el-table__empty-text) {
  color: #b6c3d1;
}

:deep(.el-dialog) {
  background: #243447;
  border: 1px solid #32475b;
  border-radius: 8px;
  box-shadow: 0 16px 42px rgba(0, 0, 0, 0.34);
  overflow: hidden;
}

:deep(.el-dialog__title),
:deep(.el-dialog__body) {
  color: white;
}

:deep(.el-dialog__header) {
  border-bottom: 1px solid #32475b;
}

:deep(.el-dialog__body) {
  padding: 10px 15px;
}

:deep(.el-dialog__footer) {
  padding-top: 15px;
  border-top: 1px solid #32475b;
}

:deep(.el-dialog__footer .el-button) {
  min-width: 64px;
}

</style>
