package config

import (
	"flag"
	"fmt"
	"os"
	"time"

	"linfbbpages/internal/fbb"
)

type Config struct {
	FBBDir     string
	Listen     string
	FBBArch    fbb.ArchMode
	SessionTTL time.Duration
}

func Parse(args []string, getenv func(string) string) (Config, error) {
	defaults := Config{
		FBBDir:     envOr(getenv, "LINFBBPAGES_FBB_DIR", "/usr/local/var/ax25/fbb"),
		Listen:     envOr(getenv, "LINFBBPAGES_LISTEN", ":8080"),
		FBBArch:    fbb.ArchMode(envOr(getenv, "LINFBBPAGES_FBB_ARCH", string(fbb.ArchAuto))),
		SessionTTL: 8 * time.Hour,
	}
	if value := getenv("LINFBBPAGES_SESSION_TTL"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("invalid LINFBBPAGES_SESSION_TTL: %w", err)
		}
		defaults.SessionTTL = duration
	}

	flags := flag.NewFlagSet("linfbbpages", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	fbbDir := flags.String("fbb-dir", defaults.FBBDir, "directorio de datos de FBB")
	listen := flags.String("listen", defaults.Listen, "dirección HTTP de escucha")
	arch := flags.String("fbb-arch", string(defaults.FBBArch), "layout binario FBB: auto, 32 o 64")
	ttl := flags.Duration("session-ttl", defaults.SessionTTL, "duración de las sesiones")
	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf("argumentos no reconocidos: %v", flags.Args())
	}
	if *fbbDir == "" {
		return Config{}, fmt.Errorf("--fbb-dir no puede estar vacío")
	}
	if *listen == "" {
		return Config{}, fmt.Errorf("--listen no puede estar vacío")
	}
	if *ttl <= 0 {
		return Config{}, fmt.Errorf("--session-ttl debe ser positivo")
	}
	mode := fbb.ArchMode(*arch)
	if mode != fbb.ArchAuto && mode != fbb.Arch32Mode && mode != fbb.Arch64Mode {
		return Config{}, fmt.Errorf("--fbb-arch inválido %q (usar auto, 32 o 64)", *arch)
	}
	return Config{FBBDir: *fbbDir, Listen: *listen, FBBArch: mode, SessionTTL: *ttl}, nil
}

func envOr(getenv func(string) string, key, fallback string) string {
	if value := getenv(key); value != "" {
		return value
	}
	return fallback
}
