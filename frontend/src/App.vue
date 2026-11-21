<script setup lang="ts">
// Top-level orchestration: scan lifecycle, selection, prefetch, persistence and
// keyboard shortcuts.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import ArchiveList from './components/ArchiveList.vue'
import ThumbGrid from './components/ThumbGrid.vue'
import ImageViewer from './components/ImageViewer.vue'
import SettingsPanel from './components/SettingsPanel.vue'
import ArchiveMenu from './components/ArchiveMenu.vue'
import PromptDialog from './components/PromptDialog.vue'
import { backend, Priority } from './lib/backend'
import type {
  ArchiveDetail,
  ArchiveSummary,
  CacheStats,
  EntryInfo,
  OrganizeCapabilities,
  PrefetchStats,
  ScanResult,
  Settings,
} from './lib/types'
import { formatDuration } from './lib/format'

const settings = ref<Settings | null>(null)
const archives = ref<ArchiveSummary[]>([])
const selectedId = ref<number | null>(null)
const detail = shallowRef<ArchiveDetail | null>(null)
const detailLoading = ref(false)
const selectedOrdinal = ref<number | null>(null)
const viewerIndex = ref<number | null>(null)
const scanning = ref(false)
const scanInfo = ref<ScanResult | null>(null)
const skips = ref<string[]>([])
const showSettings = ref(false)
const cacheStats = ref<CacheStats | null>(null)
const prefetchStats = ref<PrefetchStats | null>(null)
const toast = ref<string | null>(null)
const dragging = ref(false)

// Organize state: the context menu target and the active dialog.
const menu = ref<{ archive: ArchiveSummary; x: number; y: number } | null>(null)
const organizeCaps = ref<OrganizeCapabilities | null>(null)
type DialogKind = 'rename' | 'move' | 'delete'
const dialog = ref<{ kind: DialogKind; archive: ArchiveSummary } | null>(null)

const listRef = ref<InstanceType<typeof ArchiveList> | null>(null)
const gridRef = ref<InstanceType<typeof ThumbGrid> | null>(null)

let toastTimer: number | undefined
function notify(msg: string) {
  toast.value = msg
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = null), 3200)
}

const thumbSize = computed(() => settings.value?.thumbSize ?? 192)
const sortMode = computed(() => settings.value?.sortMode ?? 'name')
const sortDesc = computed(() => settings.value?.sortDesc ?? false)
const search = computed(() => settings.value?.search ?? '')

const entries = computed<EntryInfo[]>(() => detail.value?.entries ?? [])

const selectedArchive = computed(() =>
  archives.value.find((a) => a.id === selectedId.value) ?? null,
)

// --- settings -------------------------------------------------------------

async function loadSettings() {
  settings.value = await backend.getSettings()
}

let saveTimer: number | undefined
function patchSettings(patch: Partial<Settings>) {
  if (!settings.value) return
  settings.value = { ...settings.value, ...patch }
  window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(async () => {
    try {
      settings.value = await backend.updateSettings(patch)
    } catch (err) {
      notify(`設定を保存できません: ${String(err)}`)
    }
  }, 250)
}

// --- scanning -------------------------------------------------------------

async function startScan(root: string) {
  if (!root) return
  archives.value = []
  selectedId.value = null
  detail.value = null
  selectedOrdinal.value = null
  skips.value = []
  scanning.value = true
  scanInfo.value = null
  try {
    await backend.startScan(root)
    patchSettings({ root })
  } catch (err) {
    scanning.value = false
    notify(`走査を開始できません: ${String(err)}`)
  }
}

async function pickFolder() {
  try {
    const dir = await backend.pickFolder()
    if (dir) await startScan(dir)
  } catch (err) {
    notify(`フォルダを選択できません: ${String(err)}`)
  }
}

async function cancelScan() {
  await backend.cancelScan()
}

function upsertArchive(summary: ArchiveSummary) {
  const idx = archives.value.findIndex((a) => a.path === summary.path)
  if (idx >= 0) {
    archives.value[idx] = summary
  } else {
    archives.value.push(summary)
  }
}

// --- selection ------------------------------------------------------------

