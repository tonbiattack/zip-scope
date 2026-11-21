<script setup lang="ts">
// Full-size viewer. It owns zoom, pan and keyboard navigation, and it reports
// every navigation back to the parent so the parent can prefetch and persist
// the position.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { EntryInfo } from '../lib/types'
import { fullURL } from '../lib/backend'
import { errorKindLabel } from '../lib/format'

const props = defineProps<{
  archiveId: number
  archiveName: string
  entries: EntryInfo[]
  index: number
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'navigate', index: number): void
  (e: 'navigate-archive', delta: number): void
}>()

const MIN_ZOOM = 0.05
const MAX_ZOOM = 16

const zoom = ref(1)
const fitMode = ref(true)
const panX = ref(0)
const panY = ref(0)
const loading = ref(true)
const failed = ref<string | null>(null)
const naturalWidth = ref(0)
const naturalHeight = ref(0)
const stage = ref<HTMLElement | null>(null)
const viewport = ref({ w: 800, h: 600 })

const entry = computed<EntryInfo | undefined>(() => props.entries[props.index])
const src = computed(() => (entry.value ? fullURL(props.archiveId, entry.value.ordinal) : ''))

// fitScale is the zoom that makes the image fit the viewport. Small images are
// enlarged so "fit" really fills the screen, but the upscale is capped so a
// tiny image does not turn into a blurry mess.
const MAX_FIT_UPSCALE = 4

const fitScale = computed(() => {
  if (!naturalWidth.value || !naturalHeight.value) return 1
  const sx = (viewport.value.w - 32) / naturalWidth.value
  const sy = (viewport.value.h - 32) / naturalHeight.value
  return Math.min(MAX_FIT_UPSCALE, Math.max(0.01, Math.min(sx, sy)))
})

const effectiveZoom = computed(() => (fitMode.value ? fitScale.value : zoom.value))

const imageStyle = computed(() => ({
  transform: `translate(${panX.value}px, ${panY.value}px) scale(${effectiveZoom.value})`,
}))

function resetView() {
  fitMode.value = true
  zoom.value = 1
  panX.value = 0
  panY.value = 0
}

function setZoom(next: number, originX?: number, originY?: number) {
  const clamped = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, next))
  const prev = effectiveZoom.value
  if (clamped === prev) return
  // Keep the point under the cursor fixed while zooming.
  if (originX != null && originY != null) {
    const cx = viewport.value.w / 2
    const cy = viewport.value.h / 2
    const dx = originX - cx
    const dy = originY - cy
    const ratio = clamped / prev
    panX.value = dx - (dx - panX.value) * ratio
    panY.value = dy - (dy - panY.value) * ratio
  }
  fitMode.value = false
  zoom.value = clamped
}

function toggleFit() {
  if (fitMode.value) {
    // Switching to 1:1 centres the image at its natural size.
    fitMode.value = false
    zoom.value = 1
    panX.value = 0
    panY.value = 0
  } else {
    resetView()
  }
}

function onWheel(e: WheelEvent) {
  e.preventDefault()
  const rect = stage.value?.getBoundingClientRect()
  const ox = rect ? e.clientX - rect.left : undefined
  const oy = rect ? e.clientY - rect.top : undefined
  const factor = e.deltaY < 0 ? 1.15 : 1 / 1.15
  setZoom(effectiveZoom.value * factor, ox, oy)
}

let dragging = false
let dragStartX = 0
let dragStartY = 0
let panStartX = 0
let panStartY = 0

function onPointerDown(e: PointerEvent) {
  if (e.button !== 0) return
  dragging = true
  dragStartX = e.clientX
  dragStartY = e.clientY
  panStartX = panX.value
  panStartY = panY.value
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
}

function onPointerMove(e: PointerEvent) {
  if (!dragging) return
  panX.value = panStartX + (e.clientX - dragStartX)
  panY.value = panStartY + (e.clientY - dragStartY)
}

