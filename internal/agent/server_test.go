package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/wllamasr/passtore/internal/vault"
)

const testToken = "test-token-123"

// setup crea un vault con KDF ligero, una sesión y un servidor httptest.
func setup(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.pstore")
	kdf, _ := vault.DefaultKDFParams()
	kdf.MemoryKiB = 8 * 1024
	kdf.Iterations = 1
	kdf.Parallelism = 1
	v, err := vault.Create(path, []byte("maestra"), vault.WithKDFParams(kdf))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	v.Lock()

	sess := NewSession(path, time.Minute)
	ts := httptest.NewServer(NewServer(sess, testToken).Handler())
	t.Cleanup(ts.Close)
	return ts, path
}

func do(t *testing.T, ts *httptest.Server, method, path, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ts.URL+path, rdr)
	if token != "" {
		req.Header.Set("X-Passtore-Token", token)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

func TestAuthRequired(t *testing.T) {
	ts, _ := setup(t)
	resp, _ := do(t, ts, "GET", "/status", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sin token esperaba 401, obtuve %d", resp.StatusCode)
	}
	resp, _ = do(t, ts, "GET", "/status", "token-malo", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token malo esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestOTPAndCapabilities(t *testing.T) {
	ts, _ := setup(t)
	do(t, ts, "POST", "/unlock", testToken, map[string]string{"master": "maestra"})

	// Añadir entrada con OTP (secreto conocido).
	_, data := do(t, ts, "POST", "/entries", testToken, map[string]any{
		"title":   "GitHub",
		"otpauth": "otpauth://totp/GitHub?secret=JBSWY3DPEHPK3PXP&issuer=GitHub",
	})
	var created map[string]string
	json.Unmarshal(data, &created)
	id := created["id"]

	// El listado marca has_otp y NO expone el secreto.
	_, listData := do(t, ts, "GET", "/entries", testToken, nil)
	if !bytes.Contains(listData, []byte(`"has_otp":true`)) {
		t.Fatalf("el listado debería marcar has_otp: %s", listData)
	}
	if bytes.Contains(listData, []byte("JBSWY3DPEHPK3PXP")) {
		t.Fatal("el listado NO debe exponer el secreto OTP")
	}

	// El código OTP se calcula en el agente.
	resp, otpData := do(t, ts, "GET", "/entries/"+id+"/otp", testToken, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("otp: %d", resp.StatusCode)
	}
	var otp struct {
		Code   string `json:"code"`
		Period int    `json:"period"`
	}
	json.Unmarshal(otpData, &otp)
	if len(otp.Code) != 6 || otp.Period != 30 {
		t.Fatalf("código OTP inesperado: %+v", otp)
	}

	// Capabilities: por defecto la webcam está habilitada.
	resp, capData := do(t, ts, "GET", "/capabilities", testToken, nil)
	if resp.StatusCode != 200 || !bytes.Contains(capData, []byte(`"otp_webcam_enabled":true`)) {
		t.Fatalf("capabilities inesperado (%d): %s", resp.StatusCode, capData)
	}
}

func TestUnlockAndCRUDFlow(t *testing.T) {
	ts, _ := setup(t)

	// status: bloqueado
	resp, data := do(t, ts, "GET", "/status", testToken, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var st statusResp
	json.Unmarshal(data, &st)
	if !st.Locked {
		t.Fatal("debería empezar bloqueado")
	}

	// operar bloqueado → 423
	resp, _ = do(t, ts, "GET", "/entries", testToken, nil)
	if resp.StatusCode != http.StatusLocked {
		t.Fatalf("bloqueado esperaba 423, obtuve %d", resp.StatusCode)
	}

	// unlock con contraseña mala → 401
	resp, _ = do(t, ts, "POST", "/unlock", testToken, map[string]string{"master": "mala"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unlock malo esperaba 401, obtuve %d", resp.StatusCode)
	}

	// unlock correcto
	resp, _ = do(t, ts, "POST", "/unlock", testToken, map[string]string{"master": "maestra"})
	if resp.StatusCode != 200 {
		t.Fatalf("unlock: %d", resp.StatusCode)
	}

	// add
	resp, data = do(t, ts, "POST", "/entries", testToken, vault.Entry{Title: "GitHub", Username: "wllamas", Password: "s3cr3t", URL: "https://github.com"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add: %d", resp.StatusCode)
	}
	var created map[string]string
	json.Unmarshal(data, &created)
	id := created["id"]
	if id == "" {
		t.Fatal("sin id en add")
	}

	// list: no debe incluir la contraseña
	resp, data = do(t, ts, "GET", "/entries", testToken, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list: %d", resp.StatusCode)
	}
	if bytes.Contains(data, []byte("s3cr3t")) {
		t.Fatal("el listado NO debe exponer la contraseña")
	}

	// get by id: sí incluye la contraseña
	resp, data = do(t, ts, "GET", "/entries/"+id, testToken, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get: %d", resp.StatusCode)
	}
	var got vault.Entry
	json.Unmarshal(data, &got)
	if got.Password != "s3cr3t" {
		t.Fatalf("get debería traer la contraseña, obtuve %q", got.Password)
	}

	// generate
	resp, data = do(t, ts, "POST", "/generate", testToken, map[string]int{"Length": 24})
	if resp.StatusCode != 200 {
		t.Fatalf("generate: %d", resp.StatusCode)
	}
	var gen map[string]string
	json.Unmarshal(data, &gen)
	if len(gen["password"]) != 24 {
		t.Fatalf("generate longitud esperada 24, obtuve %d", len(gen["password"]))
	}

	// delete
	resp, _ = do(t, ts, "DELETE", "/entries/"+id, testToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}

	// lock
	resp, _ = do(t, ts, "POST", "/lock", testToken, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("lock: %d", resp.StatusCode)
	}
	resp, _ = do(t, ts, "GET", "/entries", testToken, nil)
	if resp.StatusCode != http.StatusLocked {
		t.Fatalf("tras lock esperaba 423, obtuve %d", resp.StatusCode)
	}
}
