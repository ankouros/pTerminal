package p2p

import (
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ankouros/pterminal/internal/model"
)

const (
	versionEqual = iota
	versionLess
	versionGreater
	versionConcurrent
)

func ApplyLocalEdits(current, incoming model.AppConfig) (model.AppConfig, bool) {
	changed := false
	deviceID := current.User.DeviceID
	actor := actorName(current.User)
	now := time.Now().Unix()

	incoming.User.DeviceID = deviceID

	incoming.Teams, changed = applyLocalTeams(current.Teams, incoming.Teams, deviceID, actor, now, changed)
	incoming.Scripts, changed = applyLocalScripts(current.Scripts, incoming.Scripts, deviceID, actor, now, changed)
	incoming.Networks, changed = applyLocalNetworks(current.Networks, incoming.Networks, deviceID, actor, now, changed)

	return incoming, changed
}

func MergeRemote(local, remote model.AppConfig) (model.AppConfig, bool) {
	changed := false

	merged := local
	merged.Teams, changed = mergeTeams(local.Teams, remote.Teams, changed)
	merged.Scripts, changed = mergeScripts(local.Scripts, remote.Scripts, changed)
	merged.Networks, changed = mergeNetworks(local.Networks, remote.Networks, changed)

	merged.User = local.User
	return merged, changed
}

func actorName(user model.UserProfile) string {
	if strings.TrimSpace(user.Email) != "" {
		return user.Email
	}
	if strings.TrimSpace(user.Name) != "" {
		return user.Name
	}
	return user.DeviceID
}

func compareVersion(a, b map[string]int, aUpdated, bUpdated int64) int {
	aEmpty := len(a) == 0
	bEmpty := len(b) == 0
	if aEmpty && bEmpty {
		if aUpdated == bUpdated {
			return versionEqual
		}
		if aUpdated < bUpdated {
			return versionLess
		}
		return versionGreater
	}

	less := false
	greater := false
	for k, av := range a {
		bv := b[k]
		if av < bv {
			less = true
		} else if av > bv {
			greater = true
		}
	}
	for k, bv := range b {
		av := a[k]
		if av < bv {
			less = true
		} else if av > bv {
			greater = true
		}
	}

	switch {
	case less && greater:
		return versionConcurrent
	case less:
		return versionLess
	case greater:
		return versionGreater
	default:
		return versionEqual
	}
}

func bumpVersion(v map[string]int, deviceID string) map[string]int {
	if v == nil {
		v = map[string]int{}
	}
	v[deviceID] = v[deviceID] + 1
	return v
}

type idAllocator struct {
	used map[int]struct{}
	next int
}

func newIDAllocator(ids []int) *idAllocator {
	used := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			used[id] = struct{}{}
		}
	}
	next := 1
	for {
		if _, ok := used[next]; !ok {
			break
		}
		next++
	}
	return &idAllocator{used: used, next: next}
}

func (a *idAllocator) Next() int {
	for {
		if _, ok := a.used[a.next]; !ok {
			break
		}
		a.next++
	}
	id := a.next
	a.used[id] = struct{}{}
	a.next++
	return id
}

func applyLocalTeams(current, incoming []model.Team, deviceID, actor string, now int64, changed bool) ([]model.Team, bool) {
	curMap := map[string]model.Team{}
	for _, t := range current {
		if t.ID != "" {
			curMap[t.ID] = t
		}
	}

	seen := map[string]struct{}{}
	out := make([]model.Team, 0, len(incoming))

	for _, t := range incoming {
		if t.ID == "" {
			t.ID = model.NewID()
			changed = true
		}
		if _, ok := seen[t.ID]; ok {
			t.ID = model.NewID()
			changed = true
		}
		seen[t.ID] = struct{}{}

		if cur, ok := curMap[t.ID]; ok {
			if !teamCoreEqual(cur, t) || cur.Deleted != t.Deleted {
				t.Version = bumpVersion(cur.Version, deviceID)
				t.UpdatedAt = now
				t.UpdatedBy = actor
				changed = true
			} else {
				t.Version = cur.Version
				t.UpdatedAt = cur.UpdatedAt
				t.UpdatedBy = cur.UpdatedBy
				t.Conflict = cur.Conflict
				t.Deleted = cur.Deleted
			}
		} else {
			t.Version = bumpVersion(t.Version, deviceID)
			t.UpdatedAt = now
			t.UpdatedBy = actor
			changed = true
		}
		out = append(out, t)
	}

	for _, cur := range current {
		if _, ok := seen[cur.ID]; ok {
			continue
		}
		cur.Deleted = true
		cur.Version = bumpVersion(cur.Version, deviceID)
		cur.UpdatedAt = now
		cur.UpdatedBy = actor
		cur.Conflict = false
		out = append(out, cur)
		changed = true
	}

	return out, changed
}

