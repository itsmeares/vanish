import type { VanishAPI } from '../shared/types'

declare global {
  interface Window { vanish: VanishAPI }
}

export {}
