package update

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultAPI    = "https://api.github.com/repos/tomkalon/Parish-Mass-Intentions"
	AssetName     = "Intencje-Mszalne.exe"
	ChecksumsName = "SHA256SUMS.txt"

	maxExeSize       = 200 << 20
	maxChecksumsSize = 64 << 10
	maxReleaseSize   = 1 << 20
)

var (
	ErrCannotInstall = errors.New("aktualizację można zainstalować tylko w programie dla Windows")
	ErrNotAvailable  = errors.New("ta wersja nie jest już dostępna – sprawdź aktualizacje jeszcze raz")
	ErrInProgress    = errors.New("aktualizacja już trwa")
	ErrMissingAsset  = errors.New("w wydaniu brakuje pliku programu albo sumy kontrolnej")
	ErrChecksum      = errors.New("pobrany plik jest uszkodzony (niezgodna suma kontrolna)")
	ErrNotProgram    = errors.New("pobrany plik nie jest programem dla Windows")
	ErrStart         = errors.New("nie udało się uruchomić nowej wersji (Windows mógł ją zablokować), więc program został w poprzedniej wersji")
)

type Release struct {
	Version   string
	Name      string
	Notes     string
	Published time.Time

	exeURL  string
	sumsURL string
}

type Status struct {
	Current   string
	Latest    *Release
	CheckedAt time.Time
	Err       string
}

func (s Status) Available() bool {
	return s.Latest != nil && Newer(s.Latest.Version, s.Current)
}

type Updater struct {
	Current string
	API     string
	Client  *http.Client
	ExePath string
	Restart func(exePath string) error

	now        func() time.Time
	mu         sync.Mutex
	status     Status
	installing bool
	progress   Progress
}

func New(current, exePath string, restart func(exePath string) error) *Updater {
	return &Updater{
		Current: current,
		API:     DefaultAPI,
		Client:  &http.Client{Timeout: 3 * time.Minute},
		ExePath: exePath,
		Restart: restart,
		now:     time.Now,
		status:  Status{Current: current},
	}
}

func (u *Updater) CanInstall() bool {
	return u.ExePath != "" && u.Restart != nil
}

func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.status
}

func (u *Updater) Check(ctx context.Context) (Status, error) {
	return u.check(ctx, true)
}

// CheckQuietly is Check for the automatic check at startup: a failure
// (e.g. no internet) leaves the previous status untouched.
func (u *Updater) CheckQuietly(ctx context.Context) error {
	_, err := u.check(ctx, false)
	return err
}

func (u *Updater) check(ctx context.Context, report bool) (Status, error) {
	release, err := u.latest(ctx)

	u.mu.Lock()
	defer u.mu.Unlock()

	if err != nil && !report {
		return u.status, err
	}

	u.status = Status{Current: u.Current, Latest: release, CheckedAt: u.now()}
	if err != nil {
		u.status.Err = "Nie udało się sprawdzić aktualizacji. Sprawdź połączenie z internetem."
	}

	return u.status, err
}

func (u *Updater) SetError(message string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.status.Err = message
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (u *Updater) latest(ctx context.Context) (*Release, error) {
	body, status, err := u.get(ctx, u.API+"/releases/latest", "application/vnd.github+json", maxReleaseSize)
	if status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var gh githubRelease
	err = json.Unmarshal(body, &gh)
	if err != nil {
		return nil, fmt.Errorf("reading release: %w", err)
	}
	if gh.Draft || gh.Prerelease {
		return nil, nil
	}
	if _, ok := parseVersion(gh.TagName); !ok {
		return nil, fmt.Errorf("release tag %q is not a version", gh.TagName)
	}

	release := &Release{
		Version:   strings.TrimPrefix(gh.TagName, "v"),
		Name:      gh.Name,
		Notes:     strings.TrimSpace(gh.Body),
		Published: gh.PublishedAt,
	}
	for _, asset := range gh.Assets {
		switch asset.Name {
		case AssetName:
			release.exeURL = asset.URL
		case ChecksumsName:
			release.sumsURL = asset.URL
		}
	}

	return release, nil
}

func (u *Updater) get(ctx context.Context, url, accept string, limit int64) ([]byte, int, error) {
	resp, err := u.open(ctx, url, accept)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if int64(len(body)) > limit {
		return nil, resp.StatusCode, fmt.Errorf("GET %s: response larger than %d bytes", url, limit)
	}

	return body, resp.StatusCode, nil
}

func (u *Updater) open(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "go-parafia/"+u.Current)

	return u.Client.Do(req)
}

// Install downloads the given version next to the running program, checks
// it, swaps the files (the running exe can be renamed on Windows, not
// overwritten) and starts the new program. If the new program cannot be
// started, the previous one is put back.
func (u *Updater) Install(ctx context.Context, version string) error {
	release, err := u.begin(version)
	if err != nil {
		return err
	}

	err = u.run(ctx, release, nil)
	if err != nil {
		u.fail(err)
	}

	return err
}

