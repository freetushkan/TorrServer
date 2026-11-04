package torr

// Per-user data separation (fork feature, not in upstream).
// Ownership is kept in its own DB bucket (see settings/peruser.go), so the
// upstream Torrent / TorrentDB / TorrentStatus types stay untouched.
//
// Owners semantics:
//   - no owners recorded  -> the torrent is shared: every account sees it;
//   - owners recorded     -> only those accounts see it;
//   - accounts deleted from accs.db are ignored.

import (
	"slices"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	sets "server/settings"
)

// torrentOwners returns the recorded owners of hash that still exist in accs.db.
func torrentOwners(hash string, accounts []string) []string {
	owners := sets.GetTorrentOwners(hash)
	if len(accounts) == 0 {
		return owners
	}
	ret := owners[:0:0]
	for _, u := range owners {
		if slices.Contains(accounts, u) && !slices.Contains(ret, u) {
			ret = append(ret, u)
		}
	}
	return ret
}

func torrentExists(hash metainfo.Hash) bool {
	return bts.GetTorrent(hash) != nil || GetTorrentDB(hash) != nil
}

// AddTorrentForUser is AddTorrent that also records user as an owner.
// A torrent that did not exist before becomes owned by user alone; an existing
// owned torrent gets user added; an existing shared torrent stays shared.
func AddTorrentForUser(user string, spec *torrent.TorrentSpec, title, poster, data, category string) (*Torrent, error) {
	existed := torrentExists(spec.InfoHash)
	tor, err := AddTorrent(spec, title, poster, data, category)
	if err != nil || !sets.PerUserEnabled() || user == "" {
		return tor, err
	}
	hash := spec.InfoHash.HexString()
	if !existed {
		sets.SetTorrentOwners(hash, []string{user})
		return tor, nil
	}
	owners := torrentOwners(hash, sets.ListUsers())
	if len(owners) != 0 && !slices.Contains(owners, user) {
		sets.SetTorrentOwners(hash, append(owners, user))
	}
	return tor, nil
}

// RemTorrentForUser removes user from the owners of the torrent and deletes the
// torrent once nobody owns it. Without per-user mode it is RemTorrent.
// Returns true if the torrent itself was removed.
func RemTorrentForUser(hashHex, user string) bool {
	if !sets.PerUserEnabled() || user == "" {
		RemTorrent(hashHex)
		sets.RemTorrentOwners(hashHex)
		return true
	}
	if !torrentExists(metainfo.NewHashFromHex(hashHex)) {
		sets.RemTorrentOwners(hashHex)
		return false
	}
	accounts := sets.ListUsers()
	owners := torrentOwners(hashHex, accounts)
	if len(owners) == 0 {
		owners = accounts // shared: owned by everybody
	}
	if !slices.Contains(owners, user) {
		return false // not visible to this user, nothing to remove
	}
	rest := slices.DeleteFunc(slices.Clone(owners), func(u string) bool { return u == user })
	if len(rest) == 0 {
		RemTorrent(hashHex)
		sets.RemTorrentOwners(hashHex)
		return true
	}
	sets.SetTorrentOwners(hashHex, rest)
	return false
}

// ListTorrentForUser is ListTorrent limited to the torrents visible to user.
func ListTorrentForUser(user string) []*Torrent {
	list := ListTorrent()
	if !sets.PerUserEnabled() || user == "" {
		return list
	}
	accounts := sets.ListUsers()
	ret := make([]*Torrent, 0, len(list))
	for _, t := range list {
		owners := torrentOwners(t.Hash().HexString(), accounts)
		if len(owners) == 0 || slices.Contains(owners, user) {
			ret = append(ret, t)
		}
	}
	return ret
}
