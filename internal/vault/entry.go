package vault

import "time"

// EntryType clasifica el tipo de secreto almacenado.
type EntryType string

const (
	TypeLogin    EntryType = "login"    // usuario + contraseña (+ url)
	TypeNote     EntryType = "note"     // nota segura (title + notes)
	TypeCard     EntryType = "card"     // tarjeta
	TypeIdentity EntryType = "identity" // identidad
	TypeEnv      EntryType = "env"      // variables/secretos de entorno
)

// CustomField es un campo personalizado de una entrada.
type CustomField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"` // si true, se oculta al listar/mostrar por defecto
}

// Entry es una entrada del vault. Todo el objeto se guarda cifrado dentro del payload.
type Entry struct {
	ID        string        `json:"id"`
	Type      EntryType     `json:"type"`
	Title     string        `json:"title"`
	Username  string        `json:"username,omitempty"`
	Password  string        `json:"password,omitempty"`
	URL       string        `json:"url,omitempty"`
	Notes     string        `json:"notes,omitempty"`
	// OTPAuth guarda el URI otpauth:// del 2FA (contiene el secreto y parámetros).
	// Se cifra como el resto de la entrada; el código se calcula bajo demanda.
	OTPAuth   string        `json:"otpauth,omitempty"`
	Fields    []CustomField `json:"fields,omitempty"`
	Tags      []string      `json:"tags,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}
