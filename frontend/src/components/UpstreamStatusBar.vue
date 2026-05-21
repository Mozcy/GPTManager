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
    ElMessage.success('代理配置已保存')
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
    <el-button class="icon-action settings" size="small" text :icon="Setting" title="代理配置"
      @click="openUpstreamDialog" />
  </footer>

  <div class="upstream-dialog-host">
    <el-dialog v-model="upstreamDialogVisible" class="upstream-dialog" draggable title="代理设置" width="380"
      :append-to-body="false" :close-on-click-modal="!upstreamLoading"
      :close-on-press-escape="!upstreamLoading" :show-close="!upstreamLoading">
      <el-form class="upstream-form" :model="upstreamForm" label-width="60px" @submit.prevent>
        <el-form-item label="协议">
          <el-select v-model="upstreamForm.type" popper-class="upstream-select-popper" :disabled="upstreamLoading"
            :teleported="false">
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
        <el-button type="primary" size="small" :loading="upstreamLoading" :disabled="isUpstreamFormInvalid()"
          @click="saveUpstreamConfig">
          保存
        </el-button>
      </template>
    </el-dialog>
  </div>
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
  font-size: 0.9rem;
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
}

.upstream-dialog-host :deep(.upstream-dialog) {
  --el-dialog-bg-color: #243447 !important;
  --el-dialog-title-font-size: 16px;
  border: 1px solid #32475b;
}

.upstream-dialog-host :deep(.upstream-dialog .el-dialog__header) {
  border-bottom: 1px solid #32475b !important;
}

.upstream-dialog-host :deep(.upstream-dialog .el-dialog__title),
.upstream-dialog-host :deep(.upstream-dialog .el-dialog__body) {
  color: #ffffff;
}

.upstream-dialog-host :deep(.upstream-dialog .el-dialog__footer) {
  border-top: 1px solid #32475b;
}

.upstream-dialog-host :deep(.upstream-dialog .el-form-item__label) {
  color: #d9e4ef;
}

.upstream-dialog-host :deep(.upstream-dialog .el-button--primary) {
  --el-button-bg-color: #2f8ee8;
  --el-button-border-color: #2f8ee8;
  --el-button-hover-bg-color: #409eff;
  --el-button-hover-border-color: #409eff;
  --el-button-active-bg-color: #1f73c9;
  --el-button-active-border-color: #1f73c9;
  --el-button-disabled-bg-color: #2d4054;
  --el-button-disabled-border-color: #46586d;
  --el-button-disabled-text-color: #94a8bd;
}

.upstream-form {
  margin-top: 10px;
}

.upstream-dialog-host :deep(.upstream-dialog .el-select),
.upstream-dialog-host :deep(.upstream-dialog .el-input) {
  width: 100%;
}

.upstream-dialog-host :deep(.upstream-dialog .el-select__wrapper) {
  --el-select-input-color: #ffffff;
  --el-select-border-color-hover: #64b5ff;
  --el-select-disabled-border: #32475b;
  background: #1f2f3f !important;
  box-shadow: 0 0 0 1px #46586d inset !important;
}

.upstream-dialog-host :deep(.upstream-dialog .el-select__wrapper.is-hovering),
.upstream-dialog-host :deep(.upstream-dialog .el-select__wrapper.is-focused) {
  box-shadow: 0 0 0 1px #64b5ff inset !important;
}

.upstream-dialog-host :deep(.upstream-dialog .el-select__selected-item),
.upstream-dialog-host :deep(.upstream-dialog .el-select__placeholder) {
  color: #ffffff;
}

.upstream-dialog-host :deep(.upstream-dialog .el-select__wrapper.is-disabled) {
  background: #1b2636 !important;
  box-shadow: 0 0 0 1px #32475b inset !important;
}

.upstream-dialog-host :deep(.upstream-dialog .el-input__wrapper) {
  --el-input-bg-color: #1f2f3f;
  --el-input-border-color: #46586d;
  --el-input-hover-border-color: #64b5ff;
  --el-input-focus-border-color: #64b5ff;
  --el-input-text-color: #ffffff;
  --el-input-placeholder-color: #94a8bd;
  box-shadow: 0 0 0 1px var(--el-input-border-color) inset;
}

.upstream-dialog-host :deep(.upstream-dialog .el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 1px #64b5ff inset;
}

.upstream-dialog-host :deep(.upstream-dialog .el-input__inner) {
  color: #ffffff;
}

.upstream-dialog-host :deep(.upstream-dialog .el-input__suffix),
.upstream-dialog-host :deep(.upstream-dialog .el-select__caret) {
  color: #94a8bd;
}

.upstream-dialog-host :deep(.upstream-dialog .el-input.is-disabled .el-input__wrapper) {
  background: #1b2636;
  box-shadow: 0 0 0 1px #32475b inset;
}

.upstream-dialog-host :deep(.upstream-select-popper.el-popper),
.upstream-dialog-host :deep(.upstream-select-popper.el-popper.is-light) {
  background: #243447 !important;
  border: 1px solid #32475b !important;
  box-shadow: 0 10px 24px rgba(0, 0, 0, 0.24) !important;
}

.upstream-dialog-host :deep(.upstream-select-popper .el-popper__arrow::before) {
  background: #243447 !important;
  border-color: #32475b !important;
}

.upstream-dialog-host :deep(.upstream-select-popper .el-select-dropdown),
.upstream-dialog-host :deep(.upstream-select-popper .el-select-dropdown__wrap),
.upstream-dialog-host :deep(.upstream-select-popper .el-select-dropdown__list) {
  background: #243447 !important;
}

.upstream-dialog-host :deep(.upstream-select-popper .el-select-dropdown__item) {
  color: #d9e4ef !important;
  background: transparent !important;
}

.upstream-dialog-host :deep(.upstream-select-popper .el-select-dropdown__item.is-hovering),
.upstream-dialog-host :deep(.upstream-select-popper .el-select-dropdown__item:hover) {
  color: #ffffff !important;
  background: #2d4054 !important;
}

.upstream-dialog-host :deep(.upstream-select-popper .el-select-dropdown__item.is-selected) {
  color: #64b5ff !important;
  background: #1f2f3f !important;
}

.upstream-dialog-host :deep(.upstream-select-popper .el-select-dropdown__item.is-disabled) {
  color: #607184 !important;
  background: transparent !important;
}

@media (max-width: 520px) {
  .upstream-status-bar {
    gap: 6px;
    padding: 0 8px;
  }
}
</style>
