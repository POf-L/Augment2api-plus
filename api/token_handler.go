package api

import (
	"augment2api/config"
	"augment2api/pkg/logger"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// TokenInfo 存储token信息
type TokenInfo struct {
	Token           string    `json:"token"`
	TenantURL       string    `json:"tenant_url"`
	UsageCount      int       `json:"usage_count"`        // 总对话次数
	ChatUsageCount  int       `json:"chat_usage_count"`   // CHAT模式对话次数
	AgentUsageCount int       `json:"agent_usage_count"`  // AGENT模式对话次数
	Remark          string    `json:"remark"`             // 备注字段
	InCool          bool      `json:"in_cool"`            // 是否在冷却中
	CoolEnd         time.Time `json:"cool_end,omitempty"` // 冷却结束时间
	// 新增字段
	Enabled         bool      `json:"enabled"`            // Token 是否启用
	RequestInterval int       `json:"request_interval"`   // 请求间隔（秒）
	ChatLimit       int       `json:"chat_limit"`         // CHAT模式调用上限
	AgentLimit      int       `json:"agent_limit"`        // AGENT模式调用上限
	DailyLimit      int       `json:"daily_limit"`        // 每日总调用上限
	DailyUsage      int       `json:"daily_usage"`        // 今日已使用次数
}

// TokenItem token项结构
type TokenItem struct {
	Token     string `json:"token"`
	TenantUrl string `json:"tenantUrl"`
}

// TokenRequestStatus 记录 token 请求状态
type TokenRequestStatus struct {
	InProgress    bool      `json:"in_progress"`
	LastRequestAt time.Time `json:"last_request_at"`
}

// TokenCoolStatus 记录 token 冷却状态
type TokenCoolStatus struct {
	InCool  bool      `json:"in_cool"`
	CoolEnd time.Time `json:"cool_end"`
}

// GetRedisTokenHandler 从Redis获取token列表，支持分页
func GetRedisTokenHandler(c *gin.Context) {
	// 获取分页参数（可选）
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("page_size", "0") // 0表示不分页，返回所有

	pageNum, _ := strconv.Atoi(page)
	pageSizeNum, _ := strconv.Atoi(pageSize)

	if pageNum < 1 {
		pageNum = 1
	}

	// 获取所有token的key (使用通配符模式)
	keys, err := config.RedisKeys("token:*")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "error",
			"error":  "获取token列表失败: " + err.Error(),
		})
		return
	}

	// 如果没有token
	if len(keys) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":      "success",
			"tokens":      []TokenInfo{},
			"total":       0,
			"page":        pageNum,
			"page_size":   pageSizeNum,
			"total_pages": 0,
		})
		return
	}

	// 对keys进行排序，确保顺序稳定
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	// 使用并发方式批量获取token信息
	var wg sync.WaitGroup
	tokenList := make([]TokenInfo, 0, len(keys))
	tokenListChan := make(chan TokenInfo, len(keys))
	concurrencyLimit := 10 // 限制并发数
	sem := make(chan struct{}, concurrencyLimit)

	for _, key := range keys {
		// 从key中提取token (格式: "token:{token}")
		token := key[6:] // 去掉前缀 "token:"

		wg.Add(1)
		sem <- struct{}{} // 获取信号量

		go func(tokenKey string, tokenValue string) {
			defer wg.Done()
			defer func() { <-sem }() // 释放信号量

			// 使用HGETALL一次性获取所有字段，减少网络往返
			fields, err := config.RedisHGetAll(tokenKey)
			if err != nil {
				return // 跳过无效的token
			}

			// 检查必要字段
			tenantURL, ok := fields["tenant_url"]
			if !ok {
				return
			}

			// 检查token状态
			status, ok := fields["status"]
			if ok && status == "disabled" {
				return // 跳过被标记为不可用的token
			}

			// 获取备注信息
			remark := fields["remark"]

			// 获取token的冷却状态 (异步获取)
			coolStatus, _ := GetTokenCoolStatus(tokenValue)

			// 获取使用次数 (可以考虑将这些计数缓存在Redis中)
			chatCount := getTokenChatUsageCount(tokenValue)
			agentCount := getTokenAgentUsageCount(tokenValue)
			totalCount := chatCount + agentCount

			// 获取新增字段
			enabled := getTokenEnabled(tokenValue)
			requestInterval := getTokenRequestInterval(tokenValue)
			chatLimit := getTokenChatLimit(tokenValue)
			agentLimit := getTokenAgentLimit(tokenValue)
			dailyLimit := getTokenDailyLimit(tokenValue)
			dailyUsage := getTokenDailyUsage(tokenValue)

			// 构建token信息并发送到channel
			tokenListChan <- TokenInfo{
				Token:           tokenValue,
				TenantURL:       tenantURL,
				UsageCount:      totalCount,
				ChatUsageCount:  chatCount,
				AgentUsageCount: agentCount,
				Remark:          remark,
				InCool:          coolStatus.InCool,
				CoolEnd:         coolStatus.CoolEnd,
				Enabled:         enabled,
				RequestInterval: requestInterval,
				ChatLimit:       chatLimit,
				AgentLimit:      agentLimit,
				DailyLimit:      dailyLimit,
				DailyUsage:      dailyUsage,
			}
		}(key, token)
	}

	// 启动一个goroutine来收集结果
	go func() {
		wg.Wait()
		close(tokenListChan)
	}()

	// 从channel中收集结果
	for info := range tokenListChan {
		tokenList = append(tokenList, info)
	}

	// 对token列表按照token字符串进行排序，确保每次刷新结果顺序一致
	sort.Slice(tokenList, func(i, j int) bool {
		return tokenList[i].Token > tokenList[j].Token // 降序排序
	})

	// 计算总页数和分页数据
	totalItems := len(tokenList)
	totalPages := 1

	// 如果需要分页
	if pageSizeNum > 0 {
		totalPages = (totalItems + pageSizeNum - 1) / pageSizeNum

		// 确保页码有效
		if pageNum > totalPages && totalPages > 0 {
			pageNum = totalPages
		}

		// 计算分页的起始和结束索引
		startIndex := (pageNum - 1) * pageSizeNum
		endIndex := startIndex + pageSizeNum

		if startIndex < totalItems {
			if endIndex > totalItems {
				endIndex = totalItems
			}
			tokenList = tokenList[startIndex:endIndex]
		} else {
			tokenList = []TokenInfo{}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"tokens":      tokenList,
		"total":       totalItems,
		"page":        pageNum,
		"page_size":   pageSizeNum,
		"total_pages": totalPages,
	})
}

