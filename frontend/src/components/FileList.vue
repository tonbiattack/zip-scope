<script setup lang="ts">
// Explorer-style file list. One component renders both the folder's archives
// and an archive's images, so navigating between them feels like moving through
// folders rather than switching between two different screens.
//
// Three view modes mirror Explorer: details (rows with columns), medium icons
// and large icons. All three are virtualised, so a folder with ten thousand
// entries still renders only what is on screen.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { ArchiveSummary, EntryInfo } from '../lib/types'
import { coverURL, thumbURL } from '../lib/backend'
import { formatBytes, formatDate, naturalCompare, statusLabel } from '../lib/format'

export type ViewMode = 'details' | 'medium' | 'large'

/** One row in the list, normalised from either an archive or an entry. */
interface Item {
  key: string
  name: string
  /** Secondary text: image count for archives, dimensions for images. */
  detail: string
  size: number
  modTime: number
  /** Thumbnail URL, or empty when none can be produced. */
  thumb: string
  /** Status label shown instead of a thumbnail when the item is unusable. */
  badge: string
  /** True for a broken or empty archive. */
  bad: boolean
  archive?: ArchiveSummary
  entry?: EntryInfo
}

const props = defineProps<{
  /** Archives shown when browsing a folder. */
  archives: ArchiveSummary[]
  /** Images shown when browsing inside an archive. */
  entries: EntryInfo[]
  /** True when the current location is inside an archive. */
  insideArchive: boolean
  /** Archive id, needed to build image URLs. */
  archiveId: number | null
  viewMode: ViewMode
  iconSize: number
  sortMode: 'name' | 'mtime' | 'size'
  sortDesc: boolean
  search: string
  loading: boolean
  status: string
  error?: string
  /** Selected item keys. */
  selected: string[]
}>()

const emit = defineEmits<{
  (e: 'select', keys: string[], anchor: string): void
  (e: 'open', key: string): void
  (e: 'context-menu', key: string, x: number, y: number): void
  (e: 'update:sortMode', value: 'name' | 'mtime' | 'size'): void
  (e: 'update:sortDesc', value: boolean): void
  (e: 'update:viewMode', value: ViewMode): void
  (e: 'update:iconSize', value: number): void
  (e: 'prefetch', ordinal: number): void
}>()

const ROW_HEIGHT = 26
const DETAILS_OVERSCAN = 12
const ICON_OVERSCAN = 2
const GAP = 8
const LABEL_HEIGHT = 30

const scroller = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const viewportWidth = ref(800)
const viewportHeight = ref(600)

// --- items ----------------------------------------------------------------

const items = computed<Item[]>(() => {
  if (props.insideArchive) {
    return props.entries.map((e) => ({
      key: `e:${e.ordinal}`,
      name: e.name,
      detail: e.encrypted ? '暗号化' : '',
      size: e.size,
      modTime: 0,
      // An encrypted entry cannot be decoded, so no thumbnail is requested.
      thumb:
        !e.encrypted && props.archiveId != null
          ? thumbURL(props.archiveId, e.ordinal, props.iconSize)
          : '',
      badge: e.encrypted ? '暗号化' : '',
      bad: e.encrypted,
      entry: e,
    }))
  }
  return props.archives.map((a) => ({
    key: `a:${a.id || a.path}`,
    name: a.name,
    detail: a.status === 'ok' ? `${a.count} 枚` : statusLabel(a.status),
    size: a.size,
    modTime: a.modTime,
    thumb: a.id && a.coverOrdinal >= 0 ? coverURL(a.id, a.coverWidth) : '',
    badge: statusLabel(a.status),
    bad: a.status !== 'ok',
    archive: a,
  }))
})

