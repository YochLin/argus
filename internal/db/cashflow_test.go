package db

import "testing"

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
