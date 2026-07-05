// 包声明：核心业务服务层
// 作用：封装聊天核心业务逻辑（发消息、上线、会话管理），衔接数据层和传输层
package service

// 导入依赖：数据仓库 + 配置 + 日志 + 工具库
import (
	"BeeLinkIM/internal/api"
	"BeeLinkIM/internal/mq"
	"BeeLinkIM/internal/repository/mysqlx" // MySQL数据操作（房间、消息）
	"BeeLinkIM/internal/repository/redisx" // Redis数据操作（会话、序列号、在线状态）
	"BeeLinkIM/internal/ws"
	"BeeLinkIM/pkg/config" // 全局配置
	"BeeLinkIM/pkg/logger" // 全局日志
	"context"              // 上下文（超时、链路追踪）
	"encoding/json"        // 解析客户端消息
	"net/http"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap" // 日志结构化打印
)

// -------------------------- 1. 聊天服务核心结构体（依赖注入） --------------------------
// 所有聊天业务的入口，通过构造函数注入所有依赖（解耦、可测试、易维护）
type ChatService struct {
	mysqlRepo *mysqlx.MySQLRepo // MySQL仓库：操作数据库（房间、消息）
	redisRepo *redisx.RedisRepo // Redis仓库：操作缓存（会话、在线状态、序列号）
	hub       *ws.Hub           // 连接管理器：给【本服务器】用户推消息
	mqMgr     *mq.MQManager     // 消息队列：给【其他服务器】用户推消息
	cfg       *config.Config    // 全局配置：服务ID、MQ主题等
	wsHandler *api.WsHandler    // WebSocket处理程序：处理客户端连接、消息
}

// -------------------------- 1. 消息载体结构体 --------------------------
// 聊天消息的标准结构体：MQ传递的消息格式
// 作用：分布式节点之间传递聊天消息，必须统一格式
// json标签：用于JSON序列化，把结构体转成字节流在MQ中传输
type ChatMessagePayload struct {
	FromUID  int64  `json:"from_uid"` // 发送者用户ID
	ToUID    int64  `json:"to_uid"`   // 接收者用户ID
	Content  string `json:"content"`  // 消息内容
	Sequence int64  `json:"sequence"` // 消息序列号（用于去重/排序）
}

// -------------------------- 2. 客户端消息结构体 --------------------------
// 前端客户端通过WebSocket发送的原始消息格式
// json标签：用于解析前端传来的JSON字符串
type ClientMessage struct {
	ToUID   int64  `json:"to_uid"`  // 接收方用户ID
	Content string `json:"content"` // 消息内容
}

// -------------------------- 3. 构造函数：依赖注入 --------------------------
// 创建ChatService实例，统一注入所有依赖（生产级标准写法：控制反转）
func NewChatService(m *mysqlx.MySQLRepo, r *redisx.RedisRepo, h *ws.Hub, mq *mq.MQManager, cfg *config.Config, wsH *api.WsHandler) *ChatService {
	return &ChatService{
		mysqlRepo: m,
		redisRepo: r,
		hub:       h,
		mqMgr:     mq,
		cfg:       cfg,
		wsHandler: wsH,
	}
}

// -------------------------- 7. 用户上线：记录在线服务器 --------------------------
// 附属方法：用户连接WebSocket时调用，记录用户在哪个服务器
// 作用：分布式路由消息的核心依据
func (chatService *ChatService) UserOnline(ctx context.Context, uid int64) error {
	// 把用户ID和当前服务器ID绑定存入Redis
	return chatService.redisRepo.SetUserServer(ctx, uid, chatService.cfg.App.ServerID)
}

