import type { ActivityItem, InstagramPage } from '../shared/types'

type UnknownRecord = Record<string, any>

export interface InstagramIdentity {
  id: string
  username: string
}

export interface InstagramLikesRequest {
  url: string
  body: string
  requestToken: string
}

const identityError = 'Instagram did not expose the signed-in account. Reload Instagram, confirm your profile appears, and try again.'

const text = (value: unknown): string => typeof value === 'string' ? value : typeof value === 'number' && Number.isFinite(value) ? String(value) : ''

export function normalizeInstagramIdentity(payload: unknown, expectedId?: string): InstagramIdentity | null {
  const root = (payload && typeof payload === 'object' ? payload : {}) as UnknownRecord
  const viewer = ((root.viewer ?? root.data) && typeof (root.viewer ?? root.data) === 'object' ? root.viewer ?? root.data : {}) as UnknownRecord
  const current = (root.currentUser && typeof root.currentUser === 'object' ? root.currentUser : root) as UnknownRecord
  const user = (current.user && typeof current.user === 'object' ? current.user : current) as UnknownRecord
  const ids = [...new Set([root.viewerId, root.id, viewer.id, viewer.pk, user.pk, user.id].map(text).filter((id) => /^\d+$/.test(id)))]
  if (expectedId && (!/^\d+$/.test(expectedId) || ids.some((id) => id !== expectedId))) return null
  const id = expectedId || ids[0] || ''
  const username = text(root.username) || text(viewer.username) || text(user.username)
  return /^\d+$/.test(id) && /^[A-Za-z0-9._]{1,30}$/.test(username) ? { id, username } : null
}

export function identityFromOwnProfile(url: string, ownsProfile: boolean, sessionUserId: string | undefined): InstagramIdentity | null {
  if (!ownsProfile || !sessionUserId) return null
  try {
    const parsed = new URL(url)
    const username = /^\/([A-Za-z0-9._]{1,30})\/?$/.exec(parsed.pathname)?.[1]
    if (parsed.protocol !== 'https:' || !['instagram.com', 'www.instagram.com'].includes(parsed.hostname) || !username) return null
    return normalizeInstagramIdentity({ id: sessionUserId, username }, sessionUserId)
  } catch { return null }
}

export function extractInstagramBootstrapIdentity(html: string, expectedId?: string): InstagramIdentity | null {
  const marker = /"PolarisViewer"\s*,\s*\[\]\s*,/g
  for (let match = marker.exec(html); match; match = marker.exec(html)) {
    let start = match.index + match[0].length
    while (/\s/.test(html[start] ?? '')) start += 1
    if (html[start] !== '{') continue
    let depth = 0
    let quoted = false
    let escaped = false
    for (let index = start; index < html.length; index += 1) {
      const character = html[index]
      if (quoted) {
        if (escaped) escaped = false
        else if (character === '\\') escaped = true
        else if (character === '"') quoted = false
      } else if (character === '"') quoted = true
      else if (character === '{') depth += 1
      else if (character === '}' && --depth === 0) {
        try {
          const identity = normalizeInstagramIdentity(JSON.parse(html.slice(start, index + 1)), expectedId)
          if (identity) return identity
        } catch { break }
        break
      }
    }
  }
  return null
}

export async function withTimeout<T>(work: Promise<T>, timeoutMs: number, errorMessage = identityError): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    return await Promise.race([
      work,
      new Promise<T>((_resolve, reject) => { timer = setTimeout(() => reject(new Error(errorMessage)), timeoutMs) }),
    ])
  } finally {
    if (timer) clearTimeout(timer)
  }
}

