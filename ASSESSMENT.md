# Passtore — Assessment y plan de revival

> Documento de evaluación del estado del proyecto y hoja de ruta para retomarlo.
> Fecha: 2026-09-18

## 1. Objetivo del proyecto

Un gestor de contraseñas y notas seguras **local-first**, estilo 1Password:

- Un **vault local cifrado** protegido por una **master key** que el usuario recuerda.
- La master key deriva la clave de cifrado; los secretos se cifran/descifran localmente.
- Clientes externos que consumen el vault:
  - **App de escritorio (Electron)** como UI principal.
  - **Extensión de navegador** para autocompletar formularios y guardar contraseñas desde ellos.
  - (Opcional) **CLI** para uso rápido en terminal.

## 2. Estado actual (lo que hay hoy)

El repo es un **esqueleto de API REST en Go en etapa muy temprana**. De la funcionalidad de gestor de contraseñas **no existe nada todavía**.

### Stack actual
| Componente | Versión | Nota |
|---|---|---|
| Go | 1.15 | EOL (sin soporte) |
| [Fiber](https://gofiber.io) | v2.2.0 | Muy viejo (actual ~2.52) |
| jinzhu/gorm | v1.9.16 | **Deprecado** (reemplazado por `gorm.io/gorm`) |
| MySQL | — | Driver vía gorm |
| godotenv | v1.3.0 | Carga `.env` |

### Qué hace el código
- `main.go` → `app.StartApplication()`.
- `app/app.go` — arranca Fiber en `:3000` con middlewares (cors, cache, logger, requestid) y conecta a MySQL.
- `app/map_urls.go` — única ruta: `GET /` → `{"hello":"world"}`.
- `models/user.go` — modelo `User` con hashing bcrypt (hook `BeforeSave`).
- `models/model.go` — modelo base (ID, timestamps, soft delete).
- `utils/validations/` — validador custom `unrepeated` (unicidad en BD) + wrapper de go-playground/validator.
- `config/db/database.go` — conexión MySQL vía gorm.

## 3. Problema de fondo: arquitectura ≠ objetivo

Lo montado es una **API REST cliente-servidor multi-usuario contra MySQL** (cuentas + bcrypt). El objetivo es un **vault local cifrado single-user con master key**. Son diseños distintos:

- **bcrypt** sirve para *verificar* passwords (login); **no es reversible**, así que no sirve para cifrar/descifrar el contenido del vault. Para eso se necesita cifrado simétrico reversible (**AES-256-GCM**) con la clave derivada de la master key mediante un KDF (**Argon2id**).
- Un vault local **no necesita** un servidor MySQL ni cuentas de usuario.

**Decisión tomada:** vault local (librería/CLI) como núcleo, consumido por clientes (Electron + extensión).

## 4. Bugs y deuda técnica del código actual

1. **Build roto — variable shadowed:** `config/db/database.go:27` usa `Client, err := gorm.Open(...)` (`:=`), creando un `Client` local que tapa la variable global del paquete. El `db.Client` global queda en `nil` → nil panic en cualquier consulta. Debe ser `Client, err = gorm.Open(...)`.
2. **`go.mod` incompleto:** el código importa `golang.org/x/crypto/bcrypt` y `go-playground/validator/v10`, pero no están en el bloque `require` (validator ni siquiera está en `go.sum`). **No compila** sin `go mod tidy`.
3. **Struct tags inválidos:**
   - `models/user.go:11` → `validate: "required"` y `gorm: "..."` (espacio tras los `:` = tag inválido, se ignora en silencio).
   - `models/model.go:6` → `json:"id"gorm:"primary_key"` (tags pegados sin espacio).
4. **`.env` basura:** es copia-pega de un proyecto Laravel ("Booking Core", redis, mail, `APP_KEY`). Casi todo sin usar.
5. **Errores ignorados:** `hashPassword()` descarta el error de bcrypt; `ctx.JSON(...)` no chequea retorno.

## 5. Entorno

- **Go NO está instalado** en la máquina — primer bloqueante.
- Dependencias EOL/deprecadas (ver tabla arriba).

## 6. Arquitectura objetivo

Modelo local-first estilo 1Password, en capas:

```
┌─────────────────────────────────────────────────────────────┐
│  Clientes                                                     │
│  ┌──────────────┐  ┌───────────────┐  ┌──────────────────┐   │
│  │ App Electron │  │ Extensión web │  │  CLI (opcional)  │   │
│  └──────┬───────┘  └───────┬───────┘  └────────┬─────────┘   │
│         │ HTTP loopback    │ native messaging  │ in-process   │
│         │ (127.0.0.1+token)│                    │             │
└─────────┼──────────────────┼────────────────────┼────────────┘
          ▼                  ▼                    ▼
┌─────────────────────────────────────────────────────────────┐
│  Agente local (daemon)                                        │
│  - Mantiene el vault DESBLOQUEADO en memoria                  │
│  - Auto-lock por inactividad                                  │
│  - API loopback autenticada (token de sesión)                 │
└──────────────────────────┬────────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  vault core (paquete Go, sin red)                             │
│  - Master key → Argon2id → clave de cifrado                   │
│  - AES-256-GCM sobre las entradas                             │
│  - Init / Unlock / Lock / Add / Get / List / Update / Delete  │
│  - Almacenamiento: archivo cifrado local (o SQLite cifrado)   │
└─────────────────────────────────────────────────────────────┘
```

### Piezas
- **`vault` core (librería Go):** el corazón. Puro, testeable sin UI ni red. Crypto + formato de almacenamiento + operaciones CRUD sobre entradas.
- **Agente local (daemon):** el único proceso que tiene el vault descifrado en RAM. Expone API **solo en loopback** con token de sesión y auto-lock. Aquí *podría* reutilizarse Fiber (bindeado a 127.0.0.1). Es donde va la mayor parte del diseño de seguridad.
- **Clientes:** Electron (HTTP loopback), extensión (native messaging host → agente), CLI (usa el core directamente o vía agente).

### Decisiones de seguridad clave (a profundizar)
- La master key **nunca se guarda**; solo vive la clave derivada en memoria mientras el vault está desbloqueado.
- Guardar un *verification token* (valor conocido cifrado) para validar que la master key es correcta al desbloquear.
- La API loopback debe estar **autenticada**: otras apps locales o extensiones no deben poder leer el vault. Handshake con token + verificación de origen.
- Auto-lock por inactividad; borrado de secretos en memoria cuando se pueda.
- Nunca poner secretos en URLs/query strings ni en logs.

### Modernización propuesta
- **Go** última estable (1.23+).
- **Crypto:** `golang.org/x/crypto/argon2` + stdlib `crypto/aes` + `crypto/cipher` (GCM).
- **Almacenamiento:** archivo cifrado (JSON/msgpack) para empezar; si hace falta búsqueda/consulta → SQLite pure-Go (`modernc.org/sqlite`, sin CGO).
- **API (si se usa):** Fiber v2.52+ o v3, solo en loopback.
- **Descartar:** MySQL, jinzhu/gorm, multi-usuario, `.env` Laravel, validador `unrepeated`.

## 7. Qué se reutiliza del código actual

| Elemento | Decisión |
|---|---|
| Esqueleto Fiber (`app/`) | Reutilizable como base del agente loopback (o descartar; es mínimo). |
| `models/user.go` (bcrypt) | Mayormente innecesario en single-user; la master key va por Argon2id. |
| `config/db` (MySQL/gorm) | Descartar. |
| `utils/validations` | Descartar (`unrepeated` era para unicidad de emails). |
| `.env` | Descartar / reescribir. |

## 8. Roadmap de revival

- [ ] **Fase 0 — Setup:** instalar Go, dejar el repo compilando (arreglar bug de `:=`, `go mod tidy`), confirmar arquitectura.
- [ ] **Fase 1 — `vault` core:** librería con Argon2id + AES-256-GCM, formato de vault, CRUD de entradas, **tests unitarios**. Se construye y prueba de forma aislada.
- [ ] **Fase 2 — CLI:** cliente fino sobre el core (`add`, `get`, `list`, ...). Es la vía más rápida de ejercitar el core de punta a punta.
- [ ] **Fase 3 — Agente local:** API loopback autenticada, auto-lock, manejo de sesión desbloqueada.
- [ ] **Fase 4 — Cliente Electron:** UI de escritorio contra el agente.
- [ ] **Fase 5 — Extensión de navegador:** native messaging host + autofill + guardar desde formularios.

## 9. Riesgos / decisiones abiertas

- Formato de almacenamiento: archivo cifrado único vs SQLite cifrado (afecta búsqueda y crecimiento).
- Canal extensión ↔ agente: native messaging (estándar, más seguro) vs localhost directo.
- Multi-vault / multi-perfil a futuro.
- Sincronización entre dispositivos (¿fuera de alcance por ahora? el objetivo es local).
