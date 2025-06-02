package api

import (
	"augment2api/config"
	"augment2api/pkg/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GetSystemConfigs 获取所有系统配置
func GetSystemConfigs(c *gin.Context) {
	configs, err := config.GetAllSystemConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "获取系统配置失败: " + err.Error(),
		})
		return
	}

	// 按分类和键名排序
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].Category != configs[j].Category {
			return configs[i].Category < configs[j].Category
		}
		return configs[i].Key < configs[j].Key
	})

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"configs": configs,
	})
}

// UpdateSystemConfig 更新系统配置
func UpdateSystemConfig(c *gin.Context) {
	var req struct {
		Key         string `json:"key" binding:"required"`
		Value       string `json:"value"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求数据: " + err.Error(),
		})
		return
	}

	// 更新配置
	err := config.SetSystemConfig(req.Key, req.Value, req.Description, req.Category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "更新配置失败: " + err.Error(),
		})
		return
	}

	// 重新加载配置到内存
	err = config.LoadConfigFromDatabase()
	if err != nil {
		logger.Log.Error("重新加载配置失败: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
}

// DeleteSystemConfig 删除系统配置
func DeleteSystemConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "未指定配置键",
		})
		return
	}

	// 检查是否为必要配置
	requiredKeys := []string{"access_pwd", "auth_token", "route_prefix", "coding_mode"}
	for _, requiredKey := range requiredKeys {
		if key == requiredKey {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "不能删除必要的系统配置",
			})
			return
		}
	}

	err := config.DeleteSystemConfig(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "删除配置失败: " + err.Error(),
		})
		return
	}

	// 重新加载配置到内存
	err = config.LoadConfigFromDatabase()
	if err != nil {
		logger.Log.Error("重新加载配置失败: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
}

// GetDatabaseStats 获取数据库统计信息
func GetDatabaseStats(c *gin.Context) {
	stats := make(map[string]interface{})

	// 获取token数量
	tokenKeys, err := config.RedisKeys("token:*")
	if err == nil {
		stats["token_count"] = len(tokenKeys)
	}

	// 获取系统配置数量
	configKeys, err := config.RedisKeys("system_config:*")
	if err == nil {
		stats["config_count"] = len(configKeys)
	}

	// 获取每日使用数据数量
	dailyUsageKeys, err := config.RedisKeys("token_daily_usage:*")
	if err == nil {
		stats["daily_usage_count"] = len(dailyUsageKeys)
	}

	// 获取请求状态数据数量
	requestStatusKeys, err := config.RedisKeys("token_request_status:*")
	if err == nil {
		stats["request_status_count"] = len(requestStatusKeys)
	}

	// 获取所有键的数量
	allKeys, err := config.RedisKeys("*")
	if err == nil {
		stats["total_keys"] = len(allKeys)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"stats":  stats,
	})
}

// DatabaseDetail 数据库详细信息结构
type DatabaseDetail struct {
	Key         string      `json:"key"`
	Type        string      `json:"type"`
	Value       interface{} `json:"value"`
	Size        int64       `json:"size"`
	TTL         int64       `json:"ttl"`
	Category    string      `json:"category"`
	Description string      `json:"description"`
}

// GetDatabaseDetails 获取数据库详细信息
func GetDatabaseDetails(c *gin.Context) {
	category := c.Query("category") // tokens, configs, usage, status, all

	var keys []string
	var err error

	switch category {
	case "tokens":
		keys, err = config.RedisKeys("token:*")
	case "configs":
		keys, err = config.RedisKeys("system_config:*")
	case "usage":
		keys, err = config.RedisKeys("token_daily_usage:*")
	case "status":
		keys, err = config.RedisKeys("token_request_status:*")
	default:
		keys, err = config.RedisKeys("*")
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "获取数据库键失败: " + err.Error(),
		})
		return
	}

	details := make([]DatabaseDetail, 0, len(keys))

	for _, key := range keys {
		detail := DatabaseDetail{
			Key:      key,
			Category: getCategoryFromKey(key),
		}

		// 获取键的类型和值
		keyType, value, size, ttl := getKeyDetails(key)
		detail.Type = keyType
		detail.Value = value
		detail.Size = size
		detail.TTL = ttl
		detail.Description = getKeyDescription(key)

		details = append(details, detail)
	}

	// 按键名排序
	sort.Slice(details, func(i, j int) bool {
		return details[i].Key < details[j].Key
	})

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"details": details,
		"total":   len(details),
	})
}

// getCategoryFromKey 根据键名获取分类
func getCategoryFromKey(key string) string {
	if strings.HasPrefix(key, "token:") {
		return "tokens"
	} else if strings.HasPrefix(key, "system_config:") {
		return "configs"
	} else if strings.HasPrefix(key, "token_daily_usage:") {
		return "usage"
	} else if strings.HasPrefix(key, "token_request_status:") {
		return "status"
	} else if strings.HasPrefix(key, "token_usage") {
		return "usage_stats"
	}
	return "other"
}

// getKeyDescription 获取键的描述
func getKeyDescription(key string) string {
	descriptions := map[string]string{
		"token:":                "Token配置和状态信息",
		"system_config:":        "系统配置项",
		"token_daily_usage:":    "Token每日使用统计",
		"token_request_status:": "Token请求状态记录",
		"token_usage:":          "Token总使用次数",
		"token_usage_chat:":     "Token CHAT模式使用次数",
		"token_usage_agent:":    "Token AGENT模式使用次数",
	}

	for prefix, desc := range descriptions {
		if strings.HasPrefix(key, prefix) {
			return desc
		}
	}
	return "其他数据"
}

// getKeyDetails 获取键的详细信息
func getKeyDetails(key string) (keyType string, value interface{}, size int64, ttl int64) {
	// 获取键类型
	keyType = "string" // 默认类型

	// 获取值
	rawValue, err := config.RedisGet(key)
	if err != nil {
		value = "无法获取"
		return
	}

	// 尝试解析JSON
	var jsonValue interface{}
	if err := json.Unmarshal([]byte(rawValue), &jsonValue); err == nil {
		value = jsonValue
		keyType = "json"
	} else {
		value = rawValue
	}

	// 获取大小（字节数）
	size = int64(len(rawValue))

	// 获取TTL（暂时设为-1，表示永不过期）
	ttl = -1

	return
}

// ExportSystemConfig 导出系统配置
func ExportSystemConfig(c *gin.Context) {
	configs, err := config.GetAllSystemConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "获取配置失败: " + err.Error(),
		})
		return
	}

	// 设置下载头
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=system_config.json")

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"configs":   configs,
		"export_at": config.SystemConfig{UpdatedAt: time.Now()}.UpdatedAt,
	})
}

// ImportSystemConfig 导入系统配置
func ImportSystemConfig(c *gin.Context) {
	var req struct {
		Configs []config.SystemConfig `json:"configs" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求数据: " + err.Error(),
		})
		return
	}

	successCount := 0
	failedCount := 0

	for _, cfg := range req.Configs {
		err := config.SetSystemConfig(cfg.Key, cfg.Value, cfg.Description, cfg.Category)
		if err != nil {
			logger.Log.Error("导入配置 %s 失败: %v", cfg.Key, err)
			failedCount++
		} else {
			successCount++
		}
	}

	// 重新加载配置到内存
	err := config.LoadConfigFromDatabase()
	if err != nil {
		logger.Log.Error("重新加载配置失败: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"success_count": successCount,
		"failed_count":  failedCount,
		"total_count":   len(req.Configs),
	})
}

