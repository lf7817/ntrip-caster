export interface User {
  id: number
  username: string
  role: "admin" | "base" | "rover"
  enabled: boolean
}

export interface CreateUserReq {
  username: string
  password: string
  role: string
}

export interface UpdateUserReq {
  role?: string
  enabled?: boolean
  password?: string
}

export interface MountpointRow {
  id: number
  name: string
  description: string
  enabled: boolean
  format: string
  source_auth_mode: string
  write_queue: number
  write_timeout_ms: number
  max_clients: number
}

export interface MountpointInfo extends MountpointRow {
  source_online: boolean
  client_count: number
}

export interface CreateMountpointReq {
  name: string
  description: string
  format?: string
  source_secret?: string
  max_clients?: number
}

export interface UpdateMountpointReq {
  description?: string
  format?: string
  enabled?: boolean
  source_secret?: string
  max_clients?: number
}

export interface Binding {
  id: number
  user_id: number
  mountpoint_id: number
  username?: string
  mountpoint_name?: string
}

export interface CreateBindingReq {
  user_id: number
  mountpoint_id: number
}

export interface SourceInfo {
  mountpoint: string
  source_id: string
  user_id: number
  username: string
  bytes_in: number
  connected_at: string
  duration_seconds: number
}

export interface ClientInfo {
  mountpoint: string
  client_id: string
  user_id: number
  username: string
  bytes_out: number
  connected_at: string
  duration_seconds: number
}

export interface MountpointStats {
  name: string
  client_count: number
  source_online: boolean
  bytes_in: number
  bytes_out: number
  slow_clients: number
  kick_count: number
}

export interface SystemStats {
  total_clients: number
  total_sources: number
  mountpoints: MountpointStats[]
  timestamp: string
}
