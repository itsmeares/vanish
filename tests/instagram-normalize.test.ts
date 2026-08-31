import { describe, expect, it, vi } from 'vitest'
import { normalizeInstagramIdentity, normalizeInstagramPage, resolveInstagramIdentity } from '../src/main/instagram-normalize'

describe('normalizeInstagramPage', () => {
  it('normalizes current native media shapes and removes duplicate media ids', () => {
    const page = normalizeInstagramPage({
      items: [
        { pk: '10', code: 'POST10', media_type: 1, user: { username: 'alice' }, caption: { text: 'A post' }, image_versions2: { candidates: [{ url: 'https://scontent.cdninstagram.com/a.jpg' }] }, liked_at: 1_710_000_000 },
        { id: 10, shortcode: 'POST10', media_type: 1 },
        { id: '11', code: 'REEL11', media_type: 2, product_type: 'clips', owner: { username: 'bob' } },
        { pk: '12', code: 'CAR12', media_type: 8, carousel_media: [{}] },
        { code: 'missing-id' },
      ],
      more_available: true,
      next_max_id: 'cursor-2',
    }, '2026-01-01T00:00:00.000Z')

    expect(page.items).toHaveLength(3)
    expect(page.items[0]).toMatchObject({ mediaId: '10', mediaType: 'post', ownerUsername: 'alice', permalink: 'https://www.instagram.com/p/POST10/' })
    expect(page.items[1]).toMatchObject({ mediaId: '11', mediaType: 'reel', permalink: 'https://www.instagram.com/reel/REEL11/' })
    expect(page.items[2]?.mediaType).toBe('carousel')
    expect(page).toMatchObject({ cursor: 'cursor-2', hasMore: true })
  })

  it('accepts wrapped responses and ends without a cursor', () => {
    expect(normalizeInstagramPage({ data: { liked_items: [], more_available: true } })).toEqual({ items: [], cursor: null, hasMore: false })
  })
})

describe('normalizeInstagramIdentity', () => {
  it('requires a real username and never fabricates one from a numeric user id', () => {
    expect(normalizeInstagramIdentity({ viewerId: '123', viewer: { username: 'real.user' } })).toEqual({ id: '123', username: 'real.user' })
    expect(normalizeInstagramIdentity({ viewerId: '123', ds_user_id: '123' })).toBeNull()
    expect(normalizeInstagramIdentity({ currentUser: { user: { pk: '456', username: 'fallback_name' } } })).toEqual({ id: '456', username: 'fallback_name' })
  })

  it('returns an available viewer identity without starting the fallback request', async () => {
    const fallback = vi.fn<() => Promise<unknown>>()
    await expect(resolveInstagramIdentity(
      async () => ({ viewerId: '123', viewer: { username: 'real.user' } }),
      fallback,
      10,
    )).resolves.toEqual({ id: '123', username: 'real.user' })
    expect(fallback).not.toHaveBeenCalled()
  })

  it('bounds a hanging identity fallback', async () => {
    vi.useFakeTimers()
    try {
      const result = expect(resolveInstagramIdentity(
        async () => ({ viewerId: '123' }),
        () => new Promise(() => undefined),
        20,
      )).rejects.toThrow('Keep Instagram open')
      await vi.advanceTimersByTimeAsync(20)
      await result
    } finally { vi.useRealTimers() }
  })

  it('uses the fallback after a viewer read hangs', async () => {
    vi.useFakeTimers()
    try {
      const fallback = vi.fn(async () => ({ user: { pk: '456', username: 'fallback_name' } }))
      const result = expect(resolveInstagramIdentity(
        () => new Promise(() => undefined),
        fallback,
        20,
      )).resolves.toEqual({ id: '456', username: 'fallback_name' })
      await vi.advanceTimersByTimeAsync(20)
      await result
      expect(fallback).toHaveBeenCalledOnce()
    } finally { vi.useRealTimers() }
  })

  it('returns an error when neither source contains a complete identity', async () => {
    await expect(resolveInstagramIdentity(
      async () => ({ viewerId: '123' }),
      async () => ({ user: { pk: '456' } }),
      20,
    )).rejects.toThrow('Vanish could not confirm the Instagram account')
  })
})
