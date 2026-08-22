package cli

import "testing"

// resetCreateCenterFlags restores every create-center flag var to its zero
// value. cobra's flag parsing only overwrites vars for flags present in the
// new args, so tests that call Execute repeatedly on the same *cobra.Command
// must reset by hand or a later test would silently inherit an earlier
// test's flag values.
func resetCreateCenterFlags() {
	createCenterName = ""
	createCenterOwnerPhone = ""
	createCenterOwnerName = ""
	createCenterGenerate = false
	createCenterForce = false
}

func runCreateCenter(t *testing.T, args ...string) error {
	t.Helper()
	resetCreateCenterFlags()
	t.Cleanup(resetCreateCenterFlags)

	// cobra always dispatches Execute through the root command regardless of
	// which command in the tree it is called on, using the root's own args —
	// so args must be set on rootCmd, not on createCenterCmd directly.
	rootCmd.SetArgs(append([]string{"create-center"}, args...))
	return rootCmd.Execute()
}

func TestCreateCenterRequiresName(t *testing.T) {
	err := runCreateCenter(t, "--owner-phone", "+84901234567", "--owner-name", "Owner", "--force")
	if err == nil {
		t.Fatal("want error when --name is missing")
	}
}

func TestCreateCenterRequiresOwnerPhone(t *testing.T) {
	err := runCreateCenter(t, "--name", "Center", "--owner-name", "Owner", "--force")
	if err == nil {
		t.Fatal("want error when --owner-phone is missing")
	}
}

func TestCreateCenterRequiresOwnerName(t *testing.T) {
	err := runCreateCenter(t, "--name", "Center", "--owner-phone", "+84901234567", "--force")
	if err == nil {
		t.Fatal("want error when --owner-name is missing")
	}
}

func TestCreateCenterRequiresForce(t *testing.T) {
	err := runCreateCenter(t, "--name", "Center", "--owner-phone", "+84901234567", "--owner-name", "Owner")
	if err == nil {
		t.Fatal("want error when --force is missing")
	}
}
