import { Users, Radio, Cable, AlertTriangle } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { useStats } from "@/api/hooks"

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B"
  const units = ["B", "KB", "MB", "GB"]
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

export default function DashboardPage() {
  const { data: stats, isLoading } = useStats()

  const activeMountpoints =
    stats?.mountpoints?.filter((m) => m.source_online).length ?? 0
  const totalSlowClients =
    stats?.mountpoints?.reduce((sum, m) => sum + m.slow_clients, 0) ?? 0

  if (isLoading) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-semibold">仪表盘</h1>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28" />
          ))}
        </div>
        <Skeleton className="h-64" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">仪表盘</h1>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="在线 Rover"
          value={stats?.total_clients ?? 0}
          icon={<Users className="h-4 w-4 text-muted-foreground" />}
        />
        <StatCard
          title="在线 Source"
          value={stats?.total_sources ?? 0}
          icon={<Radio className="h-4 w-4 text-muted-foreground" />}
        />
        <StatCard
          title="活跃挂载点"
          value={activeMountpoints}
          icon={<Cable className="h-4 w-4 text-muted-foreground" />}
        />
        <StatCard
          title="慢客户端"
          value={totalSlowClients}
          icon={<AlertTriangle className="h-4 w-4 text-muted-foreground" />}
          alert={totalSlowClients > 0}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">挂载点状态</CardTitle>
        </CardHeader>
        <CardContent>
          {!stats?.mountpoints?.length ? (
            <p className="text-sm text-muted-foreground py-8 text-center">
              暂无挂载点
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>Source</TableHead>
                  <TableHead className="text-right">客户端</TableHead>
                  <TableHead className="text-right">流入</TableHead>
                  <TableHead className="text-right">流出</TableHead>
                  <TableHead className="text-right">慢客户端</TableHead>
                  <TableHead className="text-right">踢除次数</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {stats.mountpoints.map((mp) => (
                  <TableRow key={mp.name}>
                    <TableCell className="font-medium">{mp.name}</TableCell>
                    <TableCell>
                      <Badge
                        variant={mp.source_online ? "default" : "secondary"}
                        className="text-xs"
                      >
                        {mp.source_online ? "在线" : "离线"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      {mp.client_count}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatBytes(mp.bytes_in)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatBytes(mp.bytes_out)}
                    </TableCell>
                    <TableCell className="text-right">
                      {mp.slow_clients > 0 ? (
                        <span className="text-destructive font-medium">
                          {mp.slow_clients}
                        </span>
                      ) : (
                        mp.slow_clients
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      {mp.kick_count}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function StatCard({
  title,
  value,
  icon,
  alert,
}: {
  title: string
  value: number
  icon: React.ReactNode
  alert?: boolean
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {title}
        </CardTitle>
        {icon}
      </CardHeader>
      <CardContent>
        <div
          className={`text-2xl font-bold ${alert ? "text-destructive" : ""}`}
        >
          {value.toLocaleString()}
        </div>
      </CardContent>
    </Card>
  )
}
