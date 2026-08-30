import type { InstagramResult, ReconcileResult } from '../shared/types'
import { VanishDatabase } from './database'

export interface CleanupRemote {
  unlike(accountId: string, mediaId: string): Promise<InstagramResult>
  reconcile(accountId: string, mediaId: string): Promise<ReconcileResult>
}

const waitUntil = (milliseconds = 300_000): string => new Date(Date.now() + milliseconds).toISOString()

export class CleanupRunner {
  private readonly active = new Set<string>()
  private readonly pauseRequested = new Set<string>()

  constructor(private readonly db: VanishDatabase, private readonly remote: CleanupRemote, private readonly changed: (jobId: string) => void) {}

  pause(jobId: string): void {
    this.pauseRequested.add(jobId)
    if (!this.active.has(jobId)) {
      this.db.setJobState(jobId, 'paused', 'Paused.')
      this.changed(jobId)
    }
  }

  start(jobId: string): void {
    if (this.active.has(jobId)) return
    this.pauseRequested.delete(jobId)
    this.active.add(jobId)
    void this.run(jobId)
      .catch((error) => {
        const inFlight = this.db.nextItem(jobId, 'in_flight')
        const detail = error instanceof Error ? error.message : 'Unexpected cleanup error.'
        if (inFlight) this.db.interruptItem(jobId, inFlight.id, 'needs_reconciliation', `Vanish stopped before it could save a definite result. ${detail}`)
        else this.db.setJobState(jobId, 'paused', detail)
        this.changed(jobId)
      })
      .finally(() => this.active.delete(jobId))
  }

  private async run(jobId: string): Promise<void> {
    const initial = this.db.getJob(jobId)
    if (initial.state === 'completed') return
    if (initial.waitUntil && Date.parse(initial.waitUntil) > Date.now()) {
      this.db.setJobState(jobId, 'waiting_rate_limit', 'Instagram is still asking Vanish to wait.', initial.waitUntil)
      this.changed(jobId)
      return
    }
    this.db.setJobState(jobId, 'running')
    this.changed(jobId)

    while (true) {
      if (this.pauseRequested.has(jobId)) {
        this.db.setJobState(jobId, 'paused', 'Paused after the current item.')
        this.changed(jobId)
        return
      }

      const ambiguous = this.db.nextItem(jobId, 'ambiguous')
      if (ambiguous) {
        const result = await this.remote.reconcile(ambiguous.accountId, ambiguous.mediaId)
        if (result.kind === 'unliked' || result.kind === 'unavailable' || result.kind === 'liked') {
          this.db.markReconciled(jobId, ambiguous.id, result.kind)
          this.changed(jobId)
          continue
        }
        const state = result.kind === 'rate_limited' ? 'waiting_rate_limit' : result.kind === 'needs_auth' ? 'needs_auth' : result.kind === 'offline' ? 'offline' : 'needs_reconciliation'
        this.db.setJobState(jobId, state, result.message ?? 'Vanish could not confirm the last result.', result.kind === 'rate_limited' ? waitUntil(result.retryAfterMs) : null)
        this.changed(jobId)
        return
      }

      const item = this.db.nextItem(jobId)
      if (!item) {
        this.db.setJobState(jobId, 'completed')
        this.changed(jobId)
        return
      }
      if (item.attempts >= 3) {
        this.db.finishItem(jobId, item.id, 'failed', 'Stopped after three confirmed attempts.')
        this.changed(jobId)
        continue
      }

      this.db.beginAttempt(jobId, item.id)
      this.changed(jobId)
      let result: InstagramResult
      try {
        result = await this.remote.unlike(item.accountId, item.mediaId)
      } catch {
        result = { kind: 'ambiguous', message: 'Vanish lost the result and must check Instagram before retrying.' }
      }
      if (result.kind === 'succeeded' || result.kind === 'already_unliked') {
        this.db.finishItem(jobId, item.id, result.kind)
        this.changed(jobId)
        continue
      }
      const state = result.kind === 'rate_limited' ? 'waiting_rate_limit' : result.kind === 'needs_auth' ? 'needs_auth' : result.kind === 'offline' ? 'offline' : 'needs_reconciliation'
      this.db.interruptItem(jobId, item.id, state, result.message ?? 'Instagram did not return a definite result.', result.kind === 'rate_limited' ? waitUntil(result.retryAfterMs) : null)
      this.changed(jobId)
      return
    }
  }
}
