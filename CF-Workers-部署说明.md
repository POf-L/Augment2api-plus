# Cloudflare Workers 反代脚本部署说明

## 📋 概述

为了避免 Augment API 被风控IP，我们提供了两个 Cloudflare Workers 反代脚本：

1. **基础版本** (`cloudflare-worker-proxy.js`) - 简单反代
2. **高级版本** (`cloudflare-worker-advanced-proxy.js`) - 支持负载均衡、重试、故障转移

## 🚀 部署步骤

### 1. 创建 Cloudflare Workers

1. 登录 [Cloudflare Dashboard](https://dash.cloudflare.com/)
2. 选择您的域名（或使用 workers.dev 子域名）
3. 点击左侧菜单 "Workers & Pages"
4. 点击 "Create application"
5. 选择 "Create Worker"
6. 给 Worker 起个名字，如：`augment-proxy`

### 2. 部署代码

#### 基础版本部署：
1. 复制 `cloudflare-worker-proxy.js` 的内容
2. 粘贴到 Worker 编辑器中
3. **重要**：修改第12行的 `TARGET_HOST` 为您的后端地址：
   ```javascript
   const TARGET_HOST = 'https://your-augment2api-backend.com';
   ```
4. 点击 "Save and Deploy"

#### 高级版本部署：
1. 复制 `cloudflare-worker-advanced-proxy.js` 的内容
2. 粘贴到 Worker 编辑器中
3. **重要**：修改第11-15行的 `BACKEND_SERVERS` 数组：
   ```javascript
   const BACKEND_SERVERS = [
     'https://your-primary-backend.com',
     'https://your-secondary-backend.com',
     // 可以添加更多后端服务器
   ];
   ```
4. 点击 "Save and Deploy"

### 3. 获取 Worker URL

部署成功后，您会得到一个类似这样的URL：
```
https://augment-proxy.your-username.workers.dev
```

### 4. 在 Augment2API 中配置

1. 登录您的 Augment2API 管理面板
2. 进入 "系统配置" 页面
3. 找到 "proxy_url" 配置项
4. 填入您的 Worker URL：`https://augment-proxy.your-username.workers.dev`
5. 保存配置

## ⚙️ 配置说明

### 基础版本配置

```javascript
// 目标后端地址
const TARGET_HOST = 'https://your-backend.com';

// 允许的API路径
const ALLOWED_PATHS = [
  '/v1/chat/completions',
  '/v1/completions', 
  '/v1/models',
  '/v1/embeddings',
  '/health',
  '/status'
];
```

### 高级版本配置

```javascript
// 多个后端服务器（负载均衡）
const BACKEND_SERVERS = [
  'https://backend1.com',
  'https://backend2.com',
  'https://backend3.com'
];

// 重试配置
const RETRY_CONFIG = {
  maxRetries: 3,           // 最大重试次数
  retryDelay: 1000,        // 重试延迟（毫秒）
  retryOn: [502, 503, 504] // 需要重试的HTTP状态码
};

// 请求超时时间
const TIMEOUT_MS = 30000; // 30秒
```

## 🔧 功能特性

### 基础版本功能：
- ✅ 简单HTTP代理
- ✅ CORS支持
- ✅ 路径白名单控制
- ✅ 基础错误处理

### 高级版本功能：
- ✅ **负载均衡** - 多后端轮询
- ✅ **故障转移** - 自动切换后端
- ✅ **重试机制** - 失败自动重试
- ✅ **超时控制** - 防止请求卡死
- ✅ **健康检查** - `/health` 端点
- ✅ **详细日志** - 便于调试
- ✅ **性能监控** - 响应时间统计

## 🛡️ 安全特性

1. **路径白名单** - 只允许特定API路径
2. **方法限制** - 只允许安全的HTTP方法
3. **头部过滤** - 过滤敏感请求头
4. **CORS控制** - 严格的跨域策略

## 📊 监控和调试

### 健康检查
访问 `https://your-worker.workers.dev/health` 查看状态：

```json
{
  "status": "healthy",
  "timestamp": "2025-06-02T08:00:00.000Z",
  "backends": ["https://backend1.com"],
  "version": "2.0.0",
  "features": ["load_balancing", "retry", "timeout", "cors"]
}
```

### 查看日志
在 Cloudflare Dashboard 中：
1. 进入您的 Worker
2. 点击 "Logs" 标签
3. 查看实时日志和错误信息

## 🔄 使用方式

配置完成后，您的API请求会自动通过Cloudflare Workers代理：

```bash
# 原始请求
curl https://your-backend.com/v1/chat/completions

# 通过CF Workers代理的请求
curl https://your-worker.workers.dev/v1/chat/completions
```

## 💡 优化建议

1. **多地域部署** - 在不同地区部署多个Worker
2. **缓存策略** - 对静态响应启用缓存
3. **速率限制** - 添加请求频率控制
4. **监控告警** - 设置错误率监控

## 🆘 常见问题

### Q: Worker 返回 403 错误？
A: 检查 `ALLOWED_PATHS` 配置，确保您的API路径在白名单中。

### Q: 请求超时？
A: 调整 `TIMEOUT_MS` 配置，或检查后端服务器响应速度。

### Q: 负载均衡不工作？
A: 确保 `BACKEND_SERVERS` 数组中的所有URL都是有效的。

### Q: CORS 错误？
A: 检查 `CORS_HEADERS` 配置，确保包含了您需要的头部。

## 📞 技术支持

如果遇到问题，请检查：
1. Worker 部署日志
2. 后端服务器状态
3. 网络连接情况
4. API路径配置

---

**注意**: 请确保您的后端服务器地址正确，并且可以从互联网访问。Cloudflare Workers 会使用其全球IP池来访问您的后端，有效避免IP被风控的问题。
