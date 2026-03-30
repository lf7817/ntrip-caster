import { useState } from "react"
import { Plus } from "lucide-react"
import { toast } from "sonner"
import type { MountpointInfo } from "@/api/types"
import {
  useMountpoints,
  useCreateMountpoint,
  useUpdateMountpoint,
  useDeleteMountpoint,
} from "@/api/hooks"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

const FORMAT_OPTIONS = ["RTCM3", "RTCM 3.2", "RTCM 3.3"] as const

export default function MountpointsPage() {
  const { data: mounts, isLoading } = useMountpoints()
  const createMp = useCreateMountpoint()
  const updateMp = useUpdateMountpoint()
  const deleteMp = useDeleteMountpoint()

  const [createOpen, setCreateOpen] = useState(false)
  const [editMount, setEditMount] = useState<MountpointInfo | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<MountpointInfo | null>(null)

  // Create form
  const [newName, setNewName] = useState("")
  const [newDesc, setNewDesc] = useState("")
  const [newFormat, setNewFormat] = useState("RTCM3")
  const [newSourceSecret, setNewSourceSecret] = useState("")
  const [newMaxClients, setNewMaxClients] = useState(0)

  // Edit form
  const [editDesc, setEditDesc] = useState("")
  const [editFormat, setEditFormat] = useState("")
  const [editEnabled, setEditEnabled] = useState(true)
  const [editSourceSecret, setEditSourceSecret] = useState("")
  const [editSourceSecretDirty, setEditSourceSecretDirty] = useState(false)
  const [editMaxClients, setEditMaxClients] = useState(0)

  function openCreate() {
    setNewName("")
    setNewDesc("")
    setNewFormat("RTCM3")
    setNewSourceSecret("")
    setNewMaxClients(0)
    setCreateOpen(true)
  }

  function openEdit(mp: MountpointInfo) {
    setEditMount(mp)
    setEditDesc(mp.description)
    setEditFormat(mp.format)
    setEditEnabled(mp.enabled)
    setEditSourceSecret("")
    setEditSourceSecretDirty(false)
    setEditMaxClients(mp.max_clients)
  }

  async function handleCreate() {
    try {
      await createMp.mutateAsync({
        name: newName,
        description: newDesc,
        format: newFormat || undefined,
        max_clients: newMaxClients || undefined,
        ...(newSourceSecret ? { source_secret: newSourceSecret } : {}),
      })
      toast.success("挂载点创建成功")
      setCreateOpen(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建失败")
    }
  }

  async function handleUpdate() {
    if (!editMount) return
    try {
      await updateMp.mutateAsync({
        id: editMount.id,
        data: {
          description: editDesc,
          format: editFormat,
          enabled: editEnabled,
          max_clients: editMaxClients,
          ...(editSourceSecretDirty ? { source_secret: editSourceSecret } : {}),
        },
      })
      toast.success("挂载点更新成功")
      setEditMount(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新失败")
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return
    try {
      await deleteMp.mutateAsync(deleteTarget.id)
      toast.success("挂载点已删除")
      setDeleteTarget(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败")
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">挂载点管理</h1>
        <Button onClick={openCreate} size="sm">
          <Plus className="mr-1 h-4 w-4" />
          创建挂载点
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-12" />
          ))}
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-16">ID</TableHead>
                <TableHead>名称</TableHead>
                <TableHead>描述</TableHead>
                <TableHead>格式</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>Source</TableHead>
                <TableHead className="text-right">客户端</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!mounts?.length ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-muted-foreground py-8">
                    暂无挂载点
                  </TableCell>
                </TableRow>
              ) : (
                mounts.map((mp) => (
                  <TableRow key={mp.id}>
                    <TableCell>{mp.id}</TableCell>
                    <TableCell className="font-medium">{mp.name}</TableCell>
                    <TableCell className="max-w-48 truncate text-muted-foreground">
                      {mp.description || "—"}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{mp.format}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={mp.enabled ? "default" : "secondary"}>
                        {mp.enabled ? "启用" : "禁用"}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-1.5">
                        <span
                          className={`inline-block h-2 w-2 rounded-full ${
                            mp.source_online ? "bg-green-500" : "bg-gray-300"
                          }`}
                        />
                        {mp.source_online ? "在线" : "离线"}
                      </span>
                    </TableCell>
                    <TableCell className="text-right">
                      {mp.max_clients > 0 ? `${mp.client_count} / ${mp.max_clients}` : mp.client_count}
                    </TableCell>
                    <TableCell className="text-right space-x-2">
                      {mp.antenna_lat != null && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => window.location.href = `/map?mount=${mp.name}`}
                        >
                          位置
                        </Button>
                      )}
                      <Button variant="ghost" size="sm" onClick={() => openEdit(mp)}>
                        编辑
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setDeleteTarget(mp)}
                      >
                        删除
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      )}

      {/* Create Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建挂载点</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label>名称</Label>
              <Input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="RTCM_01" />
            </div>
            <div className="space-y-2">
              <Label>描述</Label>
              <Input value={newDesc} onChange={(e) => setNewDesc(e.target.value)} placeholder="基站描述" />
            </div>
            <div className="space-y-2">
              <Label>格式</Label>
              <Select value={newFormat} onValueChange={(v) => v && setNewFormat(v)}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="选择格式" />
                </SelectTrigger>
                <SelectContent>
                  {FORMAT_OPTIONS.map((fmt) => (
                    <SelectItem key={fmt} value={fmt}>
                      {fmt}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>基站密码（可选）</Label>
              <Input
                type="password"
                value={newSourceSecret}
                onChange={(e) => setNewSourceSecret(e.target.value)}
                placeholder="用于 SOURCE <password> 认证"
              />
              <p className="text-xs text-muted-foreground">
                兼容只发送 SOURCE password、不发送 Authorization 的旧基站设备
              </p>
            </div>
            <div className="space-y-2">
              <Label>最大客户端数</Label>
              <Input
                type="number"
                min={0}
                value={newMaxClients}
                onChange={(e) => setNewMaxClients(parseInt(e.target.value) || 0)}
                placeholder="0 表示无限制"
              />
              <p className="text-xs text-muted-foreground">
                限制该挂载点允许的 Rover 连接数，0 表示无限制
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>取消</Button>
            <Button onClick={handleCreate} disabled={createMp.isPending || !newName}>
              {createMp.isPending ? "创建中…" : "创建"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog open={!!editMount} onOpenChange={(open) => !open && setEditMount(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑挂载点: {editMount?.name}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label>描述</Label>
              <Input value={editDesc} onChange={(e) => setEditDesc(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>格式</Label>
              <Select value={editFormat} onValueChange={(v) => v && setEditFormat(v)}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="选择格式" />
                </SelectTrigger>
                <SelectContent>
                  {FORMAT_OPTIONS.map((fmt) => (
                    <SelectItem key={fmt} value={fmt}>
                      {fmt}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center justify-between">
              <Label>启用</Label>
              <Switch checked={editEnabled} onCheckedChange={setEditEnabled} />
            </div>
            <div className="space-y-2">
              <Label>修改基站密码（留空则不修改）</Label>
              <div className="flex gap-2">
                <Input
                  type="password"
                  value={editSourceSecret}
                  onChange={(e) => {
                    setEditSourceSecret(e.target.value)
                    setEditSourceSecretDirty(true)
                  }}
                  placeholder="输入新密码"
                />
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setEditSourceSecret("")
                    setEditSourceSecretDirty(true)
                  }}
                >
                  清空
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                清空后，仅支持带 Authorization 的基站（或关闭认证）
              </p>
            </div>
            <div className="space-y-2">
              <Label>最大客户端数</Label>
              <Input
                type="number"
                min={0}
                value={editMaxClients}
                onChange={(e) => setEditMaxClients(parseInt(e.target.value) || 0)}
                placeholder="0 表示无限制"
              />
              <p className="text-xs text-muted-foreground">
                限制该挂载点允许的 Rover 连接数，0 表示无限制
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditMount(null)}>取消</Button>
            <Button onClick={handleUpdate} disabled={updateMp.isPending}>
              {updateMp.isPending ? "保存中…" : "保存"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除挂载点 <strong>{deleteTarget?.name}</strong> 吗？
              {deleteTarget?.source_online && " 当前 Source 在线，删除后连接将断开。"}
              {(deleteTarget?.client_count ?? 0) > 0 &&
                ` 当前有 ${deleteTarget?.client_count} 个客户端连接。`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