async function selectArchive(summary: ArchiveSummary) {
  if (summary.id === 0) {
    // The archive could not be indexed; show its error without a grid.
    selectedId.value = null
    detail.value = {
      id: 0,
      path: summary.path,
      name: summary.name,
      status: summary.status,
      error: summary.error,
      entries: [],
    }
    return
  }
  if (selectedId.value === summary.id) return
  selectedId.value = summary.id
  selectedOrdinal.value = null
  viewerIndex.value = null
  detailLoading.value = true
  await backend.bumpSelection()
  try {
    const d = await backend.getArchiveEntries(summary.id)
    detail.value = d
    // Restore the saved entry when it belongs to this archive.
    const saved = settings.value
    if (saved && saved.selectedArchive === summary.path && d.entries.length > 0) {
      const found = d.entries.find((e) => e.ordinal === saved.selectedEntry)
      selectedOrdinal.value = found ? found.ordinal : d.entries[0].ordinal
    } else if (d.entries.length > 0) {
      selectedOrdinal.value = d.entries[0].ordinal
    }
    await nextTick()
    gridRef.value?.measure()
    if (saved && saved.selectedArchive === summary.path && saved.scrollTop > 0) {
      gridRef.value?.restoreScroll(saved.scrollTop)
    }
    // Warm the first screen at low priority so scrolling feels instant.
    void backend.warmArchive(summary.id, 24, thumbSize.value)
    patchSettings({ selectedArchive: summary.path })
  } catch (err) {
    detail.value = {
      id: summary.id,
      path: summary.path,
      name: summary.name,
      status: 'error',
      error: String(err),
      entries: [],
    }
    notify(`ZIP を開けません: ${String(err)}`)
  } finally {
    detailLoading.value = false
  }
}

function selectEntry(entry: EntryInfo, index: number) {
  selectedOrdinal.value = entry.ordinal
  patchSettings({ selectedEntry: entry.ordinal })
  void backend.bumpSelection()
  void backend.prefetchImage(selectedId.value ?? 0, entry.ordinal, thumbSize.value, Priority.Selected)
  // Prefetch the neighbours of the newly selected entry.
  const ahead = settings.value?.prefetchAhead ?? 2
  for (let d = 1; d <= ahead; d++) {
    for (const j of [index - d, index + d]) {
      const e = entries.value[j]
      if (!e) continue
      void backend.prefetchImage(selectedId.value ?? 0, e.ordinal, thumbSize.value, Priority.Neighbour)
    }
  }
}

function openViewer(entry: EntryInfo, index: number) {
  selectedOrdinal.value = entry.ordinal
  viewerIndex.value = index
  void backend.prefetchFull(selectedId.value ?? 0, entry.ordinal, Priority.Selected)
}

function closeViewer() {
  viewerIndex.value = null
  // Persist where the user stopped.
  const top = gridRef.value?.currentScroll() ?? 0
  patchSettings({ scrollTop: top })
}

function navigateViewer(index: number) {
  const e = entries.value[index]
  if (!e) return
  viewerIndex.value = index
  selectedOrdinal.value = e.ordinal
  patchSettings({ selectedEntry: e.ordinal })
  const id = selectedId.value ?? 0
  void backend.prefetchFull(id, e.ordinal, Priority.Selected)
  const ahead = settings.value?.prefetchAhead ?? 2
  for (let d = 1; d <= ahead; d++) {
    for (const j of [index - d, index + d]) {
      const n = entries.value[j]
      if (!n) continue
      void backend.prefetchFull(id, n.ordinal, Priority.Neighbour)
    }
  }
}

// Ctrl+Left/Right moves between archives while the viewer is open.
async function navigateArchive(delta: number) {
  const list = sortedArchives.value
  const cur = list.findIndex((a) => a.id === selectedId.value)
  if (cur < 0) return
  const next = cur + delta
  if (next < 0 || next >= list.length) return
  await selectArchive(list[next])
  if (entries.value.length > 0) {
    viewerIndex.value = 0
    selectedOrdinal.value = entries.value[0].ordinal
  }
}

// The viewer navigates in the same order the list shows.
const sortedArchives = computed(() => {
  const q = search.value.trim().toLowerCase()
  let list = archives.value
  if (q) list = list.filter((a) => a.name.toLowerCase().includes(q))
  const sorted = [...list]
  const dir = sortDesc.value ? -1 : 1
  sorted.sort((a, b) => {
    if (sortMode.value === 'mtime' && a.modTime !== b.modTime) return (a.modTime - b.modTime) * dir
    if (sortMode.value === 'size' && a.size !== b.size) return (a.size - b.size) * dir
    return a.name.localeCompare(b.name, 'ja', { numeric: true }) * dir
  })
  return sorted
})

// --- cache ----------------------------------------------------------------

async function refreshStats() {
  try {
    cacheStats.value = await backend.getCacheStats()
    prefetchStats.value = await backend.getPrefetchStats()
  } catch {
    // Diagnostics are best-effort.
  }
}

async function clearCache() {
  await backend.clearCache()
  notify('キャッシュを消去しました')
  await refreshStats()
}

