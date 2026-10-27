package api

// Per-user data separation (fork feature, not in upstream): request helpers
// used by the one-line hooks in the upstream handlers.

import (
	"encoding/json"
	"net/url"

	"github.com/gin-gonic/gin"

	sets "server/settings"
)

const (
	lastViewed  = sets.LastUserViewedAccess
	lastPreload = sets.LastUserPreloadAccess
)

// rememberPreloadUser lets later unauthenticated links of hash act for user.
func rememberPreloadUser(hash, user string) {
	sets.SetLastUser(hash, user, lastPreload)
}

// currentUser is the authenticated (HTTP Basic) account of the request, or "".
func currentUser(c *gin.Context) string {
	user, _ := c.Get(gin.AuthUserKey)
	login, _ := user.(string)
	return login
}

// requestUser resolves the account a request acts for and attaches it to the
// request context (torr.Stream reads it from there). Unauthenticated
// play/playlist links carry it in ?user=, falling back to the last account
// that used the torrent through lastAccess ("" disables the fallback).
func requestUser(c *gin.Context, hash, lastAccess string) string {
	user := currentUser(c)
	if user == "" && sets.PerUserEnabled() {
		user = c.Query("user")
		if user == "" && lastAccess != "" {
			user = sets.GetLastUser(hash, lastAccess)
		}
	}
	if user != "" {
		c.Request = c.Request.WithContext(sets.ContextWithUser(c.Request.Context(), user))
	}
	return user
}

// userParam is the "&user=..." suffix for play links in generated playlists.
func userParam(user string) string {
	if user == "" || !sets.PerUserEnabled() {
		return ""
	}
	return "&user=" + url.QueryEscape(user)
}

// settingsForUser is BTsets plus the read-only fields the settings UI needs.
func settingsForUser(c *gin.Context) any {
	resp := map[string]any{}
	if buf, err := json.Marshal(sets.BTsets); err == nil {
		_ = json.Unmarshal(buf, &resp)
	}
	resp["PerUserDataForced"] = sets.PerUserForced()
	resp["CurrentUser"] = currentUser(c)
	return resp
}
