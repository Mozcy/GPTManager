<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection, CopyDocument, QuestionFilled } from '@element-plus/icons-vue'
import {
  GetCodexAuthInfo,
  InjectActiveAccountToCodexProcess,
  ScanCodexAuth,
  ScanCodexProcesses,
  SetSelectedCodexProcessPIDs,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import ValuePopover from './ValuePopover.vue'

const authRows = ref([])
const processRows = ref([])
const environmentLoading = ref(false)
const processLoading = ref(false)
const injectingPID = ref(null)
const processDetailPopoverLabels = new Set(['命令行', '程序路径', '父进程命令行', '启动来源路径', '启动来源命令行', '进程链'])
let offCodexAuthUpdated = null
let offCodexProcessChanged = null
let codexProcessRefreshTimer = null

onMounted(async () => {
  offCodexAuthUpdated = EventsOn('codex-auth:updated', (info) => {
    applyCodexAuthInfo(info)
  })
  offCodexProcessChanged = EventsOn('codex-process:changed', () => {
    scheduleCodexProcessRefresh()
  })
  await loadCodexAuthInfo()
  await scanCodexProcesses(false)
})

onUnmounted(() => {
  offCodexAuthUpdated?.()
  offCodexAuthUpdated = null
  offCodexProcessChanged?.()
  offCodexProcessChanged = null
  if (codexProcessRefreshTimer) {
    clearTimeout(codexProcessRefreshTimer)
    codexProcessRefreshTimer = null
  }
})

function applyCodexAuthInfo(info) {
  authRows.value = info?.path ? [info] : []
}

async function loadCodexAuthInfo() {
  try {
    const info = await GetCodexAuthInfo()
    applyCodexAuthInfo(info)
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  }
}

async function scanCodexAuth() {
  environmentLoading.value = true
  try {
    const info = await ScanCodexAuth()
    applyCodexAuthInfo(info)
    ElMessage.success('认证扫描已完成')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    environmentLoading.value = false
  }
}

async function scanCodexProcesses(showMessage = true) {
  processLoading.value = true
  try {
    processRows.value = await ScanCodexProcesses()
    if (showMessage) {
      ElMessage.success('进程扫描已完成')
    }
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    processLoading.value = false
  }
}

function scheduleCodexProcessRefresh() {
  if (codexProcessRefreshTimer) {
    clearTimeout(codexProcessRefreshTimer)
  }
  codexProcessRefreshTimer = setTimeout(async () => {
    codexProcessRefreshTimer = null
    await scanCodexProcesses(false)
  }, 350)
}

async function handleProcessSelectionChange(selection) {
  try {
    await SetSelectedCodexProcessPIDs(selection.map((item) => item.pid))
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  }
}

function formatSubscription(value) {
  return value || '-'
}

function displayValue(value) {
  return value || '-'
}

function getSubscriptionType(value) {
  const normalized = String(value || '').toLowerCase()

  if (normalized.includes('team')) return 'team'
  if (normalized.includes('plus')) return 'plus'
  if (normalized.includes('free')) return 'free'
  return 'unknown'
}

function formatBool(value) {
  if (value === true) return 'True'
  if (value === false) return 'False'
  return '-'
}

function formatNumber(value) {
  if (value === null || value === undefined || value === '') return '-'
  return value
}

function displayLauncher(row) {
  return row?.launcherName || '未知'
}

function displayLauncherPID(row) {
  return row?.launcherPid > 0 ? row.launcherPid : ''
}

function formatLauncherConfidence(value) {
  if (value === 'high') return '高'
  if (value === 'medium') return '中'
  if (value === 'low') return '低'
  return '-'
}

function processDetailFields(row) {
  return [
    ['PID', row.pid],
    ['名称', row.name],
    ['account_id', row.accountId],
    ['邮箱', row.email],
    ['命令行', row.commandLine],
    ['程序路径', row.executablePath],
    ['所属用户', row.owner],
    ['启动时间', row.creationDate],
    ['状态', row.status],
    ['父进程 ID', row.parentPid],
    ['父进程名称', row.parentName],
    ['父进程命令行', row.parentCommandLine],
    ['启动来源', displayLauncher(row)],
    ['启动来源 PID', displayLauncherPID(row)],
    ['启动来源路径', row.launcherPath],
    ['启动来源命令行', row.launcherCommandLine],
    ['识别置信度', formatLauncherConfidence(row.launcherConfidence)],
    ['进程链', row.processTree],
    ['子进程', row.childProcesses],
    ['线程数', row.threadCount],
    ['句柄数', row.handleCount],
    ['工作集内存 MB', row.workingSetMB],
    ['虚拟内存 MB', row.virtualSizeMB],
    ['峰值工作集 MB', row.peakWorkingSetMB],
    ['共享内存 MB', row.sharedMemoryMB],
    ['数据内存 MB', row.dataMemoryMB],
    ['读操作次数', row.readCount],
    ['写操作次数', row.writeCount],
    ['读取 MB', row.readBytesMB],
    ['写入 MB', row.writeBytesMB],
    ['CPU 百分比', row.cpuPercent],
    ['CPU 总秒数', row.totalCPUSeconds],
    ['用户态 CPU 秒数', row.userModeTimeSec],
    ['内核态 CPU 秒数', row.kernelModeTimeSec],
    ['仍在运行', formatBool(row.isRunning)],
    ['前台进程', formatBool(row.foreground)],
    ['文件大小 MB', row.fileSizeMB],
    ['文件创建时间', row.fileCreated],
    ['文件修改时间', row.fileModified],
    ['文件产品名', row.fileProductName],
    ['文件产品版本', row.fileProductVersion],
    ['文件版本', row.fileVersion],
    ['文件公司', row.fileCompany],
    ['文件描述', row.fileDescription],
    ['SHA256', row.sha256],
    ['TCP 连接', row.tcpConnections],
  ]
}

function isProcessDetailPopoverField(label) {
  return processDetailPopoverLabels.has(label)
}

async function injectCodexProcess(row) {
  if (!row?.pid) {
    ElMessage.warning('PID 无效')
    return
  }

  injectingPID.value = row.pid
  try {
    await InjectActiveAccountToCodexProcess(row.pid)
    ElMessage.success('注入已完成')
  } catch (error) {
    ElMessage.error(error?.message || String(error))
  } finally {
    injectingPID.value = null
  }
}

async function copyText(value, label) {
  if (!value) {
    ElMessage.warning(`${label}为空，无法复制`)
    return
  }

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = value
      textarea.setAttribute('readonly', '')
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    ElMessage.success(`${label}已复制`)
  } catch (error) {
    ElMessage.error(`${label}复制失败`)
  }
}
</script>

