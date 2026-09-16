// 进程内限流 + 配额中间件（设计文档 §3.5，数值全部来自 config）
import { config } from '../config.js';
import { usageDAO, today } from '../db/index.js';

// --- 高频限流：滑动窗口（每用户） ---
const buckets = new Map(); // uid -> [timestamps]
setInterval(() => {
  const cutoff = Date.now() - 60_000;
  for (const [k, arr] of buckets) {
    const kept = arr.filter((t) => t > cutoff);
    if (kept.length) buckets.set(k, kept);
    else buckets.delete(k);
  }
}, 60_000).unref();

const cooldowns = new Map(); // uid -> untilTs

export async function rateLimitHook(request, reply) {
  const uid = request.user?.id;
  if (!uid) return;
  const now = Date.now();

  if ((cooldowns.get(uid) || 0) > now) {
    return reply.code(429).send({ error: 'rate_limited', message: '慢一点，30 秒后再试' });
  }

  const arr = (buckets.get(uid) || []).filter((t) => t > now - 1000);
  arr.push(now);
  buckets.set(uid, arr);
  if (arr.length > config.abuse.msgsPerSec) {
    cooldowns.set(uid, now + config.abuse.cooldownSec * 1000);
    return reply.code(429).send({ error: 'rate_limited', message: '消息太快啦，休息 30 秒' });
  }
}

// --- 日配额：LLM 消息数（挂在 chat 路由内部调用前检查） ---
export function checkDailyQuota(userId, reply) {
  const u = usageDAO.get(userId, today());
  if (u.llm_messages >= config.quota.dailyMessages) {
    reply.code(429).send({ error: 'quota', message: '今天聊得够多啦，明天见 🌙' });
    return false;
  }
  if (config.globalDailyCostCap > 0) {
    const total = usageDAO.totalCost(today());
    if (total >= config.globalDailyCostCap) {
      reply.code(503).send({ error: 'global_cap', message: '教练今天休息了，明天再来' });
      return false;
    }
  }
  return true;
}

// --- 长消息连击检测：降为仅按钮模式（返回 true 表示疑似脚本） ---
const longStreaks = new Map();
export function isLongMsgStreak(userId, len) {
  const max = config.quota.messageMaxLen;
  if (len < max * 0.9) {
    longStreaks.delete(userId);
    return false;
  }
  const n = (longStreaks.get(userId) || 0) + 1;
  longStreaks.set(userId, n);
  return n >= config.abuse.longMsgStreak;
}
