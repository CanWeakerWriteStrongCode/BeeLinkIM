package config

// 导入依赖
import (
	"fmt" // 用于格式化错误信息，包装底层错误
	"time"

	"github.com/spf13/viper" // Go 生态最主流的配置管理库，支持 yaml/json/环境变量等
)

// -------------------------- 1. 定义配置结构体（映射 yaml 配置文件） --------------------------
// Go 中用【结构体】对应配置文件的【层级结构】，viper 自动映射字段
// 所有字段首字母大写：因为 viper 反序列化需要访问权限（public）

//	服务基础配置
//
// 对应 yaml 里的 server 节点
type ServerConfig struct {
	Port         int    // 服务监听端口 (8080)
	Mode         string // Gin 运行模式 (debug/release)
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

//	MySQL 数据库配置
//
// 对应 yaml 里的 mysql 节点
type MySQLConfig struct {
	DSN          string // 数据库连接字符串 (用户名:密码@tcp(ip:端口)/库名)
	MaxOpenConns int    // 最大打开连接数（连接池配置）
	MaxIdleConns int    // 最大空闲连接数（连接池配置）
}

//	Redis 配置
//
// 对应 yaml 里的 redis 节点
type RedisConfig struct {
	Addr     string // Redis 地址 (ip:端口)
	Password string // Redis 密码
	DB       int    // Redis 库号 (0-15)
}

//	消息队列配置
//
// 对应 yaml 里的 rocketmq 节点
type RocketMQConfig struct {
	NameServer string // RocketMQ 命名服务地址
	Group      string // 消费者组ID
	Topic      string // 消息主题
}

//	应用自定义配置
//
// 对应 yaml 里的 app 节点
type AppConfig struct {
	ServerID  string // 服务ID（分布式多实例区分）
	JWTSecret string // JWT 签名密钥（登录鉴权用）
}

//	总配置结构体
//
// 把所有子配置整合在一起，对应 yaml 根节点
type Config struct {
	Server   ServerConfig   // 服务配置
	MySQL    MySQLConfig    // 数据库配置
	Redis    RedisConfig    // 缓存配置
	RocketMQ RocketMQConfig // 消息队列配置
	App      AppConfig      // 应用配置
}

// -------------------------- 2. 全局配置对象 --------------------------
// 全局变量：整个项目所有包都能直接读取配置
// 大写：外部包可访问
var GlobalConfig *Config

// -------------------------- 3. 加载配置文件核心函数 --------------------------
// LoadConfig 加载并解析 yaml 配置文件
// 参数 path：配置文件路径 (configs/config.yaml)
// 返回值：加载失败返回错误
func LoadConfig(path string) error {

	// 指定要加载的配置文件路径
	viper.SetConfigFile(path)

	// 明确指定配置文件类型为 YAML
	// 作用：viper 自动按 yaml 格式解析，不指定会自动识别，显式写更严谨
	viper.SetConfigType("yaml")

	// 读取配置文件内容到 viper 内存中
	if err := viper.ReadInConfig(); err != nil {
		// 包装错误：%w 保留底层错误信息，方便排查问题
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// 初始化全局配置对象（空结构体）
	GlobalConfig = &Config{}

	// 将 viper 内存中的配置，反序列化为 Go 结构体
	// 作用：把 yaml 文本 → 代码里的结构体对象，方便代码调用
	if err := viper.Unmarshal(GlobalConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 配置加载解析成功，返回 nil
	return nil
}