func (u *Updater) begin(version string) (Release, error) {
	if !u.CanInstall() {
		return Release{}, ErrCannotInstall
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	release := u.status.Latest
	switch {
	case u.installing:
		return Release{}, ErrInProgress
	case release == nil || release.Version != version || !Newer(version, u.Current):
		return Release{}, ErrNotAvailable
	}
	u.installing = true
	u.progress = Progress{Version: version, Stage: StageBackup}

	return *release, nil
}

func (u *Updater) run(ctx context.Context, release Release, prepare func() error) error {
	if prepare != nil {
		err := prepare()
		if err != nil {
			return fmt.Errorf("%w: %w", ErrBackup, err)
		}
	}

	return u.install(ctx, release)
}

func (u *Updater) fail(err error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.installing = false
	u.progress.Stage = StageError
	u.progress.Err = Message(err)
	u.status.Err = u.progress.Err
}

func (u *Updater) install(ctx context.Context, release Release) error {
	u.setStage(StageDownload)
	if release.exeURL == "" || release.sumsURL == "" {
		return ErrMissingAsset
	}

	sums, _, err := u.get(ctx, release.sumsURL, "application/octet-stream", maxChecksumsSize)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	want, ok := checksumFor(sums, AssetName)
	if !ok {
		return ErrMissingAsset
	}

	newPath := u.ExePath + ".new"
	err = u.download(ctx, release.exeURL, newPath, want)
	if err != nil {
		return errors.Join(err, removeIfExists(newPath))
	}

	u.setStage(StageInstall)
	oldPath := u.ExePath + ".old"
	err = removeIfExists(oldPath)
	if err != nil {
		return errors.Join(fmt.Errorf("removing previous backup: %w", err), removeIfExists(newPath))
	}
	err = os.Rename(u.ExePath, oldPath)
	if err != nil {
		return errors.Join(fmt.Errorf("moving the current program aside: %w", err), removeIfExists(newPath))
	}
	err = os.Rename(newPath, u.ExePath)
	if err != nil {
		return errors.Join(fmt.Errorf("putting the new program in place: %w", err), os.Rename(oldPath, u.ExePath))
	}

	u.setStage(StageRestart)
	err = u.Restart(u.ExePath)
	if err != nil {
		rollback := errors.Join(os.Rename(u.ExePath, newPath), os.Rename(oldPath, u.ExePath), removeIfExists(newPath))
		return errors.Join(fmt.Errorf("%w: %w", ErrStart, err), rollback)
	}

	return nil
}

func (u *Updater) download(ctx context.Context, url, path, wantSum string) error {
	resp, err := u.open(ctx, url, "application/octet-stream")
	if err != nil {
		return fmt.Errorf("downloading the program: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading the program: %s", resp.Status)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}

	u.setDownloadTotal(resp.ContentLength)
	hash := sha256.New()
	head := &prefix{size: 2}
	n, err := io.Copy(io.MultiWriter(f, hash, head, progressWriter{u}), io.LimitReader(resp.Body, maxExeSize+1))
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	u.setStage(StageVerify)
	switch {
	case err != nil:
		return fmt.Errorf("saving the program: %w", err)
	case n > maxExeSize:
		return fmt.Errorf("downloading the program: larger than %d bytes", maxExeSize)
	case hex.EncodeToString(hash.Sum(nil)) != wantSum:
		return ErrChecksum
	case !bytes.Equal(head.data, []byte("MZ")):
		return ErrNotProgram
	}

	return nil
}

type prefix struct {
	size int
	data []byte
}

func (p *prefix) Write(b []byte) (int, error) {
	if missing := p.size - len(p.data); missing > 0 {
		p.data = append(p.data, b[:min(missing, len(b))]...)
	}

	return len(b), nil
}

func checksumFor(sums []byte, name string) (string, bool) {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name && len(fields[0]) == sha256.Size*2 {
			return strings.ToLower(fields[0]), true
		}
	}

	return "", false
}

// Cleanup removes the files left by an update once the new program runs.
func Cleanup(exePath string) error {
	if exePath == "" {
		return nil
	}

	return errors.Join(removeIfExists(exePath+".old"), removeIfExists(exePath+".new"))
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func Newer(a, b string) bool {
	va, okA := parseVersion(a)
	vb, okB := parseVersion(b)
	if !okA {
		return false
	}
	if !okB {
		return true
	}

	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}

	return false
}

func parseVersion(text string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(strings.TrimPrefix(text, "v"), ".")
	if len(parts) != 3 {
		return v, false
	}

	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}

	return v, true
}
