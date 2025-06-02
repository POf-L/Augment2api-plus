package api

import (
	"augment2api/config"
	"augment2api/pkg/logger"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// OpenAIRequest OpenAI兼容的请求结构
type OpenAIRequest struct {
	Model       string        `json:"model,omitempty"`
	Messages    []ChatMessage `json:"messages,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// OpenAIResponse OpenAI兼容的响应结构
type OpenAIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// OpenAIStreamResponse OpenAI兼容的流式响应结构
type OpenAIStreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        ChatMessage `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason *string     `json:"finish_reason"`
}

type ChatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// GetContent 添加一个辅助方法来获取消息内容
func (m ChatMessage) GetContent() string {
	switch v := m.Content.(type) {
	case string:
		return v
	case []interface{}:
		var result string
		for _, item := range v {
			if contentMap, ok := item.(map[string]interface{}); ok {
				if text, exists := contentMap["text"]; exists {
					if textStr, ok := text.(string); ok {
						result += textStr
					}
				}
			}
		}
		return result
	default:
		return ""
	}
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolDefinition 工具定义结构
type ToolDefinition struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	InputSchemaJSON string `json:"input_schema_json"`
	ToolSafety      int    `json:"tool_safety"`
}

// Node 节点结构
type Node struct {
	ID          int         `json:"id"`
	Type        int         `json:"type"`
	Content     string      `json:"content"`
	ToolUse     ToolUse     `json:"tool_use"`
	AgentMemory AgentMemory `json:"agent_memory"`
}

type ToolUse struct {
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
	InputJSON string `json:"input_json"`
}

type AgentMemory struct {
	Content string `json:"content"`
}

// AugmentRequest Augment API请求结构
type AugmentRequest struct {
	ChatHistory    []AugmentChatHistory `json:"chat_history"`
	Message        string               `json:"message"`
	AgentMemories  string               `json:"agent_memories"`
	Mode           string               `json:"mode"`
	Prefix         string               `json:"prefix"`
	Suffix         string               `json:"suffix"`
	Lang           string               `json:"lang"`
	Path           string               `json:"path"`
	UserGuideLines string               `json:"user_guidelines"`
	Blobs          struct {
		CheckpointID string        `json:"checkpoint_id"`
		AddedBlobs   []interface{} `json:"added_blobs"`
		DeletedBlobs []interface{} `json:"deleted_blobs"`
	} `json:"blobs"`
	UserGuidedBlobs       []interface{} `json:"user_guided_blobs"`
	ExternalSourceIds     []interface{} `json:"external_source_ids"`
	FeatureDetectionFlags struct {
		SupportRawOutput bool `json:"support_raw_output"`
	} `json:"feature_detection_flags"`
	ToolDefinitions []ToolDefinition `json:"tool_definitions"`
	Nodes           []Node           `json:"nodes"`
}

type AugmentChatHistory struct {
	ResponseText   string `json:"response_text"`
	RequestMessage string `json:"request_message"`
	RequestID      string `json:"request_id"`
	RequestNodes   []Node `json:"request_nodes"`
	ResponseNodes  []Node `json:"response_nodes"`
}

// AugmentResponse Augment API响应结构
type AugmentResponse struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// CodeResponse 用于解析从授权服务返回的代码
type CodeResponse struct {
	Code      string `json:"code"`
	State     string `json:"state"`
	TenantURL string `json:"tenant_url"`
}

// ModelObject OpenAI模型对象结构
type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelsResponse OpenAI模型列表响应结构
type ModelsResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// 全局变量
var (
	accessToken string
	tenantURL   string
)

const (
	// 错误信息
	errBlocked = "Request blocked. Please reach out to support@augmentcode.com if you think this was a mistake."
)

// SetAuthInfo 设置认证信息
func SetAuthInfo(token, tenant string) {
	accessToken = token
	tenantURL = tenant
}

// GetAuthInfo 获取认证信息
func GetAuthInfo() (string, string) {
	logger.Log.WithFields(logrus.Fields{
		"coding_mode":   config.AppConfig.CodingMode,
		"has_memory_token": accessToken != "",
		"has_memory_tenant": tenantURL != "",
		"has_config_token": config.AppConfig.CodingToken != "",
		"has_config_tenant": config.AppConfig.TenantURL != "",
	}).Info("GetAuthInfo调用")

	if config.AppConfig.CodingMode == "true" {
		// 调试模式
		logger.Log.Info("使用调试模式配置")
		return config.AppConfig.CodingToken, config.AppConfig.TenantURL
	}

	// 如果内存中有token和tenantURL，直接返回
	if accessToken != "" && tenantURL != "" {
		logger.Log.Info("使用内存中的token和tenantURL")
		return accessToken, tenantURL
	}

	// 如果内存中没有，尝试从配置中获取
	if config.AppConfig.CodingToken != "" && config.AppConfig.TenantURL != "" {
		logger.Log.Info("使用配置中的token和tenantURL")
		return config.AppConfig.CodingToken, config.AppConfig.TenantURL
	}

	// 都没有则返回空
	logger.Log.Warning("GetAuthInfo返回空值")
	return "", ""
}

const (
	// 默认提示，不加这个会导致Agent触发文件创建，回复截断
	defaultPrompt = "Your are claude3.7, All replies cannot create, modify, or delete files, and must provide content directly!"
	// 默认上下文，影响模型回复风格
	defaultPrefix = "You are AI assistant,help me to solve problems!"
)

// generateCheckpointID 生成一个基于时间戳的SHA-256哈希值作为CheckpointID
func generateCheckpointID() string {
	// 使用当前时间戳作为输入
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
	hash := sha256.New()
	hash.Write([]byte(timestamp))
	// 将哈希值转换为十六进制字符串
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// generatePath 生成一个随机文件路径（暂时无用）
func generatePath() string {
	extensions := []string{".txt", ".md", ".go", ".py", ".js", ".html", ".css"}
	dirs := []string{"src", "docs", "test", "lib", "utils"}
	dir := dirs[rand.Intn(len(dirs))]
	ext := extensions[rand.Intn(len(extensions))]
	filename := fmt.Sprintf("%x", rand.Int31())
	return fmt.Sprintf("%s/%s%s", dir, filename, ext)
}

// convertToAugmentRequest 将OpenAI请求转换为Augment请求
func convertToAugmentRequest(req OpenAIRequest) AugmentRequest {
	// 确定模式和其他参数基于模型名称
	mode := "CHAT" // 默认使用CHAT模式
	userGuideLines := "must answer in Chinese."
	includeToolDefinitions := false
	includeDefaultPrompt := false

	// 将模型名称转换为小写，然后检查后缀
	modelLower := strings.ToLower(req.Model)

	// 检查模型名称后缀 (不区分大小写)
	if strings.HasSuffix(modelLower, "-chat") {
		// 保持使用CHAT模式的默认设置
		mode = "CHAT"
	} else if strings.HasSuffix(modelLower, "-agent") {
		// 使用AGENT模式
		mode = "AGENT"
		userGuideLines = "Answer in Chinese, do not use any tools, and for questions involving internet searches, please answer based on your existing knowledge."
		includeToolDefinitions = true
		includeDefaultPrompt = true
	}

	augmentReq := AugmentRequest{
		Path:           "",                  // 这个是关联的项目文件路径，暂时传空，不影响对话
		Mode:           mode,                // 根据模型名称决定模式
		Prefix:         defaultPrefix,       // 固定前缀，影响模型回复风格
		Suffix:         " ",                 // 固定后缀，暂时传空，不影响对话
		Lang:           detectLanguage(req), // 简单检测当前对话语言类型，不传好像回答有问题
		Message:        "",                  // 当前对话消息
		UserGuideLines: userGuideLines,      // 根据模型类型设置指南
		// 初始化为空列表
		ChatHistory: make([]AugmentChatHistory, 0),
		Blobs: struct {
			CheckpointID string        `json:"checkpoint_id"`
			AddedBlobs   []interface{} `json:"added_blobs"`
			DeletedBlobs []interface{} `json:"deleted_blobs"`
		}{
			CheckpointID: generateCheckpointID(),
			AddedBlobs:   make([]interface{}, 0),
			DeletedBlobs: make([]interface{}, 0),
		},
		UserGuidedBlobs:   make([]interface{}, 0),
		ExternalSourceIds: make([]interface{}, 0),
		FeatureDetectionFlags: struct {
			SupportRawOutput bool `json:"support_raw_output"`
		}{
			SupportRawOutput: true,
		},
		ToolDefinitions: []ToolDefinition{}, // 初始化为空
		Nodes:           make([]Node, 0),
	}

	// 根据模型类型决定是否包含工具定义
	if includeToolDefinitions {
		augmentReq.ToolDefinitions = getFullToolDefinitions()
	}

	// 处理消息历史
	if len(req.Messages) > 1 { // 有历史消息
		// 每次处理一对消息（用户问题和助手回答）
		for i := 0; i < len(req.Messages)-1; i += 2 {
			if i+1 < len(req.Messages) {
				userMsg := req.Messages[i]
				assistantMsg := req.Messages[i+1]

				chatHistory := AugmentChatHistory{
					RequestMessage: userMsg.GetContent(),
					ResponseText:   assistantMsg.GetContent(),
					RequestID:      generateRequestID(), // 生成唯一的请求ID
					RequestNodes:   make([]Node, 0),
					ResponseNodes: []Node{
						{
							ID:      0,
							Type:    0,
							Content: assistantMsg.GetContent(),
							ToolUse: ToolUse{
								ToolUseID: "",
								ToolName:  "",
								InputJSON: "",
							},
							AgentMemory: AgentMemory{
								Content: "",
							},
						},
					},
				}
				augmentReq.ChatHistory = append(augmentReq.ChatHistory, chatHistory)
			}
		}
	}

	// 设置当前消息
	if len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if includeDefaultPrompt {
			augmentReq.Message = defaultPrompt + "\n" + lastMsg.GetContent()
		} else {
			augmentReq.Message = lastMsg.GetContent()
		}
	}

	return augmentReq
}

// generateRequestID 生成唯一的请求ID
func generateRequestID() string {
	// 使用UUID v4生成唯一ID
	return uuid.New().String()
}

// detectLanguage 检测编程语言
func detectLanguage(req OpenAIRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}

	content := req.Messages[len(req.Messages)-1].GetContent()
	// 简单判断一下当前对话语言类型
	if strings.Contains(strings.ToLower(content), "html") {
		return "HTML"
	} else if strings.Contains(strings.ToLower(content), "python") {
		return "Python"
	} else if strings.Contains(strings.ToLower(content), "javascript") {
		return "JavaScript"
	} else if strings.Contains(strings.ToLower(content), "go") {
		return "Go"
	} else if strings.Contains(strings.ToLower(content), "rust") {
		return "Rust"
	} else if strings.Contains(strings.ToLower(content), "java") {
		return "Java"
	} else if strings.Contains(strings.ToLower(content), "c++") {
		return "C++"
	} else if strings.Contains(strings.ToLower(content), "c#") {
		return "C#"
	} else if strings.Contains(strings.ToLower(content), "php") {
		return "PHP"
	} else if strings.Contains(strings.ToLower(content), "ruby") {
		return "Ruby"
	} else if strings.Contains(strings.ToLower(content), "swift") {
		return "Swift"
	} else if strings.Contains(strings.ToLower(content), "kotlin") {
		return "Kotlin"
	} else if strings.Contains(strings.ToLower(content), "typescript") {
		return "TypeScript"
	} else if strings.Contains(strings.ToLower(content), "c") {
		return "C"
	}
	return "HTML"
}

