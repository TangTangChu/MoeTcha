# API

## 端口

8080

## 说明

服务通过 HTTP 提供验证码相关接口。图片资源返回二进制内容，内容类型按实际图片格式返回；普通验证码接口可随构建配置返回 PNG/WebP，`POST /grid/generate` 固定返回 WebP。接口响应均为 JSON。

## GET /challenge

用途：获取验证码题目。可选查询参数 type，值为 grid 或 click，不传则随机。
注意：当启用 IP/UA 绑定策略或 Token 签名策略时，需要客户端请求带上稳定的 IP 与 User-Agent。

示例请求

```bash
curl "http://localhost:8080/challenge"
```

示例响应

```json
{
  "session_id": "2f0c8e1b9a9c4e1f1a3b2c4d5e6f7788",
  "type": "grid",
  "question": "请选择所有包含【猫】的图片",
  "grid": {
    "images": [
      {"image_id": "animals:cat_01", "asset_key": "a1b2c3d4"},
      {"image_id": "animals:dog_02", "asset_key": "e5f6g7h8"}
    ]
  },
  "token": "<signed-token>"
}
```

## POST /verify

用途：提交答案进行校验。请求体包含 session_id、token、type 以及 grid 或 click 对象。grid 提交 image_ids 数组，click 提交 points 数组，points 内含 x 与 y。
注意：当启用 IP/UA 绑定策略或 Token 签名策略时，需要与获取 challenge 时的 IP 与 User-Agent 一致，否则会被拒绝；Token 签名开启后需提交 token 字段。

示例请求 grid

```bash
curl -X POST "http://localhost:8080/verify" -H "Content-Type: application/json" -d '{"session_id":"2f0c8e1b9a9c4e1f1a3b2c4d5e6f7788","token":"<signed-token>","type":"grid","grid":{"image_ids":["animals:cat_01","animals:cat_03"]}}'
```

示例请求 click

```bash
curl -X POST "http://localhost:8080/verify" -H "Content-Type: application/json" -d '{"session_id":"2f0c8e1b9a9c4e1f1a3b2c4d5e6f7788","token":"<signed-token>","type":"click","click":{"points":[{"x":120,"y":220},{"x":300,"y":260}]}}'
```

示例响应

```json
{
  "ok": true,
  "correct": 2,
  "total": 2
}
```

## GET /asset/:key

用途：根据 asset_key 获取图片内容。返回内容类型按 asset 实际编码判断。

示例请求

```bash
curl "http://localhost:8080/asset/a1b2c3d4" --output image.webp
```

## POST /grid/generate

用途：从已加载的 Grid 素材中合成一张带编号的 WebP 网格图。生成结果会写入临时 asset，默认使用服务的 `CAPTCHA_TTL` 过期；响应中的 `asset_url` 和 `temporary_file_url` 都是临时访问地址，不暴露服务器本地文件系统路径。

鉴权：当配置了 `API_TOKENS` 时，此接口需要 `Authorization: Bearer <token>` 或 `X-API-Token: <token>`；未配置时该接口开放（仅适合内网/本地开发，生产环境应通过 `API_TOKENS` 或反代鉴权保护）。

请求体为 JSON。最小示例：

```json
{
  "tag": "猫",
  "image_count": 9,
  "correct_count": 3
}
```

常用参数：

- `tag`：目标标签。正确图片必须包含该标签；不传时随机选择一个 Grid 标签。
- `image_count` 或 `size`：图片数量，默认使用目标标签所在 pack 的 Grid 配置。
- `correct_count`：正确图片数量；不传时使用 pack 的 `correct_min/correct_max` 随机选择。
- `image_ids`：可选，显式指定全部图片的 ID（支持 `pack_id:image_id` 或唯一的裸 ID）。
- `correct_image_ids`：可选，显式指定正确图片；与 `image_ids` 一起使用时必须是其子集。
- `correct_numbers`：可选，与 `image_ids` 一起使用，按输入顺序指定 1-based 正确编号。
- `distractor_image_ids`：可选，固定部分干扰图片；其余干扰图由后端按 `difficulty` 补齐。
- `rows`、`columns`（或 `cols`）：布局行列；不传时自动选择接近正方形的布局。
- `tile_width`、`tile_height`、`gap`、`padding`：输出尺寸参数，默认 tile 为 `160x160`。
- `fit`：`cover`、`contain` 或 `stretch`，默认 `cover`。
- `show_labels`、`label_scale`、`label_position`：是否绘制编号以及编号样式；编号从 1 开始。
- `background`、`label_color`、`label_background`：支持 `#RGB`、`#RGBA`、`#RRGGBB`、`#RRGGBBAA` 和少量颜色名。
- `shuffle`：是否在合成前打乱图片顺序，默认 `true`；显式编号场景通常设为 `false`。
- `quality`：WebP 质量，范围 1~100，默认 80。
- `difficulty`：`easy`、`medium` 或 `hard`。
- `seed`：可选随机种子，便于复现同一选图结果。
- `apply_renderer`：是否应用服务配置的渲染/干扰管线，默认 `true`。
- `temporary_ttl_seconds`：临时 asset 生命周期，0 表示使用默认 TTL，最大 86400 秒。

显式指定图片和正确编号的示例：

```json
{
  "tag": "猫",
  "image_ids": [
    "animals:cat_01",
    "animals:dog_01",
    "animals:cat_02",
    "animals:bird_01"
  ],
  "correct_numbers": [1, 3],
  "rows": 2,
  "columns": 2,
  "shuffle": false,
  "tile_width": 180,
  "tile_height": 180,
  "gap": 8,
  "padding": 8,
  "label_position": "top_left"
}
```

响应示例（`correct_numbers` 和每个 tile 的 `number` 均为 1-based）：

```json
{
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
  "width": 376,
  "height": 376,
  "rows": 2,
  "columns": 2,
  "tile_width": 180,
  "tile_height": 180,
  "gap": 8,
  "padding": 8,
  "tiles": [
    {
      "number": 1,
      "image_id": "animals:cat_01",
      "file": "cat_01.webp",
      "tags": ["猫"],
      "correct": true,
      "x": 8,
      "y": 8,
      "width": 180,
      "height": 180
    }
  ]
}
```

该接口会返回答案元数据，适合编辑器、素材预览和内部生成工具使用；如果暴露给最终验证码客户端，应在网关或鉴权层限制访问，否则客户端可以直接读取正确答案。

错误响应统一格式（4xx/5xx）：

```json
{
  "error": "可读错误信息",
  "code": "机器可读的错误码"
}
```

常见 `code`：

- `BAD_REQUEST`：请求参数不合法（400）
- `RATE_LIMITED`：触发限流（429）
- `UNAUTHORIZED`：缺少或无效 API Token（401）
- `INTERNAL`：服务内部错误（500，不回显内部细节，详见日志）

## /healthz

用途：健康检查。允许任何 HTTP 方法，返回 JSON，包含状态码、固定文本与请求方法。

示例请求

```bash
curl -X POST "http://localhost:8080/healthz"
```

示例响应

```json
{
  "status": 200,
  "text": "Ciallo～(∠・ω< )⌒★",
  "method": "POST"
}
```
