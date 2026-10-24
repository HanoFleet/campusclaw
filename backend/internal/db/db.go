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
		`CREATE TABLE IF NOT EXISTS knowledge_chunks (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			material_id BIGINT NOT NULL,
			knowledge_entry_id BIGINT NOT NULL,
			chunk_index INT NOT NULL,
			chunk_text MEDIUMTEXT NOT NULL,
			start_offset INT NOT NULL,
			end_offset INT NOT NULL,
			index_status ENUM('pending','ready','failed') NOT NULL,
			strategy VARCHAR(32) NOT NULL,
			created_at DATETIME NOT NULL,
			INDEX idx_chunks_class (class_id),
			INDEX idx_chunks_material (material_id),
			FULLTEXT KEY ft_chunk_text (chunk_text) WITH PARSER ngram,
			CONSTRAINT fk_chunks_class FOREIGN KEY (class_id) REFERENCES classes(id),
			CONSTRAINT fk_chunks_material FOREIGN KEY (material_id) REFERENCES materials(id),
			CONSTRAINT fk_chunks_entry FOREIGN KEY (knowledge_entry_id) REFERENCES knowledge_entries(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, stmt := range stmts {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return migrateStudio(ctx, conn)
}

func migrateStudio(ctx context.Context, conn *sql.DB) error {
	cols := []struct{ table, name, def string }{
		{"skills", "class_id", "BIGINT NULL"},
		{"skills", "trigger_text", "VARCHAR(120) NOT NULL DEFAULT ''"},
		{"skills", "description", "TEXT NULL"},
		{"skills", "tools_json", "TEXT NULL"},
		{"skills", "source_text", "MEDIUMTEXT NULL"},
		{"skills", "subject_text", "VARCHAR(32) NOT NULL DEFAULT ''"},
		{"skills", "enabled", "TINYINT NOT NULL DEFAULT 1"},
		{"skills", "seed_key", "VARCHAR(64) NULL"},
	}
	for _, c := range cols {
		if err := addColumn(ctx, conn, c.table, c.name, c.def); err != nil {
			return err
		}
	}
	extra := []string{
		`CREATE TABLE IF NOT EXISTS tutor_agents (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			name VARCHAR(64) NOT NULL,
			prompt VARCHAR(2000) NOT NULL DEFAULT '',
			enabled TINYINT NOT NULL DEFAULT 1,
			seed_key VARCHAR(64) NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE KEY uq_agent_class_name (class_id, name),
			UNIQUE KEY uq_agent_seed (seed_key),
			INDEX idx_agents_class (class_id),
			CONSTRAINT fk_agents_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS tutor_agent_skills (
			agent_id BIGINT NOT NULL,
			skill_id BIGINT NOT NULL,
			PRIMARY KEY (agent_id, skill_id),
			CONSTRAINT fk_bind_skill_agent FOREIGN KEY (agent_id) REFERENCES tutor_agents(id) ON DELETE CASCADE,
			CONSTRAINT fk_bind_skill_skill FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS tutor_agent_materials (
			agent_id BIGINT NOT NULL,
			material_id BIGINT NOT NULL,
			PRIMARY KEY (agent_id, material_id),
			CONSTRAINT fk_bind_mat_agent FOREIGN KEY (agent_id) REFERENCES tutor_agents(id) ON DELETE CASCADE,
			CONSTRAINT fk_bind_mat_material FOREIGN KEY (material_id) REFERENCES materials(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			agent_id BIGINT NOT NULL,
			title VARCHAR(64) NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			INDEX idx_conv_user (user_id, updated_at),
			CONSTRAINT fk_conv_class FOREIGN KEY (class_id) REFERENCES classes(id),
			CONSTRAINT fk_conv_user FOREIGN KEY (user_id) REFERENCES users(id),
			CONSTRAINT fk_conv_agent FOREIGN KEY (agent_id) REFERENCES tutor_agents(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS messages (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			conversation_id BIGINT NOT NULL,
			role ENUM('user','assistant') NOT NULL,
			content MEDIUMTEXT NOT NULL,
			citations_json MEDIUMTEXT NULL,
			skill_name VARCHAR(64) NULL,
			trace_json MEDIUMTEXT NULL,
			created_at DATETIME NOT NULL,
			INDEX idx_msg_conv (conversation_id, id),
			CONSTRAINT fk_msg_conv FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS llm_exchanges (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			conversation_id BIGINT NOT NULL,
			round_no INT NOT NULL,
			request_json MEDIUMTEXT NOT NULL,
			response_json MEDIUMTEXT NOT NULL,
			created_at DATETIME NOT NULL,
			INDEX idx_llm_conv (conversation_id, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS mcp_servers (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			name VARCHAR(64) NOT NULL,
			base_url VARCHAR(255) NOT NULL,
			auth_token VARCHAR(255) NOT NULL,
			connected TINYINT NOT NULL DEFAULT 0,
			last_error VARCHAR(255) NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			UNIQUE KEY uq_mcp_class_name (class_id, name),
			INDEX idx_mcp_class (class_id),
			CONSTRAINT fk_mcp_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS mcp_tools (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			server_id BIGINT NOT NULL,
			class_id BIGINT NOT NULL,
			name VARCHAR(64) NOT NULL,
			description TEXT NOT NULL,
			input_schema TEXT NOT NULL,
			UNIQUE KEY uq_mcp_tool (server_id, name),
			CONSTRAINT fk_mcp_tool_server FOREIGN KEY (server_id) REFERENCES mcp_servers(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS mcp_calls (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			server_name VARCHAR(64) NOT NULL,
			tool_name VARCHAR(64) NOT NULL,
			ref_text VARCHAR(160) NOT NULL,
			status VARCHAR(16) NOT NULL,
			detail VARCHAR(500) NOT NULL DEFAULT '',
			round_no INT NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			INDEX idx_mcp_calls_class (class_id, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, stmt := range extra {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	_, _ = conn.ExecContext(ctx, `CREATE UNIQUE INDEX uq_skills_seed ON skills (seed_key)`)
	_, _ = conn.ExecContext(ctx, `CREATE UNIQUE INDEX uq_skills_class_name ON skills (class_id, name)`)
	return nil
}

func addColumn(ctx context.Context, conn *sql.DB, table, name, def string) error {
	var n int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, name).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err = conn.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+name+" "+def)
	return err
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
		{
			key: "seed-class-a-science", class: "A", uploader: "teacher_a",
			title: "A班-小学科学课程.md",
			body:  "# 小学科学课程\n\n水在零摄氏度结成冰，加热到一百摄氏度时沸腾，变成水蒸气。磁铁能吸铁钉，不能吸木块。本资料发给 A 班科学课使用。\n\n每节先观察，再写下看到的现象，最后才写原因。水的三态：冰是固体，水是液体，水蒸气是气体。把冰块放在室温的盘子里，冰块会变小，盘子里会出现水。水开了之后壶嘴冒出白雾，是水蒸气遇冷变成的小水滴。磁铁能吸铁，不能吸木头。条形磁铁两端磁力最强，中间较弱。同名磁极互相推开，异名磁极互相靠近。\n",
		},
		{
			key: "seed-class-a-math", class: "A", uploader: "teacher_a",
			title: "A班-数学自学笔记",
			body:  "# 数学自学笔记\n\n去括号时，括号前面的数要乘到括号里的每一项。分母不同的分数相加，要先通分。矩形面积等于长乘宽，长和宽要先换成同一个单位。设未知数时，先写清楚它表示哪一个量。\n",
		},
		{
			key: "seed-class-a-chinese", class: "A", uploader: "teacher_a",
			title: "A班-语文自学笔记",
			body:  "# 语文自学笔记\n\n春晓的前两句是春眠不觉晓，处处闻啼鸟。读一首诗先看题目，再找写景的句子，最后才说中心。改作文的开头，先看第一句有没有点题。\n",
		},
	}
	for _, m := range materials {
		if err := ensureMaterial(ctx, conn, cfg.UploadDir, m); err != nil {
			return err
		}
	}
	if err := seedStudio(ctx, conn, cfg.MCPAuthToken); err != nil {
		return err
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
