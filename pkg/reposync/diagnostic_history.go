// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// HistorySnapshot represents a point-in-time health check result.
type HistorySnapshot struct {
	Timestamp time.Time     `json:"timestamp"`
	Report    *HealthReport `json:"report"`
}

// HistoryStore manages historical health check snapshots.
type HistoryStore interface {
	Save(ctx context.Context, report *HealthReport) error
	Load(ctx context.Context, limit int) ([]HistorySnapshot, error)
	GetTrend(ctx context.Context, repoName string) ([]RepoHealth, error)
}

// FileHistoryStore stores health snapshots in JSON files.
type FileHistoryStore struct {
	BaseDir string
}

// NewFileHistoryStore creates a history store that saves to a directory.
func NewFileHistoryStore(baseDir string) *FileHistoryStore {
	return &FileHistoryStore{BaseDir: baseDir}
}

// Save stores a health report snapshot.
func (s *FileHistoryStore) Save(ctx context.Context, report *HealthReport) error {
	if report == nil {
		return fmt.Errorf("report is nil")
	}

	// Ensure base directory exists
	if err := os.MkdirAll(s.BaseDir, 0o750); err != nil {
		return fmt.Errorf("create history directory: %w", err)
	}

	snapshot := HistorySnapshot{
		Timestamp: report.CheckedAt,
		Report:    report,
	}

	// File name: YYYYMMDD-HHMMSS.json
	filename := snapshot.Timestamp.Format("20060102-150405") + ".json"
	snapshotPath := filepath.Join(s.BaseDir, filename)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	if err := os.WriteFile(snapshotPath, data, 0o600); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	return nil
}

// Load retrieves the most recent N snapshots.
func (s *FileHistoryStore) Load(ctx context.Context, limit int) ([]HistorySnapshot, error) {
	entries, err := os.ReadDir(s.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []HistorySnapshot{}, nil
		}
		return nil, fmt.Errorf("read history directory: %w", err)
	}

	// Sort by filename (descending, most recent first)
	var snapshots []HistorySnapshot
	count := 0

	// Read files in reverse order (most recent first)
	for i := len(entries) - 1; i >= 0 && count < limit; i-- {
		entry := entries[i]
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		entryPath := filepath.Join(s.BaseDir, entry.Name())
		data, err := os.ReadFile(entryPath) // #nosec G304 -- entryPath is constrained to the diagnostic history base directory.
		if err != nil {
			continue // Skip unreadable files
		}

		var snapshot HistorySnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue // Skip invalid files
		}

		snapshots = append(snapshots, snapshot)
		count++
	}

	return snapshots, nil
}

// GetTrend retrieves health history for a specific repository.
func (s *FileHistoryStore) GetTrend(ctx context.Context, repoName string) ([]RepoHealth, error) {
	snapshots, err := s.Load(ctx, 100) // Load last 100 snapshots
	if err != nil {
		return nil, err
	}

	var trend []RepoHealth
	for _, snapshot := range snapshots {
		if snapshot.Report == nil {
			continue
		}

		for _, health := range snapshot.Report.Results {
			// Match by name or path
			name := health.Repo.Name
			if name == "" {
				name = health.Repo.TargetPath
			}

			if name == repoName || health.Repo.TargetPath == repoName {
				trend = append(trend, health)
				break
			}
		}
	}

	return trend, nil
}

// CleanupOld removes snapshots older than the specified duration.
func (s *FileHistoryStore) CleanupOld(ctx context.Context, olderThan time.Duration) error {
	entries, err := os.ReadDir(s.BaseDir)
	if err != nil {
		return fmt.Errorf("read history directory: %w", err)
	}

	cutoff := time.Now().Add(-olderThan)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			removePath := filepath.Join(s.BaseDir, entry.Name())
			if err := os.Remove(removePath); err == nil {
				removed++
			}
		}
	}

	return nil
}