// getFullToolDefinitions 返回官方定义的完整工具定义列表
// TODO 验证实际作用
func getFullToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:            "save-file",
			Description:     "Save a new file. Use this tool to write new files with the attached content. It CANNOT modify existing files. Do NOT use this tool to edit an existing file by overwriting it entirely. Use the str-replace-editor tool to edit existing files instead.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"file_path\":{\"type\":\"string\",\"description\":\"The path of the file to save.\"},\"file_content\":{\"type\":\"string\",\"description\":\"The content of the file.\"},\"add_last_line_newline\":{\"type\":\"boolean\",\"description\":\"Whether to add a newline at the end of the file (default: true).\"}},\"required\":[\"file_path\",\"file_content\"]}",
			ToolSafety:      1,
		},
		{
			Name:            "launch-process",
			Description:     "Launch a new process with a shell command. A process can be waiting (`wait=true`) or non-waiting (`wait=false`).\n\nIf `wait=true`, launches the process in an interactive terminal, and waits for the process to complete up to\n`max_wait_seconds` seconds. If the process ends during this period, the tool call returns. If the timeout\nexpires, the process will continue running in the background but the tool call will return. You can then\ninteract with the process using the other process tools.\n\nNote: Only one waiting process can be running at a time. If you try to launch a process with `wait=true`\nwhile another is running, the tool will return an error.\n\nIf `wait=false`, launches a background process in a separate terminal. This returns immediately, while the\nprocess keeps running in the background.\n\nNotes:\n- Use `wait=true` processes when the command is expected to be short, or when you can't\nproceed with your task until the process is complete. Use `wait=false` for processes that are\nexpected to run in the background, such as starting a server you'll need to interact with, or a\nlong-running process that does not need to complete before proceeding with the task.\n- If this tool returns while the process is still running, you can continue to interact with the process\nusing the other available tools. You can wait for the process, read from it, write to it, kill it, etc.\n- You can use this tool to interact with the user's local version control system. Do not use the\nretrieval tool for that purpose.\n- If there is a more specific tool available that can perform the function, use that tool instead of\nthis one.\n\nThe OS is darwin.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"command\":{\"type\":\"string\",\"description\":\"The shell command to execute.\"},\"wait\":{\"type\":\"boolean\",\"description\":\"Whether to wait for the command to complete.\"},\"max_wait_seconds\":{\"type\":\"number\",\"description\":\"Number of seconds to wait for the command to complete. Only relevant when wait=true. 10 minutes may be a good default: increase from there if needed.\"},\"cwd\":{\"type\":\"string\",\"description\":\"Working directory for the command. If not supplied, uses the current working directory.\"}},\"required\":[\"command\",\"wait\",\"max_wait_seconds\"]}",
			ToolSafety:      2,
		},
		{
			Name:            "read-process",
			Description:     "Read output from a terminal.\n\nIf `wait=true` and the process has not yet completed, waits for the terminal to complete up to `max_wait_seconds` seconds before returning its output.\n\nIf `wait=false` or the process has already completed, returns immediately with the current output.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"terminal_id\":{\"type\":\"integer\",\"description\":\"Terminal ID to read from.\"},\"wait\":{\"type\":\"boolean\",\"description\":\"Whether to wait for the command to complete.\"},\"max_wait_seconds\":{\"type\":\"number\",\"description\":\"Number of seconds to wait for the command to complete. Only relevant when wait=true. 1 minute may be a good default: increase from there if needed.\"}},\"required\":[\"terminal_id\",\"wait\",\"max_wait_seconds\"]}",
			ToolSafety:      1,
		},
		{
			Name:            "kill-process",
			Description:     "Kill a process by its process ID.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"terminal_id\":{\"type\":\"integer\",\"description\":\"Process ID to kill.\"}},\"required\":[\"terminal_id\"]}",
			ToolSafety:      1,
		},
		{
			Name:            "write-process",
			Description:     "Write input to a process's stdin.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"terminal_id\":{\"type\":\"integer\",\"description\":\"Process ID to write to.\"},\"input_text\":{\"type\":\"string\",\"description\":\"Text to write to the process's stdin.\"}},\"required\":[\"terminal_id\",\"input_text\"]}",
			ToolSafety:      1,
		},
		{
			Name:            "list-processes",
			Description:     "List all known processes and their states.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{},\"required\":[]}",
			ToolSafety:      1,
		},
		{
			Name:            "web-search",
			Description:     "Search the web for information. Returns results in markdown format.\nEach result includes the URL, title, and a snippet from the page if available.\n\nThis tool uses Google's Custom Search API to find relevant web pages.",
			InputSchemaJSON: "{\"description\": \"Input schema for the web search tool.\", \"properties\": {\"query\": {\"description\": \"The search query to send.\", \"title\": \"Query\", \"type\": \"string\"}, \"num_results\": {\"default\": 5, \"description\": \"Number of results to return\", \"maximum\": 10, \"minimum\": 1, \"title\": \"Num Results\", \"type\": \"integer\"}}, \"required\": [\"query\"], \"title\": \"WebSearchInput\", \"type\": \"object\"}",
			ToolSafety:      0,
		},
		{
			Name:            "web-fetch",
			Description:     "Fetches data from a webpage and converts it into Markdown.\n\n1. The tool takes in a URL and returns the content of the page in Markdown format;\n2. If the return is not valid Markdown, it means the tool cannot successfully parse this page.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"url\":{\"type\":\"string\",\"description\":\"The URL to fetch.\"}},\"required\":[\"url\"]}",
			ToolSafety:      0,
		},
		{
			Name:            "codebase-retrieval",
			Description:     "This tool is Augment's context engine, the world's best codebase context engine. It:\n1. Takes in a natural language description of the code you are looking for;\n2. Uses a proprietary retrieval/embedding model suite that produces the highest-quality recall of relevant code snippets from across the codebase;\n3. Maintains a real-time index of the codebase, so the results are always up-to-date and reflects the current state of the codebase;\n4. Can retrieve across different programming languages;\n5. Only reflects the current state of the codebase on the disk, and has no information on version control or code history.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"information_request\":{\"type\":\"string\",\"description\":\"A description of the information you need.\"}},\"required\":[\"information_request\"]}",
			ToolSafety:      1,
		},
		{
			Name:            "remove-files",
			Description:     "Remove files. ONLY use this tool to delete files in the user's workspace. This is the only safe tool to delete files in a way that the user can undo the change. Do NOT use the shell or launch-process tools to remove files.",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"file_paths\":{\"type\":\"array\",\"description\":\"The paths of the files to remove.\",\"items\":{\"type\":\"string\"}}},\"required\":[\"file_paths\"]}",
			ToolSafety:      1,
		},
		{
			Name:            "remember",
			Description:     "Call this tool when user asks you:\n- to remember something\n- to create memory/memories\n\nUse this tool only with information that can be useful in the long-term.\nDo not use this tool for temporary information.\n",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"memory\":{\"type\":\"string\",\"description\":\"The concise (1 sentence) memory to remember.\"}},\"required\":[\"memory\"]}",
			ToolSafety:      1,
		},
		{
			Name:            "str-replace-editor",
			Description:     "Custom editing tool for viewing, creating and editing files\n* `path` is a file path relative to the workspace root\n* command `view` displays the result of applying `cat -n`.\n* If a `command` generates a long output, it will be truncated and marked with `<response clipped>`\n* `insert` and `str_replace` commands output a snippet of the edited section for each entry. This snippet reflects the final state of the file after all edits and IDE auto-formatting have been applied.\n\n\nNotes for using the `str_replace` command:\n* Use the `str_replace_entries` parameter with an array of objects\n* Each object should have `old_str`, `new_str`, `old_str_start_line_number` and `old_str_end_line_number` properties\n* The `old_str_start_line_number` and `old_str_end_line_number` parameters are 1-based line numbers\n* Both `old_str_start_line_number` and `old_str_end_line_number` are INCLUSIVE\n* The `old_str` parameter should match EXACTLY one or more consecutive lines from the original file. Be mindful of whitespace!\n* Empty `old_str` is allowed only when the file is empty or contains only whitespaces\n* It is important to specify `old_str_start_line_number` and `old_str_end_line_number` to disambiguate between multiple occurrences of `old_str` in the file\n* Make sure that `old_str_start_line_number` and `old_str_end_line_number` do not overlap with other entries in `str_replace_entries`\n* The `new_str` parameter should contain the edited lines that should replace the `old_str`. Can be an empty string to delete content\n\nNotes for using the `insert` command:\n* Use the `insert_line_entries` parameter with an array of objects\n* Each object should have `insert_line` and `new_str` properties\n* The `insert_line` parameter specifies the line number after which to insert the new string\n* The `insert_line` parameter is 1-based line number\n* To insert at the very beginning of the file, use `insert_line: 0`\n\nNotes for using the `view` command:\n* Strongly prefer to use larger ranges of at least 500 lines when scanning through files. One call with large range is much more efficient than many calls with small ranges\n\nIMPORTANT:\n* This is the only tool you should use for editing files.\n* If it fails try your best to fix inputs and retry.\n* DO NOT fall back to removing the whole file and recreating it from scratch.\n* DO NOT use sed or any other command line tools for editing files.\n* Try to fit as many edits in one tool call as possible\n* Use view command to read the file before editing it.\n",
			InputSchemaJSON: "{\"type\":\"object\",\"properties\":{\"command\":{\"type\":\"string\",\"enum\":[\"view\",\"str_replace\",\"insert\"],\"description\":\"The commands to run. Allowed options are: 'view', 'str_replace', 'insert'.\"},\"path\":{\"description\":\"Full path to file relative to the workspace root, e.g. 'services/api_proxy/file.py' or 'services/api_proxy'.\",\"type\":\"string\"},\"view_range\":{\"description\":\"Optional parameter of `view` command when `path` points to a file. If none is given, the full file is shown. If provided, the file will be shown in the indicated line number range, e.g. [501, 1000] will show lines from 501 to 1000. Indices are 1-based and inclusive. Setting `[start_line, -1]` shows all lines from `start_line` to the end of the file.\",\"type\":\"array\",\"items\":{\"type\":\"integer\"}},\"insert_line_entries\":{\"description\":\"Required parameter of `insert` command. A list of entries to insert. Each entry is a dictionary with keys `insert_line` and `new_str`.\",\"type\":\"array\",\"items\":{\"type\":\"object\",\"properties\":{\"insert_line\":{\"description\":\"The line number after which to insert the new string. This line number is relative to the state of the file before any insertions in the current tool call have been applied.\",\"type\":\"integer\"},\"new_str\":{\"description\":\"The string to insert. Can be an empty string.\",\"type\":\"string\"}},\"required\":[\"insert_line\",\"new_str\"]}},\"str_replace_entries\":{\"description\":\"Required parameter of `str_replace` command. A list of entries to replace. Each entry is a dictionary with keys `old_str`, `old_str_start_line_number`, `old_str_end_line_number` and `new_str`. `old_str` from different entries should not overlap.\",\"type\":\"array\",\"items\":{\"type\":\"object\",\"properties\":{\"old_str\":{\"description\":\"The string in `path` to replace.\",\"type\":\"string\"},\"old_str_start_line_number\":{\"description\":\"The line number of the first line of `old_str` in the file. This is used to disambiguate between multiple occurrences of `old_str` in the file.\",\"type\":\"integer\"},\"old_str_end_line_number\":{\"description\":\"The line number of the last line of `old_str` in the file. This is used to disambiguate between multiple occurrences of `old_str` in the file.\",\"type\":\"integer\"},\"new_str\":{\"description\":\"The string to replace `old_str` with. Can be an empty string to delete content.\",\"type\":\"string\"}},\"required\":[\"old_str\",\"new_str\",\"old_str_start_line_number\",\"old_str_end_line_number\"]}}},\"required\":[\"command\",\"path\"]}",
			ToolSafety:      1,
		},
	}
}

