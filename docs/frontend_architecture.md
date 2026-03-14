# NTRIP Caster Web 管理界面 — 前端架构设计

> 为 NTRIP Caster 提供嵌入式 Web 管理仪表盘，使用 `go:embed` 将前端静态资源打包进 Go 二进制。\
> 生成时间：2026-03-14

---

# 1. 设计目标

- **单二进制交付**：前端构建产物通过 `go:embed` 嵌入 Go 二进制，部署零依赖
- **现代技术栈**：React 19 + Vite 8 + shadcn/ui + Tailwind CSS v4
- **管理仪表盘**：用户管理、挂载点管理、在线连接监控、实时统计
- **响应式设计**：桌面端优先，兼顾平板和移动端
- **开发体验**：Vite HMR 热更新，前后端分离开发，一键构建嵌入

---

# 2. 技术栈

| 分类 | 技术 | 版本 | 说明 |
|------|------|------|------|
| 构建工具 | Vite | 8.0+ | Rolldown 统一打包器，10–30x 构建提速 |
| UI 框架 | React | 19+ | 并发渲染、Server Actions 支持 |
| UI 组件库 | shadcn/ui | latest | 可定制的 Radix 组件，非 npm 包形式 |
| 样式 | Tailwind CSS | v4 | 简化配置，`@import "tailwindcss"` 即可 |
| 路由 | React Router | v7+ | 客户端 SPA 路由 |
| 数据请求 | TanStack Query | v5 | 服务端状态管理、自动轮询、缓存 |
| HTTP 客户端 | ky / fetch | — | 轻量 HTTP 封装 |
| 图标 | Lucide React | latest | shadcn/ui 默认图标库 |
| 图表 | Recharts | latest | shadcn/ui 推荐图表库 |
| 语言 | TypeScript | 5.x | 全量类型覆盖 |
| 包管理器 | pnpm | 9+ | 快速、磁盘友好 |

### 关于 Vite 8

Vite 8（2026-03-12 发布）的核心变化：

- **Rolldown 统一打包器**：替代原有的 esbuild（开发）+ Rollup（生产）双引擎架构
- **@vitejs/plugin-react v6**：使用 Oxc 进行 React Refresh 转换，不再依赖 Babel
- **默认构建目标**：`baseline-widely-available`（2026-01-01 基线浏览器）
- **内置 lightningcss**：CSS 压缩开箱即用
- **Node.js 要求**：20.19+ 或 22.12+

---

# 3. 项目结构

```
ntrip-caster/
├── web/                          # 前端项目根目录
│   ├── public/
│   │   └── favicon.svg
│   │
│   ├── src/
│   │   ├── api/                  # API 客户端与类型定义
│   │   │   ├── client.ts         # HTTP 客户端封装（fetch/ky）
│   │   │   ├── types.ts          # API 请求/响应类型
│   │   │   └── hooks.ts          # TanStack Query hooks
│   │   │
│   │   ├── components/
│   │   │   ├── ui/               # shadcn/ui 组件（自动生成）
│   │   │   │   ├── button.tsx
│   │   │   │   ├── card.tsx
│   │   │   │   ├── table.tsx
│   │   │   │   ├── dialog.tsx
│   │   │   │   ├── badge.tsx
│   │   │   │   ├── input.tsx
│   │   │   │   ├── toast.tsx
│   │   │   │   └── ...
│   │   │   │
│   │   │   └── layout/           # 布局组件
│   │   │       ├── app-layout.tsx     # 主布局（侧边栏 + 内容区）
│   │   │       ├── sidebar.tsx        # 侧边栏导航
│   │   │       ├── header.tsx         # 顶栏
│   │   │       └── protected-route.tsx # 登录守卫
│   │   │
│   │   ├── pages/                # 页面组件
│   │   │   ├── login.tsx         # 登录页
│   │   │   ├── dashboard.tsx     # 仪表盘（统计概览）
│   │   │   ├── users.tsx         # 用户管理
│   │   │   ├── mountpoints.tsx   # 挂载点管理
│   │   │   ├── connections.tsx   # 在线 Source/Client 监控
│   │   │   └── not-found.tsx     # 404 页面
│   │   │
│   │   ├── lib/                  # 通用工具
│   │   │   └── utils.ts          # cn() 等工具函数
│   │   │
│   │   ├── hooks/                # 自定义 React Hooks
│   │   │   └── use-auth.ts       # 登录状态管理
│   │   │
│   │   ├── App.tsx               # 路由定义
│   │   ├── main.tsx              # 入口
│   │   └── index.css             # Tailwind 入口 + shadcn 主题变量
│   │
│   ├── components.json           # shadcn/ui 配置
│   ├── index.html
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── tsconfig.app.json
│   ├── package.json
│   └── pnpm-lock.yaml
│
├── internal/
│   ├── api/
│   │   ├── http_server.go        # 修改：增加前端静态文件服务
│   │   └── ...
│   └── web/
│       └── embed.go              # go:embed 指令，嵌入 web/dist
│
└── ...
```