// SaveTokenToRedis 保存token到Redis
func SaveTokenToRedis(token, tenantURL string) error {
	// 创建一个唯一的key，包含token和tenant_url
	tokenKey := "token:" + token

	// token已存在，则跳过
	exists, err := config.RedisExists(tokenKey)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// 将tenant_url存储在token对应的哈希表中
	err = config.RedisHSet(tokenKey, "tenant_url", tenantURL)
	if err != nil {
		return err
	}

	// 默认将新添加的token标记为活跃状态
	err = config.RedisHSet(tokenKey, "status", "active")
	if err != nil {
		return err
	}

	// 初始化备注为空字符串
	err = config.RedisHSet(tokenKey, "remark", "")
	if err != nil {
		return err
	}

	// 初始化新增字段的默认值
	err = config.RedisHSet(tokenKey, "enabled", "true")
	if err != nil {
		return err
	}

	err = config.RedisHSet(tokenKey, "request_interval", "3")
	if err != nil {
		return err
	}

	err = config.RedisHSet(tokenKey, "chat_limit", "3000")
	if err != nil {
		return err
	}

	err = config.RedisHSet(tokenKey, "agent_limit", "50")
	if err != nil {
		return err
	}

	return config.RedisHSet(tokenKey, "daily_limit", "1000")
}

// DeleteTokenHandler 删除指定的token
func DeleteTokenHandler(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "未指定token",
		})
		return
	}

	tokenKey := "token:" + token

	// 检查token是否存在
	exists, err := config.RedisExists(tokenKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "检查token失败: " + err.Error(),
		})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "token不存在",
		})
		return
	}

	// 删除token
	if err := config.RedisDel(tokenKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "删除token失败: " + err.Error(),
		})
		return
	}

	// 删除token关联的使用次数（如果存在）
	// 删除总使用次数
	tokenUsageKey := "token_usage:" + token
	exists, err = config.RedisExists(tokenUsageKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "检查token使用次数失败: " + err.Error(),
		})
		return
	}
	if exists {
		if err := config.RedisDel(tokenUsageKey); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "删除token使用次数失败: " + err.Error(),
			})
		}
	}

	// 删除CHAT模式使用次数
	tokenChatUsageKey := "token_usage_chat:" + token
	exists, err = config.RedisExists(tokenChatUsageKey)
	if err == nil && exists {
		config.RedisDel(tokenChatUsageKey)
	}

	// 删除AGENT模式使用次数
	tokenAgentUsageKey := "token_usage_agent:" + token
	exists, err = config.RedisExists(tokenAgentUsageKey)
	if err == nil && exists {
		config.RedisDel(tokenAgentUsageKey)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
}

