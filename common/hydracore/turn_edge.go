package hydracore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The last TURN edge a transport actually reached, kept for a client that wants to measure
// reachability without a worker: a single STUN Binding to the edge needs an address to send
// it to, and the only trustworthy one is an edge the transport itself allocated through.
//
// The value outlives the transport. It is written to a small file in the working directory,
// so a client can ask for it from a later process — one that never ran the transport at all
// — and a client that gets an empty string shows "not measured" rather than guessing an
// address, because no VK authorisation is ever performed just to obtain one.
type turnEdgeRecord struct {
	Endpoint  string `json:"endpoint,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

type turnEdgeStore struct {
	mu       sync.RWMutex
	endpoint string
	path     string
	loaded   bool
}

var turnEdge turnEdgeStore

// SetTurnEdgeStorePath points the store at its file. Called from setup, before any
// transport exists; an empty path keeps the value in memory for this process only.
func SetTurnEdgeStorePath(path string) {
	turnEdge.mu.Lock()
	turnEdge.path = path
	turnEdge.mu.Unlock()
}

// RecordTurnEdgeEndpoint remembers an endpoint the transport reached, and makes a
// best-effort attempt to keep it for later processes. A store that cannot be written
// still serves the running one; the transport must not fail over a diagnostic hint.
func RecordTurnEdgeEndpoint(endpoint string) {
	if endpoint == "" {
		return
	}
	turnEdge.mu.Lock()
	defer turnEdge.mu.Unlock()
	turnEdge.endpoint = endpoint
	turnEdge.loaded = true
	if turnEdge.path == "" {
		return
	}
	record := turnEdgeRecord{Endpoint: endpoint, UpdatedAt: time.Now().UnixMilli()}
	content, err := json.Marshal(record)
	if err != nil {
		return
	}
	directory := filepath.Dir(turnEdge.path)
	if err = os.MkdirAll(directory, 0o755); err != nil {
		return
	}
	temporary := turnEdge.path + ".tmp"
	if err = os.WriteFile(temporary, content, 0o644); err != nil {
		return
	}
	_ = os.Rename(temporary, turnEdge.path)
}

// TurnEdgeEndpoint answers the endpoint a transport last reached, from memory or from the
// file a previous process wrote. Empty means no transport has ever recorded one.
func TurnEdgeEndpoint() string {
	turnEdge.mu.Lock()
	defer turnEdge.mu.Unlock()
	if turnEdge.endpoint == "" && !turnEdge.loaded && turnEdge.path != "" {
		turnEdge.loaded = true
		if content, err := os.ReadFile(turnEdge.path); err == nil {
			var record turnEdgeRecord
			if json.Unmarshal(content, &record) == nil {
				turnEdge.endpoint = record.Endpoint
			}
		}
	}
	return turnEdge.endpoint
}

func resetTurnEdgeStore() {
	turnEdge.mu.Lock()
	turnEdge.endpoint = ""
	turnEdge.loaded = false
	turnEdge.mu.Unlock()
}