// -------------------------- 8. 启动MQ消费者 --------------------------
// 附属方法：启动消费者，监听MQ消息，消费后推送给本地用户
// 核心逻辑：其他节点发来的消息 → MQ → 本节点消费 → 推送给本节点在线用户
func StartConsumer(cfg *config.RocketMQConfig, localServerID string, hub *ws.Hub) {
	// 创建PushConsumer（推模式消费者：MQ主动把消息推给客户端）
	pushConsumer, err := rocketmq.NewPushConsumer(
		// 配置NameServer
		consumer.WithNameServer([]string{cfg.NameServer}),
		// 配置消费者组（同一组的消费者负载均衡消费消息）
		consumer.WithGroupName(cfg.Group),
	)
	// 创建消费者失败：致命错误
	if err != nil {
		logger.Fatal("create mq consumer failed", zap.Error(err))
	}

	// 订阅MQ主题：监听聊天消息
	// 第二个参数：消息过滤器（这里不过滤，消费所有消息）
	// 第三个参数：消息消费回调函数（收到消息自动执行）
	_ = pushConsumer.Subscribe(cfg.Topic, consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		// 遍历收到的消息（批量消费）
		for _, msg := range msgs {
			// 定义消息载体
			var payload ChatMessagePayload
			// 反序列化：把MQ的字节流转回结构体
			if err := json.Unmarshal(msg.Body, &payload); err != nil {
				logger.Error("unmarshal mq msg failed", zap.Error(err))
				continue // 解析失败，跳过这条消息
			}

			// 核心！调用本地Hub，把消息推送给【本服务器】的在线用户
			// 场景：接收者连接在本节点，直接推送
			hub.SendToUID(payload.ToUID, msg.Body)
		}
		// 返回消费成功：告诉MQ已经处理完消息
		return consumer.ConsumeSuccess, nil
	})

	// 启动消费者（开始监听消息）
	if err := pushConsumer.Start(); err != nil {
		logger.Fatal("start mq consumer failed", zap.Error(err))
	}

	// 消费者启动成功日志
	logger.Info("mq consumer started")
}

// -------------------------- 3. 核心接口方法：处理 WebSocket 连接请求 --------------------------
// 作用：处理 GET /ws 请求，完成 鉴权 → 协议升级 → 客户端注册 → 启动读写协程
func (chatService *ChatService) ServeWs(ginContext *gin.Context) {

	// 1. 从 Gin 上下文获取用户ID（uid）
	// 关键：uid 是 JWT 鉴权中间件 解析后存入上下文的
	uidInterface, exists := ginContext.Get("uid")
	// 判断：是否成功获取到用户ID（鉴权是否通过）
	if !exists {
		// 未获取到 → 鉴权失败，返回 401 未授权
		ginContext.JSON(http.StatusUnauthorized, gin.H{"msg": "auth failed"})
		return
	}
	// 类型断言：将空接口转为 int64 类型的用户ID
	// 原因：Gin 上下文存储的是 interface{} 类型，必须强转才能使用
	uid := uidInterface.(int64)

	// 打印日志：用户正在建立 WebSocket 连接
	logger.Info("user connecting via ws", zap.Int64("uid", uid))

	// 2. 协议升级：将 HTTP 连接 升级为 WebSocket 长连接
	// 核心：HTTP 是短连接，无法实时推送消息，必须升级为 WebSocket
	// ginContext.Writer：响应写入器 | ginContext.Request：当前请求对象
	conn, err := chatService.wsHandler.Upgrader.Upgrade(ginContext.Writer, ginContext.Request, nil)
	// 升级失败（网络错误、协议不支持）：打印日志并退出
	if err != nil {
		logger.Error("upgrade websocket failed", zap.Error(err))
		return
	}

	// 3. 创建客户端实例：绑定 Hub、WebSocket 连接、用户ID、业务服务
	// 每个用户对应一个 Client 实例，管理该用户的所有消息收发
	client := ws.NewClient(chatService.hub, conn, uid)

	// 4. 客户端注册（上线）
	// 向 Hub 的 Register 通道发送客户端实例
	// Hub.Run() 会监听该通道，将客户端存入在线列表
	chatService.hub.Register <- client

	// 5. 标记用户上线：将【用户ID + 当前服务器ID】存入 Redis
	// 核心作用：分布式集群中，知道用户连接在哪个服务器，实现跨服消息推送
	_ = chatService.UserOnline(ginContext.Request.Context(), uid)

	// 6. 启动【写消息协程】：专门负责给客户端发送消息
	// 分离读写协程：高并发下不阻塞，生产环境 WebSocket 标准写法
	go client.WritePump()

	// 7. 启动【读消息协程】：专门负责接收客户端发送的消息
	// 两个协程独立运行，互不干扰
	go chatService.ReadPump(client)
}

