package logger

// 导入依赖包
import (
	"os" // 系统标准库，用于控制台输出

	"go.uber.org/zap"                  // 生产级高性能日志库（Go官方推荐）
	"go.uber.org/zap/zapcore"          // Zap核心库，定义日志核心结构（编码器、输出、级别）
	"gopkg.in/natefinch/lumberjack.v2" // 日志文件切割库（解决日志文件过大问题）
)

// 全局日志对象
// 大写：外部所有包都能直接使用
// 整个项目共用一个日志实例，保证配置统一
var Logger *zap.Logger

// 初始化日志核心函数
// 作用：配置日志输出格式、文件切割、控制台打印、日志级别，全局只调用一次
func InitLogger() {

	// -------------------------- 1. 配置日志文件切割（生产环境必备） --------------------------
	// lumberjack.Logger：日志滚动切割工具，防止日志文件无限变大撑爆磁盘
	writer := &lumberjack.Logger{
		Filename:   "./logs/app.log", // 日志文件存放路径（相对路径）
		MaxSize:    100,              // 单个日志文件最大 100MB，超过自动切割新文件
		MaxBackups: 120,              // 最多保留 120 个历史日志文件
		MaxAge:     60,               // 日志文件最多保留 60 天，过期自动删除
		Compress:   true,             // 开启压缩：旧日志文件自动打包成.gz，节省磁盘空间
	}

	// -------------------------- 2. 创建【文件日志核心】 --------------------------
	// zapcore.Core：Zap的核心组件，决定日志「怎么编码、输出到哪、输出什么级别」
	core := zapcore.NewCore(
		// 编码器：JSON格式（生产环境推荐，方便ELK日志系统采集解析）
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		// 输出目标：绑定上面的日志切割工具，输出到文件
		zapcore.AddSync(writer),
		// 日志级别：INFO级别及以上（INFO/WARN/ERROR/FATAL）才写入文件
		// 原因：DEBUG日志太多，只打印到控制台，不写入文件节省磁盘
		zapcore.InfoLevel,
	)

	// -------------------------- 3. 创建【控制台日志核心】 --------------------------
	consoleCore := zapcore.NewCore(
		// 编码器：友好的控制台格式（易读，开发/运维调试用）
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		// 输出目标：标准输出（控制台/终端）
		zapcore.AddSync(os.Stdout),
		// 日志级别：DEBUG级别及以上（开发调试需要看详细日志）
		zapcore.DebugLevel,
	)

	// -------------------------- 4. 合并两个日志核心，创建全局Logger --------------------------
	// zapcore.NewTee：同时使用多个Core → 日志**同时输出到文件+控制台**
	// zap.AddCaller()：记录日志打印的【文件名+行号】（生产排查问题必备）
	// zap.AddCallerSkip(1)：跳过1层调用栈
	// 原因：下面封装了Info/Error等快捷方法，需要跳过封装层，打印真实业务代码行号
	Logger = zap.New(zapcore.NewTee(core, consoleCore), zap.AddCaller(), zap.AddCallerSkip(1))

	// 日志初始化成功，打印提示日志
	// 作用：启动服务时确认日志模块正常工作
	Logger.Info("init Logger success")

	// -------------------------- 5. 延迟执行：刷新日志缓冲区 --------------------------
	// defer：函数退出前**最后一刻**执行
	// Logger.Sync()：将缓冲区的日志**强制刷写到磁盘**
	// 原因：Zap有缓冲区，不执行Sync可能导致最后几条日志丢失
	defer Logger.Sync()
}

// -------------------------- 6. 封装全局快捷方法（简化外部调用） --------------------------
// 外部不用写 logger.Logger.Info，直接写 logger.Info 即可
// 作用：简化代码，统一调用入口

// 普通信息日志
func Info(msg string, fields ...zap.Field) { Logger.Info(msg, fields...) }

// 警告日志（不影响运行，需要关注）
func Warn(msg string, fields ...zap.Field) { Logger.Warn(msg, fields...) }

// 错误日志（业务异常，必须排查）
func Error(msg string, fields ...zap.Field) { Logger.Error(msg, fields...) }

// 致命错误（程序无法运行，打印后直接退出进程）
func Fatal(msg string, fields ...zap.Field) { Logger.Fatal(msg, fields...) }
