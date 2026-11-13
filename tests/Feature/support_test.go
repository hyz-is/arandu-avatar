package feature_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	hedatabase "github.com/arandu-io/hesape/database"
	sqliteconnector "github.com/arandu-io/hesape/database/connectors/sqlite"
	"github.com/arandu-io/hesape/filesystem"
	hhttp "github.com/arandu-io/hesape/http"
	activitylog "github.com/hyz-is/arandu-activitylog"

	avatar "github.com/hyz-is/arandu-avatar"
	"github.com/hyz-is/arandu-avatar/components"
)

// dsn opens a database file the way the tests need it: the write lock taken when
// a transaction begins, readers beside a writer, and a wait instead of an error
// when the lock is held.
const dsn = "?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(20000)"

// appKey is the key a session store is built over.
const appKey = "0123456789abcdef0123456789abcdef"

// realm is the tenant the platform team signs in to.
const realm = "platform"

// signedIn are the subjects the tests started sessions for: what a session
// store's resolver answers, as an application's reads the account's roles and
// actions again on every request.
var signedIn sync.Map

func resolveSubjects(_ context.Context, who security.Subject) (security.Subject, error) {
	if s, ok := signedIn.Load(who.Tenant + "/" + who.ID); ok {
		return s.(security.Subject), nil
	}
	return who, hhttp.ErrSubjectGone
}

// remember makes a subject the one the resolver answers for its account.
func remember(s security.Subject) security.Subject {
	signedIn.Store(s.Tenant+"/"+s.ID, s)
	return s
}

// person is a user of a space, as the application's directory knows them.
type person struct {
	Name, Email string
}

// directory is the application's Directory: users and organizations by tenant.
type directory struct {
	mu     sync.Mutex
	owners map[string]avatar.Owner // by tenant/kind:id
}

func newDirectory() *directory { return &directory{owners: map[string]avatar.Owner{}} }

func (d *directory) add(tenant string, o avatar.Owner) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.owners[tenant+"/"+o.Ref.String()] = o
}

func (d *directory) Owner(_ context.Context, g security.Grant, ref avatar.Ref) (avatar.Owner, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if o, ok := d.owners[data.Tenant(g)+"/"+ref.String()]; ok {
		return o, nil
	}
	return avatar.Owner{}, avatar.ErrUnknownOwner
}

// gravatar is a Gravatar that answers from a table of digests, and remembers
// what it was asked.
type gravatar struct {
	*httptest.Server
	mu       sync.Mutex
	pictures map[string][]byte // by digest
	asked    []string          // the request URIs
	status   int               // when not 0, every answer
}

func newGravatar(t *testing.T) *gravatar {
	t.Helper()
	g := &gravatar{pictures: map[string][]byte{}}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.asked = append(g.asked, r.URL.RequestURI())
		if g.status != 0 {
			w.WriteHeader(g.status)
			return
		}
		digest := strings.TrimPrefix(r.URL.Path, "/avatar/")
		body, ok := g.pictures[digest]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(body)
	}))
	t.Cleanup(g.Close)
	return g
}

func (g *gravatar) requests() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.asked...)
}

// world is one application with this package installed over a real database.
type world struct {
	sql      *sql.DB
	conn     *hedatabase.Connection
	db       *data.DB
	sessions *security.SessionStore
	module   *avatar.Module
	avatars  *avatar.AvatarService
	team     *avatar.TeamService
	router   *fhttp.Router
	disk     *filesystem.Disk
	dir      *directory
	gravatar *gravatar
	logs     *syncBuffer
	clock    *clock
}

// clock is a time the tests move by hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Second)
	return c.now
}

func install(t *testing.T) world { t.Helper(); return installWith(t, nil) }

// installWith is install with the module's configuration changed first.
func installWith(t *testing.T, change func(*avatar.Config)) world {
	t.Helper()

	path := filepath.Join(t.TempDir(), "avatar.sqlite")
	handle, err := sql.Open(sqliteconnector.SQLiteConnector{}.DriverName(), "file:"+path+dsn)
	if err != nil {
		t.Fatalf("opening SQLite: %v", err)
	}
	handle.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = handle.Close() })

	connection := hedatabase.NewConnection(handle, path, "", map[string]any{"driver": string(hedatabase.DialectSQLite), "name": "avatar-test"})
	db := data.Wrap(handle, data.DialectSQLite)
	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
	sessions.ResolveSubjects(resolveSubjects)

	activity, err := activitylog.New(activitylog.Config{Tenant: realm}, db, sessions)
	if err != nil {
		t.Fatalf("building arandu-activitylog: %v", err)
	}
	adapter, err := filesystem.NewLocalFilesystemAdapter(t.TempDir())
	if err != nil {
		t.Fatalf("building a local Disk: %v", err)
	}
	disk := filesystem.NewDisk("avatars", adapter)

	g := newGravatar(t)
	dir := newDirectory()
	logs := &syncBuffer{}
	held := &clock{now: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)}
	cfg := avatar.Config{
		TeamTenant:  realm,
		Directory:   dir,
		Files:       disk,
		Activity:    activity.Logger(),
		Render:      components.Render,
		GravatarURL: g.URL + "/avatar",
		Now:         held.Now,
		Logger:      slog.New(slog.NewTextHandler(logs, nil)),
	}
	if change != nil {
		change(&cfg)
	}
	module, err := avatar.New(cfg, db, sessions)
	if err != nil {
		t.Fatalf("building the module: %v", err)
	}
	migrationsConn := hedatabase.ForMigrations(connection)
	for _, migration := range append(activity.Migrations(), module.Migrations()...) {
		if err := migration.Up(context.Background(), migrationsConn); err != nil {
			t.Fatalf("applying %s: %v", migration.GetName(), err)
		}
	}
	router := fhttp.NewRouter().WithFlash(security.NewFlash([]byte(appKey), false))
	module.Routes(router.ForModule(module.Name()))
	return world{
		sql: handle, conn: connection, db: db, sessions: sessions, module: module, avatars: module.Avatars(), team: module.Team(),
		router: router, disk: disk, dir: dir, gravatar: g, logs: logs, clock: held,
	}
}

