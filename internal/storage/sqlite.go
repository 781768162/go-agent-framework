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
