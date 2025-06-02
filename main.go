package main

import (
	"augment2api/api"
	"augment2api/config"
	"augment2api/middleware"
	"augment2api/pkg/logger"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const clientID = "v"

// OAuthState 存储OAuth状态信息
type OAuthState struct {
	CodeVerifier  string    `json:"code_verifier"`
	CodeChallenge string    `json:"code_challenge"`
	State         string    `json:"state"`
	CreationTime  time.Time `json:"creation_time"`
}

// 全局变量存储OAuth状态
var (
	globalOAuthState OAuthState
)

// base64URLEncode 编码Buffer为base64 URL安全格式
func base64URLEncode(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	encoded = strings.ReplaceAll(encoded, "+", "-")
	encoded = strings.ReplaceAll(encoded, "/", "_")
	encoded = strings.ReplaceAll(encoded, "=", "")
	return encoded
}

// sha256Hash 计算SHA256哈希
func sha256Hash(input []byte) []byte {
	hash := sha256.Sum256(input)
	return hash[:]
}

// createOAuthState 创建OAuth状态
func createOAuthState() OAuthState {
	codeVerifierBytes := make([]byte, 32)
	_, err := rand.Read(codeVerifierBytes)
	if err != nil {
		log.Fatalf("生成随机字节失败: %v", err)
	}

	codeVerifier := base64URLEncode(codeVerifierBytes)
	codeChallenge := base64URLEncode(sha256Hash([]byte(codeVerifier)))

	stateBytes := make([]byte, 8)
	_, err = rand.Read(stateBytes)
	if err != nil {
		log.Fatalf("生成随机状态失败: %v", err)
	}
	state := base64URLEncode(stateBytes)

	return OAuthState{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
		State:         state,
		CreationTime:  time.Now(),
	}
}

// generateAuthorizeURL 生成授权URL
func generateAuthorizeURL(oauthState OAuthState) string {
	params := url.Values{}
	params.Add("response_type", "code")
	params.Add("code_challenge", oauthState.CodeChallenge)
	params.Add("client_id", clientID)
	params.Add("state", oauthState.State)
	params.Add("prompt", "login")

	authorizeURL := fmt.Sprintf("https://auth.augmentcode.com/authorize?%s", params.Encode())
	return authorizeURL
}

// getAccessToken 获取访问令牌
func getAccessToken(tenantURL, codeVerifier, code string) (string, error) {
	data := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     clientID,
		"code_verifier": codeVerifier,
		"redirect_uri":  "",
		"code":          code,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("序列化数据失败: %v", err)
	}

	resp, err := http.Post(tenantURL+"token", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("请求令牌失败: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	token, ok := result["access_token"].(string)
	if !ok {
		return "", fmt.Errorf("响应中没有访问令牌")
	}

	return token, nil
}

// 初始化路由
func setupRouter() *gin.Engine {
	r := gin.Default()

	// 跨域
	r.Use(middleware.CORS())

	// 初始化OAuth状态
	globalOAuthState = createOAuthState()

	// 静态文件服务
	r.Static("/static", "./static")
	r.LoadHTMLGlob("templates/*")

	// 登录页面
	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", gin.H{})
	})

	// 登录
	r.POST("/api/login", api.LoginHandler)

	// 登出
	r.POST("/api/logout", api.LogoutHandler)

	// 管理页面 - 需要会话验证
	r.GET("/", func(c *gin.Context) {
		// 如果设置了访问密码，检查是否已登录
		if config.AppConfig.AccessPwd != "" {
			// 从查询参数或Cookie中获取会话令牌
			token := c.Query("token")
			if token == "" {
				// 尝试从Cookie获取
				token, _ = c.Cookie("auth_token")
			}

			// 从请求头获取
			if token == "" {
				token = c.GetHeader("X-Auth-Token")
			}

			// 验证会话令牌
			if !api.ValidateToken(token) {
				c.Redirect(http.StatusFound, "/login")
				return
			}
		}
		c.HTML(http.StatusOK, "admin.html", gin.H{})
	})

	// 管理页面 - 需要会话验证
	r.GET("/admin", api.AuthTokenMiddleware(), func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin.html", gin.H{})
	})

	// 授权端点 - 需要会话验证
	r.GET("/auth", api.AuthTokenMiddleware(), func(c *gin.Context) {
		authorizeURL := generateAuthorizeURL(globalOAuthState)
		api.AuthHandler(c, authorizeURL)
	})

	// 获取token - 需要会话验证
	r.GET("/api/tokens", api.AuthTokenMiddleware(), api.GetRedisTokenHandler)

	// 删除token - 需要会话验证
	r.DELETE("/api/token/:token", api.AuthTokenMiddleware(), api.DeleteTokenHandler)

	// 更新token备注 - 需要会话验证
	r.PUT("/api/token/:token/remark", api.AuthTokenMiddleware(), api.UpdateTokenRemark)

	// 更新token状态 - 需要会话验证
	r.PUT("/api/token/:token/status", api.AuthTokenMiddleware(), api.UpdateTokenStatus)

	// 更新token限制 - 需要会话验证
	r.PUT("/api/token/:token/limits", api.AuthTokenMiddleware(), api.UpdateTokenLimits)

	// 数据库清理 - 需要会话验证
	r.POST("/api/cleanup", api.AuthTokenMiddleware(), api.CleanupDatabase)

	// 系统配置管理 - 需要会话验证
	r.GET("/api/system/configs", api.AuthTokenMiddleware(), api.GetSystemConfigs)
	r.PUT("/api/system/config", api.AuthTokenMiddleware(), api.UpdateSystemConfig)
	r.DELETE("/api/system/config/:key", api.AuthTokenMiddleware(), api.DeleteSystemConfig)
	r.GET("/api/system/stats", api.AuthTokenMiddleware(), api.GetDatabaseStats)
	r.GET("/api/system/details", api.AuthTokenMiddleware(), api.GetDatabaseDetails)
	r.GET("/api/system/export", api.AuthTokenMiddleware(), api.ExportSystemConfig)
	r.POST("/api/system/import", api.AuthTokenMiddleware(), api.ImportSystemConfig)
	r.POST("/api/system/test-proxy", api.AuthTokenMiddleware(), api.TestProxy)

	// 批量检测token - 需要会话验证
	r.GET("/api/check-tokens", api.AuthTokenMiddleware(), api.CheckAllTokensHandler)

	// Cloudflare Workers管理 - 需要会话验证
	r.GET("/api/cf-workers", api.AuthTokenMiddleware(), api.GetCFWorkers)
	r.POST("/api/cf-workers", api.AuthTokenMiddleware(), api.CreateCFWorker)
	r.PUT("/api/cf-workers/:id", api.AuthTokenMiddleware(), api.UpdateCFWorker)
	r.DELETE("/api/cf-workers/:id", api.AuthTokenMiddleware(), api.DeleteCFWorker)
	r.POST("/api/cf-workers/test", api.AuthTokenMiddleware(), api.TestCFWorker)

	// 回调端点，用于处理授权码 - 需要会话验证
	r.POST("/callback", api.AuthTokenMiddleware(), func(c *gin.Context) {
		api.CallbackHandler(c, func(tenantURL, _, code string) (string, error) {
			return getAccessToken(tenantURL, globalOAuthState.CodeVerifier, code)
		})
	})

	// 鉴权路由组
	authGroup := r.Group(ProcessPath(config.AppConfig.RoutePrefix))
	authGroup.Use(api.AuthMiddleware())
	{
		// OpenAI兼容的聊天端点
		chatGroup := authGroup.Group("/")
		// 并发控制
		chatGroup.Use(middleware.TokenConcurrencyMiddleware())
		{
			chatGroup.POST("/v1/chat/completions", api.ChatCompletionsHandler)
			chatGroup.POST("/v1", api.ChatCompletionsHandler)
			chatGroup.POST("/v1/chat", api.ChatCompletionsHandler)
		}

		authGroup.GET("/v1/models", api.ModelsHandler)
		authGroup.POST("/api/add/tokens", api.AddTokenHandler)
	}

	return r
}

