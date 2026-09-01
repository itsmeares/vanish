import { randomUUID } from 'node:crypto'
import { mkdirSync } from 'node:fs'
import { dirname } from 'node:path'
import Database from 'better-sqlite3'
import type {
  Account,
  ActivityFilter,
  ActivityItem,
  ActivityPage,
  AmbiguousItem,
  InstagramPage,
  ItemState,
  Job,
  JobState,
  Selection,
} from '../shared/types'

type Row = Record<string, unknown>

const now = (): string => new Date().toISOString()

function accountFromRow(row: Row): Account {
  return {
    id: String(row.id),
    username: row.username === null ? null : String(row.username),
    partition: String(row.partition),
    state: row.state as Account['state'],
    scanState: row.scan_state as Account['scanState'],
    scanCursor: row.scan_cursor === null ? null : String(row.scan_cursor),
    scanCount: Number(row.scan_count),
    lastScanAt: row.last_scan_at === null ? null : String(row.last_scan_at),
    message: row.message === null ? null : String(row.message),
  }
}

function activityFromRow(row: Row): ActivityItem {
  return {
    id: Number(row.id),
    mediaId: String(row.media_id),
    shortcode: String(row.shortcode),
    ownerUsername: String(row.owner_username),
    caption: String(row.caption),
    mediaType: row.media_type as ActivityItem['mediaType'],
    thumbnailUrl: row.thumbnail_url === null ? null : String(row.thumbnail_url),
    permalink: String(row.permalink),
    likedAt: row.liked_at === null ? null : String(row.liked_at),
    discoveredAt: String(row.discovered_at),
  }
}

function jobFromRow(row: Row): Job {
  return {
    id: String(row.id),
    accountId: String(row.account_id),
    accountUsername: row.account_username === null ? null : String(row.account_username),
    state: row.state as JobState,
    createdAt: String(row.created_at),
    confirmedAt: String(row.confirmed_at),
    startedAt: row.started_at === null ? null : String(row.started_at),
    finishedAt: row.finished_at === null ? null : String(row.finished_at),
    waitUntil: row.wait_until === null ? null : String(row.wait_until),
    total: Number(row.total),
    pending: Number(row.pending),
    succeeded: Number(row.succeeded),
    alreadyUnliked: Number(row.already_unliked),
    failed: Number(row.failed),
    skipped: Number(row.skipped),
    ambiguous: Number(row.ambiguous),
    message: row.message === null ? null : String(row.message),
  }
}

function ftsQuery(value: string): string {
  return value.trim().split(/\s+/).filter(Boolean).map((term) => `"${term.replaceAll('"', '""')}"*`).join(' ')
}

function filterSql(filter: ActivityFilter, alias = 'a'): { joins: string; where: string; params: unknown[] } {
  const clauses = [`${alias}.is_liked = 1`]
  const params: unknown[] = []
  const query = ftsQuery(filter.search)
  const joins = query ? `JOIN activity_fts ON activity_fts.rowid = ${alias}.id` : ''
  if (query) {
    clauses.push('activity_fts MATCH ?')
    params.push(query)
  }
  if (filter.mediaType) {
    clauses.push(`${alias}.media_type = ?`)
    params.push(filter.mediaType)
  }
  if (filter.from) {
    clauses.push(`COALESCE(${alias}.liked_at, ${alias}.discovered_at) >= ?`)
    params.push(`${filter.from}T00:00:00.000Z`)
  }
  if (filter.to) {
    clauses.push(`COALESCE(${alias}.liked_at, ${alias}.discovered_at) < ?`)
    const end = new Date(`${filter.to}T00:00:00.000Z`)
    end.setUTCDate(end.getUTCDate() + 1)
    params.push(end.toISOString())
  }
  return { joins, where: clauses.join(' AND '), params }
}

export class VanishDatabase {
  readonly raw: Database.Database

  constructor(path: string) {
    if (path !== ':memory:') mkdirSync(dirname(path), { recursive: true })
    this.raw = new Database(path)
    this.raw.pragma('foreign_keys = ON')
    this.raw.pragma('journal_mode = WAL')
    this.raw.pragma('synchronous = FULL')
    this.raw.pragma('busy_timeout = 5000')
    this.migrate()
    this.recover()
  }

