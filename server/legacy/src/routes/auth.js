import { config } from '../config.js';
import { signToken } from '../middleware/auth.js';
import { userDAO } from '../db/index.js';

// POST /api/auth/login  { code }  → wx.login code2Session → JWT
export default async function authRoutes(app) {
  app.post('/api/auth/login', async (request, reply) => {
    const { code } = request.body || {};
    if (!code) return reply.code(400).send({ error: 'bad_request', message: '缺少 code' });

    let openid;
    if (config.wx.mock) {
      openid = `mock_${code}`; // 开发模式
    } else {
      const url =
        `https://api.weixin.qq.com/sns/jscode2session?appid=${config.wx.appid}` +
        `&secret=${config.wx.secret}&js_code=${encodeURIComponent(code)}&grant_type=authorization_code`;
      const res = await fetch(url);
      const data = await res.json();
      if (!data.openid) {
        return reply.code(401).send({ error: 'wx_error', message: 'code 换取失败', detail: data.errcode });
      }
      openid = data.openid;
    }

    const user = userDAO.upsertByOpenid(openid);
    return { token: signToken(user.id), userId: user.id };
  });
}
