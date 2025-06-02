/**
 * Cloudflare Workers 高级反代脚本 - Augment2API 专用
 * 支持多后端负载均衡、健康检查、重试机制
 * 
 * 功能特性：
 * - 多后端负载均衡
 * - 自动故障转移
 * - 请求重试机制
 * - IP轮换（利用CF的IP池）
 * - 详细的错误处理和日志
 */

// 配置多个后端服务器（负载均衡）
const BACKEND_SERVERS = [
  'https://your-primary-backend.com',
  'https://your-secondary-backend.com',
  // 可以添加更多后端服务器
];

// 当前使用的后端索引（简单轮询）
let currentBackendIndex = 0;

// 重试配置
const RETRY_CONFIG = {
  maxRetries: 3,
  retryDelay: 1000, // 毫秒
  retryOn: [502, 503, 504, 520, 521, 522, 523, 524] // 需要重试的状态码
};

// 超时配置
const TIMEOUT_MS = 30000; // 30秒

// 允许的路径和方法（安全控制）
const ALLOWED_PATHS = [
  '/v1/chat/completions',
  '/v1/completions',
  '/v1/models',
  '/v1/embeddings',
  '/health',
  '/status',
  '/api/' // 允许所有API路径
];

const ALLOWED_METHODS = ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS', 'HEAD'];

// CORS 配置
const CORS_HEADERS = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS, HEAD',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-Requested-With, Accept, Origin, User-Agent, X-API-Key',
  'Access-Control-Max-Age': '86400',
};

/**
 * 主处理函数
 */
export default {
  async fetch(request, env, ctx) {
    const startTime = Date.now();
    
    try {
      // 处理 CORS 预检请求
      if (request.method === 'OPTIONS') {
        return handleCORS();
      }

      // 健康检查端点
      const url = new URL(request.url);
      if (url.pathname === '/health' || url.pathname === '/ping') {
        return handleHealthCheck();
      }

      // 验证请求
      const validationError = validateRequest(request);
      if (validationError) {
        return validationError;
      }

      // 执行代理请求（带重试机制）
      const response = await proxyRequestWithRetry(request);
      
      // 添加性能头
      const processingTime = Date.now() - startTime;
      response.headers.set('X-Processing-Time', `${processingTime}ms`);
      
      return response;

    } catch (error) {
      console.error('Proxy error:', error);
      return createErrorResponse('Internal server error: ' + error.message, 500);
    }
  }
};

/**
 * 验证请求
 */
function validateRequest(request) {
  // 验证HTTP方法
  if (!ALLOWED_METHODS.includes(request.method)) {
    return createErrorResponse('Method not allowed', 405);
  }

  // 验证路径
  const url = new URL(request.url);
  const path = url.pathname;
  
  if (!isPathAllowed(path)) {
    return createErrorResponse('Path not allowed', 403);
  }

  return null;
}

/**
 * 检查路径是否被允许
 */
function isPathAllowed(path) {
  return ALLOWED_PATHS.some(allowedPath => path.startsWith(allowedPath));
}

/**
 * 带重试机制的代理请求
 */
async function proxyRequestWithRetry(request) {
  let lastError;
  
  for (let attempt = 0; attempt <= RETRY_CONFIG.maxRetries; attempt++) {
    try {
      // 选择后端服务器
      const backendUrl = selectBackend();
      
      // 执行代理请求
      const response = await proxyToBackend(request, backendUrl);
      
      // 检查是否需要重试
      if (shouldRetry(response.status) && attempt < RETRY_CONFIG.maxRetries) {
        console.log(`Attempt ${attempt + 1} failed with status ${response.status}, retrying...`);
        await sleep(RETRY_CONFIG.retryDelay * (attempt + 1)); // 指数退避
        continue;
      }
      
      return response;
      
    } catch (error) {
      lastError = error;
      console.error(`Attempt ${attempt + 1} failed:`, error);
      
      if (attempt < RETRY_CONFIG.maxRetries) {
        await sleep(RETRY_CONFIG.retryDelay * (attempt + 1));
        continue;
      }
    }
  }
  
  throw lastError || new Error('All retry attempts failed');
}

