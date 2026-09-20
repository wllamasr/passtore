# Passtore — Design Doc

> Diseño del gestor de contraseñas local-first. Acompaña a [SPEC.md](SPEC.md) (detalles técnicos) y [../ASSESSMENT.md](../ASSESSMENT.md) (estado y roadmap).
> Fecha: 2026-09-18 · Estado: **decisiones de diseño cerradas — listo para Fase 1**

## 1. Objetivo y principios

Gestor de contraseñas y notas seguras **local-first** estilo 1Password.

**Principios de diseño:**
1. **Local-first:** los secretos nunca salen del equipo salvo que el usuario lo decida. Sin servidor obligatorio, sin cuenta en la nube.
2. **Vault portable y autocontenido:** toda la información vive en **un único archivo** que se puede mover, copiar, respaldar y restaurar libremente. Para descifrarlo solo hace falta la **contraseña maestra** — nada más (ni config, ni claves externas, ni la máquina original).
3. **Zero-knowledge del almacenamiento:** en disco todo está cifrado. Quien tenga el archivo pero no la contraseña maestra no puede leer nada.
4. **El usuario manda sobre sus datos:** puede ver dónde está el vault, cambiarlo de lugar, borrarlo o restaurarlo sin fricción.
5. **Agilidad criptográfica:** los parámetros de cifrado se guardan *dentro* del archivo, así se pueden endurecer en el futuro sin romper vaults viejos.

## 2. Requisito clave: portabilidad y control del almacenamiento

> "Que la ubicación de la base de datos sea accesible; que el usuario pueda cambiar la ubicación, borrar la info o restaurarla sin problemas, y solo usando la contraseña maestra."

Esto se logra separando **dos cosas** que suelen confundirse:

| | El **vault** (archivo `.pstore`) | El **config** (`config.json`) |
|---|---|---|
| Qué contiene | Todos los secretos, **cifrados** | Solo un puntero: la ruta actual del vault + preferencias |
| ¿Secreto? | Sí (cifrado con la master key) | No — es texto plano, no revela nada |
| ¿Portable? | **Sí, totalmente autocontenido** | Es local a la máquina, desechable |
| Si se pierde | Se pierden los datos (salvo backup) | No pasa nada: se vuelve a apuntar al vault |

**Consecuencias prácticas:**
- **Cambiar de ubicación:** mover el archivo `.pstore` a otra carpeta/disco/USB y actualizar el puntero (`passtore move` o `passtore use`). El archivo sigue funcionando idéntico.
- **Respaldar:** copiar el `.pstore` a donde sea. Como ya está cifrado, el backup es seguro por sí mismo.
- **Restaurar:** colocar el `.pstore` de vuelta y apuntar la app a él (`passtore use <ruta>`). Con la contraseña maestra queda operativo.
- **Borrar:** eliminar el archivo (`passtore destroy` con confirmación, o borrarlo a mano).
- **Mover a otra PC:** copiar el archivo; en la otra máquina solo se necesita la contraseña maestra.

**Ubicación por defecto (accesible):** `~/Documents/Passtore/vault.pstore` — una carpeta visible y fácil de encontrar/respaldar, en vez de un directorio oculto del sistema. El usuario puede cambiarla cuando quiera.

**Resolución de ruta** (orden de prioridad):
1. Flag explícito `--vault <ruta>` o variable de entorno `PASSTORE_VAULT`.
2. Ruta guardada en `config.json`.
3. Ruta por defecto (`~/Documents/Passtore/vault.pstore`).

## 3. Arquitectura en capas

```
Clientes:   [ CLI ]   [ App Electron ]   [ Extensión navegador ]
                │             │                     │
                │ in-process  │ HTTP loopback       │ native messaging
                │             │ (127.0.0.1 + token) │
                ▼             ▼                     ▼
            ┌───────────────────────────────────────────┐
            │  Agente local (daemon)                      │  ← Fases 3+
            │  vault desbloqueado en RAM · auto-lock       │
            │  API loopback autenticada                    │
            └───────────────────┬─────────────────────────┘
                                ▼
            ┌───────────────────────────────────────────┐
            │  vault core (paquete Go, sin red)           │  ← Fase 1
            │  cripto + formato + CRUD de entradas         │
            └───────────────────┬─────────────────────────┘
                                ▼
                    archivo .pstore (cifrado, portable)
```