// 附属方法：属于 ChatService 结构体
// 核心作用：**专门监听并读取客户端发送的消息**，处理心跳、异常断开、消息分发
// 参数：*ws.Client 当前用户的 WebSocket 客户端实例
// 必须在独立协程中运行（go chatService.ReadPump(client)）
func (chatService *ChatService) ReadPump(client *ws.Client) {

	// ====================== 1. defer 延迟清理：连接断开必执行 ======================
	// defer：函数退出（无论正常/异常断开）时，最后执行
	// 作用：防止客户端断开后，资源泄漏、在线列表残留
	defer func() {
		// 向 Hub 的注销通道发送客户端
		// Hub 会将客户端从【在线列表】中删除，释放服务端资源
		client.Hub.Unregister <- client
		// 关闭 WebSocket 底层连接，释放网络端口
		_ = client.Conn.Close()
	}()

	// ====================== 2. WebSocket 基础配置 ======================
	// 设置客户端消息最大读取长度 = 512字节（常量定义在 ws 包）
	// 原因：限制单条消息大小，防止超大消息攻击、占用过多内存
	client.Conn.SetReadLimit(ws.MaxMessageSize)

	// 设置【初始读超时时间】= 60秒（PongWait）
	// 原因：如果60秒内没有收到任何客户端消息（包括心跳），判定连接断开
	// 后续会通过 Pong 心跳不断重置这个时间
	_ = client.Conn.SetReadDeadline(time.Now().Add(ws.PongWait))

	// 设置 Pong 心跳处理器（核心：保活长连接）
	// 场景：服务端发送 Ping 后，客户端回复 Pong 时，自动触发这个函数
	client.Conn.SetPongHandler(func(string) error {
		// 收到客户端的 Pong 响应：**重置读超时时间为60秒**
		// 原因：只要客户端正常回复心跳，连接就不会因超时断开
		_ = client.Conn.SetReadDeadline(time.Now().Add(ws.PongWait))
		return nil
	})

	// ====================== 3. 无限循环：持续读取客户端消息 ======================
	// 死循环：只要连接不断开，就一直监听客户端消息
	for {
		// ReadMessage：阻塞读取客户端发送的消息
		// 返回值：消息类型、消息字节数组、错误
		// 阻塞逻辑：没有消息时，协程休眠，不占用CPU
		msgType, message, err := client.Conn.ReadMessage()

		logger.Info("websocket read message", zap.Int("msgType", msgType), zap.ByteString("message", message))
		// ====================== 4. 消息读取错误处理 ======================
		// err != nil：说明客户端断开连接/网络异常
		if err != nil {
			// 判断是否为【非预期关闭错误】（排除正常关闭的情况）
			// CloseGoingAway：客户端主动离开
			// CloseAbnormalClosure：异常关闭
			// 只打印真正的异常错误，避免日志泛滥
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error("websocket read error", zap.Error(err))
			}
			// 退出循环：连接已断开，结束读协程
			break
		}

		// ====================== 5. 异步处理业务消息 ======================
		// 开启独立协程处理客户端消息
		// 原因：**绝对不能阻塞读协程**
		// 如果同步处理消息（耗时/查库），会导致无法接收新消息、心跳超时
		go chatService.HandleClientMessage(client.Uid, message)
	}
}

// -------------------------- 4. 处理客户端发来的WebSocket消息 --------------------------
// 附属方法：解析前端原始消息，调用发送逻辑
// 参数：fromUID=发送者ID，rawMsg=前端发来的原始字节数据
func (chatService *ChatService) HandleClientMessage(fromUID int64, rawMsg []byte) {
	// 1. 定义消息结构体，用于接收解析后的数据
	var msg ClientMessage
	// 2. 反序列化：把前端的JSON字节 → Go结构体
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		logger.Error("parse client msg failed", zap.Error(err))
		return // 解析失败，直接丢弃消息
	}
	// 3. 调用核心发送方法
	if err := chatService.SendMessage(context.Background(), fromUID, msg.ToUID, msg.Content); err != nil {
		logger.Error("send msg failed", zap.Error(err))
	}
}

