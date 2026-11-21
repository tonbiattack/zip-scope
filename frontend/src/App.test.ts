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
    viewMode: 'details',
    iconSize: 128,
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
    StartupTarget: vi.fn(async () => ''),
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

// mountApp installs the fakes, mounts the app and pushes archives into the list
// through the same event the scanner uses.
async function mountApp(opts: { settings?: Settings; archives?: ArchiveSummary[] } = {}) {
  const fake = installFake(opts)
  const rt = installRuntime()
  const w = mount(App, { attachTo: document.body })
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

  it('scans the saved root on startup', async () => {
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
    const { w } = await mountApp()
    expect(w.text()).toContain('book.zip')
  })

  it('shows the scan summary when the walk finishes', async () => {
    const { w, rt } = await mountApp()
    rt.emit('scan:done', {
      result: { root: 'C:\\pics', files: 42, dirs: 7, skipped: 1, cancelled: false, durationMs: 1500, gen: 1 },
    })
    await flushPromises()
    expect(w.text()).toContain('42 ZIP')
  })

  it('reports a cancelled scan', async () => {
    const { w, rt } = await mountApp()
    rt.emit('scan:done', {
      result: { root: 'C:\\pics', files: 3, dirs: 1, skipped: 0, cancelled: true, durationMs: 100, gen: 1 },
    })
    await flushPromises()
    expect(w.text()).toContain('走査を中止しました')
  })

  it('renders the address bar with the current folder', async () => {
    const { w } = await mountApp({ settings: makeSettings({ root: 'C:\\pics' }) })
    expect(w.find('.addressbar').exists()).toBe(true)
    expect(w.text()).toContain('pics')
  })

  it('renders the command bar', async () => {
    const { w } = await mountApp()
    expect(w.find('.commandbar').exists()).toBe(true)
    expect(w.text()).toContain('フォルダ')
  })

  it('shows archives in the details view by default', async () => {
    const { w } = await mountApp()
    expect(w.find('.item.details').exists()).toBe(true)
  })

  it('opens an archive on double click and shows its images', async () => {
    const { w, fake } = await mountApp()
    await w.find('.item').trigger('dblclick')
    await flushPromises()
    expect(fake.calls).toContain('GetArchiveEntries:1')
    // The address bar now shows the archive with a ZIP badge.
    expect(w.find('.zip-badge').exists()).toBe(true)
    expect(w.text()).toContain('page1.jpg')
  })

  it('goes back to the folder after entering an archive', async () => {
    const { w } = await mountApp({ settings: makeSettings({ root: 'C:\\pics' }) })
    await w.find('.item').trigger('dblclick')
    await flushPromises()
    expect(w.find('.zip-badge').exists()).toBe(true)

    // The back button becomes enabled and returns to the folder.
    const back = w.findAll('.navbtn')[0]
    expect(back.attributes('disabled')).toBeUndefined()
    await back.trigger('click')
    await flushPromises()
    expect(w.find('.zip-badge').exists()).toBe(false)
  })

  it('goes up from an archive to its folder', async () => {
    const { w } = await mountApp({ settings: makeSettings({ root: 'C:\\pics' }) })
    await w.find('.item').trigger('dblclick')
    await flushPromises()
    const up = w.findAll('.navbtn')[2]
    await up.trigger('click')
    await flushPromises()
    expect(w.find('.zip-badge').exists()).toBe(false)
  })

  it('switches between view modes', async () => {
    const { w } = await mountApp()
    const viewButtons = w.findAll('.viewbtn')
    await viewButtons[1].trigger('click')
    await flushPromises()
    expect(w.find('.item.medium').exists()).toBe(true)

    await viewButtons[2].trigger('click')
    await flushPromises()
    expect(w.find('.item.large').exists()).toBe(true)
  })

  it('selects an item on click', async () => {
    const { w } = await mountApp()
    await w.find('.item').trigger('click')
    await flushPromises()
    expect(w.find('.item').classes()).toContain('selected')
  })

  it('opens the viewer on double click inside an archive', async () => {
    const { w } = await mountApp()
    await w.find('.item').trigger('dblclick')
    await flushPromises()

    const image = w.findAll('.item')[0]
    await image.trigger('dblclick')
    await flushPromises()
    expect(w.find('.viewer').exists()).toBe(true)
  })

  it('closes the viewer on Escape', async () => {
    const { w } = await mountApp()
    await w.find('.item').trigger('dblclick')
    await flushPromises()
    await w.findAll('.item')[0].trigger('dblclick')
    await flushPromises()
    expect(w.find('.viewer').exists()).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(w.find('.viewer').exists()).toBe(false)
  })

  it('opens the settings panel', async () => {
    const { w } = await mountApp()
    const settingsButton = w.findAll('.cmd').find((b) => b.text() === '設定')
    await settingsButton?.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('キャッシュ')
  })

  it('clears the cache from the settings panel', async () => {
    const { w, fake } = await mountApp()
    const settingsButton = w.findAll('.cmd').find((b) => b.text() === '設定')
    await settingsButton?.trigger('click')
    await flushPromises()

    const clear = w.findAll('.btn').find((b) => b.text() === 'すべて消去')
    await clear?.trigger('click')
    await flushPromises()
    expect(fake.calls).toContain('ClearCache')
  })

  it('opens the archive passed on the command line', async () => {
    const fake = installFake()
    const rt = installRuntime()
    const target = 'C:\\pics\\book.zip'
    ;(fake.api as { StartupTarget: unknown }).StartupTarget = vi.fn(async () => target)

    mount(App)
    await flushPromises()

    rt.emit('scan:file', { archive: makeArchive({ id: 1, path: target }) })
    await flushPromises()
    await new Promise((r) => setTimeout(r, 250))
    await flushPromises()

    expect(fake.calls).toContain('StartScan:C:\\pics')
    expect(fake.calls).toContain('GetArchiveEntries:1')
  })

  it('removes the row after a delete', async () => {
    const { w, fake } = await mountApp()
    await w.find('.item').trigger('click')
    await flushPromises()

    ;(fake.api as { DeleteArchiveToRecycleBin: unknown }).DeleteArchiveToRecycleBin = vi.fn(
      async () => ({ source: 'C:\\pics\\book.zip', recycleBin: true, archiveId: 1 }),
    )

    await w.find('.item').trigger('contextmenu')
    await flushPromises()
    const del = w.findAll('.item').find((b) => b.text() === 'ごみ箱へ移動')
    await del?.trigger('click')
    await flushPromises()

    const confirm = w.findAll('.actions .btn').find((b) => b.text() === 'ごみ箱へ移動')
    await confirm?.trigger('click')
    await flushPromises()

    expect(w.findAll('.item')).toHaveLength(0)
  })
})
