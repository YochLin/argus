package app

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"
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

			// Headless, not nil (Phase 24 Stage 3): a Telegram-less
			// process still has to run every scheduler job, or the
			// dashboard's P&L curve replays a daily_snapshots table
			// nothing ever wrote to.
			if a.Bot == nil {
				t.Error("Bot must be wired headless without a Telegram token")
			}
			if a.telegram {
				t.Error("telegram must be false without a Telegram token")
			}
			// 11 with the dashboard on: the 9 Telegram-era jobs + 2
			// universe scans + log rotation + backup, and the two
			// sector-flow scans only when Web exists. Asserted as a count,
			// not a list, so re-gating any of them on Telegram fails here.
			wantJobs := 13
			if tc.wantWeb {
				wantJobs += 2
			}
			if got := a.Scheduler.JobCount(); got != wantJobs {
				t.Errorf("scheduler jobs = %d, want %d", got, wantJobs)
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

// TestRunExitsWhenWebListenerFails is the regression test for the zombie
// process a bad WEB_ADDR used to cause: Run must return on its own once the
// HTTP listener fails to bind, not hang on <-ctx.Done() forever with a
// caller-supplied ctx that's never cancelled (systemd would see a "healthy"
// PID with nothing listening on its port and never restart it).
func TestRunExitsWhenWebListenerFails(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	defer occupied.Close()

	a, err := Boot(context.Background(), Config{
		DBPath:  filepath.Join(dir, "test.db"),
		LogPath: filepath.Join(dir, "test.log"),
		WebAddr: occupied.Addr().String(),
	})
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	defer a.Close()

	done := make(chan struct{})
	go func() {
		a.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after the web listener failed to bind (zombie process)")
	}
}
