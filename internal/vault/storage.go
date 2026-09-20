package vault

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic escribe data en path de forma atómica: escribe a un archivo
// temporal en el mismo directorio, hace fsync y luego rename sobre el destino.
// Así un fallo a mitad de escritura nunca corrompe el vault existente.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creando directorio %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".pstore-*.tmp")
	if err != nil {
		return fmt.Errorf("creando archivo temporal: %w", err)
	}
	tmpName := tmp.Name()

	// Si algo falla, limpiamos el temporal.
	defer func() {
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ajustando permisos: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("escribiendo datos: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sincronizando a disco: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cerrando temporal: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renombrando a destino: %w", err)
	}
	return nil
}
