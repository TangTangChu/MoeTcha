# API

## 端口

8080

## 说明

服务通过 HTTP 提供验证码相关接口。图片资源返回二进制内容，内容类型随编码变化。启用 WebP 构建时为 image/webp，未启用时为 image/png。接口响应均为 JSON。

## GET /challenge

用途：获取验证码题目。可选查询参数 type，值为 grid 或 click，不传则随机。

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
  }
}
```

## POST /verify

用途：提交答案进行校验。请求体包含 session_id，type 以及 grid 或 click 对象。grid 提交 image_ids 数组，click 提交 points 数组，points 内含 x 与 y。

示例请求 grid

```bash
curl -X POST "http://localhost:8080/verify" -H "Content-Type: application/json" -d '{"session_id":"2f0c8e1b9a9c4e1f1a3b2c4d5e6f7788","type":"grid","grid":{"image_ids":["animals:cat_01","animals:cat_03"]}}'
```

示例请求 click

```bash
curl -X POST "http://localhost:8080/verify" -H "Content-Type: application/json" -d '{"session_id":"2f0c8e1b9a9c4e1f1a3b2c4d5e6f7788","type":"click","click":{"points":[{"x":120,"y":220},{"x":300,"y":260}]}}'
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

用途：根据 asset_key 获取图片内容。返回内容类型根据编码而定，启用 WebP 时为 image/webp，否则为 image/png。

示例请求

```bash
curl "http://localhost:8080/asset/a1b2c3d4" --output image.webp
```

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
