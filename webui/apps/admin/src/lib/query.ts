import { QueryClient } from '@tanstack/react-query'
import type { LogFilter } from './types'

// 单一缓存：REST 首屏 + SSE 实时写回同一份，页面只读缓存。
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      refetchOnWindowFocus: false,
      staleTime: 10_000,
    },
  },
})

export const qk = {
  me: ['me'] as const,
  overview: ['overview'] as const,
  logs: (filter: LogFilter) => ['logs', filter] as const,
  plugins: ['plugins'] as const,
  plugin: (id: string) => ['plugins', id] as const,
  assets: ['assets'] as const,
  jobs: ['jobs'] as const,
  config: ['config'] as const,
}
