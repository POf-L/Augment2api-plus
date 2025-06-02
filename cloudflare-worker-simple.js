/**
 * Cloudflare Workers 简单反代脚本
 * 用于代理Augment2API请求，利用Cloudflare IP池避免限制
 */

// 配置你的后端服务器地址
const BACKEND_URL = 'https://linjinpeng-augment.hf.space';

// 允许的路径前缀（可选，留空表示代理所有请求）
const ALLOWED_PATHS = [
  '/v1/',
  '/api/',
  '/login',
  '/admin'
];

// CORS配置
const CORS_HEADERS = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-Auth-Token',
  'Access-Control-Max-Age': '86400',
};

addEventListener('fetch', event => {
  event.respondWith(handleRequest(event.request));
});

async function handleRequest(request) {
  const url = new URL(request.url);
  
  // 处理CORS预检请求
  if (request.method === 'OPTIONS') {
    return new Response(null, {
      status: 200,
      headers: CORS_HEADERS
    });
  }
  
  // 检查路径是否允许（如果配置了ALLOWED_PATHS）
  if (ALLOWED_PATHS.length > 0) {
    const isAllowed = ALLOWED_PATHS.some(path => url.pathname.startsWith(path));
    if (!isAllowed) {
      return new Response('Path not allowed', { 
        status: 403,
        headers: CORS_HEADERS
      });
    }
  }
  
  try {
    // 构建目标URL
    const targetUrl = BACKEND_URL + url.pathname + url.search;
    
    // 创建新的请求
    const modifiedRequest = new Request(targetUrl, {
      method: request.method,
      headers: request.headers,
      body: request.method !== 'GET' && request.method !== 'HEAD' ? request.body : null,
    });
    
    // 发送请求到后端
    const response = await fetch(modifiedRequest);
    
    // 创建新的响应，添加CORS头
    const modifiedResponse = new Response(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: {
        ...Object.fromEntries(response.headers),
        ...CORS_HEADERS
      }
    });
    
    return modifiedResponse;
    
  } catch (error) {
    console.error('Proxy error:', error);
    return new Response(JSON.stringify({
      error: 'Proxy request failed',
      message: error.message
    }), {
      status: 500,
      headers: {
        'Content-Type': 'application/json',
        ...CORS_HEADERS
      }
    });
  }
}

// 健康检查端点
async function healthCheck() {
  try {
    const response = await fetch(BACKEND_URL + '/v1/models', {
      method: 'GET',
      headers: {
        'User-Agent': 'Cloudflare-Workers-Health-Check'
      }
    });
    
    return {
      status: 'healthy',
      backend_status: response.status,
      timestamp: new Date().toISOString()
    };
  } catch (error) {
    return {
      status: 'unhealthy',
      error: error.message,
      timestamp: new Date().toISOString()
    };
  }
}
