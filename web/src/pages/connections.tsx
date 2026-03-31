import { useState, useRef, useMemo } from "react"
import { toast } from "sonner"
import { useVirtualizer } from "@tanstack/react-virtual"
import { useSources, useClients, useKickSource, useKickClient, useMountpoints } from "@/api/hooks"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import type { SourceInfo, ClientInfo } from "@/api/types"

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  return `${h}h ${m}m ${s}s`
}

interface FilterBarProps {
  mountFilter: string
  setMountFilter: (v: string) => void
  usernameFilter: string
  setUsernameFilter: (v: string) => void
  mountpoints: string[]
}

function FilterBar({
  mountFilter,
  setMountFilter,
  usernameFilter,
  setUsernameFilter,
  mountpoints,
}: FilterBarProps) {
  return (
    <div className="flex gap-3 mb-4">
      <Select
        value={mountFilter || "__all__"}
        onValueChange={(v) => setMountFilter(v === "__all__" ? "" : v ?? "")}
      >
        <SelectTrigger className="w-48">
          <SelectValue placeholder="全部挂载点" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">全部挂载点</SelectItem>
          {mountpoints.map((m) => (
            <SelectItem key={m} value={m}>
              {m}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        placeholder="用户名搜索"
        value={usernameFilter}
        onChange={(e) => setUsernameFilter(e.target.value)}
        className="w-48"
      />
    </div>
  )
}

interface VirtualTableProps<T extends SourceInfo | ClientInfo> {
  data: T[]
  type: "source" | "client"
  onKick: (item: T) => void
}

function VirtualTable<T extends SourceInfo | ClientInfo>({ data, type, onKick }: VirtualTableProps<T>) {
  const parentRef = useRef<HTMLDivElement>(null)

  const rowHeight = 53
  const virtualizer = useVirtualizer({
    count: data.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 10,
  })

  if (data.length === 0) {
    return (
      <div className="rounded-md border text-center text-muted-foreground py-8">
        无在线 {type === "source" ? "Source" : "Client"}
      </div>
    )
  }

  const isSource = type === "source"
  const headerHeight = 45
  const minHeight = 300
  const contentHeight = headerHeight + virtualizer.getTotalSize()
  const totalHeight = Math.max(minHeight, contentHeight)

  return (
    <div
      ref={parentRef}
      className="rounded-md border overflow-auto"
      style={{ height: totalHeight, maxHeight: "calc(100vh - 300px)" }}
    >
      <div className="relative" style={{ height: totalHeight - headerHeight }}>
        {/* Fixed header */}
        <div className="sticky top-0 z-10 bg-background border-b grid font-medium text-sm"
          style={{
            gridTemplateColumns: isSource
              ? "1fr 1fr 1fr 120px 100px 80px"
              : "1fr 1fr 1fr 120px 100px 80px",
          }}
        >
          <div className="px-4 py-3">挂载点</div>
          <div className="px-4 py-3">{isSource ? "Source ID" : "Client ID"}</div>
          <div className="px-4 py-3">用户</div>
          <div className="px-4 py-3 text-right">{isSource ? "入站流量" : "出站流量"}</div>
          <div className="px-4 py-3 text-right">时长</div>
          <div className="px-4 py-3 text-right">操作</div>
        </div>

        {/* Virtual rows */}
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const item = data[virtualRow.index]
          return (
            <div
              key={virtualRow.key}
              className="absolute w-full grid border-b text-sm items-center"
              style={{
                height: virtualRow.size,
                transform: `translateY(${virtualRow.start}px)`,
                gridTemplateColumns: isSource
                  ? "1fr 1fr 1fr 120px 100px 80px"
                  : "1fr 1fr 1fr 120px 100px 80px",
              }}
            >
              <div className="px-4 py-3 font-medium truncate">
                {item.mountpoint}
              </div>
              <div className="px-4 py-3 font-mono text-sm text-muted-foreground truncate">
                {isSource ? (item as SourceInfo).source_id : (item as ClientInfo).client_id}
              </div>
              <div className="px-4 py-3 truncate">
                {item.username || <span className="text-muted-foreground">—</span>}
              </div>
              <div className="px-4 py-3 text-right font-mono text-sm">
                {formatBytes(isSource ? (item as SourceInfo).bytes_in : (item as ClientInfo).bytes_out)}
              </div>
              <div className="px-4 py-3 text-right font-mono text-sm">
                {formatDuration(item.duration_seconds)}
              </div>
              <div className="px-4 py-3 text-right">
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={() => onKick(item)}
                >
                  踢除
                </Button>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default function ConnectionsPage() {
  const { data: sources, isLoading: sourcesLoading } = useSources()
  const { data: clients, isLoading: clientsLoading } = useClients()
  const { data: mountpointsData } = useMountpoints({ limit: 1000 })
  const kickSource = useKickSource()
  const kickClient = useKickClient()

  const [mountFilter, setMountFilter] = useState("")
  const [usernameFilter, setUsernameFilter] = useState("")

  const [kickSourceTarget, setKickSourceTarget] = useState<string | null>(null)
  const [kickClientTarget, setKickClientTarget] = useState<{
    id: string
    mount: string
  } | null>(null)

  const mountpoints = useMemo(() => {
    return mountpointsData?.data?.map((m) => m.name) ?? []
  }, [mountpointsData])

  const filteredSources = useMemo(() => {
    if (!sources) return []
    return sources.filter((s) => {
      if (mountFilter && s.mountpoint !== mountFilter) return false
      if (usernameFilter && s.username !== usernameFilter) return false
      return true
    })
  }, [sources, mountFilter, usernameFilter])

  const filteredClients = useMemo(() => {
    if (!clients) return []
    return clients.filter((c) => {
      if (mountFilter && c.mountpoint !== mountFilter) return false
      if (usernameFilter && c.username !== usernameFilter) return false
      return true
    })
  }, [clients, mountFilter, usernameFilter])

  async function handleKickSource() {
    if (!kickSourceTarget) return
    try {
      await kickSource.mutateAsync(kickSourceTarget)
      toast.success(`Source 已从 ${kickSourceTarget} 踢除`)
      setKickSourceTarget(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "踢除失败")
    }
  }

  async function handleKickClient() {
    if (!kickClientTarget) return
    try {
      await kickClient.mutateAsync(kickClientTarget.id)
      toast.success("Client 已踢除")
      setKickClientTarget(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "踢除失败")
    }
  }

  function handleKickSourceItem(item: SourceInfo) {
    setKickSourceTarget(item.mountpoint)
  }

  function handleKickClientItem(item: ClientInfo) {
    setKickClientTarget({ id: item.client_id, mount: item.mountpoint })
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">连接监控</h1>

      <Tabs defaultValue="sources">
        <TabsList>
          <TabsTrigger value="sources">
            Source ({filteredSources.length})
          </TabsTrigger>
          <TabsTrigger value="clients">
            Client ({filteredClients.length})
          </TabsTrigger>
        </TabsList>

        <TabsContent value="sources" className="mt-4">
          <FilterBar
            mountFilter={mountFilter}
            setMountFilter={setMountFilter}
            usernameFilter={usernameFilter}
            setUsernameFilter={setUsernameFilter}
            mountpoints={mountpoints}
          />
          {sourcesLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12" />
              ))}
            </div>
          ) : (
            <VirtualTable
              data={filteredSources}
              type="source"
              onKick={handleKickSourceItem}
            />
          )}
        </TabsContent>

        <TabsContent value="clients" className="mt-4">
          <FilterBar
            mountFilter={mountFilter}
            setMountFilter={setMountFilter}
            usernameFilter={usernameFilter}
            setUsernameFilter={setUsernameFilter}
            mountpoints={mountpoints}
          />
          {clientsLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12" />
              ))}
            </div>
          ) : (
            <VirtualTable
              data={filteredClients}
              type="client"
              onKick={handleKickClientItem}
            />
          )}
        </TabsContent>
      </Tabs>

      {/* Kick Source Confirmation */}
      <AlertDialog
        open={!!kickSourceTarget}
        onOpenChange={(open) => !open && setKickSourceTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认踢除 Source</AlertDialogTitle>
            <AlertDialogDescription>
              确定要踢除挂载点 <strong>{kickSourceTarget}</strong> 的 Source
              吗？该挂载点将变为离线状态。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleKickSource}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              踢除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Kick Client Confirmation */}
      <AlertDialog
        open={!!kickClientTarget}
        onOpenChange={(open) => !open && setKickClientTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认踢除 Client</AlertDialogTitle>
            <AlertDialogDescription>
              确定要踢除挂载点 <strong>{kickClientTarget?.mount}</strong> 上的
              Client <strong>{kickClientTarget?.id}</strong> 吗？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleKickClient}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              踢除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}