import Fastify from 'fastify';
import { config } from './config.js';
import { authHook } from './middleware/auth.js';
import { rateLimitHook } from './middleware/quota.js';
import authRoutes from './routes/auth.js';
import healthRoutes from './routes/health.js';

export function buildApp() {
  const app = Fastify({ logger: true });

  // 全局：所有 /api/* 需要鉴权（health 除外，注册时用配置跳过）
  app.addHook('onRequest', async (request, reply) => {
    if (request.url.startsWith('/api/') && !request.url.startsWith('/api/health') && !request.url.startsWith('/api/auth')) {
      await authHook(request, reply);
    }
  });
  app.addHook('preHandler', async (request, reply) => {
    if (request.user && request.body && request.method === 'POST') {
      await rateLimitHook(request, reply);
    }
  });

  app.get('/api/health', async () => ({ ok: true, ts: Date.now() }));
  app.register(authRoutes);

  return app;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  buildApp().listen({ port: config.port, host: '127.0.0.1' });
  console.log(`fogg-coach server on :${config.port}`);
}
