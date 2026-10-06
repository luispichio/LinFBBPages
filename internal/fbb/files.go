package fbb

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DecodedFile describes one file under 7pfbb/ok without exposing its
// filesystem path to the HTTP layer.
type DecodedFile struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	MIME      string    `json:"mime"`
	Modified  time.Time `json:"modified"`
	Auxiliary bool      `json:"auxiliary"`
}

type DecodedGroup struct {
	Name      string        `json:"name"`
	Primary   *DecodedFile  `json:"primary,omitempty"`
	Auxiliary []DecodedFile `json:"auxiliary,omitempty"`
}

// DecodedFiles lists the decoded output directory and groups metadata files
// with their payload. .7mf is treated as a payload when present: real FBB
// installations commonly store a JPEG directly with that suffix.
func (s *Store) DecodedFiles() ([]DecodedGroup, error) {
	directory := filepath.Join(s.root, "7pfbb", "ok")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read decoded files: %w", err)
	}
	type groupBuilder struct {
		name      string
		primary   []DecodedFile
		auxiliary []DecodedFile
	}
	groups := make(map[string]*groupBuilder)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if !safeBaseName(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat decoded file %s: %w", name, err)
		}
		file := DecodedFile{
			Name:     name,
			Size:     info.Size(),
			MIME:     detectMIME(filepath.Join(directory, name), info.Size()),
			Modified: info.ModTime().UTC(),
		}
		key, auxiliary := decodedGroupKey(name)
		builder := groups[key]
		if builder == nil {
			builder = &groupBuilder{name: key}
			groups[key] = builder
		}
		if auxiliary {
			file.Auxiliary = true
			builder.auxiliary = append(builder.auxiliary, file)
		} else {
			builder.primary = append(builder.primary, file)
		}
	}

	result := make([]DecodedGroup, 0, len(groups))
	for _, builder := range groups {
		sort.Slice(builder.primary, func(i, j int) bool {
			return primaryRank(builder.primary[i]) < primaryRank(builder.primary[j])
		})
		group := DecodedGroup{Name: builder.name, Auxiliary: builder.auxiliary}
		if len(builder.primary) > 0 {
			primary := builder.primary[0]
			group.Primary = &primary
			if len(builder.primary) > 1 {
				for _, extra := range builder.primary[1:] {
					extra.Auxiliary = true
					group.Auxiliary = append(group.Auxiliary, extra)
				}
			}
		}
		sort.Slice(group.Auxiliary, func(i, j int) bool {
			return group.Auxiliary[i].Name < group.Auxiliary[j].Name
		})
		result = append(result, group)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func decodedGroupKey(name string) (string, bool) {
	extension := strings.ToLower(filepath.Ext(name))
	if extension == ".7ix" || extension == ".err" {
		return strings.TrimSuffix(name, filepath.Ext(name)), true
	}
	return strings.TrimSuffix(name, filepath.Ext(name)), false
}

func primaryRank(file DecodedFile) int {
	extension := strings.ToLower(filepath.Ext(file.Name))
	if strings.HasPrefix(file.MIME, "image/") && extension != ".7mf" {
		return 0
	}
	if strings.HasPrefix(file.MIME, "image/") {
		return 1
	}
	return 2
}

func detectMIME(path string, size int64) string {
	file, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer file.Close()
	limit := int64(512)
	if size < limit {
		limit = size
	}
	buffer := make([]byte, limit)
	read, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "application/octet-stream"
	}
	return http.DetectContentType(buffer[:read])
}

// ReadDecodedFile opens only a validated basename from the public decoded
// directory. The caller owns and must close the returned file.
func (s *Store) ReadDecodedFile(name string) (*os.File, os.FileInfo, error) {
	if !safeBaseName(name) {
		return nil, nil, os.ErrInvalid
	}
	path := filepath.Join(s.root, "7pfbb", "ok", name)
	lstat, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if lstat.Mode()&os.ModeSymlink != 0 {
		return nil, nil, os.ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, os.ErrInvalid
	}
	return file, info, nil
}

func safeBaseName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsRune(name, 0)
}
