# Passtore

Gestor de contraseñas y notas seguras **local-first**, estilo 1Password. Los
secretos viven en **un único archivo cifrado y portable** (`.pstore`) protegido
por una **contraseña maestra**. Para descifrarlo solo hace falta esa contraseña:
el archivo se puede mover, copiar, respaldar y restaurar libremente.

> Estado: núcleo funcional (fases 1–5). Ver [ASSESSMENT.md](ASSESSMENT.md),
> [docs/DESIGN.md](docs/DESIGN.md) y [docs/SPEC.md](docs/SPEC.md).

Repositorio: `github.com/wllamasr/passtore` · Módulo Go: `github.com/wllamasr/passtore`

## Instalación

- **App de escritorio (Windows):** descarga el instalador `.exe` desde la
  [página de Releases](https://github.com/wllamasr/passtore/releases). Incluye el agente y el host nativo,
  registra la extensión y se auto-actualiza.
- **Extensión:** desde la Chrome Web Store / Edge Add-ons (o el `.zip` adjunto en el Release, cargándola
  descomprimida).
- **Desde el código:** ver "Compilar todo" abajo. Para publicar nuevas versiones, ver [docs/RELEASING.md](docs/RELEASING.md).

## Arquitectura

```
Clientes:   CLI          App Electron        Extensión de navegador
             │                │                        │
             │ in-process     │ HTTP loopback          │ native messaging
             │                │ (127.0.0.1 + token)    ▼
             │                │                 passtore-native (host)
             │                ▼                        │ HTTP loopback
             │        ┌──────────────────────────────────────────┐
             │        │  passtore-agent (daemon)                    │
             │        │  vault desbloqueado en RAM · auto-lock       │
             │        └───────────────────┬──────────────────────────┘
             ▼                            ▼
        ┌───────────────────────────────────────────┐
        │  vault core (Go)                            │
        │  Argon2id (256 MiB) + XChaCha20-Poly1305     │
        └───────────────────┬─────────────────────────┘
                            ▼
                archivo .pstore (cifrado, portable)
```

- **`internal/vault`** — núcleo: cripto, formato `.pstore`, CRUD. Sin red.
- **`cmd/passtore`** — CLI.
- **`cmd/passtore-agent`** — daemon con API loopback autenticada por token y auto-lock.
- **`cmd/passtore-native`** — host de native messaging para la extensión.
- **`clients/electron`** — app de escritorio.
- **`clients/extension`** — extensión de navegador (autofill + guardado + OTP/2FA con escaneo de QR).
- **`internal/otp`** — motor TOTP (RFC 6238) para el 2FA guardado en el vault.

## Seguridad

- **Derivación:** Argon2id endurecido (256 MiB, 3 iter, paralelismo 4) — memory-hard,
  inviable de crackear por fuerza bruta con GPU/ASIC. Es la defensa principal, junto
  con una contraseña maestra fuerte.
- **Cifrado:** XChaCha20-Poly1305 (AEAD), con el header autenticado como AAD.
- **Sin hash de la maestra:** si la clave derivada es incorrecta, la verificación AEAD falla.
- **Agente:** solo escucha en 127.0.0.1, exige token de sesión y bloquea por inactividad.
- **Extensión:** vía native messaging; el navegador no ve el token ni abre puertos.

## Uso rápido (CLI)

```bash
# Compilar
go build -o bin/passtore.exe ./cmd/passtore

# Crear el vault (pide contraseña maestra)
./bin/passtore.exe init

# Añadir, listar, ver, buscar
./bin/passtore.exe add
./bin/passtore.exe list
./bin/passtore.exe get GitHub --copy
./bin/passtore.exe otp GitHub --copy     # código TOTP/2FA
./bin/passtore.exe search trabajo

# Portabilidad del archivo
./bin/passtore.exe where           # dónde está el vault
./bin/passtore.exe move D:\seguro\vault.pstore
./bin/passtore.exe use  E:\backup\vault.pstore   # apuntar a uno existente
./bin/passtore.exe export F:\usb\backup.pstore
./bin/passtore.exe change-password
```

La ubicación por defecto es `~/Documents/Passtore/vault.pstore` y se puede cambiar
con `--vault`, la variable `PASSTORE_VAULT`, o `move`/`use`.

## Compilar todo

```bash
go build -o bin/passtore.exe        ./cmd/passtore
go build -o bin/passtore-agent.exe  ./cmd/passtore-agent
go build -o bin/passtore-native.exe ./cmd/passtore-native
go test ./...
```

Clientes: ver [clients/electron/README.md](clients/electron/README.md) y
[clients/extension/README.md](clients/extension/README.md).

## Requisitos
- Go 1.27+
- Node.js 18+ (solo para la app Electron)