// AddTokenHandler 批量添加token到Redis
func AddTokenHandler(c *gin.Context) {
	var tokens []TokenItem
	if err := c.ShouldBindJSON(&tokens); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求数据",
		})
		return
	}

	// 检查是否有token数据
	if len(tokens) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "token列表为空",
		})
		return
	}

	// 批量保存token
	successCount := 0
	failedTokens := make([]string, 0)

	for _, item := range tokens {
		// 验证token格式
		if item.Token == "" || item.TenantUrl == "" {
			failedTokens = append(failedTokens, item.Token)
			continue
		}

		// 保存到Redis
		err := SaveTokenToRedis(item.Token, item.TenantUrl)
		if err != nil {
			failedTokens = append(failedTokens, item.Token)
			continue
		}
		successCount++
	}

	// 返回处理结果
	result := gin.H{
		"status":        "success",
		"total":         len(tokens),
		"success_count": successCount,
	}

	if len(failedTokens) > 0 {
		result["failed_tokens"] = failedTokens
		result["failed_count"] = len(failedTokens)
	}

	c.JSON(http.StatusOK, result)
}

// CheckTokenTenantURL 检测token的租户地址
func CheckTokenTenantURL(token string) (string, error) {
	// 构建测试消息
	testMsg := map[string]interface{}{
		"message":              "hello，what is your name",
		"mode":                 "CHAT",
		"prefix":               "You are AI assistant,help me to solve problems!",
		"suffix":               " ",
		"lang":                 "HTML",
		"user_guidelines":      "You are a helpful assistant, you can help me to solve problems and always answer in Chinese.",
		"workspace_guidelines": "",
		"feature_detection_flags": map[string]interface{}{
			"support_raw_output": true,
		},
		"tool_definitions": []map[string]interface{}{},
		"blobs": map[string]interface{}{
			"checkpoint_id": nil,
			"added_blobs":   []string{},
			"deleted_blobs": []string{},
		},
	}

	jsonData, err := json.Marshal(testMsg)
	if err != nil {
		return "", fmt.Errorf("序列化测试消息失败: %v", err)
	}

	tokenKey := "token:" + token

	currentTenantURL, err := config.RedisHGet(tokenKey, "tenant_url")

	var tenantURLResult string
	var foundValid bool
	var tenantURLsToTest []string

	// 如果Redis中有有效的租户地址，优先测试该地址
	if err == nil && currentTenantURL != "" {
		tenantURLsToTest = append(tenantURLsToTest, currentTenantURL)
	}

	// 创建一个map来跟踪已添加的URL，避免重复
	uniqueTenantURLs := make(map[string]bool)
	if currentTenantURL != "" {
		uniqueTenantURLs[currentTenantURL] = true
	}

	// 添加其他租户地址
	// 添加 d1-d20 地址
	for i := 20; i >= 0; i-- {
		newTenantURL := fmt.Sprintf("https://d%d.api.augmentcode.com/", i)
		// 避免重复测试已有的租户地址
		if !uniqueTenantURLs[newTenantURL] {
			tenantURLsToTest = append(tenantURLsToTest, newTenantURL)
			uniqueTenantURLs[newTenantURL] = true
		}
	}

	// 添加 i0-i5 地址
	for i := 5; i >= 0; i-- {
		newTenantURL := fmt.Sprintf("https://i%d.api.augmentcode.com/", i)
		if !uniqueTenantURLs[newTenantURL] {
			tenantURLsToTest = append(tenantURLsToTest, newTenantURL)
			uniqueTenantURLs[newTenantURL] = true
		}
	}

	// 测试租户地址
	for _, tenantURL := range tenantURLsToTest {
		// 创建请求
		req, err := http.NewRequest("POST", tenantURL+"chat-stream", bytes.NewReader(jsonData))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "augment.intellij/0.160.0 (Mac OS X; aarch64; 15.2) WebStorm/2024.3.5")
		req.Header.Set("x-api-version", "2")
		req.Header.Set("x-request-id", uuid.New().String())
		req.Header.Set("x-request-session-id", uuid.New().String())

		client := createHTTPClient()
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("请求失败: %v\n", err)
			continue
		}

		isInvalid := false
		func() {
			defer resp.Body.Close()

			// 检查是否返回401状态码（未授权）
			if resp.StatusCode == http.StatusUnauthorized {
				// 读取响应体内容
				buf := make([]byte, 1024)
				n, readErr := resp.Body.Read(buf)
				responseBody := ""
				if readErr == nil && n > 0 {
					responseBody = string(buf[:n])
				}

				// 只有当响应中包含"Invalid token"时才标记为不可用
				if readErr == nil && n > 0 && bytes.Contains(buf[:n], []byte("Invalid token")) {
					// 将token标记为不可用
					err = config.RedisHSet(tokenKey, "status", "disabled")
					if err != nil {
						fmt.Printf("标记token为不可用失败: %v\n", err)
					}
					logger.Log.WithFields(logrus.Fields{
						"token":         token,
						"response_body": responseBody,
					}).Info("token: 已被标记为不可用,返回401未授权")
					isInvalid = true
				}
				return
			}

			// 检查响应状态
			if resp.StatusCode == http.StatusOK {
				// 尝试读取一小部分响应以确认是否有效
				buf := make([]byte, 1024)
				n, err := resp.Body.Read(buf)
				if err == nil && n > 0 {
					// 更新Redis中的租户地址和状态
					err = config.RedisHSet(tokenKey, "tenant_url", tenantURL)
					if err != nil {
						return
					}
					// 将token标记为可用
					err = config.RedisHSet(tokenKey, "status", "active")
					if err != nil {
						fmt.Printf("标记token为可用失败: %v\n", err)
					}
					logger.Log.WithFields(logrus.Fields{
						"token":          token,
						"new_tenant_url": tenantURL,
					}).Info("token: 更新租户地址成功")
					tenantURLResult = tenantURL
					foundValid = true
				}
			}
		}()

		// 如果token无效，立即返回错误，不再测试其他地址
		if isInvalid {
			return "", fmt.Errorf("token被标记为不可用")
		}

		// 如果找到有效的租户地址，跳出循环
		if foundValid {
			return tenantURLResult, nil
		}
	}

	return "", fmt.Errorf("未找到有效的租户地址")
}

