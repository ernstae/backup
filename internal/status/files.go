package status

import "sort"

// FileBackup represents a filesystem backup copy.
type FileBackup struct {
	Key        string `json:"key"`
	Host       string `json:"host"`
	Interval   string `json:"interval"`
	Name       string `json:"name"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	Bytes      int64  `json:"bytes"`
}

// FilesStatus stores file backup metadata.
type FilesStatus struct {
	Version int          `json:"version"`
	Backups []FileBackup `json:"backups"`
}

func NewFilesStatus() *FilesStatus {
	return &FilesStatus{Version: Version, Backups: []FileBackup{}}
}

func (s *FilesStatus) Add(backup FileBackup) {
	s.Backups = append(s.Backups, backup)
	s.sort()
}

func (s *FilesStatus) Latest() *FileBackup {
	if len(s.Backups) == 0 {
		return nil
	}
	return &s.Backups[len(s.Backups)-1]
}

func (s *FilesStatus) sort() {
	sort.Slice(s.Backups, func(i, j int) bool {
		return s.Backups[i].StartedAt < s.Backups[j].StartedAt
	})
}
