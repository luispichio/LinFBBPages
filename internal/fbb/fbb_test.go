package fbb

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "design", "usr", "local", "var", "ax25", "fbb")
}

func TestLayouts(t *testing.T) {
	layout32, err := LayoutForArch(Arch32)
	if err != nil {
		t.Fatal(err)
	}
	if layout32.InfoRecordSize != 360 || layout32.DirmesRecordSize != 194 || layout32.Info.Password != 338 || layout32.Dirmes.Number != 2 {
		t.Fatalf("unexpected 32-bit layout: %+v", layout32)
	}
	layout64, err := LayoutForArch(Arch64)
	if err != nil {
		t.Fatal(err)
	}
	if layout64.InfoRecordSize != 384 || layout64.DirmesRecordSize != 224 || layout64.Info.Password != 360 || layout64.Dirmes.Number != 8 {
		t.Fatalf("unexpected 64-bit layout: %+v", layout64)
	}
}

func TestDetectFixtureLayout(t *testing.T) {
	layout, err := DetectLayout(fixtureRoot(t), ArchAuto)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Arch != Arch32 {
		t.Fatalf("detected %d-bit layout, want 32", layout.Arch)
	}
	if _, err := DetectLayout(fixtureRoot(t), Arch64Mode); err == nil {
		t.Fatal("explicit 64-bit layout unexpectedly accepted 32-bit fixtures")
	}
}

func TestParseFixtureUsersAndAuthentication(t *testing.T) {
	root := fixtureRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "inf.sys"))
	if err != nil {
		t.Fatal(err)
	}
	layout, _ := LayoutForArch(Arch32)
	users, err := parseUsers(data, layout)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 8 {
		t.Fatalf("got %d users, want 8", len(users))
	}
	if users[0].Sysop {
		t.Fatalf("unexpected sysop flag for %s", users[0].Callsign)
	}
	var foundSysop bool
	for _, user := range users {
		if user.Callsign == "LW6DIO" {
			foundSysop = user.Sysop
		}
	}
	if !foundSysop {
		t.Fatal("LW6DIO sysop flag was not parsed")
	}
	if users[0].Callsign != "LU4ECL" || users[0].FirstName != "Ernesto" || users[0].password != "ECL1234" {
		t.Fatalf("unexpected first user: %+v", users[0])
	}
	store, err := NewStore(root, ArchAuto)
	if err != nil {
		t.Fatal(err)
	}
	user, ok, err := store.Authenticate("lu4ecl", "ECL1234")
	if err != nil || !ok || user.Callsign != "LU4ECL" {
		t.Fatalf("authentication result: user=%+v ok=%v err=%v", user, ok, err)
	}
	if _, ok, err := store.Authenticate("LU4ECL", "wrong"); err != nil || ok {
		t.Fatalf("wrong password result: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.Authenticate("LU9DCE", ""); err != nil || ok {
		t.Fatalf("empty password user unexpectedly authenticated: ok=%v err=%v", ok, err)
	}
}

func TestParseFixtureMessages(t *testing.T) {
	root := fixtureRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "dirmes.sys"))
	if err != nil {
		t.Fatal(err)
	}
	layout, _ := LayoutForArch(Arch32)
	lastNumber, messages, err := parseDirmes(data, layout)
	if err != nil {
		t.Fatal(err)
	}
	if lastNumber != 1170 {
		t.Fatalf("last message number = %d, want 1170", lastNumber)
	}
	if len(messages) != 1037 {
		t.Fatalf("got %d active messages, want 1037", len(messages))
	}
	if messages[0].Number != 1170 || messages[0].From != "NS2B" || messages[0].Title != "PCL Packet Net" {
		t.Fatalf("unexpected newest message: %+v", messages[0])
	}
	body, err := (&Store{root: root, layout: layout}).ReadMessageBody(110)
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Routing) == 0 || !strings.HasPrefix(body.Routing[0], "R:") || !strings.Contains(body.Body, "Motorola") {
		t.Fatalf("unexpected message body: %+v", body)
	}
}

