package cli

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"teka/apps/api/internal/app"
	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/id"
)

var (
	createCenterName       string
	createCenterOwnerPhone string
	createCenterOwnerName  string
	createCenterGenerate   bool
	createCenterForce      bool
)

var createCenterCmd = &cobra.Command{
	Use:   "create-center",
	Short: "Bootstrap a new center with its owner account",
	Long: `create-center provisions a new center and its owner teacher account in a
single atomic transaction: on success the owner can log in immediately and
ResolveScope reports IsOwner=true. There is never a window where the center
exists without an owner, or an owner without a center — either both rows
land, or neither does (e.g. the phone already has an account).

Center names are not unique: this command always creates a new center, it
never updates an existing one. Use it to onboard the first customer of a
center, or to recover a center whose owner account was lost.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if createCenterName == "" || createCenterOwnerPhone == "" || createCenterOwnerName == "" {
			return errors.New("--name, --owner-phone, and --owner-name are required")
		}
		// This is a production-facing operator command with no environment
		// refusal (it must run unattended in a deploy shell) — --force is the
		// only confirmation gate, not a TTY prompt.
		if !createCenterForce {
			return errors.New("this creates a new center and owner account; re-run with --force to confirm")
		}

		password, err := resolvePassword(cmd, createCenterGenerate)
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

		centerID, accountID, err := bootstrapCenter(cmd.Context(), c.DB, c.TxManager, c.Teachers, c.Centers,
			createCenterName, createCenterOwnerPhone, createCenterOwnerName, password)
		if err != nil {
			return err
		}

		cmd.Println("center id:", centerID)
		cmd.Println("owner account id:", accountID)
		if createCenterGenerate {
			cmd.Println("owner password (store this now, it will not be shown again):", password)
		}
		return nil
	},
}

// bootstrapCenter inserts the center and its owner account in one
// transaction: insert the center row (owner_id set to the center's own new
// id — a self-referencing placeholder that satisfies the NOT NULL constraint
// while its real FK check is deferred to commit, per migration
// 000007_centers.up.sql) -> create the owner's teacher account in that center
// -> rewrite owner_id to the real account id -> open its membership. If the
// phone already has an account, CreateInCenter fails and the whole
// transaction — including the placeholder center row — rolls back, so there
// is never a center left ownerless or an account left centerless.
//
// It takes db/tx/teachersSvc/centersSvc directly rather than *app.Container
// so it can be exercised in an integration test against a real Postgres
// without building the rest of the container (secrets cipher, notifications,
// zalo) that create-center never touches.
func bootstrapCenter(ctx context.Context, db *gorm.DB, tx database.TxManager, teachersSvc *teachers.Service, centersSvc *centers.Service,
	name, ownerPhone, ownerName, password string) (centerID, accountID uuid.UUID, err error) {
	centerRepo := centers.NewRepository(db)
	centerID = id.New()

	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		center := &centers.Center{ID: centerID, Name: name, OwnerID: centerID}
		if err := centerRepo.CreateCenter(ctx, center); err != nil {
			return err
		}

		aid, err := teachersSvc.CreateInCenter(ctx, ownerPhone, ownerName, password, centerID)
		if err != nil {
			return err
		}
		accountID = aid

		// owner_id must point at the real account BEFORE the membership
		// opens: OpenMembership assigns the default member role to everyone
		// except the center's owner, and against the placeholder it would
		// misread the owner as a plain member. The FK is deferred, so the
		// order swap is free.
		if err := database.FromContext(ctx, db).
			Model(&centers.Center{}).
			Where("id = ?", centerID).
			Update("owner_id", accountID).Error; err != nil {
			return err
		}

		return centersSvc.OpenMembership(ctx, accountID, centerID)
	})
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return centerID, accountID, nil
}

func init() {
	createCenterCmd.Flags().StringVar(&createCenterName, "name", "", "center display name (required)")
	createCenterCmd.Flags().StringVar(&createCenterOwnerPhone, "owner-phone", "", "owner's phone number (required)")
	createCenterCmd.Flags().StringVar(&createCenterOwnerName, "owner-name", "", "owner's display name (required)")
	createCenterCmd.Flags().BoolVar(&createCenterGenerate, "generate", false, "generate a random owner password instead of prompting")
	createCenterCmd.Flags().BoolVar(&createCenterForce, "force", false, "confirm creating a new center and owner account (required)")
}
