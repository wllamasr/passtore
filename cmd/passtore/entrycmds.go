package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/wllamasr/passtore/internal/otp"
	"github.com/wllamasr/passtore/internal/vault"
)

// cmdAdd añade una entrada de forma interactiva.
func cmdAdd(_ []string) error {
	v, err := openVault()
	if err != nil {
		return err
	}
	defer v.Lock()

	title, _ := readLine("Título: ")
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("el título es obligatorio")
	}
	username, _ := readLine("Usuario: ")
	pw, err := readSecret("Contraseña (enter para omitir): ")
	if err != nil {
		return err
	}
	url, _ := readLine("URL: ")
	notes, _ := readLine("Notas: ")
	tagsLine, _ := readLine("Tags (separados por coma): ")
	otpauth, _ := readLine("OTP (otpauth:// o secreto base32, enter para omitir): ")
	otpauth = normalizeOTP(otpauth)

	e := vault.Entry{
		Type:     vault.TypeLogin,
		Title:    title,
		Username: username,
		Password: string(pw),
		URL:      url,
		Notes:    notes,
		OTPAuth:  otpauth,
		Tags:     splitTags(tagsLine),
	}
	zeroBytes(pw)

	id, err := v.Add(e)
	if err != nil {
		return err
	}
	if err := v.Save(); err != nil {
		return err
	}
	fmt.Printf("✔ Entrada añadida (id %s)\n", shortID(id))
	return nil
}

// cmdList lista entradas en tabla, sin mostrar contraseñas.
func cmdList(args []string) error {
	tag := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--tag" && i+1 < len(args) {
			tag = args[i+1]
			i++
		}
	}
	v, err := openVault()
	if err != nil {
		return err
	}
	defer v.Lock()

	entries, err := v.List()
	if err != nil {
		return err
	}
	printEntries(filterByTag(entries, tag))
	return nil
}

// cmdGet muestra una entrada. Por defecto oculta la contraseña; --show la
// muestra y --copy la copia al portapapeles.
func cmdGet(args []string) error {
	show, copyPw, query := parseGetFlags(args)
	if query == "" {
		return fmt.Errorf("uso: passtore get <id|título> [--show] [--copy]")
	}
	v, err := openVault()
	if err != nil {
		return err
	}
	defer v.Lock()

	e, err := findEntry(v, query)
	if err != nil {
		return err
	}

	fmt.Printf("Título:   %s\n", e.Title)
	fmt.Printf("Tipo:     %s\n", e.Type)
	if e.Username != "" {
		fmt.Printf("Usuario:  %s\n", e.Username)
	}
	if e.Password != "" {
		if show {
			fmt.Printf("Password: %s\n", e.Password)
		} else {
			fmt.Printf("Password: %s (usa --show para verla)\n", strings.Repeat("•", 8))
		}
	}
	if e.URL != "" {
		fmt.Printf("URL:      %s\n", e.URL)
	}
	if e.Notes != "" {
		fmt.Printf("Notas:    %s\n", e.Notes)
	}
	if len(e.Tags) > 0 {
		fmt.Printf("Tags:     %s\n", strings.Join(e.Tags, ", "))
	}
	if e.OTPAuth != "" {
		if cfg, err := otp.Parse(e.OTPAuth); err == nil {
			if code, rem, err := otp.Now(cfg); err == nil {
				fmt.Printf("OTP:      %s (%ds)\n", code, rem)
			}
		}
	}
	for _, f := range e.Fields {
		val := f.Value
		if f.Secret && !show {
			val = strings.Repeat("•", 8)
		}
		fmt.Printf("· %s: %s\n", f.Name, val)
	}
	fmt.Printf("id:       %s\n", e.ID)

	if copyPw && e.Password != "" {
		if err := copyToClipboard(e.Password); err != nil {
			return fmt.Errorf("no pude copiar al portapapeles: %w", err)
		}
		fmt.Println("✔ Contraseña copiada al portapapeles.")
	}
	return nil
}

// cmdOTP muestra el código TOTP actual de una entrada.
func cmdOTP(args []string) error {
	copyPw := false
	query := ""
	for _, a := range args {
		if a == "--copy" {
			copyPw = true
		} else if query == "" {
			query = a
		} else {
			query += " " + a
		}
	}
	if query == "" {
		return fmt.Errorf("uso: passtore otp <id|título> [--copy]")
	}
	v, err := openVault()
	if err != nil {
		return err
	}
	defer v.Lock()

	e, err := findEntry(v, query)
	if err != nil {
		return err
	}
	if e.OTPAuth == "" {
		return fmt.Errorf("la entrada %q no tiene OTP configurado", e.Title)
	}
	cfg, err := otp.Parse(e.OTPAuth)
	if err != nil {
		return err
	}
	code, remaining, err := otp.Now(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("%s  (válido %d s)\n", code, remaining)
	if copyPw {
		if err := copyToClipboard(code); err != nil {
			return fmt.Errorf("no pude copiar al portapapeles: %w", err)
		}
		fmt.Println("✔ Código copiado al portapapeles.")
	}
	return nil
}

// cmdSearch busca entradas por texto.
func cmdSearch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: passtore search <texto>")
	}
	v, err := openVault()
	if err != nil {
		return err
	}
	defer v.Lock()

	res, err := v.Search(strings.Join(args, " "))
	if err != nil {
		return err
	}
	printEntries(res)
	return nil
}

