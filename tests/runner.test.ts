import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import type { ActivityFilter, InstagramResult, ReconcileResult } from '../src/shared/types'
import { VanishDatabase } from '../src/main/database'
import { CleanupRunner, type CleanupRemote } from '../src/main/runner'

const filter: ActivityFilter = { search: '', mediaType: '', from: '', to: '' }

function seed(db: VanishDatabase): { accountId: string; jobId: string } {
  const account = db.createAccount()
  db.connectAccount(account.id, 'tester')
  db.saveScanPage(account.id, { cursor: null, hasMore: false, items: [{
    mediaId: '123', shortcode: 'POST123', ownerUsername: 'owner', caption: '', mediaType: 'post', thumbnailUrl: null,
    permalink: 'https://www.instagram.com/p/POST123/', likedAt: null, discoveredAt: '2026-01-01T00:00:00.000Z',
  }] })
  const job = db.confirmSelection(account.id, { filter, allMatching: true, ids: [], excludedIds: [] })
  return { accountId: account.id, jobId: job.id }
}

async function settle(db: VanishDatabase, jobId: string, expected: string): Promise<void> {
  await vi.waitFor(() => expect(db.getJob(jobId).state).toBe(expected), { timeout: 3_000, interval: 5 })
}

describe('CleanupRunner', () => {
  it('records success and completes with an exact summary', async () => {
    const db = new VanishDatabase(':memory:')
    const { jobId } = seed(db)
    const remote: CleanupRemote = { unlike: async () => ({ kind: 'succeeded' }), reconcile: async () => ({ kind: 'ambiguous' }) }
    new CleanupRunner(db, remote, () => undefined).start(jobId)
    await settle(db, jobId, 'completed')
    expect(db.getJob(jobId)).toMatchObject({ total: 1, succeeded: 1, pending: 0, ambiguous: 0 })
    db.close()
  })

  it('reconciles a crash after remote success before another mutation', async () => {
    const path = join(mkdtempSync(join(tmpdir(), 'vanish-test-')), 'db.sqlite')
    let db = new VanishDatabase(path)
    const { jobId } = seed(db)
    const item = db.nextItem(jobId)!
    db.setJobState(jobId, 'running')
    db.beginAttempt(jobId, item.id)
    db.close()

    db = new VanishDatabase(path)
    expect(db.getJob(jobId)).toMatchObject({ state: 'needs_reconciliation', ambiguous: 1 })
    let mutations = 0
    const remote: CleanupRemote = {
      unlike: async () => { mutations++; return { kind: 'succeeded' } },
      reconcile: async () => ({ kind: 'unliked' }),
    }
    new CleanupRunner(db, remote, () => undefined).start(jobId)
    await settle(db, jobId, 'completed')
    expect(mutations).toBe(0)
    expect(db.getJob(jobId).succeeded).toBe(1)
    db.close()
  })

  it('waits on rate limits, then reconciles before a safe retry', async () => {
    const db = new VanishDatabase(':memory:')
    const { jobId } = seed(db)
    const unlikeResults: InstagramResult[] = [{ kind: 'rate_limited', retryAfterMs: 1 }, { kind: 'succeeded' }]
    const reconcileResults: ReconcileResult[] = [{ kind: 'liked' }]
    const remote: CleanupRemote = {
      unlike: async () => unlikeResults.shift()!,
      reconcile: async () => reconcileResults.shift()!,
    }
    const runner = new CleanupRunner(db, remote, () => undefined)
    runner.start(jobId)
    await settle(db, jobId, 'waiting_rate_limit')
    db.setJobState(jobId, 'paused')
    runner.start(jobId)
    await settle(db, jobId, 'completed')
    expect(db.getJob(jobId)).toMatchObject({ succeeded: 1, ambiguous: 0 })
    db.close()
  })

  it('requires a user decision when read-only reconciliation stays ambiguous', async () => {
    const db = new VanishDatabase(':memory:')
    const { jobId } = seed(db)
    const remote: CleanupRemote = { unlike: async () => ({ kind: 'offline' }), reconcile: async () => ({ kind: 'ambiguous' }) }
    const runner = new CleanupRunner(db, remote, () => undefined)
    runner.start(jobId)
    await settle(db, jobId, 'offline')
    runner.start(jobId)
    await settle(db, jobId, 'needs_reconciliation')
    const item = db.ambiguousItem(jobId)!
    db.resolveAmbiguous(jobId, item.id, 'skip')
    runner.start(jobId)
    await settle(db, jobId, 'completed')
    expect(db.getJob(jobId).skipped).toBe(1)
    db.close()
  })

  it('stops for a client update without recording or retrying a mutation', async () => {
    const db = new VanishDatabase(':memory:')
    const { jobId } = seed(db)
    const remote: CleanupRemote = { unlike: async () => ({ kind: 'client_update_required', message: 'Update required.' }), reconcile: async () => ({ kind: 'ambiguous' }) }
    new CleanupRunner(db, remote, () => undefined).start(jobId)
    await settle(db, jobId, 'client_update_required')
    expect(db.getJob(jobId)).toMatchObject({ pending: 1, ambiguous: 0, message: 'Update required.' })
    expect(db.nextItem(jobId)).toMatchObject({ attempts: 0 })
    db.close()
  })
})
