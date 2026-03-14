import { NavLink } from "react-router"
import { LayoutDashboard, Users, Radio, Cable, LogOut } from "lucide-react"
import { useAuth } from "@/hooks/use-auth"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"

const navItems = [
  { to: "/", icon: LayoutDashboard, label: "概览" },
  { to: "/users", icon: Users, label: "用户管理" },
  { to: "/mountpoints", icon: Radio, label: "挂载点" },
  { to: "/connections", icon: Cable, label: "连接监控" },
]

export function AppSidebar() {
  const { username, logout } = useAuth()

  return (
    <aside className="flex h-screen w-56 flex-col border-r bg-sidebar text-sidebar-foreground">
      <div className="flex h-14 items-center gap-2 px-4 font-semibold">
        <Radio className="h-5 w-5 text-primary" />
        <span>NTRIP Caster</span>
      </div>
      <Separator />
      <nav className="flex-1 space-y-1 p-2">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === "/"}
            className={({ isActive }) =>
              cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
                isActive
                  ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                  : "text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground",
              )
            }
          >
            <item.icon className="h-4 w-4" />
            {item.label}
          </NavLink>
        ))}
      </nav>
      <Separator />
      <div className="p-2">
        <div className="flex items-center justify-between px-3 py-2">
          <span className="text-xs text-muted-foreground truncate">
            {username}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={() => logout()}
            title="退出登录"
          >
            <LogOut className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
    </aside>
  )
}
