package outbox

import (
	"sync"
	"testing"

	"github.com/vincent-tien/gokit/eda/event"
)

// helpers

func mustReg(t *testing.T, name string, version int, source string) {
	t.Helper()
	if err := Register(RegisteredEvent{Name: name, Version: version, Source: source}); err != nil {
		t.Fatalf("unexpected Register error: %v", err)
	}
}

// --- tests ---

func TestRegister_Duplicate(t *testing.T) {
	t.Cleanup(Reset)

	mustReg(t, "order.created", 1, "orders")
	err := Register(RegisteredEvent{Name: "order.created", Version: 1, Source: "orders"})
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}

func TestRegister_EmptyName(t *testing.T) {
	t.Cleanup(Reset)

	err := Register(RegisteredEvent{Name: "", Version: 1, Source: "svc"})
	if err == nil {
		t.Fatal("expected error on empty name, got nil")
	}
}

func TestRegister_ZeroVersion(t *testing.T) {
	t.Cleanup(Reset)

	err := Register(RegisteredEvent{Name: "order.created", Version: 0, Source: "svc"})
	if err == nil {
		t.Fatal("expected error on version 0, got nil")
	}
}

func TestMustRegister_Panics(t *testing.T) {
	t.Cleanup(Reset)

	MustRegister(RegisteredEvent{Name: "payment.failed", Version: 1, Source: "payments"})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate MustRegister, got none")
		}
	}()
	MustRegister(RegisteredEvent{Name: "payment.failed", Version: 1, Source: "payments"})
}

func TestLookup_FoundAndNotFound(t *testing.T) {
	t.Cleanup(Reset)

	mustReg(t, "invoice.published", 2, "billing")

	got, ok := Lookup("invoice.published", 2)
	if !ok {
		t.Fatal("expected to find registered event, got false")
	}
	if got.Name != "invoice.published" || got.Version != 2 || got.Source != "billing" {
		t.Fatalf("unexpected result: %+v", got)
	}

	_, ok = Lookup("invoice.published", 99)
	if ok {
		t.Fatal("expected not-found for unregistered version, got true")
	}
}

func TestAll_StableOrder(t *testing.T) {
	t.Cleanup(Reset)

	// register out of order
	mustReg(t, "z.event", 1, "svc")
	mustReg(t, "a.event", 2, "svc")
	mustReg(t, "a.event", 1, "svc")
	mustReg(t, "m.event", 1, "svc")

	all := All()
	if len(all) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(all))
	}

	want := []struct{ name string; ver int }{
		{"a.event", 1},
		{"a.event", 2},
		{"m.event", 1},
		{"z.event", 1},
	}
	for i, w := range want {
		if all[i].Name != w.name || all[i].Version != w.ver {
			t.Errorf("all[%d] = {%q, %d}, want {%q, %d}", i, all[i].Name, all[i].Version, w.name, w.ver)
		}
	}
}

func TestValidateMeta_OK_AndUnknown(t *testing.T) {
	t.Cleanup(Reset)

	mustReg(t, "user.registered", 1, "auth")

	okMeta := event.EventMeta{Name: "user.registered", Version: 1}
	if err := ValidateMeta(okMeta); err != nil {
		t.Fatalf("expected nil error for registered event, got: %v", err)
	}

	unknownMeta := event.EventMeta{Name: "user.registered", Version: 99}
	if err := ValidateMeta(unknownMeta); err == nil {
		t.Fatal("expected error for unregistered event version, got nil")
	}
}

func TestVersionCoexistence(t *testing.T) {
	t.Cleanup(Reset)

	mustReg(t, "foo", 1, "svc")
	mustReg(t, "foo", 2, "svc")

	e1, ok1 := Lookup("foo", 1)
	e2, ok2 := Lookup("foo", 2)

	if !ok1 || e1.Version != 1 {
		t.Fatalf("expected foo v1 to be found, got ok=%v, entry=%+v", ok1, e1)
	}
	if !ok2 || e2.Version != 2 {
		t.Fatalf("expected foo v2 to be found, got ok=%v, entry=%+v", ok2, e2)
	}
}

func TestConcurrent_RegisterAndLookup(t *testing.T) {
	t.Cleanup(Reset)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			// Register ignores the duplicate error — races between goroutines are expected.
			_ = Register(RegisteredEvent{
				Name:    "concurrent.event",
				Version: 1,
				Source:  "test",
			})
			// Lookup must never panic or return inconsistent results.
			_, _ = Lookup("concurrent.event", 1)
			_ = n
		}(i)
	}
	wg.Wait()

	// After all goroutines complete, exactly one registration should exist.
	all := All()
	found := false
	for _, e := range all {
		if e.Name == "concurrent.event" && e.Version == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected concurrent.event v1 to be present after concurrent registrations")
	}
}
