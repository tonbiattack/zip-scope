<script setup lang="ts">
// Settings panel: cache budget, worker count, cache clearing and diagnostics.
import { computed, ref } from 'vue'
import type { CacheStats, PrefetchStats, Settings } from '../lib/types'
import { formatBytes } from '../lib/format'

const props = defineProps<{
  settings: Settings
  stats: CacheStats | null
  prefetch: PrefetchStats | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'update', patch: Partial<Settings>): void
  (e: 'clear-cache'): void
  (e: 'clear-thumbs'): void
  (e: 'evict'): void
}>()

const busy = ref(false)

const cacheGB = computed(() => (props.settings.cacheLimitBytes / (1 << 30)).toFixed(1))
const memMB = computed(() => Math.round(props.settings.memoryLimitBytes / (1 << 20)))

function setCacheGB(v: string) {
  const gb = Number(v)
  if (!Number.isFinite(gb) || gb <= 0) return
  emit('update', { cacheLimitBytes: Math.round(gb * (1 << 30)) })
}

function setMemMB(v: string) {
  const mb = Number(v)
  if (!Number.isFinite(mb) || mb <= 0) return
  emit('update', { memoryLimitBytes: Math.round(mb * (1 << 20)) })
}

async function run(fn: () => void) {
  busy.value = true
  try {
    fn()
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="backdrop" @click.self="emit('close')">
    <div class="panel">
      <header>
        <h2>設定</h2>
        <button class="btn ghost" @click="emit('close')">✕</button>
      </header>

      <section>
        <h3>キャッシュ</h3>
        <div class="row">
          <label>ディスク上限</label>
          <div class="control">
            <input
              type="number"
              min="0.1"
              step="0.5"
              :value="cacheGB"
              @change="setCacheGB(($event.target as HTMLInputElement).value)"
            />
            <span class="unit">GB</span>
          </div>
        </div>
        <div class="row">
          <label>メモリ上限</label>
          <div class="control">
            <input
              type="number"
              min="32"
              step="32"
              :value="memMB"
              @change="setMemMB(($event.target as HTMLInputElement).value)"
            />
            <span class="unit">MB</span>
          </div>
        </div>
        <div class="row">
          <label>先読みワーカー</label>
          <div class="control">
            <input
              type="number"
              min="1"
              max="16"
              :value="settings.workers"
              @change="emit('update', { workers: Number(($event.target as HTMLInputElement).value) })"
            />
          </div>
        </div>
        <div class="row">
          <label>先読み枚数</label>
          <div class="control">
            <input
              type="number"
              min="0"
              max="8"
              :value="settings.prefetchAhead"
              @change="emit('update', { prefetchAhead: Number(($event.target as HTMLInputElement).value) })"
            />
          </div>
        </div>

        <div v-if="stats" class="stats">
          <div><span>サムネイル</span><b>{{ stats.thumbCount }} 件 / {{ formatBytes(stats.thumbBytes) }}</b></div>
          <div><span>ディスク上限</span><b>{{ formatBytes(stats.thumbLimit) }}</b></div>
          <div><span>登録 ZIP</span><b>{{ stats.archiveCount }}</b></div>
          <div><span>メモリ</span><b>{{ formatBytes(stats.memoryUsed) }} / {{ formatBytes(stats.memoryLimit) }}</b></div>
          <div class="path"><span>保存先</span><b :title="stats.root">{{ stats.root }}</b></div>
        </div>

        <div class="actions">
          <button class="btn" :disabled="busy" @click="run(() => emit('evict'))">LRU 整理</button>
          <button class="btn" :disabled="busy" @click="run(() => emit('clear-thumbs'))">
            サムネイル削除
          </button>
          <button class="btn danger" :disabled="busy" @click="run(() => emit('clear-cache'))">
            すべて消去
          </button>
        </div>
      </section>

      <section v-if="prefetch">
        <h3>先読み</h3>
        <div class="stats">
          <div><span>完了</span><b>{{ prefetch.scheduler.completed }}</b></div>
          <div><span>破棄</span><b>{{ prefetch.scheduler.discarded }}</b></div>
          <div><span>重複統合</span><b>{{ prefetch.scheduler.deduped }}</b></div>
          <div><span>待機</span><b>{{ prefetch.scheduler.pending }}</b></div>
          <div><span>デコード中</span><b>{{ formatBytes(prefetch.decodeBytes) }}</b></div>
        </div>
      </section>

      <section>
        <h3>制限</h3>
        <div class="stats">
          <div><span>1画像の展開上限</span><b>{{ formatBytes(settings.maxEntryBytes) }}</b></div>
          <div><span>画素数上限</span><b>{{ (settings.maxPixels / 1_000_000).toFixed(0) }} MP</b></div>
          <div><span>ZIPあたりエントリ上限</span><b>{{ settings.maxEntries.toLocaleString() }}</b></div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 60;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
}
.panel {
  width: min(560px, 92vw);
  max-height: 86vh;
  overflow-y: auto;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 16px 18px 20px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
h2 {
  font-size: 15px;
  margin: 0;
}
h3 {
  font-size: 12px;
  color: var(--fg-dim);
  margin: 16px 0 8px;
  font-weight: 600;
  letter-spacing: 0.04em;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 4px 0;
  font-size: 12px;
}
.control {
  display: flex;
  align-items: center;
  gap: 6px;
}
.control input {
  width: 90px;
  padding: 4px 6px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--bg);
  color: var(--fg);
  font-size: 12px;
}
.unit {
  color: var(--fg-dim);
  font-size: 11px;
}
.stats {
  margin-top: 10px;
  border-top: 1px solid var(--border);
  padding-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 11px;
}
.stats > div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}
.stats span {
  color: var(--fg-dim);
}
.stats b {
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 60%;
}
.actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
</style>
