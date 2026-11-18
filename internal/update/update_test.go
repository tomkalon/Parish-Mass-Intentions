package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var newProgram = append([]byte("MZ"), []byte(" nowa wersja programu")...)

type fakeGitHub struct {
	status   int
	tag      string
	draft    bool
	exe      []byte
	sums     string
	noAssets bool
}

func (g *fakeGitHub) server(t *testing.T) *httptest.Server {
	t.Helper()

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request without User-Agent")
		}
		if g.status != 0 {
			w.WriteHeader(g.status)
			return
		}
		assets := fmt.Sprintf(`[{"name": %q, "browser_download_url": %q}, {"name": %q, "browser_download_url": %q}]`,
			AssetName, srv.URL+"/download/exe", ChecksumsName, srv.URL+"/download/sums")
		if g.noAssets {
			assets = "[]"
		}
		_, _ = fmt.Fprintf(w, `{"tag_name": %q, "name": "Intencje Mszalne", "body": " Nowy wydruk. ", "published_at": "2026-11-20T10:00:00Z", "draft": %t, "assets": %s}`, g.tag, g.draft, assets)
	})
	mux.HandleFunc("/download/exe", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(g.exe)
	})
	mux.HandleFunc("/download/sums", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, g.sums)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func sumLine(data []byte, name string) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + "  " + name + "\n"
}

func newRelease() *fakeGitHub {
	return &fakeGitHub{tag: "v1.1.0", exe: newProgram, sums: "abc  inny.txt\n" + sumLine(newProgram, AssetName)}
}

type restarts struct {
	calls []string
	err   error
}

func (r *restarts) restart(exe string) error {
	r.calls = append(r.calls, exe)
	return r.err
}

