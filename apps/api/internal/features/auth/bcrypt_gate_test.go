package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBcryptGateRefusesWhenSlotsStayBusy(t *testing.T) {
	g := newBcryptGate(1, 10*time.Millisecond)
	release := make(chan struct{})
	holding := make(chan struct{})
	go func() {
		_ = g.run(context.Background(), func() {
			close(holding)
			<-release
		})
	}()
	<-holding

	err := g.run(context.Background(), func() { t.Fatal("must not run while the only slot is held") })
	if !errors.Is(err, errBcryptBusy) {
		t.Fatalf("err = %v, want errBcryptBusy", err)
	}

	close(release)
	ran := false
	if err := g.run(context.Background(), func() { ran = true }); err != nil || !ran {
		t.Fatalf("after release: err = %v, ran = %v", err, ran)
	}
}

func TestBcryptGateReturnsContextErrorWhenCallerLeaves(t *testing.T) {
	g := newBcryptGate(1, time.Minute)
	release := make(chan struct{})
	holding := make(chan struct{})
	go func() {
		_ = g.run(context.Background(), func() {
			close(holding)
			<-release
		})
	}()
	<-holding
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.run(ctx, func() {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