func applyLocalScripts(current, incoming []model.TeamScript, deviceID, actor string, now int64, changed bool) ([]model.TeamScript, bool) {
	curMap := map[string]model.TeamScript{}
	for _, s := range current {
		if s.ID != "" {
			curMap[s.ID] = s
		}
	}
	seen := map[string]struct{}{}
	out := make([]model.TeamScript, 0, len(incoming))

	for _, s := range incoming {
		if s.ID == "" {
			s.ID = model.NewID()
			changed = true
		}
		if _, ok := seen[s.ID]; ok {
			s.ID = model.NewID()
			changed = true
		}
		seen[s.ID] = struct{}{}
		if s.Scope == "" {
			s.Scope = model.ScopePrivate
		}
		if s.Scope == model.ScopePrivate {
			s.TeamID = ""
		}

		if cur, ok := curMap[s.ID]; ok {
			if !scriptCoreEqual(cur, s) || cur.Deleted != s.Deleted {
				s.Version = bumpVersion(cur.Version, deviceID)
				s.UpdatedAt = now
				s.UpdatedBy = actor
				changed = true
			} else {
				s.Version = cur.Version
				s.UpdatedAt = cur.UpdatedAt
				s.UpdatedBy = cur.UpdatedBy
				s.Conflict = cur.Conflict
				s.Deleted = cur.Deleted
			}
		} else {
			s.Version = bumpVersion(s.Version, deviceID)
			s.UpdatedAt = now
			s.UpdatedBy = actor
			changed = true
		}
		out = append(out, s)
	}

	for _, cur := range current {
		if _, ok := seen[cur.ID]; ok {
			continue
		}
		cur.Deleted = true
		cur.Version = bumpVersion(cur.Version, deviceID)
		cur.UpdatedAt = now
		cur.UpdatedBy = actor
		cur.Conflict = false
		out = append(out, cur)
		changed = true
	}

	return out, changed
}

func applyLocalNetworks(current, incoming []model.Network, deviceID, actor string, now int64, changed bool) ([]model.Network, bool) {
	curMap := map[string]model.Network{}
	for _, n := range current {
		if n.UID == "" {
			n.UID = model.NewID()
		}
		curMap[n.UID] = n
	}

	netAlloc := newIDAllocator(collectNetworkIDs(current, incoming))
	hostAlloc := newIDAllocator(collectHostIDs(current, incoming))

	seen := map[string]struct{}{}
	out := make([]model.Network, 0, len(incoming))

	for _, netw := range incoming {
		if netw.UID == "" {
			netw.UID = model.NewID()
			changed = true
		}
		if _, ok := seen[netw.UID]; ok {
			netw.UID = model.NewID()
			changed = true
		}
		seen[netw.UID] = struct{}{}

		if netw.ID <= 0 {
			netw.ID = netAlloc.Next()
		}

		cur, hasCur := curMap[netw.UID]
		if hasCur {
			if cur.ID > 0 && netw.ID != cur.ID {
				netw.ID = cur.ID
				changed = true
			}
			if !networkCoreEqual(cur, netw) || cur.Deleted != netw.Deleted {
				netw.Version = bumpVersion(cur.Version, deviceID)
				netw.UpdatedAt = now
				netw.UpdatedBy = actor
				changed = true
			} else {
				netw.Version = cur.Version
				netw.UpdatedAt = cur.UpdatedAt
				netw.UpdatedBy = cur.UpdatedBy
				netw.Conflict = cur.Conflict
				netw.Deleted = cur.Deleted
			}
		} else {
			netw.Version = bumpVersion(netw.Version, deviceID)
			netw.UpdatedAt = now
			netw.UpdatedBy = actor
			changed = true
		}

		netw.Hosts, changed = applyLocalHosts(cur.Hosts, netw.Hosts, deviceID, actor, now, hostAlloc, changed, netw.TeamID)
		if netw.TeamID == "" {
			for _, h := range netw.Hosts {
				if h.Scope == model.ScopeTeam && h.TeamID != "" {
					netw.TeamID = h.TeamID
					break
				}
			}
		}

		out = append(out, netw)
	}

	for _, cur := range current {
		if _, ok := seen[cur.UID]; ok {
			continue
		}
		cur.Deleted = true
		cur.Version = bumpVersion(cur.Version, deviceID)
		cur.UpdatedAt = now
		cur.UpdatedBy = actor
		cur.Conflict = false
		out = append(out, cur)
		changed = true
	}

	return out, changed
}

