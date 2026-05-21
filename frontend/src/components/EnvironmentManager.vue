<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  GetCodexAuthInfo,
  ScanCodexAuth,
} from '../../wailsjs/go/main/App'

const authRows = ref([])
const environmentLoading = ref(false)

onMounted(async () => {
  await loadCodexAuthInfo()
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

:deep(.environment-table .el-table__empty-text) {
  color: #9fb0c2;
}
</style>