---

# 4. Go Embed 集成策略

## 4.1 嵌入方式

在 `internal/web/embed.go` 中通过 `go:embed` 嵌入前端构建产物：

```go
package web

import "embed"

//go:embed all:dist
var Assets embed.FS
```

## 4.2 静态文件服务

在 `internal/api/http_server.go` 中增加前端文件服务：

```go
import (
    "io/fs"
    "net/http"
    "ntrip-caster/internal/web"
)

func NewHTTPServer(...) *HTTPServer {
    mux := http.NewServeMux()

    // API 路由（保持不变）
    mux.HandleFunc("POST /api/login", h.Login)
    mux.Handle("/api/", sess.AuthMiddleware(protected))

    // 前端静态文件服务（SPA fallback）
    distFS, _ := fs.Sub(web.Assets, "dist")
    fileServer := http.FileServer(http.FS(distFS))
    mux.Handle("/", spaHandler(fileServer, distFS))

    // ...
}

// spaHandler 处理 SPA 路由回退：
// 如果请求的文件存在则直接返回，否则返回 index.html
func spaHandler(fileServer http.Handler, fsys fs.FS) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Path
        if path == "/" {
            path = "index.html"
        } else {
            path = path[1:] // 去掉前导 /
        }

        if _, err := fs.Stat(fsys, path); err != nil {
            // 文件不存在，返回 index.html（SPA fallback）
            r.URL.Path = "/"
        }
        fileServer.ServeHTTP(w, r)
    })
}
```

## 4.3 构建流程

```bash
# 1. 构建前端
cd web && pnpm build        # 输出到 web/dist/

# 2. 构建 Go 二进制（自动嵌入 web/dist）
cd .. && go build -o bin/caster ./cmd/caster
```

Vite 构建输出到 `web/dist/`，Go embed 从 `internal/web/embed.go` 引用该目录。
需要在 `vite.config.ts` 中配置输出目录：

```typescript
export default defineConfig({
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
})
```

## 4.4 开发模式

开发时前后端分离运行，通过 Vite 代理转发 API 请求：

```typescript
// vite.config.ts
export default defineConfig({
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
})
```

开发工作流：

```bash
# 终端 1：启动 Go 后端
go run ./cmd/caster

# 终端 2：启动 Vite 开发服务器
cd web && pnpm dev
```

访问 `http://localhost:5173` 进行前端开发，API 请求自动代理到 `:8080`。

---

# 5. 页面设计

## 5.1 页面清单与路由

| 路由 | 页面 | 说明 | 认证要求 |
|------|------|------|----------|
| `/login` | 登录页 | 管理员账号登录 | 否 |
| `/` | 仪表盘 | 系统概览与实时统计 | 是 |
| `/users` | 用户管理 | CRUD 用户，角色分配 | 是 |
| `/mountpoints` | 挂载点管理 | CRUD 挂载点，启停控制 | 是 |
| `/connections` | 连接监控 | 在线 Source/Client 列表，踢除操作 | 是 |
| `*` | 404 | 未匹配路由 | — |

## 5.2 登录页 (`/login`)

```
┌──────────────────────────────────────┐
│                                      │
│          NTRIP Caster                │
│          管理控制台                    │
│                                      │
│    ┌──────────────────────────┐      │
│    │  用户名                   │      │
│    └──────────────────────────┘      │
│    ┌──────────────────────────┐      │
│    │  密码                     │      │
│    └──────────────────────────┘      │
│    ┌──────────────────────────┐      │
│    │        登  录             │      │
│    └──────────────────────────┘      │
│                                      │
└──────────────────────────────────────┘
```

- 居中卡片布局，简洁
- 登录成功后写入 session cookie，跳转到 `/`
- 登录失败显示 toast 错误提示

## 5.3 仪表盘 (`/`)

