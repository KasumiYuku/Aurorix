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

const (
	sessionTTLRemember = 30 * 24 * time.Hour
	sessionTTLShort    = 24 * time.Hour
)

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
	fails map[string]*failEntry
}

func newSessions() *sessions {
	return &sessions{m: make(map[string]*session), fails: make(map[string]*failEntry)}
}

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
			Name: sessionCookie, Value: token, MaxAge: maxAge, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
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
			Name: sessionCookie, Value: "", MaxAge: -1, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
		writeJSON(w, http.StatusOK, H{"ok": true})
	}
}
