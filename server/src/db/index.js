import Database from 'better-sqlite3';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const __dirname = dirname(fileURLToPath(import.meta.url));

export const db = new Database(join(__dirname, '../../fogg-coach.db'));
db.pragma('journal_mode = WAL');
db.exec(readFileSync(join(__dirname, 'schema.sql'), 'utf8'));

export const uid = (prefix) =>
  `${prefix}_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;

export const today = () => {
  const d = new Date(Date.now() + 8 * 3600 * 1000); // Asia/Shanghai
  return d.toISOString().slice(0, 10);
};

// ---------- users ----------
export const userDAO = {
  upsertByOpenid(openid) {
    const id = uid('usr');
    db.prepare(
      `INSERT INTO users (id, openid) VALUES (?, ?)
       ON CONFLICT(openid) DO NOTHING`
    ).run(id, openid);
    return db.prepare('SELECT * FROM users WHERE openid = ?').get(openid);
  },
};

// ---------- sessions ----------
export const sessionDAO = {
  create(userId) {
    const id = uid('sess');
    db.prepare('INSERT INTO sessions (id, user_id) VALUES (?, ?)').run(id, userId);
    return db.prepare('SELECT * FROM sessions WHERE id = ?').get(id);
  },
  get(id) {
    return db.prepare('SELECT * FROM sessions WHERE id = ?').get(id);
  },
  getActiveByUser(userId) {
    return db
      .prepare("SELECT * FROM sessions WHERE user_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 1")
      .get(userId);
  },
  update(id, { stage, status, wish, context }) {
    const s = this.get(id);
    db.prepare(
      `UPDATE sessions SET stage=?, status=?, wish=?, context=?, updated_at=datetime('now') WHERE id=?`
    ).run(
      stage ?? s.stage,
      status ?? s.status,
      wish ?? s.wish,
      context ? JSON.stringify(context) : s.context,
      id
    );
    return this.get(id);
  },
};

// ---------- messages ----------
export const messageDAO = {
  add(sessionId, role, content, meta = null) {
    const id = uid('m');
    db.prepare('INSERT INTO messages (id, session_id, role, content, meta) VALUES (?,?,?,?,?)').run(
      id, sessionId, role, content, meta ? JSON.stringify(meta) : null
    );
    return id;
  },
  listRecent(sessionId, limit = 20) {
    const rows = db
      .prepare('SELECT * FROM (SELECT * FROM messages WHERE session_id = ? ORDER BY created_at DESC, id DESC LIMIT ?) ORDER BY created_at ASC, id ASC')
      .all(sessionId, limit);
    return rows;
  },
  countTurns(sessionId) {
    return db.prepare("SELECT COUNT(*) AS n FROM messages WHERE session_id = ? AND role='user'").get(sessionId).n;
  },
};

// ---------- plans ----------
export const planDAO = {
  create(userId, sessionId, data) {
    const id = uid('plan');
    db.prepare('INSERT INTO plans (id, user_id, session_id, data) VALUES (?,?,?,?)').run(
      id, userId, sessionId, JSON.stringify(data)
    );
    return this.get(id);
  },
  get(id) {
    return db.prepare('SELECT * FROM plans WHERE id = ?').get(id);
  },
  getActiveByUser(userId) {
    return db
      .prepare("SELECT * FROM plans WHERE user_id = ? AND status='active' ORDER BY created_at DESC LIMIT 1")
      .get(userId);
  },
};

// ---------- usage / quota ----------
export const usageDAO = {
  bump(userId, day, field, amount = 1) {
    db.prepare(
      `INSERT INTO usage (user_id, day, ${field}) VALUES (?,?,?)
       ON CONFLICT(user_id, day) DO UPDATE SET ${field} = ${field} + ?`
    ).run(userId, day, amount, amount);
  },
  get(userId, day) {
    return (
      db.prepare('SELECT * FROM usage WHERE user_id = ? AND day = ?').get(userId, day) || {
        llm_messages: 0, plan_generations: 0, cost_yuan: 0,
      }
    );
  },
  totalCost(day) {
    return db.prepare('SELECT COALESCE(SUM(cost_yuan),0) AS c FROM usage WHERE day = ?').get(day).c;
  },
};
