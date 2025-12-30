package p2p

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ankouros/pterminal/internal/model"
	"github.com/ankouros/pterminal/internal/teamrepo"
)

const (
	udpPort          = 43277
	announceInterval = 3 * time.Second
	syncInterval     = 6 * time.Second
	peerTTL          = 18 * time.Second
	syncTimeout      = 25 * time.Second
)

type Service struct {
	cfgMu    sync.RWMutex
	cfg      model.AppConfig
	deviceID string
	user     model.UserProfile
	baseDir  string
	secret   []byte
	insecure bool

	peersMu sync.Mutex
	peers   map[string]*peerState

	udpConn  *net.UDPConn
	udpAddrs []*net.UDPAddr
	tcpLn    net.Listener
	tcpPort  int
	stopCh   chan struct{}
	onMerged func(model.AppConfig)
}

type peerState struct {
	info     PeerInfo
	lastSeen time.Time
	lastSync time.Time
}

type helloMsg struct {
	App       string        `json:"app"`
	Type      string        `json:"type"`
	DeviceID  string        `json:"deviceId"`
	Name      string        `json:"name,omitempty"`
	Email     string        `json:"email,omitempty"`
	Host      string        `json:"host,omitempty"`
	TCPPort   int           `json:"tcpPort"`
	Teams     []TeamSummary `json:"teams,omitempty"`
	Timestamp int64         `json:"ts,omitempty"`
	Auth      string        `json:"auth,omitempty"`
}

type wireMessage struct {
	Type      string              `json:"type"`
	DeviceID  string              `json:"deviceId,omitempty"`
	User      model.UserProfile   `json:"user,omitempty"`
	Config    model.AppConfig     `json:"config,omitempty"`
	Teams     []TeamSummary       `json:"teams,omitempty"`
	Manifests []teamrepo.Manifest `json:"manifests,omitempty"`
	TeamID    string              `json:"teamId,omitempty"`
	Path      string              `json:"path,omitempty"`
	Paths     []string            `json:"paths,omitempty"`
	Hash      string              `json:"hash,omitempty"`
	ModTime   int64               `json:"modTime,omitempty"`
	DataB64   string              `json:"dataB64,omitempty"`
}

func NewService(cfg model.AppConfig, baseDir string) (*Service, error) {
	if cfg.User.DeviceID == "" {
		return nil, errors.New("missing device id")
	}
	if baseDir == "" {
		return nil, errors.New("missing base dir")
	}

	secret, insecure, err := loadSecret()
	if err != nil {
		return nil, err
	}
	if insecure {
		log.Printf("p2p: insecure mode enabled via %s", envP2PInsecure)
	}

	s := &Service{
		cfg:      cfg,
		deviceID: cfg.User.DeviceID,
		user:     cfg.User,
		baseDir:  baseDir,
		secret:   secret,
		insecure: insecure,
		peers:    make(map[string]*peerState),
		stopCh:   make(chan struct{}),
	}

	if err := s.start(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Service) start() error {
	addrs, err := discoverBroadcastAddrs(udpPort)
	if err != nil {
		return err
	}
	s.udpAddrs = addrs

	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: udpPort})
	if err != nil {
		return err
	}
	if err := udpConn.SetReadBuffer(1 << 20); err != nil {
		log.Printf("p2p: udp read buffer: %v", err)
	}
	if err := udpConn.SetWriteBuffer(1 << 20); err != nil {
		log.Printf("p2p: udp write buffer: %v", err)
	}
	s.udpConn = udpConn

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	s.tcpLn = ln
	if addr, ok := ln.Addr().(*net.TCPAddr); ok {
		s.tcpPort = addr.Port
	}

	go s.announceLoop()
	go s.listenLoop()
	go s.syncLoop()
	go s.acceptLoop()

	return nil
}

func (s *Service) Close() {
	close(s.stopCh)
	if s.udpConn != nil {
		_ = s.udpConn.Close()
	}
	if s.tcpLn != nil {
		_ = s.tcpLn.Close()
	}
}

func (s *Service) SetConfig(cfg model.AppConfig) {
	s.cfgMu.Lock()
	s.cfg = cfg
	s.user = cfg.User
	s.cfgMu.Unlock()
}

func (s *Service) SetOnMerged(fn func(model.AppConfig)) {
	s.onMerged = fn
}

func (s *Service) Peers() []PeerInfo {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	out := make([]PeerInfo, 0, len(s.peers))
	for _, peer := range s.peers {
		info := peer.info
		info.LastSeen = peer.lastSeen.Unix()
		out = append(out, info)
	}
	return out
}

func (s *Service) Presence() PresenceSnapshot {
	return PresenceSnapshot{Peers: s.Peers(), User: s.user}
}

