package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EnvVaultPath es la variable de entorno que fuerza la ruta del vault.
const EnvVaultPath = "PASSTORE_VAULT"

// Config es el archivo NO secreto que apunta a la ubicación del vault y guarda
// preferencias. Si se pierde, no se pierde ningún dato: solo hay que volver a
// apuntar al archivo .pstore.
type Config struct {
	VaultPath       string `json:"vault_path"`
	AutoLockMinutes int    `json:"auto_lock_minutes"`
	// OTPWebcamEnabled es un control a nivel de máquina (lo edita el admin en
	// config.json, no se expone en la UI de los clientes). Puntero: ausente ⇒
	// habilitado por defecto. Ponerlo en false deshabilita el escaneo por webcam.
	OTPWebcamEnabled *bool `json:"otp_webcam_enabled,omitempty"`
}

// WebcamEnabled indica si el escaneo de OTP por webcam está permitido.
// Por defecto true (cuando el campo no está presente en config.json).
func (c Config) WebcamEnabled() bool {
	return c.OTPWebcamEnabled == nil || *c.OTPWebcamEnabled
}

// ConfigPath devuelve la ruta de config.json en el directorio de config del SO
// (Windows: %APPDATA%\passtore; Linux: ~/.config/passtore; macOS: Application Support).
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolviendo directorio de config: %w", err)
	}
	return filepath.Join(dir, "passtore", "config.json"), nil
}

// DefaultVaultPath devuelve la ubicación accesible por defecto:
// ~/Documents/Passtore/vault.pstore
func DefaultVaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolviendo home: %w", err)
	}
	return filepath.Join(home, "Documents", "Passtore", "vault.pstore"), nil
}

// LoadConfig lee config.json. Si no existe, devuelve un Config vacío (sin error).
func LoadConfig() (Config, error) {
	p, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("leyendo config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("config.json inválido: %w", err)
	}
	return c, nil
}

// SaveConfig escribe config.json de forma atómica.
func SaveConfig(c Config) error {
	p, err := ConfigPath()
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando config: %w", err)
	}
	return writeFileAtomic(p, out, 0o600)
}

// ResolveVaultPath decide qué ruta de vault usar, en orden de prioridad:
//  1. flagPath (si no está vacío) — flag --vault
//  2. variable de entorno PASSTORE_VAULT
//  3. vault_path de config.json
//  4. ruta por defecto (~/Documents/Passtore/vault.pstore)
func ResolveVaultPath(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	if env := os.Getenv(EnvVaultPath); env != "" {
		return env, nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}
	if cfg.VaultPath != "" {
		return cfg.VaultPath, nil
	}
	return DefaultVaultPath()
}

// SetVaultPath persiste la ruta del vault en config.json (usado por `use`/`move`).
func SetVaultPath(path string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.VaultPath = path
	return SaveConfig(cfg)
}