/**
 * 选择后端服务器（简单轮询）
 */
function selectBackend() {
  if (BACKEND_SERVERS.length === 0) {
    throw new Error('No backend servers configured');
  }
  
  const backend = BACKEND_SERVERS[currentBackendIndex];
  currentBackendIndex = (currentBackendIndex + 1) % BACKEND_SERVERS.length;
  
  return backend;
}

/**
 * 代理到指定后端
 */
async function proxyToBackend(request, backendUrl) {
  const url = new URL(request.url);
  const targetUrl = backendUrl + url.pathname + url.search;
  
  // 创建代理请求
  const proxyRequest = await createProxyRequest(request, targetUrl);
  
  // 添加超时控制
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);
  
  try {
    const response = await fetch(proxyRequest, {
      signal: controller.signal
    });
    
    clearTimeout(timeoutId);
    return await createProxyResponse(response, backendUrl);
    
  } catch (error) {
    clearTimeout(timeoutId);
    if (error.name === 'AbortError') {
      throw new Error('Request timeout');
    }
    throw error;
  }
}

/**
 * 创建代理请求
 */
async function createProxyRequest(originalRequest, targetUrl) {
  const headers = new Headers();
  
  // 复制原始请求头
  for (const [key, value] of originalRequest.headers.entries()) {
    if (!shouldSkipHeader(key)) {
      headers.set(key, value);
    }
  }

  // 添加代理相关头
  headers.set('User-Agent', 'Cloudflare-Workers-Proxy/2.0');
  headers.set('X-Forwarded-For', originalRequest.headers.get('CF-Connecting-IP') || 'unknown');
  headers.set('X-Real-IP', originalRequest.headers.get('CF-Connecting-IP') || 'unknown');
  headers.set('X-Forwarded-Proto', 'https');
  
  // 添加CF特有信息
  const cfCountry = originalRequest.headers.get('CF-IPCountry');
  if (cfCountry) {
    headers.set('X-Country-Code', cfCountry);
  }

  const requestInit = {
    method: originalRequest.method,
    headers: headers,
  };

  // 复制请求体
  if (originalRequest.method !== 'GET' && originalRequest.method !== 'HEAD') {
    requestInit.body = originalRequest.body;
  }

  return new Request(targetUrl, requestInit);
}

/**
 * 判断是否应该跳过请求头
 */
function shouldSkipHeader(headerName) {
  const skipHeaders = [
    'cf-ray', 'cf-connecting-ip', 'cf-ipcountry', 'cf-visitor',
    'x-forwarded-proto', 'x-forwarded-for', 'host', 'content-length'
  ];
  
  return skipHeaders.includes(headerName.toLowerCase());
}

/**
 * 创建代理响应
 */
async function createProxyResponse(response, backendUrl) {
  const headers = new Headers(response.headers);
  
  // 添加 CORS 头
  Object.entries(CORS_HEADERS).forEach(([key, value]) => {
    headers.set(key, value);
  });

  // 添加代理信息头
  headers.set('X-Proxy-By', 'Cloudflare-Workers');
  headers.set('X-Proxy-Backend', backendUrl);
  headers.set('X-Proxy-Time', new Date().toISOString());
  headers.set('X-CF-Ray', crypto.randomUUID());

  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers: headers
  });
}

/**
 * 判断是否需要重试
 */
function shouldRetry(status) {
  return RETRY_CONFIG.retryOn.includes(status);
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
 * 健康检查
 */
function handleHealthCheck() {
  const healthInfo = {
    status: 'healthy',
    timestamp: new Date().toISOString(),
    backends: BACKEND_SERVERS,
    current_backend_index: currentBackendIndex,
    version: '2.0.0',
    features: ['load_balancing', 'retry', 'timeout', 'cors']
  };

  return new Response(JSON.stringify(healthInfo, null, 2), {
    status: 200,
    headers: {
      'Content-Type': 'application/json',
      ...CORS_HEADERS
    }
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
      code: status,
      timestamp: new Date().toISOString()
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
 * 延迟函数
 */
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}
