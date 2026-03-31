import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "./client"
import type { CreateUserReq, UpdateUserReq, CreateMountpointReq, UpdateMountpointReq, CreateBindingReq, ListUsersParams, ListMountpointsParams } from "./types"

export const queryKeys = {
  users: (params?: ListUsersParams) => ["users", params] as const,
  usersAll: ["users"] as const,
  mountpoints: (params?: ListMountpointsParams) => ["mountpoints", params] as const,
  mountpointsAll: ["mountpoints"] as const,
  bindings: ["bindings"] as const,
  sources: ["sources"] as const,
  clients: ["clients"] as const,
  stats: ["stats"] as const,
}

// Stats — 5s polling
export function useStats() {
  return useQuery({
    queryKey: queryKeys.stats,
    queryFn: api.getStats,
    refetchInterval: 5_000,
  })
}

// Users
export function useUsers(params?: ListUsersParams) {
  return useQuery({
    queryKey: queryKeys.users(params),
    queryFn: () => api.listUsers(params),
  })
}

export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateUserReq) => api.createUser(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.usersAll }),
  })
}

export function useUpdateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: UpdateUserReq }) =>
      api.updateUser(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.usersAll }),
  })
}

export function useDeleteUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.deleteUser(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.usersAll }),
  })
}

// Mountpoints
export function useMountpoints(params?: ListMountpointsParams) {
  return useQuery({
    queryKey: queryKeys.mountpoints(params),
    queryFn: () => api.listMountpoints(params),
  })
}

export function useCreateMountpoint() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateMountpointReq) => api.createMountpoint(data),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.mountpointsAll }),
  })
}

export function useUpdateMountpoint() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      data,
    }: {
      id: number
      data: UpdateMountpointReq
    }) => api.updateMountpoint(id, data),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.mountpointsAll }),
  })
}

export function useDeleteMountpoint() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.deleteMountpoint(id),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.mountpointsAll }),
  })
}

// Bindings
export function useBindings() {
  return useQuery({
    queryKey: queryKeys.bindings,
    queryFn: api.listBindings,
  })
}

export function useUserBindings(userId: number) {
  return useQuery({
    queryKey: [...queryKeys.bindings, userId],
    queryFn: () => api.listUserBindings(userId),
    enabled: userId > 0,
  })
}

export function useCreateBinding() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateBindingReq) => api.createBinding(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.bindings }),
  })
}

export function useDeleteBinding() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.deleteBinding(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.bindings }),
  })
}

// Connections — 10s polling
export function useSources() {
  return useQuery({
    queryKey: queryKeys.sources,
    queryFn: api.listSources,
    refetchInterval: 10_000,
  })
}

export function useClients() {
  return useQuery({
    queryKey: queryKeys.clients,
    queryFn: api.listClients,
    refetchInterval: 10_000,
  })
}

export function useKickSource() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (mount: string) => api.kickSource(mount),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.sources })
      qc.invalidateQueries({ queryKey: queryKeys.stats })
    },
  })
}

export function useKickClient() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.kickClient(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.clients })
      qc.invalidateQueries({ queryKey: queryKeys.stats })
    },
  })
}