func newTestUpdater(t *testing.T, gh *fakeGitHub, r *restarts) *Updater {
	t.Helper()

	exe := filepath.Join(t.TempDir(), "Intencje Mszalne.exe")
	err := os.WriteFile(exe, []byte("MZ stara wersja"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	u := New("1.0.0", exe, r.restart)
	u.API = gh.server(t).URL
	u.now = func() time.Time { return time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC) }

	return u
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestNewer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.1.0", "1.0.0", true},
		{"v1.0.1", "1.0.0", true},
		{"1.0.10", "1.0.9", true},
		{"2.0.0", "1.99.99", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.7.1", false},
		{"1.0", "0.9.0", false},
		{"1.0.0", "", true},
		{"abc", "1.0.0", false},
	}
	for _, tt := range tests {
		if got := Newer(tt.a, tt.b); got != tt.want {
			t.Errorf("Newer(%q, %q) = %t, want %t", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCheckFindsNewerRelease(t *testing.T) {
	u := newTestUpdater(t, newRelease(), &restarts{})

	status, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	if !status.Available() || status.Latest.Version != "1.1.0" || status.Latest.Notes != "Nowy wydruk." {
		t.Errorf("status = %+v (latest %+v)", status, status.Latest)
	}
	if !status.Latest.Published.Equal(time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("Published = %v", status.Latest.Published)
	}
	if got := u.Status(); got.CheckedAt.IsZero() || got.Latest == nil {
		t.Errorf("status not remembered: %+v", got)
	}
}

func TestCheckSameVersionIsNotAvailable(t *testing.T) {
	gh := newRelease()
	gh.tag = "v1.0.0"
	u := newTestUpdater(t, gh, &restarts{})

	status, err := u.Check(context.Background())
	if err != nil || status.Available() {
		t.Errorf("status = %+v, err = %v; want no update", status, err)
	}
}

func TestCheckWithoutReleasesOrWithDraft(t *testing.T) {
	for name, gh := range map[string]*fakeGitHub{
		"no releases": {status: http.StatusNotFound},
		"draft":       {tag: "v2.0.0", draft: true},
	} {
		t.Run(name, func(t *testing.T) {
			u := newTestUpdater(t, gh, &restarts{})

			status, err := u.Check(context.Background())
			if err != nil || status.Latest != nil || status.Err != "" {
				t.Errorf("status = %+v, err = %v; want no release and no error", status, err)
			}
		})
	}
}

func TestCheckErrors(t *testing.T) {
	for name, gh := range map[string]*fakeGitHub{
		"server error": {status: http.StatusInternalServerError},
		"bad tag":      {tag: "najnowsza"},
	} {
		t.Run(name, func(t *testing.T) {
			u := newTestUpdater(t, gh, &restarts{})

			status, err := u.Check(context.Background())
			if err == nil || status.Err == "" || status.Available() {
				t.Errorf("status = %+v, err = %v; want an error message", status, err)
			}
		})
	}
}

func TestCheckQuietlyKeepsStatusOnError(t *testing.T) {
	gh := newRelease()
	u := newTestUpdater(t, gh, &restarts{})

	err := u.CheckQuietly(context.Background())
	if err != nil || !u.Status().Available() {
		t.Fatalf("CheckQuietly: %v, status %+v", err, u.Status())
	}

	gh.status = http.StatusBadGateway
	err = u.CheckQuietly(context.Background())
	if err == nil {
		t.Fatal("CheckQuietly ignored an error")
	}
	if status := u.Status(); status.Err != "" || !status.Available() {
		t.Errorf("status after a quiet failure = %+v, want the previous one", status)
	}
}

func TestInstall(t *testing.T) {
	r := &restarts{}
	u := newTestUpdater(t, newRelease(), r)
	_, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	err = u.Install(context.Background(), "1.1.0")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if got := readFile(t, u.ExePath); got != string(newProgram) {
		t.Errorf("program after update = %q", got)
	}
	if got := readFile(t, u.ExePath+".old"); got != "MZ stara wersja" {
		t.Errorf("previous program kept as .old = %q", got)
	}
	if exists(u.ExePath + ".new") {
		t.Error(".new left behind")
	}
	if len(r.calls) != 1 || r.calls[0] != u.ExePath {
		t.Errorf("restart calls = %q", r.calls)
	}

	err = u.Install(context.Background(), "1.1.0")
	if !errors.Is(err, ErrInProgress) {
		t.Errorf("second Install = %v, want ErrInProgress", err)
	}

	err = Cleanup(u.ExePath)
	if err != nil || exists(u.ExePath+".old") {
		t.Errorf("Cleanup: %v, .old exists: %t", err, exists(u.ExePath+".old"))
	}
}

func TestInstallRollsBackWhenNewProgramDoesNotStart(t *testing.T) {
	r := &restarts{err: errors.New("blocked by Smart App Control")}
	u := newTestUpdater(t, newRelease(), r)
	_, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	err = u.Install(context.Background(), "1.1.0")
	if !errors.Is(err, ErrStart) {
		t.Fatalf("Install = %v, want ErrStart", err)
	}

	if got := readFile(t, u.ExePath); got != "MZ stara wersja" {
		t.Errorf("program after rollback = %q, want the previous one", got)
	}
	for _, leftover := range []string{".old", ".new"} {
		if exists(u.ExePath + leftover) {
			t.Errorf("%s left behind after rollback", leftover)
		}
	}

	r.err = nil
	err = u.Install(context.Background(), "1.1.0")
	if err != nil {
		t.Errorf("retry after rollback: %v", err)
	}
}

func TestInstallRejectsBrokenDownloads(t *testing.T) {
	tests := map[string]struct {
		edit func(gh *fakeGitHub)
		want error
	}{
		"wrong checksum":   {func(gh *fakeGitHub) { gh.exe = append([]byte("MZ"), []byte(" podmieniony")...) }, ErrChecksum},
		"not a program":    {func(gh *fakeGitHub) { gh.exe = []byte("<html>"); gh.sums = sumLine(gh.exe, AssetName) }, ErrNotProgram},
		"missing checksum": {func(gh *fakeGitHub) { gh.sums = sumLine(newProgram, "inny.exe") }, ErrMissingAsset},
		"missing assets":   {func(gh *fakeGitHub) { gh.noAssets = true }, ErrMissingAsset},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			gh := newRelease()
			tt.edit(gh)
			r := &restarts{}
			u := newTestUpdater(t, gh, r)
			_, err := u.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}

			err = u.Install(context.Background(), "1.1.0")
			if !errors.Is(err, tt.want) {
				t.Fatalf("Install = %v, want %v", err, tt.want)
			}
			if got := readFile(t, u.ExePath); got != "MZ stara wersja" {
				t.Errorf("program changed after a failed update: %q", got)
			}
			if exists(u.ExePath+".new") || exists(u.ExePath+".old") || len(r.calls) > 0 {
				t.Error("failed update left files behind or restarted the program")
			}
		})
	}
}

func TestInstallNeedsCheckedNewerVersion(t *testing.T) {
	u := newTestUpdater(t, newRelease(), &restarts{})

	err := u.Install(context.Background(), "1.1.0")
	if !errors.Is(err, ErrNotAvailable) {
		t.Errorf("Install before Check = %v, want ErrNotAvailable", err)
	}

	_, err = u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	err = u.Install(context.Background(), "9.9.9")
	if !errors.Is(err, ErrNotAvailable) {
		t.Errorf("Install of another version = %v, want ErrNotAvailable", err)
	}
}

func TestInstallOutsideWindowsProgram(t *testing.T) {
	u := New("1.0.0", "", nil)

	if u.CanInstall() {
		t.Error("an updater without a program path can install")
	}
	if err := u.Install(context.Background(), "1.1.0"); !errors.Is(err, ErrCannotInstall) {
		t.Errorf("Install = %v, want ErrCannotInstall", err)
	}
	if err := Cleanup(""); err != nil {
		t.Errorf("Cleanup(\"\") = %v", err)
	}
}

func TestChecksumFor(t *testing.T) {
	sums := []byte("0123  other\n" + "AB" + string(make([]byte, 0)) + "\n" +
		"aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899 *Intencje-Mszalne.exe\n")

	got, ok := checksumFor(sums, AssetName)
	if !ok || got != "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899" {
		t.Errorf("checksumFor = %q, %t", got, ok)
	}
	if _, ok := checksumFor([]byte("short  Intencje-Mszalne.exe"), AssetName); ok {
		t.Error("a malformed checksum was accepted")
	}
}

func TestStartRunsInBackground(t *testing.T) {
	r := &restarts{}
	u := newTestUpdater(t, newRelease(), r)
	_, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	prepared := make(chan struct{})
	err = u.Start("1.1.0", func() error {
		close(prepared)
		return nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := u.Start("1.1.0", nil); !errors.Is(err, ErrInProgress) {
		t.Errorf("second Start = %v, want ErrInProgress", err)
	}

	<-prepared
	deadline := time.Now().Add(5 * time.Second)
	for u.Progress().Stage != StageRestart && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p := u.Progress(); p.Stage != StageRestart || p.Done != int64(len(newProgram)) || p.Total != p.Done || !p.Active() {
		t.Errorf("progress = %+v", p)
	}
}

func TestStartReportsBackupFailure(t *testing.T) {
	u := newTestUpdater(t, newRelease(), &restarts{})
	_, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	err = u.Start("1.1.0", func() error { return errors.New("disk full") })
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for u.Progress().Stage != StageError && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	p := u.Progress()
	if p.Stage != StageError || p.Err != "Nie udało się zapisać kopii danych przed aktualizacją, więc aktualizacja została przerwana." || p.Active() {
		t.Errorf("progress = %+v", p)
	}
	if got := readFile(t, u.ExePath); got != "MZ stara wersja" {
		t.Errorf("program changed after a failed backup: %q", got)
	}
	if u.Status().Err != p.Err {
		t.Errorf("status error = %q, want the progress error", u.Status().Err)
	}

	err = u.Start("1.1.0", nil)
	if err != nil {
		t.Errorf("Start after a failure: %v", err)
	}
}

func TestMessage(t *testing.T) {
	if got := Message(fmt.Errorf("wrapped: %w", ErrStart)); got != "Nie udało się uruchomić nowej wersji (Windows mógł ją zablokować), więc program został w poprzedniej wersji." {
		t.Errorf("Message(ErrStart) = %q", got)
	}
	if got := Message(os.ErrPermission); got != "Nie udało się zainstalować aktualizacji. Sprawdź połączenie z internetem i spróbuj ponownie – program działa dalej w obecnej wersji." {
		t.Errorf("generic message = %q", got)
	}
}

func TestInstallDownloadServerError(t *testing.T) {
	gh := newRelease()
	srv := gh.server(t)
	r := &restarts{}
	u := newTestUpdater(t, gh, r)
	u.API = srv.URL
	_, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u.mu.Lock()
	u.status.Latest.exeURL = srv.URL + "/brak"
	u.mu.Unlock()

	err = u.Install(context.Background(), "1.1.0")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("Install = %v, want a 404 error", err)
	}
	if got := readFile(t, u.ExePath); got != "MZ stara wersja" || exists(u.ExePath+".new") || len(r.calls) > 0 {
		t.Errorf("failed download changed files or restarted: %q", got)
	}
	if p := u.Progress(); p.Stage != StageError || p.Active() {
		t.Errorf("progress = %+v, want an error", p)
	}
}

func TestSetError(t *testing.T) {
	u := New("1.0.0", "", nil)
	u.SetError("Coś poszło nie tak.")

	if got := u.Status().Err; got != "Coś poszło nie tak." {
		t.Errorf("Err = %q", got)
	}
}
