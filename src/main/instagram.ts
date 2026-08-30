import { BrowserWindow, shell } from 'electron'
import type { Account, InstagramResult, ReconcileResult } from '../shared/types'
import { VanishDatabase } from './database'
import { normalizeInstagramPage } from './instagram-normalize'

const LOGIN_URL = 'https://www.instagram.com/accounts/login/'
const HOME_URL = 'https://www.instagram.com/'
const allowedRemoteHost = (hostname: string): boolean => hostname === 'instagram.com' || hostname.endsWith('.instagram.com') || hostname === 'facebook.com' || hostname.endsWith('.facebook.com')
const instagramUrl = (value: string): boolean => {
  try {
    const url = new URL(value)
    return url.protocol === 'https:' && (url.hostname === 'instagram.com' || url.hostname.endsWith('.instagram.com'))
  } catch { return false }
}

type RemoteResult = { ok: boolean; status?: number; data?: any; message?: string; retryAfter?: number }

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
    win.webContents.on('did-finish-load', () => { void this.detectLogin(account.id) })
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

  private async hasSession(account: Account): Promise<boolean> {
    const win = await this.createWindow(account, false)
    const cookies = await win.webContents.session.cookies.get({ url: HOME_URL, name: 'sessionid' })
    return cookies.some((cookie) => Boolean(cookie.value))
  }

  private async detectLogin(accountId: string): Promise<Account> {
    const account = this.db.getAccount(accountId)
    if (!(await this.hasSession(account))) return account
    const win = this.windows.get(accountId)
    if (!win || win.isDestroyed()) return account
    const response = await win.webContents.executeJavaScript(`(async () => {
      try {
        const api = window.require?.('PolarisInstapi')
        const result = api?.apiGet
          ? await api.apiGet('/api/v1/accounts/current_user/', { query: { edit: 'true' } })
          : await fetch('/api/v1/accounts/current_user/?edit=true', { credentials: 'include', headers: { 'x-requested-with': 'XMLHttpRequest' } }).then(async r => ({ status: r.status, data: await r.json() }))
        const data = result?.data ?? result
        return { username: data?.user?.username ?? data?.username ?? null }
      } catch (error) { return { username: null, message: String(error) } }
    })()`, true) as { username?: string | null }
    const cookie = (await win.webContents.session.cookies.get({ url: HOME_URL, name: 'ds_user_id' }))[0]
    const username = response.username || (cookie?.value ? `account-${cookie.value}` : null)
    if (username) {
      const connected = this.db.connectAccount(accountId, username)
      this.changed(accountId)
      return connected
    }
    return account
  }

  async finishLogin(accountId: string): Promise<Account> {
    const account = await this.detectLogin(accountId)
    if (account.state !== 'connected') {
      await this.showLogin(accountId)
      throw new Error('Finish signing in on Instagram, then try again.')
    }
    return account
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
        const unlike = window.require?.('PolarisAPIUnlikePost')?.unlikePost
        if (unlike) {
          const data = await unlike(mediaId)
          const media = data?.xig_media_unlike?.media
          return { ok: media?.has_liked === false, data, message: media ? 'Instagram still reports this item as liked.' : 'Instagram returned no final state.' }
        }
        const csrf = document.cookie.split('; ').find(value => value.startsWith('csrftoken='))?.split('=')[1] ?? ''
        const response = await fetch('/api/v1/web/likes/' + encodeURIComponent(mediaId) + '/unlike/', { method: 'POST', credentials: 'include', headers: { 'x-csrftoken': csrf, 'x-requested-with': 'XMLHttpRequest', 'x-ig-app-id': '936619743392459' } })
        if (response.redirected && response.url.includes('/accounts/login')) return { ok: false, status: 401, message: 'login required' }
        let data = null; try { data = await response.json() } catch {}
        return { ok: response.ok && data?.status === 'ok', status: response.status, data, message: data?.message, retryAfter: Number(response.headers.get('retry-after')) || undefined }
      } catch (error) { return { ok: false, status: Number(error?.status ?? error?.response?.status) || undefined, message: String(error) } }
    })()`, true) as RemoteResult
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
