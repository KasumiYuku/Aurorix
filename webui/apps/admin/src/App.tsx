import { useEffect } from 'react'
import { Navigate, Outlet, Route, Routes, useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Spinner } from '@aurorix/webui'
import { AppShell } from './components/layout'
import { get, setUnauthorizedHandler } from './lib/api'
import { connectLive, disconnectLive } from './lib/live'
import { qk } from './lib/query'
import type { MeView } from './lib/types'
import AssetsPage from './features/assets'
import JobsPage from './features/jobs'
import LoginPage from './features/login'
import LogsPage from './features/logs'
import OverviewPage from './features/overview'
import PluginsPage from './features/plugins'
import SettingsPage from './features/settings'

function UnauthorizedBridge() {
  const navigate = useNavigate()
  useEffect(() => {
    setUnauthorizedHandler(() => navigate('/login', { replace: true }))
    return () => setUnauthorizedHandler(null)
  }, [navigate])
  return null
}

function Splash() {
  return (
    <div className="grid h-full place-items-center bg-background">
      <Spinner className="size-6 text-ink-faint" />
    </div>
  )
}

function RequireAuth() {
  const { data, isPending, isError } = useQuery({
    queryKey: qk.me,
    queryFn: () => get<MeView>('/me'),
    retry: false,
  })
  const authed = !isPending && !isError && Boolean(data?.ok)

  useEffect(() => {
    if (!authed) return
    connectLive()
    return () => disconnectLive()
  }, [authed])

  if (isPending) return <Splash />
  if (!authed) return <Navigate to="/login" replace />
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}

export default function App() {
  return (
    <>
      <UnauthorizedBridge />
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route index element={<Navigate to="/overview" replace />} />
          <Route path="/overview" element={<OverviewPage />} />
          <Route path="/logs" element={<LogsPage />} />
          <Route path="/plugins" element={<PluginsPage />} />
          <Route path="/assets" element={<AssetsPage />} />
          <Route path="/jobs" element={<JobsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/overview" replace />} />
      </Routes>
    </>
  )
}
