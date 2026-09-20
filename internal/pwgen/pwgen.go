// Package pwgen genera contraseñas aleatorias criptográficamente seguras.
package pwgen

import (
	"crypto/rand"
	"math/big"
	"strings"
)

const (
	lower   = "abcdefghijkmnopqrstuvwxyz"    // sin l
	upper   = "ABCDEFGHJKLMNPQRSTUVWXYZ"     // sin I, O
	digits  = "23456789"                     // sin 0, 1
	symbols = "!@#$%^&*()-_=+[]{};:,.?"
)

// Options configura la generación.
type Options struct {
	Length  int
	Upper   bool
	Digits  bool
	Symbols bool
}

// DefaultOptions: 20 caracteres con mayúsculas, dígitos y símbolos.
func DefaultOptions() Options {
	return Options{Length: 20, Upper: true, Digits: true, Symbols: true}
}

// Generate produce una contraseña aleatoria según opts usando crypto/rand.
func Generate(opts Options) (string, error) {
	if opts.Length <= 0 {
		opts.Length = 20
	}
	pool := lower
	if opts.Upper {
		pool += upper
	}
	if opts.Digits {
		pool += digits
	}
	if opts.Symbols {
		pool += symbols
	}

	var sb strings.Builder
	sb.Grow(opts.Length)
	max := big.NewInt(int64(len(pool)))
	for i := 0; i < opts.Length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		sb.WriteByte(pool[n.Int64()])
	}
	return sb.String(), nil
}
