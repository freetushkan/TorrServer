package settings

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	bolt "go.etcd.io/bbolt"
)

type memDB map[string]map[string][]byte

func (m memDB) CloseDB() {}
func (m memDB) Get(x, n string) []byte {
	return m[x][n]
}

func (m memDB) Set(x, n string, v []byte) {
	// mimic JsonDB, the default route when settings are stored in JSON:
	// it silently drops anything that is not a JSON object
	var obj map[string]any
	if json.Unmarshal(v, &obj) != nil {
		return
	}
	if m[x] == nil {
		m[x] = map[string][]byte{}
	}
	m[x][n] = v
}

func (m memDB) List(x string) []string {
	var r []string
	for k := range m[x] {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
func (m memDB) Rem(x, n string) { delete(m[x], n) }
func (m memDB) Clear(x string)  { delete(m, x) }

func setupPerUser(t *testing.T, enabled bool) memDB {
	t.Helper()
	oldDB, oldSets, oldArgs, oldPath, oldRO := tdb, BTsets, Args, Path, ReadOnly
	t.Cleanup(func() { tdb, BTsets, Args, Path, ReadOnly = oldDB, oldSets, oldArgs, oldPath, oldRO })
	db := memDB{}
	tdb = db
	BTsets = &BTSets{PerUserData: enabled, TrackTimecode: true}
	Args = &ExecArgs{}
	ReadOnly = false
	Path = t.TempDir()
	if err := os.WriteFile(filepath.Join(Path, "accs.db"), []byte(`{"bob":"x","alice":"y"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return db
}

const h1 = "0123456789abcdef0123456789abcdef01234567"

func indexes(list []*Viewed) []int {
	var r []int
	for _, v := range list {
		r = append(r, v.FileIndex)
	}
	sort.Ints(r)
	return r
}

func TestPerUserFlags(t *testing.T) {
	setupPerUser(t, false)
	if PerUserEnabled() || PerUserForced() {
		t.Fatal("should be off")
	}
	Args.PerUserData = true
	if !PerUserEnabled() || !PerUserForced() {
		t.Fatal("flag must force it on")
	}
	if got := ListUsers(); len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("ListUsers = %v", got)
	}
}

func TestViewedSeparatedAndSeeded(t *testing.T) {
	db := setupPerUser(t, true)
	// legacy shared marks exist before the feature was enabled
	SetViewed(&Viewed{Hash: h1, FileIndex: 1})

	// alice sees the shared marks until she has her own
	if got := indexes(ListViewedForUser(h1, "alice")); len(got) != 1 || got[0] != 1 {
		t.Fatalf("alice before = %v", got)
	}
	SetViewedForUser(&Viewed{Hash: h1, FileIndex: 2}, "alice")
	if got := indexes(ListViewedForUser(h1, "alice")); len(got) != 2 {
		t.Fatalf("alice after = %v (seed lost?)", got)
	}
	// bob is unaffected by alice
	if got := indexes(ListViewedForUser(h1, "bob")); len(got) != 1 || got[0] != 1 {
		t.Fatalf("bob = %v", got)
	}
	// shared record untouched
	if got := indexes(ListViewed(h1)); len(got) != 1 {
		t.Fatalf("shared = %v", got)
	}
	// returned hashes never leak the namespace
	for _, v := range ListViewedForUser("", "alice") {
		if v.Hash != h1 {
			t.Fatalf("leaked key %q", v.Hash)
		}
	}
	if len(ListViewedForUser("", "alice")) != 2 {
		t.Fatal("alice should see only her own marks for h1 in the full list")
	}
	// clearing all marks must not resurrect the shared ones
	RemViewedForUser(&Viewed{Hash: h1, FileIndex: -1}, "alice")
	if got := ListViewedForUser(h1, "alice"); len(got) != 0 {
		t.Fatalf("alice after clear = %v", indexes(got))
	}
	if _, ok := db[viewedBucket]["alice:"+h1]; !ok {
		t.Fatal("expected an empty per-user record")
	}
}

func TestViewedDisabledIsUpstreamBehaviour(t *testing.T) {
	db := setupPerUser(t, false)
	SetViewedForUser(&Viewed{Hash: h1, FileIndex: 3}, "alice")
	if _, ok := db[viewedBucket][h1]; !ok || len(db[viewedBucket]) != 1 {
		t.Fatalf("keys = %v", db.List(viewedBucket))
	}
}

func TestViewedKeyForRequest(t *testing.T) {
	setupPerUser(t, true)
	req := httptest.NewRequest("GET", "/", nil)
	if k := ViewedKeyForRequest(h1, req); k != h1 {
		t.Fatalf("anonymous key = %q", k)
	}
	req = req.WithContext(ContextWithUser(req.Context(), "bob"))
	if k := ViewedKeyForRequest(h1, req); k != "bob:"+h1 {
		t.Fatalf("bob key = %q", k)
	}
}

func TestLastUser(t *testing.T) {
	setupPerUser(t, true)
	SetLastUser(h1, "bob", LastUserPreloadAccess)
	SetLastUser(h1, "alice", LastUserViewedAccess)
	SetLastUser(h1, "x", "bogus")
	if GetLastUser(h1, LastUserPreloadAccess) != "bob" || GetLastUser(h1, LastUserViewedAccess) != "alice" {
		t.Fatal("wrong last users")
	}
}

func TestStartupImportsOldOwnersOnce(t *testing.T) {
	db := setupPerUser(t, true)
	h2 := "fedcba9876543210fedcba9876543210fedcba98"
	db.Set("Torrents", h1, []byte(`{"title":"a","users":["alice","bob"]}`))
	db.Set("Torrents", h2, []byte(`{"title":"b"}`))
	db.Set(torrentUsersBucket, "deadbeef", []byte(`{"users":["bob"]}`)) // stale record

	PerUserStartup()
	if got := GetTorrentOwners(h1); len(got) != 2 {
		t.Fatalf("owners h1 = %v", got)
	}
	if got := GetTorrentOwners(h2); got != nil {
		t.Fatalf("owners h2 = %v", got)
	}
	if GetTorrentOwners("deadbeef") != nil {
		t.Fatal("stale owner record not pruned")
	}

	// second run must not re-import after the user changed owners
	SetTorrentOwners(h1, []string{"bob"})
	PerUserStartup()
	if got := GetTorrentOwners(h1); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("owners re-imported: %v", got)
	}
}

func TestOwnersReadLegacyArray(t *testing.T) {
	db := setupPerUser(t, true)
	db[torrentUsersBucket] = map[string][]byte{h1: []byte(`["alice"]`)} // bypasses the object check
	if got := GetTorrentOwners(h1); len(got) != 1 || got[0] != "alice" {
		t.Fatalf("legacy owners = %v", got)
	}
	SetTorrentOwners(h1, []string{"alice", "bob"})
	if got := GetTorrentOwners(h1); len(got) != 2 {
		t.Fatalf("owners after rewrite = %v (not persisted?)", got)
	}
}

func TestImportOwnersFromFile(t *testing.T) {
	db := setupPerUser(t, true)
	h2 := "fedcba9876543210fedcba9876543210fedcba98"
	// current DB: torrents already rewritten by upstream, users field gone
	db.Set("Torrents", h1, []byte(`{"title":"a"}`))
	db.Set("Torrents", h2, []byte(`{"title":"b"}`))
	db.Set(perUserBucket, "owners_migrated", []byte(`{"done":true}`))
	SetTorrentOwners(h2, []string{"bob"}) // already has owners: must stay

	// backup of the old config.db
	old, err := bolt.Open(filepath.Join(Path, PerUserImportFile), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucket([]byte("Torrents"))
		_ = b.Put([]byte(h1), []byte(`{"title":"a","users":["alice"]}`))
		_ = b.Put([]byte(h2), []byte(`{"title":"b","users":["alice"]}`))
		_ = b.Put([]byte("0000000000000000000000000000000000000000"), []byte(`{"users":["alice"]}`)) // deleted since
		return nil
	})
	old.Close()

	PerUserStartup()
	if got := GetTorrentOwners(h1); len(got) != 1 || got[0] != "alice" {
		t.Fatalf("h1 owners = %v", got)
	}
	if got := GetTorrentOwners(h2); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("h2 owners overwritten: %v", got)
	}
	if GetTorrentOwners("0000000000000000000000000000000000000000") != nil {
		t.Fatal("imported owners of a torrent that no longer exists")
	}
	if _, err := os.Stat(filepath.Join(Path, PerUserImportFile+".imported")); err != nil {
		t.Fatal("import file not renamed")
	}
}