```
┌─────────┬────────────────────────────────────────┐
│         │  NTRIP Caster Dashboard    [admin] [⇥] │
│  导航    ├────────────────────────────────────────┤
│         │                                        │
│ 📊 概览  │  ┌─────────┐ ┌─────────┐ ┌─────────┐  │
│ 👥 用户  │  │ Rover    │ │ Source  │ │ 挂载点   │  │
│ 📡 挂载点│  │   127    │ │    3    │ │   5     │  │
│ 🔗 连接  │  └─────────┘ └─────────┘ └─────────┘  │
│         │                                        │
│         │  ┌───────────────────────────────────┐  │
│         │  │  流量趋势图（Recharts）              │  │
│         │  │  bytes_in / bytes_out              │  │
│         │  └───────────────────────────────────┘  │
│         │                                        │
│         │  挂载点状态一览                          │
│         │  ┌──────┬────────┬──────┬──────┬────┐  │
│         │  │ 名称  │ Source │ 客户端│ 流入  │流出│  │
│         │  ├──────┼────────┼──────┼──────┼────┤  │
│         │  │RTCM01│  ● 在线│   45 │ 1.2M │89M │  │
│         │  │RTCM02│  ○ 离线│    0 │    0 │  0 │  │
│         │  └──────┴────────┴──────┴──────┴────┘  │
│         │                                        │
└─────────┴────────────────────────────────────────┘
```

统计卡片：

| 指标 | 数据源 | 说明 |
|------|--------|------|
| 在线 Rover 总数 | `stats.total_clients` | — |
| 在线 Source 总数 | `stats.total_sources` | — |
| 活跃挂载点 | `stats.mountpoints` 中 `source_online` 的数量 | — |
| 慢客户端 | `stats.mountpoints[].slow_clients` 求和 | 需关注 |

数据轮询策略：

- 使用 TanStack Query 的 `refetchInterval` 自动轮询
- 统计数据：每 **5 秒** 刷新
- 连接列表：每 **10 秒** 刷新
- 用户/挂载点：手动刷新或操作后 `invalidateQueries`

## 5.4 用户管理 (`/users`)

```
┌─────────────────────────────────────────────────┐
│  用户管理                          [+ 创建用户]  │
├─────────────────────────────────────────────────┤
│  ┌──────┬──────────┬──────┬──────┬──────────┐   │
│  │  ID  │  用户名   │ 角色  │ 状态 │   操作    │   │
│  ├──────┼──────────┼──────┼──────┼──────────┤   │
│  │  1   │  admin   │admin │ 启用 │ 编辑 删除  │   │
│  │  2   │  base01  │base  │ 启用 │ 编辑 删除  │   │
│  │  3   │  rover01 │rover │ 禁用 │ 编辑 删除  │   │
│  └──────┴──────────┴──────┴──────┴──────────┘   │
└─────────────────────────────────────────────────┘
```

功能：

- 列表展示所有用户，带角色 Badge（`admin` 紫色 / `base` 蓝色 / `rover` 绿色）
- 创建用户对话框：用户名、密码、角色选择
- 编辑用户对话框：修改角色、启停状态、重置密码
- 删除确认对话框
- 禁止删除当前登录的 admin 用户

## 5.5 挂载点管理 (`/mountpoints`)

```
┌────────────────────────────────────────────────────────────┐
│  挂载点管理                                  [+ 创建挂载点] │
├────────────────────────────────────────────────────────────┤
│  ┌────┬────────┬──────┬──────┬────────┬──────┬─────────┐  │
│  │ ID │  名称   │ 格式 │ 状态  │ Source │ 客户端│  操作    │  │
│  ├────┼────────┼──────┼──────┼────────┼──────┼─────────┤  │
│  │  1 │RTCM_01 │RTCM3 │ 启用 │  ● 在线│   45 │编辑 删除 │  │
│  │  2 │RTCM_02 │RTCM3 │ 启用 │  ○ 离线│    0 │编辑 删除 │  │
│  │  3 │TEST_01 │RTCM3 │ 禁用 │  ○ 离线│    0 │编辑 删除 │  │
│  └────┴────────┴──────┴──────┴────────┴──────┴─────────┘  │
└────────────────────────────────────────────────────────────┘
```

功能：

- Source 在线状态用圆点指示器（绿色在线 / 灰色离线）
- 创建挂载点对话框：名称、描述、格式
- 编辑挂载点对话框：描述、格式、启停状态
- 删除前警告：如果有在线连接会断开

