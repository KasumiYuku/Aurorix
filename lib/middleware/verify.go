package middleware

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"io"
	"net/http"
	"strings"
)

var verifyLog = logx.New("verify")

// DeriveEd25519Key 从 AppSecret 派生 QQ 签名密钥对。
// QQ 平台无独立公钥分发，官方 SDK 定义以 secret 为种子生成：seed 长度不足时
// 重复拼接补齐到 32 字节。verify 用公钥验签，webhook Op=13 用私钥回签。
// 两处共用同一函数避免派生逻辑漂移。
func DeriveEd25519Key(secret string) (ed25519.PublicKey, ed25519.PrivateKey) {
	seed := secret
	for len(seed) < ed25519.SeedSize {
		seed += seed
	}
	reader := strings.NewReader(seed[:ed25519.SeedSize])
	pub, priv, _ := ed25519.GenerateKey(reader)
	return pub, priv
}

// VerifySignature 包装下一个 handler, 对 QQ 回调做 ed25519 验签。
func VerifySignature(botSecret string) func(http.Handler) http.Handler {
	pub, _ := DeriveEd25519Key(botSecret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 获取 Header 参数
			signature := r.Header.Get("X-Signature-Ed25519")
			timestamp := r.Header.Get("X-Signature-Timestamp")

			if signature == "" || timestamp == "" {
				verifyLog.Warnf("签名校验失败: 缺少签名字段")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// 解码签名
			sig, err := hex.DecodeString(signature)
			if err != nil || len(sig) != ed25519.SignatureSize || sig[63]&224 != 0 {
				verifyLog.Warnf("签名校验失败: 签名格式不合法")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// 读取Body并重写
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			// 拼接签名体
			var msg bytes.Buffer
			msg.WriteString(timestamp)
			msg.Write(bodyBytes)

			// 校验签名
			if !ed25519.Verify(pub, msg.Bytes(), sig) {
				verifyLog.Warnf("签名校验失败: 签名验证不通过，可能遭遇伪造请求")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// 校验通过，继续后面的路由逻辑
			next.ServeHTTP(w, r)
		})
	}
}
