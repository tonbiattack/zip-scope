<script setup lang="ts">
// Top-level orchestration.
//
// The app behaves like a small file browser: a folder shows its archives, and
// opening one descends into it to show its images. A navigation stack provides
// back/forward, and the address bar reflects the current location.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import AddressBar from './components/AddressBar.vue'
import CommandBar from './components/CommandBar.vue'
import FileList, { type ViewMode } from './components/FileList.vue'
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

// --- state ----------------------------------------------------------------

const settings = ref<Settings | null>(null)
const archives = ref<ArchiveSummary[]>([])
const detail = shallowRef<ArchiveDetail | null>(null)
const detailLoading = ref(false)
const viewerIndex = ref<number | null>(null)
const scanning = ref(false)
const scanInfo = ref<ScanResult | null>(null)
const skips = ref<string[]>([])
const showSettings = ref(false)
const cacheStats = ref<CacheStats | null>(null)
const prefetchStats = ref<PrefetchStats | null>(null)
const toast = ref<string | null>(null)
const dragging = ref(false)

// Navigation: the current folder, and the archive we are inside (if any).
const currentFolder = ref('')
const openArchive = ref<ArchiveSummary | null>(null)
const selectedKeys = ref<string[]>([])

// History stacks hold the location, not the selection.
interface Location {
  folder: string
  archivePath: string | null
}
const history = ref<Location[]>([])
const historyIndex = ref(-1)

// Organize state.
const menu = ref<{ archive: ArchiveSummary; x: number; y: number } | null>(null)
const organizeCaps = ref<OrganizeCapabilities | null>(null)
type DialogKind = 'rename' | 'move' | 'delete'
const dialog = ref<{ kind: DialogKind; archive: ArchiveSummary } | null>(null)

const listRef = ref<InstanceType<typeof FileList> | null>(null)

let toastTimer: number | undefined
function notify(msg: string) {
  toast.value = msg
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = null), 3200)
}

// --- derived --------------------------------------------------------------

const sortMode = computed(() => settings.value?.sortMode ?? 'name')
const sortDesc = computed(() => settings.value?.sortDesc ?? false)
const search = computed(() => settings.value?.search ?? '')
const viewMode = computed<ViewMode>(() => settings.value?.viewMode ?? 'details')
const iconSize = computed(() => settings.value?.iconSize ?? 128)

const insideArchive = computed(() => openArchive.value != null)
const entries = computed<EntryInfo[]>(() => detail.value?.entries ?? [])

const currentPath = computed(() =>
  openArchive.value ? openArchive.value.path : currentFolder.value,
)

// The selected archive, when exactly one archive is selected in a folder.
const selectedArchive = computed<ArchiveSummary | null>(() => {
  if (insideArchive.value) return openArchive.value
  if (selectedKeys.value.length !== 1) return null
  const key = selectedKeys.value[0]
  if (!key.startsWith('a:')) return null
  return archives.value.find((a) => `a:${a.id || a.path}` === key) ?? null
})

const canBack = computed(() => historyIndex.value > 0)
const canForward = computed(() => historyIndex.value < history.value.length - 1)
const canUp = computed(() => insideArchive.value || parentOf(currentFolder.value) !== '')

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

// --- navigation -----------------------------------------------------------

function parentOf(path: string): string {
  if (!path) return ''
  const trimmed = path.replace(/[\\/]+$/, '')
  const idx = Math.max(trimmed.lastIndexOf('\\'), trimmed.lastIndexOf('/'))
  if (idx <= 0) return ''
  const parent = trimmed.slice(0, idx)
  // "C:" alone is the drive root; keep the trailing separator.
  return parent.endsWith(':') ? parent + '\\' : parent
}

// pushLocation records a new location in the history, dropping any forward
// entries the way a browser does.
function pushLocation(loc: Location) {
  const cur = history.value[historyIndex.value]
  if (cur && cur.folder === loc.folder && cur.archivePath === loc.archivePath) return
  history.value = history.value.slice(0, historyIndex.value + 1)
  history.value.push(loc)
  historyIndex.value = history.value.length - 1
}