// ProxyTestRequest 代理测试请求结构
type ProxyTestRequest struct {
	ProxyURL string `json:"proxy_url" binding:"required"`
}

// ProxyTestResponse 代理测试响应结构
type ProxyTestResponse struct {
	Status      string                 `json:"status"`
	Success     bool                   `json:"success"`
	Message     string                 `json:"message"`
	ProxyURL    string                 `json:"proxy_url"`
	BackendURL  string                 `json:"backend_url"`
	TestResults map[string]interface{} `json:"test_results"`
	Timestamp   string                 `json:"timestamp"`
}

// TestProxy 测试代理连接
func TestProxy(c *gin.Context) {
	var req ProxyTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求数据: " + err.Error(),
		})
		return
	}

	// 执行代理测试
	testResult := performProxyTest(req.ProxyURL)

	c.JSON(http.StatusOK, testResult)
}

// performProxyTest 执行代理测试
func performProxyTest(proxyURL string) ProxyTestResponse {
	result := ProxyTestResponse{
		ProxyURL:    proxyURL,
		BackendURL:  "https://linjinpeng-augment.hf.space",
		TestResults: make(map[string]interface{}),
		Timestamp:   time.Now().Format("2006-01-02 15:04:05"),
	}

	// 测试1: 健康检查端点
	logger.Log.Info("开始测试代理健康检查端点: %s/health", proxyURL)
	healthTest := testProxyEndpoint(proxyURL + "/health")
	result.TestResults["health_check"] = healthTest

	// 测试2: 模型列表端点
	logger.Log.Info("开始测试代理模型列表端点: %s/v1/models", proxyURL)
	modelsTest := testProxyEndpoint(proxyURL + "/v1/models")
	result.TestResults["models_endpoint"] = modelsTest

	// 测试3: 根路径安全检查（应该返回403）
	logger.Log.Info("开始测试代理安全检查: %s", proxyURL)
	rootTest := testProxyEndpoint(proxyURL)
	rootTest["expected_403"] = true
	if statusCode, ok := rootTest["status_code"].(int); ok && statusCode == 403 {
		rootTest["success"] = true
		rootTest["message"] = "安全检查通过：根路径正确返回403"
	}
	result.TestResults["security_check"] = rootTest

	// 综合评估
	healthSuccess, _ := healthTest["success"].(bool)
	modelsSuccess, _ := modelsTest["success"].(bool)
	securitySuccess, _ := rootTest["success"].(bool)

	if healthSuccess && modelsSuccess && securitySuccess {
		result.Status = "success"
		result.Success = true
		result.Message = "🎉 代理测试全部通过！代理工作正常，可以有效避免IP风控。"
		logger.Log.Info("代理测试成功: %s", proxyURL)
	} else {
		result.Status = "error"
		result.Success = false
		result.Message = "❌ 代理测试失败，请检查代理配置和部署状态。"
		logger.Log.Error("代理测试失败: %s", proxyURL)
	}

	return result
}

