import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import '@fontsource-variable/fraunces'
import '@fontsource/ibm-plex-sans/400.css'
import '@fontsource/ibm-plex-sans/500.css'
import '@fontsource/ibm-plex-sans/600.css'
import '@fontsource/ibm-plex-mono/400.css'
import './styles.css'
import { AuthProvider } from './lib/auth'
import { Layout } from './components/Layout'
import { Home, NotFound } from './pages/Home'
import { AuthPage, RequireAuth } from './pages/Auth'
import { Dashboard } from './pages/Dashboard'
import { NewPoll } from './pages/NewPoll'
import { PollPage } from './pages/PollPage'
import { Present } from './pages/Present'

const queryClient = new QueryClient({
  defaultOptions: { queries: { refetchOnWindowFocus: false, staleTime: 5_000 } },
})

const router = createBrowserRouter([
  {
    element: <Layout />,
    children: [
      { path: '/', element: <Home /> },
      { path: '/login', element: <AuthPage mode="login" /> },
      { path: '/signup', element: <AuthPage mode="signup" /> },
      { path: '/dashboard', element: <RequireAuth><Dashboard /></RequireAuth> },
      { path: '/new', element: <RequireAuth><NewPoll /></RequireAuth> },
      { path: '/p/:slug', element: <PollPage /> },
      { path: '*', element: <NotFound /> },
    ],
  },
  { path: '/p/:slug/present', element: <Present /> },
])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </QueryClientProvider>
  </StrictMode>,
)