  close(): void { this.raw.close() }

  private migrate(): void {
    this.raw.exec(`
      CREATE TABLE IF NOT EXISTS accounts (
        id TEXT PRIMARY KEY,
        username TEXT,
        instagram_id TEXT,
        partition TEXT NOT NULL UNIQUE,
        state TEXT NOT NULL DEFAULT 'disconnected',
        scan_state TEXT NOT NULL DEFAULT 'idle',
        scan_cursor TEXT,
        scan_is_full INTEGER NOT NULL DEFAULT 0 CHECK (scan_is_full IN (0, 1)),
        scan_count INTEGER NOT NULL DEFAULT 0,
        last_scan_at TEXT,
        message TEXT,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
      );

      CREATE TABLE IF NOT EXISTS activity (
        id INTEGER PRIMARY KEY,
        account_id TEXT NOT NULL REFERENCES accounts(id),
        media_id TEXT NOT NULL,
        shortcode TEXT NOT NULL,
        owner_username TEXT NOT NULL DEFAULT '',
        caption TEXT NOT NULL DEFAULT '',
        media_type TEXT NOT NULL CHECK (media_type IN ('post', 'reel', 'carousel')),
        thumbnail_url TEXT,
        permalink TEXT NOT NULL,
        liked_at TEXT,
        discovered_at TEXT NOT NULL,
        last_seen_at TEXT NOT NULL,
        seen_in_scan INTEGER NOT NULL DEFAULT 0 CHECK (seen_in_scan IN (0, 1)),
        is_liked INTEGER NOT NULL DEFAULT 1 CHECK (is_liked IN (0, 1)),
        UNIQUE(account_id, media_id)
      );
      CREATE INDEX IF NOT EXISTS activity_account_liked ON activity(account_id, is_liked, liked_at DESC, id DESC);
      CREATE INDEX IF NOT EXISTS activity_account_type ON activity(account_id, is_liked, media_type);
      CREATE VIRTUAL TABLE IF NOT EXISTS activity_fts USING fts5(owner_username, caption, content='activity', content_rowid='id');
      CREATE TRIGGER IF NOT EXISTS activity_fts_insert AFTER INSERT ON activity BEGIN
        INSERT INTO activity_fts(rowid, owner_username, caption) VALUES (new.id, new.owner_username, new.caption);
      END;
      CREATE TRIGGER IF NOT EXISTS activity_fts_delete AFTER DELETE ON activity BEGIN
        INSERT INTO activity_fts(activity_fts, rowid, owner_username, caption) VALUES ('delete', old.id, old.owner_username, old.caption);
      END;
      CREATE TRIGGER IF NOT EXISTS activity_fts_update AFTER UPDATE OF owner_username, caption ON activity BEGIN
        INSERT INTO activity_fts(activity_fts, rowid, owner_username, caption) VALUES ('delete', old.id, old.owner_username, old.caption);
        INSERT INTO activity_fts(rowid, owner_username, caption) VALUES (new.id, new.owner_username, new.caption);
      END;

      CREATE TABLE IF NOT EXISTS jobs (
        id TEXT PRIMARY KEY,
        account_id TEXT NOT NULL REFERENCES accounts(id),
        state TEXT NOT NULL,
        created_at TEXT NOT NULL,
        confirmed_at TEXT,
        started_at TEXT,
        finished_at TEXT,
        wait_until TEXT,
        message TEXT
      );
      CREATE INDEX IF NOT EXISTS jobs_account_created ON jobs(account_id, created_at DESC);

      CREATE TABLE IF NOT EXISTS job_items (
        id INTEGER PRIMARY KEY,
        job_id TEXT NOT NULL REFERENCES jobs(id),
        ordinal INTEGER NOT NULL,
        activity_id INTEGER NOT NULL REFERENCES activity(id),
        media_id TEXT NOT NULL,
        shortcode TEXT NOT NULL,
        permalink TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'pending',
        attempts INTEGER NOT NULL DEFAULT 0,
        attempt_started_at TEXT,
        finished_at TEXT,
        message TEXT,
        UNIQUE(job_id, ordinal),
        UNIQUE(job_id, activity_id)
      );
      CREATE INDEX IF NOT EXISTS job_items_work ON job_items(job_id, status, ordinal);
      CREATE TRIGGER IF NOT EXISTS job_items_frozen_insert BEFORE INSERT ON job_items
        WHEN EXISTS (SELECT 1 FROM jobs WHERE id = new.job_id AND confirmed_at IS NOT NULL)
        BEGIN SELECT RAISE(ABORT, 'confirmed cleanup is immutable'); END;
      CREATE TRIGGER IF NOT EXISTS job_items_frozen_delete BEFORE DELETE ON job_items
        WHEN EXISTS (SELECT 1 FROM jobs WHERE id = old.job_id AND confirmed_at IS NOT NULL)
        BEGIN SELECT RAISE(ABORT, 'confirmed cleanup is immutable'); END;
      CREATE TRIGGER IF NOT EXISTS job_items_frozen_targets BEFORE UPDATE OF ordinal, activity_id, media_id, shortcode, permalink ON job_items
        WHEN EXISTS (SELECT 1 FROM jobs WHERE id = old.job_id AND confirmed_at IS NOT NULL)
        BEGIN SELECT RAISE(ABORT, 'confirmed cleanup targets are immutable'); END;
    `)
    const accountColumns = this.raw.pragma('table_info(accounts)') as Row[]
    if (!accountColumns.some((column) => column.name === 'instagram_id')) this.raw.exec(`ALTER TABLE accounts ADD COLUMN instagram_id TEXT`)
    if (!accountColumns.some((column) => column.name === 'scan_is_full')) this.raw.exec(`ALTER TABLE accounts ADD COLUMN scan_is_full INTEGER NOT NULL DEFAULT 0 CHECK (scan_is_full IN (0, 1))`)
    const activityColumns = this.raw.pragma('table_info(activity)') as Row[]
    if (!activityColumns.some((column) => column.name === 'seen_in_scan')) this.raw.exec(`ALTER TABLE activity ADD COLUMN seen_in_scan INTEGER NOT NULL DEFAULT 0 CHECK (seen_in_scan IN (0, 1))`)
    this.raw.prepare(`UPDATE accounts SET username = NULL, state = 'disconnected', message = 'Confirm your Instagram account again.' WHERE instagram_id IS NULL AND username GLOB 'account-[0-9]*' AND substr(username, 9) NOT GLOB '*[^0-9]*'`).run()
    this.raw.prepare(`UPDATE accounts SET state = 'disconnected', message = COALESCE(message, 'Confirm your Instagram account again.') WHERE instagram_id IS NULL AND state = 'connected'`).run()
  }

