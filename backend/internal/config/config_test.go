package config

import "testing"

func TestLoadRejectsMissingSecret(t *testing.T) {
	for _, k := range []string{
		"SESSION_SECRET", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
		"UPLOAD_DIR", "MAX_UPLOAD_BYTES", "SESSION_TTL_HOURS",
		"SEED_TEACHER_PASSWORD", "SEED_STUDENT_A_PASSWORD", "SEED_STUDENT_B_PASSWORD",
		"LOGIN_FAIL_THRESHOLD", "LOGIN_LOCK_MINUTES", "API_ADDR",
	} {
		t.Setenv(k, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected error when env is empty")
	}
}

func TestLoadAcceptsExplicitEnv(t *testing.T) {
	t.Setenv("SESSION_SECRET", "0123456789abcdef")
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "3306")
	t.Setenv("DB_USER", "campusclaw")
	t.Setenv("DB_PASSWORD", "app-secret")
	t.Setenv("DB_NAME", "campusclaw")
	t.Setenv("UPLOAD_DIR", "/data/uploads")
	t.Setenv("MAX_UPLOAD_BYTES", "1048576")
	t.Setenv("SESSION_TTL_HOURS", "12")
	t.Setenv("SEED_TEACHER_PASSWORD", "teacher-pass")
	t.Setenv("SEED_STUDENT_A_PASSWORD", "student-a-pass")
	t.Setenv("SEED_STUDENT_B_PASSWORD", "student-b-pass")
	t.Setenv("LOGIN_FAIL_THRESHOLD", "5")
	t.Setenv("LOGIN_LOCK_MINUTES", "15")
	t.Setenv("API_ADDR", ":8080")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxUploadBytes != 1048576 || cfg.LoginFailThreshold != 5 {
		t.Fatalf("parsed config = %+v", cfg)
	}
}
