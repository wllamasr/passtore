# Guía de publicación y releases

Passtore libera **dos artefactos independientes**, cada uno con su propio tag/versión:

| Artefacto | Tag que lo dispara | Versión | Destino |
|---|---|---|---|
| Cliente de escritorio (instalador Windows) | `desktop-vX.Y.Z` | `clients/electron/package.json` | GitHub Release (+ auto-update) |
| Extensión de navegador | `ext-vX.Y.Z` | `clients/extension/manifest.json` | GitHub Release + Chrome Web Store + Edge Add-ons |

CI (`.github/workflows/ci.yml`) corre en cada push/PR a `master`: build/vet/test de Go y `node --check` del JS.

## Puesta a punto (una sola vez)

1. **Repo público** en GitHub (`wllamasr/passtore`). El `GITHUB_TOKEN` de Actions basta para publicar Releases.
2. **Tiendas de extensiones** (para la publicación automatizada):
   - **Chrome Web Store:** crea cuenta de desarrollador (pago único ~5 USD). Sube el `.zip` **manualmente una vez**
     para crear el ítem y obtener el **Extension ID**. Genera credenciales OAuth (Client ID, Client Secret,
     Refresh Token) siguiendo la guía de `chrome-webstore-upload`.
   - **Edge Add-ons:** cuenta en Partner Center (gratis). Primer envío manual → **Product ID** + credenciales de API.
3. **Rellenar los IDs de la extensión** en `clients/electron/build/installer.nsh`
   (`__CHROME_EXTENSION_ID__`, `__EDGE_EXTENSION_ID__`) para que el instalador registre el host nativo con los
   `allowed_origins` correctos.
4. **Secretos del repo** (Settings → Secrets and variables → Actions):
   - Chrome: `CHROME_EXTENSION_ID`, `CHROME_CLIENT_ID`, `CHROME_CLIENT_SECRET`, `CHROME_REFRESH_TOKEN`.
   - Edge: `EDGE_PRODUCT_ID`, `EDGE_CLIENT_ID`, `EDGE_API_KEY`.
   - (Opcional) Firma Windows: `WIN_CSC_LINK` (base64 del .pfx), `WIN_CSC_KEY_PASSWORD`.
   > Sin los secretos de tienda, el workflow de extensión igual empaqueta el `.zip` y lo adjunta al Release;
   > solo se salta la subida a las tiendas.
5. **Iconos/ficha:** los iconos ya están (`clients/extension/icons/*`, `clients/electron/build/icon.png`).
   Prepara capturas y textos para las fichas de las tiendas.

## Sacar una versión del **cliente de escritorio**

```bash
# 1. Sube la versión en clients/electron/package.json (p.ej. 0.2.0)
# 2. Commit
git commit -am "Desktop v0.2.0"
# 3. Tag + push
git tag desktop-v0.2.0
git push origin master --tags
```

El workflow (runner Windows) compila los 3 binarios Go, corre `electron-builder --win --publish always` y publica
el instalador `.exe` + `latest.yml` en el GitHub Release. Las instalaciones previas se **auto-actualizan**
(electron-updater comprueba Releases al abrir).

**Dry-run local** (sin publicar): compila los binarios a `clients/electron/bin/` y luego:
```bash
cd clients/electron && npm ci && npm run dist   # genera el instalador en dist/
```

## Sacar una versión de la **extensión**

```bash
# 1. Sube "version" en clients/extension/manifest.json
# 2. Commit + tag
git commit -am "Extension v0.2.0"
git tag ext-v0.2.0
git push origin master --tags
```

El workflow empaqueta el `.zip` (sin `native-host/`), lo adjunta al Release y —si hay secretos— lo publica a
Chrome Web Store (con `--auto-publish`) y Edge Add-ons. Recomendado: revisar primero en modo borrador.

## Notas de seguridad
- **Firma de código:** sin firma, Windows muestra SmartScreen y el auto-update es menos fiable. Añade el
  certificado (`WIN_CSC_*`) antes de una difusión amplia; el certificado NUNCA se commitea, va en Secrets.
- El instalador registra el host nativo en `HKCU` (por usuario, sin elevación) y lo elimina al desinstalar.
- La extensión y el cliente comparten el vault y el agente locales; el token de sesión nunca sale del equipo.
