package fbb

import (
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// userFlagSysop is F_SYS, the S (Sysop) flag defined by LinFBB.
const userFlagSysop uint16 = 0x0008 // F_SYS: the S (Sysop) flag in LinFBB.

// User is the subset of an FBB user record needed by the web application.
// Password is intentionally not exported as JSON by the API; it is retained
// here only so Authenticate can compare it with the login request.
type User struct {
	Callsign  string `json:"callsign"`
	SSID      byte   `json:"ssid,omitempty"`
	Name      string `json:"name,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	Address   string `json:"address,omitempty"`
	City      string `json:"city,omitempty"`
	Home      string `json:"home_bbs,omitempty"`
	QTH       string `json:"qth,omitempty"`
	ZIP       string `json:"zip,omitempty"`
	Sysop     bool   `json:"sysop"`

	password string
}

func (u User) loginCallsign() string {
	if u.SSID == 0 {
		return u.Callsign
	}
	return fmt.Sprintf("%s-%d", u.Callsign, u.SSID)
}

// parseUsers decodes all fixed-size records from inf.sys.
func parseUsers(data []byte, layout Layout) ([]User, error) {
	if len(data)%layout.InfoRecordSize != 0 {
		return nil, fmt.Errorf("inf.sys has %d bytes, not a multiple of %d", len(data), layout.InfoRecordSize)
	}
	users := make([]User, 0, len(data)/layout.InfoRecordSize)
	for offset := 0; offset < len(data); offset += layout.InfoRecordSize {
		record := data[offset : offset+layout.InfoRecordSize]
		callsign := cString(record, layout.Info.Callsign, 7)
		if callsign == "" {
			continue
		}
		flags := binary.LittleEndian.Uint16(record[layout.Info.Flags : layout.Info.Flags+2])
		users = append(users, User{
			Callsign:  callsign,
			SSID:      record[layout.Info.SSID],
			Name:      cString(record, layout.Info.Name, 18),
			FirstName: cString(record, layout.Info.FirstName, 13),
			Address:   cString(record, layout.Info.Address, 61),
			City:      cString(record, layout.Info.City, 31),
			Home:      cString(record, layout.Info.Home, 41),
			QTH:       cString(record, layout.Info.QTH, 7),
			ZIP:       cString(record, layout.Info.ZIP, 9),
			Sysop:     flags&userFlagSysop != 0,
			password:  cStringRaw(record, layout.Info.Password, 13),
		})
	}
	return users, nil
}

// cString reads a fixed-width C string and removes bytes after the first NUL.
func cString(record []byte, offset, size int) string {
	return strings.TrimSpace(cStringRaw(record, offset, size))
}

func cStringRaw(record []byte, offset, size int) string {
	if offset < 0 || size < 0 || offset+size > len(record) {
		return ""
	}
	field := record[offset : offset+size]
	if end := indexByte(field, 0); end >= 0 {
		field = field[:end]
	}
	return string(field)
}

func indexByte(value []byte, target byte) int {
	for i, b := range value {
		if b == target {
			return i
		}
	}
	return -1
}

type fileStamp struct {
	size    int64
	modTime int64
}

func stamp(info os.FileInfo) fileStamp {
	return fileStamp{size: info.Size(), modTime: info.ModTime().UnixNano()}
}

type usersCache struct {
	stamp fileStamp
	users []User
	valid bool
}

// Users returns a snapshot of users. The file is read again only when its
// size or modification time changes.
func (s *Store) Users() ([]User, error) {
	path := s.path("inf.sys")
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat inf.sys: %w", err)
	}
	current := stamp(info)

	s.usersMu.Lock()
	defer s.usersMu.Unlock()
	if s.usersCache.valid && s.usersCache.stamp == current {
		return cloneUsers(s.usersCache.users), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read inf.sys: %w", err)
	}
	users, err := parseUsers(data, s.layout)
	if err != nil {
		return nil, err
	}
	s.usersCache = usersCache{stamp: current, users: users, valid: true}
	return cloneUsers(users), nil
}

func cloneUsers(users []User) []User {
	result := make([]User, len(users))
	copy(result, users)
	return result
}

// Authenticate looks up a callsign case-insensitively and compares the FBB
// plaintext password without altering it. SSID callsigns may be entered as
// CALL or CALL-SSID.
func (s *Store) Authenticate(callsign, password string) (User, bool, error) {
	users, err := s.Users()
	if err != nil {
		return User{}, false, err
	}
	wanted := strings.TrimSpace(callsign)
	for _, user := range users {
		matchesCallsign := strings.EqualFold(wanted, user.Callsign) || strings.EqualFold(wanted, user.loginCallsign())
		if !matchesCallsign || user.password == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(user.password), []byte(password)) == 1 {
			user.password = ""
			return user, true, nil
		}
	}
	return User{}, false, nil
}

// Store owns the FBB directory and its small in-memory caches.
type Store struct {
	root   string
	layout Layout

	usersMu    sync.Mutex
	usersCache usersCache

	messagesMu    sync.Mutex
	messagesCache messagesCache

	mailMu sync.Mutex
}

// NewStore validates the data directory, determines the binary layout and
// creates a read-only view of FBB's files.
func NewStore(root string, mode ArchMode) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("FBB directory cannot be empty")
	}
	layout, err := DetectLayout(root, mode)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, layout: layout}, nil
}

func (s *Store) path(parts ...string) string {
	pathParts := append([]string{s.root}, parts...)
	return filepath.Join(pathParts...)
}

// Layout returns the selected binary layout.
func (s *Store) Layout() Layout {
	return s.layout
}
