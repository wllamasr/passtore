// Command passtore es el cliente de línea de comandos del gestor de contraseñas.
// Opera directamente sobre la librería vault (sin agente ni red).
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/wllamasr/passtore/internal/vault"
)

// flagVault captura --vault <ruta> presente en cualquier posición.
var flagVault string

const usage = `passtore — gestor de contraseñas local, portable y cifrado

Uso:
  passtore <comando> [opciones]

Gestión del vault:
  init [--path <ruta>]   Crea un vault nuevo (pide contraseña maestra)
  where                  Muestra la ruta del vault actual
  use <ruta>             Apunta la config a un vault existente (restaurar)
  move <ruta>            Mueve el archivo del vault y actualiza la config
  export <destino>       Copia el vault (ya cifrado) como respaldo
  import <origen>        Trae un vault desde otra ubicación
  destroy                Borra el archivo del vault (pide confirmación)

Entradas:
  add                    Añade una entrada (interactivo)
  list [--tag <t>]       Lista entradas (sin mostrar contraseñas)
  get <id|título>        Muestra una entrada (--show para ver la contraseña, --copy para copiarla)
  otp <id|título>        Muestra el código TOTP actual (--copy para copiarlo)
  search <texto>         Busca entradas
  edit <id|título>       Edita una entrada
  rm <id|título>         Elimina una entrada (pide confirmación)
  change-password        Cambia la contraseña maestra (rekey)

Opciones globales:
  --vault <ruta>         Usa este archivo de vault (ignora config/env)

La ubicación del vault se resuelve: --vault > $PASSTORE_VAULT > config.json >
~/Documents/Passtore/vault.pstore
`

func main() {
	args := extractGlobalFlags(os.Args[1:])
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(1)
	}
	cmd, rest := args[0], args[1:]

	var err error
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	case "init":
		err = cmdInit(rest)
	case "where":
		err = cmdWhere(rest)
	case "use":
		err = cmdUse(rest)
	case "move":
		err = cmdMove(rest)
	case "export":
		err = cmdExport(rest)
	case "import":
		err = cmdImport(rest)
	case "destroy":
		err = cmdDestroy(rest)
	case "add":
		err = cmdAdd(rest)
	case "list":
		err = cmdList(rest)
	case "get":
		err = cmdGet(rest)
	case "otp":
		err = cmdOTP(rest)
	case "search":
		err = cmdSearch(rest)
	case "edit":
		err = cmdEdit(rest)
	case "rm":
		err = cmdRemove(rest)
	case "change-password":
		err = cmdChangePassword(rest)
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %q\n\n", cmd)
		fmt.Print(usage)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// extractGlobalFlags saca --vault (con valor pegado o separado) de cualquier
// posición y devuelve el resto de argumentos.
func extractGlobalFlags(in []string) []string {
	var out []string
	for i := 0; i < len(in); i++ {
		a := in[i]
		switch {
		case a == "--vault":
			if i+1 < len(in) {
				flagVault = in[i+1]
				i++
			}
		case strings.HasPrefix(a, "--vault="):
			flagVault = strings.TrimPrefix(a, "--vault=")
		default:
			out = append(out, a)
		}
	}
	return out
}

// resolvePath resuelve la ruta del vault según prioridad (flag/env/config/default).
func resolvePath() (string, error) {
	return vault.ResolveVaultPath(flagVault)
}

// openVault resuelve la ruta, pide la contraseña maestra y abre el vault.
func openVault() (*vault.Vault, error) {
	path, err := resolvePath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("no hay vault en %q. Usa 'passtore init' para crear uno o 'passtore use <ruta>' para apuntar a uno existente", path)
	}
	master, err := promptMaster()
	if err != nil {
		return nil, err
	}
	defer zeroBytes(master)
	return vault.Open(path, master)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
