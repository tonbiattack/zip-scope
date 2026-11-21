<script setup lang="ts">
// Left pane: the archive list. It renders only the rows in view, so a folder
// with ten thousand ZIPs scrolls without creating ten thousand DOM nodes.
import { computed, ref, watch } from 'vue'
import type { ArchiveSummary } from '../lib/types'
import { coverURL } from '../lib/backend'
import { formatBytes, formatDate, naturalCompare, statusLabel } from '../lib/format'

const props = defineProps<{
  archives: ArchiveSummary[]
  selectedId: number | null
  sortMode: 'name' | 'mtime' | 'size'
  sortDesc: boolean
  search: string
  scanning: boolean
}>()

const emit = defineEmits<{
  (e: 'select', archive: ArchiveSummary): void
  (e: 'update:search', value: string): void
  (e: 'update:sortMode', value: 'name' | 'mtime' | 'size'): void
  (e: 'update:sortDesc', value: boolean): void
  (e: 'pick-folder'): void
  (e: 'cancel-scan'): void
  (e: 'reveal', path: string): void
  (e: 'context-menu', archive: ArchiveSummary, x: number, y: number): void
}>()

const ROW_HEIGHT = 64
const OVERSCAN = 6

const scroller = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const viewportHeight = ref(600)

const filtered = computed(() => {
  const q = props.search.trim().toLowerCase()
  let list = props.archives
  if (q) {
    list = list.filter((a) => a.name.toLowerCase().includes(q) || a.path.toLowerCase().includes(q))
  }
  const sorted = [...list]
  const dir = props.sortDesc ? -1 : 1
  sorted.sort((a, b) => {
    switch (props.sortMode) {
      case 'mtime':
        if (a.modTime !== b.modTime) return (a.modTime - b.modTime) * dir
        break
      case 'size':
        if (a.size !== b.size) return (a.size - b.size) * dir
        break
      default:
        break
    }
    return naturalCompare(a.name, b.name) * dir
  })
  return sorted
})

const totalHeight = computed(() => filtered.value.length * ROW_HEIGHT)

const visible = computed(() => {
  const start = Math.max(0, Math.floor(scrollTop.value / ROW_HEIGHT) - OVERSCAN)
  const end = Math.min(
    filtered.value.length,
    Math.ceil((scrollTop.value + viewportHeight.value) / ROW_HEIGHT) + OVERSCAN,
  )
  return filtered.value.slice(start, end).map((archive, i) => ({
    archive,
    top: (start + i) * ROW_HEIGHT,
  }))
})

function onScroll(e: Event) {
  const el = e.target as HTMLElement
  scrollTop.value = el.scrollTop
  viewportHeight.value = el.clientHeight
}

function measure() {
  const el = scroller.value
  if (!el) return
  viewportHeight.value = el.clientHeight
}

watch(
  () => props.selectedId,
  (id) => {
    if (id == null) return
    const idx = filtered.value.findIndex((a) => a.id === id)
    if (idx < 0) return
    const el = scroller.value
    if (!el) return
    const top = idx * ROW_HEIGHT
    const bottom = top + ROW_HEIGHT
    if (top < el.scrollTop) {
      el.scrollTop = top
    } else if (bottom > el.scrollTop + el.clientHeight) {
      el.scrollTop = bottom - el.clientHeight
    }
  },
)

function setSort(mode: 'name' | 'mtime' | 'size') {
  if (props.sortMode === mode) {
    emit('update:sortDesc', !props.sortDesc)
  } else {
    emit('update:sortMode', mode)
    emit('update:sortDesc', mode !== 'name')
  }
}

// A cover that fails to load is remembered so the placeholder is not retried on
// every scroll tick.
const failedCovers = ref(new Set<number>())
function onCoverError(id: number) {
  failedCovers.value.add(id)
  failedCovers.value = new Set(failedCovers.value)
}

defineExpose({ measure })
</script>

