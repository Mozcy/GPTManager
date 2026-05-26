<script setup>
import { computed } from 'vue'

const props = defineProps({
  label: {
    type: String,
    required: true,
  },
  value: {
    type: [String, Number, Boolean],
    default: '',
  },
  emptyText: {
    type: String,
    default: '-',
  },
  placement: {
    type: String,
    default: 'top',
  },
  width: {
    type: [String, Number],
    default: 520,
  },
  popperClass: {
    type: String,
    default: '',
  },
})

const displayValue = computed(() => {
  if (props.value === null || props.value === undefined || props.value === '') {
    return props.emptyText
  }
  return String(props.value)
})

const hasValue = computed(() => displayValue.value !== props.emptyText)

const mergedPopperClass = computed(() => {
  return ['value-popover', props.popperClass].filter(Boolean).join(' ')
})
</script>

<template>
  <el-popover
    trigger="click"
    :placement="placement"
    :width="width"
    :disabled="!hasValue"
    :popper-class="mergedPopperClass"
  >
    <template #reference>
      <slot name="reference" :label="label" :value="value" :display-value="displayValue">
        <code class="value-popover-reference" :class="{ disabled: !hasValue }">{{ displayValue }}</code>
      </slot>
    </template>

    <div class="value-popover-panel">
      <div class="value-popover-title">{{ label }}</div>
      <div class="value-popover-content">{{ displayValue }}</div>
    </div>
  </el-popover>
</template>

<style scoped>
.value-popover-reference {
  display: block;
  min-width: 0;
  padding: 6px 8px;
  overflow: hidden;
  border: 1px solid #32475b;
  border-radius: 5px;
  background: #1f2f3f;
  color: #e8eef5;
  cursor: pointer;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.5;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.value-popover-reference.disabled {
  cursor: default;
}

:global(.value-popover) {
  max-width: min(720px, calc(100vw - 48px));
  border: 1px solid #32475b !important;
  background: #1f2f3f !important;
  color: #e8eef5 !important;
}

:global(.value-popover .el-popper__arrow::before) {
  border-color: #32475b !important;
  background: #1f2f3f !important;
}

:global(.value-popover-panel) {
  min-width: 0;
}

:global(.value-popover-title) {
  margin-bottom: 10px;
  color: #ffffff;
  font-size: 13px;
  font-weight: 700;
  line-height: 1.4;
}

:global(.value-popover-content) {
  max-height: 360px;
  overflow: auto;
  color: #e8eef5;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  scrollbar-color: #4f6680 #1f2f3f;
  scrollbar-width: thin;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
}

:global(.value-popover-content::-webkit-scrollbar) {
  width: 8px;
}

:global(.value-popover-content::-webkit-scrollbar-track) {
  background: #1f2f3f;
  border-radius: 999px;
}

:global(.value-popover-content::-webkit-scrollbar-thumb) {
  background: #4f6680;
  border-radius: 999px;
}

:global(.value-popover-content::-webkit-scrollbar-thumb:hover) {
  background: #66809b;
}
</style>
