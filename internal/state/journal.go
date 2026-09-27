package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	journalMagic     = "FRJ1"
	journalHeaderLen = 8
	journalDigestLen = sha256.Size
	maxJournalRecord = 4 << 20
)

var (
	ErrJournalCorrupt = errors.New("journal corrupt")
	ErrJournalLocked  = errors.New("journal already has an active writer")
)

type Journal struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func OpenJournal(path string) (*Journal, error) {
	if path == "" {
		return nil, fmt.Errorf("journal path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create journal directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	if err := lockJournalFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("%w: %v", ErrJournalLocked, err)
	}
	j := &Journal{file: f, path: path}
	if _, err := j.Recover(); err != nil {
		_ = unlockJournalFile(f)
		f.Close()
		return nil, err
	}
	return j, nil
}

func (j *Journal) Path() string {
	return j.path
}

func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file == nil {
		return nil
	}
	unlockErr := unlockJournalFile(j.file)
	closeErr := j.file.Close()
	j.file = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func (j *Journal) Append(payload []byte) error {
	if len(payload) == 0 || len(payload) > maxJournalRecord {
		return fmt.Errorf("%w: invalid record length %d", ErrJournalCorrupt, len(payload))
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file == nil {
		return os.ErrClosed
	}

	frame := make([]byte, journalHeaderLen+len(payload)+journalDigestLen)
	copy(frame[:4], journalMagic)
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:8+len(payload)], payload)
	sum := sha256.Sum256(payload)
	copy(frame[8+len(payload):], sum[:])

	if _, err := j.file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek journal end: %w", err)
	}
	if err := writeFull(j.file, frame); err != nil {
		return fmt.Errorf("append journal: %w", err)
	}
	if err := j.file.Sync(); err != nil {
		return fmt.Errorf("fsync journal: %w", err)
	}
	return nil
}

func (j *Journal) Recover() ([][]byte, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.recoverLocked()
}

func (j *Journal) recoverLocked() ([][]byte, error) {
	if j.file == nil {
		return nil, os.ErrClosed
	}
	if _, err := j.file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek journal start: %w", err)
	}

	var records [][]byte
	var goodOffset int64
	header := make([]byte, journalHeaderLen)

	for {
		recordOffset, err := j.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}

		n, err := io.ReadFull(j.file, header)
		if errors.Is(err, io.EOF) && n == 0 {
			goodOffset = recordOffset
			break
		}
		if errors.Is(err, io.ErrUnexpectedEOF) || (errors.Is(err, io.EOF) && n > 0) {
			if err := j.truncateLocked(recordOffset); err != nil {
				return nil, err
			}
			goodOffset = recordOffset
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read journal header: %w", err)
		}
		if !bytes.Equal(header[:4], []byte(journalMagic)) {
			return nil, fmt.Errorf("%w: bad magic at offset %d", ErrJournalCorrupt, recordOffset)
		}

		length := int(binary.BigEndian.Uint32(header[4:8]))
		if length <= 0 || length > maxJournalRecord {
			return nil, fmt.Errorf("%w: invalid length %d at offset %d", ErrJournalCorrupt, length, recordOffset)
		}

		body := make([]byte, length+journalDigestLen)
		n, err = io.ReadFull(j.file, body)
		if errors.Is(err, io.ErrUnexpectedEOF) || (errors.Is(err, io.EOF) && n < len(body)) {
			if err := j.truncateLocked(recordOffset); err != nil {
				return nil, err
			}
			goodOffset = recordOffset
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read journal record: %w", err)
		}

		payload := body[:length]
		want := body[length:]
		got := sha256.Sum256(payload)
		if !bytes.Equal(want, got[:]) {
			return nil, fmt.Errorf("%w: checksum mismatch at offset %d", ErrJournalCorrupt, recordOffset)
		}

		record := append([]byte(nil), payload...)
		records = append(records, record)
		goodOffset = recordOffset + int64(journalHeaderLen+len(body))
	}

	if _, err := j.file.Seek(goodOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek recovered journal end: %w", err)
	}
	return records, nil
}

func (j *Journal) truncateLocked(size int64) error {
	if err := j.file.Truncate(size); err != nil {
		return fmt.Errorf("truncate torn journal tail: %w", err)
	}
	if err := j.file.Sync(); err != nil {
		return fmt.Errorf("fsync truncated journal: %w", err)
	}
	return nil
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