async function clearThumbs() {
  await backend.clearThumbnails()
  notify('サムネイルを削除しました')
  await refreshStats()
}

async function evict() {
  const n = await backend.evictCache()
  notify(`${n} 件のサムネイルを整理しました`)
  await refreshStats()
}

// --- organize -------------------------------------------------------------

function openMenu(archive: ArchiveSummary, x: number, y: number) {
  menu.value = { archive, x, y }
}

function startDialog(kind: DialogKind) {
  const target = menu.value?.archive
  if (!target) return
  dialog.value = { kind, archive: target }
  menu.value = null
}

async function confirmDialog(value: string) {
  const d = dialog.value
  if (!d) return
  dialog.value = null
  const { archive, kind } = d
  try {
    if (kind === 'rename') {
      const res = await backend.renameArchive(archive.id, value)
      notify(`名前を変更しました: ${res.destination ?? value}`)
      await rescanAfterOrganize(archive.path, res.destination ?? archive.path)
    } else if (kind === 'move') {
      const dir = await backend.pickFolder()
      if (!dir) return
      const res = await backend.moveArchive(archive.id, dir)
      notify(`移動しました: ${res.destination ?? dir}`)
      await rescanAfterOrganize(archive.path, res.destination ?? archive.path)
    } else if (kind === 'delete') {
      await backend.deleteToRecycleBin(archive.id)
      notify(`ごみ箱へ移動しました: ${archive.name}`)
      removeArchive(archive.path)
    }
  } catch (err) {
    notify(`操作に失敗しました: ${String(err)}`)
  }
}

// removeArchive drops a row from the list and clears the selection if it was
// the one removed.
function removeArchive(path: string) {
  const before = archives.value.length
  archives.value = archives.value.filter((a) => a.path !== path)
  // Keep the status bar honest: the scan summary was taken before the removal.
  if (scanInfo.value && archives.value.length < before) {
    scanInfo.value = { ...scanInfo.value, files: Math.max(0, scanInfo.value.files - 1) }
  }
  if (selectedArchive.value?.path === path) {
    selectedId.value = null
    detail.value = null
    selectedOrdinal.value = null
    viewerIndex.value = null
  }
}

// rescanAfterOrganize replaces the row for oldPath with a freshly indexed row
// for newPath. The archive gets a new id because its cache row was deleted, so
// the old row must be removed by path rather than matched by id.
async function rescanAfterOrganize(oldPath: string, newPath: string) {
  const wasSelected = selectedArchive.value?.path === oldPath
  try {
    const sum = await backend.refreshArchive(newPath)
    // Drop the stale row and any duplicate for the new path.
    archives.value = archives.value.filter((a) => a.path !== oldPath && a.path !== newPath)
    archives.value.push(sum)

    if (wasSelected) {
      selectedId.value = sum.id
      selectedOrdinal.value = null
      viewerIndex.value = null
      detail.value = await backend.getArchiveEntries(sum.id)
      if (detail.value.entries.length > 0) {
        selectedOrdinal.value = detail.value.entries[0].ordinal
      }
      patchSettings({ selectedArchive: newPath, selectedEntry: 0 })
    }
  } catch (err) {
    // The file moved but re-indexing failed: drop the stale row so the list
    // does not show a path that no longer exists.
    removeArchive(oldPath)
    notify(`再読み込みに失敗しました: ${String(err)}`)
  }
}

// --- drag and drop --------------------------------------------------------

function onDragOver(e: DragEvent) {
  e.preventDefault()
  dragging.value = true
}

function onDragLeave() {
  dragging.value = false
}

async function onDrop(e: DragEvent) {
  e.preventDefault()
  dragging.value = false
  const files = Array.from(e.dataTransfer?.files ?? [])
  if (files.length === 0) return
  // The WebView exposes a path on dropped files; a folder drop scans it.
  const first = files[0] as File & { path?: string }
  const p = first.path
  if (!p) {
    notify('この環境ではドロップしたパスを取得できません')
    return
  }
  await startScan(p)
}

// --- keyboard -------------------------------------------------------------

function onKeydown(e: KeyboardEvent) {
  if (viewerIndex.value != null) return // the viewer owns the keyboard
  const target = e.target as HTMLElement | null
  const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA')

  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'o') {
    e.preventDefault()
    void pickFolder()
    return
  }
  if (typing) return

  if (e.key === 'Enter' && selectedOrdinal.value != null) {
    const idx = entries.value.findIndex((x) => x.ordinal === selectedOrdinal.value)
    if (idx >= 0) {
      e.preventDefault()
      openViewer(entries.value[idx], idx)
    }
    return
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    const list = sortedArchives.value
    if (list.length === 0) return
    e.preventDefault()
    const cur = list.findIndex((a) => a.id === selectedId.value)
    const next = Math.min(list.length - 1, Math.max(0, cur + (e.key === 'ArrowDown' ? 1 : -1)))
    if (next !== cur) void selectArchive(list[next])
  }
}

