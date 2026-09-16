package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

var DB *sql.DB

// Open 打开 SQLite（WAL 模式）并按 schema.sql 自动建表
func Open(path string) error {
	d, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return err
	}
	if _, err := d.Exec(schemaSQL); err != nil {
		return fmt.Errorf("建表失败: %w", err)
	}
	DB = d
	log.Printf("[db] 已连接 %s（8 张表就绪）", path)
	return nil
}
