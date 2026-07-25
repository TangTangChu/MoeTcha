package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type tokenClaims struct {
	ID        string
	SessionID string
	IP        string
	UserAgent string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

func buildTokenClaims(sessionID string, ctx VerifyContext, policy TokenPolicy) tokenClaims {
	issued := time.Now()
	ttl := policy.TTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	exp := issued.Add(ttl)
	claims := tokenClaims{
		ID:        RandomHex(16),
		SessionID: sessionID,
		IssuedAt:  issued,
		ExpiresAt: exp,
	}
	if policy.BindIP {
		claims.IP = bindIP(ctx.IP, policy.BindIPPrefix)
	}
	if policy.BindUserAgent {
		claims.UserAgent = ctx.UserAgent
	}
	if policy.BindSession {
		claims.SessionID = sessionID
	}
	return claims
}

func signToken(claims tokenClaims, key string) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%d|%d",
		claims.ID, claims.SessionID, claims.IP, claims.UserAgent,
		claims.IssuedAt.Unix(), claims.ExpiresAt.Unix())
	sig := hmacSHA256(data, key)
	return fmt.Sprintf("%s.%s", data, sig)
}

func verifyToken(raw string, policy TokenPolicy) (tokenClaims, error) {
	idx := strings.LastIndex(raw, ".")
	if idx < 0 {
		return tokenClaims{}, fmt.Errorf("token 格式错误")
	}
	data := raw[:idx]
	sig := raw[idx+1:]
	fields := strings.Split(data, "|")
	if len(fields) != 6 {
		return tokenClaims{}, fmt.Errorf("token 内容不完整")
	}
	issuedAt, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return tokenClaims{}, fmt.Errorf("token 时间错误")
	}
	expiresAt, err := strconv.ParseInt(fields[5], 10, 64)
	if err != nil {
		return tokenClaims{}, fmt.Errorf("token 时间错误")
	}
	claims := tokenClaims{
		ID:        fields[0],
		SessionID: fields[1],
		IP:        fields[2],
		UserAgent: fields[3],
		IssuedAt:  time.Unix(issuedAt, 0),
		ExpiresAt: time.Unix(expiresAt, 0),
	}
	match := verifyHMAC(data, sig, policy.SigningKey, policy.SigningKeyNext)
	if match == hmacNoMatch {
		return tokenClaims{}, fmt.Errorf("token 签名错误")
	}
	if match == hmacNextMatch {
		if policy.RotationGrace <= 0 || time.Since(claims.IssuedAt) > policy.RotationGrace {
			return tokenClaims{}, fmt.Errorf("token 过期密钥")
		}
	}
	if time.Now().After(claims.ExpiresAt) {
		return tokenClaims{}, fmt.Errorf("token 已过期")
	}
	return claims, nil
}

func verifyTokenBinding(claims tokenClaims, ctx VerifyContext, policy TokenPolicy) error {
	if policy.BindIP {
		if claims.IP == "" {
			return fmt.Errorf("token 缺少 IP 绑定")
		}
		if bindIP(ctx.IP, policy.BindIPPrefix) != claims.IP {
			return fmt.Errorf("token IP 不匹配")
		}
	}
	if policy.BindUserAgent {
		if claims.UserAgent == "" {
			return fmt.Errorf("token 缺少 UA 绑定")
		}
		if ctx.UserAgent != claims.UserAgent {
			return fmt.Errorf("token UA 不匹配")
		}
	}
	return nil
}

func hmacSHA256(data, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

const (
	hmacNoMatch = iota
	hmacKeyMatch
	hmacNextMatch
)

func verifyHMAC(data, sig, key, next string) int {
	if sig == hmacSHA256(data, key) {
		return hmacKeyMatch
	}
	if next == "" {
		return hmacNoMatch
	}
	if sig == hmacSHA256(data, next) {
		return hmacNextMatch
	}
	return hmacNoMatch
}

func bindIP(ip string, prefix int) string {
	if ip == "" {
		return ""
	}
	if prefix <= 0 || prefix >= 32 {
		return ip
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ip
	}
	if prefix >= 24 {
		return strings.Join(parts[:3], ".")
	}
	return ip
}
