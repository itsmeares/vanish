import { BrowserWindow, session, shell } from 'electron'
import type { Account, InstagramResult, ReconcileResult } from '../shared/types'
import { VanishDatabase } from './database'
import { normalizeInstagramPage, resolveInstagramIdentity, withIdentityTimeout, type InstagramIdentity } from './instagram-normalize'

const LOGIN_URL = 'https://www.instagram.com/accounts/login/'
const HOME_URL = 'https://www.instagram.com/'
const IDENTITY_SOURCE_TIMEOUT = 3_000
const IDENTITY_ACTION_TIMEOUT = 8_000
const allowedRemoteHost = (hostname: string): boolean => hostname === 'instagram.com' || hostname.endsWith('.instagram.com') || hostname === 'facebook.com' || hostname.endsWith('.facebook.com')
const instagramUrl = (value: string): boolean => {
  try {
    const url = new URL(value)
    return url.protocol === 'https:' && (url.hostname === 'instagram.com' || url.hostname.endsWith('.instagram.com'))
  } catch { return false }
}

type RemoteResult = { ok: boolean; status?: number; data?: any; message?: string; retryAfter?: number; clientUpdateRequired?: boolean }

function classifyRemote(result: RemoteResult): InstagramResult {
  const message = result.message || 'Instagram did not return a definite result.'
  if (result.status === 429 || /rate.?limit|please wait/i.test(message)) return { kind: 'rate_limited', retryAfterMs: Math.max(60_000, (result.retryAfter ?? 300) * 1000), message: 'Instagram asked Vanish to wait.' }
  if (result.status === 401 || result.status === 403 || /login|checkpoint|challenge|verification|consent_required/i.test(message)) return { kind: 'needs_auth', message: 'Instagram needs you to sign in or complete a check.' }
  if (/network|offline|failed to fetch|internet/i.test(message)) return { kind: 'offline', message: 'The network connection was lost.' }
  return { kind: 'ambiguous', message }
}

export class InstagramService {
  private readonly windows = new Map<string, BrowserWindow>()

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
    const { account, win } = await this.ensureReady(accountId)
    if (!(await this.hasSession(account))) {
      this.reveal(accountId)
      throw new Error('Finish signing in on Instagram, then check the account again.')
    }
    if (!instagramUrl(win.webContents.getURL())) await win.loadURL(HOME_URL)
    try {
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
        () => win.webContents.executeJavaScript(`(async () => {
          const response = await fetch('/api/v1/accounts/current_user/?edit=true', {
            credentials: 'include',
            headers: { 'x-requested-with': 'XMLHttpRequest' },
            signal: AbortSignal.timeout(${IDENTITY_SOURCE_TIMEOUT}),
          })
          if (!response.ok) throw new Error('Instagram account lookup failed.')
          return response.json()
        })()`, true),
        IDENTITY_SOURCE_TIMEOUT,
      )
    } catch (error) {
      this.reveal(accountId)
      throw error
    }
  }

  async identifyAccount(accountId: string): Promise<string> {
    try { return (await withIdentityTimeout(this.detectIdentity(accountId), IDENTITY_ACTION_TIMEOUT)).username }
    catch (error) { this.reveal(accountId); throw error }
  }

  async bindAccount(accountId: string, expectedUsername: string): Promise<Account> {
    try {
      const identity = await withIdentityTimeout(this.detectIdentity(accountId), IDENTITY_ACTION_TIMEOUT)
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

  async scanPage(accountId: string, cursor: string | null): Promise<ReturnType<typeof normalizeInstagramPage>> {
    const { account, win } = await this.ensureReady(accountId)
    if (!(await this.hasSession(account))) throw Object.assign(new Error('Instagram needs you to sign in again.'), { kind: 'needs_auth' })
    if (!instagramUrl(win.webContents.getURL())) await win.loadURL(HOME_URL)
    const result = await win.webContents.executeJavaScript(`(async () => {
      try {
        const query = ${JSON.stringify({ count: '50', cursor })}
        if (!query.cursor) delete query.cursor
        const api = window.require?.('PolarisInstapi')
        if (api?.apiGet) {
          const response = await api.apiGet('/api/v1/feed/liked/', { query: { count: query.count, max_id: query.cursor } })
          return { ok: true, data: response?.data ?? response }
        }
        const url = new URL('/api/v1/feed/liked/', location.origin)
        url.searchParams.set('count', query.count)
        if (query.cursor) url.searchParams.set('max_id', query.cursor)
        const response = await fetch(url, { credentials: 'include', headers: { 'x-requested-with': 'XMLHttpRequest', 'x-ig-app-id': '936619743392459' } })
        if (response.redirected && response.url.includes('/accounts/login')) return { ok: false, status: 401, message: 'login required' }
        let data = null; try { data = await response.json() } catch {}
        return { ok: response.ok, status: response.status, data, message: data?.message, retryAfter: Number(response.headers.get('retry-after')) || undefined }
      } catch (error) { return { ok: false, status: Number(error?.status ?? error?.response?.status) || undefined, message: String(error) } }
    })()`, true) as RemoteResult
    if (!result.ok) throw Object.assign(new Error(classifyRemote(result).message), classifyRemote(result))
    return normalizeInstagramPage(result.data)
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
