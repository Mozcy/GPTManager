<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import dayjs from 'dayjs'
import { ElMessage } from 'element-plus'
import { Delete, QuestionFilled, Refresh } from '@element-plus/icons-vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  ActivateAccount,
  DeleteAccount,
  ListAccounts,
  RefreshAccountUsage,
  StartOpenAIAuth,
} from '../../wailsjs/go/main/App'

const accounts = ref([])
const accountLoading = ref(false)
const authLoading = ref(false)
const accountRefreshing = ref(false)
const accountActivating = ref(false)
const selectedAccountId = ref(null)

onMounted(async () => {
  await loadAccounts()
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

async function loadAccounts() {
  accountLoading.value = true
  try {
    accounts.value = await ListAccounts()
    const activeAccount = accounts.value.find((item) => item.active)
    if (activeAccount) {
      selectedAccountId.value = activeAccount.id
    } else if (selectedAccountId.value && !accounts.value.some((item) => item.id === selectedAccountId.value)) {
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
    syncActiveAccount(account)
    return
  }
  accounts.value.unshift(account)
  syncActiveAccount(account)
}

function syncActiveAccount(account) {
  if (!account?.active || !account.id) return

  selectedAccountId.value = account.id
  accounts.value = accounts.value.map((item) => ({
    ...item,
    active: item.id === account.id,
  }))
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

function formatUsageSeconds(value) {
  const seconds = Number(value)
  if (!Number.isFinite(seconds) || seconds <= 0) return '-'

  const total = Math.floor(seconds)
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const remainSeconds = total % 60
  const parts = []

  if (days > 0) parts.push(`${days}天`)
  if (hours > 0) parts.push(`${hours}小时`)
  if (minutes > 0) parts.push(`${minutes}分`)
  if (remainSeconds > 0 || parts.length === 0) parts.push(`${remainSeconds}秒`)

  return parts.join(' ')
}

</script>

<template>
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
      <el-table-column label="活动" width="60" align="center">
        <template #default="{ row }">
          <el-radio v-model="selectedAccountId" class="account-radio" :label="row.id" :disabled="accountActivating" />
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
      <el-table-column label="5/hours" width="80" align="center">
        <template #default="{ row }">
          <span class="proxy-text">{{ formatRemainingQuota(row.primaryWindow) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="刷新时间(5h)" width="160">
        <template #default="{ row }">
          <span class="proxy-text">{{ formatUsageResetTime(row.primaryWindow) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="7/day" width="80" align="center">
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
      <el-table-column label="操作" width="112" align="center">
        <template #default="{ row }">
          <div class="operation-actions">
            <el-button class="icon-action danger" size="small" text :icon="Delete" @click="deleteAccount(row)" />
            <el-popover trigger="click" placement="left" width="440" popper-class="account-detail-popover">
              <template #reference>
                <el-button class="icon-action info" size="small" text :icon="QuestionFilled" />
              </template>
              <div class="account-detail">
                <div class="detail-title">账号详情</div>
                <div class="detail-grid">
                  <span>ID</span><strong>{{ row.id || '-' }}</strong>
                  <span>名称</span><strong>{{ row.name || '-' }}</strong>
                  <span>订阅</span><strong>{{ formatSubscription(row.subscription) }}</strong>
                  <span>邮箱</span><strong>{{ row.email || '-' }}</strong>
                  <span>Subject</span><strong>{{ row.subject || '-' }}</strong>
                  <span>User ID</span><strong>{{ row.userId || '-' }}</strong>
                  <span>Account ID</span><strong>{{ row.accountId || '-' }}</strong>
                  <span>订阅过期</span><strong>{{ formatDateTime(row.subscriptionExpiresAt) }}</strong>
                  <span>Token过期</span><strong>{{ formatDateTime(row.expiresAt) }}</strong>
                  <span>更新时间</span><strong>{{ formatDateTime(row.updatedAt) }}</strong>
                </div>

                <div class="detail-title secondary">5小时额度</div>
                <div class="detail-grid">
                  <span>剩余额度</span><strong>{{ formatRemainingQuota(row.primaryWindow) }}</strong>
                  <span>窗口秒数</span><strong>{{ formatUsageSeconds(row.primaryWindow?.limitWindowSeconds) }}</strong>
                  <span>重置剩余</span><strong>{{ formatUsageSeconds(row.primaryWindow?.resetAfterSeconds) }}</strong>
                  <span>重置时间</span><strong>{{ formatUsageResetTime(row.primaryWindow) }}</strong>
                </div>

                <div class="detail-title secondary">7天额度</div>
                <div class="detail-grid">
                  <span>剩余额度</span><strong>{{ formatRemainingQuota(row.secondaryWindow) }}</strong>
                  <span>窗口秒数</span><strong>{{ formatUsageSeconds(row.secondaryWindow?.limitWindowSeconds) }}</strong>
                  <span>重置剩余</span><strong>{{ formatUsageSeconds(row.secondaryWindow?.resetAfterSeconds) }}</strong>
                  <span>重置时间</span><strong>{{ formatUsageResetTime(row.secondaryWindow) }}</strong>
                </div>
              </div>
            </el-popover>
          </div>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<style scoped>
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

.operation-actions {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
}

.operation-actions :deep(.el-button + .el-button) {
  margin-left: 0;
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

.icon-action.info {
  color: #9bd0ff;
  --el-button-hover-text-color: #ffffff;
  --el-button-active-text-color: #ffffff;
}

.icon-action.info:hover,
.icon-action.info:focus {
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

:deep(.el-table__empty-text) {
  color: #b6c3d1;
}

:global(.account-detail-popover) {
  border: 1px solid #32475b !important;
  background: #243447 !important;
  color: #e8eef5 !important;
}

:global(.account-detail-popover .el-popper__arrow::before) {
  border-color: #32475b !important;
  background: #243447 !important;
}

.account-detail {
  max-height: min(700px, 80vh);
  overflow: auto;
  padding-right: 2px;
  scrollbar-color: #4f6680 #1f2f3f;
  scrollbar-width: thin;
}

.account-detail::-webkit-scrollbar {
  width: 8px;
}

.account-detail::-webkit-scrollbar-track {
  background: #1f2f3f;
  border-radius: 999px;
}

.account-detail::-webkit-scrollbar-thumb {
  background: #4f6680;
  border-radius: 999px;
}

.account-detail::-webkit-scrollbar-thumb:hover {
  background: #66809b;
}

.detail-title {
  margin-bottom: 10px;
  color: #ffffff;
  font-size: 14px;
  font-weight: 700;
}

.detail-title.secondary {
  margin-top: 16px;
}

.detail-grid {
  display: grid;
  grid-template-columns: 96px minmax(0, 1fr);
  gap: 8px 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid #32475b;
}

.detail-grid:last-child {
  padding-bottom: 0;
  border-bottom: none;
}

.detail-grid span {
  color: #94a8bd;
  font-size: 12px;
  line-height: 1.5;
}

.detail-grid strong {
  min-width: 0;
  color: #e8eef5;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.5;
  overflow-wrap: anywhere;
  word-break: break-word;
}
</style>
