import { BrowserWindow, session, shell } from 'electron'
import type { Account, InstagramResult, ReconcileResult } from '../shared/types'
import { VanishDatabase } from './database'
import { addInstagramRequestToken, buildInstagramLikesRequest, extractInstagramBootstrapIdentity, identityFromOwnProfile, normalizeInstagramPage, resolveInstagramIdentity, withTimeout, type InstagramIdentity } from './instagram-normalize'

const LOGIN_URL = 'https://www.instagram.com/accounts/login/'
const HOME_URL = 'https://www.instagram.com/'
const LIKES_URL = 'https://www.instagram.com/your_activity/interactions/likes/'
const IDENTITY_SOURCE_TIMEOUT = 3_000
const IDENTITY_ACTION_TIMEOUT = 8_000
const INSTAGRAM_PAGE_TIMEOUT = 20_000
const allowedRemoteHost = (hostname: string): boolean => hostname === 'instagram.com' || hostname.endsWith('.instagram.com') || hostname === 'facebook.com' || hostname.endsWith('.facebook.com')
const instagramUrl = (value: string): boolean => {
  try {
    const url = new URL(value)
    return url.protocol === 'https:' && (url.hostname === 'instagram.com' || url.hostname.endsWith('.instagram.com'))
  } catch { return false }
}

type RemoteResult = { ok: boolean; status?: number; data?: any; message?: string; retryAfter?: number; clientUpdateRequired?: boolean }
type LikesRequest = { url: string; body: string }

function classifyRemote(result: RemoteResult): InstagramResult {
  const message = result.message || 'Instagram did not return a definite result.'
  if (result.status === 429 || /rate.?limit|please wait/i.test(message)) return { kind: 'rate_limited', retryAfterMs: Math.max(60_000, (result.retryAfter ?? 300) * 1000), message: 'Instagram asked Vanish to wait.' }
  if (result.status === 401 || result.status === 403 || /login|checkpoint|challenge|verification|consent_required/i.test(message)) return { kind: 'needs_auth', message: 'Instagram needs you to sign in or complete a check.' }
  if (/network|offline|failed to fetch|internet/i.test(message)) return { kind: 'offline', message: 'The network connection was lost.' }
  return { kind: 'ambiguous', message }
}

export class InstagramService {
  private readonly windows = new Map<string, BrowserWindow>()
  private readonly likesRequests = new Map<string, LikesRequest>()

  constructor(private readonly db: VanishDatabase, private readonly changed: (accountId: string) => void) {}

  private async createWindow(account: Account, show: boolean): Promise<BrowserWindow> {
    const existing = this.windows.get(account.id)
    if (existing && !existing.isDestroyed()) {
      if (show) existing.show()
      return existing
    }
    const win = new BrowserWindow({
      width: 1080,
      height: 760,
      minWidth: 720,
      minHeight: 560,
      show,
      title: `Instagram${account.username ? ` · @${account.username}` : ''}`,
      autoHideMenuBar: true,
      webPreferences: {
        partition: account.partition,
        nodeIntegration: false,
        contextIsolation: true,
        sandbox: true,
        webSecurity: true,
        allowRunningInsecureContent: false,
        backgroundThrottling: false,
      },
    })
    const ses = win.webContents.session
    ses.setPermissionCheckHandler(() => false)
    ses.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
    win.webContents.setWindowOpenHandler(({ url }) => {
      if (instagramUrl(url)) void shell.openExternal(url)
      return { action: 'deny' }
    })
    win.webContents.on('will-navigate', (event, target) => {
      try {
        if (!allowedRemoteHost(new URL(target).hostname)) event.preventDefault()
      } catch { event.preventDefault() }
    })
    win.on('closed', () => this.windows.delete(account.id))
    this.windows.set(account.id, win)
    await win.loadURL(account.state === 'connected' ? HOME_URL : LOGIN_URL)
    return win
  }

  private async ensureReady(accountId: string, show = false): Promise<{ account: Account; win: BrowserWindow }> {
    const account = this.db.getAccount(accountId)
    const win = await this.createWindow(account, show)
    if (win.webContents.isLoading()) await new Promise<void>((resolve) => win.webContents.once('did-finish-load', () => resolve()))
    return { account, win }
  }