  private recover(): void {
    const recoveredAt = now()
    this.raw.transaction(() => {
      this.raw.prepare(`UPDATE job_items SET status = 'ambiguous', message = COALESCE(message, 'Vanish closed before the result was saved.') WHERE status = 'in_flight'`).run()
      this.raw.prepare(`UPDATE jobs SET state = 'needs_reconciliation', message = 'Check the last item before continuing.' WHERE state = 'running'`).run()
      this.raw.prepare(`UPDATE accounts SET scan_state = 'paused', message = 'Scan paused when Vanish closed.', updated_at = ? WHERE scan_state = 'scanning'`).run(recoveredAt)
    })()
  }

  createAccount(): Account {
    const id = randomUUID()
    const timestamp = now()
    this.raw.prepare(`INSERT INTO accounts (id, partition, created_at, updated_at) VALUES (?, ?, ?, ?)`).run(id, `persist:vanish-instagram-${id}`, timestamp, timestamp)
    return this.getAccount(id)
  }

  listAccounts(): Account[] {
    return (this.raw.prepare(`SELECT * FROM accounts ORDER BY created_at`).all() as Row[]).map(accountFromRow)
  }

  getAccount(id: string): Account {
    const row = this.raw.prepare(`SELECT * FROM accounts WHERE id = ?`).get(id) as Row | undefined
    if (!row) throw new Error('Account not found.')
    return accountFromRow(row)
  }

