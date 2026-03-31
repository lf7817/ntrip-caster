# 用户列表和挂载点列表分页搜索设计

## 目标

为用户管理页面和挂载点管理页面添加分页和搜索功能，改善大量数据时的使用体验。

## API 设计

### 用户列表

**请求**
```
GET /api/users?page=1&limit=50&search=xxx&role=xxx&enabled=xxx
```

**参数**
| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| page | int | 1 | 页码，从 1 开始 |
| limit | int | 50 | 每页条数 |
| search | string | - | 用户名模糊搜索 |
| role | string | - | 角色过滤 (admin/base/rover) |
| enabled | string | - | 状态过滤 (true/false) |

**响应**
```json
{
  "data": [...],
  "total": 100,
  "page": 1,
  "limit": 50
}
```

### 挂载点列表

**请求**
```
GET /api/mountpoints?page=1&limit=50&search=xxx&format=xxx&enabled=xxx
```

**参数**
| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| page | int | 1 | 页码，从 1 开始 |
| limit | int | 50 | 每页条数 |
| search | string | - | 名称模糊搜索 |
| format | string | - | 格式过滤 |
| enabled | string | - | 状态过滤 (true/false) |

**响应**
```json
{
  "data": [...],
  "total": 50,
  "page": 1,
  "limit": 50
}
```

## 后端实现

### account.Service 改动

新增方法：
- `ListUsersPaginated(page, limit int, search, role string, enabled *bool) ([]*User, int, error)`
- `ListMountPointRowsPaginated(page, limit int, search, format string, enabled *bool) ([]*MountPointRow, int, error)`

SQL 查询模式：
```sql
SELECT ... FROM users
WHERE username LIKE '%search%'
  AND (? = '' OR role = ?)
  AND (? IS NULL OR enabled = ?)
ORDER BY id
LIMIT ? OFFSET ?

SELECT COUNT(*) FROM users WHERE ... -- 同上条件
```

### handlers.go 改动

- `ListUsers`: 解析查询参数，调用 paginated 方法
- `ListMountpoints`: 解析查询参数，调用 paginated 方法

## 前端实现

### 搜索交互

- 搜索框 + 搜索按钮（点击触发搜索）
- 过滤下拉框（角色/格式/状态）即时生效
- 搜索或过滤变更后跳转回第一页

### 分页组件

- 显示：上一页 | 页码 | 下一页
- 页码范围：当前页前后各 2 页
- 总数显示：显示 total 条数

### TanStack Query

```typescript
// queryKey 包含所有参数，自动缓存不同组合
useUsers({ page, limit, search, role, enabled })
useMountpoints({ page, limit, search, format, enabled })
```

### 组件拆分

- `Pagination` 组件：通用分页 UI
- `SearchInput` 组件：搜索框 + 按钮
- 页面组件组合使用

## 文件改动清单

| 文件 | 改动 |
|------|------|
| `internal/account/service.go` | 新增 paginated 方法 |
| `internal/api/handlers.go` | ListUsers/ListMountpoints 改为分页响应 |
| `web/src/api/client.ts` | listUsers/listMountpoints 支持参数 |
| `web/src/api/hooks.ts` | useUsers/useMountpoints 支持参数 |
| `web/src/api/types.ts` | 新增 PaginatedResponse 类型 |
| `web/src/components/pagination.tsx` | 新建分页组件 |
| `web/src/pages/users.tsx` | 添加搜索和分页 UI |
| `web/src/pages/mountpoints.tsx` | 添加搜索和分页 UI |