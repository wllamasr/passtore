// Package otp implementa códigos TOTP (RFC 6238) para el 2FA guardado en el vault.
// No tiene dependencias externas.
package otp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"hash"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config son los parámetros de un token TOTP.
type Config struct {
	Secret    []byte // secreto ya decodificado
	Algorithm string // SHA1 | SHA256 | SHA512
	Digits    int    // normalmente 6
	Period    int    // segundos, normalmente 30
	Label     string // cuenta (informativo)
	Issuer    string // emisor (informativo)
}

const (
	defaultAlgorithm = "SHA1"
	defaultDigits    = 6
	defaultPeriod    = 30
)

// FromSecret construye una Config a partir de un secreto base32 suelto, con los
// parámetros por defecto (SHA1/6/30).
func FromSecret(base32Secret string) (Config, error) {
	sec, err := decodeBase32(base32Secret)
	if err != nil {
		return Config{}, err
	}
	return Config{Secret: sec, Algorithm: defaultAlgorithm, Digits: defaultDigits, Period: defaultPeriod}, nil
}

// ParseOtpauth parsea un URI otpauth://totp/Label?secret=...&issuer=...&algorithm=...&digits=...&period=...
func ParseOtpauth(uri string) (Config, error) {
	u, err := url.Parse(strings.TrimSpace(uri))
	if err != nil {
		return Config{}, fmt.Errorf("otpauth inválido: %w", err)
	}
	if u.Scheme != "otpauth" {
		return Config{}, fmt.Errorf("no es un URI otpauth://")
	}
	if strings.ToLower(u.Host) != "totp" {
		return Config{}, fmt.Errorf("solo se soporta totp (host=%q)", u.Host)
	}
	q := u.Query()
	secret := q.Get("secret")
	if secret == "" {
		return Config{}, fmt.Errorf("falta el parámetro secret")
	}
	sec, err := decodeBase32(secret)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Secret:    sec,
		Algorithm: defaultAlgorithm,
		Digits:    defaultDigits,
		Period:    defaultPeriod,
		Label:     strings.TrimPrefix(u.Path, "/"),
		Issuer:    q.Get("issuer"),
	}
	if a := strings.ToUpper(q.Get("algorithm")); a != "" {
		cfg.Algorithm = a
	}
	if d := q.Get("digits"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			cfg.Digits = n
		}
	}
	if p := q.Get("period"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			cfg.Period = n
		}
	}
	return cfg, nil
}

// Parse acepta tanto un otpauth:// como un secreto base32 suelto.
func Parse(s string) (Config, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "otpauth://") {
		return ParseOtpauth(s)
	}
	return FromSecret(s)
}

// Code calcula el código TOTP para el instante t (RFC 6238).
func Code(cfg Config, t time.Time) (string, error) {
	if cfg.Period <= 0 {
		cfg.Period = defaultPeriod
	}
	if cfg.Digits <= 0 {
		cfg.Digits = defaultDigits
	}
	counter := uint64(t.Unix()) / uint64(cfg.Period)
	return hotp(cfg, counter)
}

// Now devuelve el código actual y los segundos que le quedan de validez.
func Now(cfg Config) (string, int, error) {
	now := time.Now()
	code, err := Code(cfg, now)
	if err != nil {
		return "", 0, err
	}
	period := cfg.Period
	if period <= 0 {
		period = defaultPeriod
	}
	remaining := period - int(now.Unix()%int64(period))
	return code, remaining, nil
}

// hotp implementa HOTP (RFC 4226) sobre el que se construye TOTP.
func hotp(cfg Config, counter uint64) (string, error) {
	newHash, err := hasherFor(cfg.Algorithm)
	if err != nil {
		return "", err
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(newHash, cfg.Secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// Truncado dinámico (RFC 4226 §5.3).
	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)

	mod := uint32(math.Pow10(cfg.Digits))
	return fmt.Sprintf("%0*d", cfg.Digits, binCode%mod), nil
}

func hasherFor(algorithm string) (func() hash.Hash, error) {
	switch strings.ToUpper(algorithm) {
	case "", "SHA1":
		return sha1.New, nil
	case "SHA256":
		return sha256.New, nil
	case "SHA512":
		return sha512.New, nil
	default:
		return nil, fmt.Errorf("algoritmo no soportado: %q", algorithm)
	}
}

// decodeBase32 decodifica un secreto base32 (RFC 4648), tolerante a minúsculas,
// espacios y ausencia de padding.
func decodeBase32(s string) ([]byte, error) {
	clean := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	clean = strings.TrimRight(clean, "=")
	if clean == "" {
		return nil, fmt.Errorf("secreto vacío")
	}
	sec, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("secreto base32 inválido: %w", err)
	}
	return sec, nil
}
