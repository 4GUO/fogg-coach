import 'dotenv/config';

const int = (v, d) => (Number.isFinite(+v) ? +v : d);

export const config = {
  port: int(process.env.PORT, 3210),
  jwtSecret: process.env.JWT_SECRET || 'dev-secret',
  jwtTtlDays: 7,

  llm: {
    baseUrl: process.env.LLM_BASE_URL || '',
    apiKey: process.env.LLM_API_KEY || '',
    model: process.env.LLM_MODEL || 'glm-4-flash',
  },

  wx: {
    appid: process.env.WX_APPID || '',
    secret: process.env.WX_SECRET || '',
    mock: process.env.WX_MOCK === '1',
  },

  quota: {
    dailyMessages: int(process.env.QUOTA_DAILY_MESSAGES, 50),
    sessionTurns: int(process.env.QUOTA_SESSION_TURNS, 25),
    messageMaxLen: int(process.env.QUOTA_MESSAGE_MAX_LEN, 500),
    planGenerations: int(process.env.QUOTA_PLAN_GENERATIONS, 3),
  },

  abuse: {
    msgsPerSec: int(process.env.RATE_LIMIT_MSGS_PER_SEC, 2),
    cooldownSec: int(process.env.RATE_LIMIT_COOLDOWN_SEC, 30),
    longMsgStreak: int(process.env.LONG_MSG_STREAK, 3),
  },

  globalDailyCostCap: int(process.env.GLOBAL_DAILY_COST_CAP, 0), // 元, 0=off
};
