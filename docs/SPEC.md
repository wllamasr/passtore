# Passtore — Especificación técnica

> Detalles concretos de formato, criptografía, modelo de datos y superficie de comandos. Acompaña a [DESIGN.md](DESIGN.md).
> Fecha: 2026-09-18 · Estado: **decisiones cerradas — listo para Fase 1** · Versión de formato: 1

## 1. Criptografía

### 1.1 Derivación de clave (KDF) — **la defensa principal contra fuerza bruta**
- **Algoritmo:** Argon2id (`golang.org/x/crypto/argon2`).
- **Parámetros por defecto endurecidos** (guardados en el header del vault; ajustables sin romper vaults viejos):
  - `memory` = 262144 KiB (**256 MiB**)
  - `iterations` (time) = 3
  - `parallelism` = 4
  - `key_len` = 32 bytes (256 bits)
- **Sal:** 16 bytes aleatorios (`crypto/rand`), única por vault, guardada en el header.
- La clave derivada de 32 bytes es la clave del cifrador AEAD.
- **Rationale:** 256 MiB por intento hace inviable el crackeo masivo con GPU/ASIC (Argon2id es memory-hard). El desbloqueo legítimo cuesta ~0.3–1 s de una vez. Nota de portabilidad: abrir el vault requiere ~256 MiB de RAM disponibles; aceptable en desktop, a tener en cuenta si algún día corre en dispositivos muy limitados.
- **Política de contraseña maestra:** el eslabón más débil es la contraseña, no la cripto. Al crear/cambiar la maestra se recomienda una **frase larga** (≥ 12–16 chars / varias palabras) y se advierte (no se bloquea) si es corta o común.

### 1.2 Cifrado (AEAD) — **decidido: XChaCha20-Poly1305**
- **Algoritmo:** XChaCha20-Poly1305 (`golang.org/x/crypto/chacha20poly1305`, `NewX`). Elegido por ser el más robusto: nonce de 24 bytes (sin riesgo de reutilización), software constante en tiempo (resistente a timing/caché), sin dependencia de hardware AES.
- **Nonce:** 24 bytes aleatorios por operación de guardado (`crypto/rand`). No se reutiliza.
- **AAD (datos autenticados adicionales):** los campos del header (magic, version, kdf, cipher, nonce) se incluyen como AAD para que no puedan alterarse sin invalidar el tag.
- **Agilidad:** el campo `cipher` del header permite migrar a otro AEAD (p.ej. AES-256-GCM) en el futuro sin romper vaults existentes.

### 1.3 Verificación de contraseña
No se almacena hash de la contraseña maestra. Al desbloquear se deriva la clave y se intenta descifrar el payload; si el tag Poly1305 no valida → **contraseña incorrecta o archivo corrupto** (no se distingue, a propósito).

### 1.4 Cambio de contraseña maestra (`rekey`)
Genera **sal nueva**, deriva clave nueva y re-cifra el payload completo con nonce nuevo. El archivo resultante ya no es descifrable con la contraseña anterior.

## 2. Formato del archivo de vault (`.pstore`)

Envoltura **JSON** (inspeccionable, portable). Campos binarios en base64 (std, con padding).

```jsonc
{
  "magic": "passtore-vault",
  "format_version": 1,
  "kdf": {
    "algo": "argon2id",
    "salt": "<base64 16 bytes>",
    "memory_kib": 262144,
    "iterations": 3,
    "parallelism": 4,
    "key_len": 32
  },
  "cipher": "xchacha20poly1305",
  "nonce": "<base64 24 bytes>",
  "ciphertext": "<base64: AEAD(payload_plano)>",
  "created_at": "2026-09-18T00:00:00Z",
  "updated_at": "2026-09-18T00:00:00Z"
}
```

**Autocontenido:** `salt`, parámetros KDF, `cipher` y `nonce` viven en el archivo → se puede descifrar en cualquier lugar solo con la contraseña maestra. El header (todo menos `ciphertext`) es AAD.

### 2.1 Payload en claro (una vez descifrado)
```jsonc
{
  "schema_version": 1,
  "entries": [ /* ver §3 */ ]
}
```

### 2.2 Escritura atómica
Guardar = escribir a `vault.pstore.tmp` en el mismo directorio → `fsync` → `rename` sobre `vault.pstore`. Evita corrupción ante fallos. Opcional: rotar un `.bak` previo.

## 3. Modelo de datos (entradas)

```jsonc
{
  "id": "<uuid v4>",
  "type": "login",              // login | note | card | identity | env  (extensible)
  "title": "GitHub",
  "username": "wllamas",
  "password": "<secreto>",
  "url": "https://github.com",
  "notes": "<texto libre / nota segura>",
  "fields": [                    // campos personalizados
    { "name": "TOTP secret", "value": "...", "secret": true }
  ],
  "tags": ["dev", "trabajo"],
  "created_at": "2026-09-18T00:00:00Z",
  "updated_at": "2026-09-18T00:00:00Z"
}
```

- `type: "note"` → nota segura (usa `title` + `notes`).
- Todo el objeto va cifrado dentro del payload; no hay campos "en claro" por entrada.

## 4. Resolución de ubicación y config

### 4.1 Orden de resolución del vault
1. `--vault <ruta>` (flag) o `PASSTORE_VAULT` (env).
2. `vault_path` en `config.json`.
3. Por defecto: `~/Documents/Passtore/vault.pstore`.

### 4.2 `config.json` (no secreto)
Ubicación: directorio de config del SO.
- Windows: `%APPDATA%\passtore\config.json`
- Linux: `${XDG_CONFIG_HOME:-~/.config}/passtore/config.json`
- macOS: `~/Library/Application Support/passtore/config.json`

