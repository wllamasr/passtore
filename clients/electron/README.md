# Passtore Desktop (Electron)

Cliente de escritorio para Passtore. Habla con el **agente local** (`passtore-agent`)
por su API loopback (127.0.0.1), autenticada con el token de `agent.json`. El token
vive solo en el proceso principal de Electron; el renderer nunca lo ve.

## Requisitos
- Node.js 18+ (probado con Node 24).
- El binario `passtore-agent` compilado y accesible:
  - en el `PATH`, o
  - vía la variable de entorno `PASSTORE_AGENT_BIN` con la ruta al ejecutable.
- Un vault ya creado (`passtore init`).

## Puesta en marcha (desarrollo)
```bash
# 1. Compilar los binarios de Go (desde la raíz del repo)
go build -o bin/passtore.exe ./cmd/passtore
go build -o bin/passtore-agent.exe ./cmd/passtore-agent

# 2. Crear el vault si aún no existe
./bin/passtore.exe init

# 3. Instalar dependencias de la app
cd clients/electron
npm install

# 4. Arrancar (apuntando al agente compilado)
#    PowerShell:  $env:PASSTORE_AGENT_BIN="..\..\bin\passtore-agent.exe"; npm start
#    bash:        PASSTORE_AGENT_BIN=../../bin/passtore-agent.exe npm start
npm start
```

La app intenta conectar con un agente en marcha; si no lo encuentra, arranca uno
usando `PASSTORE_AGENT_BIN` (o `passtore-agent` del PATH).

## Verificación de sintaxis (sin GUI)
```bash
npm run check
```

## Flujo en la app
1. **Conectar** → localiza/arranca el agente.
2. **Desbloquear** → introduces la contraseña maestra (se envía al agente).
3. **Gestionar** → listar, buscar, ver (con copiar/mostrar), añadir, editar,
   borrar y generar contraseñas.
4. **Bloquear** → cierra la sesión del vault en el agente. El agente también hace
   auto-lock por inactividad.

## Notas de seguridad
- El renderer corre con `contextIsolation: true`, `nodeIntegration: false` y una CSP estricta.
- Las contraseñas copiadas se limpian del portapapeles a los ~20 s.
