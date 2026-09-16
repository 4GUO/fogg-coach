-- fogg-coach schema v1.1（system-design §5，2026-09-16 定稿）
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,            -- usr_xxx
  nickname TEXT,
  token_ver INTEGER NOT NULL DEFAULT 1,  -- JWT 吊销版本（§3.6.2）
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS user_identities (    -- 登录凭证（§3.6），一人可挂多个
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  provider TEXT NOT NULL,         -- wechat_mp | phone
  uid TEXT NOT NULL,              -- openid 或手机号
  credentials TEXT,               -- session_key 等，仅服务端
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(provider, uid)
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,            -- sess_xxx
  user_id TEXT NOT NULL,
  stage TEXT NOT NULL DEFAULT 'S1',
  status TEXT NOT NULL DEFAULT 'active',  -- active|done|abandoned
  wish TEXT,
  context TEXT NOT NULL DEFAULT '{}',
  reset_count INTEGER NOT NULL DEFAULT 0,  -- RESET_WISH 已用次数，上限2（§3.1.5）
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id, status);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  role TEXT NOT NULL,             -- user|assistant|system_event
  content TEXT NOT NULL,
  meta TEXT,                      -- quick_replies 等 JSON
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);

CREATE TABLE IF NOT EXISTS plans (
  id TEXT PRIMARY KEY,            -- plan_xxx
  user_id TEXT NOT NULL,
  session_id TEXT,
  status TEXT NOT NULL DEFAULT 'active',  -- active|paused|finished
  version INTEGER NOT NULL DEFAULT 1,
  data TEXT NOT NULL,             -- Plan JSON
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_plans_user ON plans(user_id, status);

CREATE TABLE IF NOT EXISTS checkins (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL, habit_id TEXT NOT NULL,
  date TEXT NOT NULL,             -- YYYY-MM-DD（Asia/Shanghai）
  done INTEGER NOT NULL DEFAULT 1,
  mood INTEGER, note TEXT,
  media TEXT,                     -- JSON [{type,url}]，图片≤3（§3.3.6）
  group_id TEXT,                  -- v2 团队打卡留口，MVP 恒 NULL
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(plan_id, habit_id, date)
);
CREATE INDEX IF NOT EXISTS idx_checkins_date ON checkins(date);

CREATE TABLE IF NOT EXISTS posts (              -- 社区动态（§3.3.6）
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  checkin_id TEXT,
  content TEXT NOT NULL,
  images TEXT,                    -- JSON 数组
  likes INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',  -- pending|pass|reject（机审）
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_posts_user ON posts(user_id, status);

CREATE TABLE IF NOT EXISTS usage (              -- 配额计数（§3.5）
  user_id TEXT NOT NULL,
  day TEXT NOT NULL,              -- YYYY-MM-DD
  llm_messages INTEGER NOT NULL DEFAULT 0,
  plan_generations INTEGER NOT NULL DEFAULT 0,
  cost_yuan REAL NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, day)
);
