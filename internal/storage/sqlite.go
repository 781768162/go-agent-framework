package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s 失败: %w", path, err)
	}

	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err != nil { // 连接探测
		db.Close()
		return nil, fmt.Errorf("数据库 %s 连接异常: %w", path, err)
	}

	if err = syncDBVersion(ctx, db); err != nil { // 表版本同步
		db.Close()
		return nil, fmt.Errorf("数据库 %s 同步版本异常: %w", path, err)
	}

	return db, nil
}

// 根据 schemas 中的ddl，自动初始化表结构
func syncDBVersion(ctx context.Context, db *sql.DB) error {
	var version int

	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("初始化数据库错误: %w", err)
	}
	if version > latestVersion || version < 0 {
		return fmt.Errorf("数据库实际版本: %d 超出指定版本范围: 0-%d", version, latestVersion)
	} else if version == latestVersion {
		return nil
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("初始化数据库错误: %w", err)
	}
	defer tx.Rollback()

	for version < latestVersion { //
		for _, s := range schema[version] {
			_, err = tx.ExecContext(ctx, s)
			if err != nil {
				return fmt.Errorf("初始化数据库错误: %w", err)
			}
		}
		version++
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("初始化数据库错误: %w", err)
	}

	return nil
}

// 创建 sessions 记录，返回sessionId
func CreateSession(ctx context.Context, db *sql.DB, title string) (int64, error) {
	res, err := db.ExecContext(ctx, "INSERT INTO sessions (title) VALUES (?)", title)
	if err != nil {
		return 0, fmt.Errorf("插入 sessions 失败: %w", err)
	}

	return res.LastInsertId()
}

// 创建 messages 记录
func CreateMessage(ctx context.Context, db *sql.DB, msg Message) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("插入 messages 失败: %w", err)
	}
	defer tx.Rollback()

	if msg.ToolCalls == "" {
		msg.ToolCalls = "[]"
	}

	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO messages 
		(session_id, role, content, reasoning_content, tool_calls, tool_call_id)
		VALUES (?, ?, ?, ?, ?, ?);`,
		msg.SessionId,
		msg.Role,
		msg.Content,
		msg.ReasoningContent,
		msg.ToolCalls,
		msg.ToolCallId,
	); err != nil {
		return fmt.Errorf("插入 messages 失败: %w", err)
	}

	res, err := tx.ExecContext( // 更新 sessions 更新时间
		ctx,
		`UPDATE sessions
		SET updated_at = datetime('now')
		WHERE id = ?;`,
		msg.SessionId,
	)
	if err != nil {
		return fmt.Errorf("插入 messages 失败: %w", err)
	}
	affected, err := res.RowsAffected() // 检查修改行数
	if err != nil {
		return fmt.Errorf("插入 messages 失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("会话 %d 不存在", msg.SessionId)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("插入 messages 失败: %w", err)
	}

	return nil
}

func QuerySession(ctx context.Context, db *sql.DB, sessionId int64) (string, error) {
	var res string
	err := db.QueryRowContext(
		ctx,
		`SELECT title FROM sessions WHERE id = ?`,
		sessionId,
	).Scan(&res)
	if err != nil {
		return "", err
	}

	return res, nil
}

// 按 sessionId 查询 messages 记录
func QueryMessages(ctx context.Context, db *sql.DB, sessionId int64) ([]Message, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT role, content, reasoning_content, tool_calls, tool_call_id
		FROM messages
		WHERE session_id = ?
		ORDER BY id;`,
		sessionId,
	)
	if err != nil {
		return nil, fmt.Errorf("查询 messages 失败: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		msg := Message{SessionId: sessionId}

		if err := rows.Scan(
			&msg.Role,
			&msg.Content,
			&msg.ReasoningContent,
			&msg.ToolCalls,
			&msg.ToolCallId,
		); err != nil {
			return nil, fmt.Errorf("查询 messages 失败: %w", err)
		}

		msgs = append(msgs, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("查询 messages 失败: %w", err)
	}

	return msgs, nil
}
