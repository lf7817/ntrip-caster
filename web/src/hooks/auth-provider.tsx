import { useState, useCallback, useEffect, type ReactNode } from "react"
import { AuthContext, type AuthContextType } from "./use-auth"
import { api } from "@/api/client"

export function AuthProvider({ children }: { children: ReactNode }) {
  const [username, setUsername] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const checkAuth = useCallback(async () => {
    try {
      await api.getStats()
      // If stats succeeds the session is valid, but we don't know the username.
      // Preserve existing username if we have one; otherwise mark as authenticated
      // with a placeholder.
      setUsername((prev) => prev ?? "admin")
    } catch {
      setUsername(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    checkAuth()
  }, [checkAuth])

  const login = useCallback(async (user: string, password: string) => {
    const res = await api.login(user, password)
    setUsername(res.username)
  }, [])

  const logout = useCallback(async () => {
    await api.logout()
    setUsername(null)
  }, [])

  const value: AuthContextType = {
    username,
    isAuthenticated: username !== null,
    login,
    logout,
    checkAuth,
  }

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-muted-foreground">Loading...</div>
      </div>
    )
  }

  return <AuthContext value={value}>{children}</AuthContext>
}
