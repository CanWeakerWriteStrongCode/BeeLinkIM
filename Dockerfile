# 第一阶段：构建
FROM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装必要的工具 (git, ca-certificates, tzdata)
RUN apk add --no-cache git ca-certificates tzdata

# 复制 go mod 和 sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 编译 (静态链接，减小体积)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o chat-server ./cmd/chat-server

# 第二阶段：运行
FROM alpine:latest

# 安装时区数据
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai

# 设置工作目录
WORKDIR /root/

# 从 builder 阶段复制编译好的二进制文件
COPY --from=builder /app/chat-server .
# 复制配置文件 (注意：生产环境建议通过 ConfigMap 或 Volume 挂载，不要打包进镜像)
COPY --from=builder /app/configs/config.yaml ./configs/

# 暴露端口
EXPOSE 8080

# 启动命令
CMD ["./chat-server"]