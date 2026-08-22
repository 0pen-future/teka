package cli

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// readPasswordLine reads one line of input from the given file descriptor
// without echoing it. A func-var seam, not a direct term.ReadPassword call:
// term.ReadPassword needs a real terminal fd, which unit tests don't have, so
// tests substitute a fake here instead.
var readPasswordLine = term.ReadPassword

// passwordAlphabet is 64 characters — 26 upper, 26 lower, 10 digits, and 2
// URL-safe symbols — chosen so len(passwordAlphabet) evenly divides 256: a
// random byte reduced mod 64 has zero modulo bias, so no rejection sampling
// is needed.
const passwordAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// generatedPasswordLength is comfortably above the entropy a human would
// type, since a generated password is never memorized — only ever displayed
// once and immediately handed off or stored in a secrets manager.
const generatedPasswordLength = 20

// generatePassword returns a random one-time password built from
// passwordAlphabet using crypto/rand — never math/rand, which is predictable.
func generatePassword() (string, error) {
	raw := make([]byte, generatedPasswordLength)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	out := make([]byte, generatedPasswordLength)
	for i, b := range raw {
		out[i] = passwordAlphabet[int(b)%len(passwordAlphabet)]
	}
	return string(out), nil
}

// promptPassword double-prompts stdin for a password without echoing it,
// confirming both entries match before returning. out is where the prompts
// and newlines are written (cmd.OutOrStdout() in production, a buffer in
// tests) — never the password itself.
func promptPassword(out io.Writer) (string, error) {
	// The prompt text is a courtesy for an interactive terminal; a failed
	// write to it is never fatal on its own — readPasswordLine below is what
	// actually surfaces a broken stdin/stdout.
	_, _ = fmt.Fprint(out, "Password: ")
	first, err := readPasswordLine(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	_, _ = fmt.Fprint(out, "Confirm password: ")
	second, err := readPasswordLine(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("read password confirmation: %w", err)
	}
	if len(first) == 0 {
		return "", errors.New("password must not be empty")
	}
	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	return string(first), nil
}

// resolvePassword returns the password an onboarding/recovery command should
// set: a freshly generated one when generate is true (the caller must print
// it to stdout exactly once — this function never does), otherwise an
// interactive non-echo double-entry prompt. There is deliberately no
// --password flag: a password on the command line lands in shell history.
func resolvePassword(cmd *cobra.Command, generate bool) (string, error) {
	if generate {
		return generatePassword()
	}
	return promptPassword(cmd.OutOrStdout())
}