// CheckAllTokensHandler 批量检测所有token的租户地址
func CheckAllTokensHandler(c *gin.Context) {
	// 获取所有token的key
	keys, err := config.RedisKeys("token:*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "获取token列表失败: " + err.Error(),
		})
		return
	}

	if len(keys) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":   "success",
			"total":    0,
			"updated":  0,
			"disabled": 0,
		})
		return
	}

	var wg sync.WaitGroup
	// 使用互斥锁保护计数器
	var mu sync.Mutex
	var updatedCount int
	var disabledCount int
	var validTokenCount int

	for _, key := range keys {
		// 获取token状态，跳过已标记为不可用的token
		status, err := config.RedisHGet(key, "status")
		if err == nil && status == "disabled" {
			continue // 跳过此token
		}

		// 计算有效token数量
		mu.Lock()
		validTokenCount++
		mu.Unlock()

		wg.Add(1)
		go func(key string) {
			defer wg.Done()

			// 从key中提取token
			token := key[6:] // 去掉前缀 "token:"

			// 获取当前的租户地址
			oldTenantURL, _ := config.RedisHGet(key, "tenant_url")

			// 检测租户地址
			newTenantURL, err := CheckTokenTenantURL(token)
			logger.Log.WithFields(logrus.Fields{
				"token":          token,
				"old_tenant_url": oldTenantURL,
				"new_tenant_url": newTenantURL,
			}).Info("检测token租户地址")

			mu.Lock()
			if err != nil && err.Error() == "token被标记为不可用" {
				disabledCount++
			} else if err == nil && newTenantURL != oldTenantURL {
				updatedCount++
			}
			mu.Unlock()
		}(key)
	}

	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"total":    validTokenCount,
		"updated":  updatedCount,
		"disabled": disabledCount,
	})
}

// SetTokenRequestStatus 设置token请求状态
func SetTokenRequestStatus(token string, status TokenRequestStatus) error {
	// 使用Redis存储token请求状态
	key := "token_status:" + token

	// 将状态转换为JSON
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return err
	}

	// 存储到Redis，设置过期时间为1小时
	return config.RedisSet(key, string(statusJSON), time.Hour)
}

// GetTokenRequestStatus 获取token请求状态
func GetTokenRequestStatus(token string) (TokenRequestStatus, error) {
	key := "token_status:" + token

	// 从Redis获取状态
	statusJSON, err := config.RedisGet(key)
	if err != nil {
		// 如果key不存在，返回默认状态
		if errors.Is(err, redis.Nil) {
			return TokenRequestStatus{
				InProgress:    false,
				LastRequestAt: time.Time{}, // 零值时间
			}, nil
		}
		return TokenRequestStatus{}, err
	}

	// 解析JSON
	var status TokenRequestStatus
	if err := json.Unmarshal([]byte(statusJSON), &status); err != nil {
		return TokenRequestStatus{}, err
	}

	return status, nil
}

// SetTokenCoolStatus 将token加入冷却队列
func SetTokenCoolStatus(token string, duration time.Duration) error {
	// 使用Redis存储token冷却状态
	key := "token_cool_status:" + token

	coolStatus := TokenCoolStatus{
		InCool:  true,
		CoolEnd: time.Now().Add(duration),
	}

	// 将状态转换为JSON
	coolStatusJSON, err := json.Marshal(coolStatus)
	if err != nil {
		return err
	}

	// 存储到Redis，设置过期时间与冷却时间相同
	return config.RedisSet(key, string(coolStatusJSON), duration)
}

