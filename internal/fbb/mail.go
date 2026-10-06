package fbb

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// MessageBody contains the routing headers and plain-text body stored in an
// m_XXXXXX.mes file.
type MessageBody struct {
	Routing []string `json:"routing"`
	Body    string   `json:"body"`
}

// ReadMessageBody reads the directory selected by the message number. FBB
// stores message N in mail/mail(N modulo 10)/m_%06d.mes.
func (s *Store) ReadMessageBody(number int64) (MessageBody, error) {
	if number <= 0 {
		return MessageBody{}, os.ErrNotExist
	}
	directory := fmt.Sprintf("mail%d", number%10)
	name := fmt.Sprintf("m_%06d.mes", number)
	data, err := os.ReadFile(filepath.Join(s.root, "mail", directory, name))
	if err != nil {
		return MessageBody{}, fmt.Errorf("read message %d: %w", number, err)
	}
	return parseMessageBody(string(data)), nil
}

func parseMessageBody(value string) MessageBody {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	var routing []string
	bodyStart := 0
	for bodyStart < len(lines) && strings.HasPrefix(lines[bodyStart], "R:") {
		routing = append(routing, lines[bodyStart])
		bodyStart++
	}
	if bodyStart < len(lines) && lines[bodyStart] == "" {
		bodyStart++
	}
	body := strings.Join(lines[bodyStart:], "\n")
	// A final line ending is a file-format detail rather than part of the
	// visible message. Preserve all other whitespace exactly.
	body = strings.TrimSuffix(body, "\n")
	return MessageBody{Routing: routing, Body: body}
}

// ComposeRequest is the validated logical representation of an outgoing FBB
// message. Validation is kept here so CLI or future non-HTTP callers get the
// same mail.in guarantees.
type ComposeRequest struct {
	To    string
	Route string
	Type  string
	Title string
	Body  string
}

func (r ComposeRequest) Validate() error {
	if r.Type != "P" && r.Type != "B" {
		return errors.New("message type must be P or B")
	}
	if !validEnvelopePart(r.To) {
		return errors.New("destination must be a non-empty single line")
	}
	if r.Route != "" && !validEnvelopePart(r.Route) {
		return errors.New("route must be a single line")
	}
	if !validSingleLine(r.Title) || strings.TrimSpace(r.Title) == "" {
		return errors.New("title must be a non-empty single line")
	}
	if strings.TrimSpace(r.Title) == "/EX" {
		return errors.New("title cannot be /EX")
	}
	if strings.ContainsRune(r.Body, '\x00') {
		return errors.New("body contains a NUL byte")
	}
	for _, line := range strings.Split(strings.ReplaceAll(r.Body, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "/EX" {
			return errors.New("body cannot contain a line equal to /EX")
		}
	}
	if len(r.To) > 80 || len(r.Route) > 160 || len(r.Title) > 200 || len(r.Body) > 512*1024 {
		return errors.New("message exceeds the allowed size")
	}
	return nil
}

func validSingleLine(value string) bool {
	return !strings.ContainsAny(value, "\r\n\x00")
}

func validEnvelopePart(value string) bool {
	return validSingleLine(value) && value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, " \t<>$@")
}

// AppendMail queues one message using FBB's native import format. A process
// mutex serializes requests from this application and an advisory flock
// coordinates with other Unix processes that honor the same lock.
func (s *Store) AppendMail(source string, request ComposeRequest) error {
	if !validEnvelopePart(source) {
		return errors.New("source callsign must be a non-empty single line")
	}
	if err := request.Validate(); err != nil {
		return err
	}

	var command string
	if request.Type == "B" {
		command = "SB"
	} else {
		command = "SP"
	}
	destination := request.To
	if request.Route != "" {
		destination += " @" + request.Route
	}
	message := fmt.Sprintf("%s %s < %s\n%s\n%s\n/EX\n", command, destination, source, request.Title, normalizeNewlines(request.Body))

	s.mailMu.Lock()
	defer s.mailMu.Unlock()
	path := filepath.Join(s.root, "mail", "mail.in")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return fmt.Errorf("open mail.in: %w", err)
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock mail.in: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if _, err := io.WriteString(file, message); err != nil {
		return fmt.Errorf("write mail.in: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync mail.in: %w", err)
	}
	return nil
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
}
