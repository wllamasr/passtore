// Package vault implementa el núcleo del gestor de contraseñas: un vault local
// cifrado, portable y autocontenido. Un único archivo .pstore lleva dentro su
// propia sal, parámetros de Argon2id, cifrador y nonce, de modo que puede moverse
// a cualquier lugar y descifrarse solo con la contraseña maestra.
//
// Esta capa no sabe nada de red ni de UI.
package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	magic         = "passtore-vault"
	formatVersion = uint8(1)
	schemaVersion = 1
	vaultPerm     = 0o600 // rw solo para el dueño
)

// Errores públicos.
var (
	// ErrWrongPassword: la contraseña maestra es incorrecta o el archivo está
	// corrupto/alterado. No se distingue entre ambos casos a propósito.
	ErrWrongPassword = errors.New("contraseña maestra incorrecta o vault corrupto")
	ErrLocked        = errors.New("el vault está bloqueado")
	ErrNotFound      = errors.New("entrada no encontrada")
	ErrExists        = errors.New("ya existe un vault en esa ruta")
	ErrBadFormat     = errors.New("formato de vault inválido")
)

// envelope es la estructura JSON en disco. Los campos []byte se serializan en
// base64 automáticamente. Todo menos ciphertext forma el header (y el AAD).
type envelope struct {
	Magic         string    `json:"magic"`
	FormatVersion uint8     `json:"format_version"`
	KDF           KDFParams `json:"kdf"`
	Cipher        string    `json:"cipher"`
	Nonce         []byte    `json:"nonce"`
	Ciphertext    []byte    `json:"ciphertext"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// payload es el contenido en claro, una vez descifrado.
type payload struct {
	SchemaVersion int     `json:"schema_version"`
	Entries       []Entry `json:"entries"`
}

// Vault representa un vault abierto (desbloqueado) en memoria.
type Vault struct {
	path      string
	key       []byte // clave derivada; vive solo mientras está desbloqueado
	kdf       KDFParams
	createdAt time.Time
	data      payload
	locked    bool
}

// Option configura la creación de un vault.
type Option func(*createOpts)

type createOpts struct {
	kdf *KDFParams
}

// WithKDFParams fuerza parámetros de KDF concretos (p.ej. sal fija o parámetros
// ligeros en tests). Por defecto se usan los parámetros endurecidos.
func WithKDFParams(p KDFParams) Option {
	return func(o *createOpts) { o.kdf = &p }
}

// Create crea un vault nuevo cifrado en path. Falla si ya existe un archivo ahí.
// master es la contraseña maestra (se puede limpiar tras la llamada).
func Create(path string, master []byte, opts ...Option) (*Vault, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, ErrExists
	}
	var co createOpts
	for _, o := range opts {
		o(&co)
	}
	var (
		kdf KDFParams
		err error
	)
	if co.kdf != nil {
		kdf = *co.kdf
	} else if kdf, err = DefaultKDFParams(); err != nil {
		return nil, err
	}
	key, err := deriveKey(master, kdf)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	v := &Vault{
		path:      path,
		key:       key,
		kdf:       kdf,
		createdAt: now,
		data:      payload{SchemaVersion: schemaVersion, Entries: []Entry{}},
	}
	if err := v.Save(); err != nil {
		zero(key)
		return nil, err
	}
	return v, nil
}

// Open abre y descifra un vault existente. Devuelve ErrWrongPassword si la
// contraseña es incorrecta (o el archivo fue alterado).
func Open(path string, master []byte) (*Vault, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leyendo vault: %w", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFormat, err)
	}
	if env.Magic != magic {
		return nil, ErrBadFormat
	}
	if env.FormatVersion != formatVersion {
		return nil, fmt.Errorf("%w: versión de formato %d no soportada", ErrBadFormat, env.FormatVersion)
	}
	if env.Cipher != cipherXChaCha {
		return nil, fmt.Errorf("%w: cifrador %q no soportado", ErrBadFormat, env.Cipher)
	}

	key, err := deriveKey(master, env.KDF)
	if err != nil {
		return nil, err
	}
	aad := aadBytes(env.Magic, env.FormatVersion, env.KDF, env.Cipher, env.Nonce)
	plain, err := decrypt(key, env.Nonce, env.Ciphertext, aad)
	if err != nil {
		zero(key)
		return nil, err // ErrWrongPassword
	}

	var data payload
	if err := json.Unmarshal(plain, &data); err != nil {
		zero(key)
		zero(plain)
		return nil, fmt.Errorf("%w: payload ilegible", ErrBadFormat)
	}
	zero(plain)

	return &Vault{
		path:      path,
		key:       key,
		kdf:       env.KDF,
		createdAt: env.CreatedAt,
		data:      data,
		locked:    false,
	}, nil
}

// Save re-cifra el payload completo con un nonce nuevo y lo escribe atómicamente.
func (v *Vault) Save() error {
	if v.locked {
		return ErrLocked
	}
	plain, err := json.Marshal(v.data)
	if err != nil {
		return fmt.Errorf("serializando payload: %w", err)
	}
	defer zero(plain)

	nonce, err := newNonce()
	if err != nil {
		return err
	}
	aad := aadBytes(magic, formatVersion, v.kdf, cipherXChaCha, nonce)
	ct, err := seal(v.key, nonce, plain, aad)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if v.createdAt.IsZero() {
		v.createdAt = now
	}
	env := envelope{
		Magic:         magic,
		FormatVersion: formatVersion,
		KDF:           v.kdf,
		Cipher:        cipherXChaCha,
		Nonce:         nonce,
		Ciphertext:    ct,
		CreatedAt:     v.createdAt,
		UpdatedAt:     now,
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando envelope: %w", err)
	}
	return writeFileAtomic(v.path, out, vaultPerm)
}

// Lock limpia la clave derivada y los secretos de memoria. Tras esto el vault
// queda inutilizable hasta reabrirlo con Open.
func (v *Vault) Lock() {
	zero(v.key)
	v.key = nil
	for i := range v.data.Entries {
		v.data.Entries[i].Password = ""
		for j := range v.data.Entries[i].Fields {
			v.data.Entries[i].Fields[j].Value = ""
		}
	}
	v.data.Entries = nil
	v.locked = true
}

// Path devuelve la ruta actual del archivo de vault.
func (v *Vault) Path() string { return v.path }

// ---- CRUD ----

// Add añade una entrada nueva, asignándole ID y timestamps. Devuelve el ID.
func (v *Vault) Add(e Entry) (string, error) {
	if v.locked {
		return "", ErrLocked
	}
	e.ID = uuid.NewString()
	now := time.Now().UTC()
	e.CreatedAt = now
	e.UpdatedAt = now
	if e.Type == "" {
		e.Type = TypeLogin
	}
	v.data.Entries = append(v.data.Entries, e)
	return e.ID, nil
}

// Get devuelve una copia de la entrada con el ID dado.
func (v *Vault) Get(id string) (Entry, error) {
	if v.locked {
		return Entry{}, ErrLocked
	}
	for _, e := range v.data.Entries {
		if e.ID == id {
			return e, nil
		}
	}
	return Entry{}, ErrNotFound
}

// List devuelve todas las entradas, ordenadas por título.
func (v *Vault) List() ([]Entry, error) {
	if v.locked {
		return nil, ErrLocked
	}
	out := make([]Entry, len(v.data.Entries))
	copy(out, v.data.Entries)
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out, nil
}

// Update reemplaza la entrada con el mismo ID, refrescando UpdatedAt.
func (v *Vault) Update(e Entry) error {
	if v.locked {
		return ErrLocked
	}
	for i := range v.data.Entries {
		if v.data.Entries[i].ID == e.ID {
			e.CreatedAt = v.data.Entries[i].CreatedAt
			e.UpdatedAt = time.Now().UTC()
			v.data.Entries[i] = e
			return nil
		}
	}
	return ErrNotFound
}

// Delete elimina la entrada con el ID dado.
func (v *Vault) Delete(id string) error {
	if v.locked {
		return ErrLocked
	}
	for i := range v.data.Entries {
		if v.data.Entries[i].ID == id {
			v.data.Entries = append(v.data.Entries[:i], v.data.Entries[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// Search devuelve las entradas cuyo título, usuario, URL o tags contienen q
// (case-insensitive). No busca dentro de contraseñas.
func (v *Vault) Search(q string) ([]Entry, error) {
	if v.locked {
		return nil, ErrLocked
	}
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return v.List()
	}
	var out []Entry
	for _, e := range v.data.Entries {
		hay := strings.ToLower(e.Title + " " + e.Username + " " + e.URL + " " + strings.Join(e.Tags, " "))
		if strings.Contains(hay, q) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out, nil
}

// Rekey cambia la contraseña maestra: genera sal nueva, deriva clave nueva y
// re-cifra el vault. Tras esto la contraseña anterior deja de servir.
func (v *Vault) Rekey(newMaster []byte) error {
	if v.locked {
		return ErrLocked
	}
	kdf, err := DefaultKDFParams()
	if err != nil {
		return err
	}
	key, err := deriveKey(newMaster, kdf)
	if err != nil {
		return err
	}
	old := v.key
	v.key = key
	v.kdf = kdf
	if err := v.Save(); err != nil {
		v.key = old // revertir en memoria si falló el guardado
		v.kdf = KDFParams{}
		zero(key)
		return err
	}
	zero(old)
	return nil
}
