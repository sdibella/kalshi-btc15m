package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/sdibella/kalshi-btc15m/internal/journal"
)

// Reader reads and parses journal JSONL files.
type Reader struct {
	cfg Config
}

// NewReader creates a new journal reader with the given configuration.
func NewReader(cfg Config) *Reader {
	return &Reader{cfg: cfg}
}

// Config returns the reader's configuration.
func (r *Reader) Config() Config {
	return r.cfg
}

// journalFilePattern matches journal-YYYYMMDD-HHMMSS.jsonl
var journalFilePattern = regexp.MustCompile(`^journal-(\d{8})-(\d{6})\.jsonl$`)

// DiscoverSessions scans the journal directory and returns SessionInfo for all
// journal files, sorted by timestamp descending (newest first).
func (r *Reader) DiscoverSessions() ([]SessionInfo, error) {
	entries, err := os.ReadDir(r.cfg.JournalDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read journal directory %s: %w", r.cfg.JournalDir, err)
	}

	var sessions []SessionInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		matches := journalFilePattern.FindStringSubmatch(filename)
		if matches == nil {
			continue
		}

		dateStr := matches[1]
		timeStr := matches[2]
		combined := dateStr + timeStr
		startTime, err := time.Parse("20060102150405", combined)
		if err != nil {
			continue
		}

		display := startTime.Format("Jan 2, 3:04 PM")

		sessions = append(sessions, SessionInfo{
			Filename:  filename,
			StartTime: startTime,
			Display:   display,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions, nil
}

// Event represents a parsed journal event with type-specific fields.
type Event struct {
	Type       string
	SessionStart *journal.SessionStart
	Trade        *journal.Trade
	Settlement   *journal.Settlement
}

// ParseJournal reads a JSONL journal file and parses all events.
func (r *Reader) ParseJournal(filename string) ([]Event, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open journal file %s: %w", filename, err)
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		if len(line) == 0 {
			continue
		}

		var typeOnly struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &typeOnly); err != nil {
			return nil, fmt.Errorf("failed to parse type field at line %d: %w", lineNum, err)
		}

		var event Event
		event.Type = typeOnly.Type

		switch typeOnly.Type {
		case "session_start":
			var ss journal.SessionStart
			if err := json.Unmarshal(line, &ss); err != nil {
				return nil, fmt.Errorf("failed to parse session_start at line %d: %w", lineNum, err)
			}
			event.SessionStart = &ss

		case "trade":
			var tr journal.Trade
			if err := json.Unmarshal(line, &tr); err != nil {
				return nil, fmt.Errorf("failed to parse trade at line %d: %w", lineNum, err)
			}
			event.Trade = &tr

		case "settlement":
			var st journal.Settlement
			if err := json.Unmarshal(line, &st); err != nil {
				return nil, fmt.Errorf("failed to parse settlement at line %d: %w", lineNum, err)
			}
			event.Settlement = &st

		default:
			continue
		}

		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading journal file: %w", err)
	}

	return events, nil
}

// ParseAllSessions discovers all journal sessions and parses them,
// returning aggregated events from all sessions sorted chronologically.
func (r *Reader) ParseAllSessions() ([]Event, error) {
	sessions, err := r.DiscoverSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to discover sessions: %w", err)
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no journal sessions found")
	}

	var allEvents []Event

	for i := len(sessions) - 1; i >= 0; i-- {
		session := sessions[i]
		journalPath := fmt.Sprintf("%s/%s", r.cfg.JournalDir, session.Filename)

		events, err := r.ParseJournal(journalPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", session.Filename, err)
			continue
		}

		allEvents = append(allEvents, events...)
	}

	return allEvents, nil
}
