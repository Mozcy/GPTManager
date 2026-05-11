<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import dayjs from 'dayjs'
import { ElMessage } from 'element-plus'
import { Delete, Refresh, Setting } from '@element-plus/icons-vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  ActivateAccount,
  CheckUpstreamStatus,
  CreateProxy,
  DeleteAccount,
  DeleteProxy,
  GetUpstreamConfig,
  ListAccounts,
  ListProxies,
  RefreshAccountUsage,
  SaveUpstreamConfig,
  SetProxyEnabled,
  StartOpenAIAuth,
  UpdateProxy,
} from '../../wailsjs/go/main/App'

const upstreamTypes = [
  { label: 'HTTP', value: 'http' },
  { label: 'SOCKS5', value: 'socks5' },
]

const proxies = ref([])
const accounts = ref([])
const pageLoading = ref(false)
const accountLoading = ref(false)
const authLoading = ref(false)
const accountRefreshing = ref(false)
const accountActivating = ref(false)
const selectedAccountId = ref(null)
const upstreamLoading = ref(false)
const upstreamChecking = ref(false)
const upstreamStatus = reactive({
  checked: false,
  connected: false,
  message: '',
})

const savedUpstream = reactive({
  type: 'http',
  ip: '127.0.0.1',
  port: '1080',
})

const upstreamForm = reactive({
  type: 'http',
  ip: '127.0.0.1',
  port: '1080',
})
const upstreamDialogVisible = ref(false)

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

const upstreamStatusText = computed(() => {
  if (!upstreamStatus.checked) return '未检查'
  return upstreamStatus.connected ? '已连接' : '未连接'
})

const upstreamAddress = computed(() => {
  return `${savedUpstream.type}://${savedUpstream.ip}:${savedUpstream.port}`
})

onMounted(async () => {
  await Promise.all([loadUpstreamConfig(), loadProxies(), loadAccounts()])
  await checkUpstreamStatus()
})

const offAuthSuccess = EventsOn('account:auth-success', async () => {
  await loadAccounts()
  ElMessage.success('账号已添加')
})

const offAuthError = EventsOn('account:auth-error', (message) => {
  ElMessage.error(message || '账号授权失败')
})

const offUsageUpdated = EventsOn('account:usage-updated', (account) => {
  upsertAccount(account)
})

const offUsageError = EventsOn('account:usage-error', (payload) => {
  console.warn('账号额度刷新失败', payload)
})

