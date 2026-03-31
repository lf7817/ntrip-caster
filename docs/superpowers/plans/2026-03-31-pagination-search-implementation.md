# 用户列表和挂载点列表分页搜索实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为用户管理页面和挂载点管理页面添加服务端分页和搜索功能

**Architecture:** 后端使用 SQLite LIMIT/OFFSET 分页和 LIKE 搜索，前端使用 TanStack Query 缓存分页数据，搜索点击按钮触发

**Tech Stack:** Go + SQLite (后端), React + TanStack Query + shadcn/ui (前端)

---

## 文件结构

| 文件 | 改动类型 | 职责 |
|------|----------|------|
| `internal/account/service.go` | 修改 | 新增 paginated 查询方法 |
| `internal/api/handlers.go` | 修改 | ListUsers/ListMountpoints 解析参数并返回分页响应 |
| `web/src/api/types.ts` | 修改 | 新增 PaginatedResponse, ListUsersParams, ListMountpointsParams |
| `web/src/api/client.ts` | 修改 | listUsers/listMountpoints 支持查询参数 |
| `web/src/api/hooks.ts` | 修改 | useUsers/useMountpoints 支持参数 |
| `web/src/components/pagination.tsx` | 新建 | 通用分页 UI 组件 |
| `web/src/pages/users.tsx` | 修改 | 添加搜索框、过滤下拉、分页组件 |
| `web/src/pages/mountpoints.tsx` | 修改 | 添加搜索框、过滤下拉、分页组件 |

---

### Task 1: 后端 - account.Service 新增分页方法

**Files:**
- Modify: `internal/account/service.go`

- [ ] **Step 1: 添加 ListUsersPaginated 方法**

在 `ListUsers` 方法后添加：

```go
// ListUsersPaginated returns users with pagination and filtering.
func (s *Service) ListUsersPaginated(page, limit int, search, role string, enabled *bool) ([]*User, int, error) {
	// Build WHERE clause
	where := "WHERE 1=1"
	args := []any{}

	if search != "" {
		where += " AND username LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if role != "" {
		where += " AND role = ?"
		args = append(args, role)
	}
	if enabled != nil {
		where += " AND enabled = ?"
		args = append(args, *enabled)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM users " + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Get paginated rows
	offset := (page - 1) * limit
	args = append(args, limit, offset)
	query := "SELECT id, username, password_hash, role, enabled FROM users " + where + " ORDER BY id LIMIT ? OFFSET ?"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users paginated: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		var en int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &en); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		u.Enabled = en != 0
		users = append(users, u)
	}
	return users, total, rows.Err()
}
```

- [ ] **Step 2: 添加 ListMountPointRowsPaginated 方法**

在 `ListMountPointRows` 方法后添加：

```go
// ListMountPointRowsPaginated returns mountpoints with pagination and filtering.
func (s *Service) ListMountPointRowsPaginated(page, limit int, search, format string, enabled *bool) ([]*MountPointRow, int, error) {
	// Build WHERE clause
	where := "WHERE 1=1"
	args := []any{}

	if search != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if format != "" {
		where += " AND format = ?"
		args = append(args, format)
	}
	if enabled != nil {
		where += " AND enabled = ?"
		args = append(args, *enabled)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM mountpoints " + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mountpoints: %w", err)
	}

	// Get paginated rows
	offset := (page - 1) * limit
	args = append(args, limit, offset)
	query := "SELECT id, name, description, enabled, format, source_auth_mode, write_queue, write_timeout_ms, max_clients FROM mountpoints " + where + " ORDER BY id LIMIT ? OFFSET ?"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list mountpoints paginated: %w", err)
	}
	defer rows.Close()

	var list []*MountPointRow
	for rows.Next() {
		mp := &MountPointRow{}
		var en int
		var wq, wt, mc sql.NullInt64
		if err := rows.Scan(&mp.ID, &mp.Name, &mp.Description, &en, &mp.Format, &mp.SourceAuthMode, &wq, &wt, &mc); err != nil {
			return nil, 0, fmt.Errorf("scan mountpoint: %w", err)
		}
		mp.Enabled = en != 0
		if wq.Valid {
			v := int(wq.Int64)
			mp.WriteQueue = &v
		}
		if wt.Valid {
			v := int(wt.Int64)
			mp.WriteTimeoutMs = &v
		}
		if mc.Valid {
			mp.MaxClients = int(mc.Int64)
		}
		list = append(list, mp)
	}
	return list, total, rows.Err()
}
```

