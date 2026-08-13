package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newTestCmd builds a bare cobra.Command whose OutOrStdout() is a throwaway
// buffer — enough for resolvePassword/promptPassword, which only need
// something to write prompts to.
func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	return cmd
}

func TestGeneratePasswordLengthAndCharset(t *testing.T) {
	pw, err := generatePassword()
	if err != nil {
		t.Fatalf("generatePassword: %v", err)
	}
	if len(pw) != generatedPasswordLength {
		t.Fatalf("length = %d, want %d", len(pw), generatedPasswordLength)
	}
	for _, r := range pw {
		if !strings.ContainsRune(passwordAlphabet, r) {
			t.Fatalf("password %q contains char %q outside the alphabet", pw, r)
		}
	}
}

func TestGeneratePasswordIsRandomAcrossCalls(t *testing.T) {
	seen := make(map[string]bool)
	for range 20 {
		pw, err := generatePassword()
		if err != nil {
			t.Fatalf("generatePassword: %v", err)
		}
		if seen[pw] {
			t.Fatalf("generatePassword produced a duplicate: %q", pw)
		}
		seen[pw] = true
	}
}

func withFakeReadPasswordLine(t *testing.T, fn func(fd int) ([]byte, error)) {
	t.Helper()
	orig := readPasswordLine
	readPasswordLine = fn
	t.Cleanup(func() { readPasswordLine = orig })
}

func TestPromptPasswordMatchingEntriesSucceeds(t *testing.T) {
	calls := 0
	withFakeReadPasswordLine(t, func(int) ([]byte, error) {
		calls++
		return []byte("s3cret-pw"), nil
	})

	var out bytes.Buffer
	got, err := promptPassword(&out)
	if err != nil {
		t.Fatalf("promptPassword: %v", err)
	}
	if got != "s3cret-pw" {
		t.Fatalf("password = %q, want s3cret-pw", got)
	}
	if calls != 2 {
		t.Fatalf("readPasswordLine called %d times, want 2 (entry + confirmation)", calls)
	}
	if out.Len() == 0 {
		t.Error("expected prompt text written to out")
	}
}

func TestPromptPasswordMismatchedEntriesFails(t *testing.T) {
	first := true
	withFakeReadPasswordLine(t, func(int) ([]byte, error) {
		defer func() { first = false }()
		if first {
			return []byte("first-password"), nil
		}
		return []byte("second-password"), nil
	})

	var out bytes.Buffer
	if _, err := promptPassword(&out); err == nil {
		t.Fatal("want error on mismatched entries, got nil")
	}
}

func TestPromptPasswordEmptyEntryFails(t *testing.T) {
	withFakeReadPasswordLine(t, func(int) ([]byte, error) { return []byte(""), nil })

	var out bytes.Buffer
	if _, err := promptPassword(&out); err == nil {
		t.Fatal("want error on empty password, got nil")
	}
}

func TestPromptPasswordReadErrorPropagates(t *testing.T) {
	withFakeReadPasswordLine(t, func(int) ([]byte, error) { return nil, errors.New("no tty") })

	var out bytes.Buffer
	if _, err := promptPassword(&out); err == nil {
		t.Fatal("want error when readPasswordLine fails, got nil")
	}
}

func TestResolvePasswordGenerateSkipsPrompt(t *testing.T) {
	withFakeReadPasswordLine(t, func(int) ([]byte, error) {
		t.Fatal("readPasswordLine must not be called when generate=true")
		return nil, nil
	})

	pw, err := resolvePassword(newTestCmd(), true)
	if err != nil {
		t.Fatalf("resolvePassword: %v", err)
	}
	if len(pw) != generatedPasswordLength {
		t.Fatalf("generated password length = %d, want %d", len(pw), generatedPasswordLength)
	}
}

func TestResolvePasswordPromptsWhenNotGenerating(t *testing.T) {
	withFakeReadPasswordLine(t, func(int) ([]byte, error) { return []byte("typed-pw"), nil })

	pw, err := resolvePassword(newTestCmd(), false)
	if err != nil {
		t.Fatalf("resolvePassword: %v", err)
	}
	if pw != "typed-pw" {
		t.Fatalf("password = %q, want typed-pw", pw)
	}
}
