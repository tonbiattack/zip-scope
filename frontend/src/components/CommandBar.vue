<script setup lang="ts">
// Explorer-style command bar: the actions that apply to the current location,
// plus the view switcher and the icon-size slider.
import { computed } from 'vue'
import type { ViewMode } from './FileList.vue'

const props = defineProps<{
  insideArchive: boolean
  hasSelection: boolean
  selectionCount: number
  viewMode: ViewMode
  iconSize: number
  scanning: boolean
  canRecycle: boolean
}>()

const emit = defineEmits<{
  (e: 'pick-folder'): void
  (e: 'open'): void
  (e: 'reveal'): void
  (e: 'rename'): void
  (e: 'move'): void
  (e: 'delete'): void
  (e: 'cancel-scan'): void
  (e: 'update:viewMode', value: ViewMode): void
  (e: 'update:iconSize', value: number): void
  (e: 'settings'): void
}>()

const selectionLabel = computed(() => {
  if (props.selectionCount === 0) return '未選択'
  if (props.selectionCount === 1) return '1 件を選択'
  return `${props.selectionCount} 件を選択`
})

function setMode(mode: ViewMode) {
  emit('update:viewMode', mode)
}
</script>

<template>
  <div class="commandbar">
    <div class="group">
      <button class="cmd" title="フォルダを選択 (Ctrl+O)" @click="emit('pick-folder')">
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M2 4.5A1.5 1.5 0 0 1 3.5 3h2.6l1.2 1.5h5.2A1.5 1.5 0 0 1 14 6v5.5A1.5 1.5 0 0 1 12.5 13h-9A1.5 1.5 0 0 1 2 11.5z"
                fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" />
        </svg>
        フォルダ
      </button>
      <button class="cmd" :disabled="!hasSelection" title="開く (Enter)" @click="emit('open')">
        開く
      </button>
      <button
        v-if="!insideArchive"
        class="cmd"
        :disabled="!hasSelection"
        title="エクスプローラで表示"
        @click="emit('reveal')"
      >
        場所を開く
      </button>
      <template v-if="!insideArchive">
        <span class="divider" />
        <button class="cmd" :disabled="!hasSelection" title="名前を変更" @click="emit('rename')">
          名前変更
        </button>
        <button class="cmd" :disabled="!hasSelection" title="別のフォルダへ移動" @click="emit('move')">
          移動
        </button>
        <button
          class="cmd danger"
          :disabled="!hasSelection || !canRecycle"
          title="ごみ箱へ移動"
          @click="emit('delete')"
        >
          削除
        </button>
      </template>
      <button v-if="scanning" class="cmd danger" title="走査を中止" @click="emit('cancel-scan')">
        中止
      </button>
    </div>

    <div class="group right">
      <span class="selection">{{ selectionLabel }}</span>
      <span class="divider" />
      <button
        :class="['viewbtn', { on: viewMode === 'details' }]"
        title="詳細"
        @click="setMode('details')"
      >
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M2 4h12M2 8h12M2 12h12" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
        </svg>
      </button>
      <button
        :class="['viewbtn', { on: viewMode === 'medium' }]"
        title="中アイコン"
        @click="setMode('medium')"
      >
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <rect x="2.5" y="2.5" width="4.5" height="4.5" rx="1" fill="none" stroke="currentColor" stroke-width="1.3" />
          <rect x="9" y="2.5" width="4.5" height="4.5" rx="1" fill="none" stroke="currentColor" stroke-width="1.3" />
          <rect x="2.5" y="9" width="4.5" height="4.5" rx="1" fill="none" stroke="currentColor" stroke-width="1.3" />
          <rect x="9" y="9" width="4.5" height="4.5" rx="1" fill="none" stroke="currentColor" stroke-width="1.3" />
        </svg>
      </button>
      <button
        :class="['viewbtn', { on: viewMode === 'large' }]"
        title="大アイコン"
        @click="setMode('large')"
      >
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <rect x="2.5" y="2.5" width="11" height="11" rx="1.5" fill="none" stroke="currentColor" stroke-width="1.3" />
        </svg>
      </button>
      <input
        v-if="viewMode !== 'details'"
        class="slider"
        type="range"
        min="64"
        max="256"
        step="16"
        :value="iconSize"
        title="アイコンの大きさ"
        @input="emit('update:iconSize', Number(($event.target as HTMLInputElement).value))"
      />
      <span class="divider" />
      <button class="cmd" title="設定" @click="emit('settings')">設定</button>
    </div>
  </div>
</template>

<style scoped>
.commandbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 4px 8px;
  background: var(--panel);
  border-bottom: 1px solid var(--border);
  min-height: 34px;
  flex: 0 0 auto;
}
.group {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
}
.group.right {
  flex: 0 0 auto;
}
.cmd {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 9px;
  border: 1px solid transparent;
  border-radius: 5px;
  background: transparent;
  color: var(--fg);
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
}
.cmd:hover:not(:disabled) {
  background: var(--hover);
}
.cmd:disabled {
  opacity: 0.4;
  cursor: default;
}
.cmd.danger {
  color: var(--danger);
}
.cmd.danger:hover:not(:disabled) {
  background: rgba(255, 90, 106, 0.12);
}
.divider {
  width: 1px;
  height: 16px;
  background: var(--border);
  margin: 0 4px;
  flex: 0 0 auto;
}
.selection {
  font-size: 11px;
  color: var(--fg-dim);
  white-space: nowrap;
  padding: 0 4px;
}
.viewbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 24px;
  border: 1px solid transparent;
  border-radius: 5px;
  background: transparent;
  color: var(--fg-dim);
  cursor: pointer;
}
.viewbtn:hover {
  background: var(--hover);
  color: var(--fg);
}
.viewbtn.on {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--fg);
}
.slider {
  width: 90px;
  margin: 0 4px;
  accent-color: var(--accent);
}
</style>