- [ ] **Step 3: 运行测试验证后端编译通过**

```bash
go build ./...
```

Expected: 编译成功，无错误

---

### Task 2: 后端 - handlers.go 改为分页响应

**Files:**
- Modify: `internal/api/handlers.go`

- [ ] **Step 1: 修改 ListUsers handler**

替换 `ListUsers` 方法：

```go
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	page := parseIntParam(r.URL.Query().Get("page"), 1)
	limit := parseIntParam(r.URL.Query().Get("limit"), 50)
	search := r.URL.Query().Get("search")
	role := r.URL.Query().Get("role")

	var enabled *bool
	if e := r.URL.Query().Get("enabled"); e != "" {
		val := e == "true"
		enabled = &val
	}

	users, total, err := h.acctSvc.ListUsersPaginated(page, limit, search, role, enabled)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]any{
		"data":  users,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
```

- [ ] **Step 2: 添加 parseIntParam helper 函数**

在 `parseID` 函数后添加：

```go
func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return defaultVal
	}
	return v
}
```

- [ ] **Step 3: 修改 ListMountpoints handler**

替换 `ListMountpoints` 方法（保留原有的 source_online/client_count 等实时信息逻辑）：

```go
func (h *Handlers) ListMountpoints(w http.ResponseWriter, r *http.Request) {
	page := parseIntParam(r.URL.Query().Get("page"), 1)
	limit := parseIntParam(r.URL.Query().Get("limit"), 50)
	search := r.URL.Query().Get("search")
	format := r.URL.Query().Get("format")

	var enabled *bool
	if e := r.URL.Query().Get("enabled"); e != "" {
		val := e == "true"
		enabled = &val
	}

	rows, total, err := h.acctSvc.ListMountPointRowsPaginated(page, limit, search, format, enabled)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type mpInfo struct {
		account.MountPointRow
		SourceOnline     bool    `json:"source_online"`
		ClientCount      int64   `json:"client_count"`
		AntennaLat       float64 `json:"antenna_lat,omitempty"`
		AntennaLon       float64 `json:"antenna_lon,omitempty"`
		AntennaHeight    float64 `json:"antenna_height,omitempty"`
		AntennaUpdatedAt string  `json:"antenna_updated_at,omitempty"`
	}

	result := make([]mpInfo, 0, len(rows))
	for _, row := range rows {
		info := mpInfo{MountPointRow: *row}
		if mp := h.mgr.Get(row.Name); mp != nil {
			snap := mp.Stats.Snapshot()
			info.SourceOnline = snap.SourceOnline
			info.ClientCount = snap.ClientCount
			antPos := mp.GetAntennaPosition()
			if antPos != nil {
				info.AntennaLat = antPos.Latitude
				info.AntennaLon = antPos.Longitude
				info.AntennaHeight = antPos.Height
				if antPos.UpdatedAt > 0 {
					info.AntennaUpdatedAt = time.Unix(antPos.UpdatedAt, 0).Format(time.RFC3339)
				}
			}
		}
		result = append(result, info)
	}

	jsonOK(w, map[string]any{
		"data":  result,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
```

- [ ] **Step 4: 运行编译验证**

```bash
go build ./...
```

Expected: 编译成功

- [ ] **Step 5: 提交后端改动**

```bash
git add internal/account/service.go internal/api/handlers.go
git commit -m "feat(api): add pagination and search to users and mountpoints list"
```

---

### Task 3: 前端 - types.ts 新增分页相关类型

**Files:**
- Modify: `web/src/api/types.ts`

- [ ] **Step 1: 添加分页响应和参数类型**

在文件末尾添加：

```typescript
// Pagination
export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  limit: number
}

export interface ListUsersParams {
  page?: number
  limit?: number
  search?: string
  role?: string
  enabled?: string
}

export interface ListMountpointsParams {
  page?: number
  limit?: number
  search?: string
  format?: string
  enabled?: string
}
```

---

### Task 4: 前端 - client.ts 支持查询参数

**Files:**
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: 导入新类型**

