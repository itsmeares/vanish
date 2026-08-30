import { join } from 'node:path'
import { app, BrowserWindow, ipcMain, type IpcMainInvokeEvent, session } from 'electron'
import type { ActivityFilter, AppEvent, Selection } from '../shared/types'
import { VanishDatabase } from './database'
import { InstagramService } from './instagram'
import { CleanupRunner } from './runner'
import { Scanner } from './scanner'

let mainWindow: BrowserWindow | null = null
let db: VanishDatabase
let instagram: InstagramService
let runner: CleanupRunner
let scanner: Scanner

const identifier = (value: unknown): string => {
  if (typeof value !== 'string' || !/^[0-9a-f-]{36}$/i.test(value)) throw new Error('Invalid identifier.')
  return value
}

function cleanFilter(value: unknown): ActivityFilter {
  if (!value || typeof value !== 'object') throw new Error('Invalid filter.')
  const input = value as Record<string, unknown>
  const mediaType = typeof input.mediaType === 'string' && ['', 'post', 'reel', 'carousel'].includes(input.mediaType) ? input.mediaType : ''
  return {
    search: typeof input.search === 'string' ? input.search.slice(0, 200) : '',
    mediaType: mediaType as ActivityFilter['mediaType'],
    from: typeof input.from === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(input.from) ? input.from : '',
    to: typeof input.to === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(input.to) ? input.to : '',
  }
}

function cleanIds(value: unknown): number[] {
  if (!Array.isArray(value) || value.length > 200_000 || value.some((id) => !Number.isSafeInteger(id) || id < 1)) throw new Error('Invalid selection.')
  return value as number[]
}

function cleanSelection(value: unknown): Selection {
  if (!value || typeof value !== 'object') throw new Error('Invalid selection.')
  const input = value as Record<string, unknown>
  return {
    filter: cleanFilter(input.filter),
    allMatching: input.allMatching === true,
    ids: cleanIds(input.ids),
    excludedIds: cleanIds(input.excludedIds),
  }
}

function trusted(event: IpcMainInvokeEvent): void {
  const url = event.senderFrame?.url ?? ''
  const dev = !app.isPackaged && /^https?:\/\/(localhost|127\.0\.0\.1):\d+\//.test(url)
  const packaged = app.isPackaged && url.startsWith('file://') && url.includes('/dist/renderer/')
  if (!dev && !packaged) throw new Error('Untrusted renderer.')
}

function handle(channel: string, action: (event: IpcMainInvokeEvent, ...args: any[]) => unknown): void {
  ipcMain.handle(channel, async (event, ...args) => {
    trusted(event)
    return action(event, ...args)
  })
}

function emit(event: AppEvent): void {
  if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send('app:event', event)
}

const queuedEvents = new Set<string>()
function queueEvent(key: string, event: AppEvent): void {
  if (queuedEvents.has(key)) return
  queuedEvents.add(key)
  setTimeout(() => {
    queuedEvents.delete(key)
    emit(event)
  }, 200)
}

function registerIpc(): void {
  handle('accounts:list', () => db.listAccounts())
  handle('accounts:add', async () => {
    const account = db.createAccount()
    emit({ type: 'accounts-changed' })
    await instagram.showLogin(account.id)
    return account
  })
  handle('accounts:show-login', (_event, accountId) => instagram.showLogin(identifier(accountId)))
  handle('accounts:finish-login', async (_event, accountId) => {
    const account = await instagram.finishLogin(identifier(accountId))
    emit({ type: 'accounts-changed' })
    return account
  })
  handle('accounts:scan', (_event, accountId) => scanner.start(identifier(accountId)))
  handle('accounts:pause-scan', (_event, accountId) => scanner.pause(identifier(accountId)))
  handle('activity:page', (_event, accountId, filter, offset, limit) => db.activityPage(identifier(accountId), cleanFilter(filter), Number(offset), Number(limit)))
  handle('jobs:confirm', (_event, accountId, selection) => {
    const job = db.confirmSelection(identifier(accountId), cleanSelection(selection))
    emit({ type: 'job-changed', jobId: job.id })
    return job
  })
  handle('jobs:list', (_event, accountId) => db.listJobs(accountId === undefined ? undefined : identifier(accountId)))
  handle('jobs:get', (_event, jobId) => db.getJob(identifier(jobId)))
  handle('jobs:ambiguous', (_event, jobId) => db.ambiguousItem(identifier(jobId)))
  handle('jobs:start', (_event, jobId) => runner.start(identifier(jobId)))
  handle('jobs:pause', (_event, jobId) => runner.pause(identifier(jobId)))
  handle('jobs:resume', (_event, jobId) => runner.start(identifier(jobId)))
  handle('jobs:resolve', (_event, jobId, itemId, resolution) => {
    const cleanJobId = identifier(jobId)
    if (!Number.isSafeInteger(itemId) || !['done', 'retry', 'skip'].includes(resolution)) throw new Error('Invalid resolution.')
    db.resolveAmbiguous(cleanJobId, itemId, resolution)
    emit({ type: 'job-changed', jobId: cleanJobId })
  })
  handle('jobs:show-item', (_event, jobId, itemId) => {
    const item = db.jobItem(identifier(jobId), Number(itemId))
    return instagram.showItem(item.accountId, item.permalink)
  })
}

async function createMainWindow(): Promise<void> {
  mainWindow = new BrowserWindow({
    width: 1240,
    height: 800,
    minWidth: 900,
    minHeight: 620,
    backgroundColor: '#0d0f12',
    title: 'Vanish',
    autoHideMenuBar: true,
    webPreferences: {
      preload: join(__dirname, '../preload/index.cjs'),
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: true,
      webSecurity: true,
      allowRunningInsecureContent: false,
    },
  })
  mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
  mainWindow.webContents.on('will-navigate', (event) => event.preventDefault())
  mainWindow.on('closed', () => {
    mainWindow = null
    if (process.platform !== 'darwin') app.quit()
  })
  if (process.env.ELECTRON_RENDERER_URL) await mainWindow.loadURL(process.env.ELECTRON_RENDERER_URL)
  else await mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
}

app.whenReady().then(async () => {
  session.defaultSession.setPermissionCheckHandler(() => false)
  session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
  db = new VanishDatabase(join(app.getPath('userData'), 'vanish.sqlite'))
  instagram = new InstagramService(db, () => emit({ type: 'accounts-changed' }))
  runner = new CleanupRunner(db, instagram, (jobId) => queueEvent(`job:${jobId}`, { type: 'job-changed', jobId }))
  scanner = new Scanner(db, instagram, (accountId) => queueEvent(`scan:${accountId}`, { type: 'scan-changed', accountId }))
  registerIpc()
  await createMainWindow()
  app.on('activate', () => { if (BrowserWindow.getAllWindows().length === 0) void createMainWindow() })
})

app.on('before-quit', () => { if (db?.raw.open) db.close() })
app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit() })
