import { describe, expect, it, vi } from 'vitest'
import { addInstagramRequestToken, buildInstagramLikesRequest, extractInstagramBootstrapIdentity, identityFromOwnProfile, normalizeInstagramIdentity, normalizeInstagramPage, resolveInstagramIdentity } from '../src/main/instagram-normalize'

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

  it('normalizes the current Activity Center liked-media response', () => {
    const cursor = 'A'.repeat(40)
    const activityCenterParams = '{\\"py/object\\":\\"ActivityCenterParams\\"}'
    const bloks = `(bk.action.map.Make, (bk.action.array.Make, "media_id", "media_code", "media_product_type", "media_type", "media_image_url", "location_name", "icon", "margin_right"), (bk.action.array.Make, "100_200", "REEL100", "clips", (bk.action.i32.Const, 2), "https:\\/\\/cdn.example\\/100.jpg", "", "reels", "0")) AsyncActionWithDataManifest, "com.instagram.privacy.activity_center.liked_next", (bk.action.array.Make, "page_size", "activity_center_params", "cursor", "container_id", "element_id"), (bk.action.array.Make, "9", "${activityCenterParams}", "${cursor}", "123", "456"))`
    const next = JSON.stringify({ pageSize: '9', activityCenterParams: '{"py/object":"ActivityCenterParams"}', cursor, containerId: '123', elementId: '456' })
    expect(normalizeInstagramPage({ payload: { layout: { bloks_payload: { tree: { actions: [bloks] } } } } }, '2026-01-01T00:00:00.000Z')).toEqual({
      items: [{
        mediaId: '100_200',
        shortcode: 'REEL100',
        ownerUsername: '',
        caption: '',
        mediaType: 'reel',
        thumbnailUrl: 'https://cdn.example/100.jpg',
        permalink: 'https://www.instagram.com/reel/REEL100/',
        likedAt: null,
        discoveredAt: '2026-01-01T00:00:00.000Z',
      }],
      cursor: next,
      hasMore: true,
    })
  })

  it('does not mistake an unknown Instagram response for a completed empty scan', () => {
    expect(() => normalizeInstagramPage({ payload: { changed: true } })).toThrow('unsupported Likes response')
  })

  it('builds the next Activity Center request from the persisted manifest', () => {
    const template = { url: 'https://www.instagram.com/async/wbloks/fetch/?appid=com.instagram.privacy.activity_center.liked_refresh&type=action', body: '__req=a&params=%7B%22initial%22%3Atrue%7D' }
    const cursor = addInstagramRequestToken(JSON.stringify({ pageSize: '9', activityCenterParams: '{"py/object":"ActivityCenterParams"}', cursor: 'QVF_cursor', containerId: '123', elementId: '456' }), 'b')
    const request = buildInstagramLikesRequest(template, cursor)
    expect(new URL(request.url).searchParams.get('appid')).toBe('com.instagram.privacy.activity_center.liked_next')
    expect(request.requestToken).toBe('c')
    expect(JSON.parse(new URLSearchParams(request.body).get('params') ?? '')).toEqual({ page_size: '9', activity_center_params: '{"py/object":"ActivityCenterParams","initial_cursor":"QVF_cursor"}', cursor: 'QVF_cursor', container_id: '123', element_id: '456' })
  })

  it('replays the captured refresh request for the first page', () => {
    const request = buildInstagramLikesRequest({ url: 'https://www.instagram.com/async/wbloks/fetch/?appid=com.instagram.privacy.activity_center.liked_refresh&type=action', body: '__req=z&params=%7B%22captured%22%3Atrue%7D' }, null)
    expect(request.requestToken).toBe('10')
    expect(JSON.parse(new URLSearchParams(request.body).get('params') ?? '')).toEqual({ captured: true })
  })
})

describe('normalizeInstagramIdentity', () => {
  it('requires a real username and never fabricates one from a numeric user id', () => {
    expect(normalizeInstagramIdentity({ viewerId: '123', viewer: { username: 'real.user' } })).toEqual({ id: '123', username: 'real.user' })
    expect(normalizeInstagramIdentity({ viewerId: '123', ds_user_id: '123' })).toBeNull()
    expect(normalizeInstagramIdentity({ currentUser: { user: { pk: '456', username: 'fallback_name' } } })).toEqual({ id: '456', username: 'fallback_name' })
  })

  it('uses the session user id only when it agrees with the viewer', () => {
    expect(normalizeInstagramIdentity({ viewer: { username: 'real.user' } }, '123')).toEqual({ id: '123', username: 'real.user' })
    expect(normalizeInstagramIdentity({ viewer: { id: '456', username: 'wrong.user' } }, '123')).toBeNull()
  })

  it('uses a proven own-profile URL but not an unproven or non-Instagram page', () => {
    expect(identityFromOwnProfile('https://www.instagram.com/zerunostar/', true, '123')).toEqual({ id: '123', username: 'zerunostar' })
    expect(identityFromOwnProfile('https://www.instagram.com/someone_else/', false, '123')).toBeNull()
    expect(identityFromOwnProfile('https://example.com/zerunostar/', true, '123')).toBeNull()
  })

  it('reads the current viewer from Instagram bootstrap data', () => {
    const html = '<script>handle([["PolarisViewer", [], {"data":{"id":"123","username":"real.user","biography":"a } brace"},"id":"123"}, 7365]])</script>'
    expect(extractInstagramBootstrapIdentity(html, '123')).toEqual({ id: '123', username: 'real.user' })
    expect(extractInstagramBootstrapIdentity(html, '999')).toBeNull()
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
      )).rejects.toThrow('Reload Instagram')
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
    )).rejects.toThrow('Instagram did not expose the signed-in account')
  })
})
