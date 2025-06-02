# Cloudflare Workers 反代功能使用说明

## 📋 功能概述

Cloudflare Workers反代功能允许您通过Cloudflare的全球IP池来访问Augment2API服务，有效避免IP限制问题。

## 🚀 快速开始

### 1. 部署Cloudflare Workers脚本

1. 登录到您的 [Cloudflare Dashboard](https://dash.cloudflare.com/)
2. 进入 **Workers & Pages** 页面
3. 点击 **Create application** → **Create Worker**
4. 将项目根目录下的 `cloudflare-worker-simple.js` 脚本内容复制到编辑器中
5. 修改脚本中的 `BACKEND_URL` 为您的Augment2API服务地址：
   ```javascript
   const BACKEND_URL = 'https://linjinpeng-augment.hf.space';
   ```
6. 点击 **Save and Deploy** 部署脚本
7. 记录您的Workers URL（例如：`https://your-worker.your-subdomain.workers.dev`）

### 2. 在管理面板中配置

1. 登录Augment2API管理面板
2. 点击左侧菜单的 **CF Workers**
3. 点击 **添加Workers** 按钮
4. 填写以下信息：
   - **Workers名称**：为您的Workers起一个便于识别的名称
   - **Workers URL**：步骤1中获得的Workers URL
   - **描述**：可选，添加一些说明信息
5. 点击确认添加

### 3. 测试连接

1. 在CF Workers列表中，点击对应Workers的 **测试** 按钮
2. 输入测试路径（默认：`/v1/models`）
3. 查看测试结果，确保连接正常

## 🔧 使用方式

配置完成后，您可以通过以下方式使用：

### 直接访问
将原来的API请求地址替换为您的Workers URL：

**原地址**：
```
https://linjinpeng-augment.hf.space/v1/chat/completions
```

**Workers地址**：
```
https://your-worker.your-subdomain.workers.dev/v1/chat/completions
```

### API调用示例

```bash
curl -X POST "https://your-worker.your-subdomain.workers.dev/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "model": "claude-3.7-agent",
    "messages": [
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ]
  }'
```

## ⚙️ 脚本配置说明

### 基础配置

```javascript
// 后端服务器地址
const BACKEND_URL = 'https://linjinpeng-augment.hf.space';

// 允许的路径前缀（可选）
const ALLOWED_PATHS = [
  '/v1/',
  '/api/',
  '/login',
  '/admin'
];
```

### 高级配置

如果您需要更多自定义功能，可以修改脚本：

1. **路径过滤**：修改 `ALLOWED_PATHS` 数组来限制可访问的路径
2. **CORS设置**：修改 `CORS_HEADERS` 对象来调整跨域设置
3. **请求处理**：在 `handleRequest` 函数中添加自定义逻辑

## 🛠️ 管理功能

### 添加Workers
- 支持添加多个Workers配置
- 每个Workers可以有独立的名称和描述

### 测试连接
- 一键测试Workers连接状态
- 支持自定义测试路径
- 显示详细的测试结果

### 编辑和删除
- 支持编辑Workers配置（开发中）
- 支持删除不需要的Workers配置

## 🔍 故障排除

### 常见问题

1. **测试失败**
   - 检查Workers URL是否正确
   - 确认Workers脚本已正确部署
   - 检查后端服务是否正常运行

2. **CORS错误**
   - 确认脚本中的CORS设置正确
   - 检查请求头是否符合要求

3. **路径不匹配**
   - 检查 `ALLOWED_PATHS` 配置
   - 确认请求路径在允许列表中

### 调试方法

1. **查看Workers日志**
   - 在Cloudflare Dashboard中查看Workers的实时日志
   - 检查错误信息和请求详情

2. **测试后端连接**
   - 直接访问后端服务确认其正常运行
   - 使用浏览器开发者工具检查网络请求

## 📝 注意事项

1. **免费限制**：Cloudflare Workers免费版有请求次数限制
2. **延迟考虑**：通过Workers代理可能会增加少量延迟
3. **安全性**：确保不要在脚本中暴露敏感信息
4. **更新维护**：定期检查和更新Workers脚本

## 🎯 最佳实践

1. **命名规范**：使用有意义的Workers名称便于管理
2. **定期测试**：定期测试Workers连接确保服务正常
3. **监控使用**：关注Workers的使用量和性能
4. **备份配置**：保存好Workers配置以便恢复

## 📞 技术支持

如果您在使用过程中遇到问题，可以：

1. 查看项目文档和FAQ
2. 在GitHub Issues中提交问题
3. 联系技术支持团队

---

**提示**：这是一个简化版的Cloudflare Workers反代功能，专注于核心的代理功能和基础管理。如需更高级的功能（如负载均衡、监控统计等），可以考虑扩展开发。
