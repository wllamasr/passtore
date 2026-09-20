package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wllamasr/passtore/internal/vault"
)

// RuntimeInfo describe cómo alcanzar al agente en ejecución. Se escribe en un
// archivo local protegido para que los clientes (Electron, host de la extensión)
// descubran el puerto y el token de sesión.
type RuntimeInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// RuntimePath devuelve la ruta del archivo agent.json en el directorio de config.
func RuntimePath() (string, error) {
	base, err := vault.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(base), "agent.json"), nil
}

// NewToken genera un token de sesión aleatorio (32 bytes en hex).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generando token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// WriteRuntime escribe agent.json con permisos restrictivos (0600).
func WriteRuntime(info RuntimeInfo) (string, error) {
	p, err := RuntimePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(p, out, 0o600); err != nil {
		return "", err
	}
	return p, nil
}

// LoadRuntime lee agent.json (usado por los clientes).
func LoadRuntime() (RuntimeInfo, error) {
	p, err := RuntimePath()
	if err != nil {
		return RuntimeInfo{}, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return RuntimeInfo{}, err
	}
	var info RuntimeInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return RuntimeInfo{}, err
	}
	return info, nil
}

// RemoveRuntime borra agent.json al apagar el agente.
func RemoveRuntime() {
	if p, err := RuntimePath(); err == nil {
		_ = os.Remove(p)
	}
}