  async showLogin(accountId: string): Promise<void> {
    const { win } = await this.ensureReady(accountId, true)
    win.show()
    win.focus()
  }

  private reveal(accountId: string): void {
    const win = this.windows.get(accountId)
    if (win && !win.isDestroyed()) {
      win.show()
      win.focus()
    }
  }

  private async hasSession(account: Account): Promise<boolean> {
    const win = await this.createWindow(account, false)
    const cookies = await win.webContents.session.cookies.get({ url: HOME_URL, name: 'sessionid' })
    return cookies.some((cookie) => Boolean(cookie.value))
  }

  private async detectIdentity(accountId: string): Promise<InstagramIdentity> {
    const { win } = await this.ensureReady(accountId)
    const cookies = await win.webContents.session.cookies.get({ url: HOME_URL })
    if (!cookies.some((cookie) => cookie.name === 'sessionid' && cookie.value)) {
      this.reveal(accountId)
      throw new Error('Finish signing in on Instagram, then check the account again.')
    }
    const sessionUserId = cookies.find((cookie) => cookie.name === 'ds_user_id' && /^\d+$/.test(cookie.value))?.value
    if (!instagramUrl(win.webContents.getURL()) || win.webContents.getURL().includes('/accounts/login')) await win.loadURL(HOME_URL)
    try {
      const readOwnProfile = async (): Promise<InstagramIdentity | null> => {
        const profile = await withTimeout(win.webContents.executeJavaScript(`({
          url: location.href,
          ownsProfile: Boolean(document.querySelector('[href*="/accounts/edit"], [href^="/archive/"]')),
        })`, true), IDENTITY_SOURCE_TIMEOUT).catch(() => null) as { url?: string; ownsProfile?: boolean } | null
        return identityFromOwnProfile(profile?.url ?? '', profile?.ownsProfile === true, sessionUserId)
      }
      const profileIdentity = await readOwnProfile()
      if (profileIdentity) return profileIdentity
      const profileUrl = await withTimeout(win.webContents.executeJavaScript(`(() => {
        const reserved = new Set(['accounts', 'archive', 'direct', 'explore', 'p', 'reel', 'reels', 'stories', 'your_activity'])
        const links = [...document.querySelectorAll('a[href]')]
        const profile = links.find((link) => {
          const url = new URL(link.href, location.href)
          const name = /^\\/([A-Za-z0-9._]{1,30})\\/?$/.exec(url.pathname)?.[1]
          const rect = link.getBoundingClientRect()
          return name && !reserved.has(name) && link.querySelector('img') && (link.closest('nav, [role="navigation"]') || rect.left < 240)
        })
        return profile?.href ?? null
      })()`, true), IDENTITY_SOURCE_TIMEOUT).catch(() => null)
      if (typeof profileUrl === 'string' && instagramUrl(profileUrl)) {
        await win.loadURL(profileUrl)
        const navigatedIdentity = await readOwnProfile()
        if (navigatedIdentity) return navigatedIdentity
      }
      return await resolveInstagramIdentity(
        () => win.webContents.executeJavaScript(`(() => {
          try {
            const viewerModule = window.require?.('PolarisViewer')
            const config = window.require?.('PolarisConfig')
            return {
              viewer: viewerModule?.data ?? config?.getViewerData_DO_NOT_USE?.() ?? null,
              viewerId: viewerModule?.id ?? config?.getViewerId?.() ?? null,
            }
          } catch { return null }
        })()`, true),
        async () => {
          const response = await win.webContents.session.fetch(HOME_URL, {
            credentials: 'include',
            headers: { accept: 'text/html' },
            signal: AbortSignal.timeout(IDENTITY_SOURCE_TIMEOUT),
          })
          if (!response.ok) throw new Error('Instagram account lookup failed.')
          return extractInstagramBootstrapIdentity(await response.text(), sessionUserId)
        },
        IDENTITY_SOURCE_TIMEOUT,
        sessionUserId,
      )
    } catch (error) {
      this.reveal(accountId)
      throw error
    }
  }

  async identifyAccount(accountId: string): Promise<string> {
    try { return (await withTimeout(this.detectIdentity(accountId), IDENTITY_ACTION_TIMEOUT)).username }
    catch (error) { this.reveal(accountId); throw error }
  }

