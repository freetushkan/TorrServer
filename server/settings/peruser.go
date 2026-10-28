package settings

// Per-user data separation (fork feature, not in upstream).
//
// Everything the feature needs from the settings package lives in this file
// so that rebasing onto new upstream tags only has to deal with a handful of
// one-line hooks. See docs/fork/PER_USER.md for the list of hook points.
//
// Storage layout (compatible with the previous implementation):
//   - "Viewed"       key "<user>:<hash>" — per-user viewed marks; plain "<hash>" is shared
//   - "LastUser"     key "<hash>"        — last user that listed viewed / preloaded the torrent
//   - "TorrentUsers" key "<hash>"        — {"users":[...]} accounts that own the torrent
//   - "PerUser"      key "owners_migrated" — marker of the one-time import of the old
//     TorrentDB.users field into "TorrentUsers"

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"server/log"
)

const (
	LastUserViewedAccess  = "viewed_access"
	LastUserPreloadAccess = "preload_access"

	torrentUsersBucket = "TorrentUsers"
	perUserBucket      = "PerUser"
	lastUserBucket     = "LastUser"
	viewedBucket       = "Viewed"
)

// PerUserEnabled reports whether torrents and viewed marks are separated per account.
// The --peruserdata flag forces it on regardless of the stored setting.
func PerUserEnabled() bool {
	if Args != nil && Args.PerUserData {
		return true
	}
	return BTsets != nil && BTsets.PerUserData
}

// PerUserForced reports whether the feature is forced on by the command line flag.
func PerUserForced() bool {
	return Args != nil && Args.PerUserData
}

// ListUsers returns the sorted account names from accs.db.
func ListUsers() []string {
	buf, err := os.ReadFile(filepath.Join(Path, "accs.db"))
	if err != nil {
		return []string{}
	}
	accs := make(map[string]string)
	if err := json.Unmarshal(buf, &accs); err != nil {
		return []string{}
	}
	users := make([]string, 0, len(accs))
	for user := range accs {
		users = append(users, user)
	}
	sort.Strings(users)
	return users
}

// ---------------------------------------------------------------------------
// Request user: carried in the request context so deep code (torr.Stream)
// does not need its signature changed.

type requestUserKey struct{}

func ContextWithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, requestUserKey{}, user)
}

func UserFromRequest(req *http.Request) string {
	if req == nil {
		return ""
	}
	user, _ := req.Context().Value(requestUserKey{}).(string)
	return user
}

// ---------------------------------------------------------------------------
// Viewed marks

// ViewedKey returns the "Viewed" bucket key for hash as seen by user.
func ViewedKey(hash, user string) string {
	if !PerUserEnabled() || user == "" {
		return hash
	}
	return user + ":" + hash
}

// ViewedKeyForRequest is ViewedKey for the user carried by req.
// It seeds the user's marks from the shared ones on first use.
func ViewedKeyForRequest(hash string, req *http.Request) string {
	user := UserFromRequest(req)
	seedUserViewed(hash, user)
	return ViewedKey(hash, user)
}