function onPointerUp(e: PointerEvent) {
  if (!dragging) return
  dragging = false
  try {
    ;(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
  } catch {
    // The capture may already be gone if the pointer left the window.
  }
}

function go(delta: number) {
  const next = props.index + delta
  if (next < 0 || next >= props.entries.length) return
  emit('navigate', next)
}

function onKey(e: KeyboardEvent) {
  switch (e.key) {
    case 'Escape':
      e.preventDefault()
      emit('close')
      break
    case 'ArrowLeft':
      e.preventDefault()
      if (e.ctrlKey) emit('navigate-archive', -1)
      else go(-1)
      break
    case 'ArrowRight':
      e.preventDefault()
      if (e.ctrlKey) emit('navigate-archive', 1)
      else go(1)
      break
    case 'Home':
      e.preventDefault()
      emit('navigate', 0)
      break
    case 'End':
      e.preventDefault()
      emit('navigate', props.entries.length - 1)
      break
    case '+':
    case '=':
      e.preventDefault()
      setZoom(effectiveZoom.value * 1.25)
      break
    case '-':
      e.preventDefault()
      setZoom(effectiveZoom.value / 1.25)
      break
    case '0':
      e.preventDefault()
      resetView()
      break
    case '1':
      e.preventDefault()
      toggleFit()
      break
    default:
      break
  }
}

function onImageLoad(e: Event) {
  const img = e.target as HTMLImageElement
  naturalWidth.value = img.naturalWidth
  naturalHeight.value = img.naturalHeight
  loading.value = false
  failed.value = null
}

function onImageError() {
  loading.value = false
  failed.value = errorKindLabel('corrupt')
}

function measure() {
  const el = stage.value
  if (!el) return
  viewport.value = { w: el.clientWidth, h: el.clientHeight }
}

let observer: ResizeObserver | null = null
onMounted(() => {
  measure()
  if (typeof ResizeObserver !== 'undefined' && stage.value) {
    observer = new ResizeObserver(measure)
    observer.observe(stage.value)
  }
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
  window.removeEventListener('keydown', onKey)
})

// A new image resets the view so the previous zoom does not carry over.
watch(
  () => [props.archiveId, props.index] as const,
  () => {
    loading.value = true
    failed.value = null
    naturalWidth.value = 0
    naturalHeight.value = 0
    resetView()
  },
)

const position = computed(() => `${props.index + 1} / ${props.entries.length}`)
</script>

<template>
  <div class="viewer" @wheel="onWheel">
    <header class="bar">
      <div class="left">
        <button class="btn ghost" title="戻る (Esc)" @click="emit('close')">✕ 閉じる</button>
        <span class="name" :title="entry?.name">{{ entry?.name }}</span>
      </div>
      <div class="right">
        <span class="pos">{{ position }}</span>
        <button class="btn ghost" title="前の画像 (←)" :disabled="index <= 0" @click="go(-1)">←</button>
        <button
          class="btn ghost"
          title="次の画像 (→)"
          :disabled="index >= entries.length - 1"
          @click="go(1)"
        >
          →
        </button>
        <button class="btn ghost on" title="フィット / 等倍 (1)" @click="toggleFit">
          {{ fitMode ? 'フィット' : '等倍' }}
        </button>
        <span class="zoom">{{ Math.round(effectiveZoom * 100) }}%</span>
        <button class="btn ghost" title="縮小 (-)" @click="setZoom(effectiveZoom / 1.25)">−</button>
        <button class="btn ghost" title="拡大 (+)" @click="setZoom(effectiveZoom * 1.25)">＋</button>
        <button class="btn ghost" title="リセット (0)" @click="resetView">リセット</button>
      </div>
    </header>

    <div
      ref="stage"
      class="stage"
      :class="{ grabbing: !fitMode }"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
      @dblclick="toggleFit"
    >
      <div v-if="loading" class="hint">読み込み中…</div>
      <div v-else-if="failed" class="hint bad">{{ failed }}</div>
      <img
        v-show="!failed"
        :key="src"
        :src="src"
        :style="imageStyle"
        alt=""
        draggable="false"
        @load="onImageLoad"
        @error="onImageError"
      />
    </div>
  </div>
</template>

<style scoped>
.viewer {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  flex-direction: column;
  background: rgba(10, 10, 12, 0.97);
  backdrop-filter: blur(2px);
}
.bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  font-size: 12px;
}
.left,
.right {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.name {
  color: var(--fg-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 40vw;
}
.pos,
.zoom {
  color: var(--fg-dim);
  font-variant-numeric: tabular-nums;
  min-width: 48px;
  text-align: center;
}
.stage {
  position: relative;
  flex: 1;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: default;
  touch-action: none;
}
.stage.grabbing {
  cursor: grab;
}
.stage img {
  max-width: none;
  max-height: none;
  transform-origin: center center;
  user-select: none;
  will-change: transform;
  image-rendering: auto;
}
.hint {
  position: absolute;
  color: var(--fg-dim);
  font-size: 12px;
}
.hint.bad {
  color: var(--warn);
}
.btn.on {
  background: var(--accent-soft);
  border-color: var(--accent);
}
</style>
