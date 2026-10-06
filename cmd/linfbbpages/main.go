package main

import (
	"log"
	"net/http"
	"os"

	"linfbbpages/internal/api"
	"linfbbpages/internal/config"
	"linfbbpages/internal/fbb"
	"linfbbpages/web"
)

func main() {
	cfg, err := config.Parse(os.Args[1:], os.Getenv)
	if err != nil {
		log.Fatalf("configuración: %v", err)
	}
	store, err := fbb.NewStore(cfg.FBBDir, cfg.FBBArch)
	if err != nil {
		log.Fatalf("FBB: %v", err)
	}
	logger := log.New(os.Stderr, "linfbbpages: ", log.LstdFlags)
	logger.Printf("layout FBB detectado: %d bits", store.Layout().Arch)
	handler := api.NewServer(store, cfg.SessionTTL, web.Handler(), logger)
	logger.Printf("escuchando en %s", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, handler); err != nil {
		logger.Fatalf("servidor: %v", err)
	}
}