// AuthHandler 处理授权请求
func AuthHandler(c *gin.Context, authorizeURL string) {
	c.JSON(http.StatusOK, gin.H{
		"authorize_url": authorizeURL,
	})
}

// CallbackHandler 处理回调请求
func CallbackHandler(c *gin.Context, getAccessTokenFunc func(string, string, string) (string, error)) {
	// 1. 解析请求数据
	var codeResp CodeResponse
	if err := c.ShouldBindJSON(&codeResp); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 2. 使用授权码获取访问令牌
	token, err := getAccessTokenFunc(codeResp.TenantURL, "", codeResp.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 3. 保存令牌和租户URL
	SetAuthInfo(token, codeResp.TenantURL)

	// 4. 保存到Redis
	if err := SaveTokenToRedis(token, codeResp.TenantURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存token到Redis失败: " + err.Error()})
		return
	}

	// 5. 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"token":  token,
	})
}

// ModelsHandler 处理模型请求
func ModelsHandler(c *gin.Context) {
	// 这里直接返回写死的模型
	response := ModelsResponse{
		Object: "list",
		Data: []ModelObject{
			{
				ID:      "claude-3.7-agent",
				Object:  "model",
				Created: 1708387200,
				OwnedBy: "anthropic",
			},
			{
				ID:      "augment-chat",
				Object:  "model",
				Created: 1708387200,
				OwnedBy: "augment",
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// ChatCompletionsHandler 处理OpenAI兼容的聊天完成请求
func ChatCompletionsHandler(c *gin.Context) {
	// 获取请求数据
	var req OpenAIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		// 确保在错误情况下也清理请求状态
		cleanupRequestStatus(c)
		return
	}

	// 转换为Augment请求格式
	augmentReq := convertToAugmentRequest(req)

	// 处理流式请求
	if req.Stream {
		handleStreamRequest(c, augmentReq, req.Model)
		return
	}

	// 处理非流式请求
	handleNonStreamRequest(c, augmentReq, req.Model)
}

// 异步处理token使用计数
func asyncIncrementTokenUsage(token string, model string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Log.WithFields(logrus.Fields{
					"error": r,
					"token": token,
					"model": model,
				}).Error("system err")
			}
		}()

		// 增加token使用计数
		incrementTokenUsage(token, model)
	}()
}

