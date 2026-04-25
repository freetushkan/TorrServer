package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"server/log"
)

// ListUsers returns account names from accs.db.
func ListUsers() []string {
	start := time.Now()
	buf, err := os.ReadFile(filepath.Join(Path, "accs.db"))
	if err != nil {
		log.TLogln("ListUsers(): read error:", err)
		return []string{}
	}

	accs := make(map[string]string)
	if err := json.Unmarshal(buf, &accs); err != nil {
		log.TLogln("ListUsers(): unmarshal error:", err)
		return []string{}
	}

	users := make([]string, 0, len(accs))
	for user := range accs {
		users = append(users, user)
	}
	sort.Strings(users)
	if dur := time.Since(start); dur > 100*time.Millisecond {
		log.TLogln("ListUsers(): loaded=", len(users), " took=", dur)
	}
	return users
}
