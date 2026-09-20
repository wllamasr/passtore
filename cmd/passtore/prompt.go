package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

var stdin = bufio.NewReader(os.Stdin)

// readLine lee una línea de texto (visible) con un prompt.
func readLine(prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := stdin.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readSecret lee texto oculto (sin eco) desde la terminal. Si stdin no es una
// terminal (p.ej. pipe en scripts/tests), lee una línea normal.
func readSecret(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println()
		return b, err
	}
	line, err := stdin.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		return nil, err
	}
	return []byte(strings.TrimRight(line, "\r\n")), nil
}

// promptMaster pide la contraseña maestra una vez.
func promptMaster() ([]byte, error) {
	return readSecret("Contraseña maestra: ")
}

// promptNewMaster pide una contraseña maestra nueva dos veces y valida que
// coincidan. Advierte (sin bloquear) si es débil.
func promptNewMaster() ([]byte, error) {
	p1, err := readSecret("Nueva contraseña maestra: ")
	if err != nil {
		return nil, err
	}
	p2, err := readSecret("Confírmala: ")
	if err != nil {
		return nil, err
	}
	if string(p1) != string(p2) {
		return nil, errors.New("las contraseñas no coinciden")
	}
	if len(p1) < 12 {
		fmt.Fprintln(os.Stderr, "⚠  Advertencia: una contraseña maestra fuerte debería tener al menos 12 caracteres (mejor una frase larga). Es tu única defensa contra fuerza bruta.")
	}
	return p1, nil
}

// confirm pide confirmación sí/no para operaciones destructivas.
func confirm(question string) bool {
	ans, _ := readLine(question + " [s/N]: ")
	ans = strings.ToLower(strings.TrimSpace(ans))
	return ans == "s" || ans == "si" || ans == "sí" || ans == "y" || ans == "yes"
}
