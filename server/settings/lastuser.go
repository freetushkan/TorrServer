package settings

import (
	"encoding/json"

	"server/log"
)

const (
	LastUserViewedAccess  = "viewed_access"
	LastUserPreloadAccess = "preload_access"
)

type LastUserData struct {
	Hash          string `json:"hash"`
	ViewedAccess  string `json:"viewed_access,omitempty"`
	PreloadAccess string `json:"preload_access,omitempty"`
}

func lastUserKey(hash string) string {
	return hash
}

func normalizeLastUserAccess(access string) string {
	switch access {
	case LastUserViewedAccess, LastUserPreloadAccess:
		return access
	default:
		return ""
	}
}

func (lu *LastUserData) getUser(access string) string {
	switch normalizeLastUserAccess(access) {
	case LastUserViewedAccess:
		return lu.ViewedAccess
	case LastUserPreloadAccess:
		return lu.PreloadAccess
	default:
		return ""
	}
}

func (lu *LastUserData) setUser(access, user string) {
	switch normalizeLastUserAccess(access) {
	case LastUserViewedAccess:
		lu.ViewedAccess = user
	case LastUserPreloadAccess:
		lu.PreloadAccess = user
	}
}

func (lu *LastUserData) removeUser(access string) {
	switch normalizeLastUserAccess(access) {
	case LastUserViewedAccess:
		lu.ViewedAccess = ""
	case LastUserPreloadAccess:
		lu.PreloadAccess = ""
	}
}

func SetLastUser(hash, user, access string) {
	if hash == "" || user == "" {
		return
	}
	access = normalizeLastUserAccess(access)
	if access == "" {
		return
	}

	key := lastUserKey(hash)
	var lu LastUserData
	buf := tdb.Get("LastUser", key)
	if len(buf) != 0 {
		if err := json.Unmarshal(buf, &lu); err != nil {
			log.TLogln("Error set last user:", err)
			return
		}
	}

	lu.Hash = hash
	lu.setUser(access, user)

	buf, err := json.Marshal(&lu)
	if err != nil {
		log.TLogln("Error set last user:", err)
		return
	}
	tdb.Set("LastUser", key, buf)
}

func GetLastUser(hash, access string) string {
	if hash == "" {
		return ""
	}
	access = normalizeLastUserAccess(access)
	if access == "" {
		return ""
	}

	buf := tdb.Get("LastUser", lastUserKey(hash))
	if len(buf) == 0 {
		return ""
	}

	var lu LastUserData
	if err := json.Unmarshal(buf, &lu); err != nil {
		log.TLogln("Error get last user:", err)
		return ""
	}
	return lu.getUser(access)
}

func RemLastUser(hash, access string) {
	if hash == "" {
		return
	}
	access = normalizeLastUserAccess(access)
	if access == "" {
		return
	}

	key := lastUserKey(hash)
	buf := tdb.Get("LastUser", key)
	if len(buf) == 0 {
		return
	}

	var lu LastUserData
	if err := json.Unmarshal(buf, &lu); err != nil {
		log.TLogln("Error rem last user:", err)
		return
	}

	lu.removeUser(access)
	if lu.ViewedAccess == "" && lu.PreloadAccess == "" {
		tdb.Rem("LastUser", key)
		return
	}

	buf, err := json.Marshal(&lu)
	if err != nil {
		log.TLogln("Error rem last user:", err)
		return
	}
	tdb.Set("LastUser", key, buf)
}