  connectAccount(id: string, instagramId: string, username: string): Account {
    const row = this.raw.prepare(`SELECT instagram_id, username FROM accounts WHERE id = ?`).get(id) as Row | undefined
    if (!row) throw new Error('Account not found.')
    if (row.instagram_id !== null && String(row.instagram_id) !== instagramId) throw new Error('This Vanish account is bound to a different Instagram account.')
    if (row.instagram_id === null && row.username !== null && String(row.username).toLowerCase() !== username.toLowerCase()) throw new Error(`This Vanish account belongs to @${String(row.username)}.`)
    this.raw.prepare(`UPDATE accounts SET instagram_id = ?, username = ?, state = 'connected', message = NULL, updated_at = ? WHERE id = ?`).run(instagramId, username, now(), id)
    return this.getAccount(id)
  }

  assertAccountInactive(id: string): void {
    const account = this.raw.prepare(`SELECT scan_state FROM accounts WHERE id = ?`).get(id) as Row | undefined
    if (!account) throw new Error('Account not found.')
    if (account.scan_state === 'scanning') throw new Error('Pause the scan first.')
    if (this.raw.prepare(`SELECT 1 FROM jobs WHERE account_id = ? AND state = 'running' LIMIT 1`).get(id)) throw new Error('Pause cleanup first.')
  }

  signOutAccount(id: string): Account {
    this.assertAccountInactive(id)
    this.raw.prepare(`UPDATE accounts SET state = 'disconnected', scan_state = CASE WHEN scan_state = 'idle' THEN 'idle' ELSE 'paused' END, message = 'Signed out. Sign in to resume.', updated_at = ? WHERE id = ?`).run(now(), id)
    return this.getAccount(id)
  }

  removeAccount(id: string): void {
    this.assertAccountInactive(id)
    this.raw.transaction(() => {
      this.raw.prepare(`UPDATE jobs SET confirmed_at = NULL WHERE account_id = ?`).run(id)
      this.raw.prepare(`DELETE FROM job_items WHERE job_id IN (SELECT id FROM jobs WHERE account_id = ?)`).run(id)
      this.raw.prepare(`DELETE FROM jobs WHERE account_id = ?`).run(id)
      this.raw.prepare(`DELETE FROM activity WHERE account_id = ?`).run(id)
      this.raw.prepare(`DELETE FROM accounts WHERE id = ?`).run(id)
    })()
  }

  updateAccountState(id: string, state: Account['state'], message: string | null): void {
    this.raw.prepare(`UPDATE accounts SET state = ?, message = ?, updated_at = ? WHERE id = ?`).run(state, message, now(), id)
  }

  startScan(id: string): string | null {
    const account = this.getAccount(id)
    const full = account.scanState === 'idle'
    this.raw.transaction(() => {
      if (full) this.raw.prepare(`UPDATE activity SET seen_in_scan = 0 WHERE account_id = ? AND is_liked = 1`).run(id)
      this.raw.prepare(`UPDATE accounts SET scan_state = 'scanning', scan_cursor = CASE WHEN ? THEN NULL ELSE scan_cursor END, scan_is_full = CASE WHEN ? THEN 1 ELSE scan_is_full END, message = NULL, updated_at = ? WHERE id = ?`)
        .run(full ? 1 : 0, full ? 1 : 0, now(), id)
    })()
    return full ? null : account.scanCursor
  }

  updateScan(id: string, state: Account['scanState'], cursor: string | null, message: string | null): void {
    this.raw.prepare(`UPDATE accounts SET scan_state = ?, scan_cursor = ?, message = ?, updated_at = ? WHERE id = ?`)
      .run(state, cursor, message, now(), id)
  }

  finishScan(id: string): void {
    const timestamp = now()
    this.raw.transaction(() => {
      const account = this.raw.prepare(`SELECT scan_state, scan_is_full FROM accounts WHERE id = ?`).get(id) as Row | undefined
      if (!account || account.scan_state !== 'scanning') throw new Error('Scan is not running.')
      if (account.scan_is_full) this.raw.prepare(`UPDATE activity SET is_liked = 0 WHERE account_id = ? AND is_liked = 1 AND seen_in_scan = 0`).run(id)
      this.raw.prepare(`UPDATE accounts SET scan_state = 'idle', scan_cursor = NULL, scan_is_full = 0, scan_count = (SELECT count(*) FROM activity WHERE account_id = ? AND is_liked = 1), last_scan_at = ?, message = NULL, updated_at = ? WHERE id = ?`)
        .run(id, timestamp, timestamp, id)
    })()
  }