func ProcessPath(path string) string {
	// 判断字符串是否为空
	if path == "" {
		return ""
	}

	// 判断开头是否为/，不是则添加
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 判断结尾是否为/，是则去掉
	if strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}

	return path
}

func main() {
	// 设置全局时区为东八区（CST）
	time.Local = time.FixedZone("CST", 8*3600)

	// 设置 Gin 为发布模式
	gin.SetMode(gin.ReleaseMode)

	// 初始化日志
	logger.Init()

	// 初始化配置
	err := config.InitConfig()
	if err != nil {
		logger.Log.Fatalln("failed to initialize config: " + err.Error())
		return
	}

	// 初始化Redis
	err = config.InitRedisClient()
	if err != nil {
		logger.Log.Fatalln("failed to initialize Redis: " + err.Error())
	}

	// 从数据库加载配置
	err = config.LoadConfigFromDatabase()
	if err != nil {
		logger.Log.Error("从数据库加载配置失败: %v", err)
		logger.Log.Info("将使用默认配置继续启动")
	}

	// token备注字段迁移
	err = api.MigrateTokensRemark()
	if err != nil {
		logger.Log.Error("Token备注字段迁移失败: %v", err)
	}

	// token新字段迁移
	err = api.MigrateTokensNewFields()
	if err != nil {
		logger.Log.Error("Token新字段迁移失败: %v", err)
	}

	// 启动token使用次数重置调度器
	go api.StartTokenUsageResetScheduler()

	r := setupRouter()

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "7860" // Hugging Face Space 默认端口
	}

	if err := r.Run(":" + port); err != nil {
		logger.Log.Fatalf("启动服务失败: %v", err)
	}

	logger.Log.WithFields(map[string]interface{}{
		"port": port,
		"mode": gin.Mode(),
	}).Info("Augment2API 服务启动成功")
}