func (s *Service) SyncNow() {
	s.peersMu.Lock()
	for _, peer := range s.peers {
		peer.lastSync = time.Time{}
	}
	s.peersMu.Unlock()
}

func (s *Service) announceLoop() {
	ticker := time.NewTicker(announceInterval)
	defer ticker.Stop()

	for {
		s.sendHello()
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) sendHello() {
	hostname, _ := os.Hostname()
	teams := s.teamSummaries()

	ts := int64(0)
	auth := ""
	if len(s.secret) > 0 {
		ts = time.Now().Unix()
		auth = helloAuth(s.secret, s.deviceID, s.tcpPort, ts)
	}

	msg := helloMsg{
		App:       "pterminal",
		Type:      "hello",
		DeviceID:  s.deviceID,
		Name:      s.user.Name,
		Email:     s.user.Email,
		Host:      hostname,
		TCPPort:   s.tcpPort,
		Teams:     teams,
		Timestamp: ts,
		Auth:      auth,
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for _, addr := range s.udpAddrs {
		_, _ = s.udpConn.WriteToUDP(b, addr)
	}
}

func (s *Service) listenLoop() {
	buf := make([]byte, 8192)
	for {
		n, addr, err := s.udpConn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
			}
			continue
		}
		var msg helloMsg
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}
		if msg.App != "pterminal" || msg.Type != "hello" {
			continue
		}
		if msg.DeviceID == "" || msg.DeviceID == s.deviceID {
			continue
		}
		if !verifyHello(s.secret, msg.DeviceID, msg.TCPPort, msg.Timestamp, msg.Auth, time.Now()) {
			continue
		}
		s.upsertPeer(msg, addr)
	}
}

func (s *Service) upsertPeer(msg helloMsg, addr *net.UDPAddr) {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	peer := s.peers[msg.DeviceID]
	if peer == nil {
		peer = &peerState{}
		s.peers[msg.DeviceID] = peer
	}

	peer.info = PeerInfo{
		DeviceID: msg.DeviceID,
		Name:     msg.Name,
		Email:    msg.Email,
		Host:     msg.Host,
		Addr:     addr.IP.String(),
		TCPPort:  msg.TCPPort,
		Teams:    msg.Teams,
	}
	peer.lastSeen = time.Now()
}

func (s *Service) syncLoop() {
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.syncPeers()
		}
	}
}

func (s *Service) syncPeers() {
	peers := s.snapshotPeers()
	for _, peer := range peers {
		if peer.info.TCPPort == 0 {
			continue
		}
		if time.Since(peer.lastSeen) > peerTTL {
			continue
		}
		if time.Since(peer.lastSync) < syncInterval {
			continue
		}
		s.syncPeer(peer)
	}
}

func (s *Service) snapshotPeers() []peerState {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	out := make([]peerState, 0, len(s.peers))
	for _, peer := range s.peers {
		out = append(out, *peer)
		peer.lastSync = time.Now()
	}
	return out
}

func (s *Service) syncPeer(peer peerState) {
	addr := net.JoinHostPort(peer.info.Addr, fmt.Sprintf("%d", peer.info.TCPPort))
	conn, err := net.DialTimeout("tcp", addr, 4*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(syncTimeout))

	codec, err := newCodec(conn, s.secret)
	if err != nil {
		return
	}

	cfg := s.configSnapshot()
	manifests := s.buildManifests(cfg)

	localMsg := wireMessage{
		Type:      "sync",
		DeviceID:  s.deviceID,
		User:      s.user,
		Config:    s.teamScopedConfig(cfg),
		Teams:     s.teamSummariesFromConfig(cfg),
		Manifests: manifests,
	}
	if err := codec.Encode(localMsg); err != nil {
		return
	}

	var remote wireMessage
	if err := codec.Decode(&remote); err != nil {
		return
	}
	if remote.Type == "sync" {
		s.applyRemote(remote)
		s.syncFiles(codec, manifests, remote.Manifests, remote.DeviceID)
	}
}

func (s *Service) applyRemote(remote wireMessage) {
	if remote.Config.Version == 0 {
		return
	}

	local := s.configSnapshot()
	merged, changed := MergeRemote(local, s.teamScopedConfig(remote.Config))
	if !changed {
		return
	}

	s.SetConfig(merged)
	if s.onMerged != nil {
		s.onMerged(merged)
	}
}

