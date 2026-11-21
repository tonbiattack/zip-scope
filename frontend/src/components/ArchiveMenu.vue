<script setup lang="ts">
// Context menu for the archive list. Every action is explicit and destructive
// actions ask for confirmation first.
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{
  x: number
  y: number
  archiveName: string
  canRecycle: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'reveal'): void
  (e: 'rename'): void
  (e: 'move'): void
  (e: 'delete'): void
}>()

const menu = ref<HTMLElement | null>(null)
const pos = ref({ left: props.x, top: props.y })

// Keep the menu inside the window.
onMounted(() => {
  const el = menu.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  const maxLeft = window.innerWidth - rect.width - 8
  const maxTop = window.innerHeight - rect.height - 8
  pos.value = {
    left: Math.max(8, Math.min(props.x, maxLeft)),
    top: Math.max(8, Math.min(props.y, maxTop)),
  }
  window.addEventListener('pointerdown', onOutside, true)
  window.addEventListener('keydown', onKey)
})

onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onOutside, true)
  window.removeEventListener('keydown', onKey)
})

function onOutside(e: PointerEvent) {
  if (menu.value && !menu.value.contains(e.target as Node)) {
    emit('close')
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
  }
}

function run(action: 'reveal' | 'rename' | 'move' | 'delete') {
  if (action === 'reveal') emit('reveal')
  else if (action === 'rename') emit('rename')
  else if (action === 'move') emit('move')
  else emit('delete')
  emit('close')
}
</script>

<template>
  <div
    ref="menu"
    class="menu"
    :style="{ left: pos.left + 'px', top: pos.top + 'px' }"
    @contextmenu.prevent
  >
    <div class="head" :title="archiveName">{{ archiveName }}</div>
    <button class="item" @click="run('reveal')">エクスプローラで開く</button>
    <button class="item" @click="run('rename')">名前を変更…</button>
    <button class="item" @click="run('move')">別のフォルダへ移動…</button>
    <div class="sep" />
    <button class="item danger" :disabled="!canRecycle" @click="run('delete')">
      ごみ箱へ移動
    </button>
  </div>
</template>

<style scoped>
.menu {
  position: fixed;
  z-index: 80;
  min-width: 200px;
  padding: 4px;
  background: var(--panel-strong);
  border: 1px solid var(--border-strong);
  border-radius: 8px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.55);
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.head {
  padding: 6px 10px 8px;
  font-size: 11px;
  color: var(--fg-dim);
  border-bottom: 1px solid var(--border);
  margin-bottom: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 280px;
}
.item {
  text-align: left;
  padding: 6px 10px;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--fg);
  font-size: 12px;
  cursor: pointer;
}
.item:hover:not(:disabled) {
  background: var(--accent-soft);
}
.item:disabled {
  opacity: 0.4;
  cursor: default;
}
.item.danger {
  color: var(--danger);
}
.item.danger:hover:not(:disabled) {
  background: rgba(255, 90, 106, 0.14);
}
.sep {
  height: 1px;
  background: var(--border);
  margin: 4px 2px;
}
</style>
