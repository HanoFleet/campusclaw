package auth

import (
	"sync"
	"time"
)

// Limiter 按「用户名 + IP」计数。计数只在本进程内存里，所以部署必须是单实例。
type Limiter struct {
	mu sync.Mutex
	m  map[string]*attempt
}

type attempt struct {
	fails       int
	lockedUntil time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{m: map[string]*attempt{}}
}

func Key(username, ip string) string {
	return username + "\n" + ip
}

func (l *Limiter) Locked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec, ok := l.m[key]
	if !ok {
		return false
	}
	if rec.lockedUntil.IsZero() {
		return false
	}
	if now.Before(rec.lockedUntil) {
		return true
	}
	delete(l.m, key)
	return false
}

func (l *Limiter) Fail(key string, now time.Time, threshold int, lock time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := l.m[key]
	if rec == nil {
		rec = &attempt{}
		l.m[key] = rec
	}
	rec.fails++
	if rec.fails >= threshold {
		rec.lockedUntil = now.Add(lock)
	}
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, key)
}