// -------------------------- 5. 核心方法：发送聊天消息（最核心业务） --------------------------
// 附属方法：完整的消息发送流程（会话→序列号→封装→落库→分布式推送）
func (chatService *ChatService) SendMessage(ctx context.Context, fromUID, toUID int64, content string) error {
	// 1. 获取/初始化双人会话（房间ID+消息序列号）
	// 作用：每个聊天对有唯一房间，保证消息有序
	roomSession, err := chatService.getOrInitSession(ctx, fromUID, toUID)
	if err != nil {
		return err
	}

	// 2. 生成自增序列号（Redis原子自增）
	// 作用：保证消息有序、去重，分布式环境唯一
	seq, err := chatService.redisRepo.IncrSequence(ctx, fromUID, toUID)
	if err != nil {
		return err
	}

	// 3. 封装MQ/推送用的标准消息体
	payload := &ChatMessagePayload{
		FromUID:  fromUID,
		ToUID:    toUID,
		Content:  content,
		Sequence: seq,
	}
	// 序列化为字节（WebSocket/MQ只能传输字节）
	msgBytes, _ := json.Marshal(payload)

	// -------------------------- 异步持久化消息（不阻塞主流程） --------------------------
	// 开协程：把消息存MySQL，异步执行，不影响消息实时推送
	go func() {
		dbMsg := &mysqlx.TChatMessage{
			RoomID:   roomSession.RoomID, // 绑定房间ID
			FromUID:  fromUID,
			ToUID:    toUID,
			Content:  content,
			Sequence: seq,
		}
		// 保存消息到数据库
		if err := chatService.mysqlRepo.CreateMessage(context.Background(), dbMsg); err != nil {
			logger.Error("save msg to db failed", zap.Error(err))
		}
	}()

	// -------------------------- 分布式消息路由（核心！判断用户在哪个服务器） --------------------------
	client, ok := chatService.hub.FindByUid(toUID)

	// 判断：接收者是否在【当前服务器】
	if ok {
		//  本地用户：直接通过Hub推送（高性能）
		chatService.hub.SendToClient(client, msgBytes)
	} else {
		//  跨服务器用户：发送到MQ，让目标服务器消费推送
		body, _ := json.Marshal(payload)
		_ = chatService.mqMgr.SendMessage(ctx, chatService.cfg.RocketMQ.Topic, body)
	}

	return nil
}

// -------------------------- 6. 获取/初始化双人会话（房间） --------------------------
// 附属方法：双人聊天，没有房间就创建，有就直接用（加分布式锁防并发重复创建）
func (chatService *ChatService) getOrInitSession(ctx context.Context, uid1, uid2 int64) (*redisx.RoomSession, error) {
	// 1. 先查Redis缓存：快速获取会话
	roomSession, err := chatService.redisRepo.GetRoomSession(ctx, uid1, uid2)
	// 缓存命中，直接返回
	if err == nil && roomSession != nil {
		return roomSession, nil
	}

	// -------------------------- 缓存未命中：加分布式锁，防止并发创建房间 --------------------------
	lockKey := "room:" + string(uid1) + ":" + string(uid2)
	// 获取Redis分布式锁
	unlock, err := chatService.redisRepo.GetLock(ctx, lockKey)
	if err != nil {
		return nil, err
	}
	// defer：函数退出自动释放锁（防止死锁）
	defer unlock()

	// 双重检查：防止加锁期间其他协程已创建会话
	roomSession, _ = chatService.redisRepo.GetRoomSession(ctx, uid1, uid2)
	if roomSession != nil {
		return roomSession, nil
	}

	// 2. MySQL查询/创建房间（没有就创建，有就返回）
	room, err := chatService.mysqlRepo.GetOrCreateRoom(ctx, uid1, uid2)
	if err != nil {
		return nil, err
	}

	// 3. 把新房间会话存入Redis缓存
	newSession := &redisx.RoomSession{RoomID: room.ID, Sequence: 0}
	_ = chatService.redisRepo.SetRoomSession(ctx, uid1, uid2, newSession)

	return newSession, nil
}
