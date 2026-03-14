import { BrowserRouter, Routes, Route } from "react-router"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { AuthProvider } from "@/hooks/auth-provider"
import { ProtectedRoute } from "@/components/layout/protected-route"
import { AppLayout } from "@/components/layout/app-layout"
import LoginPage from "@/pages/login"
import DashboardPage from "@/pages/dashboard"
import UsersPage from "@/pages/users"
import MountpointsPage from "@/pages/mountpoints"
import ConnectionsPage from "@/pages/connections"
import NotFoundPage from "@/pages/not-found"

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <BrowserRouter>
          <AuthProvider>
            <Routes>
              <Route path="/login" element={<LoginPage />} />
              <Route element={<ProtectedRoute />}>
                <Route element={<AppLayout />}>
                  <Route index element={<DashboardPage />} />
                  <Route path="users" element={<UsersPage />} />
                  <Route path="mountpoints" element={<MountpointsPage />} />
                  <Route path="connections" element={<ConnectionsPage />} />
                </Route>
              </Route>
              <Route path="*" element={<NotFoundPage />} />
            </Routes>
          </AuthProvider>
        </BrowserRouter>
        <Toaster position="top-right" richColors />
      </TooltipProvider>
    </QueryClientProvider>
  )
}
