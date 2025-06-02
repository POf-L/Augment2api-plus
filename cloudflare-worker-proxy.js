/**
 * Cloudflare Workers 反代脚本 - Augment2API 专用
 * 用于代理访问 Augment API，避免IP风控
 * 
 * 部署说明：
 * 1. 在 Cloudflare Workers 中创建新的 Worker
 * 2. 将此代码复制到 Worker 编辑器中
 * 3. 修改下面的 TARGET_HOST 为您的实际后端地址
 * 4. 保存并部署
 * 5. 在 Augment2API 控制面板的"代理URL"中填入 Worker 地址
 */

// 配置目标后端地址 - 请修改为您的实际后端地址
const TARGET_HOST = 'https://your-augment2api-backend.com';

// 允许的路径前缀（安全控制）
const ALLOWED_PATHS = [
  '/v1/chat/completions',
  '/v1/completions', 
  '/v1/models',
  '/v1/embeddings',
  '/health',
  '/status'
];

// 支持的HTTP方法
const ALLOWED_METHODS = ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS', 'HEAD'];

// CORS 配置
const CORS_HEADERS = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS, HEAD',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-Requested-With, Accept, Origin, User-Agent',
  'Access-Control-Max-Age': '86400',
};

/**
 * 主处理函数
 */
export default {
  async fetch(request, env, ctx) {
    try {
      // 处理 CORS 预检请求
      if (request.method === 'OPTIONS') {
        return handleCORS();
      }

      // 验证请求方法
      if (!ALLOWED_METHODS.includes(request.method)) {
        return createErrorResponse('Method not allowed', 405);
      }

      // 解析请求URL
      const url = new URL(request.url);
      const path = url.pathname;

      // 验证路径权限
      if (!isPathAllowed(path)) {
        return createErrorResponse('Path not allowed', 403);
      }

      // 构建目标URL
      const targetUrl = TARGET_HOST + path + url.search;

      // 创建代理请求
      const proxyRequest = await createProxyRequest(request, targetUrl);

      // 发送请求到目标服务器
      const response = await fetch(proxyRequest);

      // 处理响应
      return await createProxyResponse(response);

    } catch (error) {
      console.error('Proxy error:', error);
      return createErrorResponse('Internal server error', 500);
    }
  }
};

/**
 * 检查路径是否被允许
 */
function isPathAllowed(path) {
  return ALLOWED_PATHS.some(allowedPath => path.startsWith(allowedPath));
}

/**
 * 处理 CORS 预检请求
 */
function handleCORS() {
  return new Response(null, {
    status: 204,
    headers: CORS_HEADERS
  });
}

/**
 * 创建代理请求
 */
async function createProxyRequest(originalRequest, targetUrl) {
  // 复制请求头，过滤掉一些不需要的头
  const headers = new Headers();
  
  for (const [key, value] of originalRequest.headers.entries()) {
    // 跳过一些 Cloudflare 特有的头和可能导致问题的头
    if (!shouldSkipHeader(key)) {
      headers.set(key, value);
    }
  }

  // 添加一些有用的头
  headers.set('User-Agent', 'Cloudflare-Workers-Proxy/1.0');
  headers.set('X-Forwarded-For', originalRequest.headers.get('CF-Connecting-IP') || 'unknown');
  headers.set('X-Real-IP', originalRequest.headers.get('CF-Connecting-IP') || 'unknown');

  // 创建请求配置
  const requestInit = {
    method: originalRequest.method,
    headers: headers,
  };

  // 如果有请求体，复制它
  if (originalRequest.method !== 'GET' && originalRequest.method !== 'HEAD') {
    requestInit.body = originalRequest.body;
  }

  return new Request(targetUrl, requestInit);
}

/**
 * 判断是否应该跳过某个请求头
 */
function shouldSkipHeader(headerName) {
  const skipHeaders = [
    'cf-ray',
    'cf-connecting-ip',
    'cf-ipcountry',
    'cf-visitor',
    'x-forwarded-proto',
    'x-forwarded-for',
    'host'
  ];
  
  return skipHeaders.includes(headerName.toLowerCase());
}

/**
 * 创建代理响应
 */
async function createProxyResponse(response) {
  // 复制响应头
  const headers = new Headers(response.headers);
  
  // 添加 CORS 头
  Object.entries(CORS_HEADERS).forEach(([key, value]) => {
    headers.set(key, value);
  });

  // 添加一些有用的头
  headers.set('X-Proxy-By', 'Cloudflare-Workers');
  headers.set('X-Proxy-Time', new Date().toISOString());

  // 创建新的响应
  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers: headers
  });
}

/**
 * 创建错误响应
 */
function createErrorResponse(message, status = 400) {
  const errorBody = {
    error: {
      message: message,
      type: 'proxy_error',
      code: status
    }
  };

  return new Response(JSON.stringify(errorBody), {
    status: status,
    headers: {
      'Content-Type': 'application/json',
      ...CORS_HEADERS
    }
  });
}

/**
 * 健康检查端点（可选）
 */
function handleHealthCheck() {
  const healthInfo = {
    status: 'healthy',
    timestamp: new Date().toISOString(),
    proxy_target: TARGET_HOST,
    version: '1.0.0'
  };

  return new Response(JSON.stringify(healthInfo), {
    status: 200,
    headers: {
      'Content-Type': 'application/json',
      ...CORS_HEADERS
    }
  });
}
