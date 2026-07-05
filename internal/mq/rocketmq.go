package mq

// 导入依赖包
import (
	"BeeLinkIM/pkg/config" // 项目配置中心，读取MQ配置
	"BeeLinkIM/pkg/logger" // 全局日志，打印MQ相关日志
	"context"              // 上下文，用于MQ的超时、取消控制

	"github.com/apache/rocketmq-client-go/v2"
	// 基础工具包（消息、地址等）
	"github.com/apache/rocketmq-client-go/v2/primitive"
	// 生产者相关API
	"github.com/apache/rocketmq-client-go/v2/producer"
	// 日志库
	"go.uber.org/zap"
)

// -------------------------- 2. MQ管理器结构体 --------------------------
// 管理RocketMQ的【生产者】和【消费者】，绑定Hub（本地连接管理器）
// 核心作用：
// 1. 生产者：把消息发送到MQ，给其他服务节点消费
// 2. 持有hub：消费到消息后，推送给【本服务器】的在线用户
type MQManager struct {
	producer rocketmq.Producer // MQ生产者：发送消息到队列
}

// -------------------------- 3. MQ管理器构造函数 --------------------------
// 创建并初始化MQManager：创建+启动生产者
// 参数：cfg=MQ配置，hub=本地连接管理器
func NewMQManager(cfg *config.RocketMQConfig) *MQManager {
	// 创建RocketMQ生产者实例
	newProducer, err := rocketmq.NewProducer(
		// 设置NameServer地址（MQ核心寻址服务）
		producer.WithNameServer([]string{cfg.NameServer}),
		// 设置生产者组名（集群标识）
		producer.WithGroupName(cfg.Group),
	)
	// 创建生产者失败：致命错误，服务无法启动
	if err != nil {
		logger.Fatal("create mq producer failed", zap.Error(err))
	}

	// 启动生产者（必须启动才能发送消息）
	if err := newProducer.Start(); err != nil {
		logger.Fatal("start mq producer failed", zap.Error(err))
	}

	// 返回MQManager实例，注入生产者和hub
	return &MQManager{producer: newProducer}
}

// -------------------------- 4. 发送消息到MQ（生产者方法） --------------------------
// 附属方法：属于MQManager，发送聊天消息到RocketMQ
// 场景：本节点用户发送消息给【其他节点】的用户，先发到MQ，让目标节点消费
func (mqManager *MQManager) SendMessage(ctx context.Context, topic string, body []byte) error {
	// 2. 创建MQ消息对象：指定Topic+消息体
	msg := primitive.NewMessage(topic, body)
	// 3. 同步发送消息到MQ（等待MQ确认收到）
	_, err := mqManager.producer.SendSync(ctx, msg)
	// 返回发送结果
	return err
}
