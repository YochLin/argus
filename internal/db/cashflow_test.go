package db

import (
	"errors"
	"testing"
)

// TestUpdateRecurringCashflowKeepsWhatIsNotEditable pins that an edit changes
// name/amount/day/category and leaves direction, currency and the paused flag
// alone — including on a paused line, and that an unknown id is an error.
func TestUpdateRecurringCashflowKeepsWhatIsNotEditable(t *testing.T) {
	d := newTestDB(t)

	day := int64(5)
	id, err := d.CreateRecurringCashflow(NewRecurringCashflow{
		Direction: "in", Name: "薪資", Amount: 80000, Currency: "USD", DayOfMonth: &day, Category: "salary",
	})
	if err != nil {
		t.Fatalf("CreateRecurringCashflow() error = %v", err)
	}
	if err := d.DeactivateRecurringCashflow(id, "2026-10-04"); err != nil {
		t.Fatalf("DeactivateRecurringCashflow() error = %v", err)
	}

	if err := d.UpdateRecurringCashflow(id, RecurringCashflowEdit{Name: "薪資(加薪)", Amount: 90000}); err != nil {
		t.Fatalf("UpdateRecurringCashflow() error = %v", err)
	}
	list, err := d.ListRecurringCashflows(false)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListRecurringCashflows() = %+v, %v", list, err)
	}
	c := list[0]
	if c.Name != "薪資(加薪)" || c.Amount != 90000 || c.DayOfMonth != nil || c.Category != "" {
		t.Errorf("row = %+v, want new name/amount with day and category cleared (full overwrite)", c)
	}
	if c.Direction != "in" || c.Currency != "USD" || c.Active {
		t.Errorf("row = %+v, direction/currency/active must not change", c)
	}

	if err := d.UpdateRecurringCashflow(999, RecurringCashflowEdit{Name: "x", Amount: 1}); !errors.Is(err, ErrCashflowNotFound) {
		t.Errorf("UpdateRecurringCashflow(unknown id) = %v, want ErrCashflowNotFound", err)
	}
}

func TestCreateAndListRecurringCashflows(t *testing.T) {
	d := newTestDB(t)

	day := int64(15)
	id, err := d.CreateRecurringCashflow(NewRecurringCashflow{
		Direction: "out", Name: "房貸", Amount: 35000, DayOfMonth: &day, Category: "mortgage",
	})
	if err != nil {
		t.Fatalf("CreateRecurringCashflow() error = %v", err)
	}
	if _, err := d.CreateRecurringCashflow(NewRecurringCashflow{Direction: "in", Name: "薪資", Amount: 80000, Category: "salary"}); err != nil {
		t.Fatalf("CreateRecurringCashflow() error = %v", err)
	}

	list, err := d.ListRecurringCashflows(false)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListRecurringCashflows(false) = %+v, %v; want 2 rows", list, err)
	}
	for _, c := range list {
		if c.ID == id {
			if c.DayOfMonth == nil || *c.DayOfMonth != 15 || c.Currency != "TWD" || !c.Active {
				t.Errorf("mortgage row = %+v, want DayOfMonth=15, Currency=TWD, Active=true", c)
			}
		}
	}

	if err := d.DeactivateRecurringCashflow(id, "2026-10-04"); err != nil {
		t.Fatalf("DeactivateRecurringCashflow() error = %v", err)
	}
	active, err := d.ListRecurringCashflows(true)
	if err != nil || len(active) != 1 || active[0].Name != "薪資" {
		t.Fatalf("ListRecurringCashflows(true) after deactivate = %+v, %v; want only 薪資", active, err)
	}

	// Deactivating an already-inactive row is a no-op, not an error.
	if err := d.DeactivateRecurringCashflow(id, "2026-10-04"); err != nil {
		t.Fatalf("DeactivateRecurringCashflow() twice error = %v", err)
	}
}

