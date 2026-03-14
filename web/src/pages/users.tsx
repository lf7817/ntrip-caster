import { useState } from "react"
import { Plus, X } from "lucide-react"
import { toast } from "sonner"
import type { User } from "@/api/types"
import {
  useUsers,
  useCreateUser,
  useUpdateUser,
  useDeleteUser,
  useUserBindings,
  useMountpoints,
  useCreateBinding,
  useDeleteBinding,
} from "@/api/hooks"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { Separator } from "@/components/ui/separator"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"

const roleBadgeVariant: Record<string, "default" | "secondary" | "outline"> = {
  admin: "default",
  base: "secondary",
  rover: "outline",
}

const permBadgeVariant: Record<string, "default" | "secondary" | "outline"> = {
  publish: "default",
  subscribe: "secondary",
  admin: "outline",
}

export default function UsersPage() {
  const { data: users, isLoading } = useUsers()
  const createUser = useCreateUser()
  const updateUser = useUpdateUser()
  const deleteUser = useDeleteUser()

  const [createOpen, setCreateOpen] = useState(false)
  const [editUser, setEditUser] = useState<User | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null)

  // Create form state
  const [newUsername, setNewUsername] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [newRole, setNewRole] = useState("rover")

  // Edit form state
  const [editRole, setEditRole] = useState("")
  const [editEnabled, setEditEnabled] = useState(true)
  const [editPassword, setEditPassword] = useState("")

  function openCreate() {
    setNewUsername("")
    setNewPassword("")
    setNewRole("rover")
    setCreateOpen(true)
  }

  function openEdit(user: User) {
    setEditUser(user)
    setEditRole(user.role)
    setEditEnabled(user.enabled)
    setEditPassword("")
  }

  async function handleCreate() {
    try {
      await createUser.mutateAsync({
        username: newUsername,
        password: newPassword,
        role: newRole,
      })
      toast.success("用户创建成功")
      setCreateOpen(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建失败")
    }
  }

  async function handleUpdate() {
    if (!editUser) return
    try {
      await updateUser.mutateAsync({
        id: editUser.id,
        data: {
          role: editRole,
          enabled: editEnabled,
          ...(editPassword ? { password: editPassword } : {}),
        },
      })
      toast.success("用户更新成功")
      setEditUser(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新失败")
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return
    try {
      await deleteUser.mutateAsync(deleteTarget.id)
      toast.success("用户已删除")
      setDeleteTarget(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败")
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">用户管理</h1>
        <Button onClick={openCreate} size="sm">
          <Plus className="mr-1 h-4 w-4" />
          创建用户
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
                <TableHead>用户名</TableHead>
                <TableHead>角色</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!users?.length ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground py-8">
                    暂无用户
                  </TableCell>
                </TableRow>
              ) : (
                users.map((user) => (
                  <TableRow key={user.id}>
                    <TableCell>{user.id}</TableCell>
                    <TableCell className="font-medium">{user.username}</TableCell>
                    <TableCell>
                      <Badge variant={roleBadgeVariant[user.role] ?? "outline"}>
                        {user.role}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={user.enabled ? "default" : "secondary"}>
                        {user.enabled ? "启用" : "禁用"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right space-x-2">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(user)}>
                        编辑
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setDeleteTarget(user)}
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
            <DialogTitle>创建用户</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label>用户名</Label>
              <Input value={newUsername} onChange={(e) => setNewUsername(e.target.value)} placeholder="username" />
            </div>
            <div className="space-y-2">
              <Label>密码</Label>
              <Input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} placeholder="••••••" />
            </div>
            <div className="space-y-2">
              <Label>角色</Label>
              <Select value={newRole} onValueChange={(v) => v && setNewRole(v)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="admin">admin</SelectItem>
                  <SelectItem value="base">base</SelectItem>
                  <SelectItem value="rover">rover</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>取消</Button>
            <Button onClick={handleCreate} disabled={createUser.isPending || !newUsername || !newPassword}>
              {createUser.isPending ? "创建中…" : "创建"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog with Bindings */}
      <Dialog open={!!editUser} onOpenChange={(open) => !open && setEditUser(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>编辑用户: {editUser?.username}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label>角色</Label>
              <Select value={editRole} onValueChange={(v) => v && setEditRole(v)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="admin">admin</SelectItem>
                  <SelectItem value="base">base</SelectItem>
                  <SelectItem value="rover">rover</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center justify-between">
              <Label>启用</Label>
              <Switch checked={editEnabled} onCheckedChange={setEditEnabled} />
            </div>
            <div className="space-y-2">
              <Label>重置密码（留空则不修改）</Label>
              <Input type="password" value={editPassword} onChange={(e) => setEditPassword(e.target.value)} placeholder="新密码" />
            </div>

            <Separator />

            {editUser && <UserBindingsSection userId={editUser.id} />}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditUser(null)}>取消</Button>
            <Button onClick={handleUpdate} disabled={updateUser.isPending}>
              {updateUser.isPending ? "保存中…" : "保存"}
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
              确定要删除用户 <strong>{deleteTarget?.username}</strong> 吗？此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function UserBindingsSection({ userId }: { userId: number }) {
  const { data: bindings, isLoading } = useUserBindings(userId)
  const { data: mountpoints } = useMountpoints()
  const createBinding = useCreateBinding()
  const deleteBinding = useDeleteBinding()

  const [addMpId, setAddMpId] = useState("")
  const [addPerm, setAddPerm] = useState("subscribe")

  async function handleAdd() {
    if (!addMpId) return
    try {
      await createBinding.mutateAsync({
        user_id: userId,
        mountpoint_id: Number(addMpId),
        permission: addPerm,
      })
      toast.success("绑定添加成功")
      setAddMpId("")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "添加失败")
    }
  }

  async function handleRemove(bindingId: number) {
    try {
      await deleteBinding.mutateAsync(bindingId)
      toast.success("绑定已移除")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "移除失败")
    }
  }

  return (
    <div className="space-y-3">
      <Label className="text-sm font-medium">挂载点绑定</Label>

      {isLoading ? (
        <Skeleton className="h-16" />
      ) : !bindings?.length ? (
        <p className="text-xs text-muted-foreground">暂无绑定关系</p>
      ) : (
        <div className="space-y-1.5">
          {bindings.map((b) => (
            <div
              key={b.id}
              className="flex items-center justify-between rounded-md border px-3 py-1.5 text-sm"
            >
              <div className="flex items-center gap-2">
                <span className="font-medium">{b.mountpoint_name}</span>
                <Badge variant={permBadgeVariant[b.permission] ?? "outline"} className="text-xs">
                  {b.permission}
                </Badge>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6 text-muted-foreground hover:text-destructive"
                onClick={() => handleRemove(b.id)}
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}

      <div className="flex items-end gap-2">
        <div className="flex-1">
          <Select value={addMpId} onValueChange={(v) => v && setAddMpId(v)}>
            <SelectTrigger className="h-8 text-xs">
              <SelectValue placeholder="选择挂载点" />
            </SelectTrigger>
            <SelectContent>
              {mountpoints?.map((mp) => (
                <SelectItem key={mp.id} value={String(mp.id)}>
                  {mp.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="w-28">
          <Select value={addPerm} onValueChange={(v) => v && setAddPerm(v)}>
            <SelectTrigger className="h-8 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="publish">publish</SelectItem>
              <SelectItem value="subscribe">subscribe</SelectItem>
              <SelectItem value="admin">admin</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <Button
          size="sm"
          className="h-8"
          onClick={handleAdd}
          disabled={!addMpId || createBinding.isPending}
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  )
}