// GetTokenCoolStatus 获取token冷却状态
func GetTokenCoolStatus(token string) (TokenCoolStatus, error) {
	key := "token_cool_status:" + token

	// 从Redis获取状态
	coolStatusJSON, err := config.RedisGet(key)
	if err != nil {
		// 如果key不存在，返回默认状态
		if errors.Is(err, redis.Nil) {
			return TokenCoolStatus{
				InCool:  false,
				CoolEnd: time.Time{}, // 零值时间
			}, nil
		}
		return TokenCoolStatus{}, err
	}

	// 解析JSON
	var coolStatus TokenCoolStatus
	if err := json.Unmarshal([]byte(coolStatusJSON), &coolStatus); err != nil {
		return TokenCoolStatus{}, err
	}

	// 检查冷却时间是否已过
	if time.Now().After(coolStatus.CoolEnd) {
		coolStatus.InCool = false
	}

	return coolStatus, nil
}

// GetAvailableToken 获取一个可用的token（未在使用中且冷却时间已过）
func GetAvailableToken() (string, string) {
	// 获取所有token的key
	keys, err := config.RedisKeys("token:*")
	if err != nil || len(keys) == 0 {
		return "No token", ""
	}

	// 筛选可用的token
	var availableTokens []string
	var availableTenantURLs []string
	var cooldownTokens []string
	var cooldownTenantURLs []string

	for _, key := range keys {
		// 获取token状态
		status, err := config.RedisHGet(key, "status")
		if err == nil && status == "disabled" {
			continue // 跳过被标记为不可用的token
		}

		// 从key中提取token
		token := key[6:] // 去掉前缀 "token:"

		// 检查token是否启用
		if !getTokenEnabled(token) {
			continue // 跳过被禁用的token
		}

		// 获取token的请求状态
		requestStatus, err := GetTokenRequestStatus(token)
		if err != nil {
			continue
		}

		// 如果token正在使用中，跳过
		if requestStatus.InProgress {
			continue
		}

		// 获取token的独立请求间隔
		requestInterval := getTokenRequestInterval(token)
		// 如果距离上次请求时间不足设定的间隔，跳过
		if time.Since(requestStatus.LastRequestAt) < time.Duration(requestInterval)*time.Second {
			continue
		}

		// 检查CHAT模式和AGENT模式的使用次数限制
		chatUsageCount := getTokenChatUsageCount(token)
		agentUsageCount := getTokenAgentUsageCount(token)

		// 获取token的独立限制
		chatLimit := getTokenChatLimit(token)
		agentLimit := getTokenAgentLimit(token)
		dailyLimit := getTokenDailyLimit(token)
		dailyUsage := getTokenDailyUsage(token)

		// 如果CHAT模式已达到限制，跳过
		if chatUsageCount >= chatLimit {
			continue
		}

		// 如果AGENT模式已达到限制，跳过
		if agentUsageCount >= agentLimit {
			continue
		}

		// 如果每日使用已达到限制，跳过
		if dailyUsage >= dailyLimit {
			continue
		}

		// 获取对应的tenant_url
		tenantURL, err := config.RedisHGet(key, "tenant_url")
		if err != nil {
			continue
		}

		// 检查token是否在冷却中
		coolStatus, err := GetTokenCoolStatus(token)
		if err != nil {
			continue
		}

		// 如果token在冷却中，放入冷却队列
		if coolStatus.InCool {
			cooldownTokens = append(cooldownTokens, token)
			cooldownTenantURLs = append(cooldownTenantURLs, tenantURL)
		} else {
			// 否则放入可用队列
			availableTokens = append(availableTokens, token)
			availableTenantURLs = append(availableTenantURLs, tenantURL)
		}
	}

	// 优先从可用队列中选择token
	if len(availableTokens) > 0 {
		// 随机选择一个token
		randomIndex := rand.Intn(len(availableTokens))
		return availableTokens[randomIndex], availableTenantURLs[randomIndex]
	}

	// 如果没有非冷却token可用，则从冷却队列中选择
	if len(cooldownTokens) > 0 {
		// 随机选择一个token
		randomIndex := rand.Intn(len(cooldownTokens))
		return cooldownTokens[randomIndex], cooldownTenantURLs[randomIndex]
	}

	// 如果没有任何可用的token
	return "No available token", ""
}

// getTokenUsageCount 获取token的使用次数
func getTokenUsageCount(token string) int {
	// 使用Redis中的计数器获取使用次数
	countKey := "token_usage:" + token
	count, err := config.RedisGet(countKey)
	if err != nil {
		return 0 // 如果出错或不存在，返回0
	}

	// 将字符串转换为整数
	countInt, err := strconv.Atoi(count)
	if err != nil {
		return 0
	}

	return countInt
}

