package auth

import (
	"sync"

	"golang.org/x/crypto/bcrypt"
)

var (
	dummyOnce sync.Once
	dummyHash []byte
)

// DummyHash 是一把固定口令的 bcrypt，用来在用户不存在时也花掉同样的比较时间。
// 它不能登录任何账号。
func DummyHash() []byte {
	dummyOnce.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte("campusclaw-timing-pad"), bcrypt.DefaultCost)
		if err != nil {
			panic(err)
		}
		dummyHash = h
	})
	return dummyHash
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