## 5.6 连接监控 (`/connections`)

```
┌─────────────────────────────────────────────────────────────┐
│  连接监控                                                    │
├──────────┬──────────────────────────────────────────────────┤
│          │                                                  │
│  Tab:    │  在线 Source（3）                                  │
│  Source  │  ┌──────────┬──────────────────┬──────────────┐  │
│  Client  │  │  挂载点   │    Source ID      │    操作      │  │
│          │  ├──────────┼──────────────────┼──────────────┤  │
│          │  │ RTCM_01  │ src-abc-123      │   [踢除]     │  │
│          │  │ RTCM_02  │ src-def-456      │   [踢除]     │  │
│          │  └──────────┴──────────────────┴──────────────┘  │
│          │                                                  │
│          │  在线 Client（127）                                │
│          │  ┌──────────┬──────────────────┬──────────────┐  │
│          │  │  挂载点   │    Client ID      │    操作      │  │
│          │  ├──────────┼──────────────────┼──────────────┤  │
│          │  │ RTCM_01  │ cli-789-xyz      │   [踢除]     │  │
│          │  │ RTCM_01  │ cli-abc-000      │   [踢除]     │  │
│          │  └──────────┴──────────────────┴──────────────┘  │
└──────────┴──────────────────────────────────────────────────┘
```

功能：

- Tab 切换 Source / Client 列表
- 按挂载点筛选
- 踢除操作带确认对话框
- 自动轮询刷新（10 秒）

---

# 6. API 集成

## 6.1 HTTP 客户端封装

```typescript
// src/api/client.ts
const BASE_URL = "";

async function request<T>(
  path: string,
  options?: RequestInit
): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    credentials: "include",  // 自动携带 session cookie
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
    ...options,
  });

  if (res.status === 401) {
    window.location.href = "/login";
    throw new Error("Unauthorized");
  }

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Request failed");
  }

  return res.json();
}

export const api = {
  login: (username: string, password: string) =>
    request<{ status: string; username: string }>("/api/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),

  logout: () =>
    request<{ status: string }>("/api/logout", { method: "POST" }),

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
    request<{ status: string }>(`/api/users/${id}`, {
      method: "DELETE",
    }),

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

  // Connections
  listSources: () => request<SourceInfo[]>("/api/sources"),
  listClients: () => request<ClientInfo[]>("/api/clients"),
  kickSource: (mount: string) =>
    request<{ status: string }>(`/api/sources/${mount}`, {
      method: "DELETE",
    }),
  kickClient: (id: string) =>
    request<{ status: string }>(`/api/clients/${id}`, {
      method: "DELETE",
    }),

  // Stats
  getStats: () => request<SystemStats>("/api/stats"),
};
```

## 6.2 TypeScript 类型定义

```typescript
// src/api/types.ts

export interface User {
  id: number;
  username: string;
  role: "admin" | "base" | "rover";
  enabled: boolean;
}

export interface CreateUserReq {
  username: string;
  password: string;
  role: string;
}

export interface UpdateUserReq {
  role?: string;
  enabled?: boolean;
  password?: string;
}

export interface MountpointRow {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  format: string;
  source_auth_mode: string;
  write_queue: number;
  write_timeout_ms: number;
}

export interface MountpointInfo extends MountpointRow {
  source_online: boolean;
  client_count: number;
}

export interface CreateMountpointReq {
  name: string;
  description: string;
  format?: string;
}

export interface UpdateMountpointReq {
  description?: string;
  format?: string;
  enabled?: boolean;
}

export interface SourceInfo {
  mountpoint: string;
  source_id: string;
}

export interface ClientInfo {
  mountpoint: string;
  client_id: string;
}

export interface MountpointStats {
  name: string;
  client_count: number;
  source_online: boolean;
  bytes_in: number;
  bytes_out: number;
  slow_clients: number;
  kick_count: number;
}

export interface SystemStats {
  total_clients: number;
  total_sources: number;
  mountpoints: MountpointStats[];
  timestamp: string;
}
```

## 6.3 TanStack Query Hooks

