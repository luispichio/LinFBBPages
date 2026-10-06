package fbb

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Message is a record from dirmes.sys. Timestamp fields are Unix seconds so
// the API does not impose a timezone on clients.
type Message struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	Number    int64  `json:"number"`
	Size      int64  `json:"size"`
	Date      int64  `json:"date"`
	BBSFrom   string `json:"bbs_from,omitempty"`
	Route     string `json:"route,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	BID       string `json:"bid,omitempty"`
	Title     string `json:"title,omitempty"`
	CreatedAt int64  `json:"created_at"`
	ChangedAt int64  `json:"changed_at"`
}

type messagesCache struct {
	stamp      fileStamp
	lastNumber int64
	messages   []Message
	valid      bool
}

// parseDirmes decodes the header and all valid message records. Record zero
// contains the last number but is not itself a message.
func parseDirmes(data []byte, layout Layout) (int64, []Message, error) {
	if len(data)%layout.DirmesRecordSize != 0 {
		return 0, nil, fmt.Errorf("dirmes.sys has %d bytes, not a multiple of %d", len(data), layout.DirmesRecordSize)
	}
	if len(data) < layout.DirmesRecordSize {
		return 0, nil, fmt.Errorf("dirmes.sys is missing its header record")
	}

	header := data[:layout.DirmesRecordSize]
	lastNumber := readLong(header, layout.Dirmes.Number, layout.LongSize)
	messages := make([]Message, 0, len(data)/layout.DirmesRecordSize-1)
	for offset := layout.DirmesRecordSize; offset < len(data); offset += layout.DirmesRecordSize {
		record := data[offset : offset+layout.DirmesRecordSize]
		messageType := record[layout.Dirmes.Type]
		if !validMessageType(messageType) {
			continue
		}
		status := record[layout.Dirmes.Status]
		if !validMessageStatus(status) {
			continue
		}
		message := Message{
			Type:      string(messageType),
			Status:    string(status),
			Number:    readLong(record, layout.Dirmes.Number, layout.LongSize),
			Size:      readLong(record, layout.Dirmes.Size, layout.LongSize),
			Date:      readLong(record, layout.Dirmes.Date, layout.LongSize),
			BBSFrom:   cString(record, layout.Dirmes.BBSFrom, 7),
			Route:     cString(record, layout.Dirmes.Route, 41),
			From:      cString(record, layout.Dirmes.From, 7),
			To:        cString(record, layout.Dirmes.To, 7),
			BID:       cString(record, layout.Dirmes.BID, 13),
			Title:     cString(record, layout.Dirmes.Title, 61),
			CreatedAt: readLong(record, layout.Dirmes.Created, layout.LongSize),
			ChangedAt: readLong(record, layout.Dirmes.Changed, layout.LongSize),
		}
		if message.Number <= 0 {
			continue
		}
		messages = append(messages, message)
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Number > messages[j].Number
	})
	return lastNumber, messages, nil
}

func readLong(record []byte, offset, size int) int64 {
	var value uint64
	for i := 0; i < size; i++ {
		value |= uint64(record[offset+i]) << (8 * i)
	}
	return int64(value)
}

func validMessageType(value byte) bool {
	return strings.ContainsRune("ABPT", rune(value))
}

func validMessageStatus(value byte) bool {
	return strings.ContainsRune("$AFKNY", rune(value))
}

// Messages returns a snapshot of active records. dirmes.sys is read again
// whenever its size or modification time changes.
func (s *Store) Messages() ([]Message, error) {
	path := s.path("dirmes.sys")
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat dirmes.sys: %w", err)
	}
	current := stamp(info)

	s.messagesMu.Lock()
	defer s.messagesMu.Unlock()
	if s.messagesCache.valid && s.messagesCache.stamp == current {
		return cloneMessages(s.messagesCache.messages), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dirmes.sys: %w", err)
	}
	lastNumber, messages, err := parseDirmes(data, s.layout)
	if err != nil {
		return nil, err
	}
	s.messagesCache = messagesCache{stamp: current, lastNumber: lastNumber, messages: messages, valid: true}
	return cloneMessages(messages), nil
}

func cloneMessages(messages []Message) []Message {
	result := make([]Message, len(messages))
	copy(result, messages)
	return result
}

// Message returns one indexed message and its current body. The index is
// re-read through Messages, so a removed message is never served by number.
func (s *Store) Message(number int64) (Message, MessageBody, error) {
	messages, err := s.Messages()
	if err != nil {
		return Message{}, MessageBody{}, err
	}
	for _, message := range messages {
		if message.Number == number {
			body, bodyErr := s.ReadMessageBody(number)
			if bodyErr != nil {
				return Message{}, MessageBody{}, bodyErr
			}
			return message, body, nil
		}
	}
	return Message{}, MessageBody{}, os.ErrNotExist
}
