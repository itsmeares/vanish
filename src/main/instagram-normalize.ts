import type { ActivityItem, InstagramPage } from '../shared/types'

type UnknownRecord = Record<string, any>

export interface InstagramIdentity {
  id: string
  username: string
}

const text = (value: unknown): string => typeof value === 'string' ? value : typeof value === 'number' && Number.isFinite(value) ? String(value) : ''

export function normalizeInstagramIdentity(payload: unknown): InstagramIdentity | null {
  const root = (payload && typeof payload === 'object' ? payload : {}) as UnknownRecord
  const viewer = (root.viewer && typeof root.viewer === 'object' ? root.viewer : {}) as UnknownRecord
  const current = (root.currentUser && typeof root.currentUser === 'object' ? root.currentUser : {}) as UnknownRecord
  const user = (current.user && typeof current.user === 'object' ? current.user : current) as UnknownRecord
  const id = text(root.viewerId) || text(viewer.id) || text(viewer.pk) || text(user.pk) || text(user.id)
  const username = text(viewer.username) || text(user.username)
  return /^\d+$/.test(id) && /^[A-Za-z0-9._]{1,30}$/.test(username) ? { id, username } : null
}

function firstImage(item: UnknownRecord): string | null {
  const candidates = item.image_versions2?.candidates
  if (Array.isArray(candidates)) {
    const url = candidates.find((candidate) => typeof candidate?.url === 'string')?.url
    if (url) return url
  }
  return text(item.thumbnail_url) || text(item.display_url) || null
}

function isoTime(value: unknown): string | null {
  if (typeof value === 'number' && Number.isFinite(value)) return new Date(value * 1000).toISOString()
  if (typeof value === 'string' && value) {
    const numeric = Number(value)
    if (Number.isFinite(numeric)) return new Date(numeric * 1000).toISOString()
    const parsed = new Date(value)
    if (!Number.isNaN(parsed.valueOf())) return parsed.toISOString()
  }
  return null
}

function normalizeItem(raw: UnknownRecord, discoveredAt: string): Omit<ActivityItem, 'id'> | null {
  const media = raw.media ?? raw
  const mediaId = text(media.pk) || text(media.id) || text(media.media_id)
  const shortcode = text(media.code) || text(media.shortcode)
  if (!mediaId || !shortcode) return null
  const numericType = Number(media.media_type)
  const mediaType: ActivityItem['mediaType'] = numericType === 8 || Array.isArray(media.carousel_media)
    ? 'carousel'
    : numericType === 2 || media.product_type === 'clips'
      ? 'reel'
      : 'post'
  const caption = typeof media.caption === 'object' ? text(media.caption?.text) : text(media.caption)
  const ownerUsername = text(media.user?.username) || text(media.owner?.username)
  const permalink = `https://www.instagram.com/${mediaType === 'reel' ? 'reel' : 'p'}/${encodeURIComponent(shortcode)}/`
  return {
    mediaId,
    shortcode,
    ownerUsername,
    caption,
    mediaType,
    thumbnailUrl: firstImage(media),
    permalink,
    likedAt: isoTime(raw.liked_at ?? raw.timestamp ?? media.liked_at),
    discoveredAt,
  }
}

export function normalizeInstagramPage(payload: unknown, discoveredAt = new Date().toISOString()): InstagramPage {
  const root = (payload && typeof payload === 'object' ? payload : {}) as UnknownRecord
  const body = (root.data && typeof root.data === 'object' ? root.data : root) as UnknownRecord
  const rawItems = [body.items, body.liked_items, body.media].find(Array.isArray) as UnknownRecord[] | undefined
  const items: Omit<ActivityItem, 'id'>[] = []
  const seen = new Set<string>()
  for (const raw of rawItems ?? []) {
    const item = normalizeItem(raw, discoveredAt)
    if (item && !seen.has(item.mediaId)) {
      seen.add(item.mediaId)
      items.push(item)
    }
  }
  const cursor = text(body.next_max_id) || text(body.next_cursor) || text(body.page_info?.end_cursor) || null
  const hasMore = Boolean(body.more_available ?? body.has_more ?? body.page_info?.has_next_page) && Boolean(cursor)
  return { items, cursor, hasMore }
}
