package db

import "testing"

func TestGetSettingUnsetKeyIsNotFound(t *testing.T) {
	d := newTestDB(t)

	_, ok, err := d.GetSetting("cash_balance")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}
	if ok {
		t.Error("GetSetting() on an unset key should return ok=false")
	}
}

func TestSettingRoundTripAndUpsert(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetSetting("cash_balance", "1000.50"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}
	got, ok, err := d.GetSetting("cash_balance")
	if err != nil || !ok || got != "1000.50" {
		t.Errorf("GetSetting() = %q, %v, %v; want \"1000.50\", true, nil", got, ok, err)
	}

	// Same key should overwrite, not conflict.
	if err := d.SetSetting("cash_balance", "2000"); err != nil {
		t.Fatalf("SetSetting() (upsert) error = %v", err)
	}
	got, ok, err = d.GetSetting("cash_balance")
	if err != nil || !ok || got != "2000" {
		t.Errorf("GetSetting() after upsert = %q, %v, %v; want \"2000\", true, nil", got, ok, err)
	}
}
