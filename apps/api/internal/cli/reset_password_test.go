package cli

import "testing"

// resetResetPasswordFlags restores every reset-password flag var to its zero
// value — see resetCreateCenterFlags for why this is necessary between runs.
func resetResetPasswordFlags() {
	resetPasswordPhone = ""
	resetPasswordGenerate = false
	resetPasswordForce = false
}

func runResetPassword(t *testing.T, args ...string) error {
	t.Helper()
	resetResetPasswordFlags()
	t.Cleanup(resetResetPasswordFlags)

	// See runCreateCenter: Execute always dispatches through rootCmd's args.
	rootCmd.SetArgs(append([]string{"reset-password"}, args...))
	return rootCmd.Execute()
}

func TestResetPasswordRequiresPhone(t *testing.T) {
	err := runResetPassword(t, "--force")
	if err == nil {
		t.Fatal("want error when --phone is missing")
	}
}

func TestResetPasswordRequiresForce(t *testing.T) {
	err := runResetPassword(t, "--phone", "+84901234567")
	if err == nil {
		t.Fatal("want error when --force is missing")
	}
}
