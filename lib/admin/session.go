package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"net"
	"net/http"
	"sync"
	"time"
)

const sessionCookie = "px_admin"

// 会话有效期: 记住我 30 天, 否则 24 小时(浏览器关闭即失)。
const (
	sessionTTLRemember = 30 * 24 * time.Hour
	sessionTTLShort    = 24 * time.Hour
)

// 登录防爆破: 每 IP 每窗口内失败次数上限, 超出返回 429。
const (
	loginFailWindow = 5 * time.Minute
	loginFailMax    = 10
)

type session struct {
	pwHash [32]byte
	expire time.Time
}

type failEntry struct {
	count int
	start time.Time
}

type sessions struct {
	mu    sync.Mutex
	m     map[string]*session
	fails map[string]*failEntry // host -> 失败计数, 成功登录清零
}

func newSessions() *sessions {
	return &sessions{m: make(map[string]*session), fails: make(map[string]*failEntry)}
}

// valid 校验 token 且密码未变(改密码即全体失效)。
func (s *sessions) valid(token, password string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[token]
	if !ok {
		return false
	}
	if time.Now().After(sess.expire) {
		delete(s.m, token)
		return false
	}
	want := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(sess.pwHash[:], want[:]) == 1
}

// create 建立会话并返回 token 与 Cookie 存活期。
func (s *sessions) create(password string, remember bool) (string, int) {
	var raw [32]byte
	rand.Read(raw[:])
	token := hex.EncodeToString(raw[:])
	ttl := sessionTTLShort
	if remember {
		ttl = sessionTTLRemember
	}
	pwHash := sha256.Sum256([]byte(password))
	s.mu.Lock()
	s.m[token] = &session{pwHash: pwHash, expire: time.Now().Add(ttl)}
	s.mu.Unlock()
	return token, int(ttl.Seconds())
}

func (s *sessions) delete(token string) {
	s.mu.Lock()
	delete(s.m, token)
	s.mu.Unlock()
}

// failExceeded 是否已达失败上限。漏洞窗口内连续失败即触发, 成功登录后清零。
func (s *sessions) failExceeded(host string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.fails[host]
	if e == nil {
		return false
	}
	if time.Since(e.start) > loginFailWindow {
		delete(s.fails, host)
		return false
	}
	return e.count >= loginFailMax
}

// recordFail 记录一次失败; 窗口过期重建, 惰性清理过期的其他条目防 map 无界。
func (s *sessions) recordFail(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if len(s.fails) > 256 {
		for h, e := range s.fails {
			if time.Since(e.start) > loginFailWindow {
				delete(s.fails, h)
			}
		}
	}
	e := s.fails[host]
	if e == nil || time.Since(e.start) > loginFailWindow {
		s.fails[host] = &failEntry{count: 1, start: now}
		return
	}
	e.count++
}

func (s *sessions) clearFails(host string) {
	s.mu.Lock()
	delete(s.fails, host)
	s.mu.Unlock()
}

// remoteHost 取对端 IP, 作为限流键。
func remoteHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func handleLogin(sess *sessions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Password string `json:"password"`
			Remember bool   `json:"remember"`
		}
		if err := readJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		host := remoteHost(r.RemoteAddr)
		password := config.Current().AdminPassword
		if password == "" {
			if !isLoopback(r.RemoteAddr) {
				writeJSON(w, http.StatusServiceUnavailable, H{"error": "远程管理未启用，请在 config.json 中设置 admin_password"})
				return
			}
			writeJSON(w, http.StatusOK, H{"ok": true})
			return
		}
		if sess.failExceeded(host) {
			writeJSON(w, http.StatusTooManyRequests, H{"error": "尝试过于频繁，请稍后再试"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(input.Password), []byte(password)) != 1 {
			sess.recordFail(host)
			writeJSON(w, http.StatusUnauthorized, H{"error": "密码错误"})
			return
		}
		sess.clearFails(host)
		token, maxAge := sess.create(password, input.Remember)
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: token, MaxAge: maxAge, Path: "/admin", HttpOnly: true,
		})
		writeJSON(w, http.StatusOK, H{"ok": true})
	}
}

func handleLogout(sess *sessions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token, err := r.Cookie(sessionCookie); err == nil {
			sess.delete(token.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: "", MaxAge: -1, Path: "/admin", HttpOnly: true,
		})
		writeJSON(w, http.StatusOK, H{"ok": true})
	}
}
