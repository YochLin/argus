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
	if err := d.DeactivateRecurringCashflow(id); err != nil {
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

	if err := d.DeactivateRecurringCashflow(id); err != nil {
		t.Fatalf("DeactivateRecurringCashflow() error = %v", err)
	}
	active, err := d.ListRecurringCashflows(true)
	if err != nil || len(active) != 1 || active[0].Name != "薪資" {
		t.Fatalf("ListRecurringCashflows(true) after deactivate = %+v, %v; want only 薪資", active, err)
	}

	// Deactivating an already-inactive row is a no-op, not an error.
	if err := d.DeactivateRecurringCashflow(id); err != nil {
		t.Fatalf("DeactivateRecurringCashflow() twice error = %v", err)
	}
}