  async bindAccount(accountId: string, expectedUsername: string): Promise<Account> {
    try {
      const identity = await withTimeout(this.detectIdentity(accountId), IDENTITY_ACTION_TIMEOUT)
      if (identity.username.toLowerCase() !== expectedUsername.toLowerCase()) throw new Error('The signed-in Instagram account changed. Check it again before connecting.')
      const account = this.db.connectAccount(accountId, identity.id, identity.username)
      this.changed(accountId)
      this.hide(accountId)
      return account
    } catch (error) {
      this.reveal(accountId)
      throw error
    }
  }

  private async clearBrowserSession(accountId: string): Promise<void> {
    this.db.assertAccountInactive(accountId)
    const account = this.db.getAccount(accountId)
    const win = this.windows.get(accountId)
    if (win && !win.isDestroyed()) win.destroy()
    this.windows.delete(accountId)
    this.likesRequests.delete(accountId)
    await session.fromPartition(account.partition).clearData()
  }

  async signOut(accountId: string): Promise<Account> {
    await this.clearBrowserSession(accountId)
    const account = this.db.signOutAccount(accountId)
    this.changed(accountId)
    return account
  }

  async removeAccount(accountId: string): Promise<void> {
    await this.clearBrowserSession(accountId)
    this.db.removeAccount(accountId)
    this.changed(accountId)
  }

  private async captureLikesRequest(accountId: string, win: BrowserWindow): Promise<LikesRequest> {
    const cached = this.likesRequests.get(accountId)
    if (cached) return cached
    const ses = win.webContents.session
    let timer: ReturnType<typeof setTimeout> | undefined
    const captured = new Promise<LikesRequest>((resolve, reject) => {
      timer = setTimeout(() => reject(new Error('Instagram did not load the Likes page in time.')), INSTAGRAM_PAGE_TIMEOUT)
      ses.webRequest.onBeforeRequest({ urls: ['https://www.instagram.com/async/wbloks/fetch/*'] }, (details, callback) => {
        callback({})
        if (details.webContentsId !== win.webContents.id || details.method !== 'POST' || !details.url.includes('com.instagram.privacy.activity_center.liked_refresh')) return
        const body = Buffer.concat((details.uploadData ?? []).map((part) => part.bytes).filter(Boolean)).toString()
        if (!body.includes('params=')) return
        resolve({ url: details.url, body })
      })
    })
    try {
      const navigation = withTimeout(win.loadURL(LIKES_URL), INSTAGRAM_PAGE_TIMEOUT, 'Instagram did not open the Likes page in time.')
      const [request] = await Promise.all([captured, navigation])
      this.likesRequests.set(accountId, request)
      return request
    } finally {
      if (timer) clearTimeout(timer)
      ses.webRequest.onBeforeRequest(null)
    }
  }

  async scanPage(accountId: string, cursor: string | null): Promise<ReturnType<typeof normalizeInstagramPage>> {
    const { account, win } = await this.ensureReady(accountId)
    if (!(await this.hasSession(account))) throw Object.assign(new Error('Instagram needs you to sign in again.'), { kind: 'needs_auth' })
    if (!cursor) this.likesRequests.delete(accountId)
    const request = buildInstagramLikesRequest(await this.captureLikesRequest(accountId, win), cursor)
    const result = await withTimeout(win.webContents.executeJavaScript(`(async () => {
      try {
        const request = ${JSON.stringify(request)}
        const csrf = document.cookie.match(/(?:^|; )csrftoken=([^;]*)/)?.[1]
        const headers = { accept: '*/*', 'content-type': 'application/x-www-form-urlencoded;charset=UTF-8', 'x-asbd-id': '359341', 'x-ig-app-id': '936619743392459', 'x-requested-with': 'XMLHttpRequest' }
        if (csrf) headers['x-csrftoken'] = decodeURIComponent(csrf)
        const response = await fetch(request.url, { method: 'POST', body: request.body, credentials: 'include', headers, signal: AbortSignal.timeout(${INSTAGRAM_PAGE_TIMEOUT}) })
        if (response.redirected && response.url.includes('/accounts/login')) return { ok: false, status: 401, message: 'login required' }
        const text = await response.text()
        let data = null
        try { data = JSON.parse(text.trim().replace(/^for \\(;;\\);/, '')) }
        catch { return { ok: false, status: response.status, message: 'Instagram returned an unreadable Likes response. Vanish needs an update.' } }
        return { ok: response.ok, status: response.status, data, message: data?.message, retryAfter: Number(response.headers.get('retry-after')) || undefined }
      } catch (error) { return { ok: false, status: Number(error?.status ?? error?.response?.status) || undefined, message: String(error) } }
    })()`, true), INSTAGRAM_PAGE_TIMEOUT + 1_000, 'Instagram did not return Likes data in time.') as RemoteResult
    if (!result.ok) throw Object.assign(new Error(classifyRemote(result).message), classifyRemote(result))
    const page = normalizeInstagramPage(result.data)
    if (page.cursor) page.cursor = addInstagramRequestToken(page.cursor, request.requestToken)
    return page
  }