func applyLocalHosts(current, incoming []model.Host, deviceID, actor string, now int64, hostAlloc *idAllocator, changed bool, netTeamID string) ([]model.Host, bool) {
	curMap := map[string]model.Host{}
	for _, h := range current {
		if h.UID == "" {
			h.UID = model.NewID()
		}
		curMap[h.UID] = h
	}

	seen := map[string]struct{}{}
	out := make([]model.Host, 0, len(incoming))

	for _, h := range incoming {
		if h.UID == "" {
			h.UID = model.NewID()
			changed = true
		}
		if _, ok := seen[h.UID]; ok {
			h.UID = model.NewID()
			changed = true
		}
		seen[h.UID] = struct{}{}

		if h.ID <= 0 {
			h.ID = hostAlloc.Next()
		}
		if h.Scope == "" {
			h.Scope = model.ScopePrivate
		}
		if h.Scope == model.ScopeTeam {
			if h.TeamID == "" && netTeamID != "" {
				h.TeamID = netTeamID
			}
		} else {
			h.TeamID = ""
		}

		if cur, ok := curMap[h.UID]; ok {
			if cur.ID > 0 && h.ID != cur.ID {
				h.ID = cur.ID
				changed = true
			}
			if !hostCoreEqual(cur, h) || cur.Deleted != h.Deleted {
				h.Version = bumpVersion(cur.Version, deviceID)
				h.UpdatedAt = now
				h.UpdatedBy = actor
				changed = true
			} else {
				h.Version = cur.Version
				h.UpdatedAt = cur.UpdatedAt
				h.UpdatedBy = cur.UpdatedBy
				h.Conflict = cur.Conflict
				h.Deleted = cur.Deleted
			}
		} else {
			h.Version = bumpVersion(h.Version, deviceID)
			h.UpdatedAt = now
			h.UpdatedBy = actor
			changed = true
		}

		out = append(out, h)
	}

	for _, cur := range current {
		if _, ok := seen[cur.UID]; ok {
			continue
		}
		cur.Deleted = true
		cur.Version = bumpVersion(cur.Version, deviceID)
		cur.UpdatedAt = now
		cur.UpdatedBy = actor
		cur.Conflict = false
		out = append(out, cur)
		changed = true
	}

	return out, changed
}

