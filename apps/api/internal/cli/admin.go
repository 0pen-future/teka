package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"teka/apps/api/internal/app"
	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/users"
)

var (
	adminCreateEmail    string
	adminCreatePassword string
	adminCreateName     string
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrative account operations",
}

var adminCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an administrator account",
	RunE: func(cmd *cobra.Command, _ []string) error {
		password := adminCreatePassword
		if password == "" {
			var err error
			if password, err = promptPassword(); err != nil {
				return err
			}
		}
		if len(password) < 8 {
			return errors.New("password must be at least 8 characters")
		}
		name := adminCreateName
		if name == "" {
			name = "Administrator"
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

		svc := users.NewService(users.NewRepository(c.DB))
		u, err := svc.Create(cmd.Context(), users.CreateRequest{
			Email:    adminCreateEmail,
			Password: password,
			Name:     name,
			Role:     users.RoleAdmin,
		})
		if err != nil {
			return err
		}
		cmd.Printf("admin created: %s (%s)\n", u.Email, u.ID)
		return nil
	},
}

// promptPassword reads the password without echo when stdin is a terminal,
// falling back to a plain line read for piped input.
func promptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Password: ")
	if term.IsTerminal(int(os.Stdin.Fd())) {
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return string(raw), nil
	}
	var line string
	if _, err := fmt.Fscanln(os.Stdin, &line); err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func init() {
	adminCreateCmd.Flags().StringVar(&adminCreateEmail, "email", "", "admin email address (required)")
	adminCreateCmd.Flags().StringVar(&adminCreatePassword, "password", "", "admin password (prompted when omitted)")
	adminCreateCmd.Flags().StringVar(&adminCreateName, "name", "", "admin display name")
	_ = adminCreateCmd.MarkFlagRequired("email")
	adminCmd.AddCommand(adminCreateCmd)
}
