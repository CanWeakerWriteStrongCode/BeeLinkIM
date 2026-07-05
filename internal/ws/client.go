// 包声明：ws 包
// 专门封装 WebSocket 客户端的所有逻辑（连接、读写、心跳），职责单一、代码解耦
package ws

import (
	"time" // 用于：超时控制、心跳定时器、写截止时间

	"github.com/gorilla/websocket" // Go 生态最稳定的 WebSocket 第三方库
)

// ====================== WebSocket 全局常量配置（公开，外部可调用） ======================
// 常量全部大写：遵循 Go 规范，同时支持外部包调用
const (
	// WriteWait 写超时时间：10秒
	// 作用：向客户端发送消息时，超过10秒未发送成功，判定为连接失效
	WriteWait = 10 * time.Second

	// PongWait 等待心跳响应超时：60秒
	// 作用：服务端发送 Ping 后，60秒内未收到客户端 Pong 响应，断开连接
	// 原因：防止死连接占用服务器资源
	PongWait = 60 * time.Second

	// PingPeriod 心跳发送周期：54秒
	// 公式 = PongWait * 0.9，**必须小于超时时间**
	// 原因：给网络预留缓冲时间，避免心跳刚发送就超时
	PingPeriod = (PongWait * 9) / 10

	// MaxMessageSize 客户端消息最大长度：512字节
	// 作用：限制消息大小，防止超大消息攻击、占用内存
	MaxMessageSize = 512
)

// ====================== WebSocket 客户端结构体 ======================
// Client 代表**一个用户**的 WebSocket 长连接
// 每个在线用户，对应唯一的 Client 实例
type Client struct {
	Hub  *Hub            // 绑定全局连接管理器：用于注册/注销客户端、广播消息
	Conn *websocket.Conn // WebSocket 原生连接对象：底层读写网络数据
	send chan []byte     // 消息发送通道：**缓冲通道**，解耦「业务层」和「网络发送层」
	Uid  int64           // 当前客户端绑定的用户ID：用于定向推送消息
}

// NewClient 客户端构造函数
// 作用：初始化 Client 实例，创建缓冲发送通道
func NewClient(hub *Hub, conn *websocket.Conn, uid int64) *Client {
	return &Client{
		Hub:  hub,
		Conn: conn,
		// send 通道缓冲256：
		// 1. 防止发送消息时阻塞业务协程
		// 2. 应对瞬时高并发消息，提升稳定性
		send: make(chan []byte, 256),
		Uid:  uid,
	}
}

// ====================== 核心方法：客户端写消息协程 ======================
// WritePump
// 附属方法：属于 Client 结构体
// 作用：**专门负责向客户端发送数据**（业务消息+心跳包）
// 必须在独立协程中运行（go client.WritePump()）
func (client *Client) WritePump() {
	// 创建定时器：按照 PingPeriod 周期触发（每54秒）
	// 作用：定时发送 Ping 心跳包，维持长连接
	ticker := time.NewTicker(PingPeriod)

	// defer 延迟函数：**方法退出前必执行**
	// 作用：释放资源，防止内存泄漏、连接泄漏
	defer func() {
		ticker.Stop()           // 停止心跳定时器
		_ = client.Conn.Close() // 关闭 WebSocket 连接
	}()

	// 无限循环：持续监听发送通道和心跳定时器
	for {
		// select 多路复用：同时监听多个通道事件
		select {
		// ====================== 事件1：收到业务消息，发送给客户端 ======================
		case message, ok := <-client.send:
			// 设置写超时：10秒内必须发送成功，否则断开连接
			_ = client.Conn.SetWriteDeadline(time.Now().Add(WriteWait))

			// ok=false：send 通道被关闭（客户端下线/服务端主动断开）
			if !ok {
				// 发送关闭帧：告知客户端连接关闭
				_ = client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return // 退出写协程
			}

			// 获取消息写入器：高效写入文本消息
			w, err := client.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return // 写入失败，断开连接
			}
			// 写入业务消息内容
			_, _ = w.Write(message)

			// 关闭写入器：真正将消息发送到网络
			if err := w.Close(); err != nil {
				return
			}

		// ====================== 事件2：心跳定时器触发，发送 Ping 包 ======================
		case <-ticker.C:
			// 设置写超时
			_ = client.Conn.SetWriteDeadline(time.Now().Add(WriteWait))
			// 发送 Ping 心跳包：客户端收到后必须回复 Pong
			// 作用：检测客户端是否在线，维持长连接
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // 发送失败，说明客户端已断开
			}
		}
	}
}
