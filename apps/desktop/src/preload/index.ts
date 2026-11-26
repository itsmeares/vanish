import { contextBridge, ipcRenderer } from "electron";

// A sandboxed preload can load nothing but Electron, so it stays a thin pipe.
// The main process checks every call name and its arguments.
contextBridge.exposeInMainWorld("vanishBridge", {
  invoke: (name: string, ...args: unknown[]) =>
    ipcRenderer.invoke(name, ...args),
  on: (channel: "state" | "items", listener: (value: unknown) => void) => {
    const handler = (_event: unknown, value: unknown) => listener(value);
    ipcRenderer.on(channel, handler);
    return () => ipcRenderer.off(channel, handler);
  },
});