const filtered = computed(() => {
  const q = props.search.trim().toLowerCase()
  let list = items.value
  if (q) list = list.filter((i) => i.name.toLowerCase().includes(q))
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

// --- geometry -------------------------------------------------------------

const columns = computed(() => {
  if (props.viewMode === 'details') return 1
  const cell = props.iconSize + GAP
  return Math.max(1, Math.floor(Math.max(cell, viewportWidth.value) / cell))
})

const cellHeight = computed(() =>
  props.viewMode === 'details' ? ROW_HEIGHT : props.iconSize + LABEL_HEIGHT + GAP,
)

const rowCount = computed(() => Math.ceil(filtered.value.length / columns.value))
const totalHeight = computed(() => rowCount.value * cellHeight.value)

const visible = computed(() => {
  const overscan = props.viewMode === 'details' ? DETAILS_OVERSCAN : ICON_OVERSCAN
  const firstRow = Math.max(0, Math.floor(scrollTop.value / cellHeight.value) - overscan)
  const lastRow = Math.min(
    rowCount.value - 1,
    Math.ceil((scrollTop.value + viewportHeight.value) / cellHeight.value) + overscan,
  )
  const start = Math.min(filtered.value.length, firstRow * columns.value)
  const end = Math.min(filtered.value.length, (lastRow + 1) * columns.value)
  const out: { item: Item; index: number; left: number; top: number }[] = []
  for (let i = start; i < end; i++) {
    const item = filtered.value[i]
    if (!item) continue
    const row = Math.floor(i / columns.value)
    const col = i % columns.value
    out.push({
      item,
      index: i,
      left: props.viewMode === 'details' ? 0 : col * (props.iconSize + GAP),
      top: row * cellHeight.value,
    })
  }
  return out
})

// --- interaction ----------------------------------------------------------

const selectedSet = computed(() => new Set(props.selected))

let anchorKey: string | null = null

function onClick(item: Item, e: MouseEvent) {
  const keys = filtered.value.map((i) => i.key)
  if (e.shiftKey && anchorKey) {
    const a = keys.indexOf(anchorKey)
    const b = keys.indexOf(item.key)
    if (a >= 0 && b >= 0) {
      const [lo, hi] = a < b ? [a, b] : [b, a]
      emit('select', keys.slice(lo, hi + 1), anchorKey)
      return
    }
  }
  if (e.ctrlKey || e.metaKey) {
    const next = new Set(props.selected)
    if (next.has(item.key)) next.delete(item.key)
    else next.add(item.key)
    anchorKey = item.key
    emit('select', [...next], item.key)
    return
  }
  anchorKey = item.key
  emit('select', [item.key], item.key)
}

function onDoubleClick(item: Item) {
  emit('open', item.key)
}

function onContextMenu(item: Item, e: MouseEvent) {
  // Right-clicking an unselected item selects it first, like Explorer.
  if (!selectedSet.value.has(item.key)) {
    anchorKey = item.key
    emit('select', [item.key], item.key)
  }
  emit('context-menu', item.key, e.clientX, e.clientY)
}

function onScroll(e: Event) {
  const el = e.target as HTMLElement
  scrollTop.value = el.scrollTop
}

function measure() {
  const el = scroller.value
  if (!el) return
  viewportWidth.value = el.clientWidth
  viewportHeight.value = el.clientHeight
}

let observer: ResizeObserver | null = null
onMounted(() => {
  measure()
  if (typeof ResizeObserver !== 'undefined' && scroller.value) {
    observer = new ResizeObserver(measure)
    observer.observe(scroller.value)
  }
})
onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})

// A new location resets the scroll position.
watch(
  () => [props.insideArchive, props.archiveId, props.archives.length === 0] as const,
  () => {
    scrollTop.value = 0
    if (scroller.value) scroller.value.scrollTop = 0
    anchorKey = null
  },
)

// Keep the first selected item in view when the selection moves by keyboard.
watch(
  () => props.selected,
  (sel) => {
    const el = scroller.value
    if (!el || sel.length === 0) return
    const idx = filtered.value.findIndex((i) => i.key === sel[0])
    if (idx < 0) return
    const row = Math.floor(idx / columns.value)
    const top = row * cellHeight.value
    const bottom = top + cellHeight.value
    if (top < el.scrollTop) el.scrollTop = top
    else if (bottom > el.scrollTop + el.clientHeight) el.scrollTop = bottom - el.clientHeight
  },
)

// --- column sorting -------------------------------------------------------

function sortBy(mode: 'name' | 'mtime' | 'size') {
  if (props.sortMode === mode) {
    emit('update:sortDesc', !props.sortDesc)
  } else {
    emit('update:sortMode', mode)
    emit('update:sortDesc', mode !== 'name')
  }
}

function arrow(mode: 'name' | 'mtime' | 'size'): string {
  if (props.sortMode !== mode) return ''
  return props.sortDesc ? '▾' : '▴'
}

// --- thumbnails -----------------------------------------------------------

const failed = ref(new Set<string>())
function onThumbError(key: string) {
  failed.value.add(key)
  failed.value = new Set(failed.value)
}

function thumbFor(item: Item): string {
  if (!item.thumb || failed.value.has(item.key)) return ''
  return item.thumb
}

// Ask the backend to warm the neighbours of the visible window.
const requested = new Set<string>()
watch(
  () => [visible.value.length, props.archiveId] as const,
  () => {
    if (!props.insideArchive || props.archiveId == null) return
    for (const cell of visible.value) {
      const e = cell.item.entry
      if (!e || requested.has(cell.item.key)) continue
      requested.add(cell.item.key)
      emit('prefetch', e.ordinal)
    }
  },
)

watch(
  () => props.archiveId,
  () => {
    requested.clear()
    failed.value = new Set()
  },
)

defineExpose({ measure })
</script>