// The subjects of the tests. A member holds no action: a person changes their
// own picture without a group. A manager also holds the right to change the logo
// of an organization, and an agent is a member of the platform team.

func member(tenant, id string) security.Subject {
	return remember(security.Subject{ID: id, Tenant: tenant, Verified: true})
}

func manager(tenant, id string) security.Subject {
	return remember(security.Subject{ID: id, Tenant: tenant, Verified: true, Actions: avatar.Actions()})
}

func agent(id string) security.Subject {
	return remember(security.Subject{ID: id, Tenant: realm, Verified: true, Actions: avatar.TeamActions()})
}

// enroll puts a person and an organization in the directory of a space.
func (w world) enroll(tenant, id, name, email string) avatar.Owner {
	o := avatar.Owner{Ref: avatar.User(id), Name: name, Email: email}
	w.dir.add(tenant, o)
	return o
}

// jpegBytes is a photo of w by h, red on the left and blue on the right.
func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.Set(x, y, color.RGBA{R: 220, A: 255})
			} else {
				img.Set(x, y, color.RGBA{B: 220, A: 255})
			}
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// requireErr fails unless err wraps want.
func requireErr(t *testing.T, err, want error, what string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: got %v, want %v", what, err, want)
	}
}

// entry is a row of the activity log, as the tests read it.
type entry struct{ Event, Description, Causer, Subject, Properties string }

// entries are the rows the log holds for a tenant, oldest first.
func entries(t *testing.T, w world, tenant string) []entry {
	t.Helper()
	rows, err := w.sql.Query(`SELECT COALESCE(event, ''), description, COALESCE(causer_id, ''), COALESCE(subject_id, ''), COALESCE(properties, '')
		FROM activity_log WHERE tenant_id = ? ORDER BY created_at, id`, tenant)
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	defer rows.Close()
	var out []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.Event, &e.Description, &e.Causer, &e.Subject, &e.Properties); err != nil {
			t.Fatalf("reading a log row: %v", err)
		}
		out = append(out, e)
	}
	return out
}

func events(t *testing.T, w world, tenant string) []string {
	t.Helper()
	var out []string
	for _, e := range entries(t, w, tenant) {
		out = append(out, e.Event)
	}
	return out
}

// syncBuffer is a log the tests read what the package could not return.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// --- driving the routes ---

// signIn starts a session for the subject and returns the cookies a browser
// would send back.
func (w world) signIn(t *testing.T, s security.Subject) []*http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	if _, err := w.sessions.Start(context.Background(), rec, security.Subject{ID: s.ID, Tenant: s.Tenant}); err != nil {
		t.Fatalf("starting a session: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("starting a session wrote no cookie")
	}
	return cookies
}

// request is one request against the router.
type request struct {
	method, target string
	header         http.Header
	body           []byte
	contentType    string
	cookies        []*http.Cookie
}

func (w world) do(t *testing.T, r request) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(r.method, r.target, bytes.NewReader(r.body))
	if r.contentType != "" {
		req.Header.Set("Content-Type", r.contentType)
	}
	for name, values := range r.header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	for _, c := range r.cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("%s %s panicked: %v", r.method, r.target, recovered)
		}
	}()
	w.router.ServeHTTP(rec, req)
	return rec
}

// multipartBody is a form with one file and some fields.
func multipartBody(t *testing.T, field, filename string, file []byte, fields map[string]string) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for name, value := range fields {
		if err := mw.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if file != nil {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
		h.Set("Content-Type", "image/jpeg")
		part, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(file)
	}
	_ = mw.Close()
	return b.Bytes(), mw.FormDataContentType()
}

// transparentPNG is a logo: a red square on nothing.
func transparentPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	for y := 40; y < 120; y++ {
		for x := 40; x < 120; x++ {
			img.Set(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