// 处理流式请求
func handleStreamRequest(c *gin.Context, augmentReq AugmentRequest, model string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.WithFields(logrus.Fields{
				"error": r,
				"model": model,
			}).Error("处理流式请求时发生panic")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		}
		// 函数返回时同步清理请求状态
		cleanupRequestStatus(c)
	}()

	// 从上下文中获取token和tenant_url
	tokenInterface, exists := c.Get("token")
	tenantURLInterface, exists2 := c.Get("tenant_url")

	var token, tenant string

	if exists && exists2 {
		token, _ = tokenInterface.(string)
		tenant, _ = tenantURLInterface.(string)
	}

	// 如果上下文中没有，则使用GetAuthInfo获取
	if token == "" || tenant == "" {
		token, tenant = GetAuthInfo()
	}

	if token == "" || tenant == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无可用Token,请先在管理页面获取"})
		return
	}

	// 异步处理token使用计数
	asyncIncrementTokenUsage(token, model)

	// 准备请求数据
	jsonData, err := json.Marshal(augmentReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
		return
	}

	// 提取主机部分
	parsedURL, err := url.Parse(tenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析租户URL失败"})
		return
	}
	hostName := parsedURL.Host

	// 创建请求
	requestURL := tenant + "chat-stream"
	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}

	// 设置请求头
	req.Header.Set("Host", hostName)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "augment.intellij/0.184.0 (Mac OS X; aarch64; 15.2) WebStorm/2024.3.5")
	req.Header.Set("x-api-version", "2")

	// 生成请求ID和会话ID
	requestID := uuid.New().String()
	sessionID := uuid.New().String()
	req.Header.Set("x-request-id", requestID)
	req.Header.Set("x-request-session-id", sessionID)

	// 使用createHTTPClient创建客户端
	client := createHTTPClient()

	// 设置刷新器以确保数据立即发送
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "流式传输不支持"})
		return
	}

	// 第一次尝试使用原始模式请求
	resp, err := client.Do(req)
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"error": err.Error(),
			"mode":  augmentReq.Mode,
		}).Error("请求失败")

		// 切换到CHAT模式
		augmentReq.Mode = "CHAT"
		augmentReq.UserGuideLines = "使用中文回答"
		augmentReq.ToolDefinitions = []ToolDefinition{}

		// 重新准备请求数据
		jsonData, err = json.Marshal(augmentReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
			return
		}

		// 创建新的请求
		req, err = http.NewRequest("POST", requestURL, bytes.NewReader(jsonData))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
			return
		}

		// 设置相同的请求头
		req.Header.Set("Host", hostName)
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonData)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "augment.intellij/0.184.0 (Mac OS X; aarch64; 15.2) WebStorm/2024.3.5")
		req.Header.Set("x-api-version", "2")
		req.Header.Set("x-request-id", requestID)
		req.Header.Set("x-request-session-id", sessionID)

		// 重新发送请求
		resp, err = client.Do(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "请求失败: " + err.Error()})
			return
		}
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		errMsg := "Augment response error"
		if err == nil {
			bodyStr := string(body)
			// 检查是否是HTML错误页面（Hugging Face的500错误页面）
			if strings.Contains(bodyStr, "<html>") || strings.Contains(bodyStr, "<!DOCTYPE") {
				errMsg = "Service temporarily unavailable (500 Internal Server Error)"
				logger.Log.WithFields(logrus.Fields{
					"status_code": resp.StatusCode,
					"response_preview": bodyStr[:min(200, len(bodyStr))],
				}).Error("收到HTML错误页面，可能是Hugging Face平台问题")
			} else {
				errMsg = errMsg + ": " + bodyStr
			}
		}
		c.JSON(resp.StatusCode, gin.H{"error": errMsg})
		return
	}

	// 异步记录会话事件
	asyncRecordSessionEvent(token, tenant, requestID, sessionID)

	// 读取并转发响应
	reader := bufio.NewReader(resp.Body)
	responseID := fmt.Sprintf("chatcmpl-%d", time.Now().Unix())

	var fullText string
	var hasError bool

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Log.WithFields(logrus.Fields{
				"error": err.Error(),
				"mode":  augmentReq.Mode,
			}).Error("读取响应失败")

			// 切换到CHAT模式
			if augmentReq.Mode != "CHAT" {
				logger.Log.WithFields(logrus.Fields{
					"error": err.Error(),
					"mode":  augmentReq.Mode,
				}).Info("切换到CHAT模式")

				augmentReq.Mode = "CHAT"
				augmentReq.UserGuideLines = "使用中文回答"
				augmentReq.ToolDefinitions = []ToolDefinition{}

				// 重新准备请求数据
				jsonData, err = json.Marshal(augmentReq)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
					return
				}

				// 创建新的请求
				req, err = http.NewRequest("POST", requestURL, bytes.NewReader(jsonData))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
					return
				}

				// 设置相同的请求头
				req.Header.Set("Host", hostName)
				req.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonData)))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("User-Agent", "augment.intellij/0.184.0 (Mac OS X; aarch64; 15.2) WebStorm/2024.3.5")
				req.Header.Set("x-api-version", "2")
				req.Header.Set("x-request-id", requestID)
				req.Header.Set("x-request-session-id", sessionID)

				// 重新发送请求
				resp, err = client.Do(req)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "请求失败: " + err.Error()})
					return
				}
				defer resp.Body.Close()

				// 检查响应状态码
				if resp.StatusCode != http.StatusOK {
					body, err := io.ReadAll(resp.Body)
					errMsg := "Augment response error"
					if err == nil {
						bodyStr := string(body)
						// 检查是否是HTML错误页面（Hugging Face的500错误页面）
						if strings.Contains(bodyStr, "<html>") || strings.Contains(bodyStr, "<!DOCTYPE") {
							errMsg = "Service temporarily unavailable (500 Internal Server Error)"
							logger.Log.WithFields(logrus.Fields{
								"status_code": resp.StatusCode,
								"response_preview": bodyStr[:min(200, len(bodyStr))],
							}).Error("收到HTML错误页面，可能是Hugging Face平台问题")
						} else {
							errMsg = errMsg + ": " + bodyStr
						}
					}
					c.JSON(resp.StatusCode, gin.H{"error": errMsg})
					return
				}

				// 重新设置reader
				reader = bufio.NewReader(resp.Body)
				continue
			}
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var augmentResp AugmentResponse
		if err := json.Unmarshal([]byte(line), &augmentResp); err != nil {
			log.Printf("解析响应失败: %v", err)
			continue
		}

		// 检查响应内容是否包含错误信息
		if strings.Contains(augmentResp.Text, errBlocked) {
			hasError = true

			// 获取token的个性化请求间隔作为冷却时间
			requestInterval := getTokenRequestInterval(token)
			cooldownDuration := time.Duration(requestInterval) * time.Second

			// 将当前token加入冷却队列，使用个性化冷却时间
			logger.Log.WithFields(logrus.Fields{
				"token": token,
				"mode":  augmentReq.Mode,
				"cooldown_seconds": requestInterval,
			}).Info("检测到block信息，将token加入冷却队列")

			err := SetTokenCoolStatus(token, cooldownDuration)
			if err != nil {
				logger.Log.WithFields(logrus.Fields{
					"token": token,
					"error": err.Error(),
				}).Error("将token加入冷却队列失败")
			}

			break
		}

		fullText += augmentResp.Text

		// 创建OpenAI兼容的流式响应
		streamResp := OpenAIStreamResponse{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []StreamChoice{
				{
					Index: 0,
					Delta: ChatMessage{
						Role:    "assistant",
						Content: augmentResp.Text,
					},
					FinishReason: nil,
				},
			},
		}

		// 如果是最后一条消息，设置完成原因
		if augmentResp.Done {
			finishReason := "stop"
			streamResp.Choices[0].FinishReason = &finishReason
		}

		// 序列化并发送响应
		jsonResp, err := json.Marshal(streamResp)
		if err != nil {
			log.Printf("序列化响应失败: %v", err)
			continue
		}

		fmt.Fprintf(c.Writer, "data: %s\n\n", jsonResp)
		flusher.Flush()

		// 如果完成，发送最后的[DONE]标记
		if augmentResp.Done {
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}
	}

	// 如果检测到错误信息，尝试切换到CHAT模式重新请求
	if hasError && augmentReq.Mode != "CHAT" {
		logger.Log.WithFields(logrus.Fields{
			"mode": augmentReq.Mode,
		}).Info("检测到block信息，尝试切换到 CHAT 模式回复！")

		// 切换到CHAT模式
		augmentReq.Mode = "CHAT"
		augmentReq.UserGuideLines = "使用中文回答"
		augmentReq.ToolDefinitions = []ToolDefinition{}

		// 重新准备请求数据
		jsonData, err = json.Marshal(augmentReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
			return
		}

		// 创建新的请求
		req, err = http.NewRequest("POST", requestURL, bytes.NewReader(jsonData))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
			return
		}

		// 设置相同的请求头
		req.Header.Set("Host", hostName)
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonData)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "augment.intellij/0.184.0 (Mac OS X; aarch64; 15.2) WebStorm/2024.3.5")
		req.Header.Set("x-api-version", "2")
		req.Header.Set("x-request-id", requestID)
		req.Header.Set("x-request-session-id", sessionID)

		// 重新发送请求
		resp, err = client.Do(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "请求失败: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		// 检查响应状态码
		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			errMsg := "Augment response error"
			if err == nil {
				bodyStr := string(body)
				// 检查是否是HTML错误页面（Hugging Face的500错误页面）
				if strings.Contains(bodyStr, "<html>") || strings.Contains(bodyStr, "<!DOCTYPE") {
					errMsg = "Service temporarily unavailable (500 Internal Server Error)"
					logger.Log.WithFields(logrus.Fields{
						"status_code": resp.StatusCode,
						"response_preview": bodyStr[:min(200, len(bodyStr))],
					}).Error("收到HTML错误页面，可能是Hugging Face平台问题")
				} else {
					errMsg = errMsg + ": " + bodyStr
				}
			}
			c.JSON(resp.StatusCode, gin.H{"error": errMsg})
			return
		}

		// 异步记录会话事件
		asyncRecordSessionEvent(token, tenant, requestID, sessionID)

		// 读取并转发响应
		reader = bufio.NewReader(resp.Body)
		responseID = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())

		fullText = ""
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Printf("读取响应失败: %v", err)
				break
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			var augmentResp AugmentResponse
			if err := json.Unmarshal([]byte(line), &augmentResp); err != nil {
				log.Printf("解析响应失败: %v", err)
				continue
			}

			fullText += augmentResp.Text

			// 创建OpenAI兼容的流式响应
			streamResp := OpenAIStreamResponse{
				ID:      responseID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
				Choices: []StreamChoice{
					{
						Index: 0,
						Delta: ChatMessage{
							Role:    "assistant",
							Content: augmentResp.Text,
						},
						FinishReason: nil,
					},
				},
			}

			// 如果是最后一条消息，设置完成原因
			if augmentResp.Done {
				finishReason := "stop"
				streamResp.Choices[0].FinishReason = &finishReason
			}

			// 序列化并发送响应
			jsonResp, err := json.Marshal(streamResp)
			if err != nil {
				log.Printf("序列化响应失败: %v", err)
				continue
			}

			fmt.Fprintf(c.Writer, "data: %s\n\n", jsonResp)
			flusher.Flush()

			// 如果完成，发送最后的[DONE]标记
			if augmentResp.Done {
				fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
				flusher.Flush()
				break
			}
		}
	}
}

