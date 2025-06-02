/**
 * Augment2API 专用 Cloudflare Workers 反代脚本
 * 简化版本，专门为避免IP风控而设计
 * 
 * 使用方法：
 * 1. 修改下面的 BACKEND_URL 为您的 Augment2API 后端地址
 * 2. 部署到 Cloudflare Workers
 * 3. 在 Augment2API 控制面板的"代理URL"中填入此 Worker 的地址
 */

// 🔧 配置区域 - 请修改为您的实际后端地址
const BACKEND_URL = 'https://linjinpeng-augment.hf.space';

// 🛡️ 安全配置
const ALLOWED_PATHS = [
  '/v1/',           // OpenAI API 路径
  '/api/',          // Augment API 路径
  '/health',        // 健康检查
  '/status'         // 状态检查
];

const CORS_HEADERS = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS, HEAD',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-Requested-With, Accept, Origin, User-Agent, X-API-Key',
  'Access-Control-Max-Age': '86400',
};

/**
 * 🚀 主处理函数
 */
export default {
  async fetch(request, env, ctx) {
    try {
      // 处理 CORS 预检请求
      if (request.method === 'OPTIONS') {
        return new Response(null, {
          status: 204,
          headers: CORS_HEADERS
        });
      }

      // 解析请求URL
      const url = new URL(request.url);
      
      // 健康检查端点
      if (url.pathname === '/health' || url.pathname === '/ping') {
        return new Response(JSON.stringify({
          status: 'healthy',
          proxy_target: BACKEND_URL,
          timestamp: new Date().toISOString(),
          cf_ray: request.headers.get('CF-Ray') || 'unknown'
        }), {
          status: 200,
          headers: {
            'Content-Type': 'application/json',
            ...CORS_HEADERS
          }
        });
      }

      // 验证路径权限
      const isAllowed = ALLOWED_PATHS.some(path => url.pathname.startsWith(path));
      if (!isAllowed) {
        return createErrorResponse('Path not allowed', 403);
      }

      // 构建目标URL
      const targetUrl = BACKEND_URL + url.pathname + url.search;

      // 创建代理请求
      const proxyHeaders = new Headers();
      
      // 复制原始请求头（过滤CF特有头）
      for (const [key, value] of request.headers.entries()) {
        if (!key.toLowerCase().startsWith('cf-') && key.toLowerCase() !== 'host') {
          proxyHeaders.set(key, value);
        }
      }

      // 添加代理标识和真实IP
      proxyHeaders.set('User-Agent', 'Augment-CF-Proxy/1.0');
      proxyHeaders.set('X-Forwarded-For', request.headers.get('CF-Connecting-IP') || 'unknown');
      proxyHeaders.set('X-Real-IP', request.headers.get('CF-Connecting-IP') || 'unknown');
      proxyHeaders.set('X-Forwarded-Proto', 'https');
      
      // 添加国家代码（如果可用）
      const country = request.headers.get('CF-IPCountry');
      if (country) {
        proxyHeaders.set('X-Country-Code', country);
      }

      // 创建代理请求配置
      const proxyRequestInit = {
        method: request.method,
        headers: proxyHeaders,
      };

      // 复制请求体（如果有）
      if (request.method !== 'GET' && request.method !== 'HEAD') {
        proxyRequestInit.body = request.body;
      }

      // 发送代理请求
      const response = await fetch(targetUrl, proxyRequestInit);

      // 创建响应头
      const responseHeaders = new Headers(response.headers);
      
      // 添加CORS头
      Object.entries(CORS_HEADERS).forEach(([key, value]) => {
        responseHeaders.set(key, value);
      });

      // 添加代理信息头
      responseHeaders.set('X-Proxy-By', 'Cloudflare-Workers');
      responseHeaders.set('X-Proxy-Target', BACKEND_URL);
      responseHeaders.set('X-Proxy-Time', new Date().toISOString());
      responseHeaders.set('X-CF-Ray', request.headers.get('CF-Ray') || 'unknown');

      // 返回代理响应
      return new Response(response.body, {
        status: response.status,
        statusText: response.statusText,
        headers: responseHeaders
      });

    } catch (error) {
      console.error('Proxy error:', error);
      return createErrorResponse(`Proxy error: ${error.message}`, 500);
    }
  }
};

/**
 * 🚨 创建错误响应
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
 * 📝 使用说明：
 * 
 * 1. 部署步骤：
 *    - 登录 Cloudflare Dashboard
 *    - 创建新的 Worker
 *    - 复制此代码到编辑器
 *    - 修改第10行的 BACKEND_URL
 *    - 保存并部署
 * 
 * 2. 配置 Augment2API：
 *    - 登录管理面板
 *    - 进入"系统配置"
 *    - 设置"proxy_url"为您的Worker地址
 *    - 保存配置
 * 
 * 3. 测试：
 *    - 访问 https://your-worker.workers.dev/health
 *    - 应该返回健康状态信息
 * 
 * 4. 优势：
 *    ✅ 利用 Cloudflare 的全球IP池
 *    ✅ 避免后端IP被风控
 *    ✅ 自动CORS处理
 *    ✅ 请求头清理和转发
 *    ✅ 错误处理和日志
 *    ✅ 简单易部署
 * 
 * 注意事项：
 * - 确保 BACKEND_URL 可以从互联网访问
 * - Worker 有每日10万次免费请求限制
 * - 单次请求最大6MB，响应最大100MB
 * - CPU时间限制：免费版10ms，付费版50ms
 */