修改 import 部分：

```typescript
import type {
  User,
  CreateUserReq,
  UpdateUserReq,
  MountpointInfo,
  MountpointRow,
  CreateMountpointReq,
  UpdateMountpointReq,
  Binding,
  CreateBindingReq,
  SourceInfo,
  ClientInfo,
  SystemStats,
  PaginatedResponse,
  ListUsersParams,
  ListMountpointsParams,
} from "./types"
```

- [ ] **Step 2: 修改 listUsers 方法**

替换 `listUsers` 方法：

```typescript
listUsers: (params?: ListUsersParams) => {
  const query = new URLSearchParams()
  if (params?.page) query.set("page", String(params.page))
  if (params?.limit) query.set("limit", String(params.limit))
  if (params?.search) query.set("search", params.search)
  if (params?.role) query.set("role", params.role)
  if (params?.enabled) query.set("enabled", params.enabled)
  const qs = query.toString()
  return request<PaginatedResponse<User>>(`/api/users${qs ? `?${qs}` : ""}`)
},
```

- [ ] **Step 3: 修改 listMountpoints 方法**

替换 `listMountpoints` 方法：

```typescript
listMountpoints: (params?: ListMountpointsParams) => {
  const query = new URLSearchParams()
  if (params?.page) query.set("page", String(params.page))
  if (params?.limit) query.set("limit", String(params.limit))
  if (params?.search) query.set("search", params.search)
  if (params?.format) query.set("format", params.format)
  if (params?.enabled) query.set("enabled", params.enabled)
  const qs = query.toString()
  return request<PaginatedResponse<MountpointInfo>>(`/api/mountpoints${qs ? `?${qs}` : ""}`)
},
```

---

### Task 5: 前端 - hooks.ts 支持参数

**Files:**
- Modify: `web/src/api/hooks.ts`

- [ ] **Step 1: 导入新类型**

修改 import 部分：

```typescript
import type { CreateUserReq, UpdateUserReq, CreateMountpointReq, UpdateMountpointReq, CreateBindingReq, ListUsersParams, ListMountpointsParams } from "./types"
```

- [ ] **Step 2: 修改 queryKeys**

替换 `queryKeys` 定义：

```typescript
export const queryKeys = {
  users: (params?: ListUsersParams) => ["users", params] as const,
  mountpoints: (params?: ListMountpointsParams) => ["mountpoints", params] as const,
  bindings: ["bindings"] as const,
  sources: ["sources"] as const,
  clients: ["clients"] as const,
  stats: ["stats"] as const,
}
```

- [ ] **Step 3: 修改 useUsers hook**

替换 `useUsers` 定义：

```typescript
export function useUsers(params?: ListUsersParams) {
  return useQuery({
    queryKey: queryKeys.users(params),
    queryFn: () => api.listUsers(params),
  })
}
```

- [ ] **Step 4: 修改 useMountpoints hook**

替换 `useMountpoints` 定义：

```typescript
export function useMountpoints(params?: ListMountpointsParams) {
  return useQuery({
    queryKey: queryKeys.mountpoints(params),
    queryFn: () => api.listMountpoints(params),
  })
}
```

---

### Task 6: 前端 - 新建 Pagination 组件

**Files:**
- Create: `web/src/components/pagination.tsx`

- [ ] **Step 1: 创建 Pagination 组件**

```typescript
import { Button } from "@/components/ui/button"

interface PaginationProps {
  page: number
  total: number
  limit: number
  onPageChange: (page: number) => void
}

export function Pagination({ page, total, limit, onPageChange }: PaginationProps) {
  const totalPages = Math.ceil(total / limit)

  if (totalPages <= 1) return null

  // Calculate page range to display (current ± 2)
  const startPage = Math.max(1, page - 2)
  const endPage = Math.min(totalPages, page + 2)

  const pages = []
  for (let i = startPage; i <= endPage; i++) {
    pages.push(i)
  }

  return (
    <div className="flex items-center justify-between py-4">
      <div className="text-sm text-muted-foreground">
        共 {total} 条
      </div>
      <div className="flex items-center gap-1">
        <Button
          variant="outline"
          size="sm"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          上一页
        </Button>
        {startPage > 1 && (
          <>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onPageChange(1)}
            >
              1
            </Button>
            {startPage > 2 && <span className="px-2 text-muted-foreground">...</span>}
          </>
        )}
        {pages.map((p) => (
          <Button
            key={p}
            variant={p === page ? "default" : "ghost"}
            size="sm"
            onClick={() => onPageChange(p)}
          >
            {p}
          </Button>
        ))}
        {endPage < totalPages && (
          <>
            {endPage < totalPages - 1 && <span className="px-2 text-muted-foreground">...</span>}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onPageChange(totalPages)}
            >
              {totalPages}
            </Button>
          </>
        )}
        <Button
          variant="outline"
          size="sm"
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          下一页
        </Button>
      </div>
    </div>
  )
}
```