// --- lifecycle ------------------------------------------------------------

let statsTimer: number | undefined

onMounted(async () => {
  if (!backend.available()) {
    notify('バックエンドに接続できません')
    return
  }
  await loadSettings()
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('dragover', onDragOver)
  window.addEventListener('dragleave', onDragLeave)
  window.addEventListener('drop', onDrop)

  window.runtime?.EventsOn('scan:started', (payload) => {
    const p = payload as { root: string }
    scanning.value = true
    notify(`走査中: ${p.root}`)
  })
  window.runtime?.EventsOn('scan:file', (payload) => {
    const p = payload as { archive: ArchiveSummary }
    if (p?.archive) upsertArchive(p.archive)
  })
  window.runtime?.EventsOn('scan:skip', (payload) => {
    const p = payload as { path: string; err: string }
    if (skips.value.length < 200) skips.value.push(`${p.path}: ${p.err}`)
  })
  window.runtime?.EventsOn('scan:done', (payload) => {
    const p = payload as { result: ScanResult; error?: string }
    scanning.value = false
    scanInfo.value = p.result
    if (p.error) notify(`走査エラー: ${p.error}`)
    else if (p.result.cancelled) notify('走査を中止しました')
    else
      notify(
        `${p.result.files} 件の ZIP を ${formatDuration(p.result.durationMs)} で検出しました`,
      )
    // Restore the previous selection once the list is populated.
    void restoreSelection()
  })

  await refreshStats()
  statsTimer = window.setInterval(refreshStats, 4000)

  try {
    organizeCaps.value = await backend.organizeCapabilities()
  } catch {
    organizeCaps.value = null
  }

  // Resume the last session: rescan the saved root and reselect the archive.
  const s = settings.value
  if (s?.root) {
    await startScan(s.root)
  }

  // A path from the Explorer context menu wins over the restored session: scan
  // its folder and select it.
  try {
    const target = await backend.startupTarget()
    if (target) {
      const dir = target.replace(/[\\/][^\\/]*$/, '')
      await startScan(dir)
      await selectByPath(target)
    }
  } catch {
    // A missing target is not fatal; the restored session is already showing.
  }
})

// selectByPath waits for the scan to report the archive, then selects it.
async function selectByPath(path: string) {
  const deadline = Date.now() + 15000
  while (Date.now() < deadline) {
    const found = archives.value.find((a) => a.path.toLowerCase() === path.toLowerCase())
    if (found) {
      await selectArchive(found)
      return
    }
    await new Promise((r) => setTimeout(r, 100))
  }
  notify(`ZIP が見つかりません: ${path}`)
}

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('dragover', onDragOver)
  window.removeEventListener('dragleave', onDragLeave)
  window.removeEventListener('drop', onDrop)
  window.clearInterval(statsTimer)
  window.clearTimeout(saveTimer)
  window.clearTimeout(toastTimer)
})

async function restoreSelection() {
  const s = settings.value
  if (!s?.selectedArchive) return
  const target = archives.value.find((a) => a.path === s.selectedArchive)
  if (target) await selectArchive(target)
}

// Persist the grid scroll position when the user stops scrolling.
let scrollTimer: number | undefined
function onGridScroll() {
  window.clearTimeout(scrollTimer)
  scrollTimer = window.setTimeout(() => {
    const top = gridRef.value?.currentScroll() ?? 0
    patchSettings({ scrollTop: top })
  }, 400)
}

const viewerEntry = computed(() => (viewerIndex.value != null ? entries.value[viewerIndex.value] : null))
</script>

