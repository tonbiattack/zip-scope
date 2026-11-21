import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import App from './App.vue'
import type { ArchiveSummary, Settings } from './lib/types'

// A fake backend that records calls, so the orchestration logic can be tested
// without a Wails runtime.
function makeSettings(over: Partial<Settings> = {}): Settings {
  return {
    root: '',
    selectedArchive: '',
    selectedEntry: 0,
    scrollTop: 0,
    thumbSize: 192,
    sortMode: 'name',
    sortDesc: false,
    search: '',
    cacheLimitBytes: 2 << 30,
    memoryLimitBytes: 256 << 20,
    workers: 4,
    prefetchAhead: 2,
    maxEntryBytes: 128 << 20,
    maxPixels: 40_000_000,
    maxEntries: 100_000,
    windowWidth: 1440,
    windowHeight: 900,
    ...over,
  }
}

function makeArchive(over: Partial<ArchiveSummary> = {}): ArchiveSummary {
  return {
    id: 1,
    path: 'C:\\pics\\book.zip',
    name: 'book.zip',
    size: 4096,
    modTime: Date.now(),
    count: 3,
    status: 'ok',
    coverOrdinal: 0,
    coverWidth: 192,
    ...over,
  }
}

interface Fake {
  calls: string[]
  api: Record<string, unknown>
}

function installFake(opts: { settings?: Settings; archives?: ArchiveSummary[] } = {}): Fake {
  const calls: string[] = []
  const settings = opts.settings ?? makeSettings()
  const archives = opts.archives ?? [makeArchive()]

  const api = {
    GetSettings: vi.fn(async () => {
      calls.push('GetSettings')
      return settings
    }),
    UpdateSettings: vi.fn(async (patch: Partial<Settings>) => {
      calls.push('UpdateSettings')
      return { ...settings, ...patch }
    }),
    GetCacheStats: vi.fn(async () => ({
      thumbCount: 0,
      thumbBytes: 0,
      thumbLimit: 2 << 30,
      archiveCount: archives.length,
      memoryUsed: 0,
      memoryLimit: 256 << 20,
      memoryItems: 0,
      root: 'C:\\cache',
    })),
    GetPrefetchStats: vi.fn(async () => ({
      scheduler: { completed: 0, discarded: 0, deduped: 0, pending: 0, workers: 4 },
      memory: { limit: 0, used: 0, items: 0, hits: 0, misses: 0, evicted: 0 },
      decodeBytes: 0,
    })),
    StartScan: vi.fn(async (root: string) => {
      calls.push(`StartScan:${root}`)
      return 1
    }),
    CancelScan: vi.fn(async () => {
      calls.push('CancelScan')
    }),
    IndexArchive: vi.fn(async () => archives[0]),
    GetArchiveEntries: vi.fn(async (id: number) => {
      calls.push(`GetArchiveEntries:${id}`)
      return {
        id,
        path: archives[0].path,
        name: archives[0].name,
        status: 'ok',
        entries: [
          { ordinal: 0, name: 'page1.jpg', crc32: 1, size: 100, encrypted: false },
          { ordinal: 1, name: 'page2.jpg', crc32: 2, size: 100, encrypted: false },
          { ordinal: 2, name: 'page3.jpg', crc32: 3, size: 100, encrypted: false },
        ],
      }
    }),
    BumpSelection: vi.fn(async () => {
      calls.push('BumpSelection')
      return 1
    }),
    PrefetchImage: vi.fn(async () => {
      calls.push('PrefetchImage')
    }),
    PrefetchFull: vi.fn(async () => {
      calls.push('PrefetchFull')
    }),
    WarmArchive: vi.fn(async () => {
      calls.push('WarmArchive')
    }),
    PickFolder: vi.fn(async () => ''),
    PickArchive: vi.fn(async () => ''),
    RevealInExplorer: vi.fn(async () => undefined),
    ClearCache: vi.fn(async () => {
      calls.push('ClearCache')
    }),
    ClearThumbnails: vi.fn(async () => {
      calls.push('ClearThumbnails')
    }),
    EvictCache: vi.fn(async () => 0),
    ThumbSizeOptions: vi.fn(async () => [96, 128, 192, 256, 384, 512]),
    Thumbnail: vi.fn(async () => ({})),
    FullImage: vi.fn(async () => ({})),
    CheckEntry: vi.fn(async () => ({ ordinal: 0, ok: true })),
    ArchiveCover: vi.fn(async () => ({})),
    GetOrganizeCapabilities: vi.fn(async () => ({
      move: true,
      rename: true,
      recycleBin: true,
      permanentDelete: false,
    })),
    MoveArchive: vi.fn(async () => ({ source: '', destination: '', recycleBin: false, archiveId: 0 })),
    RenameArchive: vi.fn(async () => ({ source: '', destination: '', recycleBin: false, archiveId: 0 })),
    DeleteArchiveToRecycleBin: vi.fn(async () => ({ source: '', recycleBin: true, archiveId: 0 })),
    RefreshArchive: vi.fn(async () => archives[0]),
    PruneMissingArchives: vi.fn(async () => 0),
  }

  ;(window as unknown as { go: unknown }).go = { app: { App: api } }
  return { calls, api }
}

