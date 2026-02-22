package benchutil

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGInstance represents a PostgreSQL connection, optionally backed by a podman container.
type PGInstance struct {
	Pool        *pgxpool.Pool
	DSN         string
	containerID string // empty if using external PG
}

// StartPG connects to PostgreSQL. If BENCH_PG_DSN is set, uses that directly.
// Otherwise spins up a podman postgres:16-alpine container on a random free port.
func StartPG(ctx context.Context) (*PGInstance, error) {
	if dsn := os.Getenv("BENCH_PG_DSN"); dsn != "" {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("connect to BENCH_PG_DSN: %w", err)
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("ping BENCH_PG_DSN: %w", err)
		}
		return &PGInstance{Pool: pool, DSN: dsn}, nil
	}

	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("find free port: %w", err)
	}

	password := fmt.Sprintf("bench%d", rand.Intn(100000))
	dsn := fmt.Sprintf("postgres://bench:%s@localhost:%d/bench?sslmode=disable", password, port)

	out, err := exec.CommandContext(ctx, "podman", "run", "--rm", "-d",
		"-e", "POSTGRES_USER=bench",
		"-e", "POSTGRES_PASSWORD="+password,
		"-e", "POSTGRES_DB=bench",
		"-p", fmt.Sprintf("%d:5432", port),
		"docker.io/library/postgres:16-alpine",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("podman run: %w", err)
	}

	containerID := string(out[:len(out)-1]) // trim newline

	// Wait for PG readiness.
	var pool *pgxpool.Pool
	for i := range 30 {
		_ = i
		time.Sleep(500 * time.Millisecond)
		pool, err = pgxpool.New(ctx, dsn)
		if err != nil {
			continue
		}
		if err = pool.Ping(ctx); err == nil {
			break
		}
		pool.Close()
		pool = nil
	}
	if pool == nil {
		// Cleanup container on failure.
		_ = exec.CommandContext(ctx, "podman", "stop", containerID).Run()
		return nil, fmt.Errorf("postgres not ready after 15s: %w", err)
	}

	return &PGInstance{Pool: pool, DSN: dsn, containerID: containerID}, nil
}

// Stop closes the pool and stops the podman container (no-op if external PG).
func (pg *PGInstance) Stop(ctx context.Context) {
	if pg.Pool != nil {
		pg.Pool.Close()
	}
	if pg.containerID != "" {
		_ = exec.CommandContext(ctx, "podman", "stop", pg.containerID).Run()
	}
}

// IsContainer reports whether this instance is backed by a managed podman container.
func (pg *PGInstance) IsContainer() bool {
	return pg.containerID != ""
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}
