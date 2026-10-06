package fbb

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Arch identifies the ABI used to compile the FBB binary.
type Arch int

const (
	Arch32 Arch = 32
	Arch64 Arch = 64
)

func (a Arch) String() string {
	if a == Arch32 {
		return "32"
	}
	if a == Arch64 {
		return "64"
	}
	return fmt.Sprintf("%d", int(a))
}

// ArchMode is the command-line setting. Auto detects the layout from both
// files before any records are parsed.
type ArchMode string

const (
	ArchAuto   ArchMode = "auto"
	Arch32Mode ArchMode = "32"
	Arch64Mode ArchMode = "64"
)

// Layout contains all offsets that differ between the two supported C ABIs.
// Keeping offsets here prevents parsers from silently mixing layouts.
type Layout struct {
	Arch Arch

	LongSize int

	InfoRecordSize   int
	DirmesRecordSize int

	Info struct {
		Callsign  int
		SSID      int
		LastMes   int
		NbCon     int
		HCon      int
		LastYap   int
		Flags     int
		OnBase    int
		NBL       int
		Lang      int
		NewBanner int
		Download  int
		Thema     int
		Name      int
		FirstName int
		Address   int
		City      int
		Home      int
		QTH       int
		Private   int
		Filter    int
		Password  int
		ZIP       int
	}

	Dirmes struct {
		Type      int
		Status    int
		Number    int
		Size      int
		Date      int
		BBSFrom   int
		Route     int
		From      int
		To        int
		BID       int
		Title     int
		Created   int
		Changed   int
		ForwardTo int
		Forwarded int
	}
}

func layoutForArch(arch Arch) (Layout, error) {
	switch arch {
	case Arch32:
		return Layout{
			Arch:             Arch32,
			LongSize:         4,
			InfoRecordSize:   360,
			DirmesRecordSize: 194,
			Info: struct {
				Callsign  int
				SSID      int
				LastMes   int
				NbCon     int
				HCon      int
				LastYap   int
				Flags     int
				OnBase    int
				NBL       int
				Lang      int
				NewBanner int
				Download  int
				Thema     int
				Name      int
				FirstName int
				Address   int
				City      int
				Home      int
				QTH       int
				Private   int
				Filter    int
				Password  int
				ZIP       int
			}{Callsign: 0, SSID: 7, LastMes: 72, NbCon: 76, HCon: 80, LastYap: 84, Flags: 88, OnBase: 90, NBL: 92, Lang: 93, NewBanner: 94, Download: 98, Thema: 120, Name: 121, FirstName: 139, Address: 152, City: 213, Home: 270, QTH: 311, Private: 318, Filter: 325, Password: 338, ZIP: 351},
			Dirmes: struct {
				Type      int
				Status    int
				Number    int
				Size      int
				Date      int
				BBSFrom   int
				Route     int
				From      int
				To        int
				BID       int
				Title     int
				Created   int
				Changed   int
				ForwardTo int
				Forwarded int
			}{Type: 0, Status: 1, Number: 2, Size: 6, Date: 10, BBSFrom: 14, Route: 21, From: 62, To: 69, BID: 76, Title: 89, Created: 166, Changed: 170, ForwardTo: 174, Forwarded: 184},
		}, nil
	case Arch64:
		return Layout{
			Arch:             Arch64,
			LongSize:         8,
			InfoRecordSize:   384,
			DirmesRecordSize: 224,
			Info: struct {
				Callsign  int
				SSID      int
				LastMes   int
				NbCon     int
				HCon      int
				LastYap   int
				Flags     int
				OnBase    int
				NBL       int
				Lang      int
				NewBanner int
				Download  int
				Thema     int
				Name      int
				FirstName int
				Address   int
				City      int
				Home      int
				QTH       int
				Private   int
				Filter    int
				Password  int
				ZIP       int
			}{Callsign: 0, SSID: 7, LastMes: 72, NbCon: 80, HCon: 88, LastYap: 96, Flags: 104, OnBase: 106, NBL: 108, Lang: 109, NewBanner: 112, Download: 120, Thema: 142, Name: 143, FirstName: 161, Address: 174, City: 235, Home: 292, QTH: 333, Private: 340, Filter: 353, Password: 360, ZIP: 373},
			Dirmes: struct {
				Type      int
				Status    int
				Number    int
				Size      int
				Date      int
				BBSFrom   int
				Route     int
				From      int
				To        int
				BID       int
				Title     int
				Created   int
				Changed   int
				ForwardTo int
				Forwarded int
			}{Type: 0, Status: 1, Number: 8, Size: 16, Date: 24, BBSFrom: 32, Route: 39, From: 80, To: 87, BID: 94, Title: 107, Created: 184, Changed: 192, ForwardTo: 200, Forwarded: 210},
		}, nil
	default:
		return Layout{}, fmt.Errorf("unsupported FBB architecture %d", arch)
	}
}

// LayoutForArch exposes the fixed layout for tests and callers that need to
// construct synthetic records.
func LayoutForArch(arch Arch) (Layout, error) {
	return layoutForArch(arch)
}

// DetectLayout chooses a layout from inf.sys and dirmes.sys. Both files must
// agree; this avoids accepting a partially-read or mixed installation.
func DetectLayout(root string, mode ArchMode) (Layout, error) {
	if mode == "" {
		mode = ArchAuto
	}
	var requested Arch
	switch mode {
	case Arch32Mode:
		requested = Arch32
	case Arch64Mode:
		requested = Arch64
	case ArchAuto:
	default:
		return Layout{}, fmt.Errorf("invalid FBB architecture %q (want auto, 32 or 64)", mode)
	}

	infoSize, err := fileSize(root, "inf.sys")
	if err != nil {
		return Layout{}, err
	}
	dirmesSize, err := fileSize(root, "dirmes.sys")
	if err != nil {
		return Layout{}, err
	}

	if requested != 0 {
		layout, err := layoutForArch(requested)
		if err != nil {
			return Layout{}, err
		}
		if infoSize%int64(layout.InfoRecordSize) != 0 || dirmesSize%int64(layout.DirmesRecordSize) != 0 {
			return Layout{}, fmt.Errorf("FBB files do not match %d-bit layout: inf.sys=%d bytes, dirmes.sys=%d bytes", requested, infoSize, dirmesSize)
		}
		return layout, nil
	}

	valid32 := infoSize%360 == 0 && dirmesSize%194 == 0
	valid64 := infoSize%384 == 0 && dirmesSize%224 == 0
	switch {
	case valid32 && !valid64:
		return layoutForArch(Arch32)
	case valid64 && !valid32:
		return layoutForArch(Arch64)
	case valid32 && valid64:
		// This is rare, but 64 is the configured safe default for ambiguity.
		log.Printf("FBB layout is ambiguous for inf.sys=%d and dirmes.sys=%d; assuming 64-bit", infoSize, dirmesSize)
		return layoutForArch(Arch64)
	default:
		return Layout{}, fmt.Errorf("cannot detect FBB layout: inf.sys=%d bytes, dirmes.sys=%d bytes", infoSize, dirmesSize)
	}
}

func fileSize(root, name string) (int64, error) {
	info, err := os.Stat(filepath.Join(root, name))
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", name)
	}
	return info.Size(), nil
}
