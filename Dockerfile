FROM golang:1.25-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends libwebp-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go mod tidy
RUN go build -tags=webp -o /app/bin/moetcha ./

EXPOSE 8080

CMD ["/app/bin/moetcha"]