  saveScanPage(accountId: string, page: InstagramPage): number {
    const timestamp = now()
    const upsert = this.raw.prepare(`
      INSERT INTO activity (account_id, media_id, shortcode, owner_username, caption, media_type, thumbnail_url, permalink, liked_at, discovered_at, last_seen_at, seen_in_scan)
      VALUES (@accountId, @mediaId, @shortcode, @ownerUsername, @caption, @mediaType, @thumbnailUrl, @permalink, @likedAt, @discoveredAt, @lastSeenAt, 1)
      ON CONFLICT(account_id, media_id) DO UPDATE SET
        shortcode = excluded.shortcode,
        owner_username = excluded.owner_username,
        caption = excluded.caption,
        media_type = excluded.media_type,
        thumbnail_url = excluded.thumbnail_url,
        permalink = excluded.permalink,
        liked_at = COALESCE(excluded.liked_at, activity.liked_at),
        last_seen_at = excluded.last_seen_at,
        seen_in_scan = 1,
        is_liked = 1
    `)
    this.raw.transaction(() => {
      for (const item of page.items) upsert.run({ ...item, accountId, lastSeenAt: timestamp })
      this.raw.prepare(`UPDATE accounts SET scan_cursor = ?, scan_count = (SELECT count(*) FROM activity WHERE account_id = ? AND is_liked = 1), updated_at = ? WHERE id = ?`)
        .run(page.cursor, accountId, timestamp, accountId)
    })()
    return this.getAccount(accountId).scanCount
  }

  activityPage(accountId: string, filter: ActivityFilter, offset: number, limit: number): ActivityPage {
    const safeOffset = Math.max(0, Math.floor(offset))
    const safeLimit = Math.min(250, Math.max(1, Math.floor(limit)))
    const sql = filterSql(filter)
    const base = `FROM activity a ${sql.joins} WHERE a.account_id = ? AND ${sql.where}`
    const params = [accountId, ...sql.params]
    const total = Number((this.raw.prepare(`SELECT count(*) count ${base}`).get(...params) as Row).count)
    const rows = this.raw.prepare(`SELECT a.* ${base} ORDER BY COALESCE(a.liked_at, a.discovered_at) DESC, a.id DESC LIMIT ? OFFSET ?`)
      .all(...params, safeLimit, safeOffset) as Row[]
    return { items: rows.map(activityFromRow), total, offset: safeOffset }
  }

  confirmSelection(accountId: string, selection: Selection): Job {
    const id = randomUUID()
    const timestamp = now()
    const insertOne = this.raw.prepare(`
      INSERT OR IGNORE INTO job_items (job_id, ordinal, activity_id, media_id, shortcode, permalink)
      SELECT ?, ?, id, media_id, shortcode, permalink FROM activity WHERE id = ? AND account_id = ? AND is_liked = 1
    `)
    this.raw.transaction(() => {
      const account = this.getAccount(accountId)
      if (account.scanState === 'scanning') throw new Error('Pause the scan before confirming cleanup.')
      const active = Number((this.raw.prepare(`SELECT count(*) count FROM jobs WHERE account_id = ? AND state != 'completed'`).get(accountId) as Row).count)
      if (active) throw new Error('Finish or resolve the current cleanup first.')
      this.raw.prepare(`INSERT INTO jobs (id, account_id, state, created_at) VALUES (?, ?, 'confirmed', ?)`).run(id, accountId, timestamp)
      if (selection.allMatching) {
        this.raw.exec(`CREATE TEMP TABLE IF NOT EXISTS selection_exclusions (id INTEGER PRIMARY KEY); DELETE FROM selection_exclusions;`)
        const exclude = this.raw.prepare(`INSERT OR IGNORE INTO selection_exclusions (id) VALUES (?)`)
        for (const excludedId of selection.excludedIds) exclude.run(excludedId)
        const sql = filterSql(selection.filter)
        this.raw.prepare(`
          INSERT INTO job_items (job_id, ordinal, activity_id, media_id, shortcode, permalink)
          SELECT ?, row_number() OVER (ORDER BY COALESCE(a.liked_at, a.discovered_at) DESC, a.id DESC), a.id, a.media_id, a.shortcode, a.permalink
          FROM activity a ${sql.joins}
          LEFT JOIN selection_exclusions e ON e.id = a.id
          WHERE a.account_id = ? AND e.id IS NULL AND ${sql.where}
          ORDER BY COALESCE(a.liked_at, a.discovered_at) DESC, a.id DESC
        `).run(id, accountId, ...sql.params)
      } else {
        let ordinal = 1
        for (const activityId of [...new Set(selection.ids)]) {
          if (insertOne.run(id, ordinal, activityId, accountId).changes) ordinal++
        }
      }
      const count = Number((this.raw.prepare(`SELECT count(*) count FROM job_items WHERE job_id = ?`).get(id) as Row).count)
      if (!count) throw new Error('Select at least one liked item.')
      this.raw.prepare(`UPDATE jobs SET confirmed_at = ? WHERE id = ?`).run(timestamp, id)
    })()
    return this.getJob(id)
  }

