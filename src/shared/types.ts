export type AccountState = 'disconnected' | 'connected' | 'needs_auth'
export type ScanState = 'idle' | 'scanning' | 'paused' | 'rate_limited' | 'needs_auth' | 'offline' | 'failed'
export type JobState = 'confirmed' | 'running' | 'paused' | 'waiting_rate_limit' | 'needs_auth' | 'offline' | 'needs_reconciliation' | 'client_update_required' | 'completed'
export type ItemState = 'pending' | 'in_flight' | 'succeeded' | 'already_unliked' | 'failed' | 'skipped' | 'ambiguous'

export interface Account {
  id: string
  username: string | null
  partition: string
  state: AccountState
  scanState: ScanState
  scanCursor: string | null
  scanCount: number
  lastScanAt: string | null
  message: string | null
}

export interface ActivityFilter {
  search: string
  mediaType: '' | 'post' | 'reel' | 'carousel'
  from: string
  to: string
}

export interface ActivityItem {
  id: number
  mediaId: string
  shortcode: string
  ownerUsername: string
  caption: string
  mediaType: 'post' | 'reel' | 'carousel'
  thumbnailUrl: string | null
  permalink: string
  likedAt: string | null
  discoveredAt: string
}

export interface ActivityPage {
  items: ActivityItem[]
  total: number
  offset: number
}

export interface Selection {
  filter: ActivityFilter
  allMatching: boolean
  ids: number[]
  excludedIds: number[]
}

export interface Job {
  id: string
  accountId: string
  accountUsername: string | null
  state: JobState
  createdAt: string
  confirmedAt: string
  startedAt: string | null
  finishedAt: string | null
  waitUntil: string | null
  total: number
  pending: number
  succeeded: number
  alreadyUnliked: number
  failed: number
  skipped: number
  ambiguous: number
  message: string | null
}

export interface AmbiguousItem {
  id: number
  mediaId: string
  shortcode: string
  permalink: string
  attempts: number
  message: string | null
}

export interface InstagramPage {
  items: Omit<ActivityItem, 'id'>[]
  cursor: string | null
  hasMore: boolean
}

export interface InstagramResult {
  kind: 'succeeded' | 'already_unliked' | 'rate_limited' | 'needs_auth' | 'offline' | 'ambiguous' | 'client_update_required'
  retryAfterMs?: number
  message?: string
}

export interface ReconcileResult {
  kind: 'unliked' | 'liked' | 'unavailable' | 'rate_limited' | 'needs_auth' | 'offline' | 'ambiguous'
  retryAfterMs?: number
  message?: string
}

export type AppEvent =
  | { type: 'accounts-changed' }
  | { type: 'scan-changed'; accountId: string }
  | { type: 'job-changed'; jobId: string }

export interface VanishAPI {
  accounts: {
    list(): Promise<Account[]>
    add(): Promise<Account>
    showLogin(accountId: string): Promise<void>
    identify(accountId: string): Promise<string>
    bind(accountId: string, username: string): Promise<Account>
    signOut(accountId: string): Promise<Account>
    remove(accountId: string): Promise<void>
    scan(accountId: string): Promise<void>
    pauseScan(accountId: string): Promise<void>
  }
  activity: {
    page(accountId: string, filter: ActivityFilter, offset: number, limit: number): Promise<ActivityPage>
  }
  jobs: {
    confirm(accountId: string, selection: Selection): Promise<Job>
    list(accountId?: string): Promise<Job[]>
    get(jobId: string): Promise<Job>
    ambiguous(jobId: string): Promise<AmbiguousItem | null>
    start(jobId: string): Promise<void>
    pause(jobId: string): Promise<void>
    resume(jobId: string): Promise<void>
    resolve(jobId: string, itemId: number, resolution: 'done' | 'retry' | 'skip'): Promise<void>
    showItem(jobId: string, itemId: number): Promise<void>
  }
  onEvent(listener: (event: AppEvent) => void): () => void
}
