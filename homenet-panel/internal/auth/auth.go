package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultPath 是账号密码凭证文件路径。
const DefaultPath = "/etc/homenet-panel.auth.json"

// 迭代次数：纯标准库实现，用迭代 SHA-256 加盐，足够家用面板。
const defaultIters = 120000

type credFile struct {
	Username string `json:"username"`
	Salt     string `json:"salt"`
	Hash     string `json:"hash"`
	Iters    int    `json:"iters"`
}

// derive 用加盐迭代 SHA-256 派生口令摘要。
func derive(password string, salt []byte, iters int) []byte {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	dk := h.Sum(nil)
	for i := 0; i < iters; i++ {
		h = sha256.New()
		h.Write(dk)
		h.Write(salt)
		dk = h.Sum(nil)
	}
	return dk
}

// Set 写入指定用户的口令凭证（0600）。
func Set(path, user, password string) error {
	if user == "" || password == "" {
		return errors.New("用户名和密码不能为空")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	dk := derive(password, salt, defaultIters)
	cf := credFile{
		Username: user,
		Salt:     hex.EncodeToString(salt),
		Hash:     hex.EncodeToString(dk),
		Iters:    defaultIters,
	}
	b, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Exists 判断凭证文件是否已存在。
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Verify 校验用户名+密码是否匹配凭证文件。
func Verify(path, user, password string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var cf credFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return false, err
	}
	if cf.Username != user {
		return false, nil
	}
	salt, err := hex.DecodeString(cf.Salt)
	if err != nil {
		return false, err
	}
	want, err := hex.DecodeString(cf.Hash)
	if err != nil {
		return false, err
	}
	got := derive(password, salt, cf.Iters)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// Username 读取凭证文件里的用户名。
func Username(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var cf credFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return "", err
	}
	return cf.Username, nil
}

// ---- 内存会话 ----

type session struct {
	user    string
	expires time.Time
}

// Sessions 是进程内会话表（重启后失效，需重新登录）。
type Sessions struct {
	mu  sync.Mutex
	m   map[string]session
	ttl time.Duration
}

func NewSessions(ttl time.Duration) *Sessions {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Sessions{m: make(map[string]session), ttl: ttl}
}

func randToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Create 新建会话，返回会话 token。
func (s *Sessions) Create(user string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	tok := randToken()
	s.m[tok] = session{user: user, expires: time.Now().Add(s.ttl)}
	return tok
}

// User 返回会话对应的用户名，若不存在/过期返回 ok=false。
func (s *Sessions) User(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	se, ok := s.m[tok]
	if !ok || time.Now().After(se.expires) {
		delete(s.m, tok)
		return "", false
	}
	return se.user, true
}

// Delete 删除会话。
func (s *Sessions) Delete(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, tok)
}

func (s *Sessions) cleanupLocked() {
	now := time.Now()
	for k, v := range s.m {
		if now.After(v.expires) {
			delete(s.m, k)
		}
	}
}

// TTL 返回会话有效期。
func (s *Sessions) TTL() time.Duration { return s.ttl }
