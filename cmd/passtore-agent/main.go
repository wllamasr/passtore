// Command passtore-agent es el daemon local: mantiene el vault desbloqueado en
// memoria y expone una API HTTP solo en 127.0.0.1, autenticada con token y con
// auto-lock por inactividad. Los clientes (Electron, extensión) lo descubren vía
// el archivo agent.json (puerto + token) en el directorio de config.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wllamasr/passtore/internal/agent"
	"github.com/wllamasr/passtore/internal/vault"
)

func main() {
	var (
		flagVault   = flag.String("vault", "", "ruta del vault (por defecto: config/env/default)")
		flagPort    = flag.Int("port", 0, "puerto en 127.0.0.1 (0 = asignado por el SO)")
		flagTimeout = flag.Int("timeout", 0, "minutos de inactividad para auto-lock (0 = usar config, y si no 15)")
	)
	flag.Parse()

	if err := run(*flagVault, *flagPort, *flagTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(vaultFlag string, port, timeoutMin int) error {
	path, err := vault.ResolveVaultPath(vaultFlag)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("no hay vault en %q. Créalo con 'passtore init'", path)
	}

	// Resolver timeout: flag > config > 15 min por defecto.
	if timeoutMin == 0 {
		if cfg, err := vault.LoadConfig(); err == nil && cfg.AutoLockMinutes > 0 {
			timeoutMin = cfg.AutoLockMinutes
		} else {
			timeoutMin = 15
		}
	}
	timeout := time.Duration(timeoutMin) * time.Minute

	sess := agent.NewSession(path, timeout)

	token, err := agent.NewToken()
	if err != nil {
		return err
	}
	srv := agent.NewServer(sess, token)

	// Escuchar SOLO en loopback.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("no pude escuchar en loopback: %w", err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port

	runtimePath, err := agent.WriteRuntime(agent.RuntimeInfo{Port: actualPort, Token: token, PID: os.Getpid()})
	if err != nil {
		_ = ln.Close()
		return err
	}
	defer agent.RemoveRuntime()

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Apagado limpio ante Ctrl-C / SIGTERM.
	idleClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		fmt.Fprintln(os.Stderr, "\napagando agente…")
		sess.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		close(idleClosed)
	}()

	fmt.Printf("passtore-agent escuchando en http://127.0.0.1:%d\n", actualPort)
	fmt.Printf("vault: %s\n", path)
	fmt.Printf("auto-lock: %d min · descubrimiento: %s\n", timeoutMin, runtimePath)
	fmt.Println("El vault arranca BLOQUEADO; usa POST /unlock para abrirlo.")

	if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	<-idleClosed
	return nil
}
