<script setup lang="ts">
// Right pane: the thumbnail grid. Only the visible window of cells is rendered,
// and each cell requests its thumbnail lazily so scrolling a large archive does
// not decode every image.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { EntryInfo } from '../lib/types'
import { Priority, thumbURL } from '../lib/backend'
import { computeGrid, visibleRange } from '../lib/grid'
import { errorKindLabel } from '../lib/format'

const props = defineProps<{
  archiveId: number | null
  entries: EntryInfo[]
  thumbSize: number
  selectedOrdinal: number | null
  loading: boolean
  status: string
  error?: string
}>()

const emit = defineEmits<{
  (e: 'select', entry: EntryInfo, index: number): void
  (e: 'open', entry: EntryInfo, index: number): void
  (e: 'prefetch', ordinal: number, priority: number): void
  (e: 'update:thumbSize', value: number): void
}>()

const GAP = 8
const LABEL_HEIGHT = 18
const OVERSCAN = 2

const scroller = ref<HTMLElement | null>(null)
const width = ref(600)
const height = ref(600)
const scrollTop = ref(0)

const geometry = computed(() =>
  computeGrid({
    width: width.value,
    itemCount: props.entries.length,
    thumbSize: props.thumbSize,
    gap: GAP,
    labelHeight: LABEL_HEIGHT,
  }),
)

const range = computed(() =>
  visibleRange(scrollTop.value, height.value, geometry.value, props.entries.length, OVERSCAN),
)

const cells = computed(() => {
  const g = geometry.value
  const out: { entry: EntryInfo; index: number; left: number; top: number }[] = []
  for (let i = range.value.start; i < range.value.end; i++) {
    const entry = props.entries[i]
    if (!entry) continue
    const row = Math.floor(i / g.columns)
    const col = i % g.columns
    out.push({
      entry,
      index: i,
      left: col * g.cellWidth,
      top: row * g.cellHeight,
    })
  }
  return out
})

// Track which cells have already asked for a prefetch so a scroll tick does not
// spam the scheduler.
const requested = new Set<number>()

function onScroll(e: Event) {
  const el = e.target as HTMLElement
  scrollTop.value = el.scrollTop
}

function measure() {
  const el = scroller.value
  if (!el) return
  width.value = el.clientWidth
  height.value = el.clientHeight
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

// When the visible window changes, ask the backend to warm the neighbours of
// the selection at low priority.
watch(
  () => [range.value.start, range.value.end, props.archiveId] as const,
  () => {
    if (props.archiveId == null) return
    const ahead = 2
    const sel = props.selectedOrdinal
    if (sel == null) return
    const idx = props.entries.findIndex((e) => e.ordinal === sel)
    if (idx < 0) return
    for (let d = 1; d <= ahead; d++) {
      for (const j of [idx - d, idx + d]) {
        if (j < 0 || j >= props.entries.length) continue
        const ord = props.entries[j].ordinal
        if (requested.has(ord)) continue
        requested.add(ord)
        emit('prefetch', ord, Priority.Neighbour)
      }
    }
  },
)

watch(
  () => props.archiveId,
  () => {
    requested.clear()
    scrollTop.value = 0
    if (scroller.value) scroller.value.scrollTop = 0
  },
)

// Scroll the selected cell into view when the selection moves by keyboard.
watch(
  () => props.selectedOrdinal,
  (ord) => {
    if (ord == null) return
    const el = scroller.value
    if (!el) return
    const idx = props.entries.findIndex((e) => e.ordinal === ord)
    if (idx < 0) return
    const g = geometry.value
    const row = Math.floor(idx / g.columns)
    const top = row * g.cellHeight
    const bottom = top + g.cellHeight
    if (top < el.scrollTop) el.scrollTop = top
    else if (bottom > el.scrollTop + el.clientHeight) el.scrollTop = bottom - el.clientHeight
  },
)

const failed = ref(new Set<number>())
function onError(ordinal: number) {
  failed.value.add(ordinal)
  failed.value = new Set(failed.value)
}

function imgSrc(entry: EntryInfo): string {
  if (props.archiveId == null) return ''
  return thumbURL(props.archiveId, entry.ordinal, props.thumbSize)
}

function restoreScroll(top: number) {
  if (scroller.value) scroller.value.scrollTop = top
  scrollTop.value = top
}

function currentScroll(): number {
  return scroller.value?.scrollTop ?? 0
}

defineExpose({ measure, restoreScroll, currentScroll })
</script>

<template>
  <section class="grid-pane">
    <header class="grid-head">
      <div class="title">
        <template v-if="archiveId != null">
          <span class="count">{{ entries.length }} 枚</span>
          <span v-if="status !== 'ok'" class="warn">{{ status }}</span>
        </template>
        <span v-else class="dim">ZIP を選択してください</span>
      </div>
      <label class="size">
        <span>サイズ</span>
        <input
          type="range"
          min="96"
          max="512"
          step="32"
          :value="thumbSize"
          @input="emit('update:thumbSize', Number(($event.target as HTMLInputElement).value))"
        />
      </label>
    </header>

    <div ref="scroller" class="grid-scroll" @scroll="onScroll">
      <div v-if="loading" class="overlay">読み込み中…</div>
      <div v-else-if="archiveId != null && entries.length === 0" class="overlay">
        <p v-if="status === 'empty'">この ZIP には画像がありません。</p>
        <p v-else-if="error">{{ error }}</p>
        <p v-else>表示できる画像がありません。</p>
      </div>
      <div v-else class="canvas" :style="{ height: geometry.totalHeight + 'px' }">
        <button
          v-for="cell in cells"
          :key="cell.entry.ordinal"
          :class="['cell', { selected: cell.entry.ordinal === selectedOrdinal }]"
          :style="{
            left: cell.left + 'px',
            top: cell.top + 'px',
            width: thumbSize + 'px',
            height: thumbSize + LABEL_HEIGHT + 'px',
          }"
          :title="cell.entry.name"
          @click="emit('select', cell.entry, cell.index)"
          @dblclick="emit('open', cell.entry, cell.index)"
        >
          <span class="thumb" :style="{ width: thumbSize + 'px', height: thumbSize + 'px' }">
            <img
              v-if="!failed.has(cell.entry.ordinal) && !cell.entry.encrypted"
              :src="imgSrc(cell.entry)"
              alt=""
              loading="lazy"
              decoding="async"
              @error="onError(cell.entry.ordinal)"
            />
            <span v-else class="bad">{{ cell.entry.encrypted ? '暗号化' : errorKindLabel('corrupt') }}</span>
          </span>
          <span class="label">{{ cell.entry.name.split('/').pop() }}</span>
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.grid-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: var(--bg);
}
.grid-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  background: var(--panel);
  font-size: 12px;
}
.title {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}
.count {
  color: var(--fg-dim);
}
.dim {
  color: var(--fg-dim);
}
.warn {
  color: var(--warn);
}
.size {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--fg-dim);
  font-size: 11px;
}
.size input {
  width: 120px;
  accent-color: var(--accent);
}
.grid-scroll {
  position: relative;
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}
.canvas {
  position: relative;
  width: 100%;
}
.cell {
  position: absolute;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
  overflow: hidden;
  color: inherit;
}
.cell:hover {
  border-color: var(--border-strong);
}
.cell.selected {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent) inset;
}
.thumb {
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--panel);
  border-radius: 4px;
  overflow: hidden;
}
.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.bad {
  font-size: 10px;
  color: var(--fg-dim);
}
.label {
  font-size: 10px;
  color: var(--fg-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  padding: 0 2px;
  line-height: 16px;
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