// getTokenChatUsageCount 获取token的CHAT模式使用次数
func getTokenChatUsageCount(token string) int {
	// 使用Redis中的计数器获取使用次数
	countKey := "token_usage_chat:" + token
	count, err := config.RedisGet(countKey)
	if err != nil {
		return 0 // 如果出错或不存在，返回0
	}

	// 将字符串转换为整数
	countInt, err := strconv.Atoi(count)
	if err != nil {
		return 0
	}

	return countInt
}

// getTokenAgentUsageCount 获取token的AGENT模式使用次数
func getTokenAgentUsageCount(token string) int {
	// 使用Redis中的计数器获取使用次数
	countKey := "token_usage_agent:" + token
	count, err := config.RedisGet(countKey)
	if err != nil {
		return 0 // 如果出错或不存在，返回0
	}

	// 将字符串转换为整数
	countInt, err := strconv.Atoi(count)
	if err != nil {
		return 0
	}

	return countInt
}

// UpdateTokenRemark 更新token的备注信息
func UpdateTokenRemark(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "未指定token",
		})
		return
	}

	var req struct {
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求数据",
		})
		return
	}

	tokenKey := "token:" + token

	// 检查token是否存在
	exists, err := config.RedisExists(tokenKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "检查token失败: " + err.Error(),
		})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "token不存在",
		})
		return
	}

	// 更新备注
	err = config.RedisHSet(tokenKey, "remark", req.Remark)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "更新备注失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
}

// MigrateTokensRemark 确保所有token都有remark字段
func MigrateTokensRemark() error {
	// 获取所有token的key
	keys, err := config.RedisKeys("token:*")
	if err != nil {
		return fmt.Errorf("获取token列表失败: %v", err)
	}

	for _, key := range keys {
		// 检查是否已有remark字段
		exists, err := config.RedisHExists(key, "remark")
		if err != nil {
			logger.Log.Error("check remark field of token %s failed: %v", key, err)
			continue
		}

		// 如果没有remark字段，添加一个空的remark
		if !exists {
			err = config.RedisHSet(key, "remark", "")
			if err != nil {
				logger.Log.Error("add remark field to token %s failed: %v", key, err)
				continue
			}
			logger.Log.Info("add remark field to token %s success", key)
		}
	}
	logger.Log.Info("migrate remark field to all tokens success!")

	return nil
}

// getTokenEnabled 获取token的启用状态
func getTokenEnabled(token string) bool {
	tokenKey := "token:" + token
	enabled, err := config.RedisHGet(tokenKey, "enabled")
	if err != nil {
		return true // 默认启用
	}
	return enabled == "true"
}

// getTokenRequestInterval 获取token的请求间隔
func getTokenRequestInterval(token string) int {
	tokenKey := "token:" + token
	interval, err := config.RedisHGet(tokenKey, "request_interval")
	if err != nil {
		return 3 // 默认3秒
	}
	intervalInt, err := strconv.Atoi(interval)
	if err != nil {
		return 3
	}
	return intervalInt
}

// getTokenChatLimit 获取token的CHAT模式调用上限
func getTokenChatLimit(token string) int {
	tokenKey := "token:" + token
	limit, err := config.RedisHGet(tokenKey, "chat_limit")
	if err != nil {
		return 3000 // 默认3000次
	}
	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		return 3000
	}
	return limitInt
}

// getTokenAgentLimit 获取token的AGENT模式调用上限
func getTokenAgentLimit(token string) int {
	tokenKey := "token:" + token
	limit, err := config.RedisHGet(tokenKey, "agent_limit")
	if err != nil {
		return 50 // 默认50次
	}
	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		return 50
	}
	return limitInt
}

// getTokenDailyLimit 获取token的每日调用上限
func getTokenDailyLimit(token string) int {
	tokenKey := "token:" + token
	limit, err := config.RedisHGet(tokenKey, "daily_limit")
	if err != nil {
		return 1000 // 默认1000次
	}
	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		return 1000
	}
	return limitInt
}

// getTokenDailyUsage 获取token的今日使用次数
func getTokenDailyUsage(token string) int {
	today := time.Now().Format("2006-01-02")
	countKey := "token_daily_usage:" + token + ":" + today
	count, err := config.RedisGet(countKey)
	if err != nil {
		return 0
	}
	countInt, err := strconv.Atoi(count)
	if err != nil {
		return 0
	}
	return countInt
}

// IncrementTokenDailyUsage 增加token的每日使用计数
func IncrementTokenDailyUsage(token string) error {
	today := time.Now().Format("2006-01-02")
	countKey := "token_daily_usage:" + token + ":" + today

	// 增加计数，如果key不存在则创建并设置为1
	err := config.RedisIncr(countKey)
	if err != nil {
		return err
	}

	// 设置过期时间为明天凌晨（自动清理）
	tomorrow := time.Now().AddDate(0, 0, 1)
	midnight := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
	duration := midnight.Sub(time.Now())

	return config.RedisExpire(countKey, duration)
}

