export namespace app {
	
	export class EntryInfo {
	    ordinal: number;
	    name: string;
	    crc32: number;
	    size: number;
	    encrypted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EntryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ordinal = source["ordinal"];
	        this.name = source["name"];
	        this.crc32 = source["crc32"];
	        this.size = source["size"];
	        this.encrypted = source["encrypted"];
	    }
	}
	export class ArchiveDetail {
	    id: number;
	    path: string;
	    name: string;
	    status: string;
	    error?: string;
	    entries: EntryInfo[];
	
	    static createFrom(source: any = {}) {
	        return new ArchiveDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.entries = this.convertValues(source["entries"], EntryInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ArchiveSummary {
	    id: number;
	    path: string;
	    name: string;
	    size: number;
	    modTime: number;
	    count: number;
	    status: string;
	    error?: string;
	    coverOrdinal: number;
	    coverWidth: number;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.count = source["count"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.coverOrdinal = source["coverOrdinal"];
	        this.coverWidth = source["coverWidth"];
	    }
	}
	export class CacheStats {
	    thumbCount: number;
	    thumbBytes: number;
	    thumbLimit: number;
	    archiveCount: number;
	    memoryUsed: number;
	    memoryLimit: number;
	    memoryItems: number;
	    root: string;
	
	    static createFrom(source: any = {}) {
	        return new CacheStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.thumbCount = source["thumbCount"];
	        this.thumbBytes = source["thumbBytes"];
	        this.thumbLimit = source["thumbLimit"];
	        this.archiveCount = source["archiveCount"];
	        this.memoryUsed = source["memoryUsed"];
	        this.memoryLimit = source["memoryLimit"];
	        this.memoryItems = source["memoryItems"];
	        this.root = source["root"];
	    }
	}
	
	export class EntryStatus {
	    ordinal: number;
	    ok: boolean;
	    kind?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new EntryStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ordinal = source["ordinal"];
	        this.ok = source["ok"];
	        this.kind = source["kind"];
	        this.message = source["message"];
	    }
	}
	export class ImageResponse {
	    Data: number[];
	    ContentType: string;
	    Cached: boolean;
	    Width: number;
	    Height: number;
	
	    static createFrom(source: any = {}) {
	        return new ImageResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Data = source["Data"];
	        this.ContentType = source["ContentType"];
	        this.Cached = source["Cached"];
	        this.Width = source["Width"];
	        this.Height = source["Height"];
	    }
	}
	export class OrganizeCapabilities {
	    move: boolean;
	    rename: boolean;
	    recycleBin: boolean;
	    permanentDelete: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OrganizeCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.move = source["move"];
	        this.rename = source["rename"];
	        this.recycleBin = source["recycleBin"];
	        this.permanentDelete = source["permanentDelete"];
	    }
	}
	export class OrganizeResult {
	    source: string;
	    destination?: string;
	    recycleBin: boolean;
	    archiveId: number;
	
	    static createFrom(source: any = {}) {
	        return new OrganizeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.destination = source["destination"];
	        this.recycleBin = source["recycleBin"];
	        this.archiveId = source["archiveId"];
	    }
	}
	export class PrefetchStats {
	    scheduler: prefetch.Stats;
	    memory: prefetch.CacheStats;
	    decodeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new PrefetchStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scheduler = this.convertValues(source["scheduler"], prefetch.Stats);
	        this.memory = this.convertValues(source["memory"], prefetch.CacheStats);
	        this.decodeBytes = source["decodeBytes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SettingsPatch {
	    root?: string;
	    selectedArchive?: string;
	    selectedEntry?: number;
	    scrollTop?: number;
	    thumbSize?: number;
	    viewMode?: string;
	    iconSize?: number;
	    sortMode?: string;
	    sortDesc?: boolean;
	    search?: string;
	    cacheLimitBytes?: number;
	    memoryLimitBytes?: number;
	    workers?: number;
	    prefetchAhead?: number;
	    windowWidth?: number;
	    windowHeight?: number;
	
	    static createFrom(source: any = {}) {
	        return new SettingsPatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.selectedArchive = source["selectedArchive"];
	        this.selectedEntry = source["selectedEntry"];
	        this.scrollTop = source["scrollTop"];
	        this.thumbSize = source["thumbSize"];
	        this.viewMode = source["viewMode"];
	        this.iconSize = source["iconSize"];
	        this.sortMode = source["sortMode"];
	        this.sortDesc = source["sortDesc"];
	        this.search = source["search"];
	        this.cacheLimitBytes = source["cacheLimitBytes"];
	        this.memoryLimitBytes = source["memoryLimitBytes"];
	        this.workers = source["workers"];
	        this.prefetchAhead = source["prefetchAhead"];
	        this.windowWidth = source["windowWidth"];
	        this.windowHeight = source["windowHeight"];
	    }
	}

}

export namespace prefetch {
	
	export class CacheStats {
	    limit: number;
	    used: number;
	    items: number;
	    hits: number;
	    misses: number;
	    evicted: number;
	
	    static createFrom(source: any = {}) {
	        return new CacheStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.limit = source["limit"];
	        this.used = source["used"];
	        this.items = source["items"];
	        this.hits = source["hits"];
	        this.misses = source["misses"];
	        this.evicted = source["evicted"];
	    }
	}
	export class Stats {
	    completed: number;
	    discarded: number;
	    deduped: number;
	    pending: number;
	    workers: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.completed = source["completed"];
	        this.discarded = source["discarded"];
	        this.deduped = source["deduped"];
	        this.pending = source["pending"];
	        this.workers = source["workers"];
	    }
	}

}

export namespace settings {
	
	export class Settings {
	    root: string;
	    selectedArchive: string;
	    selectedEntry: number;
	    scrollTop: number;
	    thumbSize: number;
	    viewMode: string;
	    iconSize: number;
	    sortMode: string;
	    sortDesc: boolean;
	    search: string;
	    cacheLimitBytes: number;
	    memoryLimitBytes: number;
	    workers: number;
	    prefetchAhead: number;
	    maxEntryBytes: number;
	    maxPixels: number;
	    maxEntries: number;
	    windowWidth: number;
	    windowHeight: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.selectedArchive = source["selectedArchive"];
	        this.selectedEntry = source["selectedEntry"];
	        this.scrollTop = source["scrollTop"];
	        this.thumbSize = source["thumbSize"];
	        this.viewMode = source["viewMode"];
	        this.iconSize = source["iconSize"];
	        this.sortMode = source["sortMode"];
	        this.sortDesc = source["sortDesc"];
	        this.search = source["search"];
	        this.cacheLimitBytes = source["cacheLimitBytes"];
	        this.memoryLimitBytes = source["memoryLimitBytes"];
	        this.workers = source["workers"];
	        this.prefetchAhead = source["prefetchAhead"];
	        this.maxEntryBytes = source["maxEntryBytes"];
	        this.maxPixels = source["maxPixels"];
	        this.maxEntries = source["maxEntries"];
	        this.windowWidth = source["windowWidth"];
	        this.windowHeight = source["windowHeight"];
	    }
	}

}

