<div align="center">

# BeeLinkIM

**基于 Go 的分布式实时聊天服务器**

[![Go Version](https://img.shields.io/badge/Go-1.25.0-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Gin](https://img.shields.io/badge/Gin-v1.12-0095D5?style=flat)](https://github.com/gin-gonic/gin)
[![WebSocket](https://img.shields.io/badge/WebSocket-gorilla-FF6F00?style=flat)](https://github.com/gorilla/websocket)

</div>

---

## 📖 项目简介

BeeLinkIM 是一个使用 **Go 语言** 编写的分布式实时聊天服务器，支持水平扩展。它提供 WebSocket 长连接通信，具备一对一私聊、消息持久化、集群消息路由等核心能力，可作为即时通讯（IM）系统的后端服务。

## ✨ 功能特性

- 💬 **一对一实时聊天** —— 基于 WebSocket 长连接，毫秒级消息推送
- 🔐 **JWT 认证鉴权** —— HS256 签名，支持 Header 和 Query 两种传参方式
- 🌐 **分布式集群支持** —— 多节点部署，通过 Redis + RocketMQ 实现跨服务器消息路由
- 📦 **消息持久化** —— MySQL 存储聊天记录，GORM 自动建表
- ⚡ **Redis 缓存加速** —— 房间会话缓存、消息序列号、分布式锁
- 🔄 **消息有序性保证** —— Redis 原子自增序列号，消息去重与排序
- ❤️ **心跳保活机制** —— Ping/Pong 心跳检测，自动清理死连接
- 🛡️ **优雅关闭** —— 监听系统信号，等待现有请求处理完毕后安全退出

## 🛠️ 技术栈

| 层级 | 技术 |
|------|------|
| 语言 | Go 1.25 |
| HTTP 框架 | [Gin](https://github.com/gin-gonic/gin) |
| WebSocket | [gorilla/websocket](https://github.com/gorilla/websocket) |
| ORM | [GORM](https://gorm.io/) + MySQL 8.0 |
| 缓存 / 锁 | [go-redis](https://github.com/redis/go-redis) + [redislock](https://github.com/bsm/redislock) |
| 消息队列 | [Apache RocketMQ](https://rocketmq.apache.org/) |
| 配置管理 | [Viper](https://github.com/spf13/viper) |
| 日志 | [Zap](https://github.com/uber-go/zap) + [lumberjack](https://github.com/natefinch/lumberjack) |
| 认证 | [golang-jwt](https://github.com/golang-jwt/jwt) |
| 容器化 | Docker + Docker Compose |

## 🏗️ 架构设计

```
┌─────────────────────────────────────────────┐
│                 客户端 (WebSocket / HTTP)      │
└───────────┬─────────────────────────────────┘
            │
      ┌─────▼──────┐
      │   Gin 路由   │  ← CORS / JWT鉴权 / 在线统计
      └─────┬──────┘
            │
      ┌─────▼──────┐
      │  业务服务层  │  ← 聊天核心逻辑、消息分发
      └─────┬──────┘
            │
   ┌────────┼────────┐
   │        │        │
┌──▼──┐ ┌──▼──┐ ┌───▼───┐
│MySQL│ │Redis│ │RocketMQ│
└─────┘ └─────┘ └───────┘
```

### 消息发送流程

```
发送者 ──WebSocket──▶ 服务端
                        │
                  ① 获取/创建房间会话 (Redis/MySQL)
                  ② 原子自增序列号 (Redis HIncrBy)
                  ③ 异步持久化消息 (MySQL)
                  ④ 路由分发：
                        │
            本地用户？───┼─── 跨服务器？
                         │        │
                    Hub直接推送  RocketMQ 发布
                                     │
                               目标服务器消费
                                     │
                               Hub推送给本地用户
```

### 项目目录结构

```
BeeLinkIM/
├── cmd/
│   └── chat-server/main.go    # 服务入口，依赖注入与启动编排
├── configs/
│   └── config.yaml             # 应用配置文件
├── internal/
│   ├── api/                    # WebSocket 协议升级器
│   ├── dto/                    # 统一响应结构体
│   ├── handler/                # HTTP 处理器（健康检查/登录/搜索/上传/在线数）
│   ├── middleware/              # 中间件（JWT鉴权/CORS/在线统计）
│   ├── mq/                     # RocketMQ 生产者管理
│   ├── repository/
│   │   ├── mysqlx/             # MySQL 数据访问（房间/消息）
│   │   └── redisx/             # Redis 数据访问（会话缓存/锁/序列号/用户路由）
│   ├── router/                 # Gin 路由注册
│   ├── service/                # 核心业务层（WebSocket生命周期/消息发送/MQ消费）
│   └── ws/                     # WebSocket Hub + Client（连接管理）
├── pkg/
│   ├── config/                 # Viper 配置加载
│   ├── errorx/                 # 自定义错误码
│   ├── jwt/                    # JWT 生成/解析
│   └── logger/                 # Zap 日志初始化
├── Dockerfile                  # 多阶段 Docker 构建
├── docker-compose.yml          # 一键部署（MySQL + Redis + RocketMQ + App）
├── go.mod / go.sum             # Go 模块依赖
└── LICENSE
```

## 🚀 快速开始

### 环境要求

- Go 1.25+
- MySQL 8.0+
- Redis 7+
- RocketMQ 5.x（NameServer + Broker）

### 方式一：本地运行

**1. 克隆项目**

```bash
git clone https://github.com/your-org/BeeLinkIM.git
cd BeeLinkIM
```

**2. 修改配置**

编辑 `configs/config.yaml`，填写你的数据库、缓存、消息队列连接信息：

```yaml
server:
  port: 8080
  mode: debug

mysql:
  dsn: "root:yourpassword@tcp(127.0.0.1:3306)/chat_db?charset=utf8mb4&parseTime=True&loc=Local"

redis:
  addr: "127.0.0.1:6379"
  password: ""

rocketmq:
  name_server: "127.0.0.1:9876"

app:
  server_id: "server-01"
  jwt_secret: "your-jwt-secret-key"
```

**3. 编译并启动**

```bash
go mod tidy
go build -o chat-server ./cmd/chat-server
./chat-server
```

### 方式二：Docker Compose 一键部署

```bash
docker-compose up -d
```

这将自动启动 MySQL、Redis、RocketMQ NameServer/Broker 以及聊天服务。

## 📡 API 接口

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/health` | 健康检查 |
| `GET` | `/online` | 获取在线人数 |
| `POST` | `/login` | 用户登录 |
| `POST` | `/upload` | 文件上传 |
| `GET` | `/search` | 搜索接口 |
| `GET` | `/api/v1/hello` | 测试接口 |

### 需要认证的接口

| 方法 | 路径 | 认证方式 | 说明 |
|------|------|----------|------|
| `GET` | `/chat/ws` | JWT（Header 或 Query） | 建立 WebSocket 长连接 |

### WebSocket 消息格式

**客户端发送：**

```json
{
  "to_uid": 1002,
  "content": "你好！"
}
```

**服务端推送：**

```json
{
  "from_uid": 1001,
  "to_uid": 1002,
  "content": "你好！",
  "sequence": 42
}
```

## 🔑 认证说明

采用 JWT（HS256 对称加密），Token 有效期 7 天。支持两种传参方式：

- **HTTP Header**（推荐用于 REST API）：
  ```
  Authorization: Bearer <your-token>
  ```
- **Query 参数**（用于 WebSocket，浏览器无法自定义 Header）：
  ```
  /chat/ws?token=<your-token>
  ```

## 🗄️ 数据模型

### chat_rooms（聊天房间表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键自增 |
| uid1 | BIGINT | 较小的用户ID（唯一索引） |
| uid2 | BIGINT | 较大的用户ID（唯一索引） |

### chat_messages（聊天消息表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键自增 |
| room_id | BIGINT | 房间ID（索引） |
| from_uid | BIGINT | 发送者ID（索引） |
| to_uid | BIGINT | 接收者ID（索引） |
| content | TEXT | 消息内容 |
| sequence | BIGINT | 房间内唯一序号（唯一索引） |

## 📊 错误码说明

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 1000 | 服务器错误 |
| 1001 | 参数错误 |
| 1002 | 未授权/未登录 |
| 1003 | 无权限 |
| 2001 | 房间不存在 |
| 2002 | 消息发送失败 |
| 2003 | 分布式锁获取失败 |
| 2004 | 用户不在线 |

## 📄 License

[MIT](LICENSE) © 2026 CanWeakerWriteStrongCode
