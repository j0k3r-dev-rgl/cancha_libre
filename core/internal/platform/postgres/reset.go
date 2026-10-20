package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	identityschema "canchalibre/core/internal/modules/identity/adapters/outbound/postgres/schema"
	organizationschema "canchalibre/core/internal/modules/organization/adapters/outbound/postgres/schema"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

// ResetOptions configures the unguarded development database reset and seed.
type ResetOptions struct {
	DatabaseURL      string
	SeedTopologyPath string
	AccountsFilePath string
}

// ResetAndConnect drops and recreates the target database, applies every context's
// schema/*.sql in lexical order, loads the development seed, and returns a
// connected pgxpool.Pool.
func ResetAndConnect(ctx context.Context, opts ResetOptions) (*pgxpool.Pool, error) {
	if strings.TrimSpace(opts.DatabaseURL) == "" {
		return nil, errors.New("DatabaseURL is required")
	}

	poolCfg, err := pgxpool.ParseConfig(opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	targetDB := poolCfg.ConnConfig.Database
	if targetDB == "" {
		return nil, errors.New("DATABASE_URL must name a target database")
	}

	maintCfg := poolCfg.ConnConfig.Copy()
	maintCfg.Database = "postgres"

	maintConn, err := pgx.ConnectConfig(ctx, maintCfg)
	if err != nil {
		return nil, fmt.Errorf("connect maintenance database: %w", err)
	}
	defer func() { _ = maintConn.Close(ctx) }()

	quotedDB := pgx.Identifier{targetDB}.Sanitize()
	if _, err := maintConn.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quotedDB)); err != nil {
		return nil, fmt.Errorf("drop database %s: %w", targetDB, err)
	}
	if _, err := maintConn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", quotedDB)); err != nil {
		return nil, fmt.Errorf("create database %s: %w", targetDB, err)
	}
	_ = maintConn.Close(ctx)

	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool for %s: %w", targetDB, err)
	}

	if err := applySchemas(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	if opts.SeedTopologyPath != "" {
		if err := loadSeed(ctx, pool, opts.SeedTopologyPath, opts.AccountsFilePath); err != nil {
			pool.Close()
			return nil, fmt.Errorf("load seed: %w", err)
		}
	}

	return pool, nil
}

// Connect opens a connection pool to DatabaseURL without running the reset
// (used by CLI tools like provision-admin that mutate an existing database).
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolCfg.MaxConns = 5
	poolCfg.MinConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

func applySchemas(ctx context.Context, pool *pgxpool.Pool) error {
	for _, source := range []struct {
		name string
		fsys fs.FS
	}{
		{name: "identity", fsys: identityschema.Files},
		{name: "organization", fsys: organizationschema.Files},
	} {
		entries, err := fs.ReadDir(source.fsys, ".")
		if err != nil {
			return fmt.Errorf("read %s schema dir: %w", source.name, err)
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			sqlBytes, err := fs.ReadFile(source.fsys, name)
			if err != nil {
				return fmt.Errorf("read %s/%s: %w", source.name, name, err)
			}
			if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
				return fmt.Errorf("apply %s/%s: %w", source.name, name, err)
			}
		}
	}
	return nil
}