// estimateTokenCount 粗略估计文本中的token数量
// 这是一个简单的估算方法，实际token数量取决于具体的分词算法
func estimateTokenCount(text string) int {
	// 英文单词和标点符号大约是1个token
	// 中文字符大约是1.5个token（每个字符约为0.75个token）
	// 按空格分割英文单词
	words := strings.Fields(text)
	wordCount := len(words)

	// 计算中文字符数量
	chineseCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}

	// 粗略估计：英文单词按1个token计算，中文字符按0.75个token计算
	return wordCount + int(float64(chineseCount)*0.75)
}

// 处理非流式请求
func handleNonStreamRequest(c *gin.Context, augmentReq AugmentRequest, model string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.WithFields(logrus.Fields{
				"error": r,
				"model": model,
			}).Error("处理非流式请求时发生panic")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		}
		cleanupRequestStatus(c) // 确保在函数返回时同步清理请求状态
	}()

	// 从上下文中获取token和tenant_url
	tokenInterface, exists := c.Get("token")
	tenantURLInterface, exists2 := c.Get("tenant_url")

	var token, tenant string

	if exists && exists2 {
		token, _ = tokenInterface.(string)
		tenant, _ = tenantURLInterface.(string)
	}

	// 如果上下文中没有，则使用GetAuthInfo获取
	if token == "" || tenant == "" {
		token, tenant = GetAuthInfo()
	}

	if token == "" || tenant == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无可用Token,请先在管理页面获取"})
		return
	}

	// 异步处理token使用计数
	asyncIncrementTokenUsage(token, model)

	// 准备请求数据
	jsonData, err := json.Marshal(augmentReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
		return
	}

	// 提取主机部分
	parsedURL, err := url.Parse(tenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析租户URL失败"})
		return
	}
	hostName := parsedURL.Host

	// 创建请求
	requestURL := tenant + "chat-stream"
	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}

	// 设置请求头
	req.Header.Set("Host", hostName)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "augment.intellij/0.184.0 (Mac OS X; aarch64; 15.2) WebStorm/2024.3.5")
	req.Header.Set("x-api-version", "2")

	// 生成请求ID和会话ID
	requestID := uuid.New().String()
	sessionID := uuid.New().String()
	req.Header.Set("x-request-id", requestID)
	req.Header.Set("x-request-session-id", sessionID)

	client := createHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "请求失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		errMsg := "Augment response error"
		if err == nil {
			bodyStr := string(body)
			// 检查是否是HTML错误页面（Hugging Face的500错误页面）
			if strings.Contains(bodyStr, "<html>") || strings.Contains(bodyStr, "<!DOCTYPE") {
				errMsg = "Service temporarily unavailable (500 Internal Server Error)"
				logger.Log.WithFields(logrus.Fields{
					"status_code": resp.StatusCode,
					"response_preview": bodyStr[:min(200, len(bodyStr))],
				}).Error("收到HTML错误页面，可能是Hugging Face平台问题")
			} else {
				errMsg = errMsg + ": " + bodyStr
			}
		}
		c.JSON(resp.StatusCode, gin.H{"error": errMsg})
		return
	}

	// 异步记录会话事件
	asyncRecordSessionEvent(token, tenant, requestID, sessionID)

	// 读取完整响应
	reader := bufio.NewReader(resp.Body)
	var fullText string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取响应失败: " + err.Error()})
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var augmentResp AugmentResponse
		if err := json.Unmarshal([]byte(line), &augmentResp); err != nil {
			continue
		}

		fullText += augmentResp.Text

		// 检查响应内容是否包含错误信息
		if strings.Contains(augmentResp.Text, errBlocked) {
			// 获取token的个性化请求间隔作为冷却时间
			requestInterval := getTokenRequestInterval(token)
			cooldownDuration := time.Duration(requestInterval) * time.Second

			// 将当前token加入冷却队列，使用个性化冷却时间
			logger.Log.WithFields(logrus.Fields{
				"token": token,
				"mode":  augmentReq.Mode,
				"cooldown_seconds": requestInterval,
			}).Info("检测到block信息，将token加入冷却队列")

			err := SetTokenCoolStatus(token, cooldownDuration)
			if err != nil {
				logger.Log.WithFields(logrus.Fields{
					"token": token,
					"error": err.Error(),
				}).Error("将token加入冷却队列失败")
			}
		}

		if augmentResp.Done {
			break
		}
	}

	// 创建OpenAI兼容的响应
	finishReason := "stop"

	// 估算token数量
	promptTokens := estimateTokenCount(augmentReq.Message)
	for _, history := range augmentReq.ChatHistory {
		promptTokens += estimateTokenCount(history.RequestMessage)
		promptTokens += estimateTokenCount(history.ResponseText)
	}
	completionTokens := estimateTokenCount(fullText)

	openAIResp := OpenAIResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: fullText,
				},
				FinishReason: &finishReason,
			},
		},
		Usage: Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}

	c.JSON(http.StatusOK, openAIResp)
}