function installRuntime() {
  const handlers = new Map<string, (payload: unknown) => void>()
  ;(window as unknown as { runtime: unknown }).runtime = {
    EventsOn: (name: string, cb: (payload: unknown) => void) => {
      handlers.set(name, cb)
    },
    EventsOff: (name: string) => {
      handlers.delete(name)
    },
    WindowSetTitle: () => undefined,
  }
  return {
    emit(name: string, payload: unknown) {
      handlers.get(name)?.(payload)
    },
  }
}

beforeEach(() => {
  delete (window as unknown as { go?: unknown }).go
  delete (window as unknown as { runtime?: unknown }).runtime
})

// mountApp installs the fakes, mounts the app and pushes one archive into the
// list through the same event the scanner uses.
async function mountApp(opts: { settings?: Settings; archives?: ArchiveSummary[] } = {}) {
  const fake = installFake(opts)
  const rt = installRuntime()
  const w = mount(App)
  await flushPromises()
  for (const a of opts.archives ?? [makeArchive()]) {
    rt.emit('scan:file', { archive: a })
  }
  await flushPromises()
  return { w, fake, rt }
}

describe('App', () => {
  it('shows a message when the backend is unavailable', async () => {
    const w = mount(App)
    await flushPromises()
    expect(w.text()).toContain('バックエンドに接続できません')
  })

  it('loads settings on mount', async () => {
    const fake = installFake()
    installRuntime()
    mount(App)
    await flushPromises()
    expect(fake.calls).toContain('GetSettings')
  })

  it('rescans the saved root on startup', async () => {
    const fake = installFake({ settings: makeSettings({ root: 'C:\\pics' }) })
    installRuntime()
    mount(App)
    await flushPromises()
    expect(fake.calls).toContain('StartScan:C:\\pics')
  })

  it('does not scan when no root is saved', async () => {
    const fake = installFake()
    installRuntime()
    mount(App)
    await flushPromises()
    expect(fake.calls.some((c) => c.startsWith('StartScan'))).toBe(false)
  })

  it('adds archives as scan:file events arrive', async () => {
    installFake()
    const rt = installRuntime()
    const w = mount(App)
    await flushPromises()

    rt.emit('scan:file', { archive: makeArchive({ id: 5, name: 'new.zip' }) })
    await flushPromises()
    expect(w.text()).toContain('new.zip')
  })

  it('shows the scan summary when the walk finishes', async () => {
    installFake()
    const rt = installRuntime()
    const w = mount(App)
    await flushPromises()

    rt.emit('scan:done', {
      result: { root: 'C:\\pics', files: 42, dirs: 7, skipped: 1, cancelled: false, durationMs: 1500, gen: 1 },
    })
    await flushPromises()
    expect(w.text()).toContain('42 ZIP')
  })

  it('reports a cancelled scan', async () => {
    installFake()
    const rt = installRuntime()
    const w = mount(App)
    await flushPromises()

    rt.emit('scan:done', {
      result: { root: 'C:\\pics', files: 3, dirs: 1, skipped: 0, cancelled: true, durationMs: 100, gen: 1 },
    })
    await flushPromises()
    expect(w.text()).toContain('走査を中止しました')
  })

  it('opens an archive when a row is selected', async () => {
    const { w, fake } = await mountApp()
    await w.find('.row').trigger('click')
    await flushPromises()
    expect(fake.calls).toContain('GetArchiveEntries:1')
  })

  it('bumps the prefetch epoch when the selection changes', async () => {
    const { w, fake } = await mountApp()
    await w.find('.row').trigger('click')
    await flushPromises()
    expect(fake.calls).toContain('BumpSelection')
  })

  it('opens the settings panel', async () => {
    const { w } = await mountApp()
    const settingsButton = w.findAll('.btn').find((b) => b.text() === '設定')
    await settingsButton?.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('キャッシュ')
  })

  it('persists the thumbnail size when the slider moves', async () => {
    const { w, fake } = await mountApp()
    await w.find('.row').trigger('click')
    await flushPromises()

    const slider = w.find('input[type=range]')
    await slider.setValue('256')
    await new Promise((r) => setTimeout(r, 400))
    await flushPromises()
    expect(fake.calls).toContain('UpdateSettings')
  })

  it('opens the viewer on double click and closes on Escape', async () => {
    const { w } = await mountApp()
    await w.find('.row').trigger('click')
    await flushPromises()

    await w.find('.cell').trigger('dblclick')
    await flushPromises()
    expect(w.find('.viewer').exists()).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(w.find('.viewer').exists()).toBe(false)
  })

  it('prefetches the full image when the viewer opens', async () => {
    const { w, fake } = await mountApp()
    await w.find('.row').trigger('click')
    await flushPromises()
    await w.find('.cell').trigger('dblclick')
    await flushPromises()
    expect(fake.calls).toContain('PrefetchFull')
  })

  it('navigates between images in the viewer', async () => {
    const { w } = await mountApp()
    await w.find('.row').trigger('click')
    await flushPromises()
    await w.find('.cell').trigger('dblclick')
    await flushPromises()

    expect(w.find('.pos').text()).toBe('1 / 3')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }))
    await flushPromises()
    expect(w.find('.pos').text()).toBe('2 / 3')
  })

  it('clears the cache from the settings panel', async () => {
    const { w, fake } = await mountApp()
    const settingsButton = w.findAll('.btn').find((b) => b.text() === '設定')
    await settingsButton?.trigger('click')
    await flushPromises()

    const clear = w.findAll('.btn').find((b) => b.text() === 'すべて消去')
    await clear?.trigger('click')
    await flushPromises()
    expect(fake.calls).toContain('ClearCache')
  })

  it('replaces the row after a rename instead of leaving a stale one', async () => {
    const { w, fake, rt } = await mountApp()
    await w.find('.row').trigger('click')
    await flushPromises()

    // Open the context menu and choose rename.
    await w.find('.row').trigger('contextmenu')
    await flushPromises()
    const rename = w.findAll('.item').find((b) => b.text() === '名前を変更…')
    await rename?.trigger('click')
    await flushPromises()

    // The fake backend reports the new path.
    const renamed = makeArchive({ id: 2, path: 'C:\\pics\\renamed.zip', name: 'renamed.zip' })
    ;(fake.api as { RenameArchive: unknown }).RenameArchive = vi.fn(async () => ({
      source: 'C:\\pics\\book.zip',
      destination: 'C:\\pics\\renamed.zip',
      recycleBin: false,
      archiveId: 1,
    }))
    ;(fake.api as { RefreshArchive: unknown }).RefreshArchive = vi.fn(async () => renamed)
    ;(fake.api as { GetArchiveEntries: unknown }).GetArchiveEntries = vi.fn(async (id: number) => ({
      id,
      path: renamed.path,
      name: renamed.name,
      status: 'ok',
      entries: [{ ordinal: 0, name: 'page1.jpg', crc32: 1, size: 100, encrypted: false }],
    }))

    await w.find('.field').setValue('renamed')
    const confirm = w.findAll('.actions .btn').find((b) => b.text() === '変更')
    await confirm?.trigger('click')
    await flushPromises()

    const names = w.findAll('.name').map((n) => n.text())
    // The old row must be gone and the new one present exactly once.
    expect(names).not.toContain('book.zip')
    expect(names.filter((n) => n === 'renamed.zip')).toHaveLength(1)
    void rt
  })

  it('removes the row after a delete', async () => {
    const { w, fake } = await mountApp()
    await w.find('.row').trigger('click')
    await flushPromises()

    ;(fake.api as { DeleteArchiveToRecycleBin: unknown }).DeleteArchiveToRecycleBin = vi.fn(
      async () => ({ source: 'C:\\pics\\book.zip', recycleBin: true, archiveId: 1 }),
    )

    await w.find('.row').trigger('contextmenu')
    await flushPromises()
    const del = w.findAll('.item').find((b) => b.text() === 'ごみ箱へ移動')
    await del?.trigger('click')
    await flushPromises()

    const confirm = w.findAll('.actions .btn').find((b) => b.text() === 'ごみ箱へ移動')
    await confirm?.trigger('click')
    await flushPromises()

    expect(w.findAll('.row')).toHaveLength(0)
  })
})
