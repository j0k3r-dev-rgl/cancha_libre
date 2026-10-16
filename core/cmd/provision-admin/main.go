// Command provision-admin is the private first-ADMIN provisioning command
// (BE-ACC-04). It is a server-side binary reachable only from the operating
// system shell, never over the network, and it never generates, stores or logs
// a password: it issues a single-use, expiring ADMIN invitation through the
// identity use case and prints the activation URL exactly once to stdout.
//
// Contract: docs/design/provision-admin.md §4.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	identitypostgres "canchalibre/core/internal/modules/identity/adapters/outbound/postgres"
	identitydomain "canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/modules/identity/core/ports"
	identityusecase "canchalibre/core/internal/modules/identity/core/usecase"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/config"
	platformpg "canchalibre/core/internal/platform/postgres"
	"canchalibre/core/internal/platform/security"
)

const (
	// Exit codes frozen by docs/design/provision-admin.md §4.
	exitOK      = 0
	exitRefused = 1
	exitUsage   = 2

	defaultTTL = 24 * time.Hour

	usageLine = "usage: provision-admin --email <email> [--ttl <duration>] [--supersede] [--dry-run]"
)

// errScopeBridgeUnavailable is returned by the nil scope bridge. The
// provisioning flow only issues ADMIN invitations, which have no business or
// branch scope, so the bridge is never exercised; it exists to satisfy the
// identity use-case wiring without importing another context's core.
var errScopeBridgeUnavailable = errors.New("identity scope bridge is not wired into provision-admin")

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command and returns its process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	flags := flag.NewFlagSet("provision-admin", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprintln(stderr, usageLine) }

	var (
		email     string
		ttl       time.Duration
		supersede bool
		dryRun    bool
	)
	flags.StringVar(&email, "email", "", "email address of the first ADMIN (required)")
	flags.DurationVar(&ttl, "ttl", defaultTTL, "invitation lifetime (default 24h)")
	flags.BoolVar(&supersede, "supersede", false, "replace an existing unexpired ADMIN invitation")
	flags.BoolVar(&dryRun, "dry-run", false, "validate and report without changing anything")

	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "provision-admin: unexpected argument %q\n%s\n", flags.Arg(0), usageLine)
		return exitUsage
	}

	// Argument validation happens before any configuration read or connection, so
	// an invalid invocation can never reach the database. The two rows of
	// docs/design/provision-admin.md §4 that cover this case split the outcome:
	// a missing required argument is a usage error (`2`), while a malformed email
	// is listed under *Refusals*, whose exit code is `1`.
	if strings.TrimSpace(email) == "" {
		fmt.Fprintf(stderr, "provision-admin: --email is required\n%s\n", usageLine)
		return exitUsage
	}
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		fmt.Fprintf(stderr, "provision-admin: refused: %q is not a valid email address\n", email)
		return exitRefused
	}
	if ttl <= 0 {
		fmt.Fprintf(stderr, "provision-admin: --ttl must be a positive duration\n%s\n", usageLine)
		return exitUsage
	}

	// DATABASE_URL and JWT_SECRET are mandatory with no silent default; their
	// absence is a usage failure, never a fallback connection string.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "provision-admin: %v\n%s\n", err, usageLine)
		return exitUsage
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The command mutates an existing database; it never runs the development
	// reset, which belongs to a local start of cmd/api.
	pool, err := platformpg.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("provision-admin: cannot connect to the database", "error", err.Error())
		fmt.Fprintf(stderr, "provision-admin: cannot connect to the database: %v\n", err)
		return exitRefused
	}
	defer pool.Close()

	service := identityusecase.NewService(
		identitypostgres.NewRepository(pool),
		nilScopeBridge{},
		discardEmailSender{},
		identityusecase.Config{
			JWTSecret:         []byte(cfg.JWTSecret),
			InvitationURLBase: cfg.InvitationURLBase,
		},
	)

	logger.Info("provision-admin: provisioning ADMIN invitation",
		"dry_run", dryRun, "supersede", supersede, "ttl", ttl.String())

	out, err := service.ProvisionAdmin(ctx, identityusecase.ProvisionAdminInput{
		Email:     normEmail,
		TTL:       ttl,
		Supersede: supersede,
		DryRun:    dryRun,
	})
	if err != nil {
		// Every refusal the use case returns is exit code `1` per §4's Refusals
		// row, including its defense-in-depth `ErrValidation` for a malformed
		// email. Invalid *invocation* is rejected above, before any I/O.
		fmt.Fprintf(stderr, "provision-admin: refused: %v\n", err)
		return exitRefused
	}

	if dryRun {
		expiresAt := out.ExpiresAt.UTC().Format(time.RFC3339)
		fmt.Fprintf(stdout, "dry-run: would issue a single-use ADMIN invitation for %s (expires %s)\n",
			normEmail, expiresAt)
		logger.Info("provision-admin: dry run completed", "expires_at", expiresAt)
		return exitOK
	}

	if out.ActivationURL == "" {
		fmt.Fprintln(stderr, "provision-admin: refused: the provisioning use case returned no activation URL")
		return exitRefused
	}

	logger.Info("provision-admin: invitation issued",
		"invitation_id", out.InvitationID,
		"expires_at", out.ExpiresAt.UTC().Format(time.RFC3339),
		"superseded", supersede)
	// The activation URL is printed exactly once, and only here. It is never
	// written to the structured logger and never to a file.
	fmt.Fprintf(stdout, "%s\n", out.ActivationURL)
	return exitOK
}

// nilScopeBridge satisfies ports.ScopePort for a command that only provisions a
// platform ADMIN. ADMIN has no business or branch, so no membership operation is
// reachable and every call fails loudly instead of fabricating scope.
type nilScopeBridge struct{}

var _ ports.ScopePort = nilScopeBridge{}

func (nilScopeBridge) ListAccountContexts(context.Context, string) ([]identitydomain.ContextOption, error) {
	return nil, errScopeBridgeUnavailable
}

func (nilScopeBridge) VerifyScope(context.Context, string, string, string) (actor.Role, error) {
	return "", errScopeBridgeUnavailable
}

func (nilScopeBridge) EnsureMembershipOnActivation(context.Context, string, string, *string, actor.Role) error {
	return errScopeBridgeUnavailable
}

// discardEmailSender is the non-delivering email adapter. The provisioning
// command hands the activation URL to the operator on stdout instead of sending
// mail, and it never logs or persists the raw token.
type discardEmailSender struct{}

var _ ports.EmailSender = discardEmailSender{}

func (discardEmailSender) SendInvitation(context.Context, string, string, string) error { return nil }

func (discardEmailSender) SendPasswordRecovery(context.Context, string, string) error { return nil }
