package config

import (
	"augment2api/pkg/logger"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	RedisConnString string
	AuthToken       string
	CodingMode      string
	CodingToken     string
	TenantURL       string
	AccessPwd       string
	RoutePrefix     string
	ProxyURL        string
}

// SystemConfig 系统配置结构
type SystemConfig struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const version = "v1.0.7"

var AppConfig Config

func InitConfig() error {
	// 首次启动时，只从环境变量读取Redis连接字符串
	redisConnString := getEnv("REDIS_CONN_STRING", "")
	if redisConnString == "" {
		logger.Log.Fatalln("首次启动必须配置环境变量 REDIS_CONN_STRING")
	}

	// 设置Redis连接
	AppConfig.RedisConnString = redisConnString

	logger.Log.WithFields(map[string]interface{}{
		"version": version,
	}).Info("基础配置加载完成，等待数据库配置初始化")

	return nil
}

// LoadConfigFromDatabase 从数据库加载配置
func LoadConfigFromDatabase() error {
	// 初始化默认配置
	err := initializeDefaultConfigs()
	if err != nil {
		return fmt.Errorf("初始化默认配置失败: %v", err)
	}

	// 从数据库读取配置
	configs, err := GetAllSystemConfigs()
	if err != nil {
		return fmt.Errorf("从数据库读取配置失败: %v", err)
	}

	// 应用配置到AppConfig
	for _, config := range configs {
		switch config.Key {
		case "access_pwd":
			AppConfig.AccessPwd = config.Value
		case "auth_token":
			AppConfig.AuthToken = config.Value
		case "route_prefix":
			AppConfig.RoutePrefix = config.Value
		case "coding_mode":
			AppConfig.CodingMode = config.Value
		case "coding_token":
			AppConfig.CodingToken = config.Value
		case "tenant_url":
			AppConfig.TenantURL = config.Value
		case "proxy_url":
			AppConfig.ProxyURL = config.Value
		}
	}

	// 验证必要配置
	if AppConfig.AccessPwd == "" {
		logger.Log.Warn("未设置访问密码，请在管理面板中配置")
	}

	logger.Log.Info("Welcome to use Augment2Api! Current Version: " + version)
	logger.Log.Info("Augment2Api配置加载完成:\n" +
		"----------------------------------------\n" +
		"AuthToken:    " + maskString(AppConfig.AuthToken) + "\n" +
		"AccessPwd:    " + maskString(AppConfig.AccessPwd) + "\n" +
		"RedisConnString: " + maskString(AppConfig.RedisConnString) + "\n" +
		"RoutePrefix: " + AppConfig.RoutePrefix + "\n" +
		"ProxyURL: " + AppConfig.ProxyURL + "\n" +
		"----------------------------------------")

	logger.Log.Info("Everything is set up, now start to fully enjoy the charm of AI ！")

	return nil
}

// maskString 遮蔽敏感信息
func maskString(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}

// initializeDefaultConfigs 初始化默认配置
func initializeDefaultConfigs() error {
	defaultConfigs := []SystemConfig{
		{
			Key:         "access_pwd",
			Value:       "admin123",
			Description: "管理面板访问密码",
			Category:    "security",
			UpdatedAt:   time.Now(),
		},
		{
			Key:         "auth_token",
			Value:       "",
			Description: "API鉴权Token",
			Category:    "security",
			UpdatedAt:   time.Now(),
		},
		{
			Key:         "route_prefix",
			Value:       "",
			Description: "API路由前缀",
			Category:    "api",
			UpdatedAt:   time.Now(),
		},
		{
			Key:         "coding_mode",
			Value:       "false",
			Description: "开发调试模式",
			Category:    "development",
			UpdatedAt:   time.Now(),
		},
		{
			Key:         "coding_token",
			Value:       "",
			Description: "开发模式Token",
			Category:    "development",
			UpdatedAt:   time.Now(),
		},
		{
			Key:         "tenant_url",
			Value:       "",
			Description: "开发模式租户URL",
			Category:    "development",
			UpdatedAt:   time.Now(),
		},
		{
			Key:         "proxy_url",
			Value:       "",
			Description: "HTTP代理地址",
			Category:    "network",
			UpdatedAt:   time.Now(),
		},
	}

	for _, config := range defaultConfigs {
		// 检查配置是否已存在，如果存在则跳过
		_, err := GetSystemConfig(config.Key)
		if err == nil {
			// 配置已存在，跳过
			continue
		}

		// 配置不存在，创建默认配置
		err = SetSystemConfig(config.Key, config.Value, config.Description, config.Category)
		if err != nil {
			logger.Log.Error("初始化配置 %s 失败: %v", config.Key, err)
		} else {
			logger.Log.Info("初始化默认配置: %s", config.Key)
		}
	}

	return nil
}

// GetSystemConfig 获取系统配置
func GetSystemConfig(key string) (SystemConfig, error) {
	configKey := "system_config:" + key
	data, err := RedisGet(configKey)
	if err != nil {
		return SystemConfig{}, err
	}

	var config SystemConfig
	err = json.Unmarshal([]byte(data), &config)
	if err != nil {
		return SystemConfig{}, err
	}

	return config, nil
}

// SetSystemConfig 设置系统配置
func SetSystemConfig(key, value, description, category string) error {
	config := SystemConfig{
		Key:         key,
		Value:       value,
		Description: description,
		Category:    category,
		UpdatedAt:   time.Now(),
	}

	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	configKey := "system_config:" + key
	return RedisSet(configKey, string(data), 0) // 永不过期
}

// GetAllSystemConfigs 获取所有系统配置
func GetAllSystemConfigs() ([]SystemConfig, error) {
	keys, err := RedisKeys("system_config:*")
	if err != nil {
		return nil, err
	}

	var configs []SystemConfig
	for _, key := range keys {
		data, err := RedisGet(key)
		if err != nil {
			continue
		}

		var config SystemConfig
		err = json.Unmarshal([]byte(data), &config)
		if err != nil {
			continue
		}

		configs = append(configs, config)
	}

	return configs, nil
}

// DeleteSystemConfig 删除系统配置
func DeleteSystemConfig(key string) error {
	configKey := "system_config:" + key
	return RedisDel(configKey)
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