---

### Task 7: 前端 - users.tsx 添加搜索和分页

**Files:**
- Modify: `web/src/pages/users.tsx`

- [ ] **Step 1: 添加导入和状态**

在文件开头 import 部分添加：

```typescript
import { Search } from "lucide-react"
import { Pagination } from "@/components/pagination"
```

在 `UsersPage` 函数内，现有状态定义后添加：

```typescript
  // Pagination and search state
  const [page, setPage] = useState(1)
  const [searchInput, setSearchInput] = useState("")
  const [search, setSearch] = useState("")
  const [roleFilter, setRoleFilter] = useState("")
  const [enabledFilter, setEnabledFilter] = useState("")
```

- [ ] **Step 2: 修改 useUsers 调用**

替换 `useUsers` 调用：

```typescript
  const { data: usersData, isLoading } = useUsers({
    page,
    limit: 50,
    search,
    role: roleFilter,
    enabled: enabledFilter,
  })
  const users = usersData?.data
  const total = usersData?.total ?? 0
```

- [ ] **Step 3: 添加搜索和过滤处理函数**

在状态定义后添加：

```typescript
  function handleSearch() {
    setSearch(searchInput)
    setPage(1)
  }

  function handleRoleChange(value: string) {
    setRoleFilter(value === "all" ? "" : value)
    setPage(1)
  }

  function handleEnabledChange(value: string) {
    setEnabledFilter(value === "all" ? "" : value)
    setPage(1)
  }

  function handlePageChange(newPage: number) {
    setPage(newPage)
  }
```

- [ ] **Step 4: 添加搜索和过滤 UI**

替换标题和创建按钮部分的 `<div className="flex items-center justify-between">`：

```typescript
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">用户管理</h1>
        <Button onClick={openCreate} size="sm">
          <Plus className="mr-1 h-4 w-4" />
          创建用户
        </Button>
      </div>

      {/* Search and filters */}
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <Input
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="搜索用户名"
            className="w-200"
          />
          <Button onClick={handleSearch} size="sm">
            <Search className="h-4 w-4" />
          </Button>
        </div>
        <Select value={roleFilter || "all"} onValueChange={handleRoleChange}>
          <SelectTrigger className="w-120">
            <SelectValue placeholder="角色" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部角色</SelectItem>
            <SelectItem value="admin">admin</SelectItem>
            <SelectItem value="base">base</SelectItem>
            <SelectItem value="rover">rover</SelectItem>
          </SelectContent>
        </Select>
        <Select value={enabledFilter || "all"} onValueChange={handleEnabledChange}>
          <SelectTrigger className="w-120">
            <SelectValue placeholder="状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            <SelectItem value="true">启用</SelectItem>
            <SelectItem value="false">禁用</SelectItem>
          </SelectContent>
        </Select>
      </div>
```

- [ ] **Step 5: 添加分页组件**

在表格 `<div className="rounded-md border">` 后面添加：

```typescript
      {usersData && (
        <Pagination
          page={page}
          total={total}
          limit={50}
          onPageChange={handlePageChange}
        />
      )}
```

- [ ] **Step 6: 调整表格数据渲染**

表格中 `users?.length` 检查改为：

```typescript
              {!users?.length ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground py-8">
                    暂无用户
                  </TableCell>
                </TableRow>
              ) : (
                users.map((user) => (
```

---

### Task 8: 前端 - mountpoints.tsx 添加搜索和分页

**Files:**
- Modify: `web/src/pages/mountpoints.tsx`

- [ ] **Step 1: 添加导入和状态**

