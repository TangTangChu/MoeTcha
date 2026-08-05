# ── 构建阶段 ──
FROM golang:1.25-bookworm AS builder

RUN apt-get update \
    && apt-get install -y --no-install-recommends libwebp-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go mod tidy

# 构建时注入版本信息：BuildDate 取构建时刻；VERSION/GIT_COMMIT 可由
# docker build --build-arg 传入，默认 dev / none（.dockerignore 排除了 .git）。
ARG VERSION=dev
ARG GIT_COMMIT=none
RUN BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) && \
    CGO_ENABLED=1 go build -tags=webp \
      -ldflags "-X moetcha/cli.Version=$VERSION -X moetcha/cli.GitCommit=$GIT_COMMIT -X moetcha/cli.BuildDate=$BUILD_DATE" \
      -o /app/bin/moetcha ./

# ── 运行阶段：仅含二进制与运行期动态库，剥离编译工具链 ──
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends libwebp7 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/bin/moetcha /app/bin/moetcha

EXPOSE 8080

CMD ["/app/bin/moetcha"]
