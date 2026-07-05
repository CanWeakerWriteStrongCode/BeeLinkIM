package ws

// 导入依赖包
import (
	"BeeLinkIM/pkg/logger" // 项目自定义日志包（生产环境必备）
	"sync"                 // Go 标准库：提供锁，解决并发安全问题

	"go.uber.org/zap" // 高性能日志库，生产级日志工具
)

// 核心作用：全局唯一的 WebSocket 连接管理器
// 负责：用户上线、用户下线、消息推送、在线列表维护
type Hub struct {
	// 1. 在线客户端集合
	// Key: 用户ID(int64)  Value: 客户端连接对象(Client)
	// 作用：快速查找某个用户是否在线、获取用户的连接用于发消息
	// 注意：Go 的 map 不支持并发读写，必须配合锁使用
	Clients map[int64]*Client

	// 2. 客户端注册（上线）管道
	// 大写：外部包可访问
	// 作用：接收「客户端上线」的请求，所有上线操作串行化执行
	Register chan *Client

	// 3. 客户端注销（下线）管道
	// 小写：仅当前包可访问
	// 作用：接收「客户端断开/掉线」的请求
	Unregister chan *Client

	// 4. 读写锁（生产环境最优选择）
	// 为什么不用 sync.Mutex？
	// 场景：SendToUID 读操作远多于 上下线写操作
	// 读写锁：多人同时读、一人独占写，性能比普通互斥锁高10倍以上
	mu sync.RWMutex
}

// 作用：初始化管道和map，返回Hub对象（单例全局使用）
func NewHub() *Hub {
	return &Hub{
		Register:   make(chan *Client),      // 初始化无缓冲管道：上线管道
		Unregister: make(chan *Client),      // 初始化无缓冲管道：下线管道
		Clients:    make(map[int64]*Client), // 初始化空map：存储在线用户
	}
}

// 核心方法：启动主循环（死循环）
// 必须用 go hub.Run() 启动协程，否则会阻塞主程序
// 作用：串行处理 上线/下线 请求，保证map操作绝对并发安全
func (hub *Hub) Run() {
	// 无限死循环：Hub 永久运行，直到服务停止
	for {
		// select 监听多个管道
		// 哪个管道有数据，就执行对应的case，无数据时阻塞等待
		select {
		// ====================== 处理：客户端上线 ======================
		case client := <-hub.Register:
			// 加【写锁】：修改map时必须加独占锁
			// 原因：map并发写会直接导致程序panic崩溃
			hub.mu.Lock()

			// 将新客户端存入map：建立用户ID和连接的映射关系
			hub.Clients[client.Uid] = client

			// 释放写锁：锁要尽快释放，不要长时间持有（影响性能）
			hub.mu.Unlock()

			// 生产日志：记录用户上线，方便问题排查
			logger.Info("client connected", zap.Int64("uid", client.Uid))

		// ====================== 处理：客户端下线 ======================
		case client := <-hub.Unregister:
			// 加写锁：保护map删除操作
			hub.mu.Lock()

			// 安全校验：判断用户是否存在于在线列表中
			if _, ok := hub.Clients[client.Uid]; ok {
				// 从map中删除：标记用户离线
				delete(hub.Clients, client.Uid)

				// 关闭客户端的消息发送管道
				// 核心原因：不关闭会导致goroutine泄漏（内存越来越大，服务器卡死）
				// 关闭后，客户端的WritePump协程会自动退出
				close(client.send)
			}

			// 释放写锁
			hub.mu.Unlock()

			// 生产日志：记录用户下线
			logger.Info("client disconnected", zap.Int64("uid", client.Uid))
		}
	}
}

// 核心方法：向指定用户ID发送消息（本机用户）
// 作用：业务层调用此方法，给在线用户推送消息
func (hub *Hub) SendToUID(uid int64, message []byte) {
	// 加【读锁】：读取map时加共享锁
	// 多人可以同时读，性能极高
	client, ok := hub.FindByUid(uid)

	// 判断：用户是否在线
	if ok {
		// 非阻塞发送消息（生产环境必备！）
		// select + default：防止管道满了导致协程阻塞
		select {
		// 消息成功写入客户端的发送管道
		case client.send <- message:

		// 发送失败：说明客户端管道已满/连接假死
		default:
			// 关闭无效管道，释放资源
			close(client.send)

			// 加写锁，从在线map中删除无效连接
			hub.mu.Lock()
			delete(hub.Clients, uid)
			hub.mu.Unlock()
		}
	}
}

// 核心方法：向指定用户ID发送消息（本机用户）
// 作用：业务层调用此方法，给在线用户推送消息
func (hub *Hub) SendToClient(client *Client, message []byte) {
	// 加【读锁】：读取map时加共享锁
	// 多人可以同时读，性能极高
	// 判断：用户是否在线
	// 非阻塞发送消息（生产环境必备！）
	// select + default：防止管道满了导致协程阻塞
	select {
	// 消息成功写入客户端的发送管道
	case client.send <- message:

	// 发送失败：说明客户端管道已满/连接假死
	default:
		// 关闭无效管道，释放资源
		close(client.send)

		// 加写锁，从在线map中删除无效连接
		hub.mu.Lock()
		delete(hub.Clients, client.Uid)
		hub.mu.Unlock()
	}
}

func (hub *Hub) FindByUid(uid int64) (*Client, bool) {
	hub.mu.RLock()

	// 从在线map中查找目标用户
	client, ok := hub.Clients[uid]

	// 释放读锁：读完立即释放，不要持有锁执行后续逻辑
	hub.mu.RUnlock()
	return client, ok
}