// cmdEdit edita una entrada de forma interactiva (enter mantiene el valor).
func cmdEdit(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: passtore edit <id|título>")
	}
	v, err := openVault()
	if err != nil {
		return err
	}
	defer v.Lock()

	e, err := findEntry(v, args[0])
	if err != nil {
		return err
	}
	fmt.Println("Enter para mantener el valor actual.")
	e.Title = editLine("Título", e.Title)
	e.Username = editLine("Usuario", e.Username)
	if pw, _ := readSecret("Contraseña nueva (enter para mantener): "); len(pw) > 0 {
		e.Password = string(pw)
		zeroBytes(pw)
	}
	e.URL = editLine("URL", e.URL)
	e.Notes = editLine("Notas", e.Notes)
	if line, _ := readLine(fmt.Sprintf("Tags [%s]: ", strings.Join(e.Tags, ", "))); strings.TrimSpace(line) != "" {
		e.Tags = splitTags(line)
	}
	otpState := "sin OTP"
	if e.OTPAuth != "" {
		otpState = "OTP configurado"
	}
	if line, _ := readLine(fmt.Sprintf("OTP [%s] (otpauth/secreto, enter mantiene, '-' quita): ", otpState)); strings.TrimSpace(line) != "" {
		if strings.TrimSpace(line) == "-" {
			e.OTPAuth = ""
		} else {
			e.OTPAuth = normalizeOTP(line)
		}
	}

	if err := v.Update(e); err != nil {
		return err
	}
	if err := v.Save(); err != nil {
		return err
	}
	fmt.Println("✔ Entrada actualizada.")
	return nil
}

// cmdRemove elimina una entrada tras confirmación.
func cmdRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: passtore rm <id|título>")
	}
	v, err := openVault()
	if err != nil {
		return err
	}
	defer v.Lock()

	e, err := findEntry(v, args[0])
	if err != nil {
		return err
	}
	if !confirm(fmt.Sprintf("¿Borrar la entrada %q?", e.Title)) {
		return fmt.Errorf("cancelado")
	}
	if err := v.Delete(e.ID); err != nil {
		return err
	}
	if err := v.Save(); err != nil {
		return err
	}
	fmt.Println("✔ Entrada borrada.")
	return nil
}

// cmdChangePassword cambia la contraseña maestra (rekey).
func cmdChangePassword(_ []string) error {
	v, err := openVault()
	if err != nil {
		return err
	}
	defer v.Lock()

	newMaster, err := promptNewMaster()
	if err != nil {
		return err
	}
	defer zeroBytes(newMaster)
	if err := v.Rekey(newMaster); err != nil {
		return err
	}
	fmt.Println("✔ Contraseña maestra cambiada. El vault se re-cifró con la nueva.")
	return nil
}

// ---- helpers ----

// findEntry localiza una entrada por ID exacto, prefijo de ID, título exacto
// (case-insensitive) o coincidencia parcial de título. Si hay ambigüedad,
// lista los candidatos y devuelve error.
func findEntry(v *vault.Vault, query string) (vault.Entry, error) {
	entries, err := v.List()
	if err != nil {
		return vault.Entry{}, err
	}
	q := strings.ToLower(strings.TrimSpace(query))

	var matches []vault.Entry
	for _, e := range entries {
		if e.ID == query || strings.EqualFold(e.Title, query) {
			return e, nil // coincidencia exacta gana
		}
		if strings.HasPrefix(strings.ToLower(e.ID), q) || strings.Contains(strings.ToLower(e.Title), q) {
			matches = append(matches, e)
		}
	}
	switch len(matches) {
	case 0:
		return vault.Entry{}, fmt.Errorf("no encontré ninguna entrada para %q", query)
	case 1:
		return matches[0], nil
	default:
		fmt.Fprintln(os.Stderr, "Varias entradas coinciden; sé más específico:")
		printEntries(matches)
		return vault.Entry{}, fmt.Errorf("%d coincidencias para %q", len(matches), query)
	}
}

func printEntries(entries []vault.Entry) {
	if len(entries) == 0 {
		fmt.Println("(sin entradas)")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TÍTULO\tUSUARIO\tTIPO\tTAGS\tID")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			e.Title, e.Username, e.Type, strings.Join(e.Tags, ","), shortID(e.ID))
	}
	_ = w.Flush()
}

func filterByTag(entries []vault.Entry, tag string) []vault.Entry {
	if tag == "" {
		return entries
	}
	var out []vault.Entry
	for _, e := range entries {
		for _, t := range e.Tags {
			if strings.EqualFold(t, tag) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

func parseGetFlags(args []string) (show, copyPw bool, query string) {
	for _, a := range args {
		switch a {
		case "--show":
			show = true
		case "--copy":
			copyPw = true
		default:
			if query == "" {
				query = a
			} else {
				query += " " + a
			}
		}
	}
	return
}

func editLine(label, current string) string {
	line, _ := readLine(fmt.Sprintf("%s [%s]: ", label, current))
	if strings.TrimSpace(line) == "" {
		return current
	}
	return line
}

// normalizeOTP valida la entrada de OTP y la devuelve como URI otpauth://.
// Acepta un otpauth:// completo o un secreto base32 suelto. Si es inválida,
// avisa y devuelve "" (no se guarda OTP).
func normalizeOTP(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return ""
	}
	var uri string
	if strings.HasPrefix(strings.ToLower(in), "otpauth://") {
		uri = in
	} else {
		clean := strings.ToUpper(strings.ReplaceAll(in, " ", ""))
		uri = "otpauth://totp/passtore?secret=" + clean
	}
	if _, err := otp.Parse(uri); err != nil {
		fmt.Fprintf(os.Stderr, "⚠  OTP ignorado (inválido): %v\n", err)
		return ""
	}
	return uri
}

func splitTags(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}
