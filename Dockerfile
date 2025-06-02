# 使用官方 Golang 镜像作为构建环境
FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -o augment2api

# 使用轻量级的 alpine 镜像
FROM alpine:latest

# 安装 ca-certificates 以支持 HTTPS
RUN apk --no-cache add ca-certificates tzdata

# 创建非 root 用户
RUN adduser -D -g '' appuser

# 从构建阶段复制二进制文件
COPY --from=builder /app/augment2api /app/augment2api

# 复制静态文件和模板
COPY --from=builder /app/templates /app/templates

# 设置工作目录
WORKDIR /app

# 使用非 root 用户运行
USER appuser

# 暴露端口
EXPOSE 27080


# 运行应用
CMD ["/app/augment2api"] 