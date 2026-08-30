import { describe, expect, it } from 'vitest'
import type { ActivityFilter, InstagramPage } from '../src/shared/types'
import { VanishDatabase } from '../src/main/database'

const all: ActivityFilter = { search: '', mediaType: '', from: '', to: '' }

function page(start: number, count: number, owner = 'owner'): InstagramPage {
  const discoveredAt = '2026-01-01T00:00:00.000Z'
  return {
    cursor: null,
    hasMore: false,
    items: Array.from({ length: count }, (_, offset) => {
      const value = start + offset
      return {
        mediaId: String(value),
        shortcode: `CODE${value}`,
        ownerUsername: owner,
        caption: `caption ${value}`,
        mediaType: value % 3 === 0 ? 'reel' as const : 'post' as const,
        thumbnailUrl: null,
        permalink: `https://www.instagram.com/p/CODE${value}/`,
        likedAt: new Date(Date.UTC(2025, 0, 1) + value * 1000).toISOString(),
        discoveredAt,
      }
    }),
  }
}

describe('VanishDatabase', () => {
  it('deduplicates locally and keeps accounts separate', () => {
    const db = new VanishDatabase(':memory:')
    const first = db.createAccount()
    const second = db.createAccount()
    db.saveScanPage(first.id, page(1, 3, 'alice'))
    db.saveScanPage(first.id, page(1, 3, 'alice-new'))
    db.saveScanPage(second.id, page(1, 1, 'bob'))

    expect(db.activityPage(first.id, all, 0, 20).total).toBe(3)
    expect(db.activityPage(first.id, { ...all, search: 'alice-new' }, 0, 20).total).toBe(3)
    expect(db.activityPage(second.id, all, 0, 20).items[0]?.ownerUsername).toBe('bob')
    db.close()
  })

  it('freezes exactly the confirmed set', () => {
    const db = new VanishDatabase(':memory:')
    const account = db.createAccount()
    db.saveScanPage(account.id, page(1, 10))
    const job = db.confirmSelection(account.id, { filter: { ...all, mediaType: 'reel' }, allMatching: true, ids: [], excludedIds: [] })
    expect(job.total).toBe(3)

    db.saveScanPage(account.id, page(12, 1))
    expect(db.getJob(job.id).total).toBe(3)
    expect(() => db.raw.prepare(`INSERT INTO job_items (job_id, ordinal, activity_id, media_id, shortcode, permalink) SELECT ?, 99, id, media_id, shortcode, permalink FROM activity WHERE account_id = ? LIMIT 1`).run(job.id, account.id)).toThrow(/immutable/)
    expect(() => db.raw.prepare(`DELETE FROM job_items WHERE job_id = ?`).run(job.id)).toThrow(/immutable/)
    db.close()
  })

  it('queries and snapshots 100,000 items without rendering or replay', () => {
    const db = new VanishDatabase(':memory:')
    const account = db.createAccount()
    for (let start = 1; start <= 100_000; start += 2_000) db.saveScanPage(account.id, page(start, 2_000))

    const latePage = db.activityPage(account.id, all, 99_900, 100)
    expect(latePage.total).toBe(100_000)
    expect(latePage.items).toHaveLength(100)
    const job = db.confirmSelection(account.id, { filter: all, allMatching: true, ids: [], excludedIds: [1, 2, 3] })
    expect(job.total).toBe(99_997)
    db.close()
  }, 20_000)
})