onUnmounted(() => {
  offAuthSuccess()
  offAuthError()
  offUsageUpdated()
  offUsageError()
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

function buildUpstreamPayload() {
  return {
    type: upstreamForm.type,
    ip: upstreamForm.ip.trim(),
    port: upstreamForm.port.trim(),
  }
}

function isProxyFormInvalid(form) {
  return !form.ip.trim() || !form.port.trim()
}

function isUpstreamFormInvalid() {
  return !upstreamForm.ip.trim() || !upstreamForm.port.trim()
}

function applyUpstreamStatus(status) {
  upstreamStatus.checked = true
  upstreamStatus.connected = !!status.connected
  upstreamStatus.message = status.message || ''
}

function setSavedUpstream(config) {
  savedUpstream.type = config.type || 'http'
  savedUpstream.ip = config.ip || '127.0.0.1'
  savedUpstream.port = config.port || '1080'
}

function syncUpstreamForm() {
  upstreamForm.type = savedUpstream.type
  upstreamForm.ip = savedUpstream.ip
  upstreamForm.port = savedUpstream.port
}

async function loadUpstreamConfig() {
  upstreamLoading.value = true
  try {
    const config = await GetUpstreamConfig()
    setSavedUpstream(config)
    syncUpstreamForm()
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    upstreamLoading.value = false
  }
}

async function saveUpstreamConfig() {
  if (isUpstreamFormInvalid()) return

  upstreamLoading.value = true
  try {
    const config = await SaveUpstreamConfig(buildUpstreamPayload())
    setSavedUpstream(config)
    syncUpstreamForm()
    upstreamDialogVisible.value = false
    ElMessage.success('二次代理配置已保存')
    await checkUpstreamStatus()
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    upstreamLoading.value = false
  }
}

function openUpstreamDialog() {
  syncUpstreamForm()
  upstreamDialogVisible.value = true
}

async function checkUpstreamStatus() {
  upstreamChecking.value = true
  try {
    const status = await CheckUpstreamStatus()
    applyUpstreamStatus(status)
  } catch (error) {
    applyUpstreamStatus({ connected: false, message: error?.message || String(error) })
  } finally {
    upstreamChecking.value = false
  }
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

async function loadAccounts() {
  accountLoading.value = true
  try {
    accounts.value = await ListAccounts()
    if (selectedAccountId.value && !accounts.value.some((item) => item.id === selectedAccountId.value)) {
      selectedAccountId.value = null
    }
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    accountLoading.value = false
  }
}

function upsertAccount(account) {
  if (!account) return

  const index = accounts.value.findIndex((item) => {
    return (account.id && item.id === account.id) || (account.accountId && item.accountId === account.accountId)
  })
  if (index >= 0) {
    accounts.value[index] = {
      ...accounts.value[index],
      ...account,
    }
    return
  }
  accounts.value.unshift(account)
}

async function addAccount() {
  authLoading.value = true
  try {
    await StartOpenAIAuth()
    ElMessage.info('已打开浏览器，请完成授权')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    authLoading.value = false
  }
}

async function refreshAccountUsage() {
  accountRefreshing.value = true
  try {
    await RefreshAccountUsage()
    ElMessage.success('账号额度已刷新')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    accountRefreshing.value = false
  }
}

async function activateAccount() {
  if (!selectedAccountId.value) {
    ElMessage.warning('请选择账号')
    return
  }

  accountActivating.value = true
  try {
    const account = await ActivateAccount(selectedAccountId.value)
    upsertAccount(account)
    ElMessage.success('账号已激活')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    accountActivating.value = false
  }
}

async function deleteAccount(row) {
  try {
    await DeleteAccount(row.id)
    accounts.value = accounts.value.filter((item) => item.id !== row.id)
    if (selectedAccountId.value === row.id) {
      selectedAccountId.value = null
    }
    ElMessage.success('账号已删除')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  }
}

function formatDateTime(value) {
  if (!value) return '-'

  const parsed = dayjs(value)
  return parsed.isValid() ? parsed.format('YYYY-MM-DD HH:mm:ss') : value
}

function formatSubscription(value) {
  return value || '-'
}

function getSubscriptionType(value) {
  const normalized = String(value || '').toLowerCase()

  if (normalized.includes('team')) return 'team'
  if (normalized.includes('plus')) return 'plus'
  if (normalized.includes('free')) return 'free'
  return 'unknown'
}

function hasUsageWindow(window) {
  return !!window && (window.resetAt > 0 || window.limitWindowSeconds > 0)
}

function formatRemainingQuota(window) {
  if (!hasUsageWindow(window)) return '-'

  const used = Number(window.usedPercent)
  if (!Number.isFinite(used)) return '-'
  return `${Math.max(0, 100 - used)}%`
}

function formatUsageResetTime(window) {
  if (!hasUsageWindow(window) || !window.resetAt) return '-'

  return dayjs.unix(window.resetAt).format('YYYY-MM-DD HH:mm:ss')
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
  <main class="proxy-page">
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

    <el-card class="card-proxy">
      <template #header>
        <div class="card-header">
          <span>账号管理</span>
          <div class="header-actions">
            <el-button class="icon-action settings" size="small" text :icon="Refresh" :loading="accountRefreshing"
              :disabled="authLoading || accountActivating" title="刷新账号额度" @click="refreshAccountUsage" />
            <el-button type="success" size="small" :loading="accountActivating"
              :disabled="!selectedAccountId || authLoading || accountRefreshing" @click="activateAccount">
              激活账号
            </el-button>
            <el-button type="primary" size="small" :loading="authLoading" :disabled="accountRefreshing || accountActivating"
              @click="addAccount">
              添加账号
            </el-button>
          </div>
        </div>
      </template>

      <el-table v-loading="accountLoading" :data="accounts" class="proxy-table" empty-text="暂无账号" border>
        <el-table-column type="index" label="序号" width="70" align="center" />
        <el-table-column label="" width="52" align="center">
          <template #default="{ row }">
            <el-radio v-model="selectedAccountId" class="account-radio" :label="row.id"
              :disabled="accountActivating" />
          </template>
        </el-table-column>
        <el-table-column label="account_id" width="310">
          <template #default="{ row }">
            <span class="proxy-text">{{ row.accountId || '未知' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="邮箱" min-width="220">
          <template #default="{ row }">
            <span class="proxy-text">{{ row.email || '未知' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="5/hours" width="100" align="center">
          <template #default="{ row }">
            <span class="proxy-text">{{ formatRemainingQuota(row.primaryWindow) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="刷新时间(5h)" width="160">
          <template #default="{ row }">
            <span class="proxy-text">{{ formatUsageResetTime(row.primaryWindow) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="7/day" width="100" align="center">
          <template #default="{ row }">
            <span class="proxy-text">{{ formatRemainingQuota(row.secondaryWindow) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="刷新时间(7d)" width="160">
          <template #default="{ row }">
            <span class="proxy-text">{{ formatUsageResetTime(row.secondaryWindow) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="订阅" width="80" align="center">
          <template #default="{ row }">
            <span class="subscription-badge" :class="getSubscriptionType(row.subscription)">
              {{ formatSubscription(row.subscription) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="过期时间" width="160">
          <template #default="{ row }">
            <span class="proxy-text">{{ formatDateTime(row.subscriptionExpiresAt || row.expiresAt) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80" align="center">
          <template #default="{ row }">
            <el-button class="icon-action danger" size="small" text :icon="Delete" @click="deleteAccount(row)" />
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

    <el-dialog v-model="upstreamDialogVisible" title="二次代理设置" width="420px" :close-on-click-modal="!upstreamLoading"
      :close-on-press-escape="!upstreamLoading" :show-close="!upstreamLoading">
      <el-form class="create-form" :model="upstreamForm" label-width="82px" @submit.prevent>
        <el-form-item label="协议">
          <el-select v-model="upstreamForm.type" class="form-select" :disabled="upstreamLoading">
            <el-option v-for="item in upstreamTypes" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="IP">
          <el-input v-model="upstreamForm.ip" clearable :disabled="upstreamLoading" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input v-model="upstreamForm.port" clearable :disabled="upstreamLoading"
            @keyup.enter="saveUpstreamConfig" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button size="small" :loading="upstreamChecking" :disabled="upstreamLoading" @click="checkUpstreamStatus">
          检查状态
        </el-button>
        <el-button size="small" :disabled="upstreamLoading" @click="upstreamDialogVisible = false">
          取消
        </el-button>
        <el-button size="small" type="primary" :loading="upstreamLoading" :disabled="isUpstreamFormInvalid()"
          @click="saveUpstreamConfig">
          保存
        </el-button>
      </template>
    </el-dialog>

    <footer class="upstream-status-bar">
      <div class="upstream-summary">
        <span class="status-indicator" :class="{
          connected: upstreamStatus.checked && upstreamStatus.connected,
          disconnected: upstreamStatus.checked && !upstreamStatus.connected,
        }" :title="upstreamStatus.message">
          <span class="status-dot">●</span>
          {{ upstreamStatusText }}
        </span>
        <span class="upstream-address">{{ upstreamAddress }}</span>
      </div>
      <el-button class="icon-action settings" size="small" text :icon="Setting" @click="openUpstreamDialog" />
    </footer>
  </main>
</template>

<style scoped>
.proxy-page {
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

.header-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.status-indicator {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #b6c3d1;
  font-size: 13px;
}

.status-indicator.connected {
  color: #67c23a;
}

.status-indicator.disconnected {
  color: #f56c6c;
}

.status-dot {
  font-size: 15px;
  line-height: 1;
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

.form-select {
  width: 100%;
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

.account-radio {
  height: 24px;
  margin-right: 0;
}

.account-radio :deep(.el-radio__label) {
  display: none;
}

.account-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.account-name,
.account-email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-name {
  color: #ffffff;
}

.account-email {
  color: #b6c3d1;
  font-size: 12px;
}

.subscription-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 48px;
  height: 22px;
  padding: 0 9px;
  box-sizing: border-box;
  border: 1px solid #46586d;
  border-radius: 5px;
  background: rgba(31, 47, 63, 0.86);
  color: #c9d6e3;
  font-size: 12px;
  line-height: 1;
}

.subscription-badge.team {
  border-color: rgba(100, 181, 255, 0.42);
  background: rgba(64, 158, 255, 0.14);
  color: #9bd0ff;
}

.subscription-badge.plus {
  border-color: rgba(126, 217, 87, 0.42);
  background: rgba(103, 194, 58, 0.14);
  color: #a7e88a;
}

.subscription-badge.free {
  border-color: rgba(148, 163, 184, 0.36);
  background: rgba(96, 113, 132, 0.16);
  color: #b6c3d1;
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

.upstream-status-bar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 46px;
  padding: 0 14px;
  box-sizing: border-box;
  background: #1f2f3f;
  border-top: 1px solid #32475b;
  color: white;
}

.upstream-summary {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1 1 auto;
}

.upstream-address {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #d9e4ef;
  font-size: 13px;
}

@media (max-width: 520px) {
  .upstream-status-bar {
    gap: 6px;
    padding: 0 8px;
  }
}
</style>