func collectNetworkIDs(current, incoming []model.Network) []int {
	ids := []int{}
	for _, n := range current {
		if n.ID > 0 {
			ids = append(ids, n.ID)
		}
	}
	for _, n := range incoming {
		if n.ID > 0 {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

func collectHostIDs(current, incoming []model.Network) []int {
	ids := []int{}
	for _, n := range current {
		for _, h := range n.Hosts {
			if h.ID > 0 {
				ids = append(ids, h.ID)
			}
		}
	}
	for _, n := range incoming {
		for _, h := range n.Hosts {
			if h.ID > 0 {
				ids = append(ids, h.ID)
			}
		}
	}
	return ids
}

func mergeTeams(local, remote []model.Team, changed bool) ([]model.Team, bool) {
	localMap := map[string]model.Team{}
	order := []string{}
	for _, t := range local {
		if t.ID == "" {
			t.ID = model.NewID()
		}
		localMap[t.ID] = t
		order = append(order, t.ID)
	}

	for _, r := range remote {
		if r.ID == "" {
			r.ID = model.NewID()
		}
		if l, ok := localMap[r.ID]; ok {
			base := l
			cmp := compareVersion(l.Version, r.Version, l.UpdatedAt, r.UpdatedAt)
			switch cmp {
			case versionLess:
				l = r
				changed = true
			case versionConcurrent:
				merged := mergeTeamMembers(l, r)
				l.Members = merged
				l.Conflict = true
				changed = true
			}
			mergedReq := mergeTeamRequests(base, r)
			if !requestsEqual(l.Requests, mergedReq) {
				l.Requests = mergedReq
				changed = true
			}
			localMap[r.ID] = l
		} else {
			localMap[r.ID] = r
			order = append(order, r.ID)
			changed = true
		}
	}

	out := make([]model.Team, 0, len(localMap))
	for _, id := range order {
		if t, ok := localMap[id]; ok {
			out = append(out, t)
		}
	}
	return out, changed
}

func mergeScripts(local, remote []model.TeamScript, changed bool) ([]model.TeamScript, bool) {
	localMap := map[string]model.TeamScript{}
	order := []string{}
	for _, s := range local {
		if s.ID == "" {
			s.ID = model.NewID()
		}
		localMap[s.ID] = s
		order = append(order, s.ID)
	}

	for _, r := range remote {
		if r.ID == "" {
			r.ID = model.NewID()
		}
		if r.Scope == "" {
			r.Scope = model.ScopePrivate
		}
		if r.Scope == model.ScopePrivate {
			r.TeamID = ""
		}
		if l, ok := localMap[r.ID]; ok {
			cmp := compareVersion(l.Version, r.Version, l.UpdatedAt, r.UpdatedAt)
			switch cmp {
			case versionLess:
				l = r
				changed = true
			case versionConcurrent:
				l.Conflict = true
				conflict := r
				conflict.ID = model.NewID()
				conflict.Name = conflictName(r.Name)
				conflict.Conflict = true
				order = append(order, conflict.ID)
				localMap[conflict.ID] = conflict
				changed = true
			}
			localMap[r.ID] = l
		} else {
			localMap[r.ID] = r
			order = append(order, r.ID)
			changed = true
		}
	}

	out := make([]model.TeamScript, 0, len(localMap))
	for _, id := range order {
		if s, ok := localMap[id]; ok {
			out = append(out, s)
		}
	}
	return out, changed
}

func mergeNetworks(local, remote []model.Network, changed bool) ([]model.Network, bool) {
	localMap := map[string]model.Network{}
	order := []string{}
	for _, n := range local {
		if n.UID == "" {
			n.UID = model.NewID()
		}
		localMap[n.UID] = n
		order = append(order, n.UID)
	}

	netAlloc := newIDAllocator(collectNetworkIDs(local, remote))
	hostAlloc := newIDAllocator(collectHostIDs(local, remote))

	for _, r := range remote {
		if r.UID == "" {
			r.UID = model.NewID()
		}
		if l, ok := localMap[r.UID]; ok {
			if l.ID <= 0 {
				l.ID = netAlloc.Next()
			}
			cmp := compareVersion(l.Version, r.Version, l.UpdatedAt, r.UpdatedAt)
			switch cmp {
			case versionLess:
				l.Name = r.Name
				l.TeamID = r.TeamID
				l.Deleted = r.Deleted
				l.UpdatedAt = r.UpdatedAt
				l.UpdatedBy = r.UpdatedBy
				l.Version = r.Version
				l.Conflict = r.Conflict
				changed = true
			case versionConcurrent:
				l.Conflict = true
				conflict := r
				conflict.UID = model.NewID()
				conflict.ID = netAlloc.Next()
				conflict.Name = conflictName(r.Name)
				conflict.Hosts = nil
				conflict.Conflict = true
				order = append(order, conflict.UID)
				localMap[conflict.UID] = conflict
				changed = true
			}
			l.Hosts, changed = mergeHosts(l.Hosts, r.Hosts, hostAlloc, changed)
			if l.TeamID == "" {
				if teamID := inferTeamID(l.Hosts); teamID != "" {
					l.TeamID = teamID
					changed = true
				}
			}
			localMap[r.UID] = l
		} else {
			r.ID = netAlloc.Next()
			r.Hosts, changed = mergeHosts(nil, r.Hosts, hostAlloc, changed)
			if r.TeamID == "" {
				if teamID := inferTeamID(r.Hosts); teamID != "" {
					r.TeamID = teamID
					changed = true
				}
			}
			localMap[r.UID] = r
			order = append(order, r.UID)
			changed = true
		}
	}

	out := make([]model.Network, 0, len(localMap))
	for _, id := range order {
		if n, ok := localMap[id]; ok {
			out = append(out, n)
		}
	}
	return out, changed
}

func mergeHosts(local, remote []model.Host, hostAlloc *idAllocator, changed bool) ([]model.Host, bool) {
	localMap := map[string]model.Host{}
	order := []string{}
	for _, h := range local {
		if h.UID == "" {
			h.UID = model.NewID()
		}
		localMap[h.UID] = h
		order = append(order, h.UID)
	}

	for _, r := range remote {
		if r.UID == "" {
			r.UID = model.NewID()
		}
		if r.Scope == "" {
			r.Scope = model.ScopePrivate
		}
		if r.Scope == model.ScopePrivate {
			r.TeamID = ""
		}
		if l, ok := localMap[r.UID]; ok {
			if l.ID <= 0 {
				l.ID = hostAlloc.Next()
			}
			cmp := compareVersion(l.Version, r.Version, l.UpdatedAt, r.UpdatedAt)
			switch cmp {
			case versionLess:
				id := l.ID
				l = r
				l.ID = id
				changed = true
			case versionConcurrent:
				l.Conflict = true
				conflict := r
				conflict.UID = model.NewID()
				conflict.ID = hostAlloc.Next()
				conflict.Name = conflictName(r.Name)
				conflict.Conflict = true
				order = append(order, conflict.UID)
				localMap[conflict.UID] = conflict
				changed = true
			}
			localMap[r.UID] = l
		} else {
			r.ID = hostAlloc.Next()
			localMap[r.UID] = r
			order = append(order, r.UID)
			changed = true
		}
	}

	out := make([]model.Host, 0, len(localMap))
	for _, id := range order {
		if h, ok := localMap[id]; ok {
			out = append(out, h)
		}
	}
	return out, changed
}

func teamCoreEqual(a, b model.Team) bool {
	if strings.TrimSpace(a.Name) != strings.TrimSpace(b.Name) {
		return false
	}
	am := normalizeMembers(a.Members)
	bm := normalizeMembers(b.Members)
	ar := normalizeRequests(a.Requests)
	br := normalizeRequests(b.Requests)
	return reflect.DeepEqual(am, bm) && reflect.DeepEqual(ar, br)
}

func normalizeMembers(members []model.TeamMember) []model.TeamMember {
	out := make([]model.TeamMember, 0, len(members))
	for _, m := range members {
		email := strings.ToLower(strings.TrimSpace(m.Email))
		name := strings.TrimSpace(m.Name)
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = model.TeamRoleUser
		}
		if role != model.TeamRoleAdmin && role != model.TeamRoleUser {
			role = model.TeamRoleUser
		}
		out = append(out, model.TeamMember{Email: email, Name: name, Role: role})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Email == out[j].Email {
			return out[i].Name < out[j].Name
		}
		return out[i].Email < out[j].Email
	})
	return out
}

func mergeTeamMembers(a, b model.Team) []model.TeamMember {
	byEmail := map[string]model.TeamMember{}
	for _, m := range normalizeMembers(a.Members) {
		if m.Email != "" {
			byEmail[m.Email] = m
		}
	}
	for _, m := range normalizeMembers(b.Members) {
		if m.Email == "" {
			continue
		}
		if existing, ok := byEmail[m.Email]; ok {
			if existing.Role != model.TeamRoleAdmin && m.Role == model.TeamRoleAdmin {
				existing.Role = model.TeamRoleAdmin
			}
			if existing.Name == "" && m.Name != "" {
				existing.Name = m.Name
			}
			byEmail[m.Email] = existing
			continue
		}
		byEmail[m.Email] = m
	}
	out := make([]model.TeamMember, 0, len(byEmail))
	for _, m := range byEmail {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out
}

func requestsEqual(a, b []model.TeamJoinRequest) bool {
	return reflect.DeepEqual(normalizeRequests(a), normalizeRequests(b))
}

func normalizeRequests(reqs []model.TeamJoinRequest) []model.TeamJoinRequest {
	byEmail := map[string]model.TeamJoinRequest{}
	for _, r := range reqs {
		email := strings.ToLower(strings.TrimSpace(r.Email))
		if email == "" {
			continue
		}
		r.Email = email
		r.Name = strings.TrimSpace(r.Name)
		switch r.Status {
		case model.TeamJoinPending, model.TeamJoinApproved, model.TeamJoinDeclined:
		default:
			r.Status = model.TeamJoinPending
		}
		if existing, ok := byEmail[email]; ok {
			byEmail[email] = pickJoinRequest(existing, r)
		} else {
			byEmail[email] = r
		}
	}
	out := make([]model.TeamJoinRequest, 0, len(byEmail))
	for _, r := range byEmail {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out
}

func mergeTeamRequests(a, b model.Team) []model.TeamJoinRequest {
	byEmail := map[string]model.TeamJoinRequest{}
	for _, r := range normalizeRequests(a.Requests) {
		if r.Email != "" {
			byEmail[r.Email] = r
		}
	}
	for _, r := range normalizeRequests(b.Requests) {
		if r.Email == "" {
			continue
		}
		if existing, ok := byEmail[r.Email]; ok {
			byEmail[r.Email] = pickJoinRequest(existing, r)
		} else {
			byEmail[r.Email] = r
		}
	}
	out := make([]model.TeamJoinRequest, 0, len(byEmail))
	for _, r := range byEmail {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out
}

func pickJoinRequest(a, b model.TeamJoinRequest) model.TeamJoinRequest {
	at := joinRequestUpdatedAt(a)
	bt := joinRequestUpdatedAt(b)
	if bt > at {
		return b
	}
	if at > bt {
		return a
	}
	ap := joinRequestPriority(a.Status)
	bp := joinRequestPriority(b.Status)
	if bp > ap {
		return b
	}
	if ap > bp {
		return a
	}
	return a
}

func joinRequestUpdatedAt(r model.TeamJoinRequest) int64 {
	if r.Status != model.TeamJoinPending && r.ResolvedAt > 0 {
		return r.ResolvedAt
	}
	return r.RequestedAt
}

func joinRequestPriority(status string) int {
	switch status {
	case model.TeamJoinApproved:
		return 3
	case model.TeamJoinPending:
		return 2
	case model.TeamJoinDeclined:
		return 1
	default:
		return 0
	}
}

func scriptCoreEqual(a, b model.TeamScript) bool {
	return a.Name == b.Name &&
		a.Command == b.Command &&
		a.Description == b.Description &&
		a.Scope == b.Scope &&
		a.TeamID == b.TeamID
}

func networkCoreEqual(a, b model.Network) bool {
	return a.Name == b.Name && a.TeamID == b.TeamID
}

func hostCoreEqual(a, b model.Host) bool {
	return a.Name == b.Name &&
		a.Host == b.Host &&
		a.Port == b.Port &&
		a.User == b.User &&
		a.Driver == b.Driver &&
		a.Auth == b.Auth &&
		a.HostKey == b.HostKey &&
		a.Scope == b.Scope &&
		a.TeamID == b.TeamID &&
		reflect.DeepEqual(a.Telecom, b.Telecom) &&
		reflect.DeepEqual(a.SFTP, b.SFTP)
}

func inferTeamID(hosts []model.Host) string {
	for _, h := range hosts {
		if h.Scope == model.ScopeTeam && h.TeamID != "" {
			return h.TeamID
		}
	}
	return ""
}

func conflictName(name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "item"
	}
	return base + " (conflict)"
}
