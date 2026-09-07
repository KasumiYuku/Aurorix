package middleware

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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

func VerifySignature(botSecret string) gin.HandlerFunc {
	pub, _ := DeriveEd25519Key(botSecret)

	return func(c *gin.Context) {
		// 主动推送接口不走QQ签名校验
		if strings.HasPrefix(c.Request.URL.Path, "/push/") {
			c.Next()
			return
		}

		// 获取 Header 参数
		signature := c.GetHeader("X-Signature-Ed25519")
		timestamp := c.GetHeader("X-Signature-Timestamp")

		if signature == "" || timestamp == "" {
			verifyLog.Warnf("签名校验失败: 缺少签名字段")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 解码签名
		sig, err := hex.DecodeString(signature)
		if err != nil || len(sig) != ed25519.SignatureSize || sig[63]&224 != 0 {
			verifyLog.Warnf("签名校验失败: 签名格式不合法")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 读取Body并重写
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		// log.Printf("[Debug]Raw request: %v", string(bodyBytes))
		// 拼接签名体
		var msg bytes.Buffer
		msg.WriteString(timestamp)
		msg.Write(bodyBytes)

		// 校验签名
		if !ed25519.Verify(pub, msg.Bytes(), sig) {
			verifyLog.Warnf("签名校验失败: 签名验证不通过，可能遭遇伪造请求")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 校验通过，继续后面的路由逻辑
		c.Next()
	}
}