// 清理请求状态
func cleanupRequestStatus(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.WithFields(logrus.Fields{
				"error": r,
			}).Error("清理请求状态时发生panic")
		}
	}()

	// 获取锁和 token
	lockInterface, exists := c.Get("token_lock")
	if !exists {
		return
	}

	tokenInterface, exists := c.Get("token")
	if !exists {
		return
	}

	lock, ok := lockInterface.(*sync.Mutex)
	if !ok {
		return
	}

	token, ok := tokenInterface.(string)
	if !ok {
		return
	}

	// 更新请求状态为已完成
	err := SetTokenRequestStatus(token, TokenRequestStatus{
		InProgress:    false,
		LastRequestAt: time.Now(),
	})

	// 无论更新状态是否成功，都要释放锁
	defer lock.Unlock()

	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Error("清理请求状态失败")
		return
	}
}

// 创建 HTTP 客户端，如果配置了代理则使用
func createHTTPClient() *http.Client {
	// 创建自定义Transport，强制使用HTTP/1.1
	transport := &http.Transport{
		ForceAttemptHTTP2:     false, // 禁用HTTP/2
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    false,
	}

	// 检查是否配置了代理
	if config.AppConfig.ProxyURL != "" {
		proxyURL, err := url.Parse(config.AppConfig.ProxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
			log.Printf("使用代理: %s", config.AppConfig.ProxyURL)
		} else {
			log.Printf("代理URL格式错误: %v", err)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second, // 设置总超时时间
	}

	return client
}

// 异步记录用户会话事件
func asyncRecordSessionEvent(token, tenantURL, requestID, sessionID string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Log.WithFields(logrus.Fields{
					"error":      r,
					"token":      token,
					"tenant_url": tenantURL,
				}).Error("记录会话事件时发生panic")
			}
		}()

		// 提取主机部分
		parsedURL, err := url.Parse(tenantURL)
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"error":      err.Error(),
				"tenant_url": tenantURL,
			}).Error("解析租户URL失败")
			return
		}
		hostName := parsedURL.Host

		// 构建事件数据
		currentTime := time.Now()
		eventData := map[string]interface{}{
			"events": []map[string]interface{}{
				{
					"event_name":      "used-chat",
					"event_time_sec":  currentTime.Unix(),
					"event_time_nsec": currentTime.UnixNano() % 1000000000,
				},
			},
		}

		// 序列化请求数据
		jsonData, err := json.Marshal(eventData)
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Error("序列化事件数据失败")
			return
		}

		// 构建请求URL
		requestURL := tenantURL + "record-onboarding-session-event"

		// 创建请求
		req, err := http.NewRequest("POST", requestURL, bytes.NewReader(jsonData))
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Error("创建记录事件请求失败")
			return
		}

		// 设置请求头
		req.Header.Set("Host", hostName)
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonData)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "augment.intellij/0.184.0 (Mac OS X; aarch64; 15.2) WebStorm/2024.3.5")
		req.Header.Set("x-api-version", "2")
		req.Header.Set("x-request-id", requestID)
		req.Header.Set("x-request-session-id", sessionID)
		req.Header.Set("Accept-Charset", "UTF-8")

		// 发送请求
		client := createHTTPClient()
		resp, err := client.Do(req)
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Error("发送记录事件请求失败")
			return
		}
		defer resp.Body.Close()

		// 记录响应状态
		logger.Log.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"tenant_url":  tenantURL,
		}).Info("记录会话事件完成")
	}()
}

// 在处理聊天请求时增加token使用计数
func incrementTokenUsage(token string, model string) {
	// 先将模型名称转换为小写
	modelLower := strings.ToLower(model)

	// 根据模型类型确定计数键 (不区分大小写)
	var countKey string
	if strings.HasSuffix(modelLower, "-chat") {
		countKey = "token_usage_chat:" + token
	} else if strings.HasSuffix(modelLower, "-agent") {
		countKey = "token_usage_agent:" + token
	} else {
		countKey = "token_usage:" + token // 默认键
		// 非特定结尾的模型，增加chat计数
		err := config.RedisIncr("token_usage_chat:" + token)
		if err != nil {
			logger.Log.Error("增加token chat使用计数失败: %v", err)
		}
	}

	// 使用Redis的INCR命令增加计数
	err := config.RedisIncr(countKey)
	if err != nil {
		logger.Log.Error("增加token使用计数失败: %v", err)
	}

	// 同时增加总使用计数
	totalCountKey := "token_usage:" + token
	if countKey != totalCountKey { // 避免重复计数
		err = config.RedisIncr(totalCountKey)
		if err != nil {
			logger.Log.Error("增加token总使用计数失败: %v", err)
		}
	}
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
