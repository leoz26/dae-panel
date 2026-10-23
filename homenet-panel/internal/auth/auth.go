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
	// MustChange 为真表示这是初始/临时密码，登录后必须立刻修改。
	MustChange bool `json:"mustChange,omitempty"`
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

// Set 写入指定用户的口令凭证（0600），并清除「必须修改密码」标记。
func Set(path, user, password string) error { return SetWithFlag(path, user, password, false) }

// SetWithFlag 写入口令凭证，must=true 表示下次登录必须修改密码（初始密码场景）。
func SetWithFlag(path, user, password string, must bool) error {
	if user == "" || password == "" {
		return errors.New("用户名和密码不能为空")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	dk := derive(password, salt, defaultIters)
	cf := credFile{
		Username:   user,
		Salt:       hex.EncodeToString(salt),
		Hash:       hex.EncodeToString(dk),
		Iters:      defaultIters,
		MustChange: must,
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

// NeedsChange 判断该用户是否被要求先修改密码（初始密码）。
func NeedsChange(path, user string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cf credFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return false
	}
	return cf.MustChange && cf.Username == user
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

// ---- 会话（可持久化） ----

const (
	// DefaultSessionTTL 是默认会话有效期（未勾选「记住我」）。
	DefaultSessionTTL = 12 * time.Hour
	// RememberSessionTTL 是勾选「记住我」后的会话有效期。
	RememberSessionTTL = 30 * 24 * time.Hour
)

type session struct {
	User    string    `json:"user"`
	Expires time.Time `json:"expires"`
}

// Sessions 是会话表。设置 Path 后每次变更都会落盘（0600），进程重启后仍有效；
// 表内键是 token 的 SHA-256 摘要（不存明文 token，文件泄露也不能直接拿来登录）。
type Sessions struct {
	mu   sync.Mutex
	m    map[string]session
	ttl  time.Duration
	path string
}

func NewSessions(ttl time.Duration) *Sessions {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	return &Sessions{m: make(map[string]session), ttl: ttl}
}

// SetPath 设置会话持久化文件路径（空表示仅内存）。
func (s *Sessions) SetPath(p string) {
	s.mu.Lock()
	s.path = p
	s.mu.Unlock()
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func randToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Load 从持久化文件载入未过期会话（文件缺失/损坏不算错误）。
func (s *Sessions) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var m map[string]session
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	now := time.Now()
	for k, v := range m {
		if v.User == "" || now.After(v.Expires) {
			continue
		}
		s.m[k] = v
	}
	return nil
}

// saveLocked 把当前会话表原子写入磁盘（需持锁）。
func (s *Sessions) saveLocked() {
	if s.path == "" {
		return
	}
	b, err := json.Marshal(s.m)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}

// Create 新建会话（默认有效期），返回会话 token。
func (s *Sessions) Create(user string) string { return s.CreateTTL(user, 0) }

// CreateTTL 新建会话并指定有效期（<=0 用默认）。
func (s *Sessions) CreateTTL(user string, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = s.ttl
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	tok := randToken()
	s.m[hashToken(tok)] = session{User: user, Expires: time.Now().Add(ttl)}
	s.saveLocked()
	return tok
}

// User 返回会话对应的用户名，若不存在/过期返回 ok=false。
func (s *Sessions) User(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := hashToken(tok)
	se, ok := s.m[k]
	if !ok {
		return "", false
	}
	if time.Now().After(se.Expires) {
		delete(s.m, k)
		s.saveLocked()
		return "", false
	}
	return se.User, true
}

// Delete 删除会话。
func (s *Sessions) Delete(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := hashToken(tok)
	if _, ok := s.m[k]; ok {
		delete(s.m, k)
		s.saveLocked()
	}
}

// Count 返回当前有效会话数（诊断用）。
func (s *Sessions) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	return len(s.m)
}

func (s *Sessions) cleanupLocked() {
	now := time.Now()
	changed := false
	for k, v := range s.m {
		if now.After(v.Expires) {
			delete(s.m, k)
			changed = true
		}
	}
	if changed {
		s.saveLocked()
	}
}

// TTL 返回会话有效期。
func (s *Sessions) TTL() time.Duration { return s.ttl }
