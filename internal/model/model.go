package model

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"
)

type Settings struct {
	AutoRemember        bool `json:"autoRemember"`
	AutoCapture         bool `json:"autoCapture"`
	ScanIntervalSeconds int  `json:"scanIntervalSeconds"`
}

type Project struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Root           string    `json:"root,omitempty"`
	WorkingDir     string    `json:"workingDir,omitempty"`
	StartCommand   string    `json:"startCommand,omitempty"`
	Framework      string    `json:"framework,omitempty"`
	Runtime        string    `json:"runtime,omitempty"`
	URL            string    `json:"url,omitempty"`
	Host           string    `json:"host,omitempty"`
	Port           int       `json:"port,omitempty"`
	PID            int       `json:"pid,omitempty"`
	Status         string    `json:"status"`
	Managed        bool      `json:"managed"`
	AutoDiscovered bool      `json:"autoDiscovered"`
	Confidence     int       `json:"confidence"`
	CoverFile      string    `json:"coverFile,omitempty"`
	CoverURL       string    `json:"coverUrl,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LastSeen       time.Time `json:"lastSeen,omitempty"`
	LastStarted    time.Time `json:"lastStarted,omitempty"`
	LastCaptured   time.Time `json:"lastCaptured,omitempty"`
}

type State struct {
	Version  int                 `json:"version"`
	Settings Settings            `json:"settings"`
	Projects map[string]*Project `json:"projects"`
	Ignored  map[string]bool     `json:"ignored,omitempty"`
}

func StableID(parts ...string) string {
	normalized := strings.Join(parts, "|")
	sum := sha1.Sum([]byte(normalized))
	return hex.EncodeToString(sum[:])[:12]
}
