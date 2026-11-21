// Types mirroring the Go structs bound to the frontend. Keeping them in one
// place makes a backend field rename a compile error rather than a silent
// undefined.

export interface Settings {
  root: string
  selectedArchive: string
  selectedEntry: number
  scrollTop: number
  thumbSize: number
  sortMode: 'name' | 'mtime' | 'size'
  sortDesc: boolean
  search: string
  cacheLimitBytes: number
  memoryLimitBytes: number
  workers: number
  prefetchAhead: number
  maxEntryBytes: number
  maxPixels: number
  maxEntries: number
  windowWidth: number
  windowHeight: number
}

export interface SettingsPatch {
  root?: string
  selectedArchive?: string
  selectedEntry?: number
  scrollTop?: number
  thumbSize?: number
  sortMode?: 'name' | 'mtime' | 'size'
  sortDesc?: boolean
  search?: string
  cacheLimitBytes?: number
  memoryLimitBytes?: number
  workers?: number
  prefetchAhead?: number
  windowWidth?: number
  windowHeight?: number
}

export type ArchiveStatus = 'ok' | 'empty' | 'corrupt' | 'encrypted' | 'missing' | 'error'

export interface ArchiveSummary {
  id: number
  path: string
  name: string
  size: number
  modTime: number
  count: number
  status: ArchiveStatus
  error?: string
  coverOrdinal: number
  coverWidth: number
}

export interface EntryInfo {
  ordinal: number
  name: string
  crc32: number
  size: number
  encrypted: boolean
}

export interface ArchiveDetail {
  id: number
  path: string
  name: string
  status: ArchiveStatus
  error?: string
  entries: EntryInfo[]
}

export interface ScanResult {
  root: string
  files: number
  dirs: number
  skipped: number
  cancelled: boolean
  durationMs: number
  gen: number
}

export interface CacheStats {
  thumbCount: number
  thumbBytes: number
  thumbLimit: number
  archiveCount: number
  memoryUsed: number
  memoryLimit: number
  memoryItems: number
  root: string
}

export interface EntryStatus {
  ordinal: number
  ok: boolean
  kind?: string
  message?: string
}

export interface PrefetchStats {
  scheduler: { completed: number; discarded: number; deduped: number; pending: number; workers: number }
  memory: { limit: number; used: number; items: number; hits: number; misses: number; evicted: number }
  decodeBytes: number
}

export interface OrganizeResult {
  source: string
  destination?: string
  recycleBin: boolean
  archiveId: number
}

export interface OrganizeCapabilities {
  move: boolean
  rename: boolean
  recycleBin: boolean
  permanentDelete: boolean
}

// The Go binding surface. Wails generates this at build time, but declaring it
// here keeps the frontend type-checkable without a generated file.
export interface ZipScopeAPI {
  GetSettings(): Promise<Settings>
  UpdateSettings(patch: SettingsPatch): Promise<Settings>
  GetCacheStats(): Promise<CacheStats>
  ClearCache(): Promise<void>
  ClearThumbnails(): Promise<void>
  EvictCache(): Promise<number>
  PickFolder(): Promise<string>
  PickArchive(): Promise<string>
  RevealInExplorer(path: string): Promise<void>
  ThumbSizeOptions(): Promise<number[]>
  StartScan(root: string): Promise<number>
  CancelScan(): Promise<void>
  IndexArchive(path: string): Promise<ArchiveSummary>
  GetArchiveEntries(id: number): Promise<ArchiveDetail>
  Thumbnail(id: number, ordinal: number, width: number): Promise<unknown>
  FullImage(id: number, ordinal: number): Promise<unknown>
  PrefetchImage(id: number, ordinal: number, width: number, priority: number): Promise<void>
  PrefetchFull(id: number, ordinal: number, priority: number): Promise<void>
  BumpSelection(): Promise<number>
  GetPrefetchStats(): Promise<PrefetchStats>
  CheckEntry(id: number, ordinal: number): Promise<EntryStatus>
  WarmArchive(id: number, n: number, width: number): Promise<void>
  ArchiveCover(id: number, width: number): Promise<unknown>
  MoveArchive(id: number, destDir: string): Promise<OrganizeResult>
  RenameArchive(id: number, newName: string): Promise<OrganizeResult>
  DeleteArchiveToRecycleBin(id: number): Promise<OrganizeResult>
  GetOrganizeCapabilities(): Promise<OrganizeCapabilities>
  RefreshArchive(path: string): Promise<ArchiveSummary>
  PruneMissingArchives(): Promise<number>
  StartupTarget(): Promise<string>
}

declare global {
  interface Window {
    // Wails binds the Go App struct under window.go.app.App.
    go?: {
      app?: {
        App?: ZipScopeAPI
      }
    }
    runtime?: {
      EventsOn(name: string, cb: (payload: unknown) => void): void
      EventsOff(name: string): void
      WindowSetTitle(title: string): void
    }
  }
}