- **`vault` core:** librería pura. No sabe de red ni de UI. Es lo primero que se construye y se prueba de forma aislada.
- **Agente local:** único proceso con el vault descifrado en memoria. Introduce el modelo de sesión (unlock/lock, auto-lock) y la API loopback. Necesario para Electron y la extensión, no para el CLI.
- **Clientes:** capas finas sobre el core (CLI) o sobre el agente (Electron, extensión).

## 4. Modelo criptográfico (resumen)

Detalle completo y parámetros exactos en [SPEC.md](SPEC.md).

- **Derivación de clave:** contraseña maestra → **Argon2id** (con sal aleatoria por vault) → clave de 256 bits.
- **Cifrado del vault:** **XChaCha20-Poly1305** (AEAD; nonce de 24 bytes aleatorio, sin riesgo de reutilización). AES-256-GCM queda como alternativa contemplada.
- **Verificación de contraseña:** no se guarda hash de la maestra. Si la clave derivada es incorrecta, la verificación AEAD del payload falla → "contraseña incorrecta o archivo corrupto". Simple y sin material extra que filtrar.
- **En cada guardado** se re-cifra el vault completo con un nonce nuevo.
- **La master key nunca se persiste.** Solo la clave derivada vive en memoria mientras el vault está desbloqueado; se limpia al bloquear.

## 5. Modelo de seguridad y amenazas

**Qué protege:**
- Robo del archivo `.pstore` (disco, backup, USB, sync a nube de terceros): inútil sin la contraseña maestra.
- Otras apps locales / extensiones: la API del agente exige token de sesión; no cualquiera lee el vault.

**Qué NO protege (fuera de alcance inicial, a documentar como límites):**
- Equipo comprometido con malware/keylogger mientras el vault está desbloqueado.
- Volcado de RAM con el vault desbloqueado (se mitiga con auto-lock y limpieza de memoria, no se elimina).
- Fuerza bruta si la contraseña maestra es débil (se mitiga con Argon2id costoso, pero el usuario debe elegir buena contraseña).

**Decisiones de seguridad:**
- Auto-lock por inactividad (agente).
- Nunca loguear secretos ni ponerlos en URLs/query strings.
- Escritura atómica del vault (archivo temporal + rename) para no corromper en caso de fallo.
- Confirmación explícita en operaciones destructivas (`destroy`, `rm`).

## 6. Decisiones (cerradas 2026-09-18)

1. **Cifrador:** ✅ **XChaCha20-Poly1305** — el AEAD más robusto (nonce de 24 B sin riesgo de reutilización, software constante en tiempo, sin dependencia de hardware AES).
2. **KDF endurecido:** ✅ **Argon2id con 256 MiB / 3 iteraciones / paralelismo 4**. Es la defensa real contra fuerza bruta (memory-hard → inviable con GPU/ASIC). Parámetros en el header → endurecibles sin romper vaults viejos. Complementado con política de contraseña maestra fuerte (frase larga; advertencia si es débil).
3. **Formato del payload:** ✅ **blob JSON cifrado** — simple y 100% portable. SQLite solo se evaluaría si el volumen/búsqueda lo exige.
4. **Ubicación:** ✅ por defecto `~/Documents/Passtore/vault.pstore` (accesible), **con el usuario pudiendo elegir/cambiar la ruta** vía `passtore init --path`, `passtore move`, `passtore use` o `PASSTORE_VAULT` / `--vault`.
5. **Canal extensión ↔ agente:** ✅ **native messaging** (estándar de 1Password). Más seguro que exponer el puerto loopback al navegador: no hay puerto abierto a páginas web, ni CORS, ni riesgo de DNS-rebinding. El navegador habla con un host `passtore-native` que reenvía al agente por loopback; el token nunca llega al navegador.

## 7. Roadmap (ver ASSESSMENT.md para detalle)

Fase 0 setup ✔ → **Fase 1 `vault` core** → Fase 2 CLI → Fase 3 agente → Fase 4 Electron → Fase 5 extensión.