```typescript
// src/api/hooks.ts
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "./client";

// 查询 Key 常量
export const queryKeys = {
  users: ["users"] as const,
  mountpoints: ["mountpoints"] as const,
  sources: ["sources"] as const,
  clients: ["clients"] as const,
  stats: ["stats"] as const,
};

// --- Stats (5s 轮询) ---
export function useStats() {
  return useQuery({
    queryKey: queryKeys.stats,
    queryFn: api.getStats,
    refetchInterval: 5_000,
  });
}

// --- Users ---
export function useUsers() {
  return useQuery({
    queryKey: queryKeys.users,
    queryFn: api.listUsers,
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: api.createUser,
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.users }),
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: UpdateUserReq }) =>
      api.updateUser(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.users }),
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: api.deleteUser,
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.users }),
  });
}

// --- Mountpoints ---
export function useMountpoints() {
  return useQuery({
    queryKey: queryKeys.mountpoints,
    queryFn: api.listMountpoints,
  });
}

// ... 类似 CRUD mutation hooks

// --- Connections (10s 轮询) ---
export function useSources() {
  return useQuery({
    queryKey: queryKeys.sources,
    queryFn: api.listSources,
    refetchInterval: 10_000,
  });
}

export function useClients() {
  return useQuery({
    queryKey: queryKeys.clients,
    queryFn: api.listClients,
    refetchInterval: 10_000,
  });
}
```

---

# 7. 认证流程

```
用户访问 /
   │
   ▼
ProtectedRoute 检查登录状态
   │
   ├── 未登录 → 重定向到 /login
   │
   └── 已登录 → 渲染目标页面

登录流程：
   POST /api/login { username, password }
      │
      ├── 200 OK → 服务端设置 HttpOnly Cookie (ntrip_session)
      │            → 前端存储 username 到 Context
      │            → 跳转到 /
      │
      └── 401 → 显示错误 Toast

登出流程：
   POST /api/logout
      │
      → 服务端销毁 Session，清除 Cookie
      → 前端清除 Context
      → 跳转到 /login
```

### 登录状态管理

- 使用 React Context 存储当前用户名
- 初始化时尝试调用 `/api/stats` 验证 session 有效性
- 401 响应全局拦截，自动跳转登录页
- Session Cookie 由后端管理（HttpOnly），前端不操作 token

```typescript
// src/hooks/use-auth.ts
interface AuthContextType {
  username: string | null;
  isAuthenticated: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}
```

---

# 8. 布局设计

## 8.1 整体布局

采用 shadcn/ui 的 Sidebar + Header 布局模式：

```
┌──────────────────────────────────────────────────────┐
│  ┌──────────┐ ┌────────────────────────────────────┐ │
│  │          │ │  Header                   [admin ▾]│ │
│  │ Sidebar  │ ├────────────────────────────────────┤ │
│  │          │ │                                    │ │
│  │ 📊 概览   │ │  Page Content                      │ │
│  │ 👥 用户   │ │                                    │ │
│  │ 📡 挂载点 │ │                                    │ │
│  │ 🔗 连接   │ │                                    │ │
│  │          │ │                                    │ │
│  │          │ │                                    │ │
│  │          │ │                                    │ │
│  │  ──────  │ │                                    │ │
│  │  v1.0.0  │ │                                    │ │
│  └──────────┘ └────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
```

## 8.2 响应式策略

| 断点 | 行为 |
|------|------|
| `≥ 1024px` (lg) | 侧边栏常驻展开 |
| `768–1023px` (md) | 侧边栏收缩为图标模式 |
| `< 768px` (sm) | 侧边栏隐藏，通过汉堡菜单打开 Sheet |

## 8.3 主题

- 默认使用 shadcn/ui 的 **zinc** 主题
- 支持 dark/light 模式切换
- 强调色可定制，通过 CSS 变量控制

---

# 9. shadcn/ui 组件使用清单

| 组件 | 使用场景 |
|------|----------|
| `Button` | 操作按钮、表单提交 |
| `Card` | 仪表盘统计卡片 |
| `Table` | 用户列表、挂载点列表、连接列表 |
| `Dialog` | 创建/编辑用户、创建/编辑挂载点、确认踢除 |
| `AlertDialog` | 删除确认 |
| `Input` | 表单输入 |
| `Label` | 表单标签 |
| `Select` | 角色选择、格式选择 |
| `Badge` | 角色标识、状态标识 |
| `Switch` | 启用/禁用切换 |
| `Tabs` | 连接页 Source/Client 切换 |
| `Sidebar` | 侧边栏导航 |
| `Sonner / Toast` | 操作反馈提示 |
| `Skeleton` | 数据加载占位 |
| `DropdownMenu` | 用户头像下拉菜单（退出登录等） |
| `Tooltip` | 图标按钮提示 |
| `Separator` | 分隔线 |
| `Chart` (Recharts) | 流量趋势图 |

