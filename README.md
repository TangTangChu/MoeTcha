# 萌茶

MoeTcha，可以叫做萌茶或者萌查，一个基于个人兴趣爱好而捣鼓的简单的验证码服务

API文档是[这个](API.md)

普通验证码接口的图片编码仍遵循当前构建配置；`POST /grid/generate` 使用纯 Go WebP 编码路径，Windows 下也会返回真正的 WebP 图片。

不过也可以使用Docker compose

## 命令行

```
moetcha [serve] [选项]        启动服务（默认命令，裸跑等价于 serve）
moetcha config init           生成 .env（交互向导，或 --preset dev|prod 非交互）
moetcha config show           查看生效配置及其来源
moetcha config get <KEY>      查看单个配置项（密钥默认脱敏，加 --show-secrets）
moetcha config set <KEY=V>    写入单个配置项到 .env
moetcha config validate       校验配置，有错以非零码退出
moetcha config template       输出完整 .env 模板
moetcha gen-key               生成随机密钥（--format env --name KEY 直接出环境变量行）
moetcha version               显示版本与构建信息
```

上手：

```bash
moetcha config init        # 交互式生成 .env，密钥自动随机生成
moetcha config validate    # 确认没写错
moetcha                    # 启动
```

### 配置优先级

命令行 > 真实环境变量 > `.env` 文件 > 默认值。

真实环境变量压过 `.env` 是刻意的：容器里 `docker run -e` 传入的生产密钥不该被镜像内的开发配置盖掉。`.env` 只是开发便利文件，已在 `.gitignore` 与 `.dockerignore` 中排除。

`serve` 默认读取当前目录的 `.env`，不存在则忽略；用 `--env-file` 显式指定时文件必须存在，否则报错。

### 排查配置

`config show` 会列出每一项的生效值和它到底来自哪里，密钥默认脱敏（需要明文加 `--show-secrets`）：

```
$ moetcha config show

# 存储
变量                            值                 来源
STORAGE_BACKEND                 sqlite             .env 文件
SQLITE_PATH                     ./data/moetcha.db  默认值
```

配置写错时不会再静默回落默认值，而是一次性列出全部问题：

```
$ moetcha config validate
配置加载失败（2 项）：
  - CAPTCHA_TTL 必须为时长格式（如 90s、2m、1h30m），当前=2min（来源：.env 文件）
  - CAPTCHA_MAX_ATTEMPTS 必须为整数，当前=three（来源：环境变量）
```

`config show` 则是宽松模式：即使有错也会完整渲染，出错项标注 ✗ 并显示当前实际使用的默认值，便于对照。

### 临时覆盖

```bash
moetcha serve --port 9000
moetcha serve --set CAPTCHA_DIFFICULTY=hard --set CAPTCHA_TTL=30s
```

`--set` 的键名会对照配置表校验，打错会直接报错而不是静默忽略。

### 关于 .env.example

`.env.example` 由 `moetcha config template` 生成，请勿手工增删条目——配置表是唯一事实来源，有测试保证两者不会脱节。新增配置项时改 `core/config_spec.go`，然后：

```bash
moetcha config template --output .env.example
```

### 启动与关闭

`moetcha`（或 `moetcha serve`）启动后会打印就绪信息与监听地址，按 `Ctrl+C` 优雅关闭：先停止接收新连接并排空在途请求，再关闭 SQLite（落盘 checkpoint），不会丢数据。终端着色仅在交互式终端生效，重定向到文件/管道时自动退回纯文本。

### 构建与版本信息

`moetcha version` 会打印版本号、git 提交、构建时间、go 版本与目标平台。前三项由构建时 `-ldflags` 注入，不注入时分别回落 `dev` / `none` / `unknown`：

```bash
go build -tags=webp \
  -ldflags "-X moetcha/cli.Version=$(git describe --tags 2>/dev/null || echo dev) \
            -X moetcha/cli.GitCommit=$(git rev-parse --short HEAD 2>/dev/null || echo none) \
            -X moetcha/cli.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/moetcha ./
```

Docker 构建可用 `--build-arg VERSION=... --build-arg GIT_COMMIT=...` 覆盖（默认 `dev` / `none`，`BuildDate` 自动取构建时刻）。
