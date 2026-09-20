# Passtore — Extensión de navegador

Extensión (Manifest V3, Chrome/Edge) para **autofill** y **guardar contraseñas**
desde formularios, usando tu vault local. No habla con la red: se comunica con
un **host de native messaging** (`passtore-native`) que a su vez llama al agente
local por loopback. El navegador nunca ve el token ni abre puertos.

```
Extensión ── native messaging ──> passtore-native ── HTTP loopback ──> passtore-agent ──> vault (.pstore)
```

## Instalación (desarrollo)

1. **Compila los binarios de Go** (desde la raíz del repo):
   ```bash
   go build -o bin/passtore-agent.exe ./cmd/passtore-agent
   go build -o bin/passtore-native.exe ./cmd/passtore-native
   ```
   Asegúrate de tener un vault creado (`passtore init`).

2. **Carga la extensión**:
   - Ve a `chrome://extensions` (o `edge://extensions`).
   - Activa **Modo de desarrollador**.
   - **Cargar descomprimida** → selecciona la carpeta `clients/extension`.
   - Copia el **ID de la extensión** que aparece.

3. **Registra el host nativo** (PowerShell):
   ```powershell
   cd clients/extension/native-host
   .\install.ps1 -ExtensionId <ID-de-la-extension> -NativeExe ..\..\..\bin\passtore-native.exe
   # para Edge añade:  -Browser edge
   ```

4. **Reinicia el navegador**. Abre el popup de Passtore → desbloquea con tu
   contraseña maestra.

> El host nativo arranca el agente automáticamente si no está corriendo (usa
> `PASSTORE_AGENT_BIN` o `passtore-agent` del PATH). Para que lo encuentre en
> desarrollo, deja `passtore-agent.exe` en el PATH o define esa variable.

## Uso
- **Autofill**: abre el popup en una página de login; muestra las entradas del
  dominio actual. Pulsa una (o "Rellenar") para completar usuario y contraseña.
- **Guardar**: al enviar un formulario con contraseña nueva, aparece un banner
  "¿Guardar en Passtore?".
- **OTP / 2FA**:
  - Ver/usar: si una entrada tiene OTP, el popup muestra un chip con el código y su
    cuenta atrás; al pulsarlo, autorellena el campo `one-time-code` de la página y lo copia.
  - Guardar: botón "🔍 Escanear QR de la página" (captura la pestaña visible) o
    "＋ OTP (cámara/imagen)" que abre una página con escaneo por **cámara**, subida de
    imagen o pegado de `otpauth://`. Tras leer el QR, se adjunta a una entrada existente
    o se crea una nueva. El secreto se guarda cifrado en el vault.
- **Bloquear**: botón 🔒 del popup (el agente también hace auto-lock).

## Deshabilitar la webcam (control de administrador)
El escaneo por **cámara** se puede apagar a nivel de máquina (no lo controla el usuario):
- Poner `"otp_webcam_enabled": false` en el `config.json` del agente
  (`%APPDATA%\passtore\config.json`). El agente lo publica en `/capabilities` y la
  extensión oculta el botón de cámara. La captura de pantalla y el pegado siguen disponibles.
- Alternativa enterprise: política gestionada de Chrome (`storage.managed`) con la clave
  `otpWebcamEnabled=false`.

## Notas
- Los iconos son opcionales; puedes añadir `icons/16,48,128.png` y declararlos en
  `manifest.json` si quieres un icono propio.
- El `allowed_origins` del host se fija al ID de tu extensión, así que solo esa
  extensión puede hablar con el host.
