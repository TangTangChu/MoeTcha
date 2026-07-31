# API

## 端口

8080

## 统一响应格式

所有 JSON 端点（`/challenge`、`/verify`、`/grid/generate`、`/asset` 的错误、`/healthz`、`/metrics`）都返回同一个信封：

```json
{
  "ok": true,
  "data": { ... },
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

失败时：

```json
{
  "ok": false,
  "error": {
    "code": "MACHINE_CODE",
    "message": "可读的中文信息"
  },
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

客户端只需判断 `ok`：`true` 读 `data`，`false` 读 `error`。`request_id` 与响应头 `X-Request-ID` 一致，便于排查。

> `/asset/:key` 成功时返回图片二进制（非 JSON），只有失败时才走上面的错误信封。

## 跨域与安全

- **CORS**：默认开启（`CORS_ENABLED=true`），`CORS_ALLOWED_ORIGINS=*` 放行任意来源。生产环境请改为具体域名。预检 `OPTIONS` 直接返回 204。
- **安全响应头**：所有响应带 `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: no-referrer`、`Cache-Control: no-store`。
- **内部错误不回显**：5xx 只返回 `{code:"INTERNAL", message:"内部错误"}`，细节进日志。

## GET /challenge

获取验证码题目。可选查询参数 `type`，值为 `grid` 或 `click`，不传则随机。

```bash
curl "http://localhost:8080/challenge"
```

成功响应（`data` 内）：

```json
{
  "ok": true,
  "data": {
    "session_id": "2f0c8e1b9a9c4e1f1a3b2c4d5e6f7788",
    "type": "grid",
    "question": "请选择所有包含【猫】的图片",
    "expires_at": "2026-08-01T12:02:00Z",
    "grid": {
      "images": [
        {"image_id": "animals:cat_01", "asset_key": "a1b2c3d4"}
      ]
    },
    "token": "<signed-token>"
  },
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

`expires_at` 是会话过期时间（RFC3339），客户端可据此做倒计时或自动刷新。

## POST /verify

提交答案校验。**无论答对答错都返回 HTTP 200**，客户端读 `data.solved` 判断，不要靠 HTTP 状态码。只有请求级失败（会话过期、限流、绑定校验不过等）才返回 4xx。

请求体：`session_id`、`token`、`type` 以及 `grid` 或 `click` 对象。`grid` 提交 `image_ids` 数组，`click` 提交 `points` 数组（含 `x`、`y`）。

```bash
curl -X POST "http://localhost:8080/verify" -H "Content-Type: application/json" \
  -d '{"session_id":"...","token":"<signed-token>","type":"grid","grid":{"image_ids":["animals:cat_01"]}}'
```

答对：

```json
{
  "ok": true,
  "data": {"solved": true, "correct": 2, "total": 2},
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

答错（仍是 200，`data.solved=false`，`data.code` 给出原因）：

```json
{
  "ok": true,
  "data": {"solved": false, "code": "WRONG_COUNT", "reason": "数量不匹配", "correct": 1, "total": 2},
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

会话过期等请求级失败（4xx）：

```json
{
  "ok": false,
  "error": {"code": "SESSION_EXPIRED", "message": "会话不存在或已过期"},
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

Click 挑战：题目返回的 `click.required` 表示需点击的目标数量，`question` 用 `{count}` 占位符提示。点击数必须正好等于 `required`，每个点需落在不同目标区域内。`required` 为 0 表示需点击全部匹配区域。

## GET /asset/:key

按 `asset_key` 获取图片内容。成功返回二进制，内容类型按实际编码判断。

```bash
curl "http://localhost:8080/asset/a1b2c3d4" --output image.webp
```

失败返回统一错误信封（如 404 `NOT_FOUND`）。

## POST /grid/generate

从已加载 Grid 素材合成一张带编号的 WebP 网格图，写入临时 asset。

鉴权：配置了 `API_TOKENS` 时需 `Authorization: Bearer <token>` 或 `X-API-Token: <token>`；未配置时开放（仅适合内网/本地开发）。

最小请求体：

```json
{"tag": "猫", "image_count": 9, "correct_count": 3}
```

常用参数：

- `tag`：目标标签。正确图片必须包含该标签；不传则随机选一个 Grid 标签。
- `image_count` 或 `size`：图片数量，默认用 pack 配置。
- `correct_count`：正确图片数量；不传用 `correct_min/correct_max` 随机。
- `image_ids`：可选，显式指定全部图片 ID（`pack_id:image_id` 或裸 ID）。
- `correct_image_ids`：可选，显式指定正确图片，须为 `image_ids` 子集。
- `correct_numbers`：可选，配合 `image_ids`，按顺序指定 1-based 正确编号。
- `distractor_image_ids`：可选，固定部分干扰图，其余按 `difficulty` 补齐。
- `rows`、`columns`（或 `cols`）：布局行列；不传自动选接近正方形的布局。
- `tile_width`、`tile_height`、`gap`、`padding`：尺寸参数，默认 tile `160x160`。
- `fit`：`cover`/`contain`/`stretch`，默认 `cover`。
- `show_labels`、`label_scale`、`label_position`：编号绘制，编号从 1 开始。
- `background`、`label_color`、`label_background`：支持 `#RGB`/`#RGBA`/`#RRGGBB`/`#RRGGBBAA` 及少量颜色名。
- `shuffle`：合成前是否打乱，默认 `true`；显式编号场景通常设 `false`。
- `quality`：WebP 质量 1~100，默认 80。
- `difficulty`：`easy`/`medium`/`hard`。
- `seed`：随机种子，便于复现。
- `apply_renderer`：是否应用渲染/干扰管线，默认 `true`。
- `temporary_ttl_seconds`：临时 asset 生命周期，0 用默认 TTL，最大 86400。

成功响应（`data` 内，`correct_numbers` 与每个 tile 的 `number` 均为 1-based）：

```json
{
  "ok": true,
  "data": {
    "id": "f4d1...",
    "asset_key": "a1b2c3d4",
    "asset_url": "http://localhost:8080/asset/a1b2c3d4",
    "temporary_file_url": "http://localhost:8080/asset/a1b2c3d4",
    "content_type": "image/webp",
    "expires_at": "2026-07-28T12:00:00Z",
    "tag": "猫",
    "tag_display": "猫",
    "question": "请选出所有「猫」",
    "image_count": 4,
    "correct_count": 2,
    "correct_numbers": [1, 3],
    "correct_image_ids": ["animals:cat_01", "animals:cat_02"],
    "width": 376, "height": 376,
    "rows": 2, "columns": 2,
    "tile_width": 180, "tile_height": 180, "gap": 8, "padding": 8,
    "tiles": [
      {"number": 1, "image_id": "animals:cat_01", "file": "cat_01.webp", "tags": ["猫"], "correct": true, "x": 8, "y": 8, "width": 180, "height": 180}
    ]
  },
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

> 该接口返回答案元数据，适合编辑器/内部工具。暴露给最终验证码客户端时应在网关或鉴权层限制访问。

## GET /metrics

返回累计计数快照（`data` 内）：

```json
{
  "ok": true,
  "data": {
    "challenges_generated": 12,
    "grid_images_generated": 3,
    "verifications_ok": 8,
    "verifications_fail": 2,
    "assets_served": 20
  },
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

## /healthz

健康检查，允许任何 HTTP 方法。

```bash
curl -X POST "http://localhost:8080/healthz"
```

```json
{
  "ok": true,
  "data": {"status": 200, "text": "Ciallo～(∠・ω< )⌒★", "method": "POST"},
  "request_id": "a1b2c3d4",
  "timestamp": "2026-08-01T12:00:00Z"
}
```

## 错误码

所有 4xx/5xx 响应都走统一信封的 `error`。`code` 供程序化处理/i18n，`message` 供人阅读。

通用：

| code | HTTP | 含义 |
|---|---|---|
| `BAD_REQUEST` | 400 | 请求参数不合法 |
| `UNAUTHORIZED` | 401 | 缺少或无效 API Token |
| `FORBIDDEN` | 403 | IP/UA 绑定校验不过 |
| `NOT_FOUND` | 404 | 资源不存在（含未知路由） |
| `METHOD_NOT_ALLOWED` | 405 | 不支持的请求方法 |
| `RATE_LIMITED` | 429 | 触发限流 |
| `PAYLOAD_TOO_LARGE` | 413 | 请求体超限 |
| `INTERNAL` | 500 | 内部错误（不回显细节） |
| `SERVICE_UNINITIALIZED` | 500 | 服务组件未初始化 |

`/verify` 请求级失败码：

| code | HTTP | 含义 |
|---|---|---|
| `EMPTY_SESSION` | 400 | session_id 为空 |
| `SESSION_EXPIRED` | 410 | 会话不存在或已过期 |
| `TOO_FAST` | 400 | 验证过快 |
| `MISSING_UA` | 403 | 缺少 User-Agent |
| `UA_MISMATCH` | 403 | User-Agent 不匹配 |
| `IP_MISMATCH` | 403 | IP 不匹配 |
| `TOO_MANY_ATTEMPTS` | 400 | IP 尝试次数过多 |
| `HIGH_FAIL_RATIO` | 400 | 失败率过高 |
| `TOKEN_INVALID` | 401 | Token 无效或已过期 |
| `MISSING_GRID` | 400 | 缺少 grid 请求体 |
| `MISSING_CLICK` | 400 | 缺少 click 请求体 |
| `CHALLENGE_MISSING` | 400 | challenge 数据缺失 |
| `UNKNOWN_TYPE` | 400 | 未知 challenge 类型 |

`/verify` 验证结果码（`data.solved=false` 时 `data.code`，HTTP 始终 200）：

| code | 含义 |
|---|---|
| `TYPE_MISMATCH` | challenge 类型不匹配 |
| `NO_SELECTION` | 未选择图片 |
| `EMPTY_IMAGE_ID` | 存在空图片 ID |
| `WRONG_COUNT` | 数量不匹配 |
| `WRONG_SELECTION` | 包含错误选项 |
| `NO_REGIONS` | 挑战缺少区域 |
| `NO_POINTS` | 未提供点击点 |
| `POINT_OUT_OF_REGION` | 点击点不在目标区域 |
| `WRONG_CLICK_COUNT` | 点击数量不匹配 |
| `DUPLICATE_REGION` | 重复点击同一区域 |
