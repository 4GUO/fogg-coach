package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// 今天（Asia/Shanghai），格式 YYYY-MM-DD —— streak/配额按此日界
func Today() string {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	return time.Now().In(loc).Format("2006-01-02")
}

func NewID(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// ---------- User ----------

type User struct {
	ID        string
	Nickname  *string
	TokenVer  int
	CreatedAt string
}

var ErrNotFound = errors.New("not found")

// UpsertByIdentity 按凭证查/建用户（登录主路径，§3.6.3）
func UpsertByIdentity(provider, uid, credentials string) (*User, error) {
	// 1. 已有凭证 → 直接返回用户
	var u User
	err := DB.QueryRow(`SELECT u.id, u.nickname, u.token_ver, u.created_at
		FROM users u JOIN user_identities i ON i.user_id = u.id
		WHERE i.provider = ? AND i.uid = ?`, provider, uid).
		Scan(&u.ID, &u.Nickname, &u.TokenVer, &u.CreatedAt)
	if err == nil {
		if credentials != "" { // 刷新 session_key
			_, _ = DB.Exec(`UPDATE user_identities SET credentials = ? WHERE provider = ? AND uid = ?`, credentials, provider, uid)
		}
		return &u, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	// 2. 新用户：建 user + identity
	u = User{ID: NewID("usr"), TokenVer: 1}
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO users (id, token_ver) VALUES (?, ?)`, u.ID, u.TokenVer); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO user_identities (id, user_id, provider, uid, credentials) VALUES (?,?,?,?,?)`,
		NewID("idn"), u.ID, provider, uid, credentials); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &u, nil
}

func UserByID(id string) (*User, error) {
	var u User
	err := DB.QueryRow(`SELECT id, nickname, token_ver, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Nickname, &u.TokenVer, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// ---------- Usage（配额，§3.5） ----------

type Usage struct {
	UserID         string
	Day            string
	LLMMessages    int
	PlanGenerations int
	CostYuan       float64
}

func UsageGet(userID, day string) Usage {
	var u Usage
	_ = DB.QueryRow(`SELECT user_id, day, llm_messages, plan_generations, cost_yuan
		FROM usage WHERE user_id = ? AND day = ?`, userID, day).
		Scan(&u.UserID, &u.Day, &u.LLMMessages, &u.PlanGenerations, &u.CostYuan)
	return u
}

// UsageIncr 原子累加并返回累加后的值（kind: llm_messages | plan_generations）
func UsageIncr(userID, day, kind string, costDelta float64) {
	col := map[string]string{"llm_messages": "llm_messages", "plan_generations": "plan_generations"}[kind]
	if col == "" {
		return
	}
	_, _ = DB.Exec(`INSERT INTO usage (user_id, day, llm_messages, plan_generations, cost_yuan)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, day) DO UPDATE SET `+col+` = `+col+` + 1, cost_yuan = cost_yuan + excluded.cost_yuan`,
		userID, day, boolInt(kind == "llm_messages"), boolInt(kind == "plan_generations"), costDelta)
}

func UsageTotalCost(day string) float64 {
	var v float64
	_ = DB.QueryRow(`SELECT COALESCE(SUM(cost_yuan), 0) FROM usage WHERE day = ?`, day).Scan(&v)
	return v
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