func splitViewedKey(key string) (user, hash string) {
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

// seedUserViewed copies the shared marks of hash into the user's namespace
// if the user has none yet, so enabling the feature does not lose history.
func seedUserViewed(hash, user string) {
	if !PerUserEnabled() || user == "" || hash == "" {
		return
	}
	key := ViewedKey(hash, user)
	if len(tdb.Get(viewedBucket, key)) != 0 {
		return
	}
	if buf := tdb.Get(viewedBucket, hash); len(buf) != 0 {
		tdb.Set(viewedBucket, key, buf)
	}
}

func SetViewedForUser(vv *Viewed, user string) {
	if vv == nil || vv.Hash == "" {
		return
	}
	seedUserViewed(vv.Hash, user)
	v := *vv
	v.Hash = ViewedKey(vv.Hash, user)
	SetViewed(&v)
}

func RemViewedForUser(vv *Viewed, user string) {
	if vv == nil || vv.Hash == "" {
		return
	}
	seedUserViewed(vv.Hash, user)
	v := *vv
	v.Hash = ViewedKey(vv.Hash, user)
	if v.Hash != vv.Hash && v.FileIndex == -1 {
		// keep an empty record so the shared marks do not show through again
		tdb.Set(viewedBucket, v.Hash, []byte("{}"))
		return
	}
	RemViewed(&v)
}

// ListViewedForUser lists viewed marks of user. With hash == "" it lists all
// torrents: the user's own marks, or every entry when there is no user.
func ListViewedForUser(hash, user string) []*Viewed {
	perUser := PerUserEnabled() && user != ""
	if hash != "" {
		key := hash
		if perUser {
			key = ViewedKey(hash, user)
			if len(tdb.Get(viewedBucket, key)) == 0 {
				key = hash // not seeded yet: show the shared marks
			}
		}
		list := ListViewed(key)
		for _, v := range list {
			v.Hash = hash
		}
		return list
	}

	all := ListViewed("")
	if !perUser {
		for _, v := range all {
			_, v.Hash = splitViewedKey(v.Hash)
		}
		return all
	}
	own := map[string]bool{}
	for _, v := range all {
		if u, h := splitViewedKey(v.Hash); u == user {
			own[h] = true
		}
	}
	ret := make([]*Viewed, 0, len(all))
	for _, v := range all {
		u, h := splitViewedKey(v.Hash)
		if u == user || (u == "" && !own[h]) {
			v.Hash = h
			ret = append(ret, v)
		}
	}
	return ret
}

// ---------------------------------------------------------------------------
// Last user (fallback for unauthenticated stream/playlist links)

type lastUserData struct {
	Hash          string `json:"hash"`
	ViewedAccess  string `json:"viewed_access,omitempty"`
	PreloadAccess string `json:"preload_access,omitempty"`
}

func SetLastUser(hash, user, access string) {
	if hash == "" || user == "" || !PerUserEnabled() {
		return
	}
	if access != LastUserViewedAccess && access != LastUserPreloadAccess {
		return
	}
	var lu lastUserData
	if buf := tdb.Get(lastUserBucket, hash); len(buf) != 0 {
		if err := json.Unmarshal(buf, &lu); err != nil {
			log.TLogln("Error set last user:", err)
			return
		}
	}
	lu.Hash = hash
	if access == LastUserViewedAccess {
		if lu.ViewedAccess == user {
			return
		}
		lu.ViewedAccess = user
	} else {
		if lu.PreloadAccess == user {
			return
		}
		lu.PreloadAccess = user
	}
	buf, err := json.Marshal(&lu)
	if err != nil {
		log.TLogln("Error set last user:", err)
		return
	}
	tdb.Set(lastUserBucket, hash, buf)
}

func GetLastUser(hash, access string) string {
	if hash == "" {
		return ""
	}
	buf := tdb.Get(lastUserBucket, hash)
	if len(buf) == 0 {
		return ""
	}
	var lu lastUserData
	if err := json.Unmarshal(buf, &lu); err != nil {
		log.TLogln("Error get last user:", err)
		return ""
	}
	switch access {
	case LastUserViewedAccess:
		return lu.ViewedAccess
	case LastUserPreloadAccess:
		return lu.PreloadAccess
	}
	return ""
}

// ---------------------------------------------------------------------------
// Torrent owners
//
// Values are JSON objects, not arrays: buckets without a route of their own go
// to the default DB, which is the JSON one when settings are stored in JSON,
// and JsonDB only stores objects.

type ownersRecord struct {
	Users []string `json:"users"`
}

// GetTorrentOwners returns the stored owners of hash (may contain deleted accounts).
func GetTorrentOwners(hash string) []string {
	buf := tdb.Get(torrentUsersBucket, hash)
	if len(buf) == 0 {
		return nil
	}
	var rec ownersRecord
	if err := json.Unmarshal(buf, &rec); err == nil {
		return rec.Users
	}
	var users []string // written by early builds
	if err := json.Unmarshal(buf, &users); err != nil {
		log.TLogln("Error get torrent users:", err)
		return nil
	}
	return users
}

func SetTorrentOwners(hash string, users []string) {
	if hash == "" {
		return
	}
	if len(users) == 0 {
		RemTorrentOwners(hash)
		return
	}
	if slices.Equal(GetTorrentOwners(hash), users) {
		return
	}
	buf, err := json.Marshal(ownersRecord{Users: users})
	if err != nil {
		log.TLogln("Error set torrent users:", err)
		return
	}
	tdb.Set(torrentUsersBucket, hash, buf)
}

func RemTorrentOwners(hash string) {
	if len(tdb.Get(torrentUsersBucket, hash)) != 0 {
		tdb.Rem(torrentUsersBucket, hash)
	}
}

// legacyOwners extracts TorrentDB.users values from a "Torrents" record.
func legacyOwners(buf []byte) []string {
	var old struct {
		Users []string `json:"users"`
	}
	if json.Unmarshal(buf, &old) != nil {
		return nil
	}
	return old.Users
}

// PerUserImportFile is a copy of config.db from a build that kept owners in
// TorrentDB.users. If it is present at startup, owners are imported from it
// for torrents that have none, and the file is renamed to *.imported.
const PerUserImportFile = "peruser-import.db"

func importOwnersFromFile(saved map[string]bool) int {
	path := filepath.Join(Path, PerUserImportFile)
	if _, err := os.Stat(path); err != nil {
		return 0
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: 5 * time.Second})
	if err != nil {
		log.TLogln("Per-user data: cannot open", path, err)
		return 0
	}
	imported := 0
	_ = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Torrents"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			hash := string(k)
			if users := legacyOwners(v); saved[hash] && len(users) != 0 && len(GetTorrentOwners(hash)) == 0 {
				SetTorrentOwners(hash, users)
				imported++
			}
			return nil
		})
	})
	db.Close()
	if err := os.Rename(path, path+".imported"); err != nil {
		log.TLogln("Per-user data: cannot rename", path, err)
	}
	log.TLogln("Per-user data: imported owners of", imported, "torrents from", PerUserImportFile)
	return imported
}

// PerUserStartup must run once after InitSets and before anything rewrites the
// "Torrents" bucket: it imports owners stored by the previous implementation
// in TorrentDB.users and drops owner records of torrents no longer in the DB.
func PerUserStartup() {
	if tdb == nil || ReadOnly {
		return
	}
	saved := map[string]bool{}
	migrated := len(tdb.Get(perUserBucket, "owners_migrated")) != 0
	imported := 0
	for _, key := range tdb.List("Torrents") {
		saved[key] = true
		if migrated {
			continue
		}
		if users := legacyOwners(tdb.Get("Torrents", key)); len(users) != 0 && len(GetTorrentOwners(key)) == 0 {
			SetTorrentOwners(key, users)
			imported++
		}
	}
	if !migrated {
		tdb.Set(perUserBucket, "owners_migrated", []byte(`{"done":true}`))
		if imported > 0 {
			log.TLogln("Per-user data: imported owners of", imported, "torrents")
		}
	}
	importOwnersFromFile(saved)
	for _, key := range tdb.List(torrentUsersBucket) {
		if !saved[key] {
			tdb.Rem(torrentUsersBucket, key)
		}
	}
}