// UpdateTokenStatus 更新token的启用/禁用状态
func UpdateTokenStatus(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "未指定token",
		})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求数据",
		})
		return
	}

	tokenKey := "token:" + token

	// 检查token是否存在
	exists, err := config.RedisExists(tokenKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "检查token失败: " + err.Error(),
		})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "token不存在",
		})
		return
	}

	// 更新启用状态
	enabledStr := "false"
	if req.Enabled {
		enabledStr = "true"
	}
	err = config.RedisHSet(tokenKey, "enabled", enabledStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "更新token状态失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
}

// UpdateTokenLimits 更新token的限制设置
func UpdateTokenLimits(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "未指定token",
		})
		return
	}

	var req struct {
		RequestInterval int `json:"request_interval"`
		ChatLimit       int `json:"chat_limit"`
		AgentLimit      int `json:"agent_limit"`
		DailyLimit      int `json:"daily_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求数据",
		})
		return
	}

	tokenKey := "token:" + token

	// 检查token是否存在
	exists, err := config.RedisExists(tokenKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "检查token失败: " + err.Error(),
		})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "token不存在",
		})
		return
	}

	// 验证参数范围
	if req.RequestInterval < 1 || req.RequestInterval > 3600 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "请求间隔必须在1-3600秒之间",
		})
		return
	}

	if req.ChatLimit < 0 || req.AgentLimit < 0 || req.DailyLimit < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "限制值不能为负数",
		})
		return
	}

	// 更新各项限制
	err = config.RedisHSet(tokenKey, "request_interval", strconv.Itoa(req.RequestInterval))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "更新请求间隔失败: " + err.Error(),
		})
		return
	}

	err = config.RedisHSet(tokenKey, "chat_limit", strconv.Itoa(req.ChatLimit))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "更新CHAT限制失败: " + err.Error(),
		})
		return
	}

	err = config.RedisHSet(tokenKey, "agent_limit", strconv.Itoa(req.AgentLimit))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "更新AGENT限制失败: " + err.Error(),
		})
		return
	}

	err = config.RedisHSet(tokenKey, "daily_limit", strconv.Itoa(req.DailyLimit))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "更新每日限制失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
}

// MigrateTokensNewFields 为现有token添加新字段
func MigrateTokensNewFields() error {
	// 获取所有token的key
	keys, err := config.RedisKeys("token:*")
	if err != nil {
		return fmt.Errorf("获取token列表失败: %v", err)
	}

	for _, key := range keys {
		// 检查并添加enabled字段
		exists, err := config.RedisHExists(key, "enabled")
		if err != nil {
			logger.Log.Error("检查token %s的enabled字段失败: %v", key, err)
			continue
		}
		if !exists {
			err = config.RedisHSet(key, "enabled", "true")
			if err != nil {
				logger.Log.Error("为token %s添加enabled字段失败: %v", key, err)
				continue
			}
		}

		// 检查并添加request_interval字段
		exists, err = config.RedisHExists(key, "request_interval")
		if err != nil {
			logger.Log.Error("检查token %s的request_interval字段失败: %v", key, err)
			continue
		}
		if !exists {
			err = config.RedisHSet(key, "request_interval", "3")
			if err != nil {
				logger.Log.Error("为token %s添加request_interval字段失败: %v", key, err)
				continue
			}
		}

		// 检查并添加chat_limit字段
		exists, err = config.RedisHExists(key, "chat_limit")
		if err != nil {
			logger.Log.Error("检查token %s的chat_limit字段失败: %v", key, err)
			continue
		}
		if !exists {
			err = config.RedisHSet(key, "chat_limit", "3000")
			if err != nil {
				logger.Log.Error("为token %s添加chat_limit字段失败: %v", key, err)
				continue
			}
		}

		// 检查并添加agent_limit字段
		exists, err = config.RedisHExists(key, "agent_limit")
		if err != nil {
			logger.Log.Error("检查token %s的agent_limit字段失败: %v", key, err)
			continue
		}
		if !exists {
			err = config.RedisHSet(key, "agent_limit", "50")
			if err != nil {
				logger.Log.Error("为token %s添加agent_limit字段失败: %v", key, err)
				continue
			}
		}

		// 检查并添加daily_limit字段
		exists, err = config.RedisHExists(key, "daily_limit")
		if err != nil {
			logger.Log.Error("检查token %s的daily_limit字段失败: %v", key, err)
			continue
		}
		if !exists {
			err = config.RedisHSet(key, "daily_limit", "1000")
			if err != nil {
				logger.Log.Error("为token %s添加daily_limit字段失败: %v", key, err)
				continue
			}
		}

		logger.Log.Info("为token %s迁移新字段成功", key)
	}

	logger.Log.Info("所有token新字段迁移完成!")
	return nil
}

// CleanupDatabase 数据库清理功能
func CleanupDatabase(c *gin.Context) {
	var req struct {
		Action string `json:"action"` // "usage_stats", "daily_usage", "all_tokens", "expired_data"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求数据",
		})
		return
	}

	var result gin.H
	var err error

	switch req.Action {
	case "usage_stats":
		result, err = cleanUsageStats()
	case "daily_usage":
		result, err = cleanDailyUsage()
	case "all_tokens":
		result, err = cleanAllTokens()
	case "expired_data":
		result, err = cleanExpiredData()
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "不支持的清理操作",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   result,
	})
}

// cleanUsageStats 清理使用统计数据
func cleanUsageStats() (gin.H, error) {
	// 获取所有token
	keys, err := config.RedisKeys("token:*")
	if err != nil {
		return nil, fmt.Errorf("获取token列表失败: %v", err)
	}

	cleanedCount := 0
	for _, key := range keys {
		// 重置使用统计字段
		fields := []string{"chat_usage_count", "agent_usage_count", "usage_count"}
		for _, field := range fields {
			err = config.RedisHSet(key, field, "0")
			if err != nil {
				logger.Log.Error("重置token %s的%s字段失败: %v", key, field, err)
				continue
			}
		}
		cleanedCount++
	}

	return gin.H{
		"message":       "使用统计数据清理完成",
		"cleaned_count": cleanedCount,
	}, nil
}

// cleanDailyUsage 清理每日使用数据
func cleanDailyUsage() (gin.H, error) {
	// 获取所有每日使用数据的key
	keys, err := config.RedisKeys("token_daily_usage:*")
	if err != nil {
		return nil, fmt.Errorf("获取每日使用数据失败: %v", err)
	}

	cleanedCount := 0
	for _, key := range keys {
		err = config.RedisDel(key)
		if err != nil {
			logger.Log.Error("删除每日使用数据 %s 失败: %v", key, err)
			continue
		}
		cleanedCount++
	}

	return gin.H{
		"message":       "每日使用数据清理完成",
		"cleaned_count": cleanedCount,
	}, nil
}

// cleanAllTokens 清理所有token数据
func cleanAllTokens() (gin.H, error) {
	// 获取所有token
	keys, err := config.RedisKeys("token:*")
	if err != nil {
		return nil, fmt.Errorf("获取token列表失败: %v", err)
	}

	cleanedCount := 0
	for _, key := range keys {
		err = config.RedisDel(key)
		if err != nil {
			logger.Log.Error("删除token %s 失败: %v", key, err)
			continue
		}
		cleanedCount++
	}

	// 同时清理相关的使用数据
	usageKeys, err := config.RedisKeys("token_daily_usage:*")
	if err == nil {
		for _, key := range usageKeys {
			config.RedisDel(key)
			cleanedCount++
		}
	}

	// 清理请求状态数据
	statusKeys, err := config.RedisKeys("token_request_status:*")
	if err == nil {
		for _, key := range statusKeys {
			config.RedisDel(key)
			cleanedCount++
		}
	}

	return gin.H{
		"message":       "所有token数据清理完成",
		"cleaned_count": cleanedCount,
	}, nil
}

// cleanExpiredData 清理过期数据
func cleanExpiredData() (gin.H, error) {
	cleanedCount := 0

	// 清理过期的每日使用数据（超过7天的）
	keys, err := config.RedisKeys("token_daily_usage:*")
	if err != nil {
		return nil, fmt.Errorf("获取每日使用数据失败: %v", err)
	}

	sevenDaysAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	for _, key := range keys {
		// 从key中提取日期
		parts := strings.Split(key, ":")
		if len(parts) >= 3 {
			dateStr := parts[len(parts)-1]
			if dateStr < sevenDaysAgo {
				err = config.RedisDel(key)
				if err != nil {
					logger.Log.Error("删除过期数据 %s 失败: %v", key, err)
					continue
				}
				cleanedCount++
			}
		}
	}

	// 清理过期的请求状态数据（超过1小时的）
	statusKeys, err := config.RedisKeys("token_request_status:*")
	if err == nil {
		for _, key := range statusKeys {
			// 获取状态数据
			statusData, err := config.RedisGet(key)
			if err != nil {
				continue
			}

			var status TokenRequestStatus
			err = json.Unmarshal([]byte(statusData), &status)
			if err != nil {
				continue
			}

			// 如果超过1小时且不在进行中，删除
			if time.Since(status.LastRequestAt) > time.Hour && !status.InProgress {
				config.RedisDel(key)
				cleanedCount++
			}
		}
	}

	return gin.H{
		"message":       "过期数据清理完成",
		"cleaned_count": cleanedCount,
	}, nil
}
