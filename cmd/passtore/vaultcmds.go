package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wllamasr/passtore/internal/vault"
)

// cmdInit crea un vault nuevo. Acepta --path para elegir ubicación; si no, usa
// la ruta resuelta (flag/env/config/default) y la guarda en config.
func cmdInit(args []string) error {
	path := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--path" && i+1 < len(args) {
			path = args[i+1]
			i++
		}
	}
	var err error
	if path == "" {
		if path, err = resolvePath(); err != nil {
			return err
		}
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("ya existe un archivo en %q", path)
	}

	fmt.Printf("Creando vault en: %s\n", path)
	master, err := promptNewMaster()
	if err != nil {
		return err
	}
	defer zeroBytes(master)

	v, err := vault.Create(path, master)
	if err != nil {
		return err
	}
	v.Lock()

	if err := vault.SetVaultPath(path); err != nil {
		return fmt.Errorf("vault creado, pero no pude guardar la ruta en config: %w", err)
	}
	fmt.Println("✔ Vault creado. Guárdalo/respáldalo: es un único archivo portable.")
	return nil
}

// cmdWhere muestra la ruta actual del vault y si existe.
func cmdWhere(_ []string) error {
	path, err := resolvePath()
	if err != nil {
		return err
	}
	fmt.Println(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "(el archivo aún no existe)")
	}
	return nil
}

// cmdUse apunta la config a un vault existente (escenario de restauración).
func cmdUse(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: passtore use <ruta>")
	}
	path, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("no existe el archivo %q", path)
	}
	if err := vault.SetVaultPath(path); err != nil {
		return err
	}
	fmt.Printf("✔ Config apuntando a: %s\n", path)
	return nil
}

// cmdMove mueve físicamente el vault a una ruta nueva y actualiza la config.
func cmdMove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: passtore move <nueva-ruta>")
	}
	src, err := resolvePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("no hay vault en %q", src)
	}
	dst, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("ya existe un archivo en el destino %q", dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	if err := moveFile(src, dst); err != nil {
		return err
	}
	if err := vault.SetVaultPath(dst); err != nil {
		return err
	}
	fmt.Printf("✔ Vault movido a: %s\n", dst)
	return nil
}

// cmdExport copia el vault (ya cifrado) a un destino como respaldo.
func cmdExport(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: passtore export <destino>")
	}
	src, err := resolvePath()
	if err != nil {
		return err
	}
	dst := args[0]
	// Si el destino es un directorio, respetar el nombre original.
	if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	fmt.Printf("✔ Respaldo cifrado escrito en: %s\n", dst)
	return nil
}

// cmdImport trae un vault desde otra ubicación: lo copia a la ruta resuelta y
// la deja como activa. No modifica el original.
func cmdImport(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: passtore import <origen>")
	}
	src := args[0]
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("no existe el origen %q", src)
	}
	dst, err := resolvePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		if !confirm(fmt.Sprintf("Ya existe un vault en %q. ¿Sobrescribir?", dst)) {
			return fmt.Errorf("cancelado")
		}
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	if err := vault.SetVaultPath(dst); err != nil {
		return err
	}
	fmt.Printf("✔ Vault importado a: %s\n", dst)
	return nil
}

// cmdDestroy borra el archivo del vault tras confirmación explícita.
func cmdDestroy(_ []string) error {
	path, err := resolvePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("no hay vault en %q", path)
	}
	fmt.Printf("Vas a BORRAR permanentemente el vault: %s\n", path)
	if !confirm("Esto es irreversible. ¿Continuar?") {
		return fmt.Errorf("cancelado")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	fmt.Println("✔ Vault borrado.")
	return nil
}

// ---- helpers de archivo ----

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// moveFile mueve un archivo; si el rename falla (p.ej. cruzando volúmenes),
// hace copia + borrado del origen.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}