在文件开头 import 部分添加：

```typescript
import { Search } from "lucide-react"
import { Pagination } from "@/components/pagination"
```

在 `MountpointsPage` 函数内，现有状态定义后添加：

```typescript
  // Pagination and search state
  const [page, setPage] = useState(1)
  const [searchInput, setSearchInput] = useState("")
  const [search, setSearch] = useState("")
  const [formatFilter, setFormatFilter] = useState("")
  const [enabledFilter, setEnabledFilter] = useState("")
```

- [ ] **Step 2: 修改 useMountpoints 调用**

替换 `useMountpoints` 调用：

```typescript
  const { data: mountsData, isLoading } = useMountpoints({
    page,
    limit: 50,
    search,
    format: formatFilter,
    enabled: enabledFilter,
  })
  const mounts = mountsData?.data
  const total = mountsData?.total ?? 0
```

- [ ] **Step 3: 添加搜索和过滤处理函数**

在状态定义后添加：

```typescript
  function handleSearch() {
    setSearch(searchInput)
    setPage(1)
  }

  function handleFormatChange(value: string) {
    setFormatFilter(value === "all" ? "" : value)
    setPage(1)
  }

  function handleEnabledChange(value: string) {
    setEnabledFilter(value === "all" ? "" : value)
    setPage(1)
  }

  function handlePageChange(newPage: number) {
    setPage(newPage)
  }
```

- [ ] **Step 4: 添加搜索和过滤 UI**

替换标题和创建按钮部分的 `<div className="flex items-center justify-between">`：

```typescript
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">挂载点管理</h1>
        <Button onClick={openCreate} size="sm">
          <Plus className="mr-1 h-4 w-4" />
          创建挂载点
        </Button>
      </div>

      {/* Search and filters */}
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <Input
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="搜索名称"
            className="w-200"
          />
          <Button onClick={handleSearch} size="sm">
            <Search className="h-4 w-4" />
          </Button>
        </div>
        <Select value={formatFilter || "all"} onValueChange={handleFormatChange}>
          <SelectTrigger className="w-120">
            <SelectValue placeholder="格式" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部格式</SelectItem>
            <SelectItem value="RTCM3">RTCM3</SelectItem>
            <SelectItem value="RTCM 3.2">RTCM 3.2</SelectItem>
            <SelectItem value="RTCM 3.3">RTCM 3.3</SelectItem>
          </SelectContent>
        </Select>
        <Select value={enabledFilter || "all"} onValueChange={handleEnabledChange}>
          <SelectTrigger className="w-120">
            <SelectValue placeholder="状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            <SelectItem value="true">启用</SelectItem>
            <SelectItem value="false">禁用</SelectItem>
          </SelectContent>
        </Select>
      </div>
```

- [ ] **Step 5: 添加分页组件**

在表格 `<div className="rounded-md border">` 后面添加：

```typescript
      {mountsData && (
        <Pagination
          page={page}
          total={total}
          limit={50}
          onPageChange={handlePageChange}
        />
      )}
```

- [ ] **Step 6: 调整表格数据渲染**

表格中 `mounts?.length` 检查改为：

```typescript
              {!mounts?.length ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-muted-foreground py-8">
                    暂无挂载点
                  </TableCell>
                </TableRow>
              ) : (
                mounts.map((mp) => (
```

---

### Task 9: 验证和提交

- [ ] **Step 1: 运行前端类型检查**

```bash
cd web && npm run typecheck
```

Expected: 无类型错误

- [ ] **Step 2: 运行前端构建**

```bash
cd web && npm run build
```

Expected: 构建成功

- [ ] **Step 3: 运行后端编译**

```bash
go build ./...
```

Expected: 编译成功

- [ ] **Step 4: 提交前端改动**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/hooks.ts web/src/components/pagination.tsx web/src/pages/users.tsx web/src/pages/mountpoints.tsx
git commit -m "feat(web): add pagination and search to users and mountpoints pages"
```

- [ ] **Step 5: 手动测试**

启动服务：
```bash
make dev-backend
make dev-frontend
```

访问 http://localhost:5173 验证：
- 用户列表：搜索、角色过滤、状态过滤、分页功能正常
- 挂载点列表：搜索、格式过滤、状态过滤、分页功能正常