// pendingArchivePath is set when a location names an archive that the scan has
// not reported yet. The scan:file handler consumes it as soon as the archive
// appears, which is what makes "open this ZIP from Explorer" work even though
// the folder has to be scanned first.
const pendingArchivePath = ref<string | null>(null)

async function applyLocation(loc: Location, record = true) {
  if (record) pushLocation(loc)
  selectedKeys.value = []
  viewerIndex.value = null

  if (loc.archivePath) {
    const found = archives.value.find((a) => a.path === loc.archivePath)
    if (found) {
      pendingArchivePath.value = null
      await enterArchive(found, false)
      return
    }
    // The archive is not in the list yet (a fresh scan); remember it and let
    // the scan:file handler open it when it arrives.
    pendingArchivePath.value = loc.archivePath
    openArchive.value = null
    detail.value = null
    currentFolder.value = loc.folder
    if (loc.folder) await startScan(loc.folder, false)
    return
  }

  pendingArchivePath.value = null
  openArchive.value = null
  detail.value = null
  currentFolder.value = loc.folder
  if (loc.folder) await startScan(loc.folder, false)
}

async function goBack() {
  if (!canBack.value) return
  historyIndex.value--
  await applyLocation(history.value[historyIndex.value], false)
}

async function goForward() {
  if (!canForward.value) return
  historyIndex.value++
  await applyLocation(history.value[historyIndex.value], false)
}

async function goUp() {
  if (insideArchive.value) {
    await applyLocation({ folder: currentFolder.value, archivePath: null })
    return
  }
  const parent = parentOf(currentFolder.value)
  if (parent) await applyLocation({ folder: parent, archivePath: null })
}

async function navigateTo(path: string) {
  await applyLocation({ folder: path, archivePath: null })
}

async function refresh() {
  if (insideArchive.value && openArchive.value) {
    const arc = openArchive.value
    await enterArchive(arc, true)
    return
  }
  if (currentFolder.value) await startScan(currentFolder.value, true)
}

// --- scanning -------------------------------------------------------------

async function startScan(root: string, keepSelection = false) {
  if (!root) return
  if (!keepSelection) {
    archives.value = []
    skips.value = []
  }
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
    if (dir) await applyLocation({ folder: dir, archivePath: null })
  } catch (err) {
    notify(`フォルダを選択できません: ${String(err)}`)
  }
}

async function cancelScan() {
  await backend.cancelScan()
}

function upsertArchive(summary: ArchiveSummary) {
  const idx = archives.value.findIndex((a) => a.path === summary.path)
  if (idx >= 0) archives.value[idx] = summary
  else archives.value.push(summary)
}

// --- entering an archive --------------------------------------------------