<template>
  <div class="app" @dragover.prevent="onDragOver" @drop.prevent="onDrop">
    <main class="main">
      <ArchiveList
        ref="listRef"
        :archives="archives"
        :selected-id="selectedId"
        :sort-mode="sortMode"
        :sort-desc="sortDesc"
        :search="search"
        :scanning="scanning"
        @select="selectArchive"
        @update:search="(v) => patchSettings({ search: v })"
        @update:sort-mode="(v) => patchSettings({ sortMode: v })"
        @update:sort-desc="(v) => patchSettings({ sortDesc: v })"
        @pick-folder="pickFolder"
        @cancel-scan="cancelScan"
        @reveal="(p) => backend.reveal(p)"
        @context-menu="openMenu"
      />

      <ThumbGrid
        ref="gridRef"
        :archive-id="selectedId"
        :entries="entries"
        :thumb-size="thumbSize"
        :selected-ordinal="selectedOrdinal"
        :loading="detailLoading"
        :status="detail?.status ?? 'ok'"
        :error="detail?.error"
        @select="selectEntry"
        @open="openViewer"
        @prefetch="(ord, pri) => backend.prefetchImage(selectedId ?? 0, ord, thumbSize, pri)"
        @update:thumb-size="(v) => patchSettings({ thumbSize: v })"
        @scroll="onGridScroll"
      />
    </main>

    <footer class="statusbar">
      <span v-if="scanning" class="pulse">走査中…</span>
      <span v-else-if="scanInfo">
        {{ scanInfo.files }} ZIP / {{ scanInfo.dirs }} フォルダ
        <template v-if="scanInfo.skipped"> · スキップ {{ scanInfo.skipped }}</template>
        · {{ formatDuration(scanInfo.durationMs) }}
      </span>
      <span v-else>準備完了</span>
      <span class="spacer" />
      <span v-if="selectedArchive" class="path" :title="selectedArchive.path">
        {{ selectedArchive.path }}
      </span>
      <button class="btn ghost small" @click="showSettings = true">設定</button>
    </footer>

    <ImageViewer
      v-if="viewerIndex != null && selectedId != null && viewerEntry"
      :archive-id="selectedId"
      :archive-name="detail?.name ?? ''"
      :entries="entries"
      :index="viewerIndex"
      @close="closeViewer"
      @navigate="navigateViewer"
      @navigate-archive="navigateArchive"
    />

    <SettingsPanel
      v-if="showSettings && settings"
      :settings="settings"
      :stats="cacheStats"
      :prefetch="prefetchStats"
      @close="showSettings = false"
      @update="patchSettings"
      @clear-cache="clearCache"
      @clear-thumbs="clearThumbs"
      @evict="evict"
    />

    <ArchiveMenu
      v-if="menu"
      :x="menu.x"
      :y="menu.y"
      :archive-name="menu.archive.name"
      :can-recycle="organizeCaps?.recycleBin ?? false"
      @close="menu = null"
      @reveal="backend.reveal(menu.archive.path)"
      @rename="startDialog('rename')"
      @move="startDialog('move')"
      @delete="startDialog('delete')"
    />

    <PromptDialog
      v-if="dialog && dialog.kind === 'rename'"
      title="名前を変更"
      :message="`${dialog.archive.name} の新しい名前を入力してください。`"
      :value="dialog.archive.name"
      prompt
      placeholder="新しいファイル名"
      confirm-label="変更"
      @confirm="confirmDialog"
      @cancel="dialog = null"
    />

    <PromptDialog
      v-if="dialog && dialog.kind === 'move'"
      title="別のフォルダへ移動"
      :message="`${dialog.archive.name} を移動します。移動先のフォルダを選択してください。`"
      confirm-label="フォルダを選択"
      @confirm="confirmDialog"
      @cancel="dialog = null"
    />

    <PromptDialog
      v-if="dialog && dialog.kind === 'delete'"
      title="ごみ箱へ移動"
      :message="`${dialog.archive.name} をごみ箱へ移動します。あとから復元できます。`"
      confirm-label="ごみ箱へ移動"
      danger
      @confirm="confirmDialog"
      @cancel="dialog = null"
    />

    <div v-if="dragging" class="dropzone">ZIP またはフォルダをドロップ</div>
    <div v-if="toast" class="toast">{{ toast }}</div>
  </div>
</template>

<style scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: row;
  min-height: 0;
  min-width: 0;
}
.statusbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 10px;
  border-top: 1px solid var(--border);
  background: var(--panel);
  font-size: 11px;
  color: var(--fg-dim);
  min-height: 26px;
}
.spacer {
  flex: 1;
}
.path {
  max-width: 50vw;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  direction: rtl;
  text-align: left;
}
.pulse {
  color: var(--accent);
}
.small {
  padding: 2px 8px;
  font-size: 11px;
}
.dropzone {
  position: fixed;
  inset: 12px;
  border: 2px dashed var(--accent);
  border-radius: 12px;
  background: rgba(90, 140, 255, 0.08);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  color: var(--fg);
  pointer-events: none;
  z-index: 40;
}
.toast {
  position: fixed;
  bottom: 40px;
  left: 50%;
  transform: translateX(-50%);
  background: var(--panel-strong);
  border: 1px solid var(--border-strong);
  color: var(--fg);
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 12px;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.45);
  z-index: 70;
  max-width: 70vw;
}
</style>
