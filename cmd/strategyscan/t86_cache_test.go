package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadT86Cache_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t86.csv")
	content := "Date,Code,ForeignNet,TrustNet\n" +
		"2024-01-02,2330,1000,-500\n" +
		"2024-01-03,2330,-200,300\n" +
		"2024-01-02,2317,50,0\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := loadT86Cache(path)
	if err != nil {
		t.Fatalf("loadT86Cache: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tickers, want 2", len(got))
	}
	rows := got["2330"]
	if len(rows) != 2 {
		t.Fatalf("2330: got %d rows, want 2", len(rows))
	}
	// oldest-first
	if rows[0].Date.Format("2006-01-02") != "2024-01-02" || rows[0].ForeignNet != 1000 || rows[0].Net != -500 {
		t.Errorf("2330[0] = %+v, want date 2024-01-02 foreign=1000 net=-500", rows[0])
	}
	if rows[1].Date.Format("2006-01-02") != "2024-01-03" || rows[1].ForeignNet != -200 || rows[1].Net != 300 {
		t.Errorf("2330[1] = %+v, want date 2024-01-03 foreign=-200 net=300", rows[1])
	}
	if len(got["2317"]) != 1 {
		t.Errorf("2317: got %d rows, want 1", len(got["2317"]))
	}
}