  async unlike(accountId: string, mediaId: string): Promise<InstagramResult> {
    const { account, win } = await this.ensureReady(accountId)
    if (!(await this.hasSession(account))) return { kind: 'needs_auth', message: 'Instagram needs you to sign in again.' }
    const result = await win.webContents.executeJavaScript(`(async () => {
      try {
        const mediaId = ${JSON.stringify(mediaId)}
        let unlike = null
        try { unlike = window.require?.('PolarisAPIUnlikePost')?.unlikePost } catch {}
        if (unlike) {
          const data = await unlike(mediaId)
          const media = data?.xig_media_unlike?.media
          return { ok: media?.has_liked === false, data, message: media ? 'Instagram still reports this item as liked.' : 'Instagram returned no final state.' }
        }
        return { ok: false, clientUpdateRequired: true, message: 'Instagram changed its web client. Vanish needs an update before cleanup can continue.' }
      } catch (error) { return { ok: false, status: Number(error?.status ?? error?.response?.status) || undefined, message: String(error) } }
    })()`, true) as RemoteResult
    if (result.clientUpdateRequired) return { kind: 'client_update_required', message: result.message }
    return result.ok ? { kind: 'succeeded' } : classifyRemote(result)
  }

  async reconcile(accountId: string, mediaId: string): Promise<ReconcileResult> {
    const { account, win } = await this.ensureReady(accountId)
    if (!(await this.hasSession(account))) return { kind: 'needs_auth', message: 'Instagram needs you to sign in again.' }
    const result = await win.webContents.executeJavaScript(`(async () => {
      try {
        const mediaId = ${JSON.stringify(mediaId)}
        const api = window.require?.('PolarisInstapi')
        const response = api?.apiGet
          ? await api.apiGet('/api/v1/media/{media_id}/info/', { path: { media_id: mediaId } })
          : await fetch('/api/v1/media/' + encodeURIComponent(mediaId) + '/info/', { credentials: 'include', headers: { 'x-requested-with': 'XMLHttpRequest', 'x-ig-app-id': '936619743392459' } }).then(async r => ({ status: r.redirected && r.url.includes('/accounts/login') ? 401 : r.status, data: await r.json() }))
        const data = response?.data ?? response
        const media = data?.items?.[0] ?? data?.item ?? data
        return { ok: typeof media?.has_liked === 'boolean', status: response?.status, data: { hasLiked: media?.has_liked }, message: data?.message }
      } catch (error) { return { ok: false, status: Number(error?.status ?? error?.response?.status) || undefined, message: String(error) } }
    })()`, true) as RemoteResult
    if (result.ok) return result.data?.hasLiked ? { kind: 'liked' } : { kind: 'unliked' }
    if (result.status === 404) return { kind: 'unavailable' }
    const classified = classifyRemote(result)
    if (classified.kind === 'rate_limited') return { kind: 'rate_limited', retryAfterMs: classified.retryAfterMs, message: classified.message }
    if (classified.kind === 'needs_auth') return { kind: 'needs_auth', message: classified.message }
    if (classified.kind === 'offline') return { kind: 'offline', message: classified.message }
    return { kind: 'ambiguous', message: classified.message }
  }

  async showItem(accountId: string, permalink: string): Promise<void> {
    if (!instagramUrl(permalink)) throw new Error('Unsafe Instagram link.')
    const { win } = await this.ensureReady(accountId, true)
    await win.loadURL(permalink)
    win.show()
    win.focus()
  }

  hide(accountId: string): void { this.windows.get(accountId)?.hide() }
}
