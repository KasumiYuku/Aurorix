package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionCookie = "px_admin"

// 会话有效期: 记住我 30 天, 否则 24 小时(浏览器关闭即失)。
const (
	sessionTTLRemember = 30 * 24 * time.Hour
	sessionTTLShort    = 24 * time.Hour
)

type session struct {
	pwHash [32]byte
	expire time.Time
}

type sessions struct {
	mu sync.Mutex
	m  map[string]*session
}

func newSessions() *sessions {
	return &sessions{m: make(map[string]*session)}
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

func handleLogin(sess *sessions) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
			Remember bool   `json:"remember"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效"})
			return
		}
		password := config.Current().AdminPassword
		if password == "" {
			if !isLoopback(c.Request.RemoteAddr) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "远程管理未启用，请在 config.json 中设置 admin_password"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
		if subtle.ConstantTimeCompare([]byte(input.Password), []byte(password)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
			return
		}
		token, maxAge := sess.create(password, input.Remember)
		c.SetCookie(sessionCookie, token, maxAge, "/admin", "", false, true)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func handleLogout(sess *sessions) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token, err := c.Cookie(sessionCookie); err == nil {
			sess.delete(token)
		}
		c.SetCookie(sessionCookie, "", -1, "/admin", "", false, true)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
