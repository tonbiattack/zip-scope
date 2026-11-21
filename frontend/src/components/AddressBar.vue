<script setup lang="ts">
// Explorer-style address bar: back/forward/up navigation plus a breadcrumb of
// the current location. The breadcrumb is built from the path segments so a
// deep folder can be navigated one level at a time.
import { computed } from 'vue'

const props = defineProps<{
  /** Absolute path of the current folder, or the archive's path. */
  path: string
  /** True when the location is inside an archive rather than a folder. */
  insideArchive: boolean
  canBack: boolean
  canForward: boolean
  canUp: boolean
  scanning: boolean
}>()

const emit = defineEmits<{
  (e: 'back'): void
  (e: 'forward'): void
  (e: 'up'): void
  (e: 'refresh'): void
  (e: 'navigate', path: string): void
  (e: 'pick-folder'): void
}>()

interface Crumb {
  label: string
  path: string
}

// crumbs splits the path into clickable segments. A Windows path starts with a
// drive letter, which becomes the first crumb.
const crumbs = computed<Crumb[]>(() => {
  const raw = props.path
  if (!raw) return []
  const parts = raw.split(/[\\/]+/).filter(Boolean)
  const out: Crumb[] = []
  let acc = ''
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i]
    if (i === 0) {
      // "C:" keeps its colon and is followed by a separator.
      acc = part.endsWith(':') ? part + '\\' : part
    } else {
      acc = acc.endsWith('\\') ? acc + part : acc + '\\' + part
    }
    out.push({ label: part, path: acc })
  }
  return out
})

// The last crumb is the current location and is not a link.
const parentCrumbs = computed(() => crumbs.value.slice(0, -1))
const currentCrumb = computed(() => crumbs.value[crumbs.value.length - 1] ?? null)

function go(crumb: Crumb) {
  emit('navigate', crumb.path)
}
</script>

<template>
  <div class="addressbar">
    <div class="nav">
      <button class="navbtn" title="戻る (Alt+←)" :disabled="!canBack" @click="emit('back')">
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M10 3 5 8l5 5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
      <button class="navbtn" title="進む (Alt+→)" :disabled="!canForward" @click="emit('forward')">
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M6 3l5 5-5 5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
      <button class="navbtn" title="上へ (Alt+↑)" :disabled="!canUp" @click="emit('up')">
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M8 12V4M4 8l4-4 4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
    </div>

    <div class="path" :title="path">
      <button class="crumb root" title="フォルダを選択" @click="emit('pick-folder')">
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M2 4.5A1.5 1.5 0 0 1 3.5 3h2.6l1.2 1.5h5.2A1.5 1.5 0 0 1 14 6v5.5A1.5 1.5 0 0 1 12.5 13h-9A1.5 1.5 0 0 1 2 11.5z"
                fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" />
        </svg>
      </button>
      <template v-for="crumb in parentCrumbs" :key="crumb.path">
        <button class="crumb" :title="crumb.path" @click="go(crumb)">{{ crumb.label }}</button>
        <span class="sep">›</span>
      </template>
      <span v-if="currentCrumb" class="crumb current" :title="path">
        <span v-if="insideArchive" class="zip-badge" aria-hidden="true">ZIP</span>
        {{ currentCrumb.label }}
      </span>
    </div>

    <div class="actions">
      <span v-if="scanning" class="scanning" title="走査中">走査中…</span>
      <button class="navbtn" title="最新の情報に更新 (F5)" @click="emit('refresh')">
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
          <path d="M13 8a5 5 0 1 1-1.5-3.5M13 2v3h-3" fill="none" stroke="currentColor"
                stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
    </div>
  </div>
</template>

<style scoped>
.addressbar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  background: var(--panel);
  border-bottom: 1px solid var(--border);
  min-height: 38px;
}
.nav,
.actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: 0 0 auto;
}
.navbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 26px;
  border: 1px solid transparent;
  border-radius: 5px;
  background: transparent;
  color: var(--fg-dim);
  cursor: pointer;
}
.navbtn:hover:not(:disabled) {
  background: var(--hover);
  color: var(--fg);
}
.navbtn:disabled {
  opacity: 0.35;
  cursor: default;
}
.path {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 1px;
  min-width: 0;
  height: 26px;
  padding: 0 4px;
  border: 1px solid var(--border);
  border-radius: 5px;
  background: var(--bg);
  overflow: hidden;
}
.crumb {
  border: none;
  background: transparent;
  color: var(--fg);
  font-size: 12px;
  padding: 2px 5px;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 220px;
  flex: 0 1 auto;
}
.crumb:hover {
  background: var(--hover);
}
.crumb.root {
  display: inline-flex;
  align-items: center;
  color: var(--fg-dim);
  padding: 2px 4px;
}
.crumb.current {
  cursor: default;
  color: var(--fg);
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.crumb.current:hover {
  background: transparent;
}
.sep {
  color: var(--fg-dim);
  font-size: 11px;
  flex: 0 0 auto;
  opacity: 0.7;
}
.zip-badge {
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.04em;
  padding: 1px 4px;
  border-radius: 3px;
  background: var(--accent-soft);
  color: var(--accent);
}
.scanning {
  font-size: 11px;
  color: var(--accent);
  margin-right: 4px;
}
</style>
