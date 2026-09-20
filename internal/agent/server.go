package agent

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/wllamasr/passtore/internal/otp"
	"github.com/wllamasr/passtore/internal/pwgen"
	"github.com/wllamasr/passtore/internal/vault"
)

// Server es la API HTTP del agente, servida solo en loopback.
type Server struct {
	sess  *Session
	token string
	mux   *http.ServeMux
}

// NewServer construye el servidor con su enrutado.
func NewServer(sess *Session, token string) *Server {
	s := &Server{sess: sess, token: token, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler devuelve el http.Handler con los middlewares aplicados.
func (s *Server) Handler() http.Handler {
	return s.hostGuard(s.authGuard(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /status", s.handleStatus)
	s.mux.HandleFunc("POST /unlock", s.handleUnlock)
	s.mux.HandleFunc("POST /lock", s.handleLock)
	s.mux.HandleFunc("GET /entries", s.handleList)
	s.mux.HandleFunc("POST /entries", s.handleAdd)
	s.mux.HandleFunc("GET /entries/{id}", s.handleGet)
	s.mux.HandleFunc("PUT /entries/{id}", s.handleUpdate)
	s.mux.HandleFunc("DELETE /entries/{id}", s.handleDelete)
	s.mux.HandleFunc("GET /entries/{id}/otp", s.handleOTP)
	s.mux.HandleFunc("GET /search", s.handleSearch)
	s.mux.HandleFunc("POST /generate", s.handleGenerate)
	s.mux.HandleFunc("GET /capabilities", s.handleCapabilities)
}

// ---- middlewares ----

// hostGuard rechaza peticiones cuyo Host no sea loopback (defensa ante DNS rebinding).
func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			writeErr(w, http.StatusForbidden, "host no permitido")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authGuard exige el token de sesión en Authorization: Bearer o X-Passtore-Token.
func (s *Server) authGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-Passtore-Token")
		if tok == "" {
			if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
				tok = strings.TrimPrefix(a, "Bearer ")
			}
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
			writeErr(w, http.StatusUnauthorized, "token inválido")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- handlers ----

type statusResp struct {
	Locked    bool   `json:"locked"`
	VaultPath string `json:"vault_path"`
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusResp{Locked: s.sess.Locked(), VaultPath: s.sess.Path()})
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Master string `json:"master"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	master := []byte(body.Master)
	defer zero(master)
	if err := s.sess.Unlock(master); err != nil {
		if errors.Is(err, vault.ErrWrongPassword) {
			writeErr(w, http.StatusUnauthorized, "contraseña maestra incorrecta")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, statusResp{Locked: false, VaultPath: s.sess.Path()})
}

func (s *Server) handleLock(w http.ResponseWriter, _ *http.Request) {
	s.sess.Lock()
	writeJSON(w, http.StatusOK, statusResp{Locked: true, VaultPath: s.sess.Path()})
}

// entryListItem es la vista de una entrada SIN la contraseña (para listados).
type entryListItem struct {
	ID       string          `json:"id"`
	Type     vault.EntryType `json:"type"`
	Title    string          `json:"title"`
	Username string          `json:"username,omitempty"`
	URL      string          `json:"url,omitempty"`
	Tags     []string        `json:"tags,omitempty"`
	HasOTP   bool            `json:"has_otp,omitempty"`
}

func toListItem(e vault.Entry) entryListItem {
	return entryListItem{
		ID: e.ID, Type: e.Type, Title: e.Title, Username: e.Username,
		URL: e.URL, Tags: e.Tags, HasOTP: e.OTPAuth != "",
	}
}

func (s *Server) handleList(w http.ResponseWriter, _ *http.Request) {
	s.serveList(w, func(v *vault.Vault) ([]vault.Entry, error) { return v.List() })
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	s.serveList(w, func(v *vault.Vault) ([]vault.Entry, error) { return v.Search(q) })
}

func (s *Server) serveList(w http.ResponseWriter, list func(*vault.Vault) ([]vault.Entry, error)) {
	var items []entryListItem
	err := s.sess.withVault(func(v *vault.Vault) error {
		entries, err := list(v)
		if err != nil {
			return err
		}
		items = make([]entryListItem, 0, len(entries))
		for _, e := range entries {
			items = append(items, toListItem(e))
		}
		return nil
	})
	if handled := s.handleVaultErr(w, err); handled {
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var out vault.Entry
	err := s.sess.withVault(func(v *vault.Vault) error {
		e, err := v.Get(id)
		out = e
		return err
	})
	if handled := s.handleVaultErr(w, err); handled {
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	var e vault.Entry
	if err := decode(r, &e); err != nil {
		writeErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	var id string
	err := s.sess.withVault(func(v *vault.Vault) error {
		var e2 error
		id, e2 = v.Add(e)
		if e2 != nil {
			return e2
		}
		return v.Save()
	})
	if handled := s.handleVaultErr(w, err); handled {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var e vault.Entry
	if err := decode(r, &e); err != nil {
		writeErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	e.ID = r.PathValue("id")
	err := s.sess.withVault(func(v *vault.Vault) error {
		if err := v.Update(e); err != nil {
			return err
		}
		return v.Save()
	})
	if handled := s.handleVaultErr(w, err); handled {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": e.ID})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.sess.withVault(func(v *vault.Vault) error {
		if err := v.Delete(id); err != nil {
			return err
		}
		return v.Save()
	})
	if handled := s.handleVaultErr(w, err); handled {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type otpResp struct {
	Code      string `json:"code"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	ExpiresIn int    `json:"expires_in"`
}

// handleOTP calcula el código TOTP de una entrada. El secreto no sale del agente.
func (s *Server) handleOTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var out otpResp
	err := s.sess.withVault(func(v *vault.Vault) error {
		e, err := v.Get(id)
		if err != nil {
			return err
		}
		if e.OTPAuth == "" {
			return vault.ErrNotFound // la entrada no tiene OTP configurado
		}
		cfg, err := otp.Parse(e.OTPAuth)
		if err != nil {
			return err
		}
		code, remaining, err := otp.Now(cfg)
		if err != nil {
			return err
		}
		out = otpResp{Code: code, Digits: cfg.Digits, Period: cfg.Period, ExpiresIn: remaining}
		return nil
	})
	if handled := s.handleVaultErr(w, err); handled {
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCapabilities publica flags de configuración a nivel de máquina (p.ej. si
// la webcam está permitida). No requiere que el vault esté desbloqueado.
func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	cfg, _ := vault.LoadConfig()
	writeJSON(w, http.StatusOK, map[string]bool{"otp_webcam_enabled": cfg.WebcamEnabled()})
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	opts := pwgen.DefaultOptions()
	_ = decode(r, &opts) // cuerpo opcional
	pw, err := pwgen.Generate(opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": pw})
}

// handleVaultErr traduce errores comunes a códigos HTTP. Devuelve true si escribió respuesta.
func (s *Server) handleVaultErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrLocked):
		writeErr(w, http.StatusLocked, "vault bloqueado")
	case errors.Is(err, vault.ErrNotFound):
		writeErr(w, http.StatusNotFound, "entrada no encontrada")
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
	return true
}

// ---- helpers ----

func decode(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("sin cuerpo")
	}
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
