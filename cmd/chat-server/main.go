package main

// 导入所有依赖：项目内部包 + 第三方库 + 系统标准库
import (
	"BeeLinkIM/internal/api"
	"BeeLinkIM/internal/mq"
	"BeeLinkIM/internal/repository/mysqlx" // MySQL 数据仓库
	"BeeLinkIM/internal/repository/redisx" // Redis 数据仓库
	"BeeLinkIM/internal/router"
	"BeeLinkIM/internal/service" // 核心业务服务层
	"BeeLinkIM/internal/ws"
	"BeeLinkIM/pkg/config" // 配置文件加载
	"BeeLinkIM/pkg/logger" // 全局日志工具
	"context"              // 上下文：控制超时、取消
	"fmt"                  // 格式化输出
	"net/http"             // HTTP 服务标准库
	"os"                   // 系统信号、文件操作
	"os/signal"            // 监听系统退出信号（Ctrl+C）
	"syscall"              // 系统调用定义
	"time"                 // 时间：超时控制

	"github.com/bsm/redislock" // Redis 分布式锁
	"go.uber.org/zap"          // Zap 日志
	"gorm.io/driver/mysql"     // GORM MySQL 驱动
	"gorm.io/gorm"             // ORM 数据库操作库
)

// main：Go 程序唯一入口函数，程序启动后第一个执行的函数
func main() {

	// ====================== 1. 初始化全局日志 ======================
	// 作用：启动日志系统，配置文件切割、控制台输出，必须第一个执行
	logger.InitLogger()

	// ====================== 2. 加载配置文件 ======================
	// 加载 configs/config.yaml 配置
	if err := config.LoadConfig("configs/config.yaml"); err != nil {
		// Fatal：打印日志后直接退出程序（配置加载失败，服务无法启动）
		logger.Fatal("load config failed", zap.Error(err))
	}

	// 全局配置对象：所有配置从这里取
	cfg := config.GlobalConfig

	// 打印服务启动日志（带上服务ID，分布式多实例区分）
	logger.Info("server starting...", zap.String("server_id", cfg.App.ServerID))

	// ====================== 3. 连接 MySQL 数据库 ======================
	// 通过 GORM 连接 MySQL，DSN 是数据库连接字符串
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		logger.Fatal("connect mysql failed", zap.Error(err))
	}

	// 获取底层 SQL 连接对象，用于配置连接池
	sqlDB, _ := db.DB()

	// 设置 MySQL 最大打开连接数（生产环境防连接耗尽）
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)

	// 设置 MySQL 最大空闲连接数（复用连接，提升性能）
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)

	// 自动迁移数据表：不存在则创建，不修改字段（生产安全）
	// 创建聊天房间表、聊天消息表
	_ = db.AutoMigrate(&mysqlx.TChatRoom{}, &mysqlx.TChatMessage{})

	// ====================== 4. 连接 Redis ======================
	// 初始化 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Ping Redis 验证连接是否成功
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		logger.Fatal("connect redis failed", zap.Error(err))
	}

	// ====================== 5. 初始化分布式锁 ======================
	// 基于 Redis 实现分布式锁，解决集群并发问题
	locker := redislock.New(rdb)

	// ====================== 6. 初始化数据仓库层 ======================
	// MySQL 仓库：封装所有数据库操作
	mysqlRepo := mysqlx.NewMySQLRepo(db)
	// Redis 仓库：封装缓存、分布式锁操作
	redisRepo := redisx.NewRedisRepo(rdb, locker)

	// ====================== 7. 初始化 WebSocket 连接管理器 ======================
	// 创建 Hub 实例：管理所有在线客户端、消息推送
	hub := ws.NewHub()

	// 启动 Hub 主循环：必须开协程（go func），否则阻塞主程序
	// 之前重点讲过：Run() 是无限循环，负责上下线管理
	go hub.Run()

	// ====================== 8. 初始化消息队列（RocketMQ）======================
	mqMgr := mq.NewMQManager(&cfg.RocketMQ)

	// 初始化 WebSocket 处理器
	wsH := api.NewWsHandler(hub)

	// ====================== 9. 初始化核心聊天业务服务 ======================
	// 注入所有依赖：数据库、缓存、Hub、MQ、配置（依赖注入，解耦）
	chatService := service.NewChatService(mysqlRepo, redisRepo, hub, mqMgr, cfg, wsH)

	// 启动 MQ 消费者：监听消息，实现集群消息同步
	service.StartConsumer(&cfg.RocketMQ, cfg.App.ServerID, hub)

	initRouter := router.InitRouter(cfg, chatService)

	// ====================== 11. 启动 HTTP 服务 ======================
	// 创建 HTTP 服务实例
	srv := &http.Server{
		Addr:         ":" + fmt.Sprintf("%d", cfg.Server.Port), // 监听端口
		Handler:      initRouter,                               // 绑定 Gin 路由
		ReadTimeout:  cfg.Server.ReadTimeout * time.Second,     // 读超时（单位：秒）
		WriteTimeout: cfg.Server.WriteTimeout * time.Second,    // 写超时（单位：秒）
		IdleTimeout:  cfg.Server.IdleTimeout * time.Second,     // 空闲超时（单位：秒）
	}

	// 启动 HTTP 服务：开协程，防止阻塞主程序（主程序要监听退出信号）
	go func() {
		// 启动监听
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 启动失败直接退出
			logger.Fatal("listen failed", zap.Error(err))
		}
	}()
	// 打印服务启动成功日志
	logger.Info("server started", zap.Int("port", cfg.Server.Port))

	// ====================== 12. 优雅关闭（生产环境核心）======================
	// 创建管道：监听系统退出信号
	quit := make(chan os.Signal, 1)

	// 监听 2 种退出信号：
	// SIGINT：Ctrl+C
	// SIGTERM：kill 命令
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞：等待退出信号（程序一直运行，直到收到信号）
	<-quit
	logger.Info("shutting down server...")

	// 创建上下文：5秒超时，强制关闭等待时间
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	// defer：函数退出前取消上下文，释放资源
	defer cancel()

	// 优雅关闭 HTTP 服务：
	// 1. 停止接收新请求
	// 2. 等待现有请求处理完
	// 3. 超时强制关闭
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	// 服务完全退出日志
	logger.Info("server exiting")
}
