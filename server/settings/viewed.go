package settings

import (
	"encoding/json"
	"strings"

	"server/log"
)

type Viewed struct {
	Hash      string `json:"hash"`
	FileIndex int    `json:"file_index"`
	User      string `json:"user,omitempty"`
}

func viewedKey(hash, user string) string {
	if !PerUserData || user == "" {
		return hash
	}
	return user + ":" + hash
}

func parseViewedKey(key string) (user, hash string) {
	if !PerUserData {
		return "", key
	}
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}

func splitViewedKey(key string) (user, hash string) {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}

func readViewedIndexes(key string) map[int]struct{} {
	buf := tdb.Get("Viewed", key)
	if len(buf) == 0 {
		return nil
	}
	var indexes map[int]struct{}
	if err := json.Unmarshal(buf, &indexes); err != nil {
		return nil
	}
	return indexes
}

// CopyViewedToUsers copies legacy viewed entries (hash key) to per-user keys.
func CopyViewedToUsers(hash string, users []string) {
	if !PerUserData || hash == "" || len(users) == 0 {
		return
	}
	buf := tdb.Get("Viewed", hash)
	if len(buf) == 0 {
		return
	}
	var indexes map[int]struct{}
	if err := json.Unmarshal(buf, &indexes); err != nil {
		return
	}
	for _, user := range users {
		if user == "" {
			continue
		}
		key := viewedKey(hash, user)
		if len(tdb.Get("Viewed", key)) == 0 {
			tdb.Set("Viewed", key, buf)
		}
	}
}

func SetViewed(vv *Viewed) {
	if vv == nil || vv.Hash == "" {
		return
	}
	var indexes map[int]struct{}
	var err error

	key := viewedKey(vv.Hash, vv.User)
	buf := tdb.Get("Viewed", key)
	if len(buf) == 0 {
		indexes = make(map[int]struct{})
		indexes[vv.FileIndex] = struct{}{}
		buf, err = json.Marshal(indexes)
		if err == nil {
			tdb.Set("Viewed", key, buf)
		}
	} else {
		err = json.Unmarshal(buf, &indexes)
		if err == nil {
			indexes[vv.FileIndex] = struct{}{}
			buf, err = json.Marshal(indexes)
			if err == nil {
				tdb.Set("Viewed", key, buf)
			}
		}
	}
	if err != nil {
		log.TLogln("Error set viewed:", err)
	}
}

func RemViewed(vv *Viewed) {
	if vv == nil || vv.Hash == "" {
		return
	}
	key := viewedKey(vv.Hash, vv.User)
	buf := tdb.Get("Viewed", key)
	if len(buf) == 0 {
		return
	}
	var indeces map[int]struct{}
	err := json.Unmarshal(buf, &indeces)
	if err == nil {
		if vv.FileIndex != -1 {
			delete(indeces, vv.FileIndex)
			buf, err = json.Marshal(indeces)
			if err == nil {
				tdb.Set("Viewed", key, buf)
			}
		} else {
			tdb.Rem("Viewed", key)
		}
	}
	if err != nil {
		log.TLogln("Error rem viewed:", err)
	}
}

func ListViewed(hash string) []*Viewed {
	return ListViewedForUser(hash, "")
}

func ListViewedForUser(hash, user string) []*Viewed {
	var err error
	if hash != "" {
		if !PerUserData {
			merged := make(map[int]struct{})
			for i := range readViewedIndexes(hash) {
				merged[i] = struct{}{}
			}
			for _, key := range tdb.List("Viewed") {
				if strings.HasSuffix(key, ":"+hash) {
					for i := range readViewedIndexes(key) {
						merged[i] = struct{}{}
					}
				}
			}
			ret := make([]*Viewed, 0, len(merged))
			for i := range merged {
				ret = append(ret, &Viewed{Hash: hash, FileIndex: i})
			}
			return ret
		}

		key := viewedKey(hash, user)
		buf := tdb.Get("Viewed", key)
		if len(buf) == 0 {
			return []*Viewed{}
		}
		var indeces map[int]struct{}
		err = json.Unmarshal(buf, &indeces)
		if err == nil {
			var ret []*Viewed
			for i := range indeces {
				ret = append(ret, &Viewed{Hash: hash, FileIndex: i, User: user})
			}
			return ret
		}
	} else {
		if !PerUserData {
			grouped := make(map[string]map[int]struct{})
			keys := tdb.List("Viewed")
			for _, key := range keys {
				_, keyHash := splitViewedKey(key)
				indexes := readViewedIndexes(key)
				if indexes == nil {
					continue
				}
				if _, ok := grouped[keyHash]; !ok {
					grouped[keyHash] = make(map[int]struct{})
				}
				for i := range indexes {
					grouped[keyHash][i] = struct{}{}
				}
			}
			var ret []*Viewed
			for keyHash, indexes := range grouped {
				for i := range indexes {
					ret = append(ret, &Viewed{Hash: keyHash, FileIndex: i})
				}
			}
			return ret
		}

		var ret []*Viewed
		keys := tdb.List("Viewed")
		for _, key := range keys {
			keyUser, keyHash := parseViewedKey(key)
			if PerUserData && user != "" && keyUser != user {
				continue
			}
			buf := tdb.Get("Viewed", key)
			if len(buf) == 0 {
				return []*Viewed{}
			}
			var indeces map[int]struct{}
			err = json.Unmarshal(buf, &indeces)
			if err == nil {
				for i := range indeces {
					ret = append(ret, &Viewed{Hash: keyHash, FileIndex: i, User: keyUser})
				}
			}
		}
		return ret
	}

	log.TLogln("Error list viewed:", err)
	return []*Viewed{}
}
