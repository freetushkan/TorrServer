//go:build gst

package gstreamer

// Per-user data separation (fork feature, not in upstream).
// GStreamer reads the torrent through TorrServer's own loopback /stream or /play
// requests, which carry no credentials; they act for the last user of the
// torrent, so record the account that started the probe/transcode.

import (
	"strings"

	"github.com/gin-gonic/gin"

	"server/settings"
)

func rememberRequestUser(c *gin.Context) {
	if user := c.GetString(gin.AuthUserKey); user != "" {
		settings.SetLastUser(strings.ToLower(c.Param("hash")), user, settings.LastUserPreloadAccess)
	}
}