<template>
  <el-card class="environment-card">
    <template #header>
      <div class="card-header">
        <span>环境管理</span>
      </div>
    </template>
    <section class="codex-auth-section">
      <div class="divider-row">
        <el-divider content-position="left">Codex Auth</el-divider>
        <el-button type="primary" size="small" :loading="environmentLoading" @click="scanCodexAuth">
          认证扫描
        </el-button>
      </div>
      <el-table :data="authRows" class="environment-table" border empty-text="暂无认证信息">
        <el-table-column prop="path" label="路径" min-width="220" show-overflow-tooltip />
        <el-table-column label="account_id" width="320" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ displayValue(row.accountId) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="邮箱" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ displayValue(row.email) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="工作空间" min-width="100" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ displayValue(row.workspaceName) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="订阅" align="center" width="90">
          <template #default="{ row }">
            <span v-if="!row.subscription">-</span>
            <el-tag v-else class="subscription-tag" :class="getSubscriptionType(row.subscription)" size="small" effect="dark">
              {{ formatSubscription(row.subscription) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="updatedAt" label="文件更新时间" min-width="125" show-overflow-tooltip />
        <el-table-column label="操作" width="72" align="center">
          <template #default="{ row }">
            <div class="operation-actions">
              <el-popover trigger="click" placement="left" width="400" popper-class="codex-auth-detail-popover">
                <template #reference>
                  <el-button class="icon-action info" size="small" text :icon="QuestionFilled" />
                </template>
                <div class="codex-auth-detail">
                  <div class="detail-title">基础信息</div>
                  <div class="detail-grid">
                    <span>路径</span><strong>{{ displayValue(row.path) }}</strong>
                    <span>Account ID</span><strong>{{ displayValue(row.accountId) }}</strong>
                    <span>邮箱</span><strong>{{ displayValue(row.email) }}</strong>
                    <span>订阅</span><strong>{{ displayValue(row.subscription) }}</strong>
                    <span>工作空间</span><strong>{{ displayValue(row.workspaceName) }}</strong>
                    <span>文件更新时间</span><strong>{{ displayValue(row.updatedAt) }}</strong>
                    <span>Auth Mode</span><strong>{{ displayValue(row.authMode) }}</strong>
                    <span>Last Refresh</span><strong>{{ displayValue(row.lastRefresh) }}</strong>
                    <span>Token Type</span><strong>{{ displayValue(row.tokenType) }}</strong>
                  </div>

                  <div class="detail-title secondary">认证信息</div>
                  <div class="detail-grid token-grid">
                    <span>access_token</span>
                    <div class="token-value">
                      <ValuePopover label="access_token" :value="row.accessToken" />
                      <el-button class="icon-action copy" size="small" text :icon="CopyDocument" title="复制 access_token"
                        @click="copyText(row.accessToken, 'access_token')" />
                    </div>
                    <span>id_token</span>
                    <div class="token-value">
                      <ValuePopover label="id_token" :value="row.idToken" />
                      <el-button class="icon-action copy" size="small" text :icon="CopyDocument" title="复制 id_token"
                        @click="copyText(row.idToken, 'id_token')" />
                    </div>
                    <span>refresh_token</span>
                    <div class="token-value">
                      <ValuePopover label="refresh_token" :value="row.refreshToken" />
                      <el-button class="icon-action copy" size="small" text :icon="CopyDocument" title="复制 refresh_token"
                        @click="copyText(row.refreshToken, 'refresh_token')" />
                    </div>
                  </div>
                </div>
              </el-popover>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="codex-process-section">
      <div class="divider-row">
        <el-divider content-position="left">Codex Process</el-divider>
        <el-button type="primary" size="small" :loading="processLoading" @click="scanCodexProcesses()">
          进程扫描
        </el-button>
      </div>
      <el-table
        :data="processRows"
        class="environment-table"
        border
        empty-text="暂无 Codex 进程"
        row-key="pid"
        @selection-change="handleProcessSelectionChange"
      >
        <el-table-column type="selection" width="42" align="center" />
        <el-table-column prop="pid" label="PID" width="90" align="center" />
        <!-- <el-table-column label="名称" width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ displayValue(row.name) }}</span>
          </template>
        </el-table-column> -->
        <el-table-column label="启动来源" min-width="100" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ displayLauncher(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="当前账号" width="320">
          <template #default="{ row }">
            <div class="process-account-cell">
              <span class="process-account-id">{{ displayValue(row.accountId) }}</span>
              <span class="process-account-email">{{ displayValue(row.email) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="程序路径" min-width="400" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ displayValue(row.executablePath) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="版本" width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ displayValue(row.fileVersion) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="92" align="center">
          <template #default="{ row }">
            <div class="operation-actions">
              <el-button
                class="icon-action inject"
                size="small"
                text
                :icon="Connection"
                title="注入"
                :loading="injectingPID === row.pid"
                :disabled="processLoading || (injectingPID !== null && injectingPID !== row.pid)"
                @click="injectCodexProcess(row)"
              />
              <el-popover trigger="click" placement="left" width="400" popper-class="codex-process-detail-popover">
                <template #reference>
                  <el-button class="icon-action info" size="small" text :icon="QuestionFilled" title="进程详情" />
                </template>
                <div class="codex-process-detail">
                  <div class="detail-title">进程详情</div>
                  <div class="detail-grid process-detail-grid">
                    <template v-for="field in processDetailFields(row)" :key="field[0]">
                      <span :class="{ 'popover-field-label': isProcessDetailPopoverField(field[0]) }">{{ field[0] }}</span>
                      <div v-if="isProcessDetailPopoverField(field[0])" class="token-value">
                        <ValuePopover :label="field[0]" :value="displayValue(formatNumber(field[1]))" />
                        <el-button class="icon-action copy" size="small" text :icon="CopyDocument" :title="`复制${field[0]}`"
                          @click="copyText(field[1], field[0])" />
                      </div>
                      <strong v-else>{{ displayValue(formatNumber(field[1])) }}</strong>
                    </template>
                  </div>
                </div>
              </el-popover>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </section>
  </el-card>
</template>

<style scoped>
.environment-card {
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

:deep(.el-card__header) {
  background: #243447;
  color: white;
  padding: 15px;
  border-bottom: 1px solid #32475b;
}

:deep(.el-card__body) {
  padding: 12px 15px;
  color: white;
}

.codex-auth-section {
  min-width: 0;
}

.codex-process-section {
  min-width: 0;
  margin-top: 18px;
}

.divider-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.divider-row :deep(.el-divider) {
  flex: 1;
  margin: 0;
  border-top-color: #3a5168;
}

.divider-row :deep(.el-divider__text) {
  background: #243447;
  color: #e8eef5;
  font-weight: 600;
}

.environment-table {
  width: 100%;
  --el-table-bg-color: #243447;
  --el-table-tr-bg-color: #243447;
  --el-table-header-bg-color: #1f2f3f;
  --el-table-header-text-color: #ffffff;
  --el-table-text-color: #ffffff;
  --el-table-row-hover-bg-color: #2d4054;
  --el-table-border-color: #32475b;
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

.process-account-cell {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 1px;
  line-height: 1.15;
  white-space: nowrap;
}

.process-account-email {
  color: #b8c8d8;
  font-size: 11px;
}

.process-account-id {
  color: #ffffff;
  font-family: inherit;
  font-size: inherit;
  font-weight: 400;
}

.subscription-tag {
  min-width: 48px;
  border-radius: 5px;
  border-color: #46586d;
  background: rgba(31, 47, 63, 0.86);
  color: #c9d6e3;
}

.subscription-tag.team {
  border-color: rgba(100, 181, 255, 0.42);
  background: rgba(64, 158, 255, 0.14);
  color: #9bd0ff;
}

.subscription-tag.plus {
  border-color: rgba(126, 217, 87, 0.42);
  background: rgba(103, 194, 58, 0.14);
  color: #a7e88a;
}

.subscription-tag.free {
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

.icon-action.info,
.icon-action.copy,
.icon-action.inject {
  color: #9bd0ff;
  --el-button-hover-text-color: #ffffff;
  --el-button-active-text-color: #ffffff;
}

.icon-action.info:hover,
.icon-action.info:focus,
.icon-action.copy:hover,
.icon-action.copy:focus,
.icon-action.inject:hover,
.icon-action.inject:focus {
  color: #ffffff;
  background: #1b2636;
}

:global(.codex-auth-detail-popover),
:global(.codex-process-detail-popover) {
  border: 1px solid #32475b !important;
  background: #243447 !important;
  color: #e8eef5 !important;
}

:global(.codex-auth-detail-popover .el-popper__arrow::before),
:global(.codex-process-detail-popover .el-popper__arrow::before) {
  border-color: #32475b !important;
  background: #243447 !important;
}

.codex-auth-detail,
.codex-process-detail {
  max-height: min(700px, 80vh);
  overflow: auto;
  padding-right: 2px;
  scrollbar-color: #4f6680 #1f2f3f;
  scrollbar-width: thin;
}

.codex-auth-detail::-webkit-scrollbar,
.codex-process-detail::-webkit-scrollbar {
  width: 8px;
}

.codex-auth-detail::-webkit-scrollbar-track,
.codex-process-detail::-webkit-scrollbar-track {
  background: #1f2f3f;
  border-radius: 999px;
}

.codex-auth-detail::-webkit-scrollbar-thumb,
.codex-process-detail::-webkit-scrollbar-thumb {
  background: #4f6680;
  border-radius: 999px;
}

.codex-auth-detail::-webkit-scrollbar-thumb:hover,
.codex-process-detail::-webkit-scrollbar-thumb:hover {
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
  grid-template-columns: 80px minmax(0, 1fr);
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

.detail-grid strong,
.detail-grid code {
  min-width: 0;
  color: #e8eef5;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.5;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.detail-grid code {
  padding: 6px 8px;
  border: 1px solid #32475b;
  border-radius: 5px;
  background: #1f2f3f;
  font-family: Consolas, 'Courier New', monospace;
  white-space: pre-wrap;
}

.token-value {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 24px;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.token-grid > span {
  align-self: center;
}

.process-detail-grid {
  grid-template-columns: 100px minmax(0, 1fr);
}

.popover-field-label {
  align-self: center;
}

.token-value code {
  min-width: 0;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.environment-table .el-table__empty-text) {
  color: #9fb0c2;
}
</style>
