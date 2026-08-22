package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"teka/apps/api/internal/app"
	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/teachers"
)

var (
	resetPasswordPhone    string
	resetPasswordGenerate bool
	resetPasswordForce    bool
)

var resetPasswordCmd = &cobra.Command{
	Use:   "reset-password",
	Short: "Rewrite an account's password and revoke its sessions",
	Long: `reset-password is the operator recovery path for an account that has no
self-service way back in — most commonly a center owner, who is deliberately
excluded from the forgot-password flow. It also works on a disabled account,
WITHOUT changing its status: a disabled account stays disabled and still
cannot log in after this command runs, only its stored password changes.
Every refresh token the account currently holds is revoked, so an old
session cannot outlive the reset.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if resetPasswordPhone == "" {
			return errors.New("--phone is required")
		}
		// Production-facing operator command, no environment refusal (must run
		// unattended in a deploy shell) — --force is the only confirmation
		// gate, not a TTY prompt.
		if !resetPasswordForce {
			return errors.New("this rewrites the account's password and revokes its sessions; re-run with --force to confirm")
		}

		password, err := resolvePassword(cmd, resetPasswordGenerate)
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		c, err := app.NewContainer(cfg)
		if err != nil {
			return err
		}
		defer c.Close()

		accountID, err := resetPassword(cmd.Context(), c.TxManager, c.Teachers, c.Auth, resetPasswordPhone, password)
		if err != nil {
			return err
		}

		cmd.Println("password reset for account:", accountID)
		if resetPasswordGenerate {
			cmd.Println("new password (store this now, it will not be shown again):", password)
		}
		return nil
	},
}

// resetPassword looks accountPhone up, then rewrites its password hash and
// revokes every refresh token it holds in one transaction — the write path
// itself never touches status, so it works identically on an active or a
// disabled account. Not found is reported as a plain error, not the
// anti-enumeration rejection auth.Service.ForgotPassword uses: an operator
// running this command is trusted, unlike an unauthenticated HTTP caller.
//
// It takes tx/teachersSvc/authSvc directly rather than *app.Container so it
// can be exercised in an integration test against a real Postgres without
// building the rest of the container.
func resetPassword(ctx context.Context, tx database.TxManager, teachersSvc *teachers.Service, authSvc *auth.Service,
	accountPhone, password string) (uuid.UUID, error) {
	acct, err := teachersSvc.FindByPhone(ctx, accountPhone)
	if err != nil {
		if errors.Is(err, teachers.ErrNotFound) {
			return uuid.Nil, fmt.Errorf("no account found for phone %s", accountPhone)
		}
		return uuid.Nil, err
	}

	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := teachersSvc.SetPasswordForRecovery(ctx, acct.ID, password); err != nil {
			return err
		}
		return authSvc.RevokeAllForUser(ctx, acct.ID)
	})
	if err != nil {
		return uuid.Nil, err
	}
	return acct.ID, nil
}

func init() {
	resetPasswordCmd.Flags().StringVar(&resetPasswordPhone, "phone", "", "phone number of the account to reset (required)")
	resetPasswordCmd.Flags().BoolVar(&resetPasswordGenerate, "generate", false, "generate a random password instead of prompting")
	resetPasswordCmd.Flags().BoolVar(&resetPasswordForce, "force", false, "confirm the password reset (required)")
}
