<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Setting } from '@element-plus/icons-vue'
import {
  CheckUpstreamStatus,
  GetUpstreamConfig,
  SaveUpstreamConfig,
} from '../../wailsjs/go/main/App'

const upstreamTypes = [
  { label: 'HTTP', value: 'http' },
  { label: 'SOCKS5', value: 'socks5' },
]

const upstreamLoading = ref(false)
const upstreamChecking = ref(false)
const upstreamDialogVisible = ref(false)
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

const statusText = computed(() => {
  if (!upstreamStatus.checked) return '未检查'
  return upstreamStatus.connected ? '已连接' : '未连接'
})

const upstreamAddress = computed(() => {
  return `${savedUpstream.type}://${savedUpstream.ip}:${savedUpstream.port}`
})

onMounted(async () => {
  await loadUpstreamConfig()
  await checkUpstreamStatus()
})

function buildUpstreamPayload() {
  return {
    type: upstreamForm.type,
    ip: upstreamForm.ip.trim(),
    port: upstreamForm.port.trim(),
  }
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
</script>

<template>
  <footer class="upstream-status-bar">
    <div class="upstream-summary">
      <span class="status-indicator" :class="{
        connected: upstreamStatus.checked && upstreamStatus.connected,
        disconnected: upstreamStatus.checked && !upstreamStatus.connected,
      }" :title="upstreamStatus.message">
        <span class="status-dot">●</span>
        {{ statusText }}
      </span>
      <span class="upstream-address">{{ upstreamAddress }}</span>
    </div>
    <el-button class="icon-action settings" size="small" text :icon="Setting" @click="openUpstreamDialog" />
  </footer>

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
</template>

<style scoped>
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

.upstream-address {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #d9e4ef;
  font-size: 13px;
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

:deep(.el-form-item__label) {
  color: white;
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

@media (max-width: 520px) {
  .upstream-status-bar {
    gap: 6px;
    padding: 0 8px;
  }
}
</style>
