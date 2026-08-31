import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('privileged Electron boundaries', () => {
  it('keeps Node access out of both renderers and denies permissions', () => {
    const main = readFileSync(new URL('../src/main/index.ts', import.meta.url), 'utf8')
    const instagram = readFileSync(new URL('../src/main/instagram.ts', import.meta.url), 'utf8')
    for (const source of [main, instagram]) {
      expect(source).toContain('nodeIntegration: false')
      expect(source).toContain('contextIsolation: true')
      expect(source).toContain('sandbox: true')
      expect(source).toContain('webSecurity: true')
      expect(source).toContain('setPermissionRequestHandler')
    }
    expect(instagram).not.toContain('preload:')
  })

  it('exposes named IPC methods instead of ipcRenderer', () => {
    const preload = readFileSync(new URL('../src/preload/index.ts', import.meta.url), 'utf8')
    expect(preload).toContain("contextBridge.exposeInMainWorld('vanish', api)")
    expect(preload).not.toMatch(/exposeInMainWorld\([^)]*ipcRenderer/s)
    expect(preload).not.toContain('sendSync')
  })

  it('stops for a client update instead of calling the retired REST unlike endpoint', () => {
    const instagram = readFileSync(new URL('../src/main/instagram.ts', import.meta.url), 'utf8')
    expect(instagram).toContain("window.require?.('PolarisAPIUnlikePost')?.unlikePost")
    expect(instagram).toContain('clientUpdateRequired: true')
    expect(instagram).not.toContain('/api/v1/web/likes/')
  })

  it('uses the current viewer identity and clears only the selected account partition', () => {
    const instagram = readFileSync(new URL('../src/main/instagram.ts', import.meta.url), 'utf8')
    expect(instagram).toContain("window.require?.('PolarisViewer')")
    expect(instagram).not.toContain('ds_user_id')
    expect(instagram).toContain('session.fromPartition(account.partition).clearData()')
    expect(instagram).not.toContain('defaultSession.clearData')
  })

  it('installs the Electron binary before a fresh development launch', () => {
    const pkg = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')) as { scripts: Record<string, string> }
    expect(pkg.scripts.predev).toBe('install-electron --no')
    expect(pkg.scripts.dev).toBe('electron-vite dev')
  })

  it('does not use scan recovery messages as account connection copy', () => {
    const app = readFileSync(new URL('../src/renderer/src/App.tsx', import.meta.url), 'utf8')
    const connectionScreen = app.slice(app.indexOf("if (account.state !== 'connected')"), app.indexOf('return <section className="library">'))
    expect(connectionScreen).not.toContain('account.message')
    expect(connectionScreen).toContain('Sign in to @${account.username} on Instagram')
  })
})
