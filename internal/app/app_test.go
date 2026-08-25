package app

import (
	"context"
	"path/filepath"
	"testing"
)

// TestBootWithoutTelegram is the smoke test for the one thing Boot's wiring
// can get wrong silently: which optional pieces exist. No Telegram token and
// no API keys is the state a fresh install actually boots in (Phase 17 PR1),
// and it must still produce a usable process — the dashboard being the only
// way to configure the rest. t.Chdir keeps Boot's os.MkdirAll("data") and its
// DB/log files inside the temp dir.
func TestBootWithoutTelegram(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cases := []struct {
		name    string
		webAddr string
		wantWeb bool
	}{
		{name: "dashboard off", webAddr: "", wantWeb: false},
		{name: "dashboard on", webAddr: "127.0.0.1:0", wantWeb: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := Boot(context.Background(), Config{
				DBPath:  filepath.Join(dir, tc.name+".db"),
				LogPath: filepath.Join(dir, tc.name+".log"),
				WebAddr: tc.webAddr,
			})
			if err != nil {
				t.Fatalf("Boot: %v", err)
			}
			defer a.Close()

			if a.Bot != nil {
				t.Error("Bot should be nil without a Telegram token")
			}
			if (a.Web != nil) != tc.wantWeb {
				t.Errorf("Web != nil = %v, want %v", a.Web != nil, tc.wantWeb)
			}
			if a.DB == nil || a.Scheduler == nil || a.LLM == nil {
				t.Error("DB/Scheduler/LLM must always be wired")
			}
			if a.PaperDB != nil {
				t.Error("PaperDB should be nil without PAPER_DB_PATH")
			}
		})
	}
}
