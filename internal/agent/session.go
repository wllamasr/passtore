// Package agent implementa el agente local: un daemon que mantiene el vault
// desbloqueado en memoria y expone una API HTTP solo en loopback (127.0.0.1),
// autenticada con un token de sesión y con auto-lock por inactividad.
package agent

import (
	"errors"
	"sync"
	"time"

	"github.com/wllamasr/passtore/internal/vault"
)

// Session mantiene el estado del vault desbloqueado y el temporizador de auto-lock.
type Session struct {
	mu      sync.Mutex
	path    string
	v       *vault.Vault
	timeout time.Duration
	timer   *time.Timer
}

// NewSession crea una sesión para el vault en path. timeout <= 0 desactiva el auto-lock.
func NewSession(path string, timeout time.Duration) *Session {
	return &Session{path: path, timeout: timeout}
}

// Path devuelve la ruta del vault gestionado.
func (s *Session) Path() string { return s.path }

// Locked indica si el vault está bloqueado (sin clave en memoria).
func (s *Session) Locked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v == nil
}

// Unlock abre el vault con la contraseña maestra y arranca el auto-lock.
func (s *Session) Unlock(master []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.v != nil {
		return nil // ya desbloqueado
	}
	v, err := vault.Open(s.path, master)
	if err != nil {
		return err
	}
	s.v = v
	s.resetTimerLocked()
	return nil
}

// Lock bloquea el vault, limpia la clave de memoria y detiene el temporizador.
func (s *Session) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockLocked()
}

func (s *Session) lockLocked() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.v != nil {
		s.v.Lock()
		s.v = nil
	}
}

// resetTimerLocked reinicia el auto-lock. Debe llamarse con el lock tomado.
func (s *Session) resetTimerLocked() {
	if s.timeout <= 0 {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.timeout, s.Lock)
}

// ErrLocked se devuelve cuando se opera sobre un vault bloqueado.
var ErrLocked = errors.New("vault bloqueado")

// withVault ejecuta fn con el vault desbloqueado, reiniciando el auto-lock.
// Devuelve ErrLocked si el vault no está desbloqueado.
func (s *Session) withVault(fn func(*vault.Vault) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.v == nil {
		return ErrLocked
	}
	s.resetTimerLocked()
	return fn(s.v)
}