<template>
  <aside class="sidebar">
    <header class="sidebar-head">
      <div class="toolbar">
        <button class="btn primary" title="フォルダを選択 (Ctrl+O)" @click="emit('pick-folder')">
          フォルダ
        </button>
        <button v-if="scanning" class="btn danger" title="走査を中止" @click="emit('cancel-scan')">
          中止
        </button>
      </div>
      <input
        class="search"
        type="search"
        placeholder="ZIP を検索"
        :value="search"
        @input="emit('update:search', ($event.target as HTMLInputElement).value)"
      />
      <div class="sorts">
        <button :class="['chip', { on: sortMode === 'name' }]" @click="setSort('name')">
          名前<span v-if="sortMode === 'name'">{{ sortDesc ? '↓' : '↑' }}</span>
        </button>
        <button :class="['chip', { on: sortMode === 'mtime' }]" @click="setSort('mtime')">
          更新<span v-if="sortMode === 'mtime'">{{ sortDesc ? '↓' : '↑' }}</span>
        </button>
        <button :class="['chip', { on: sortMode === 'size' }]" @click="setSort('size')">
          サイズ<span v-if="sortMode === 'size'">{{ sortDesc ? '↓' : '↑' }}</span>
        </button>
      </div>
      <div class="count">{{ filtered.length }} 件</div>
    </header>

    <div ref="scroller" class="list" @scroll="onScroll">
      <div class="spacer" :style="{ height: totalHeight + 'px' }">
        <div
          v-for="row in visible"
          :key="row.archive.id || row.archive.path"
          :class="['row', { selected: row.archive.id === selectedId, bad: row.archive.status !== 'ok' }]"
          :style="{ top: row.top + 'px' }"
          @click="emit('select', row.archive)"
          @dblclick="emit('reveal', row.archive.path)"
          @contextmenu.prevent="emit('context-menu', row.archive, $event.clientX, $event.clientY)"
        >
          <div class="cover">
            <img
              v-if="row.archive.id && row.archive.coverOrdinal >= 0 && !failedCovers.has(row.archive.id)"
              :src="coverURL(row.archive.id, row.archive.coverWidth)"
              alt=""
              loading="lazy"
              decoding="async"
              @error="onCoverError(row.archive.id)"
            />
            <span v-else class="cover-empty">{{ statusLabel(row.archive.status) || '—' }}</span>
          </div>
          <div class="meta">
            <div class="name" :title="row.archive.path">{{ row.archive.name }}</div>
            <div class="sub">
              <span v-if="row.archive.status === 'ok'">{{ row.archive.count }} 枚</span>
              <span v-else class="warn">{{ statusLabel(row.archive.status) }}</span>
              <span class="dot">·</span>
              <span>{{ formatBytes(row.archive.size) }}</span>
            </div>
            <div class="sub dim">{{ formatDate(row.archive.modTime) }}</div>
          </div>
        </div>
      </div>
      <div v-if="filtered.length === 0" class="empty">
        <p v-if="scanning">走査中…</p>
        <p v-else>ZIP が見つかりません。フォルダを選択するか、ZIP をドロップしてください。</p>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  min-width: 260px;
  max-width: 460px;
  width: 320px;
  border-right: 1px solid var(--border);
  background: var(--panel);
  overflow: hidden;
}
.sidebar-head {
  padding: 10px;
  border-bottom: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.toolbar {
  display: flex;
  gap: 6px;
}
.search {
  width: 100%;
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--bg);
  color: var(--fg);
  font-size: 12px;
}
.sorts {
  display: flex;
  gap: 4px;
}
.chip {
  flex: 1;
  padding: 4px 6px;
  font-size: 11px;
  border-radius: 999px;
  border: 1px solid var(--border);
  background: transparent;
  color: var(--fg-dim);
  cursor: pointer;
}
.chip.on {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--fg);
}
.count {
  font-size: 11px;
  color: var(--fg-dim);
}
.list {
  position: relative;
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}
.spacer {
  position: relative;
  width: 100%;
}
.row {
  position: absolute;
  left: 0;
  right: 0;
  height: 64px;
  display: flex;
  gap: 8px;
  padding: 6px 8px;
  box-sizing: border-box;
  cursor: pointer;
  border-left: 2px solid transparent;
}
.row:hover {
  background: var(--hover);
}
.row.selected {
  background: var(--accent-soft);
  border-left-color: var(--accent);
}
.row.bad .name {
  color: var(--fg-dim);
}
.cover {
  width: 52px;
  height: 52px;
  flex: 0 0 52px;
  border-radius: 4px;
  overflow: hidden;
  background: var(--bg);
  display: flex;
  align-items: center;
  justify-content: center;
}
.cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.cover-empty {
  font-size: 9px;
  color: var(--fg-dim);
  text-align: center;
  padding: 2px;
}
.meta {
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
}
.name {
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sub {
  font-size: 10px;
  color: var(--fg-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sub.dim {
  opacity: 0.7;
}
.dot {
  margin: 0 4px;
}
.warn {
  color: var(--warn);
}
.empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  text-align: center;
  font-size: 12px;
  color: var(--fg-dim);
}
</style>