func (s *Service) syncFiles(codec codec, local, remote []teamrepo.Manifest, remoteDevice string) {
	localMap := manifestMap(local)
	remoteMap := manifestMap(remote)
	localFileMaps := manifestFileMaps(local)

	for teamID, r := range remoteMap {
		l := localMap[teamID]
		_ = s.applyRemoteDeletions(l, r)
	}

	type wantReq struct {
		teamID string
		path   string
	}

	wantCh := make(chan wantReq, 256)
	fileDone := make(chan struct{})
	var sendMu sync.Mutex
	send := func(msg wireMessage) {
		sendMu.Lock()
		_ = codec.Encode(msg)
		sendMu.Unlock()
	}

	go func() {
		defer close(fileDone)
		wantClosed := false
		for {
			var msg wireMessage
			if err := codec.Decode(&msg); err != nil {
				if !wantClosed {
					close(wantCh)
				}
				return
			}
			switch msg.Type {
			case "want":
				for _, p := range msg.Paths {
					if msg.TeamID == "" || p == "" {
						continue
					}
					wantCh <- wantReq{teamID: msg.TeamID, path: p}
				}
			case "want_done":
				if !wantClosed {
					close(wantCh)
					wantClosed = true
				}
			case "file":
				_ = s.applyTeamFile(msg, remoteDevice)
			case "file_done":
				return
			}
		}
	}()

	for teamID, r := range remoteMap {
		l := localMap[teamID]
		wants := computeWants(l, r)
		for _, batch := range chunkPaths(wants, 200) {
			send(wireMessage{
				Type:   "want",
				TeamID: teamID,
				Paths:  batch,
			})
		}
	}
	send(wireMessage{Type: "want_done"})

	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		for req := range wantCh {
			files := localFileMaps[req.teamID]
			entry, ok := files[req.path]
			if !ok || entry.Deleted {
				continue
			}
			data, err := s.readTeamFile(req.teamID, req.path)
			if err != nil {
				continue
			}
			msg := wireMessage{
				Type:    "file",
				TeamID:  req.teamID,
				Path:    req.path,
				Hash:    entry.Hash,
				ModTime: entry.ModTime,
				DataB64: base64.StdEncoding.EncodeToString(data),
			}
			send(msg)
		}
	}()

	<-serveDone
	send(wireMessage{Type: "file_done"})
	<-fileDone
}

func (s *Service) applyTeamFile(msg wireMessage, remoteDevice string) error {
	if msg.TeamID == "" {
		return errors.New("missing team id")
	}
	fullPath, err := s.teamFilePath(msg.TeamID, msg.Path)
	if err != nil {
		return err
	}

	conflict := false
	if existing, err := os.ReadFile(fullPath); err == nil {
		hash := sha256Sum(existing)
		if msg.Hash != "" && hash != msg.Hash {
			conflict = true
		}
	}

	if conflict {
		suffix := time.Now().Format("20060102-150405")
		fullPath = fullPath + ".conflict-" + remoteDevice + "-" + suffix
	}

	data, err := base64.StdEncoding.DecodeString(msg.DataB64)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(fullPath, data, 0o600); err != nil {
		return err
	}
	if msg.ModTime > 0 {
		_ = os.Chtimes(fullPath, time.Unix(msg.ModTime, 0), time.Unix(msg.ModTime, 0))
	}

	return nil
}

func (s *Service) configSnapshot() model.AppConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

func (s *Service) teamSummaries() []TeamSummary {
	cfg := s.configSnapshot()
	return s.teamSummariesFromConfig(cfg)
}

func (s *Service) teamSummariesFromConfig(cfg model.AppConfig) []TeamSummary {
	out := []TeamSummary{}
	for _, t := range cfg.Teams {
		if t.Deleted {
			continue
		}
		out = append(out, TeamSummary{ID: t.ID, Name: t.Name})
	}
	return out
}

func (s *Service) buildManifests(cfg model.AppConfig) []teamrepo.Manifest {
	manifests := []teamrepo.Manifest{}
	for _, t := range cfg.Teams {
		if t.ID == "" || t.Deleted {
			continue
		}
		teamDir, err := teamrepo.EnsureTeamDir(s.baseDir, t.ID)
		if err != nil {
			continue
		}
		manifest, err := teamrepo.BuildManifest(teamDir, t.ID)
		if err != nil {
			continue
		}
		_ = teamrepo.WriteManifest(teamDir, manifest)
		manifests = append(manifests, manifest)
	}
	return manifests
}

func (s *Service) teamScopedConfig(cfg model.AppConfig) model.AppConfig {
	out := cfg
	out.Networks = nil
	for _, netw := range cfg.Networks {
		if netw.TeamID == "" || netw.Deleted {
			continue
		}
		filtered := netw
		filtered.Hosts = nil
		for _, host := range netw.Hosts {
			if host.Deleted {
				continue
			}
			if host.Scope == model.ScopeTeam && host.TeamID == netw.TeamID {
				filtered.Hosts = append(filtered.Hosts, host)
			}
		}
		if len(filtered.Hosts) == 0 {
			continue
		}
		out.Networks = append(out.Networks, filtered)
	}

	out.Scripts = nil
	for _, script := range cfg.Scripts {
		if script.Deleted {
			continue
		}
		if script.Scope == model.ScopeTeam && script.TeamID != "" {
			out.Scripts = append(out.Scripts, script)
		}
	}

	return out
}

