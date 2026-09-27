package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalAppendRecover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "journal.frj")
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{[]byte("one"), []byte("two")} {
		if err := j.Append(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	j, err = OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	got, err := j.Recover()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]byte{[]byte("one"), []byte("two")}
	if len(got) != len(want) {
		t.Fatalf("got %d records want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("record %d got %q want %q", i, got[i], want[i])
		}
	}
}

func TestJournalTornTailIsDiscarded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.frj")
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append([]byte("durable")); err != nil {
		t.Fatal(err)
	}
	if err := j.Append([]byte("torn-tail")); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, info.Size()-7); err != nil {
		t.Fatal(err)
	}

	j, err = OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	records, err := j.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || string(records[0]) != "durable" {
		t.Fatalf("unexpected recovered records: %q", records)
	}

	recoveredSize, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredSize.Size() >= info.Size() {
		t.Fatalf("torn tail was not truncated: recovered=%d original=%d", recoveredSize.Size(), info.Size())
	}
}

func TestJournalChecksumCorruptionFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.frj")
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append([]byte("protected")); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[journalHeaderLen+2] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = OpenJournal(path)
	if !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("expected journal corruption, got %v", err)
	}
}

func TestJournalRejectsEmptyAndOversizedRecord(t *testing.T) {
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err := j.Append(nil); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("expected empty record rejection, got %v", err)
	}
	if err := j.Append(make([]byte, maxJournalRecord+1)); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("expected oversized record rejection, got %v", err)
	}
}
