import type {
  User,
  CreateUserReq,
  UpdateUserReq,
  MountpointInfo,
  MountpointRow,
  CreateMountpointReq,
  UpdateMountpointReq,
  Binding,
  CreateBindingReq,
  SourceInfo,
  ClientInfo,
  SystemStats,
} from "./types"

class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
    ...options,
  })

  if (res.status === 401) {
    if (!path.endsWith("/api/login") && window.location.pathname !== "/login") {
      window.location.href = "/login"
    }
    throw new ApiError("Unauthorized", 401)
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: "Request failed" }))
    throw new ApiError(body.error || "Request failed", res.status)
  }

  return res.json()
}

export const api = {
  // Auth
  login: (username: string, password: string) =>
    request<{ status: string; username: string }>("/api/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),

  logout: () => request<{ status: string }>("/api/logout", { method: "POST" }),

  // Users
  listUsers: () => request<User[]>("/api/users"),

  createUser: (data: CreateUserReq) =>
    request<User>("/api/users", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateUser: (id: number, data: UpdateUserReq) =>
    request<{ status: string }>(`/api/users/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteUser: (id: number) =>
    request<{ status: string }>(`/api/users/${id}`, { method: "DELETE" }),

  // Mountpoints
  listMountpoints: () => request<MountpointInfo[]>("/api/mountpoints"),

  createMountpoint: (data: CreateMountpointReq) =>
    request<MountpointRow>("/api/mountpoints", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateMountpoint: (id: number, data: UpdateMountpointReq) =>
    request<{ status: string }>(`/api/mountpoints/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteMountpoint: (id: number) =>
    request<{ status: string }>(`/api/mountpoints/${id}`, {
      method: "DELETE",
    }),

  // Bindings
  listBindings: () => request<Binding[]>("/api/bindings"),

  listUserBindings: (userId: number) =>
    request<Binding[]>(`/api/users/${userId}/bindings`),

  createBinding: (data: CreateBindingReq) =>
    request<{ status: string }>("/api/bindings", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteBinding: (id: number) =>
    request<{ status: string }>(`/api/bindings/${id}`, { method: "DELETE" }),

  // Connections
  listSources: () => request<SourceInfo[]>("/api/sources"),
  listClients: () => request<ClientInfo[]>("/api/clients"),

  kickSource: (mount: string) =>
    request<{ status: string }>(`/api/sources/${mount}`, {
      method: "DELETE",
    }),

  kickClient: (id: string) =>
    request<{ status: string }>(`/api/clients/${id}`, { method: "DELETE" }),

  // Stats
  getStats: () => request<SystemStats>("/api/stats"),
}
