import { contextBridge, ipcRenderer } from 'electron'
import type { ActivityFilter, AppEvent, Selection, VanishAPI } from '../shared/types'

const api: VanishAPI = {
  accounts: {
    list: () => ipcRenderer.invoke('accounts:list'),
    add: () => ipcRenderer.invoke('accounts:add'),
    showLogin: (accountId) => ipcRenderer.invoke('accounts:show-login', accountId),
    finishLogin: (accountId) => ipcRenderer.invoke('accounts:finish-login', accountId),
    scan: (accountId) => ipcRenderer.invoke('accounts:scan', accountId),
    pauseScan: (accountId) => ipcRenderer.invoke('accounts:pause-scan', accountId),
  },
  activity: {
    page: (accountId: string, filter: ActivityFilter, offset: number, limit: number) => ipcRenderer.invoke('activity:page', accountId, filter, offset, limit),
  },
  jobs: {
    confirm: (accountId: string, selection: Selection) => ipcRenderer.invoke('jobs:confirm', accountId, selection),
    list: (accountId?: string) => ipcRenderer.invoke('jobs:list', accountId),
    get: (jobId: string) => ipcRenderer.invoke('jobs:get', jobId),
    ambiguous: (jobId: string) => ipcRenderer.invoke('jobs:ambiguous', jobId),
    start: (jobId: string) => ipcRenderer.invoke('jobs:start', jobId),
    pause: (jobId: string) => ipcRenderer.invoke('jobs:pause', jobId),
    resume: (jobId: string) => ipcRenderer.invoke('jobs:resume', jobId),
    resolve: (jobId, itemId, resolution) => ipcRenderer.invoke('jobs:resolve', jobId, itemId, resolution),
    showItem: (jobId, itemId) => ipcRenderer.invoke('jobs:show-item', jobId, itemId),
  },
  onEvent: (listener: (event: AppEvent) => void) => {
    const wrapped = (_event: Electron.IpcRendererEvent, value: AppEvent): void => listener(value)
    ipcRenderer.on('app:event', wrapped)
    return () => ipcRenderer.removeListener('app:event', wrapped)
  },
}

contextBridge.exposeInMainWorld('vanish', api)