// testProxyEndpoint 测试代理端点
func testProxyEndpoint(url string) map[string]interface{} {
	result := map[string]interface{}{
		"url":         url,
		"success":     false,
		"status_code": 0,
		"message":     "",
		"response":    "",
		"error":       "",
		"duration":    "",
	}

	// 记录开始时间
	startTime := time.Now()

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// 发送请求
	resp, err := client.Get(url)
	duration := time.Since(startTime)
	result["duration"] = fmt.Sprintf("%.2fms", float64(duration.Nanoseconds())/1e6)

	if err != nil {
		result["error"] = err.Error()
		result["message"] = "请求失败: " + err.Error()
		logger.Log.Error("代理端点测试失败 %s: %v", url, err)
		return result
	}
	defer resp.Body.Close()

	result["status_code"] = resp.StatusCode

	// 读取响应体（限制大小）
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		result["error"] = "读取响应失败: " + err.Error()
		return result
	}

	responseText := string(body)
	result["response"] = responseText

	// 判断成功条件
	if resp.StatusCode == 200 {
		result["success"] = true
		result["message"] = "✅ 请求成功"

		// 特殊检查：健康检查端点应该包含特定内容
		if strings.Contains(url, "/health") {
			if strings.Contains(responseText, "healthy") && strings.Contains(responseText, "proxy_target") {
				result["message"] = "✅ 健康检查通过，代理目标配置正确"
			} else {
				result["success"] = false
				result["message"] = "❌ 健康检查响应格式不正确"
			}
		}

		// 特殊检查：模型端点应该包含模型列表
		if strings.Contains(url, "/v1/models") {
			if strings.Contains(responseText, "claude") || strings.Contains(responseText, "augment") {
				result["message"] = "✅ 模型列表获取成功，API代理正常"
			} else {
				result["success"] = false
				result["message"] = "❌ 模型列表响应格式不正确"
			}
		}

	} else if resp.StatusCode == 403 && !strings.Contains(url, "/health") && !strings.Contains(url, "/v1/") {
		// 根路径返回403是正常的安全行为
		result["success"] = true
		result["message"] = "✅ 安全检查通过：根路径正确返回403"
	} else {
		message := fmt.Sprintf("❌ HTTP状态码: %d", resp.StatusCode)
		if resp.StatusCode >= 500 {
			message += " (服务器错误)"
		} else if resp.StatusCode >= 400 {
			message += " (客户端错误)"
		}
		result["message"] = message
	}

	return result
}
