package db

import "database/sql"

// ---------- Session（诊断会话） ----------

type Session struct {
	ID         string
	UserID     string
	Stage      string
	Status     string // active|done|abandoned
	Wish       *string
	Context    string
	ResetCount int
	CreatedAt  string
	UpdatedAt  string
}

func CreateSession(userID string) (*Session, error) {
	s := &Session{ID: NewID("sess"), UserID: userID, Stage: "S1", Status: "active", Context: "{}"}
	_, err := DB.Exec(`INSERT INTO sessions (id, user_id, stage, status, context) VALUES (?,?,?,?,?)`,
		s.ID, s.UserID, s.Stage, s.Status, s.Context)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func SessionByID(id string) (*Session, error) {
	var s Session
	var wish sql.NullString
	err := DB.QueryRow(`SELECT id, user_id, stage, status, wish, context, reset_count, created_at, updated_at
		FROM sessions WHERE id = ?`, id).
		Scan(&s.ID, &s.UserID, &s.Stage, &s.Status, &wish, &s.Context, &s.ResetCount, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if wish.Valid {
		s.Wish = &wish.String
	}
	return &s, err
}

// ActiveSessionByUser 用户当前活跃诊断会话（首 session 未结束不可开新的，§3.5）
func ActiveSessionByUser(userID string) (*Session, error) {
	var s Session
	var wish sql.NullString
	err := DB.QueryRow(`SELECT id, user_id, stage, status, wish, context, reset_count, created_at, updated_at
		FROM sessions WHERE user_id = ? AND status = 'active' ORDER BY updated_at DESC LIMIT 1`, userID).
		Scan(&s.ID, &s.UserID, &s.Stage, &s.Status, &wish, &s.Context, &s.ResetCount, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if wish.Valid {
		s.Wish = &wish.String
	}
	return &s, err
}

func SaveSessionStage(s *Session) error {
	wish := (*string)(nil)
	if s.Wish != nil {
		wish = s.Wish
	}
	_, err := DB.Exec(`UPDATE sessions SET stage=?, status=?, wish=?, context=?, reset_count=?,
		updated_at=datetime('now') WHERE id=?`, s.Stage, s.Status, wish, s.Context, s.ResetCount, s.ID)
	return err
}

// ---------- Message ----------

func InsertMessage(sessionID, role, content, meta string) (string, error) {
	id := NewID("msg")
	if meta == "" {
		meta = "{}"
	}
	_, err := DB.Exec(`INSERT INTO messages (id, session_id, role, content, meta) VALUES (?,?,?,?,?)`,
		id, sessionID, role, content, meta)
	return id, err
}

func ListMessages(sessionID string) []struct {
	Role, Content string
} {
	rows, err := DB.Query(`SELECT role, content FROM messages WHERE session_id = ? ORDER BY created_at, rowid`, sessionID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []struct{ Role, Content string }
	for rows.Next() {
		var r struct{ Role, Content string }
		if rows.Scan(&r.Role, &r.Content) == nil {
			out = append(out, r)
		}
	}
	return out
}

// CountUserTurns 用户消息数（轮次配额口径）
func CountUserTurns(sessionID string) int {
	var n int
	_ = DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ? AND role='user'`, sessionID).Scan(&n)
	return n
}

// ---------- Plan ----------

func InsertPlan(userID, sessionID, planJSON string) (string, error) {
	id := NewID("plan")
	_, err := DB.Exec(`INSERT INTO plans (id, user_id, session_id, status, data) VALUES (?,?,?,?,?)`,
		id, userID, sessionID, "active", planJSON)
	return id, err
}
