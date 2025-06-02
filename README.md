---
title: Augment2API Token Manager
emoji: 🔧
colorFrom: blue
colorTo: purple
sdk: docker
sdk_version: "20.10"
app_file: main.go
pinned: false
---

# Augment2API

一个基于 Redis 的 Augment API 代理服务，支持多 token 管理、并发控制和使用统计。

## 🚀 新功能特性

### 独立Token控制系统
- ✅ **独立启用/禁用控制**: 每个token可以单独启用或禁用
- ⏱️ **独立请求频率控制**: 每个token可设置不同的请求间隔（1-3600秒）
- 📊 **独立用量限制**:
  - CHAT模式调用上限（默认3000次）
  - AGENT模式调用上限（默认50次）
  - 每日调用上限（默认1000次，每天自动重置）
- 📈 **实时用量监控**: 显示今日使用情况和各种限制状态

### 核心功能
- 🔐 **多 Token 管理**: 支持添加、删除、查看多个 Augment token
- 🚦 **智能并发控制**: 根据token配置智能选择可用token
- 📊 **详细使用统计**: 累计统计和每日统计分离
- 🌐 **Web 管理界面**: 直观的控制面板
- 🔄 **自动轮换**: 智能的 token 轮换机制
- 📝 **备注功能**: 为每个 token 添加备注信息
- 🛡️ **安全认证**: 基于密码的访问控制

## 使用须知

- 使用本项目可能导致您的账号被标记、风控或封禁，请自行承担风险！
- 默认根据传入模型名称确定使用使用模式，`AGENT模式`下屏蔽所有工具调用，使用模型原生能力回答，否则对话会被工具调用截断
- 支持独立的Token频率控制，每个token可设置不同的请求间隔
- `Augment`的`Agent`模式很强，推荐你在编辑器中使用官方插件，体验不输`Cursor`

## 支持模型
```bash
传入模型名称以 -chat 结尾,使用CHAT模式回复

传入模型名称以 -agent 结尾,使用AGENT模式回复

其他模型名称默认使用CHAT模式
```

## 环境变量配置

| 环境变量              | 说明             | 是否必填 | 示例                                        |
|-------------------|----------------|------|-------------------------------------------|
| REDIS_CONN_STRING | Redis 连接字符串    | 是    | `redis://default:password@localhost:6379` |
| ACCESS_PWD        | 管理面板访问密码       | 是    | `your-access-password`                    |
| AUTH_TOKEN        | API 访问认证 Token | 否    | `your-auth-token`                         |
| ROUTE_PREFIX      | API 请求前缀       | 否    | `your_api_prefix`                         |
| CODING_MODE       | 调试模式开关         | 否    | `false`                                   |
| CODING_TOKEN      | 调试使用Token      | 否    | `空`                                       |
| TENANT_URL        | 调试使用租户地址       | 否    | `空`                                       |
| PROXY_URL         | HTTP代理地址       | 否    | `http://127.0.0.1:7890`                   |

提示：如果页面获取Token失败，可以配置`CODING_MODE`为true,同时配置`CODING_TOKEN`和`TENANT_URL`即可使用指定Token和租户地址，仅限单个Token

## 快速开始

### 1. 部署

#### 使用 Docker 运行

```bash
docker run -d \
  --name augment2api \
  -p 27080:27080 \
  -e REDIS_CONN_STRING="redis://default:yourpassword@your-redis-host:6379" \
  -e ACCESS_PWD="your-access-password" \
  -e AUTH_TOKEN="your-auth-token" \
  --restart always \
  linqiu1199/augment2api
```

#### 使用 Docker Compose 运行

拉取项目到本地

```bash
git clone https://github.com/linqiu1199/augment2api.git
```

进入项目目录

```bash
cd augment2api
```

创建 `.env` 文件，填写下面两个环境变量：

```
# 设置Redis密码 必填
REDIS_PASSWORD=your-redis-password

# 设置面板访问密码 必填
ACCESS_PWD=your-access-password

# 设置api鉴权token 非必填
AUTH_TOKEN=your-auth-token

```

然后运行：

```bash
docker-compose up -d
```

这将同时启动 Redis 和 Augment2Api 服务，并自动处理它们之间的网络连接。

### 2. 获取和配置Token

访问 `http://ip:27080/` 进入管理页面登录页,输入访问密码进入管理面板，点击`添加TOKEN`菜单

1. 点击获取授权链接
2. 复制授权链接到浏览器中打开
3. 使用邮箱进行登录（域名邮箱也可）
4. 复制`augment code`到授权响应输入框中，点击获取token，TOKEN列表中正常出现数据
5. 配置Token独立设置：
   - **启用/禁用**: 使用开关控制token状态
   - **请求间隔**: 设置1-3600秒的请求间隔
   - **CHAT限制**: 设置CHAT模式调用上限
   - **AGENT限制**: 设置AGENT模式调用上限
   - **每日限制**: 设置每日总调用上限
6. 点击"保存设置"应用配置
7. 开始对话测试

### Token控制功能详解

#### 🔧 独立控制面板
每个token都有独立的控制面板，包含：
- 启用/禁用开关
- 请求间隔设置（秒）
- 各种用量限制设置
- 实时使用统计显示

#### 📊 用量统计说明
- **累计统计**: CHAT/AGENT模式的总调用次数（不重置）
- **每日统计**: 每天的调用次数（每天凌晨自动重置）
- **实时显示**: 今日使用进度和剩余额度

#### ⚡ 智能选择逻辑
系统会自动选择符合以下条件的token：
- ✅ 已启用状态
- ⏱️ 满足请求间隔要求
- 📊 未超过各种用量限制
- 🔄 不在冷却期

提示：
* 如果对话报错503，请执行一次`批量检测`更新租户地址再进行对话测试（租户地址错误）
* 如果对话报错401，请执行一次`批量检测`禁用无效token再进行对话测试 （账号被封禁）
* 如果所有token都不可用，请检查token的启用状态和用量限制设置

## API 使用

### 获取模型

```bash
curl -X GET http://localhost:27080/v1/models
```

### 聊天接口

```bash
curl -X POST http://localhost:27080/v1/chat/completions \
-H "Content-Type: application/json" \
-d '{
"model": "claude-3.7",
"messages": [
{"role": "user", "content": "你好，请介绍一下自己"}
]
}'
```

## 管理界面

访问 `http://localhost:27080/` 可以打开管理界面登录页面，登录之后即可交互式获取、管理Token。

## 批量添加Token

```bash
# 批量添加Token-未设置AUTH_TOKEN
curl -X POST http://localhost:27080/api/add/tokens \
-H "Content-Type: application/json" \
-d '[
    {
        "token": "token1",
        "tenantUrl": "https://tenant1.com"
    },
    {
        "token": "token2",
        "tenantUrl": "https://tenant2.com"
    }
]'
```

```bash   
# 批量添加Token-设置AUTH_TOKEN
curl -X POST http://localhost:27080/api/add/tokens \
-H "Content-Type: application/json" \
-H "Authorization: Bearer your-auth-token" \
-d '[
    {
        "token": "token1",
        "tenantUrl": "https://tenant1.com"
    },
    {
        "token": "token2",
        "tenantUrl": "https://tenant2.com"
    }
]'    
```

## Star History

<a href="https://www.star-history.com/#linqiu919/augment2api&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=linqiu919/augment2api&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=linqiu919/augment2api&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=linqiu919/augment2api&type=Date" />
 </picture>
</a>
