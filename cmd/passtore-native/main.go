// Command passtore-native es el host de native messaging del navegador.
// Lee mensajes JSON con prefijo de longitud (protocolo de Chrome/Edge/Firefox)
// desde stdin, los traduce a llamadas a la API del agente local y devuelve la
// respuesta por stdout. IMPORTANTE: stdout es el canal de mensajería; nada más
// debe escribirse ahí (los logs van a stderr).
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/wllamasr/passtore/internal/agent"
	"github.com/wllamasr/passtore/internal/vault"
)

type request struct {
	ID      int             `json:"id"`
	Type    string          `json:"type"`
	Master  string          `json:"master,omitempty"`
	Query   string          `json:"query,omitempty"`
	EntryID string          `json:"entryId,omitempty"`
	Entry   json.RawMessage `json:"entry,omitempty"`
	Opts    map[string]any  `json:"opts,omitempty"`
}

type response struct {
	ID     int    `json:"id"`
	OK     bool   `json:"ok"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
	Status int    `json:"status,omitempty"`
}

var client *agent.Client

func main() {
	for {
		req, err := readMessage(os.Stdin)
		if err == io.EOF {
			return
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "passtore-native: error leyendo mensaje:", err)
			return
		}
		resp := handle(req)
		resp.ID = req.ID
		if err := writeMessage(os.Stdout, resp); err != nil {
			fmt.Fprintln(os.Stderr, "passtore-native: error escribiendo respuesta:", err)
			return
		}
	}
}

// ensureClient conecta con el agente (arrancándolo si hace falta).
func ensureClient() error {
	if client != nil {
		if _, err := client.Status(); err == nil {
			return nil
		}
		client = nil
	}
	c, err := agent.ConnectOrStart("")
	if err != nil {
		return err
	}
	client = c
	return nil
}

func handle(req request) response {
	if err := ensureClient(); err != nil {
		return response{Error: "no pude conectar con el agente: " + err.Error()}
	}

	switch req.Type {
	case "status":
		return dataOrErr(client.Status())
	case "unlock":
		return okOrErr(client.Unlock(req.Master))
	case "lock":
		return okOrErr(client.Lock())
	case "list":
		return dataOrErr(client.List())
	case "search":
		return dataOrErr(client.Search(req.Query))
	case "get":
		return dataOrErr(client.Get(req.EntryID))
	case "otp":
		return dataOrErr(client.OTP(req.EntryID))
	case "capabilities":
		return dataOrErr(client.Capabilities())
	case "add":
		var e vault.Entry
		if err := json.Unmarshal(req.Entry, &e); err != nil {
			return response{Error: "entrada inválida"}
		}
		id, err := client.Add(e)
		if err != nil {
			return errResp(err)
		}
		return response{OK: true, Data: map[string]string{"id": id}}
	case "update":
		var e vault.Entry
		if err := json.Unmarshal(req.Entry, &e); err != nil {
			return response{Error: "entrada inválida"}
		}
		return okOrErr(client.Update(req.EntryID, e))
	case "remove":
		return okOrErr(client.Delete(req.EntryID))
	case "generate":
		pw, err := client.Generate(req.Opts)
		if err != nil {
			return errResp(err)
		}
		return response{OK: true, Data: map[string]string{"password": pw}}
	default:
		return response{Error: "tipo de mensaje desconocido: " + req.Type}
	}
}

func dataOrErr[T any](data T, err error) response {
	if err != nil {
		return errResp(err)
	}
	return response{OK: true, Data: data}
}

func okOrErr(err error) response {
	if err != nil {
		return errResp(err)
	}
	return response{OK: true}
}

func errResp(err error) response {
	r := response{Error: err.Error()}
	if apiErr, ok := err.(*agent.APIError); ok {
		r.Status = apiErr.Status
		r.Error = apiErr.Message
	}
	return r
}

// ---- protocolo de native messaging ----

func readMessage(r io.Reader) (request, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return request{}, err
	}
	n := binary.LittleEndian.Uint32(lenBuf[:])
	if n == 0 || n > 4*1024*1024 {
		return request{}, fmt.Errorf("longitud de mensaje inválida: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return request{}, err
	}
	var req request
	if err := json.Unmarshal(buf, &req); err != nil {
		return request{}, err
	}
	return req, nil
}

func writeMessage(w io.Writer, resp response) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}
