package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func productionMode() bool { return os.Getenv("MM_ENV") == "production" }
func dataDirectory() string {
	if directory := strings.TrimSpace(os.Getenv("MM_DATA_DIR")); directory != "" {
		return directory
	}
	return ".local"
}
func databasePath() string {
	if path := strings.TrimSpace(os.Getenv("MM_DB_PATH")); path != "" {
		return path
	}
	if productionMode() || os.Getenv("MM_DATA_DIR") != "" {
		return filepath.Join(dataDirectory(), "marvellous.db")
	}
	return ".local/preview.db"
}
func uploadDirectory() string {
	if directory := strings.TrimSpace(os.Getenv("MM_UPLOADS_DIR")); directory != "" {
		return directory
	}
	if os.Getenv("MM_DATA_DIR") != "" {
		return filepath.Join(dataDirectory(), "uploads")
	}
	return "uploads"
}
func secureCookiesEnabled() bool {
	value, explicit := os.LookupEnv("MM_SECURE_COOKIES")
	if explicit {
		return value == "true"
	}
	return productionMode()
}
func listenAddress() (string, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	provided := port != ""
	if !provided {
		port = "8080"
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("PORT must be between 1 and 65535")
	}
	host := strings.TrimSpace(os.Getenv("MM_HOST"))
	if host == "" {
		host = "127.0.0.1"
		if provided || productionMode() {
			host = "0.0.0.0"
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(number)), nil
}
func runHTTPServer(handler http.Handler) error {
	address, err := listenAddress()
	if err != nil {
		return err
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	shutdownDone := make(chan struct{})
	go func() {
		<-stop.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Println("Graceful shutdown timed out")
			server.Close()
		}
		close(shutdownDone)
	}()
	log.Printf("Marvellous Motors listening on %s", address)
	err = server.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-shutdownDone
	return nil
}
