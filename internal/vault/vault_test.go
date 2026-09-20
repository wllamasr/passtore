package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// lightKDF devuelve parámetros Argon2id ligeros para que los tests sean rápidos.
// La producción usa DefaultKDFParams (256 MiB).
func lightKDF(t *testing.T) KDFParams {
	t.Helper()
	p, err := DefaultKDFParams()
	if err != nil {
		t.Fatalf("DefaultKDFParams: %v", err)
	}
	p.MemoryKiB = 8 * 1024 // 8 MiB
	p.Iterations = 1
	p.Parallelism = 1
	return p
}

func newVault(t *testing.T, path string, master string) *Vault {
	t.Helper()
	v, err := Create(path, []byte(master), WithKDFParams(lightKDF(t)))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return v
}

func TestCreateOpenRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.pstore")
	v := newVault(t, path, "correct horse battery staple")

	id, err := v.Add(Entry{Type: TypeLogin, Title: "GitHub", Username: "wllamas", Password: "s3cr3t", URL: "https://github.com"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := v.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	v.Lock()

	// Reabrir con la contraseña correcta.
	v2, err := Open(path, []byte("correct horse battery staple"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := v2.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "GitHub" || got.Password != "s3cr3t" || got.Username != "wllamas" {
		t.Fatalf("entrada no coincide tras round-trip: %+v", got)
	}
}

func TestOpenWrongPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.pstore")
	v := newVault(t, path, "master-uno")
	if err := v.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := Open(path, []byte("master-DOS"))
	if err != ErrWrongPassword {
		t.Fatalf("esperaba ErrWrongPassword, obtuve: %v", err)
	}
}

func TestTamperedHeaderFailsAAD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.pstore")
	v := newVault(t, path, "master")
	if err := v.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Alterar un campo del header (created_at) sin tocar el ciphertext.
	raw, _ := os.ReadFile(path)
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	env["created_at"] = "2000-01-01T00:00:00Z"
	tampered, _ := json.Marshal(env)
	if err := os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// created_at NO es parte del AAD, así que abrir aún debe funcionar
	// (el AAD cubre magic, version, kdf, cipher, nonce, no los timestamps).
	if _, err := Open(path, []byte("master")); err != nil {
		t.Fatalf("alterar created_at no debería romper: %v", err)
	}

	// Ahora alterar la sal (SÍ es parte del AAD y del KDF) debe fallar.
	raw, _ = os.ReadFile(path)
	_ = json.Unmarshal(raw, &env)
	kdf := env["kdf"].(map[string]any)
	kdf["salt"] = "AAAAAAAAAAAAAAAAAAAAAA==" // 16 bytes de ceros en base64
	tampered, _ = json.Marshal(env)
	_ = os.WriteFile(path, tampered, 0o600)
	if _, err := Open(path, []byte("master")); err != ErrWrongPassword {
		t.Fatalf("alterar la sal debería dar ErrWrongPassword, obtuve: %v", err)
	}
}

func TestPortabilityMoveFile(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	pathA := filepath.Join(dirA, "vault.pstore")
	pathB := filepath.Join(dirB, "otro-nombre.pstore")

	v := newVault(t, pathA, "clave-portable")
	id, _ := v.Add(Entry{Title: "Correo", Username: "yo@x.com", Password: "pw"})
	if err := v.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Mover el archivo a otra ubicación/nombre (simula cambio de carpeta o USB).
	raw, _ := os.ReadFile(pathA)
	if err := os.WriteFile(pathB, raw, 0o600); err != nil {
		t.Fatalf("copiar: %v", err)
	}

	// Debe abrir desde la nueva ruta solo con la contraseña maestra.
	v2, err := Open(pathB, []byte("clave-portable"))
	if err != nil {
		t.Fatalf("Open tras mover: %v", err)
	}
	if got, err := v2.Get(id); err != nil || got.Title != "Correo" {
		t.Fatalf("entrada no recuperada tras mover: %v %+v", err, got)
	}
}

func TestRekeyInvalidatesOldPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.pstore")
	v := newVault(t, path, "vieja")
	id, _ := v.Add(Entry{Title: "X", Password: "p"})
	if err := v.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := v.Rekey([]byte("nueva-y-mas-larga")); err != nil {
		t.Fatalf("Rekey: %v", err)
	}

	// La contraseña vieja ya no sirve.
	if _, err := Open(path, []byte("vieja")); err != ErrWrongPassword {
		t.Fatalf("la contraseña vieja debería fallar, obtuve: %v", err)
	}
	// La nueva sí, y los datos siguen.
	v2, err := Open(path, []byte("nueva-y-mas-larga"))
	if err != nil {
		t.Fatalf("Open con nueva: %v", err)
	}
	if _, err := v2.Get(id); err != nil {
		t.Fatalf("datos perdidos tras rekey: %v", err)
	}
}

func TestCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.pstore")
	v := newVault(t, path, "m")

	id1, _ := v.Add(Entry{Title: "Beta", Username: "b", Tags: []string{"trabajo"}})
	id2, _ := v.Add(Entry{Title: "alpha", Username: "a"})

	// List ordena por título (case-insensitive): alpha, Beta.
	list, err := v.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].Title != "alpha" || list[1].Title != "Beta" {
		t.Fatalf("List mal ordenada: %+v", list)
	}

	// Update.
	e, _ := v.Get(id1)
	e.Username = "beta-user"
	if err := v.Update(e); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, _ := v.Get(id1); got.Username != "beta-user" {
		t.Fatalf("Update no aplicó: %+v", got)
	}

	// Search por tag.
	res, _ := v.Search("trabajo")
	if len(res) != 1 || res[0].ID != id1 {
		t.Fatalf("Search por tag falló: %+v", res)
	}

	// Delete.
	if err := v.Delete(id2); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := v.Get(id2); err != ErrNotFound {
		t.Fatalf("esperaba ErrNotFound tras Delete, obtuve: %v", err)
	}
}

func TestLockedOperationsFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.pstore")
	v := newVault(t, path, "m")
	v.Lock()
	if _, err := v.List(); err != ErrLocked {
		t.Fatalf("List tras Lock debería dar ErrLocked, obtuve: %v", err)
	}
	if _, err := v.Add(Entry{Title: "x"}); err != ErrLocked {
		t.Fatalf("Add tras Lock debería dar ErrLocked, obtuve: %v", err)
	}
}

func TestCreateRefusesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.pstore")
	newVault(t, path, "m")
	if _, err := Create(path, []byte("m"), WithKDFParams(lightKDF(t))); err != ErrExists {
		t.Fatalf("Create sobre existente debería dar ErrExists, obtuve: %v", err)
	}
}