async function enterArchive(summary: ArchiveSummary, record = true) {
  if (summary.id === 0) {
    notify(`この ZIP は開けません: ${summary.error ?? summary.status}`)
    return
  }
  if (record) {
    pushLocation({ folder: currentFolder.value, archivePath: summary.path })
  }
  openArchive.value = summary
  detailLoading.value = true
  await backend.bumpSelection()
  try {
    const d = await backend.getArchiveEntries(summary.id)
    detail.value = d
    // Restore the saved image when this is the archive the user left off in.
    const saved = settings.value
    if (saved && saved.selectedArchive === summary.path && d.entries.length > 0) {
      const found = d.entries.find((e) => e.ordinal === saved.selectedEntry)
      selectedKeys.value = [`e:${found ? found.ordinal : d.entries[0].ordinal}`]
    } else if (d.entries.length > 0) {
      selectedKeys.value = [`e:${d.entries[0].ordinal}`]
    }
    await nextTick()
    listRef.value?.measure()
    void backend.warmArchive(summary.id, 24, iconSize.value)
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

// --- selection and opening ------------------------------------------------

function onSelect(keys: string[], anchor: string) {
  selectedKeys.value = keys
  if (insideArchive.value && keys.length === 1 && keys[0].startsWith('e:')) {
    const ord = Number(keys[0].slice(2))
    patchSettings({ selectedEntry: ord })
    void backend.bumpSelection()
    const id = openArchive.value?.id ?? 0
    void backend.prefetchImage(id, ord, iconSize.value, Priority.Selected)
    const idx = entries.value.findIndex((e) => e.ordinal === ord)
    const ahead = settings.value?.prefetchAhead ?? 2
    for (let d = 1; d <= ahead; d++) {
      for (const j of [idx - d, idx + d]) {
        const e = entries.value[j]
        if (!e) continue
        void backend.prefetchImage(id, e.ordinal, iconSize.value, Priority.Neighbour)
      }
    }
  }
  void anchor
}

async function onOpen(key: string) {
  if (key.startsWith('a:')) {
    const arc = archives.value.find((a) => `a:${a.id || a.path}` === key)
    if (arc) await enterArchive(arc)
    return
  }
  if (key.startsWith('e:')) {
    const ord = Number(key.slice(2))
    const idx = entries.value.findIndex((e) => e.ordinal === ord)
    if (idx >= 0) openViewer(idx)
  }
}

function openViewer(index: number) {
  const e = entries.value[index]
  if (!e) return
  viewerIndex.value = index
  selectedKeys.value = [`e:${e.ordinal}`]
  patchSettings({ selectedEntry: e.ordinal })
  void backend.prefetchFull(openArchive.value?.id ?? 0, e.ordinal, Priority.Selected)
}

function closeViewer() {
  viewerIndex.value = null
}

function navigateViewer(index: number) {
  const e = entries.value[index]
  if (!e) return
  viewerIndex.value = index
  selectedKeys.value = [`e:${e.ordinal}`]
  patchSettings({ selectedEntry: e.ordinal })
  const id = openArchive.value?.id ?? 0
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
  const cur = list.findIndex((a) => a.path === openArchive.value?.path)
  if (cur < 0) return
  const next = cur + delta
  if (next < 0 || next >= list.length) return
  await enterArchive(list[next], false)
  if (entries.value.length > 0) {
    viewerIndex.value = 0
    selectedKeys.value = [`e:${entries.value[0].ordinal}`]
  }
}

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

// --- organize -------------------------------------------------------------

function openMenu(key: string, x: number, y: number) {
  const arc = archives.value.find((a) => `a:${a.id || a.path}` === key)
  if (!arc) return
  menu.value = { archive: arc, x, y }
}

function startDialog(kind: DialogKind) {
  const target = menu.value?.archive ?? selectedArchive.value
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

function removeArchive(path: string) {
  const before = archives.value.length
  archives.value = archives.value.filter((a) => a.path !== path)
  if (scanInfo.value && archives.value.length < before) {
    scanInfo.value = { ...scanInfo.value, files: Math.max(0, scanInfo.value.files - 1) }
  }
  if (openArchive.value?.path === path) {
    openArchive.value = null
    detail.value = null
    selectedKeys.value = []
    viewerIndex.value = null
  }
}

async function rescanAfterOrganize(oldPath: string, newPath: string) {
  const wasOpen = openArchive.value?.path === oldPath
  try {
    const sum = await backend.refreshArchive(newPath)
    archives.value = archives.value.filter((a) => a.path !== oldPath && a.path !== newPath)
    archives.value.push(sum)
    if (wasOpen) {
      await enterArchive(sum, false)
    }
  } catch (err) {
    removeArchive(oldPath)
    notify(`再読み込みに失敗しました: ${String(err)}`)
  }
}

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
  const first = files[0] as File & { path?: string }
  const p = first.path
  if (!p) {
    notify('この環境ではドロップしたパスを取得できません')
    return
  }
  // A dropped archive opens directly; a dropped folder is scanned.
  if (/\.(zip|cbz)$/i.test(p)) {
    const dir = parentOf(p)
    await applyLocation({ folder: dir, archivePath: p })
  } else {
    await applyLocation({ folder: p, archivePath: null })
  }
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

  if (e.altKey && e.key === 'ArrowLeft') {
    e.preventDefault()
    void goBack()
    return
  }
  if (e.altKey && e.key === 'ArrowRight') {
    e.preventDefault()
    void goForward()
    return
  }
  if (e.altKey && e.key === 'ArrowUp') {
    e.preventDefault()
    void goUp()
    return
  }
  if (e.key === 'Backspace') {
    e.preventDefault()
    void goUp()
    return
  }
  if (e.key === 'F5') {
    e.preventDefault()
    void refresh()
    return
  }
  if (e.key === 'Enter' && selectedKeys.value.length === 1) {
    e.preventDefault()
    void onOpen(selectedKeys.value[0])
    return
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    moveSelection(e.key === 'ArrowDown' ? 1 : -1)
  }
}

// moveSelection walks the visible list, which is what the arrow keys do in
// Explorer's details view.
function moveSelection(delta: number) {
  const list = insideArchive.value ? entries.value.map((e) => `e:${e.ordinal}`) : sortedArchives.value.map((a) => `a:${a.id || a.path}`)
  if (list.length === 0) return
  const cur = selectedKeys.value.length === 1 ? list.indexOf(selectedKeys.value[0]) : -1
  const next = Math.min(list.length - 1, Math.max(0, cur + delta))
  selectedKeys.value = [list[next]]
  if (insideArchive.value) {
    const ord = Number(list[next].slice(2))
    patchSettings({ selectedEntry: ord })
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

  window.runtime?.EventsOn('scan:started', () => {
    scanning.value = true
  })
  window.runtime?.EventsOn('scan:file', (payload) => {
    const p = payload as { archive: ArchiveSummary }
    if (!p?.archive) return
    upsertArchive(p.archive)
    // A location that named this archive can now be completed.
    if (pendingArchivePath.value && p.archive.path === pendingArchivePath.value) {
      const target = p.archive
      pendingArchivePath.value = null
      void enterArchive(target, false)
    }
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
    else notify(`${p.result.files} 件の ZIP を ${formatDuration(p.result.durationMs)} で検出しました`)
    void restoreSelection()
  })

  await refreshStats()
  statsTimer = window.setInterval(refreshStats, 4000)

  try {
    organizeCaps.value = await backend.organizeCapabilities()
  } catch {
    organizeCaps.value = null
  }

  // A path from the Explorer context menu wins over the restored session.
  let target = ''
  try {
    target = await backend.startupTarget()
  } catch {
    target = ''
  }

  if (target) {
    // The target may be a folder or an archive; the extension decides which.
    if (/\.(zip|cbz)$/i.test(target)) {
      await applyLocation({ folder: parentOf(target), archivePath: target })
    } else {
      await applyLocation({ folder: target, archivePath: null })
    }
    return
  }

  const s = settings.value
  if (s?.root) {
    await applyLocation({ folder: s.root, archivePath: null })
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('dragover', onDragOver)
  window.removeEventListener('dragleave', onDragLeave)
  window.removeEventListener('drop', onDrop)
  window.clearInterval(statsTimer)
  window.clearTimeout(saveTimer)
  window.clearTimeout(toastTimer)
})

// After a scan finishes, reopen the archive the user was in.
async function restoreSelection() {
  const s = settings.value
  if (!s?.selectedArchive || openArchive.value) return
  const target = archives.value.find((a) => a.path === s.selectedArchive)
  if (target && history.value[historyIndex.value]?.archivePath === s.selectedArchive) {
    await enterArchive(target, false)
  }
}

const viewerEntry = computed(() => (viewerIndex.value != null ? entries.value[viewerIndex.value] : null))
</script>

<template>
  <div class="app" @dragover.prevent="onDragOver" @drop.prevent="onDrop">
    <AddressBar
      :path="currentPath"
      :inside-archive="insideArchive"
      :can-back="canBack"
      :can-forward="canForward"
      :can-up="canUp"
      :scanning="scanning"
      @back="goBack"
      @forward="goForward"
      @up="goUp"
      @refresh="refresh"
      @navigate="navigateTo"
      @pick-folder="pickFolder"
    />

    <CommandBar
      :inside-archive="insideArchive"
      :has-selection="selectedKeys.length > 0"
      :selection-count="selectedKeys.length"
      :view-mode="viewMode"
      :icon-size="iconSize"
      :scanning="scanning"
      :can-recycle="organizeCaps?.recycleBin ?? false"
      @pick-folder="pickFolder"
      @open="selectedKeys.length === 1 && onOpen(selectedKeys[0])"
      @reveal="selectedArchive && backend.reveal(selectedArchive.path)"
      @rename="startDialog('rename')"
      @move="startDialog('move')"
      @delete="startDialog('delete')"
      @cancel-scan="cancelScan"
      @update:view-mode="(v) => patchSettings({ viewMode: v })"
      @update:icon-size="(v) => patchSettings({ iconSize: v })"
      @settings="showSettings = true"
    />

    <FileList
      ref="listRef"
      :archives="sortedArchives"
      :entries="entries"
      :inside-archive="insideArchive"
      :archive-id="openArchive?.id ?? null"
      :view-mode="viewMode"
      :icon-size="iconSize"
      :sort-mode="sortMode"
      :sort-desc="sortDesc"
      :search="search"
      :loading="detailLoading"
      :status="detail?.status ?? 'ok'"
      :error="detail?.error"
      :selected="selectedKeys"
      @select="onSelect"
      @open="onOpen"
      @context-menu="openMenu"
      @update:sort-mode="(v) => patchSettings({ sortMode: v })"
      @update:sort-desc="(v) => patchSettings({ sortDesc: v })"
      @update:view-mode="(v) => patchSettings({ viewMode: v })"
      @update:icon-size="(v) => patchSettings({ iconSize: v })"
      @prefetch="(ord) => backend.prefetchImage(openArchive?.id ?? 0, ord, iconSize, Priority.Visible)"
    />

    <footer class="statusbar">
      <span v-if="scanning" class="pulse">走査中…</span>
      <span v-else-if="insideArchive">
        {{ entries.length }} 枚
        <template v-if="detail?.status !== 'ok'"> · {{ detail?.status }}</template>
      </span>
      <span v-else-if="scanInfo">
        {{ scanInfo.files }} ZIP / {{ scanInfo.dirs }} フォルダ
        <template v-if="scanInfo.skipped"> · スキップ {{ scanInfo.skipped }}</template>
        · {{ formatDuration(scanInfo.durationMs) }}
      </span>
      <span v-else>準備完了</span>
      <span class="spacer" />
      <input
        class="search"
        type="search"
        :placeholder="insideArchive ? '画像を検索' : 'ZIP を検索'"
        :value="search"
        @input="patchSettings({ search: ($event.target as HTMLInputElement).value })"
      />
    </footer>

    <ImageViewer
      v-if="viewerIndex != null && openArchive && viewerEntry"
      :archive-id="openArchive.id"
      :archive-name="openArchive.name"
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
.statusbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 3px 10px;
  border-top: 1px solid var(--border);
  background: var(--panel);
  font-size: 11px;
  color: var(--fg-dim);
  min-height: 28px;
  flex: 0 0 auto;
}
.spacer {
  flex: 1;
}
.pulse {
  color: var(--accent);
}
.search {
  width: 200px;
  padding: 3px 8px;
  border-radius: 5px;
  border: 1px solid var(--border);
  background: var(--bg);
  color: var(--fg);
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
