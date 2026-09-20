package vault

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// Identificadores de algoritmos (guardados en el header para agilidad criptográfica).
const (
	kdfArgon2id    = "argon2id"
	cipherXChaCha  = "xchacha20poly1305"
	saltLen        = 16
	derivedKeyLen  = 32
	xchachaNonce   = chacha20poly1305.NonceSizeX // 24
)

// KDFParams son los parámetros de Argon2id. Se guardan en el header del vault
// para poder endurecerlos en el futuro sin romper vaults viejos.
type KDFParams struct {
	Algo        string `json:"algo"`
	Salt        []byte `json:"salt"`
	MemoryKiB   uint32 `json:"memory_kib"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	KeyLen      uint32 `json:"key_len"`
}

// DefaultKDFParams devuelve parámetros Argon2id endurecidos (256 MiB) con sal nueva.
// 256 MiB por intento hace inviable el crackeo masivo con GPU/ASIC (memory-hard).
func DefaultKDFParams() (KDFParams, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return KDFParams{}, fmt.Errorf("generando sal: %w", err)
	}
	return KDFParams{
		Algo:        kdfArgon2id,
		Salt:        salt,
		MemoryKiB:   256 * 1024, // 256 MiB
		Iterations:  3,
		Parallelism: 4,
		KeyLen:      derivedKeyLen,
	}, nil
}

// deriveKey deriva la clave de cifrado a partir de la contraseña maestra.
func deriveKey(master []byte, p KDFParams) ([]byte, error) {
	if p.Algo != kdfArgon2id {
		return nil, fmt.Errorf("kdf no soportado: %q", p.Algo)
	}
	if len(p.Salt) == 0 {
		return nil, errors.New("sal vacía")
	}
	key := argon2.IDKey(master, p.Salt, p.Iterations, p.MemoryKiB, p.Parallelism, p.KeyLen)
	return key, nil
}

// newNonce genera un nonce aleatorio de 24 bytes para XChaCha20-Poly1305.
func newNonce() ([]byte, error) {
	nonce := make([]byte, xchachaNonce)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generando nonce: %w", err)
	}
	return nonce, nil
}

// seal cifra plaintext con XChaCha20-Poly1305 usando un nonce ya generado.
// aad son datos autenticados adicionales (el header) que no se cifran pero sí se autentican.
// El nonce se incluye en el AAD, por eso se genera antes de llamar aquí.
func seal(key, nonce, plaintext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("iniciando cifrador: %w", err)
	}
	if len(nonce) != xchachaNonce {
		return nil, errors.New("nonce inválido")
	}
	return aead.Seal(nil, nonce, plaintext, aad), nil
}

// decrypt descifra y verifica. Si la clave es incorrecta o los datos fueron
// alterados, el tag Poly1305 no valida y se devuelve error.
func decrypt(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("iniciando cifrador: %w", err)
	}
	if len(nonce) != xchachaNonce {
		return nil, errors.New("nonce inválido")
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrWrongPassword
	}
	return plaintext, nil
}

// aadBytes construye una representación canónica y determinista del header para
// usarla como AAD. No usa JSON para evitar ambigüedad de orden de claves.
func aadBytes(magic string, formatVersion uint8, p KDFParams, cipher string, nonce []byte) []byte {
	var b []byte
	appendField := func(data []byte) {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(data)))
		b = append(b, l[:]...)
		b = append(b, data...)
	}
	appendU32 := func(v uint32) {
		var x [4]byte
		binary.BigEndian.PutUint32(x[:], v)
		b = append(b, x[:]...)
	}
	appendField([]byte(magic))
	b = append(b, formatVersion)
	appendField([]byte(p.Algo))
	appendField(p.Salt)
	appendU32(p.MemoryKiB)
	appendU32(p.Iterations)
	b = append(b, p.Parallelism)
	appendU32(p.KeyLen)
	appendField([]byte(cipher))
	appendField(nonce)
	return b
}

// zero limpia un slice de bytes sensibles en memoria (best-effort).
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