---

# 10. Vite 8 配置

```typescript
// web/vite.config.ts
import path from "path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],

  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },

  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },

  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
```

### TypeScript 配置

```jsonc
// web/tsconfig.json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"]
    }
  },
  "include": ["src"]
}
```

---

# 11. 开发工作流

## 11.1 初始化项目

```bash
cd ntrip-caster

# 创建 Vite 项目
pnpm create vite@latest web --template react-ts
cd web
pnpm install

# 安装 Tailwind CSS v4
pnpm add tailwindcss @tailwindcss/vite

# 安装额外依赖
pnpm add react-router @tanstack/react-query lucide-react recharts
pnpm add -D @types/node

# 初始化 shadcn/ui
npx shadcn@latest init

# 安装常用组件
npx shadcn@latest add button card table dialog alert-dialog \
  input label select badge switch tabs sidebar sonner \
  skeleton dropdown-menu tooltip separator chart
```

## 11.2 日常开发

```bash
# 终端 1：后端
go run ./cmd/caster

# 终端 2：前端（HMR 热更新）
cd web && pnpm dev
```

## 11.3 构建发布

```bash
# 一键构建
cd web && pnpm build && cd .. && go build -o bin/caster ./cmd/caster

# 或使用 Makefile
make build
```

## 11.4 推荐 Makefile

```makefile
.PHONY: dev-frontend dev-backend build clean

dev-frontend:
	cd web && pnpm dev

dev-backend:
	go run ./cmd/caster

build: build-frontend build-backend

build-frontend:
	cd web && pnpm build

build-backend:
	go build -o bin/caster ./cmd/caster

clean:
	rm -rf bin/ web/node_modules/ internal/web/dist/
```

---

# 12. 工程注意事项

## 12.1 Go Embed 注意

- `internal/web/dist/` 目录必须在 `go build` 前存在，否则编译失败
- `.gitignore` 应排除 `internal/web/dist/`（构建产物），但可保留一个 `.gitkeep` 文件
- CI 流程中需先 `pnpm build` 再 `go build`

## 12.2 SPA 路由处理

- Go 静态文件服务器需要 SPA fallback 逻辑
- 所有非 `/api/*` 且非静态资源的请求都应返回 `index.html`
- React Router 处理客户端路由

## 12.3 CORS

- 生产环境：前端和 API 同源（均在 `:8080`），无 CORS 问题
- 开发环境：Vite 代理处理，也无 CORS 问题
- 无需在 Go 后端配置 CORS 中间件

## 12.4 安全

- Session Cookie 是 `HttpOnly + SameSite=Strict`，前端无法读取
- 所有 API 请求带 `credentials: "include"` 自动携带 Cookie
- 401 响应全局拦截，防止信息泄露

## 12.5 文件大小预估

| 项 | 预估大小（gzip） |
|----|------------------|
| React + React DOM | ~45 KB |
| React Router | ~15 KB |
| TanStack Query | ~12 KB |
| Recharts | ~45 KB |
| shadcn/ui 组件 | ~30 KB |
| 业务代码 | ~20 KB |
| **合计** | **~170 KB** |

嵌入到 Go 二进制后增加约 **500 KB**（未压缩），Go 可配置 gzip 中间件进一步优化。

---

# 13. 未来可扩展

- [ ] WebSocket 实时推送（替代轮询统计数据）
- [ ] Rover 地图可视化（集成 Leaflet/Mapbox）
- [ ] 流量历史图表（需后端持久化统计数据）
- [ ] 多语言 i18n 支持
- [ ] 用户-挂载点绑定管理界面
- [ ] 导出统计报表
- [ ] PWA 支持（离线访问基础信息）

---

# 14. 实现规模预估

| 模块 | 代码量 |
|------|--------|
| API 客户端与类型 | ~200 行 |
| 认证与路由守卫 | ~100 行 |
| 布局组件 | ~200 行 |
| 登录页 | ~80 行 |
| 仪表盘页 | ~250 行 |
| 用户管理页 | ~300 行 |
| 挂载点管理页 | ~300 行 |
| 连接监控页 | ~200 行 |
| Go embed 与 SPA handler | ~60 行 |

**总计：约 1700 行 TypeScript + 60 行 Go**\
不含 shadcn/ui 自动生成的组件代码。
