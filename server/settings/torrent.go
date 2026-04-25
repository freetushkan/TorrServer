package settings

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"server/log"
)

type TorrentDB struct {
	*torrent.TorrentSpec

	Title    string   `json:"title,omitempty"`
	Category string   `json:"category,omitempty"`
	Poster   string   `json:"poster,omitempty"`
	Data     string   `json:"data,omitempty"`
	Users    []string `json:"users,omitempty"`

	Timestamp int64 `json:"timestamp,omitempty"`
	Size      int64 `json:"size,omitempty"`
}

type File struct {
	Name string `json:"name,omitempty"`
	Id   int    `json:"id,omitempty"`
	Size int64  `json:"size,omitempty"`
}

var mu sync.Mutex

func AddTorrent(torr *TorrentDB) {
	list := ListTorrent()
	mu.Lock()
	find := -1
	for i, db := range list {
		if db.InfoHash.HexString() == torr.InfoHash.HexString() {
			find = i
			break
		}
	}
	if find != -1 {
		list[find] = torr
	} else {
		list = append(list, torr)
	}
	for _, db := range list {
		buf, err := json.Marshal(db)
		if err == nil {
			tdb.Set("Torrents", db.InfoHash.HexString(), buf)
		}
	}
	mu.Unlock()
}

func ListTorrent() []*TorrentDB {
	start := time.Now()
	// Use read lock to prevent migration during read
	dbMigrationLock.RLock()
	defer dbMigrationLock.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	var list []*TorrentDB
	listStart := time.Now()
	keys := tdb.List("Torrents")
	log.TLogln("settings.ListTorrent: list keys=", len(keys), " took=", time.Since(listStart))
	for _, key := range keys {
		getStart := time.Now()
		buf := tdb.Get("Torrents", key)
		getDur := time.Since(getStart)
		if getDur > 250*time.Millisecond {
			log.TLogln("settings.ListTorrent: slow get key=", key, " took=", getDur)
		}
		if len(buf) > 0 {
			var torr *TorrentDB
			unmarshalStart := time.Now()
			err := json.Unmarshal(buf, &torr)
			if err == nil {
				list = append(list, torr)
			} else {
				log.TLogln("settings.ListTorrent: unmarshal failed key=", key, " err=", err)
			}
			unmarshalDur := time.Since(unmarshalStart)
			if unmarshalDur > 250*time.Millisecond {
				log.TLogln("settings.ListTorrent: slow unmarshal key=", key, " took=", unmarshalDur)
			}
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Timestamp > list[j].Timestamp
	})
	log.TLogln("settings.ListTorrent: loaded=", len(list), " total=", time.Since(start))
	return list
}

func RemTorrent(hash metainfo.Hash) {
	mu.Lock()
	tdb.Rem("Torrents", hash.HexString())
	mu.Unlock()
}
