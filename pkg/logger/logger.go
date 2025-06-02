package logger

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
	"time"
)

// CustomFormatter 自定义格式化器
type CustomFormatter struct {
	TimestampFormat string
}

func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// 将时间调整为东八区
	localTime := entry.Time.In(time.FixedZone("CST", 8*3600))
	// 构建日志消息
	timestamp := localTime.Format(f.TimestampFormat)
	level := strings.ToUpper(entry.Level.String())

	// 将所有字段合并到一个字符串中，添加适当的分隔
	var fieldsStr string
	if len(entry.Data) > 0 {
		pairs := make([]string, 0, len(entry.Data))
		for k, v := range entry.Data {
			pairs = append(pairs, fmt.Sprintf("%s: %v", k, v))
		}
		fieldsStr = " | " + strings.Join(pairs, " | ")
	}

	// 简化的日志格式，移除文件名和行号
	logMsg := fmt.Sprintf("[%s] %-5s %s%s\n",
		timestamp,
		level,
		entry.Message,
		fieldsStr,
	)

	return []byte(logMsg), nil
}

var Log = logrus.New()

func Init() {
	// 使用自定义格式化器
	Log.SetFormatter(&CustomFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// 设置输出到标准输出
	Log.SetOutput(os.Stdout)

	// 设置日志级别
	if os.Getenv("DEBUG") == "true" {
		Log.SetLevel(logrus.DebugLevel)
	} else {
		Log.SetLevel(logrus.InfoLevel)
	}
}
