// Thin wrapper over the Wails bindings. Every call goes through here so the
// rest of the UI never touches window.go directly, and so tests can substitute
// a fake API.

import type {
  ArchiveDetail,
  ArchiveSummary,
  CacheStats,
  EntryStatus,
  OrganizeCapabilities,
  OrganizeResult,
  PrefetchStats,
  Settings,
  SettingsPatch,
  ZipScopeAPI,
} from './types'

function api(): ZipScopeAPI {
  const a = window.go?.app?.App
  if (!a) {
    throw new Error('ZipScope backend is not available')
  }
  return a
}

export const backend = {
  available(): boolean {
    return Boolean(window.go?.app?.App)
  },

  getSettings(): Promise<Settings> {
    return api().GetSettings()
  },

  updateSettings(patch: SettingsPatch): Promise<Settings> {
    return api().UpdateSettings(patch)
  },

  getCacheStats(): Promise<CacheStats> {
    return api().GetCacheStats()
  },

  clearCache(): Promise<void> {
    return api().ClearCache()
  },

  clearThumbnails(): Promise<void> {
    return api().ClearThumbnails()
  },

  evictCache(): Promise<number> {
    return api().EvictCache()
  },

  pickFolder(): Promise<string> {
    return api().PickFolder()
  },

  pickArchive(): Promise<string> {
    return api().PickArchive()
  },

  reveal(path: string): Promise<void> {
    return api().RevealInExplorer(path)
  },

  startScan(root: string): Promise<number> {
    return api().StartScan(root)
  },

  cancelScan(): Promise<void> {
    return api().CancelScan()
  },

  indexArchive(path: string): Promise<ArchiveSummary> {
    return api().IndexArchive(path)
  },

  getArchiveEntries(id: number): Promise<ArchiveDetail> {
    return api().GetArchiveEntries(id)
  },

  prefetchImage(id: number, ordinal: number, width: number, priority: number): Promise<void> {
    return api().PrefetchImage(id, ordinal, width, priority)
  },

  prefetchFull(id: number, ordinal: number, priority: number): Promise<void> {
    return api().PrefetchFull(id, ordinal, priority)
  },

  bumpSelection(): Promise<number> {
    return api().BumpSelection()
  },

  getPrefetchStats(): Promise<PrefetchStats> {
    return api().GetPrefetchStats()
  },

  checkEntry(id: number, ordinal: number): Promise<EntryStatus> {
    return api().CheckEntry(id, ordinal)
  },

  warmArchive(id: number, n: number, width: number): Promise<void> {
    return api().WarmArchive(id, n, width)
  },

  moveArchive(id: number, destDir: string): Promise<OrganizeResult> {
    return api().MoveArchive(id, destDir)
  },

  renameArchive(id: number, newName: string): Promise<OrganizeResult> {
    return api().RenameArchive(id, newName)
  },

  deleteToRecycleBin(id: number): Promise<OrganizeResult> {
    return api().DeleteArchiveToRecycleBin(id)
  },

  organizeCapabilities(): Promise<OrganizeCapabilities> {
    return api().GetOrganizeCapabilities()
  },

  refreshArchive(path: string): Promise<ArchiveSummary> {
    return api().RefreshArchive(path)
  },

  pruneMissing(): Promise<number> {
    return api().PruneMissingArchives()
  },

  startupTarget(): Promise<string> {
    return api().StartupTarget()
  },
}

// Image URLs are opaque identifiers, never paths. The Go handler validates the
// shape before resolving them.
export function thumbURL(archiveId: number, ordinal: number, width: number): string {
  return `/img/t${archiveId}/${ordinal}/${width}`
}

export function fullURL(archiveId: number, ordinal: number): string {
  return `/img/f${archiveId}/${ordinal}`
}

export function coverURL(archiveId: number, width: number): string {
  return `/img/c${archiveId}/${width}`
}

// Priority constants mirroring internal/prefetch.
export const Priority = {
  Selected: 0,
  Visible: 1,
  Neighbour: 2,
  Cover: 3,
} as const
