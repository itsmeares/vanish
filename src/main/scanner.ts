import type { Account, InstagramPage } from '../shared/types'
import { VanishDatabase } from './database'

interface ScanRemote {
  scanPage(accountId: string, cursor: string | null): Promise<InstagramPage>
  hide(accountId: string): void
}

export class Scanner {
  private readonly active = new Set<string>()
  private readonly pauseRequested = new Set<string>()

  constructor(private readonly db: VanishDatabase, private readonly remote: ScanRemote, private readonly changed: (accountId: string) => void) {}

  pause(accountId: string): void {
    this.pauseRequested.add(accountId)
    if (!this.active.has(accountId)) this.db.updateScan(accountId, 'paused', this.db.getAccount(accountId).scanCursor, 'Scan paused.')
    this.changed(accountId)
  }

  start(accountId: string): void {
    if (this.active.has(accountId)) return
    this.pauseRequested.delete(accountId)
    this.active.add(accountId)
    void this.run(accountId)
      .catch((error) => {
        const detail = error instanceof Error ? error.message : 'Scan stopped.'
        this.db.updateScan(accountId, 'failed', this.db.getAccount(accountId).scanCursor, detail)
        this.changed(accountId)
      })
      .finally(() => this.active.delete(accountId))
  }

  private async run(accountId: string): Promise<void> {
    const account = this.db.getAccount(accountId)
    if (account.state !== 'connected') throw new Error('Connect this Instagram account first.')
    this.remote.hide(accountId)
    let cursor = this.db.startScan(accountId)
    this.changed(accountId)
    while (true) {
      if (this.pauseRequested.has(accountId)) {
        this.db.updateScan(accountId, 'paused', cursor, 'Scan paused.')
        this.changed(accountId)
        return
      }
      try {
        const page = await this.remote.scanPage(accountId, cursor)
        this.db.saveScanPage(accountId, page)
        this.changed(accountId)
        if (!page.hasMore) {
          this.db.finishScan(accountId)
          this.changed(accountId)
          return
        }
        if (!page.cursor || page.cursor === cursor) throw new Error('Instagram returned a repeated scan cursor.')
        cursor = page.cursor
      } catch (error) {
        const reason = error as { kind?: string; message?: string }
        const state: Account['scanState'] = reason.kind === 'rate_limited' ? 'rate_limited' : reason.kind === 'needs_auth' ? 'needs_auth' : reason.kind === 'offline' ? 'offline' : 'failed'
        if (state === 'needs_auth') this.db.updateAccountState(accountId, 'needs_auth', 'Instagram needs your attention.')
        this.db.updateScan(accountId, state, cursor, reason.message ?? 'Scan stopped.')
        this.changed(accountId)
        return
      }
    }
  }
}