export async function resolveInstagramIdentity(
  readViewer: () => Promise<unknown>,
  readFallback: () => Promise<unknown>,
  timeoutMs: number,
  expectedId?: string,
): Promise<InstagramIdentity> {
  let viewer: unknown = null
  try { viewer = await withTimeout(readViewer(), timeoutMs) } catch { viewer = null }
  const immediate = normalizeInstagramIdentity(viewer, expectedId)
  if (immediate) return immediate
  let fallback: unknown
  try {
    fallback = await withTimeout(readFallback(), timeoutMs)
  } catch { throw new Error(identityError) }
  const identity = normalizeInstagramIdentity(fallback, expectedId)
  if (identity) return identity
  throw new Error(identityError)
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

function normalizeActivityCenterPage(payload: unknown, discoveredAt: string): InstagramPage {
  const root = (payload && typeof payload === 'object' ? payload : {}) as UnknownRecord
  const tree = root.payload?.layout?.bloks_payload?.tree
  if (!tree) throw new Error('Instagram returned an unsupported Likes response. Vanish needs an update before scanning can continue.')
  const strings: string[] = []
  const visit = (value: unknown): void => {
    if (typeof value === 'string') strings.push(value)
    else if (Array.isArray(value)) value.forEach(visit)
    else if (value && typeof value === 'object') Object.values(value).forEach(visit)
  }
  visit(tree)
  const items: Omit<ActivityItem, 'id'>[] = []
  const seen = new Set<string>()
  const media = /\(bk\.action\.map\.Make,\s*\(bk\.action\.array\.Make,\s*"media_id",\s*"media_code",\s*"media_product_type",\s*"media_type",\s*"media_image_url",\s*"location_name",\s*"icon",\s*"margin_right"\),\s*\(bk\.action\.array\.Make,\s*"([^"]*)",\s*"([^"]*)",\s*"([^"]*)",\s*\(bk\.action\.i32\.Const,\s*(\d+)\),\s*"((?:\\.|[^"\\])*)",\s*"((?:\\.|[^"\\])*)",\s*"([^"]*)",\s*"([^"]*)"\)\)/g
  for (const value of strings) {
    for (let match = media.exec(value); match; match = media.exec(value)) {
      const mediaId = match[1]
      const shortcode = match[2]
      if (!mediaId || !shortcode || seen.has(mediaId)) continue
      seen.add(mediaId)
      const mediaType: ActivityItem['mediaType'] = Number(match[4]) === 8 ? 'carousel' : match[3] === 'clips' || match[6] === 'reels' ? 'reel' : 'post'
      items.push({
        mediaId,
        shortcode,
        ownerUsername: '',
        caption: '',
        mediaType,
        thumbnailUrl: unescapeBloksValue(match[5] ?? ''),
        permalink: `https://www.instagram.com/${mediaType === 'reel' ? 'reel' : 'p'}/${encodeURIComponent(shortcode)}/`,
        likedAt: null,
        discoveredAt,
      })
    }
  }
  const cursor = strings.map(extractActivityCenterCursor).find(Boolean) ?? null
  return { items, cursor, hasMore: Boolean(cursor) }
}

function extractActivityCenterCursor(value: string): string | null {
  if (!value.includes('AsyncActionWithDataManifest, "com.instagram.privacy.activity_center.liked_next"')) return null
  const match = value.match(/\(bk\.action\.array\.Make,\s*"page_size",\s*"activity_center_params",\s*"cursor",\s*"container_id",\s*"element_id"\),\s*\(bk\.action\.array\.Make,\s*"([^"\\]*(?:\\.[^"\\]*)*)",\s*"([^"\\]*(?:\\.[^"\\]*)*)",\s*"([^"\\]*(?:\\.[^"\\]*)*)",\s*"([^"\\]*(?:\\.[^"\\]*)*)",\s*"([^"\\]*(?:\\.[^"\\]*)*)"\)/)
  if (!match) return null
  const [pageSize, activityCenterParams, cursor, containerId, elementId] = match.slice(1).map(unescapeBloksValue)
  if (!pageSize || !activityCenterParams || !cursor || !/^\d+$/.test(containerId ?? '') || !/^\d+$/.test(elementId ?? '')) return null
  return JSON.stringify({ pageSize, activityCenterParams, cursor, containerId, elementId })
}

function unescapeBloksValue(input: string): string {
  try { return JSON.parse(`"${input}"`) as string } catch { return input }
}

function incrementRequestToken(value: string): string {
  const parsed = Number.parseInt(value || '0', 36)
  return Number.isSafeInteger(parsed) ? (parsed + 1).toString(36) : value
}

export function buildInstagramLikesRequest(template: { url: string; body: string }, cursor: string | null): InstagramLikesRequest {
  const url = new URL(template.url)
  const body = new URLSearchParams(template.body)
  let previousToken = body.get('__req') ?? '0'
  if (cursor) {
    const state = JSON.parse(cursor) as Record<string, unknown>
    const pageSize = text(state.pageSize)
    const activityCenterParams = text(state.activityCenterParams)
    const nextCursor = text(state.cursor)
    const containerId = text(state.containerId)
    const elementId = text(state.elementId)
    previousToken = text(state.requestToken) || previousToken
    if (!pageSize || !activityCenterParams || !nextCursor || !/^\d+$/.test(containerId) || !/^\d+$/.test(elementId)) throw new Error('Instagram returned an invalid Likes cursor.')
    url.searchParams.set('appid', 'com.instagram.privacy.activity_center.liked_next')
    let activityState: UnknownRecord
    try { activityState = JSON.parse(activityCenterParams) as UnknownRecord } catch { throw new Error('Instagram returned an invalid Likes cursor.') }
    activityState.initial_cursor = nextCursor
    body.set('params', JSON.stringify({ page_size: pageSize, activity_center_params: JSON.stringify(activityState), cursor: nextCursor, container_id: containerId, element_id: elementId }))
  }
  const requestToken = incrementRequestToken(previousToken)
  body.set('__req', requestToken)
  return { url: url.toString(), body: body.toString(), requestToken }
}

export function addInstagramRequestToken(cursor: string, requestToken: string): string {
  return JSON.stringify({ ...(JSON.parse(cursor) as UnknownRecord), requestToken })
}

export function normalizeInstagramPage(payload: unknown, discoveredAt = new Date().toISOString()): InstagramPage {
  const root = (payload && typeof payload === 'object' ? payload : {}) as UnknownRecord
  const body = (root.data && typeof root.data === 'object' ? root.data : root) as UnknownRecord
  const rawItems = [body.items, body.liked_items, body.media].find(Array.isArray) as UnknownRecord[] | undefined
  if (!rawItems) return normalizeActivityCenterPage(payload, discoveredAt)
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
