// Command testenv initialises a complete test environment for the NTRIP Caster.
//
// It generates config_test.yaml and populates a fresh SQLite database with
// mountpoints, users (admin / base / rover), and user-mountpoint bindings so
// that simbase and simrover can connect without any manual setup.
//
// Usage:
//
//	go run ./cmd/testenv                           # 1 mountpoint, auth off
//	go run ./cmd/testenv -mounts 5 -auth           # 5 mountpoints, auth on
//	go run ./cmd/testenv -mounts 5 -auth -prefix MP # custom prefix → MP_0 … MP_4
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"ntrip-caster/internal/account"
	"ntrip-caster/internal/database"
)

const (
	configFile = "config_test.yaml"
	dbFile     = "caster_test.db"
	baseUser   = "base"
	roverUser  = "rover1"
	password   = "test"
)

func main() {
	numMounts := flag.Int("mounts", 1, "number of mountpoints to create")
	prefix := flag.String("prefix", "BENCH", "mountpoint name prefix (e.g. BENCH → BENCH_0)")
	authEnabled := flag.Bool("auth", false, "enable NTRIP authentication")
	maxClients := flag.Int("max-clients", 10000, "max_clients limit")
	maxConnPerIP := flag.Int("max-conn-per-ip", 12000, "max_conn_per_ip limit")
	writeQueue := flag.Int("write-queue", 128, "default write_queue per mountpoint")
	writeTimeout := flag.String("write-timeout", "5s", "default write_timeout per mountpoint")
	flag.Parse()

	// ── 1. Generate config_test.yaml ──
	cfg := generateConfig(*authEnabled, *maxClients, *maxConnPerIP, *writeQueue, *writeTimeout)
	if err := os.WriteFile(configFile, []byte(cfg), 0644); err != nil {
		log.Fatalf("write %s: %v", configFile, err)
	}
	log.Printf("✓ config written to %s", configFile)

	// ── 2. Fresh database ──
	_ = os.Remove(dbFile)
	_ = os.Remove(dbFile + "-wal")
	_ = os.Remove(dbFile + "-shm")

	db, err := database.Open(dbFile)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	svc := account.NewService(db)

	// ── 3. Users ──
	admin, err := svc.CreateUser("admin", "admin", account.RoleAdmin)
	if err != nil {
		log.Fatalf("create admin: %v", err)
	}
	_ = admin

	baseU, err := svc.CreateUser(baseUser, password, account.RoleBase)
	if err != nil {
		log.Fatalf("create base user: %v", err)
	}

	roverU, err := svc.CreateUser(roverUser, password, account.RoleRover)
	if err != nil {
		log.Fatalf("create rover user: %v", err)
	}

	log.Printf("✓ users: admin/admin, %s/%s (base), %s/%s (rover)", baseUser, password, roverUser, password)

	// ── 4. Mountpoints + Bindings ──
	mountNames := make([]string, *numMounts)
	for i := 0; i < *numMounts; i++ {
		name := fmt.Sprintf("%s_%d", *prefix, i)
		if *numMounts == 1 {
			name = *prefix
		}
		mountNames[i] = name

		mp, err := svc.CreateMountPointRow(name, fmt.Sprintf("test mountpoint %d", i), "RTCM3")
		if err != nil {
			log.Fatalf("create mountpoint %s: %v", name, err)
		}

		if err := svc.AddBinding(baseU.ID, mp.ID); err != nil {
			log.Fatalf("bind base→%s: %v", name, err)
		}
		if err := svc.AddBinding(roverU.ID, mp.ID); err != nil {
			log.Fatalf("bind rover→%s: %v", name, err)
		}
	}

	log.Printf("✓ mountpoints: %s", strings.Join(mountNames, ", "))
	log.Printf("✓ bindings: %s → all mounts (publish), %s → all mounts (subscribe)", baseUser, roverUser)

	// ── 5. Summary ──
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println("  Test environment ready!")
	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Config:     %s\n", configFile)
	fmt.Printf("  Database:   %s\n", dbFile)
	fmt.Printf("  Auth:       %v\n", *authEnabled)
	fmt.Printf("  Mounts:     %s\n", strings.Join(mountNames, ", "))
	fmt.Printf("  Base user:  %s / %s\n", baseUser, password)
	fmt.Printf("  Rover user: %s / %s\n", roverUser, password)
	fmt.Println()
	fmt.Println("  Quick start:")
	fmt.Println()
	fmt.Println("    make test-caster")
	fmt.Println()

	if *numMounts == 1 {
		if *authEnabled {
			fmt.Printf("    go run ./cmd/simbase  -mount %s -user %s -pass %s\n", mountNames[0], baseUser, password)
			fmt.Printf("    go run ./cmd/simrover -mount %s -user %s -pass %s\n", mountNames[0], roverUser, password)
		} else {
			fmt.Printf("    go run ./cmd/simbase  -mount %s\n", mountNames[0])
			fmt.Printf("    go run ./cmd/simrover -mount %s\n", mountNames[0])
		}
	} else {
		mountsArg := strings.Join(mountNames, ",")
		if *authEnabled {
			fmt.Printf("    go run ./cmd/simbase  -count %d -mount-prefix %s -user %s -pass %s\n", *numMounts, *prefix, baseUser, password)
			fmt.Printf("    go run ./cmd/simrover -mounts %s -count 5000 -ramp 2ms -user %s -pass %s\n", mountsArg, roverUser, password)
		} else {
			fmt.Printf("    go run ./cmd/simbase  -count %d -mount-prefix %s\n", *numMounts, *prefix)
			fmt.Printf("    go run ./cmd/simrover -mounts %s -count 5000 -ramp 2ms\n", mountsArg)
		}
	}
	fmt.Println()
}

func generateConfig(auth bool, maxClients, maxConnPerIP, writeQueue int, writeTimeout string) string {
	authBlock := `auth:
  enabled: false`
	if auth {
		authBlock = `auth:
  enabled: true
  admin_mode: session
  ntrip_source_auth: user_binding
  ntrip_rover_auth: basic`
	}

	return fmt.Sprintf(`server:
  listen: ":2101"
  admin_listen: ":8080"

%s

database:
  type: sqlite
  path: %s

limits:
  max_clients: %d
  max_conn_per_ip: %d

mountpoint_defaults:
  write_queue: %d
  write_timeout: %s
`, authBlock, dbFile, maxClients, maxConnPerIP, writeQueue, writeTimeout)
}