  private jobQuery(where: string): string {
    return `
      SELECT j.*, a.username account_username,
        count(ji.id) total,
        sum(CASE WHEN ji.status = 'pending' THEN 1 ELSE 0 END) pending,
        sum(CASE WHEN ji.status = 'succeeded' THEN 1 ELSE 0 END) succeeded,
        sum(CASE WHEN ji.status = 'already_unliked' THEN 1 ELSE 0 END) already_unliked,
        sum(CASE WHEN ji.status = 'failed' THEN 1 ELSE 0 END) failed,
        sum(CASE WHEN ji.status = 'skipped' THEN 1 ELSE 0 END) skipped,
        sum(CASE WHEN ji.status IN ('ambiguous', 'in_flight') THEN 1 ELSE 0 END) ambiguous
      FROM jobs j JOIN accounts a ON a.id = j.account_id LEFT JOIN job_items ji ON ji.job_id = j.id
      ${where} GROUP BY j.id ORDER BY j.created_at DESC`
  }

  listJobs(accountId?: string): Job[] {
    const rows = accountId
      ? this.raw.prepare(this.jobQuery('WHERE j.account_id = ?')).all(accountId)
      : this.raw.prepare(this.jobQuery('')).all()
    return (rows as Row[]).map(jobFromRow)
  }

  getJob(id: string): Job {
    const row = this.raw.prepare(this.jobQuery('WHERE j.id = ?')).get(id) as Row | undefined
    if (!row) throw new Error('Cleanup not found.')
    return jobFromRow(row)
  }

  setJobState(id: string, state: JobState, message: string | null = null, waitUntil: string | null = null): void {
    const timestamp = now()
    this.raw.prepare(`UPDATE jobs SET state = ?, message = ?, wait_until = ?, started_at = CASE WHEN ? = 'running' THEN COALESCE(started_at, ?) ELSE started_at END, finished_at = CASE WHEN ? = 'completed' THEN ? ELSE NULL END WHERE id = ?`)
      .run(state, message, waitUntil, state, timestamp, state, timestamp, id)
  }

  nextItem(jobId: string, status: ItemState = 'pending'): (AmbiguousItem & { accountId: string }) | null {
    const row = this.raw.prepare(`
      SELECT ji.id, ji.media_id, ji.shortcode, ji.permalink, ji.attempts, ji.message, j.account_id
      FROM job_items ji JOIN jobs j ON j.id = ji.job_id
      WHERE ji.job_id = ? AND ji.status = ? ORDER BY ji.ordinal LIMIT 1
    `).get(jobId, status) as Row | undefined
    return row ? {
      id: Number(row.id), mediaId: String(row.media_id), shortcode: String(row.shortcode), permalink: String(row.permalink),
      attempts: Number(row.attempts), message: row.message === null ? null : String(row.message), accountId: String(row.account_id),
    } : null
  }

  ambiguousItem(jobId: string): AmbiguousItem | null {
    const item = this.nextItem(jobId, 'ambiguous')
    if (!item) return null
    return { id: item.id, mediaId: item.mediaId, shortcode: item.shortcode, permalink: item.permalink, attempts: item.attempts, message: item.message }
  }

