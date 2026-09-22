package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"campusclaw/internal/auth"
	"campusclaw/internal/config"

	"github.com/go-sql-driver/mysql"
)

func Open(cfg config.Config) (*sql.DB, error) {
	mc := mysql.Config{
		User:                 cfg.DBUser,
		Passwd:               cfg.DBPassword,
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(cfg.DBHost, cfg.DBPort),
		DBName:               cfg.DBName,
		ParseTime:            true,
		Loc:                  time.UTC,
		Timeout:              3 * time.Second,
		ReadTimeout:          3 * time.Second,
		WriteTimeout:         3 * time.Second,
		AllowNativePasswords: true,
		Params: map[string]string{
			"charset": "utf8mb4",
		},
	}
	conn, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(30 * time.Minute)
	return conn, nil
}

func Wait(ctx context.Context, conn *sql.DB) error {
	var last error
	for i := 0; i < 60; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		last = conn.PingContext(pingCtx)
		cancel()
		if last == nil {
			return nil
		}
		log.Printf("waiting for database (%d/60): %v", i+1, last)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("database not ready: %w", last)
}

func Migrate(ctx context.Context, conn *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS classes (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(64) NOT NULL UNIQUE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			username VARCHAR(64) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			role ENUM('teacher','student') NOT NULL,
			class_id BIGINT NOT NULL,
			INDEX idx_users_class (class_id),
			CONSTRAINT fk_users_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id CHAR(64) PRIMARY KEY,
			user_id BIGINT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			INDEX idx_sessions_user (user_id),
			CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS materials (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			uploader_id BIGINT NOT NULL,
			title VARCHAR(255) NOT NULL,
			storage_name VARCHAR(80) NOT NULL,
			original_ext VARCHAR(8) NOT NULL,
			byte_size BIGINT NOT NULL,
			created_at DATETIME NOT NULL,
			seed_key VARCHAR(64) NULL,
			UNIQUE KEY uq_materials_seed (seed_key),
			INDEX idx_materials_class (class_id),
			CONSTRAINT fk_materials_class FOREIGN KEY (class_id) REFERENCES classes(id),
			CONSTRAINT fk_materials_user FOREIGN KEY (uploader_id) REFERENCES users(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS knowledge_entries (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			material_id BIGINT NOT NULL,
			class_id BIGINT NOT NULL,
			body MEDIUMTEXT NOT NULL,
			created_at DATETIME NOT NULL,
			UNIQUE KEY uq_knowledge_material (material_id),
			INDEX idx_knowledge_class (class_id),
			CONSTRAINT fk_knowledge_material FOREIGN KEY (material_id) REFERENCES materials(id),
			CONSTRAINT fk_knowledge_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS handouts (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			title VARCHAR(255) NOT NULL,
			INDEX idx_handouts_class (class_id),
			CONSTRAINT fk_handouts_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS assignments (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			title VARCHAR(255) NOT NULL,
			INDEX idx_assignments_class (class_id),
			CONSTRAINT fk_assignments_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS assistants (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS skills (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, stmt := range stmts {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

type seedMaterial struct {
	key      string
	class    string
	uploader string
	title    string
	body     string
}

func Seed(ctx context.Context, conn *sql.DB, cfg config.Config) error {
	if err := ensureClass(ctx, conn, "A"); err != nil {
		return err
	}
	if err := ensureClass(ctx, conn, "B"); err != nil {
		return err
	}
	if err := ensureUser(ctx, conn, "teacher_a", cfg.SeedTeacherPassword, "teacher", "A"); err != nil {
		return err
	}
	if err := ensureUser(ctx, conn, "student_a1", cfg.SeedStudentAPassword, "student", "A"); err != nil {
		return err
	}
	if err := ensureUser(ctx, conn, "student_b1", cfg.SeedStudentBPassword, "student", "B"); err != nil {
		return err
	}
	materials := []seedMaterial{
		{
			key: "seed-class-a-mono", class: "A", uploader: "teacher_a",
			title: "A班-函数单调性讲义",
			body:  "# A班函数单调性\n\n当自变量增大时函数值增大，称为单调递增。\n",
		},
		{
			key: "seed-class-b-newton", class: "B", uploader: "student_b1",
			title: "B班-牛顿定律笔记",
			body:  "# B班牛顿定律\n\n不受外力的物体保持静止或匀速直线运动。\n",
		},
	}
	for _, m := range materials {
		if err := ensureMaterial(ctx, conn, cfg.UploadDir, m); err != nil {
			return err
		}
	}
	if err := ensurePlaceholder(ctx, conn, `INSERT INTO handouts (class_id, title)
		SELECT id, 'A班占位讲义' FROM classes WHERE name='A'
		AND NOT EXISTS (SELECT 1 FROM handouts)`); err != nil {
		return err
	}
	if err := ensurePlaceholder(ctx, conn, `INSERT INTO assignments (class_id, title)
		SELECT id, 'A班占位作业' FROM classes WHERE name='A'
		AND NOT EXISTS (SELECT 1 FROM assignments)`); err != nil {
		return err
	}
	if err := ensurePlaceholder(ctx, conn, `INSERT INTO assistants (name)
		SELECT '占位助手' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM assistants)`); err != nil {
		return err
	}
	if err := ensurePlaceholder(ctx, conn, `INSERT INTO skills (name)
		SELECT '占位技能' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM skills)`); err != nil {
		return err
	}
	return nil
}

func ensurePlaceholder(ctx context.Context, conn *sql.DB, stmt string) error {
	_, err := conn.ExecContext(ctx, stmt)
	return err
}

func ensureClass(ctx context.Context, conn *sql.DB, name string) error {
	_, err := conn.ExecContext(ctx, `INSERT INTO classes (name) SELECT ? WHERE NOT EXISTS (SELECT 1 FROM classes WHERE name=?)`, name, name)
	return err
}

func ensureUser(ctx context.Context, conn *sql.DB, username, password, role, className string) error {
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username=?`, username).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO users (username, password_hash, role, class_id)
		SELECT ?, ?, ?, id FROM classes WHERE name=?`, username, hash, role, className)
	return err
}

func ensureMaterial(ctx context.Context, conn *sql.DB, uploadDir string, m seedMaterial) error {
	var id int64
	var storage string
	err := conn.QueryRowContext(ctx, `SELECT id, storage_name FROM materials WHERE seed_key=?`, m.key).Scan(&id, &storage)
	if err == sql.ErrNoRows {
		storage, err = newStorageName(".md")
		if err != nil {
			return err
		}
		body := []byte(m.body)
		if err := os.WriteFile(filepath.Join(uploadDir, storage), body, 0o640); err != nil {
			return err
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			_ = os.Remove(filepath.Join(uploadDir, storage))
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO materials
			(class_id, uploader_id, title, storage_name, original_ext, byte_size, created_at, seed_key)
			SELECT c.id, u.id, ?, ?, 'md', ?, UTC_TIMESTAMP(), ?
			FROM classes c JOIN users u ON u.username=?
			WHERE c.name=?`,
			m.title, storage, len(body), m.key, m.uploader, m.class)
		if err != nil {
			_ = tx.Rollback()
			_ = os.Remove(filepath.Join(uploadDir, storage))
			return err
		}
		id, err = res.LastInsertId()
		if err != nil {
			_ = tx.Rollback()
			_ = os.Remove(filepath.Join(uploadDir, storage))
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO knowledge_entries (material_id, class_id, body, created_at)
			SELECT ?, c.id, ?, UTC_TIMESTAMP() FROM classes c WHERE c.name=?`, id, m.body, m.class); err != nil {
			_ = tx.Rollback()
			_ = os.Remove(filepath.Join(uploadDir, storage))
			return err
		}
		if err = tx.Commit(); err != nil {
			_ = os.Remove(filepath.Join(uploadDir, storage))
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	path := filepath.Join(uploadDir, storage)
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		if writeErr := os.WriteFile(path, []byte(m.body), 0o640); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func newStorageName(ext string) (string, error) {
	id, err := auth.NewSessionID()
	if err != nil {
		return "", err
	}
	return id[:32] + ext, nil
}