func TestParse64BitSyntheticRecords(t *testing.T) {
	layout, _ := LayoutForArch(Arch64)
	userRecord := make([]byte, layout.InfoRecordSize)
	copy(userRecord[layout.Info.Callsign:], "LW1EAA")
	userRecord[layout.Info.SSID] = 2
	copy(userRecord[layout.Info.Password:], "secret")
	copy(userRecord[layout.Info.FirstName:], "Tester")
	userRecord[layout.Info.Flags] = byte(userFlagSysop)
	userRecord[layout.Info.Flags+1] = byte(userFlagSysop >> 8)
	users, err := parseUsers(userRecord, layout)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Callsign != "LW1EAA" || users[0].SSID != 2 || users[0].password != "secret" || !users[0].Sysop {
		t.Fatalf("unexpected 64-bit user: %+v", users)
	}
	userRecord[layout.Info.Flags] = 0
	userRecord[layout.Info.Flags+1] = 0
	users, err = parseUsers(userRecord, layout)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Sysop {
		t.Fatalf("unexpected non-sysop 64-bit user: %+v", users)
	}

	dirmes := make([]byte, layout.DirmesRecordSize*2)
	putLong(dirmes[:layout.DirmesRecordSize], layout.Dirmes.Number, layout.LongSize, 42)
	record := dirmes[layout.DirmesRecordSize:]
	record[layout.Dirmes.Type] = 'B'
	record[layout.Dirmes.Status] = 'N'
	putLong(record, layout.Dirmes.Number, layout.LongSize, 42)
	putLong(record, layout.Dirmes.Size, layout.LongSize, 1234)
	putLong(record, layout.Dirmes.Date, layout.LongSize, 1700000000)
	copy(record[layout.Dirmes.From:], "LW1EAA")
	copy(record[layout.Dirmes.Title:], "64-bit title")
	last, messages, err := parseDirmes(dirmes, layout)
	if err != nil {
		t.Fatal(err)
	}
	if last != 42 || len(messages) != 1 || messages[0].Number != 42 || messages[0].Title != "64-bit title" {
		t.Fatalf("unexpected 64-bit messages: last=%d messages=%+v", last, messages)
	}
}

func putLong(record []byte, offset, size int, value int64) {
	for i := 0; i < size; i++ {
		record[offset+i] = byte(uint64(value) >> (8 * i))
	}
}

func TestDecodedFiles(t *testing.T) {
	store, err := NewStore(fixtureRoot(t), Arch32Mode)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := store.DecodedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) < 8 {
		t.Fatalf("got %d decoded groups, want at least 8", len(groups))
	}
	var found bool
	for _, group := range groups {
		if group.Name == "bmwgsr_1" {
			found = true
			if group.Primary == nil || group.Primary.Name != "bmwgsr_1.7mf" || group.Primary.MIME != "image/jpeg" {
				t.Fatalf("unexpected bmwgsr_1 primary: %+v", group.Primary)
			}
		}
	}
	if !found {
		t.Fatal("bmwgsr_1 group not found")
	}
}

func TestComposeValidationAndBodyParsing(t *testing.T) {
	if err := (ComposeRequest{Type: "P", To: "LW6DIO", Title: "Hello", Body: "Line one"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ComposeRequest{Type: "P", To: "LW6DIO", Title: "Hello", Body: "bad\n/EX"}).Validate(); err == nil {
		t.Fatal("/EX body line accepted")
	}
	body := parseMessageBody("R:260925/0959Z route\nR:260925/0958Z route2\n\nHello\nworld\n")
	if len(body.Routing) != 2 || body.Body != "Hello\nworld" {
		t.Fatalf("unexpected parsed body: %+v", body)
	}
}

func TestAppendMail(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mail"), 0750); err != nil {
		t.Fatal(err)
	}
	store := &Store{root: root}
	err := store.AppendMail("LU4ECL", ComposeRequest{
		Type:  "P",
		To:    "LW6DIO",
		Route: "VDM.BA.ARG.SOAM",
		Title: "Test subject",
		Body:  "First line\nSecond line",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "mail", "mail.in"))
	if err != nil {
		t.Fatal(err)
	}
	want := "SP LW6DIO @VDM.BA.ARG.SOAM < LU4ECL\nTest subject\nFirst line\nSecond line\n/EX\n"
	if string(data) != want {
		t.Fatalf("mail.in = %q, want %q", string(data), want)
	}
}
