import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { initTheme } from '@aurorix/webui'
import App from './App'
import { queryClient } from './lib/query'
import { Toaster } from './lib/toast'
import './index.css'

const el = document.getElementById('root')
if (!el) throw new Error('缺少 #root 挂载点')

initTheme()

createRoot(el).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter basename="/admin">
        <App />
      </BrowserRouter>
      <Toaster />
    </QueryClientProvider>
  </StrictMode>,
)