type seedTopology struct {
	Businesses []struct {
		Key      string `json:"key"`
		Name     string `json:"name"`
		Enabled  bool   `json:"enabled"`
		Branches []struct {
			Key     string `json:"key"`
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"branches"`
	} `json:"businesses"`
	Memberships []struct {
		Email    string  `json:"email"`
		Business string  `json:"business"`
		Branch   *string `json:"branch"`
		Role     string  `json:"role"`
	} `json:"memberships"`
	PlatformActors []struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"platformActors"`
	Customers []struct {
		Email string `json:"email"`
	} `json:"customers"`
}

type seedAccountCredential struct {
	Role     string `json:"role"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Deterministic UUIDv7 identifiers for the seeded entities so tests and verifiers
// can reference stable UUIDs across resets when needed.
var seededBusinessUUIDs = map[string]string{
	"B1": "019294f0-0000-7000-8000-000000000101",
	"B2": "019294f0-0000-7000-8000-000000000102",
}

var seededBranchUUIDs = map[string]string{
	"B1-CENTRAL": "019294f0-0000-7000-8000-000000000201",
	"B1-NORTE":   "019294f0-0000-7000-8000-000000000202",
	"B2-SUR":     "019294f0-0000-7000-8000-000000000203",
}

var seededAccountUUIDs = map[string]string{
	"admin@canchalibre.local":    "019294f0-0000-7000-8000-000000000001",
	"owner@canchalibre.local":    "019294f0-0000-7000-8000-000000000002",
	"employee@canchalibre.local": "019294f0-0000-7000-8000-000000000003",
	"player@canchalibre.local":   "019294f0-0000-7000-8000-000000000004",
	"soporte@canchalibre.local":  "019294f0-0000-7000-8000-000000000005",
}

func resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	alt := filepath.Join("core", path)
	if _, err := os.Stat(alt); err == nil {
		return alt
	}
	trimmed := strings.TrimPrefix(path, "../")
	if _, err := os.Stat(trimmed); err == nil {
		return trimmed
	}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 8; i++ {
			for _, candidate := range []string{
				filepath.Join(dir, path),
				filepath.Join(dir, "core", path),
				filepath.Join(dir, trimmed),
			} {
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return path
}

func loadSeed(ctx context.Context, pool *pgxpool.Pool, topologyPath, accountsPath string) error {
	topoFile := resolvePath(topologyPath)
	topoBytes, err := os.ReadFile(topoFile)
	if err != nil {
		return fmt.Errorf("read seed topology %s: %w", topoFile, err)
	}
	var topo seedTopology
	if err := json.Unmarshal(topoBytes, &topo); err != nil {
		return fmt.Errorf("parse seed topology: %w", err)
	}

	passwordsByEmail := map[string]string{}
	accFile := resolvePath(accountsPath)
	if _, err := os.Stat(accFile); err != nil {
		// Fall back to the committed .example template if the local accounts file is not created yet
		exampleFile := resolvePath(accountsPath + ".example")
		if _, errEx := os.Stat(exampleFile); errEx == nil {
			accFile = exampleFile
		}
	}
	if accBytes, err := os.ReadFile(accFile); err == nil {
		var creds []seedAccountCredential
		if err := json.Unmarshal(accBytes, &creds); err != nil {
			return fmt.Errorf("parse accounts file %s: %w", accFile, err)
		}
		for _, c := range creds {
			rawEmail := strings.TrimSpace(c.Email)
			if rawEmail == "" {
				rawEmail = strings.TrimSpace(c.Username)
			}
			email, err := security.NormalizeEmail(rawEmail)
			if err != nil {
				return fmt.Errorf("invalid email %q in accounts file: %w", rawEmail, err)
			}
			passwordsByEmail[email] = c.Password
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seed tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	businessIDs := map[string]string{}
	branchIDs := map[string]string{}

	for _, b := range topo.Businesses {
		bID, ok := seededBusinessUUIDs[b.Key]
		if !ok {
			bID, err = actor.NewUUIDv7()
			if err != nil {
				return err
			}
		}
		businessIDs[b.Key] = bID
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_organization.business (id, name, enabled) VALUES ($1, $2, $3)`,
			bID, b.Name, b.Enabled,
		); err != nil {
			return fmt.Errorf("insert business %s: %w", b.Key, err)
		}
		for _, br := range b.Branches {
			brID, ok := seededBranchUUIDs[br.Key]
			if !ok {
				brID, err = actor.NewUUIDv7()
				if err != nil {
					return err
				}
			}
			branchIDs[br.Key] = brID
			slug := strings.ToLower(strings.ReplaceAll(br.Name, " ", "-"))
			street := "Av. Libertador 1000"
			city := "Buenos Aires"
			state := "CABA"
			postal := "C1425"
			if _, err := tx.Exec(ctx,
				`INSERT INTO schema_organization.branch (
					id, business_id, name, slug, street_address, city, state_province, postal_code,
					approval_status, coordinates_confirmed, enabled
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'APPROVED', true, $9)`,
				brID, bID, br.Name, slug, street, city, state, postal, br.Enabled,
			); err != nil {
				return fmt.Errorf("insert branch %s: %w", br.Key, err)
			}
		}
	}

	accountIDs := map[string]string{}
	ensureAccount := func(rawEmail, kind string, platformRole *string) (string, error) {
		email, err := security.NormalizeEmail(rawEmail)
		if err != nil {
			return "", err
		}
		key := email + "|" + kind
		if id, ok := accountIDs[key]; ok {
			return id, nil
		}
		accID, ok := seededAccountUUIDs[email]
		if !ok {
			accID, err = actor.NewUUIDv7()
			if err != nil {
				return "", err
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_identity.account (id, email, kind, platform_role, status, display_name)
			 VALUES ($1, $2, $3, $4, 'ACTIVE', $5)`,
			accID, email, kind, platformRole, email,
		); err != nil {
			return "", fmt.Errorf("insert account %s: %w", email, err)
		}
		pw := passwordsByEmail[email]
		if pw == "" {
			pw = "dev-only-default-password-123"
		}
		pwHash, err := security.HashPassword(pw)
		if err != nil {
			return "", fmt.Errorf("hash seed password for %s: %w", email, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_identity.credential (account_id, password_hash) VALUES ($1, $2)`,
			accID, pwHash,
		); err != nil {
			return "", fmt.Errorf("insert credential for %s: %w", email, err)
		}
		accountIDs[key] = accID
		return accID, nil
	}

	for _, pa := range topo.PlatformActors {
		role := pa.Role
		if _, err := ensureAccount(pa.Email, string(actor.KindWork), &role); err != nil {
			return err
		}
	}

	for _, c := range topo.Customers {
		if _, err := ensureAccount(c.Email, string(actor.KindCustomer), nil); err != nil {
			return err
		}
	}

	for i, m := range topo.Memberships {
		accID, err := ensureAccount(m.Email, string(actor.KindWork), nil)
		if err != nil {
			return err
		}
		bID, ok := businessIDs[m.Business]
		if !ok {
			return fmt.Errorf("unknown business key %q in membership", m.Business)
		}
		var brID *string
		if m.Branch != nil && *m.Branch != "" {
			resolved, ok := branchIDs[*m.Branch]
			if !ok {
				return fmt.Errorf("unknown branch key %q in membership", *m.Branch)
			}
			brID = &resolved
		}
		memID := fmt.Sprintf("019294f0-0000-7000-8000-0000000003%02d", i+1)
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_organization.membership (id, account_id, business_id, branch_id, role)
			 VALUES ($1, $2, $3, $4, $5)`,
			memID, accID, bID, brID, m.Role,
		); err != nil {
			return fmt.Errorf("insert membership for %s: %w", m.Email, err)
		}
	}

	return tx.Commit(ctx)
}
