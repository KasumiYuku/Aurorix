package providers

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func getBkn(skey string) string {
	hash := 5381
	for i := 0; i < len(skey); i++ {
		hash += (hash << 5) + int(skey[i])
	}
	return strconv.Itoa(hash & 0x7fffffff)
}

func cosURLEncode(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(url.QueryEscape(s), "+", "%20"), "*", "%2A"), "%7E", "~")
}

func sha1Hex(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hmacSha1Hex(key, data string) string {
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func cosSign(method, cosPath string, headers map[string]string, secretID, secretKey string, now, expire int64) (string, string) {
	qKeyTime := fmt.Sprintf("%d;%d", now, expire)
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var sorted strings.Builder
	for _, k := range keys {
		sorted.WriteString(cosURLEncode(k) + "=" + cosURLEncode(headers[k]) + "&")
	}
	formatString := fmt.Sprintf("%s\n%s\n\n%s\n", method, cosPath, strings.TrimSuffix(sorted.String(), "&"))
	stringToSign := fmt.Sprintf("sha1\n%s\n%s\n", qKeyTime, sha1Hex(formatString))
	signKey := hmacSha1Hex(secretKey, qKeyTime)
	sig := hmacSha1Hex(signKey, stringToSign)
	return sig, qKeyTime
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func urlValues(m map[string]string) url.Values {
	v := make(url.Values, len(m))
	for k, val := range m {
		v.Set(k, val)
	}
	return v
}

var originURLRe = regexp.MustCompile(`"origin_url":"(.*?)"`)

func regexpOriginURL(raw []byte) string {
	m := originURLRe.FindSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	return strings.ReplaceAll(string(m[1]), `\/`, "/")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func str(cfg map[string]any, key, def string) string {
	if v, ok := cfg[key].(string); ok && v != "" {
		return v
	}
	return def
}

func strPtr(cfg map[string]any, key string) (string, bool) {
	v, ok := cfg[key].(string)
	return v, ok && v != ""
}

func skeyFromCookie(cookie string) string {
	for _, seg := range strings.Split(cookie, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(seg), "=")
		if ok && k == "skey" {
			return v
		}
	}
	return ""
}

func b64(data []byte) string { return base64.StdEncoding.EncodeToString(data) }

func mimeExt(mime string) string {
	if i := strings.IndexByte(mime, '/'); i >= 0 && i+1 < len(mime) {
		return mime[i+1:]
	}
	return "jpg"
}

func mimeOr(mime string) string {
	if mime == "" {
		return "application/octet-stream"
	}
	return mime
}

func md5sumBytes(data []byte) []byte {
	sum := md5.Sum(data)
	return sum[:]
}

func md5Hex(data []byte) string {
	return fmt.Sprintf("%x", md5.Sum(data))
}

func nowUnix() int64 { return time.Now().Unix() }

func nowUnixMin() int64 { return time.Now().Unix() / 600 }

func nowUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}

func errNoData(provider string) error {
	return fmt.Errorf("%s: 响应缺少 DATA", provider)
}
