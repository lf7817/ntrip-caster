import { useState } from "react"
import { toast } from "sonner"
import { useSources, useClients, useKickSource, useKickClient } from "@/api/hooks"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Button } from "@/components/ui/button"
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
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"

export default function ConnectionsPage() {
  const { data: sources, isLoading: sourcesLoading } = useSources()
  const { data: clients, isLoading: clientsLoading } = useClients()
  const kickSource = useKickSource()
  const kickClient = useKickClient()

  const [kickSourceTarget, setKickSourceTarget] = useState<string | null>(null)
  const [kickClientTarget, setKickClientTarget] = useState<{
    id: string
    mount: string
  } | null>(null)

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

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">连接监控</h1>

      <Tabs defaultValue="sources">
        <TabsList>
          <TabsTrigger value="sources">
            Source ({sources?.length ?? 0})
          </TabsTrigger>
          <TabsTrigger value="clients">
            Client ({clients?.length ?? 0})
          </TabsTrigger>
        </TabsList>

        <TabsContent value="sources" className="mt-4">
          {sourcesLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 2 }).map((_, i) => (
                <Skeleton key={i} className="h-12" />
              ))}
            </div>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>挂载点</TableHead>
                    <TableHead>Source ID</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {!sources?.length ? (
                    <TableRow>
                      <TableCell
                        colSpan={3}
                        className="text-center text-muted-foreground py-8"
                      >
                        无在线 Source
                      </TableCell>
                    </TableRow>
                  ) : (
                    sources.map((s) => (
                      <TableRow key={`${s.mountpoint}-${s.source_id}`}>
                        <TableCell className="font-medium">
                          {s.mountpoint}
                        </TableCell>
                        <TableCell className="font-mono text-sm text-muted-foreground">
                          {s.source_id}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => setKickSourceTarget(s.mountpoint)}
                          >
                            踢除
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          )}
        </TabsContent>

        <TabsContent value="clients" className="mt-4">
          {clientsLoading ? (
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
                    <TableHead>挂载点</TableHead>
                    <TableHead>Client ID</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {!clients?.length ? (
                    <TableRow>
                      <TableCell
                        colSpan={3}
                        className="text-center text-muted-foreground py-8"
                      >
                        无在线 Client
                      </TableCell>
                    </TableRow>
                  ) : (
                    clients.map((c) => (
                      <TableRow key={`${c.mountpoint}-${c.client_id}`}>
                        <TableCell className="font-medium">
                          {c.mountpoint}
                        </TableCell>
                        <TableCell className="font-mono text-sm text-muted-foreground">
                          {c.client_id}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() =>
                              setKickClientTarget({
                                id: c.client_id,
                                mount: c.mountpoint,
                              })
                            }
                          >
                            踢除
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
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