  jobItem(jobId: string, itemId: number): { accountId: string; permalink: string } {
    const row = this.raw.prepare(`SELECT j.account_id, ji.permalink FROM job_items ji JOIN jobs j ON j.id = ji.job_id WHERE ji.job_id = ? AND ji.id = ?`).get(jobId, itemId) as Row | undefined
    if (!row) throw new Error('Cleanup item not found.')
    return { accountId: String(row.account_id), permalink: String(row.permalink) }
  }

  beginAttempt(jobId: string, itemId: number): void {
    const result = this.raw.prepare(`UPDATE job_items SET status = 'in_flight', attempts = attempts + 1, attempt_started_at = ?, message = NULL WHERE id = ? AND job_id = ? AND status = 'pending'`)
      .run(now(), itemId, jobId)
    if (!result.changes) throw new Error('Cleanup item is not ready.')
  }

  finishItem(jobId: string, itemId: number, state: Exclude<ItemState, 'pending' | 'in_flight' | 'ambiguous'>, message: string | null = null): void {
    this.raw.transaction(() => {
      const row = this.raw.prepare(`SELECT activity_id FROM job_items WHERE id = ? AND job_id = ?`).get(itemId, jobId) as Row | undefined
      if (!row) throw new Error('Cleanup item not found.')
      this.raw.prepare(`UPDATE job_items SET status = ?, message = ?, finished_at = ? WHERE id = ? AND job_id = ?`).run(state, message, now(), itemId, jobId)
      if (state === 'succeeded' || state === 'already_unliked') this.raw.prepare(`UPDATE activity SET is_liked = 0 WHERE id = ?`).run(row.activity_id)
    })()
  }

  interruptItem(jobId: string, itemId: number, state: JobState, message: string, waitUntil: string | null = null): void {
    this.raw.transaction(() => {
      this.raw.prepare(`UPDATE job_items SET status = 'ambiguous', message = ? WHERE id = ? AND job_id = ? AND status = 'in_flight'`).run(message, itemId, jobId)
      this.setJobState(jobId, state, message, waitUntil)
    })()
  }

  stopBeforeMutation(jobId: string, itemId: number, message: string): void {
    this.raw.transaction(() => {
      const result = this.raw.prepare(`UPDATE job_items SET status = 'pending', attempts = MAX(0, attempts - 1), attempt_started_at = NULL, message = ? WHERE id = ? AND job_id = ? AND status = 'in_flight'`).run(message, itemId, jobId)
      if (!result.changes) throw new Error('Cleanup item is not in progress.')
      this.setJobState(jobId, 'client_update_required', message)
    })()
  }

  resolveAmbiguous(jobId: string, itemId: number, resolution: 'done' | 'retry' | 'skip'): void {
    const status = resolution === 'done' ? 'already_unliked' : resolution === 'retry' ? 'pending' : 'skipped'
    this.raw.transaction(() => {
      const row = this.raw.prepare(`SELECT activity_id FROM job_items WHERE id = ? AND job_id = ? AND status = 'ambiguous'`).get(itemId, jobId) as Row | undefined
      if (!row) throw new Error('This item no longer needs a decision.')
      this.raw.prepare(`UPDATE job_items SET status = ?, message = NULL, finished_at = CASE WHEN ? = 'pending' THEN NULL ELSE ? END WHERE id = ?`).run(status, status, now(), itemId)
      if (resolution === 'done') this.raw.prepare(`UPDATE activity SET is_liked = 0 WHERE id = ?`).run(row.activity_id)
      this.setJobState(jobId, 'paused', resolution === 'retry' ? 'Ready to retry the checked item.' : null)
    })()
  }

  markReconciled(jobId: string, itemId: number, result: 'unliked' | 'liked' | 'unavailable'): void {
    if (result === 'liked') {
      this.raw.prepare(`UPDATE job_items SET status = 'pending', message = NULL WHERE id = ? AND job_id = ? AND status = 'ambiguous'`).run(itemId, jobId)
    } else {
      this.finishItem(jobId, itemId, result === 'unliked' ? 'succeeded' : 'already_unliked')
    }
  }
}
