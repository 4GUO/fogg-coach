import jwt from 'jsonwebtoken';
import { config } from '../config.js';

export function signToken(userId) {
  return jwt.sign({ uid: userId }, config.jwtSecret, {
    expiresIn: `${config.jwtTtlDays}d`,
  });
}

// Fastify preHandler：校验 Bearer token，挂载 request.user
export async function authHook(request, reply) {
  const auth = request.headers.authorization || '';
  const token = auth.startsWith('Bearer ') ? auth.slice(7) : null;
  if (!token) return reply.code(401).send({ error: 'unauthorized', message: '缺少 token' });
  try {
    const payload = jwt.verify(token, config.jwtSecret);
    request.user = { id: payload.uid };
  } catch {
    return reply.code(401).send({ error: 'unauthorized', message: 'token 无效或过期' });
  }
}
