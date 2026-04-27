package chat

import (
	"errors"
	"testing"
)

// TestHRCannotFetchFinanceData ensures that the HR role cannot query invoice data.
func TestHRCannotFetchFinanceData(t *testing.T) {
	_, err := HRScope(nil, 1, "invoices", nil)
	if err == nil {
		t.Fatal("expected errOutOfScope for HR fetching invoices, got nil")
	}
	if !IsOutOfScope(err) {
		t.Fatalf("expected IsOutOfScope(err)=true, got error: %v", err)
	}
}

// TestFinanceCannotFetchHRData ensures that the Finance role cannot query shift/payroll data owned by HR.
func TestFinanceCannotFetchHRData(t *testing.T) {
	_, err := FinanceScope(nil, 1, "shifts", nil)
	if err == nil {
		t.Fatal("expected errOutOfScope for Finance fetching shifts, got nil")
	}
	if !IsOutOfScope(err) {
		t.Fatalf("expected IsOutOfScope(err)=true, got error: %v", err)
	}
}

// TestFinanceCannotFetchLeaveData ensures Finance cannot access leave records.
func TestFinanceCannotFetchLeaveData(t *testing.T) {
	_, err := FinanceScope(nil, 1, "leaves", nil)
	if !IsOutOfScope(err) {
		t.Fatalf("expected out-of-scope for Finance fetching leaves, got: %v", err)
	}
}

// TestHRCannotFetchExpenses ensures HR cannot access expense records.
func TestHRCannotFetchExpenses(t *testing.T) {
	_, err := HRScope(nil, 1, "expenses", nil)
	if !IsOutOfScope(err) {
		t.Fatalf("expected out-of-scope for HR fetching expenses, got: %v", err)
	}
}

// TestGuestCannotAccessUserTables ensures that unauthenticated/public access
// cannot reach any user-level intent.
func TestGuestCannotAccessUserTables(t *testing.T) {
	protectedIntents := []string{
		"user_list",
		"invoices",
		"expenses",
		"leaves",
		"payroll",
		"guard_list",
		"customer_bookings",
	}
	for _, intent := range protectedIntents {
		_, err := PublicScope(intent, nil)
		if !IsOutOfScope(err) {
			t.Errorf("expected out-of-scope for guest intent %q, got: %v", intent, err)
		}
	}
}

// TestPublicScopeAllowsStaticIntents verifies that guests can access non-restricted intents.
func TestPublicScopeAllowsStaticIntents(t *testing.T) {
	allowedIntents := []string{
		"service_catalog",
		"pricing",
		"company_info",
		"book_service",
		"contact_request",
	}
	for _, intent := range allowedIntents {
		_, err := PublicScope(intent, nil)
		if err != nil && IsOutOfScope(err) {
			t.Errorf("expected intent %q to be allowed for guest, but got out-of-scope error", intent)
		}
	}
}

// TestIsOutOfScopeOnlyMatchesSentinel ensures IsOutOfScope does not match arbitrary errors.
func TestIsOutOfScopeOnlyMatchesSentinel(t *testing.T) {
	otherErr := errors.New("some other error")
	if IsOutOfScope(otherErr) {
		t.Fatal("IsOutOfScope should return false for non-sentinel errors")
	}
	if IsOutOfScope(nil) {
		t.Fatal("IsOutOfScope should return false for nil")
	}
	if !IsOutOfScope(errOutOfScope) {
		t.Fatal("IsOutOfScope should return true for errOutOfScope sentinel")
	}
}
