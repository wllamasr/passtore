package otp

import (
	"testing"
	"time"
)

// Vectores oficiales del RFC 6238 (Appendix B), con 8 dígitos y period 30.
// Semillas ASCII repetidas hasta el tamaño de clave de cada algoritmo.
func TestRFC6238Vectors(t *testing.T) {
	seedSHA1 := []byte("12345678901234567890")
	seedSHA256 := []byte("12345678901234567890123456789012")
	seedSHA512 := []byte("1234567890123456789012345678901234567890123456789012345678901234")

	cases := []struct {
		time int64
		sha1 string
		s256 string
		s512 string
	}{
		{59, "94287082", "46119246", "90693936"},
		{1111111109, "07081804", "68084774", "25091201"},
		{1111111111, "14050471", "67062674", "99943326"},
		{1234567890, "89005924", "91819424", "93441116"},
		{2000000000, "69279037", "90698825", "38618901"},
		{20000000000, "65353130", "77737706", "47863826"},
	}

	for _, c := range cases {
		tm := time.Unix(c.time, 0)
		check := func(algo string, seed []byte, want string) {
			cfg := Config{Secret: seed, Algorithm: algo, Digits: 8, Period: 30}
			got, err := Code(cfg, tm)
			if err != nil {
				t.Fatalf("%s @%d: %v", algo, c.time, err)
			}
			if got != want {
				t.Errorf("%s @%d: got %s, want %s", algo, c.time, got, want)
			}
		}
		check("SHA1", seedSHA1, c.sha1)
		check("SHA256", seedSHA256, c.s256)
		check("SHA512", seedSHA512, c.s512)
	}
}

func TestParseOtpauth(t *testing.T) {
	// secret base32 de "Hello!\xDE\xAD\xBE\xEF" = JBSWY3DPEHPK3PXP (ejemplo típico)
	uri := "otpauth://totp/ACME:alice@example.com?secret=JBSWY3DPEHPK3PXP&issuer=ACME&algorithm=SHA256&digits=7&period=45"
	cfg, err := ParseOtpauth(uri)
	if err != nil {
		t.Fatalf("ParseOtpauth: %v", err)
	}
	if cfg.Algorithm != "SHA256" || cfg.Digits != 7 || cfg.Period != 45 {
		t.Fatalf("parámetros mal parseados: %+v", cfg)
	}
	if cfg.Issuer != "ACME" {
		t.Fatalf("issuer mal parseado: %q", cfg.Issuer)
	}
	if len(cfg.Secret) == 0 {
		t.Fatal("secreto vacío")
	}
	// Debe poder generar un código de 7 dígitos.
	code, err := Code(cfg, time.Unix(1000, 0))
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if len(code) != 7 {
		t.Fatalf("esperaba 7 dígitos, got %q", code)
	}
}

func TestParseAcceptsBareSecret(t *testing.T) {
	cfg, err := Parse("jbswy3dpehpk3pxp") // minúsculas, sin padding
	if err != nil {
		t.Fatalf("Parse secreto suelto: %v", err)
	}
	if cfg.Digits != 6 || cfg.Period != 30 || cfg.Algorithm != "SHA1" {
		t.Fatalf("defaults incorrectos: %+v", cfg)
	}
}

func TestNowRemaining(t *testing.T) {
	cfg, _ := FromSecret("JBSWY3DPEHPK3PXP")
	code, remaining, err := Now(cfg)
	if err != nil {
		t.Fatalf("Now: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("código de 6 dígitos esperado, got %q", code)
	}
	if remaining < 1 || remaining > 30 {
		t.Fatalf("remaining fuera de rango: %d", remaining)
	}
}

func TestRejectsHOTPAndBadSecret(t *testing.T) {
	if _, err := ParseOtpauth("otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP"); err == nil {
		t.Error("debería rechazar hotp")
	}
	if _, err := FromSecret("no-es-base32-!!!"); err == nil {
		t.Error("debería rechazar secreto inválido")
	}
}
