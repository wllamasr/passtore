package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/wllamasr/passtore/internal/vault"
)

// Client es un cliente HTTP del agente local (usado por el host de native
// messaging y por otros consumidores en Go).
type Client struct {
	info RuntimeInfo
	http *http.Client
}

// Connect lee agent.json y comprueba que el agente responde.
func Connect() (*Client, error) {
	info, err := LoadRuntime()
	if err != nil {
		return nil, fmt.Errorf("no encuentro el agente (agent.json): %w", err)
	}
	c := &Client{info: info, http: &http.Client{Timeout: 10 * time.Second}}
	if _, err := c.Status(); err != nil {
		return nil, err
	}
	return c, nil
}

// ConnectOrStart intenta conectar; si falla y se indica binPath (o PATH), arranca
// el agente y reintenta. binPath vacío usa $PASSTORE_AGENT_BIN o "passtore-agent".
func ConnectOrStart(binPath string) (*Client, error) {
	if c, err := Connect(); err == nil {
		return c, nil
	}
	if binPath == "" {
		binPath = os.Getenv("PASSTORE_AGENT_BIN")
	}
	if binPath == "" {
		binPath = "passtore-agent"
	}
	cmd := exec.Command(binPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("no pude arrancar el agente %q: %w", binPath, err)
	}
	_ = cmd.Process.Release()

	for i := 0; i < 40; i++ {
		time.Sleep(150 * time.Millisecond)
		if c, err := Connect(); err == nil {
			return c, nil
		}
	}
	return nil, fmt.Errorf("el agente no respondió tras arrancarlo")
}

// APIError representa un error de la API con su status HTTP.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message) }

func (c *Client) request(method, endpoint string, body any) (json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", c.info.Port, endpoint), rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Passtore-Token", c.info.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		msg := resp.Status
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return nil, &APIError{Status: resp.StatusCode, Message: msg}
	}
	return data, nil
}

// Status devuelve el estado del vault.
func (c *Client) Status() (map[string]any, error) {
	data, err := c.request("GET", "/status", nil)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(data, &m)
}

func (c *Client) Unlock(master string) error {
	_, err := c.request("POST", "/unlock", map[string]string{"master": master})
	return err
}

func (c *Client) Lock() error {
	_, err := c.request("POST", "/lock", nil)
	return err
}

// List y Search devuelven el JSON crudo del agente (vista de listado, que
// incluye has_otp y NO incluye contraseñas). Se reenvía tal cual al cliente
// para no perder campos que no existen en vault.Entry.
func (c *Client) List() (json.RawMessage, error) { return c.request("GET", "/entries", nil) }

func (c *Client) Search(q string) (json.RawMessage, error) {
	return c.request("GET", "/search?q="+urlQuery(q), nil)
}

func (c *Client) Get(id string) (vault.Entry, error) {
	data, err := c.request("GET", "/entries/"+id, nil)
	if err != nil {
		return vault.Entry{}, err
	}
	var e vault.Entry
	return e, json.Unmarshal(data, &e)
}

func (c *Client) Add(e vault.Entry) (string, error) {
	data, err := c.request("POST", "/entries", e)
	if err != nil {
		return "", err
	}
	var r struct {
		ID string `json:"id"`
	}
	return r.ID, json.Unmarshal(data, &r)
}

func (c *Client) Update(id string, e vault.Entry) error {
	_, err := c.request("PUT", "/entries/"+id, e)
	return err
}

func (c *Client) Delete(id string) error {
	_, err := c.request("DELETE", "/entries/"+id, nil)
	return err
}

// OTP devuelve el código TOTP actual de una entrada (calculado por el agente).
func (c *Client) OTP(id string) (map[string]any, error) {
	data, err := c.request("GET", "/entries/"+id+"/otp", nil)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(data, &m)
}

// Capabilities devuelve los flags de configuración a nivel de máquina.
func (c *Client) Capabilities() (map[string]any, error) {
	data, err := c.request("GET", "/capabilities", nil)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(data, &m)
}

func (c *Client) Generate(opts map[string]any) (string, error) {
	data, err := c.request("POST", "/generate", opts)
	if err != nil {
		return "", err
	}
	var r struct {
		Password string `json:"password"`
	}
	return r.Password, json.Unmarshal(data, &r)
}

func urlQuery(s string) string {
	// Escapado mínimo suficiente para query values.
	repl := func(r rune) string {
		switch r {
		case ' ':
			return "%20"
		case '&':
			return "%26"
		case '?':
			return "%3F"
		case '#':
			return "%23"
		case '=':
			return "%3D"
		}
		return string(r)
	}
	out := ""
	for _, r := range s {
		out += repl(r)
	}
	return out
}