// TestCashflowPauseResumeDelete pins the lifecycle: pausing stamps the day and
// drops the flow from the active list, pausing again keeps the first day,
// resuming clears it, and only a paused flow can be deleted — an active one is
// refused rather than silently removed from the monthly totals.
func TestCashflowPauseResumeDelete(t *testing.T) {
	d := newTestDB(t)
	id, err := d.CreateRecurringCashflow(NewRecurringCashflow{Direction: "out", Name: "定期定額", Amount: 30000, Category: "sip"})
	if err != nil {
		t.Fatalf("CreateRecurringCashflow() error = %v", err)
	}
	row := func() RecurringCashflow {
		t.Helper()
		list, err := d.ListRecurringCashflows(false)
		if err != nil || len(list) != 1 {
			t.Fatalf("ListRecurringCashflows(false) = %+v, %v; want 1 row", list, err)
		}
		return list[0]
	}

	if err := d.DeleteRecurringCashflow(id); !errors.Is(err, ErrCashflowActive) {
		t.Fatalf("DeleteRecurringCashflow(active) = %v, want ErrCashflowActive", err)
	}
	if row().ID != id {
		t.Fatal("an active flow must survive a refused delete")
	}

	if err := d.DeactivateRecurringCashflow(id, "2026-10-04"); err != nil {
		t.Fatalf("DeactivateRecurringCashflow() error = %v", err)
	}
	if c := row(); c.Active || c.PausedAt != "2026-10-04" {
		t.Errorf("after pause row = %+v, want inactive, PausedAt 2026-10-04", c)
	}
	if err := d.DeactivateRecurringCashflow(id, "2026-10-09"); err != nil {
		t.Fatalf("pausing twice error = %v", err)
	}
	if c := row(); c.PausedAt != "2026-10-04" {
		t.Errorf("PausedAt = %q after a second pause, want the first day kept", c.PausedAt)
	}

	if err := d.ResumeRecurringCashflow(id); err != nil {
		t.Fatalf("ResumeRecurringCashflow() error = %v", err)
	}
	if c := row(); !c.Active || c.PausedAt != "" {
		t.Errorf("after resume row = %+v, want active with PausedAt cleared", c)
	}
	if err := d.ResumeRecurringCashflow(id); err != nil {
		t.Fatalf("resuming an active flow error = %v, want a no-op", err)
	}

	if err := d.DeactivateRecurringCashflow(id, "2026-10-05"); err != nil {
		t.Fatalf("DeactivateRecurringCashflow() error = %v", err)
	}
	if err := d.DeleteRecurringCashflow(id); err != nil {
		t.Fatalf("DeleteRecurringCashflow(paused) error = %v", err)
	}
	if list, err := d.ListRecurringCashflows(false); err != nil || len(list) != 0 {
		t.Errorf("ListRecurringCashflows(false) = %+v, %v; want empty after delete", list, err)
	}
}

func TestCashflowLifecycleUnknownID(t *testing.T) {
	d := newTestDB(t)
	if err := d.DeactivateRecurringCashflow(999, "2026-10-04"); !errors.Is(err, ErrCashflowNotFound) {
		t.Errorf("DeactivateRecurringCashflow(unknown) = %v, want ErrCashflowNotFound", err)
	}
	if err := d.ResumeRecurringCashflow(999); !errors.Is(err, ErrCashflowNotFound) {
		t.Errorf("ResumeRecurringCashflow(unknown) = %v, want ErrCashflowNotFound", err)
	}
	if err := d.DeleteRecurringCashflow(999); !errors.Is(err, ErrCashflowNotFound) {
		t.Errorf("DeleteRecurringCashflow(unknown) = %v, want ErrCashflowNotFound", err)
	}
}

// TestMigration38KeepsLegacyPausedFlows pins that a flow paused before the
// paused_at column existed is still listed as paused, with no date.
func TestMigration38KeepsLegacyPausedFlows(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.conn.Exec(`INSERT INTO recurring_cashflows (direction, name, amount, currency, active) VALUES ('out', '舊的', 100, 'TWD', 0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	list, err := d.ListRecurringCashflows(false)
	if err != nil || len(list) != 1 || list[0].Active || list[0].PausedAt != "" {
		t.Fatalf("ListRecurringCashflows(false) = %+v, %v; want one paused row without a date", list, err)
	}
}