```jsonc
{
  "vault_path": "C:\\Users\\Wilmer\\Documents\\Passtore\\vault.pstore",
  "auto_lock_minutes": 15
}
```

Si `config.json` no existe o se borra: no se pierde nada; se usa la ruta por defecto o el flag/env.

## 5. Superficie de la librería `vault` core (Fase 1)

API tentativa (Go). Los secretos se manejan como `[]byte` donde se pueda para poder limpiarlos.

```go
package vault

type Vault struct { /* header + entries en memoria; expuesto solo desbloqueado */ }

type Entry struct { /* ver §3 */ }

// Crea un vault nuevo cifrado en disco.
func Create(path string, master []byte, opts ...Option) (*Vault, error)

// Abre y descifra un vault existente. Error si la contraseña es incorrecta.
func Open(path string, master []byte) (*Vault, error)

// Persiste cambios (re-cifra con nonce nuevo, escritura atómica).
func (v *Vault) Save() error

// Limpia la clave derivada y los secretos de memoria.
func (v *Vault) Lock()

// CRUD
func (v *Vault) Add(e Entry) (id string, err error)
func (v *Vault) Get(id string) (Entry, error)
func (v *Vault) List() []Entry
func (v *Vault) Update(e Entry) error
func (v *Vault) Delete(id string) error
func (v *Vault) Search(q string) []Entry

// Cambia la contraseña maestra (nueva sal + re-cifrado).
func (v *Vault) Rekey(newMaster []byte) error
```

**Tests de Fase 1 (obligatorios):** round-trip cifrar/descifrar; contraseña incorrecta falla; header alterado invalida el tag (AAD); portabilidad (crear en ruta A, mover archivo, abrir desde ruta B); `rekey` invalida la contraseña vieja; escritura atómica no corrompe.

## 6. Superficie del CLI (Fase 2)

```
passtore init [--path <ruta>]        # crea vault nuevo (pide master 2 veces)
passtore where                       # imprime la ruta actual del vault
passtore use <ruta>                  # apunta config a un vault existente (restaurar)
passtore move <nueva-ruta>           # mueve el archivo y actualiza config
passtore export <destino>            # copia el vault (ya cifrado) como backup
passtore import <origen>             # trae un vault desde otra ubicación
passtore destroy                     # borra el vault (confirmación explícita)

passtore add                         # añade entrada (interactivo)
passtore get <id|título>             # muestra una entrada
passtore list [--tag <t>]            # lista entradas (sin secretos por defecto)
passtore search <texto>              # busca
passtore edit <id>                   # edita
passtore rm <id>                     # elimina entrada (confirmación)
passtore change-password             # rekey
```

**Convenciones:** la contraseña maestra se pide por prompt oculto (nunca por flag/argumento, que quedaría en el historial). Los valores de contraseñas se copian al portapapeles o se muestran solo bajo demanda; nunca se imprimen en `list`.

## 7. Errores y bordes
- Contraseña incorrecta ↔ archivo corrupto: mismo error genérico (no filtrar cuál).
- Vault inexistente en la ruta resuelta: sugerir `passtore init` o `passtore use`.
- Permisos de archivo: crear `.pstore` con permisos restrictivos (`0600` en Unix; ACL de usuario en Windows).
- Concurrencia: lock de archivo simple para evitar dos escritores (relevante con el agente en Fase 3).

## 8. Dependencias previstas
- `golang.org/x/crypto/argon2`, `golang.org/x/crypto/chacha20poly1305`
- `github.com/google/uuid` (o UUID propio)
- stdlib: `crypto/rand`, `crypto/cipher`, `encoding/json`, `os`, `path/filepath`
- CLI: `github.com/spf13/cobra` (o `flag` de stdlib para empezar minimal)
- **Se descartan:** Fiber, gorm, MySQL, godotenv, go-playground/validator (del esqueleto viejo).

## 9. OTP / TOTP (2FA)

- **Almacenamiento:** cada entrada puede llevar `otpauth` (URI `otpauth://totp/...` con secreto + parámetros). Se cifra como el resto de la entrada; **el secreto nunca sale del agente**.
- **Motor:** `internal/otp` implementa RFC 6238 (HMAC-SHA1/256/512, truncado dinámico) sin dependencias. Verificado con los vectores oficiales del RFC.
- **Agente:**
  - `GET /entries/{id}/otp` → `{code, digits, period, expires_in}` (calculado en el agente).
  - Los listados incluyen `has_otp` (bool) pero nunca el secreto.
  - `GET /capabilities` → `{otp_webcam_enabled}` (flag a nivel de máquina).
- **Gate de webcam (admin, no usuario):** `otp_webcam_enabled` en `config.json` (default `true`; ausente ⇒ habilitado). Lo edita quien administra la máquina; ningún cliente lo expone en su UI. La extensión oculta el botón de cámara si está `false`. Override opcional enterprise: política gestionada de Chrome (`storage.managed`, clave `otpWebcamEnabled`). Captura de pantalla y pegado manual siempre disponibles.
- **Captura del QR (extensión):** captura de la pestaña visible (`chrome.tabs.captureVisibleTab` + `jsQR`), subida de imagen, pegado de `otpauth://`, y webcam (gated). Tras decodificar, se adjunta a una entrada existente o se crea una nueva.
- **Clientes:** CLI (`passtore otp <id|título>`, y en `get`), Electron (fila OTP con cuenta atrás) y extensión (chip en la lista + autorelleno del campo `one-time-code`).
