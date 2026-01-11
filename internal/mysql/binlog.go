package mysql

import (
	"encoding/binary"
	"fmt"
	"os"
)

// BinlogCreatedAt returns the timestamp of the first binlog event.
func BinlogCreatedAt(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return 0, err
	}
	buf := make([]byte, 4)
	if _, err := f.Read(buf); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint32(buf)), nil
}

// BinlogEndPosition returns the last event position in the binlog.
func BinlogEndPosition(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return 0, err
	}
	pos := int64(4)
	for {
		eventHeader := make([]byte, 19)
		if _, err := f.Read(eventHeader); err != nil {
			break
		}
		length := binary.LittleEndian.Uint32(eventHeader[9:13])
		if length < 19 {
			return 0, fmt.Errorf("invalid binlog event length %d", length)
		}
		pos += int64(length)
		if _, err := f.Seek(int64(length-19), 1); err != nil {
			return 0, err
		}
	}
	return pos, nil
}
