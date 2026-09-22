package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 全部来自环境变量。缺任何一项都返回错误，不填默认密钥。
type Config struct {
	SessionSecret        string
	DBHost               string
	DBPort               string
	DBUser               string
	DBPassword           string
	DBName               string
	UploadDir            string
	MaxUploadBytes       int64
	SessionTTL           time.Duration
	SeedTeacherPassword  string
	SeedStudentAPassword string
	SeedStudentBPassword string
	LoginFailThreshold   int
	LoginLock            time.Duration
	APIAddr              string
}

func Load() (Config, error) {
	var missing []string
	need := func(key string) string {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	cfg := Config{
		SessionSecret:        need("SESSION_SECRET"),
		DBHost:               need("DB_HOST"),
		DBPort:               need("DB_PORT"),
		DBUser:               need("DB_USER"),
		DBPassword:           need("DB_PASSWORD"),
		DBName:               need("DB_NAME"),
		UploadDir:            need("UPLOAD_DIR"),
		SeedTeacherPassword:  need("SEED_TEACHER_PASSWORD"),
		SeedStudentAPassword: need("SEED_STUDENT_A_PASSWORD"),
		SeedStudentBPassword: need("SEED_STUDENT_B_PASSWORD"),
		APIAddr:              need("API_ADDR"),
	}
	maxRaw := need("MAX_UPLOAD_BYTES")
	ttlRaw := need("SESSION_TTL_HOURS")
	failRaw := need("LOGIN_FAIL_THRESHOLD")
	lockRaw := need("LOGIN_LOCK_MINUTES")
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	if len(cfg.SessionSecret) < 16 {
		return Config{}, fmt.Errorf("SESSION_SECRET must be at least 16 characters")
	}
	for _, pair := range []struct {
		name string
		val  string
	}{
		{"SEED_TEACHER_PASSWORD", cfg.SeedTeacherPassword},
		{"SEED_STUDENT_A_PASSWORD", cfg.SeedStudentAPassword},
		{"SEED_STUDENT_B_PASSWORD", cfg.SeedStudentBPassword},
	} {
		if len(pair.val) < 8 {
			return Config{}, fmt.Errorf("%s must be at least 8 characters", pair.name)
		}
	}

	maxBytes, err := strconv.ParseInt(maxRaw, 10, 64)
	if err != nil || maxBytes <= 0 || maxBytes > 16<<20 {
		return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES must be an integer from 1 to 16777216")
	}
	ttlHours, err := strconv.Atoi(ttlRaw)
	if err != nil || ttlHours <= 0 {
		return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be a positive integer")
	}
	failN, err := strconv.Atoi(failRaw)
	if err != nil || failN <= 0 {
		return Config{}, fmt.Errorf("LOGIN_FAIL_THRESHOLD must be a positive integer")
	}
	lockMin, err := strconv.Atoi(lockRaw)
	if err != nil || lockMin <= 0 {
		return Config{}, fmt.Errorf("LOGIN_LOCK_MINUTES must be a positive integer")
	}
	if _, err := strconv.Atoi(cfg.DBPort); err != nil {
		return Config{}, fmt.Errorf("DB_PORT must be numeric")
	}

	cfg.MaxUploadBytes = maxBytes
	cfg.SessionTTL = time.Duration(ttlHours) * time.Hour
	cfg.LoginFailThreshold = failN
	cfg.LoginLock = time.Duration(lockMin) * time.Minute
	return cfg, nil
}
