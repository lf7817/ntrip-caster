import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "./client"
import type { CreateUserReq, UpdateUserReq, CreateMountpointReq, UpdateMountpointReq, CreateBindingReq } from "./types"

export const queryKeys = {
  users: ["users"] as const,
  mountpoints: ["mountpoints"] as const,
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
export function useUsers() {
  return useQuery({
    queryKey: queryKeys.users,
    queryFn: api.listUsers,
  })
}

export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateUserReq) => api.createUser(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.users }),
  })
}

export function useUpdateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: UpdateUserReq }) =>
      api.updateUser(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.users }),
  })
}

export function useDeleteUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.deleteUser(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.users }),
  })
}

// Mountpoints
export function useMountpoints() {
  return useQuery({
    queryKey: queryKeys.mountpoints,
    queryFn: api.listMountpoints,
  })
}

export function useCreateMountpoint() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateMountpointReq) => api.createMountpoint(data),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.mountpoints }),
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
      qc.invalidateQueries({ queryKey: queryKeys.mountpoints }),
  })
}

export function useDeleteMountpoint() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.deleteMountpoint(id),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.mountpoints }),
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
