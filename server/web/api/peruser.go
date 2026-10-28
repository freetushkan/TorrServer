package api

// Per-user data separation (fork feature, not in upstream): request helpers
// used by the one-line hooks in the upstream handlers.

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	sets "server/settings"
)

const (
	lastViewed  = sets.LastUserViewedAccess
	lastPreload = sets.LastUserPreloadAccess
)

// currentUser is the authenticated (HTTP Basic) account of the request, or "".
func currentUser(c *gin.Context) string {
	user, _ := c.Get(gin.AuthUserKey)
	login, _ := user.(string)
	return login
}

// requestUser resolves the account a request acts for and attaches it to the
// request context (torr.Stream reads it from there).
//
// An authenticated request (stream, play, viewed, ffprobe…) is remembered as
// the last user of the torrent. Requests without credentials act for ?user=
// or, failing that, for that last user (lastAccess "" disables the fallback).
// They are common: besides TorrServer's own loopback reads (preload/ffprobe,
// GStreamer), FFmpeg-based players (mpv, VLC) keep user:pass@ from the URL
// but only send it after a 401, and a known torrent is streamed without one.
func requestUser(c *gin.Context, hash, lastAccess string) string {
	hash = strings.ToLower(hash)
	user := currentUser(c)
	if user != "" {
		sets.SetLastUser(hash, user, lastPreload)
		sets.SetLastUser(hash, user, lastViewed)
	} else if sets.PerUserEnabled() {
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