<template>
  <section class="filelist">
    <div v-if="viewMode === 'details'" class="header">
      <button class="col name" @click="sortBy('name')">名前 <span class="arrow">{{ arrow('name') }}</span></button>
      <button class="col detail" @click="sortBy('name')">種類</button>
      <button class="col size" @click="sortBy('size')">サイズ <span class="arrow">{{ arrow('size') }}</span></button>
      <button class="col date" @click="sortBy('mtime')">更新日時 <span class="arrow">{{ arrow('mtime') }}</span></button>
    </div>

    <div ref="scroller" class="scroll" @scroll="onScroll">
      <div v-if="loading" class="overlay">読み込み中…</div>
      <div v-else-if="filtered.length === 0" class="overlay">
        <p v-if="insideArchive && status === 'empty'">この ZIP には画像がありません。</p>
        <p v-else-if="error">{{ error }}</p>
        <p v-else-if="insideArchive">表示できる画像がありません。</p>
        <p v-else>ZIP が見つかりません。フォルダを選択するか、ZIP をドロップしてください。</p>
      </div>

      <div v-else class="canvas" :style="{ height: totalHeight + 'px' }">
        <div
          v-for="cell in visible"
          :key="cell.item.key"
          :class="[
            'item',
            viewMode,
            {
              selected: selectedSet.has(cell.item.key),
              bad: cell.item.bad,
            },
          ]"
          :style="
            viewMode === 'details'
              ? { top: cell.top + 'px', height: ROW_HEIGHT + 'px' }
              : {
                  left: cell.left + 'px',
                  top: cell.top + 'px',
                  width: iconSize + 'px',
                  height: iconSize + LABEL_HEIGHT + 'px',
                }
          "
          :title="cell.item.name"
          @click="onClick(cell.item, $event)"
          @dblclick="onDoubleClick(cell.item)"
          @contextmenu.prevent="onContextMenu(cell.item, $event)"
        >
          <span
            class="thumb"
            :style="
              viewMode === 'details'
                ? { width: '20px', height: '20px' }
                : { width: iconSize + 'px', height: iconSize + 'px' }
            "
          >
            <img
              v-if="thumbFor(cell.item)"
              :src="thumbFor(cell.item)"
              alt=""
              loading="lazy"
              decoding="async"
              @error="onThumbError(cell.item.key)"
            />
            <span v-else class="placeholder">{{ cell.item.badge || '—' }}</span>
          </span>

          <template v-if="viewMode === 'details'">
            <span class="name">{{ cell.item.name }}</span>
            <span class="detail">{{ cell.item.detail }}</span>
            <span class="size">{{ formatBytes(cell.item.size) }}</span>
            <span class="date">{{ cell.item.modTime ? formatDate(cell.item.modTime) : '' }}</span>
          </template>
          <template v-else>
            <span class="label">{{ cell.item.name }}</span>
          </template>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.filelist {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--bg);
}

/* Column header */
.header {
  display: flex;
  align-items: center;
  height: 26px;
  padding: 0 8px;
  border-bottom: 1px solid var(--border);
  background: var(--panel);
  flex: 0 0 auto;
}
.col {
  border: none;
  background: transparent;
  color: var(--fg-dim);
  font-size: 11px;
  text-align: left;
  padding: 0 6px;
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.col:hover {
  color: var(--fg);
}
.col.name {
  flex: 1 1 auto;
  min-width: 120px;
}
.col.detail {
  flex: 0 0 110px;
}
.col.size {
  flex: 0 0 90px;
  text-align: right;
}
.col.date {
  flex: 0 0 140px;
  text-align: right;
}
.arrow {
  font-size: 9px;
  opacity: 0.8;
}

/* Scrolling canvas */
.scroll {
  position: relative;
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  min-height: 0;
}
.canvas {
  position: relative;
  width: 100%;
}

/* Items */
.item {
  position: absolute;
  display: flex;
  align-items: center;
  border: 1px solid transparent;
  border-radius: 4px;
  cursor: default;
  color: var(--fg);
  user-select: none;
}
.item:hover {
  background: var(--hover);
}
.item.selected {
  background: var(--accent-soft);
  border-color: var(--accent);
}
.item.bad .name,
.item.bad .label {
  color: var(--fg-dim);
}

/* Details layout */
.item.details {
  left: 0;
  right: 0;
  gap: 8px;
  padding: 0 8px;
}
.item.details .name {
  flex: 1 1 auto;
  min-width: 120px;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.item.details .detail {
  flex: 0 0 110px;
  font-size: 11px;
  color: var(--fg-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.item.details .size {
  flex: 0 0 90px;
  font-size: 11px;
  color: var(--fg-dim);
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.item.details .date {
  flex: 0 0 140px;
  font-size: 11px;
  color: var(--fg-dim);
  text-align: right;
  font-variant-numeric: tabular-nums;
}

/* Icon layouts */
.item.medium,
.item.large {
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 4px;
  box-sizing: border-box;
}
.item.medium .label,
.item.large .label {
  font-size: 11px;
  line-height: 1.25;
  text-align: center;
  width: 100%;
  max-height: 28px;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  word-break: break-all;
}

/* Thumbnails */
.thumb {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  background: var(--panel);
  border-radius: 3px;
  overflow: hidden;
}
.item.medium .thumb,
.item.large .thumb {
  border-radius: 4px;
}
.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.placeholder {
  font-size: 9px;
  color: var(--fg-dim);
  text-align: center;
  padding: 2px;
  line-height: 1.2;
}

.overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  color: var(--fg-dim);
  font-size: 12px;
  padding: 24px;
}
</style>