func (s *Service) readTeamFile(teamID, relPath string) ([]byte, error) {
	fullPath, err := s.teamFilePath(teamID, relPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(fullPath)
}

func manifestMap(manifests []teamrepo.Manifest) map[string]teamrepo.Manifest {
	out := make(map[string]teamrepo.Manifest, len(manifests))
	for _, m := range manifests {
		if m.TeamID != "" {
			out[m.TeamID] = m
		}
	}
	return out
}

func manifestFileMaps(manifests []teamrepo.Manifest) map[string]map[string]teamrepo.FileEntry {
	out := make(map[string]map[string]teamrepo.FileEntry, len(manifests))
	for _, m := range manifests {
		if m.TeamID == "" {
			continue
		}
		files := make(map[string]teamrepo.FileEntry, len(m.Files))
		for _, entry := range m.Files {
			files[entry.Path] = entry
		}
		out[m.TeamID] = files
	}
	return out
}

func computeWants(local, remote teamrepo.Manifest) []string {
	localMap := map[string]teamrepo.FileEntry{}
	for _, entry := range local.Files {
		localMap[entry.Path] = entry
	}

	wants := []string{}
	for _, entry := range remote.Files {
		if _, err := cleanTeamRelPath(entry.Path); err != nil {
			continue
		}
		if entry.Deleted {
			continue
		}
		le, ok := localMap[entry.Path]
		if !ok || le.Deleted {
			if !ok || le.ModTime < entry.ModTime {
				wants = append(wants, entry.Path)
			}
			continue
		}
		if le.Hash == entry.Hash {
			continue
		}
		if entry.ModTime > le.ModTime {
			wants = append(wants, entry.Path)
			continue
		}
		if entry.ModTime == le.ModTime && entry.Hash > le.Hash {
			wants = append(wants, entry.Path)
		}
	}
	return wants
}

func chunkPaths(paths []string, size int) [][]string {
	if size <= 0 || len(paths) == 0 {
		return nil
	}
	out := [][]string{}
	for start := 0; start < len(paths); start += size {
		end := start + size
		if end > len(paths) {
			end = len(paths)
		}
		out = append(out, paths[start:end])
	}
	return out
}

func sha256Sum(b []byte) string {
	h := sha256.New()
	_, _ = h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (s *Service) applyRemoteDeletions(local, remote teamrepo.Manifest) error {
	if remote.TeamID == "" {
		return nil
	}

	localMap := map[string]teamrepo.FileEntry{}
	for _, entry := range local.Files {
		localMap[entry.Path] = entry
	}

	for _, entry := range remote.Files {
		if !entry.Deleted {
			continue
		}
		le, ok := localMap[entry.Path]
		if !ok || le.Deleted {
			continue
		}
		if entry.Hash != "" && le.Hash != "" && entry.Hash != le.Hash {
			continue
		}
		fullPath, err := s.teamFilePath(remote.TeamID, entry.Path)
		if err != nil {
			continue
		}
		_ = os.Remove(fullPath)
	}
	return nil
}

func discoverBroadcastAddrs(port int) ([]*net.UDPAddr, error) {
	addrs := []*net.UDPAddr{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		ifaceAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range ifaceAddrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}
			mask := ipNet.Mask
			if len(mask) != 4 {
				continue
			}
			bcast := net.IPv4(
				ip[0]|^mask[0],
				ip[1]|^mask[1],
				ip[2]|^mask[2],
				ip[3]|^mask[3],
			)
			addrs = append(addrs, &net.UDPAddr{IP: bcast, Port: port})
		}
	}
	addrs = append(addrs, &net.UDPAddr{IP: net.IPv4bcast, Port: port})
	return addrs, nil
}

func (s *Service) acceptLoop() {
	for {
		conn, err := s.tcpLn.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
			}
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Service) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(syncTimeout))

	codec, err := newCodec(conn, s.secret)
	if err != nil {
		return
	}

	cfg := s.configSnapshot()
	manifests := s.buildManifests(cfg)
	localMsg := wireMessage{
		Type:      "sync",
		DeviceID:  s.deviceID,
		User:      s.user,
		Config:    s.teamScopedConfig(cfg),
		Teams:     s.teamSummariesFromConfig(cfg),
		Manifests: manifests,
	}
	if err := codec.Encode(localMsg); err != nil {
		return
	}

	var remote wireMessage
	if err := codec.Decode(&remote); err != nil {
		return
	}
	if remote.Type != "sync" {
		return
	}

	s.applyRemote(remote)
	s.syncFiles(codec, manifests, remote.Manifests, remote.DeviceID)
}
