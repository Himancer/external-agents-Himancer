package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxTranscriptEntries  = 50
	maxTranscriptWarnings = 20
)

// TranscriptFormat identifies the wire format seen in a transcript. The
// legacy format is the original role/content JSONL documented by this agent;
// event is the Track 3 lifecycle-event JSONL format.
type TranscriptFormat string

const (
	TranscriptFormatManual  TranscriptFormat = "manual"
	TranscriptFormatLegacy  TranscriptFormat = "legacy"
	TranscriptFormatEvent   TranscriptFormat = "event"
	TranscriptFormatMixed   TranscriptFormat = "mixed"
	TranscriptFormatUnknown TranscriptFormat = "unknown"
)

// TranscriptStatus tells consumers whether the result contains every record
// that could be read. Partial results are useful, but must not be treated as
// authoritative without reviewing their warnings.
type TranscriptStatus string

const (
	TranscriptStatusComplete TranscriptStatus = "complete"
	TranscriptStatusPartial  TranscriptStatus = "partial"
)

// TranscriptAnalysis is the shared, format-independent representation used by
// capture and transcript-analyzer commands. All text is redacted and bounded
// before it leaves this package.
type TranscriptAnalysis struct {
	Format            TranscriptFormat `json:"format"`
	Status            TranscriptStatus `json:"status"`
	SessionID         string           `json:"session_id,omitempty"`
	SourceAgent       string           `json:"source_agent,omitempty"`
	DeveloperIntent   string           `json:"developer_intent,omitempty"`
	Prompts           []string         `json:"prompts"`
	Summary           string           `json:"summary,omitempty"`
	ModifiedFiles     []string         `json:"modified_files"`
	NewFiles          []string         `json:"new_files"`
	DeletedFiles      []string         `json:"deleted_files"`
	CheckpointRef     string           `json:"checkpoint_ref,omitempty"`
	OpenQuestions     []string         `json:"open_questions"`
	UnknownEventCount int              `json:"unknown_event_count"`
	Warnings          []string         `json:"warnings"`
}

type normalizedEvent struct {
	format        TranscriptFormat
	kind          string
	sessionID     string
	sourceAgent   string
	text          string
	path          string
	change        string
	summary       string
	intent        string
	checkpointRef string
	questions     []string
	modifiedFiles []string
	newFiles      []string
	deletedFiles  []string
}

type transcriptProbe struct {
	Event string `json:"event"`
	Role  string `json:"role"`
}

type legacyRecord struct {
	SessionID     string   `json:"session_id"`
	Role          string   `json:"role"`
	Content       string   `json:"content"`
	ModifiedFiles []string `json:"modified_files"`
	NewFiles      []string `json:"new_files"`
	DeletedFiles  []string `json:"deleted_files"`
	CheckpointRef string   `json:"checkpoint_ref"`
	Summary       string   `json:"summary"`
}

type eventRecord struct {
	Event         string   `json:"event"`
	SessionID     string   `json:"session_id"`
	Text          string   `json:"text"`
	Path          string   `json:"path"`
	Change        string   `json:"change"`
	CheckpointID  string   `json:"checkpoint_id"`
	Summary       string   `json:"summary"`
	Intent        string   `json:"intent"`
	OpenQuestions []string `json:"open_questions"`
	Agent         struct {
		Name string `json:"name"`
	} `json:"agent"`
}

// AnalyzeTranscript supports both the original role/content JSONL format and
// the Track 3 event JSONL format. It deliberately keeps all format-specific
// parsing at this boundary and applies one common normalization path.
func AnalyzeTranscript(data []byte) (TranscriptAnalysis, error) {
	analysis, _, err := AnalyzeTranscriptAfterOffset(data, 0)
	return analysis, err
}

// AnalyzeTranscriptAfterOffset analyzes physical JSONL records after offset.
// The returned position counts complete, non-empty records and is deliberately
// the same unit consumed by the offset argument. A terminal partial record is
// excluded so a future incremental read can process it after completion.
func AnalyzeTranscriptAfterOffset(data []byte, offset int) (TranscriptAnalysis, int, error) {
	if offset < 0 {
		return TranscriptAnalysis{}, 0, fmt.Errorf("offset must not be negative")
	}
	analysis := TranscriptAnalysis{
		Status:        TranscriptStatusComplete,
		Prompts:       []string{},
		ModifiedFiles: []string{},
		NewFiles:      []string{},
		DeletedFiles:  []string{},
		OpenQuestions: []string{},
		Warnings:      []string{},
	}
	if len(bytes.TrimSpace(data)) == 0 {
		analysis.Format = TranscriptFormatUnknown
		analysis.Status = TranscriptStatusPartial
		analysis.addWarning("transcript is empty")
		return analysis, 0, nil
	}

	lines := bytes.Split(data, []byte("\n"))
	lastRecord := -1
	for i, line := range lines {
		if len(bytes.TrimSpace(line)) > 0 {
			lastRecord = i
		}
	}
	position := 0
	for i, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		candidatePosition := position + 1
		if candidatePosition <= offset {
			position = candidatePosition
			continue
		}
		event, err := decodeTranscriptRecord(line)
		if err != nil {
			// A terminal record can be cut short even when its writer has already
			// emitted a newline. Do not advance the position for it: an
			// incremental caller must be able to read the completed record on a
			// later attempt.
			if i == lastRecord {
				analysis.Status = TranscriptStatusPartial
				analysis.addWarning(fmt.Sprintf("truncated final JSONL record at line %d", i+1))
				break
			}
			return TranscriptAnalysis{}, position, fmt.Errorf("decode transcript line %d: %w", i+1, err)
		}
		position = candidatePosition
		analysis.apply(event)
	}

	if analysis.Format == "" {
		analysis.Format = TranscriptFormatUnknown
		if position > offset {
			analysis.Status = TranscriptStatusPartial
			analysis.addWarning("transcript contained no recognized records")
		}
	}
	if analysis.DeveloperIntent == "" && len(analysis.Prompts) > 0 {
		analysis.DeveloperIntent = analysis.Prompts[0]
	}
	return analysis, position, nil
}

// TranscriptPosition returns the completed-record count used by the analyzer
// offset commands. Blank lines and a terminal partial record do not advance it.
func TranscriptPosition(data []byte) (int, error) {
	_, position, err := AnalyzeTranscriptAfterOffset(data, 0)
	return position, err
}

func decodeTranscriptRecord(line []byte) (normalizedEvent, error) {
	var probe transcriptProbe
	if err := json.Unmarshal(line, &probe); err != nil {
		return normalizedEvent{}, err
	}
	if probe.Event != "" {
		var record eventRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return normalizedEvent{}, err
		}
		return normalizedEvent{
			format:        TranscriptFormatEvent,
			kind:          strings.ToLower(strings.TrimSpace(record.Event)),
			sessionID:     record.SessionID,
			sourceAgent:   record.Agent.Name,
			text:          record.Text,
			path:          record.Path,
			change:        record.Change,
			summary:       record.Summary,
			intent:        record.Intent,
			checkpointRef: record.CheckpointID,
			questions:     record.OpenQuestions,
		}, nil
	}
	if probe.Role != "" {
		var record legacyRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return normalizedEvent{}, err
		}
		return normalizedEvent{
			format:        TranscriptFormatLegacy,
			kind:          "legacy_" + strings.ToLower(strings.TrimSpace(record.Role)),
			sessionID:     record.SessionID,
			text:          record.Content,
			summary:       record.Summary,
			checkpointRef: record.CheckpointRef,
			modifiedFiles: record.ModifiedFiles,
			newFiles:      record.NewFiles,
			deletedFiles:  record.DeletedFiles,
		}, nil
	}
	return normalizedEvent{}, fmt.Errorf("record has neither event nor role")
}

func (analysis *TranscriptAnalysis) apply(event normalizedEvent) {
	analysis.recordFormat(event.format)
	analysis.recordSessionID(event.sessionID)
	if analysis.SourceAgent == "" {
		analysis.SourceAgent = analysis.cleanText(event.sourceAgent, maxItemBytes)
	}
	for _, path := range event.modifiedFiles {
		analysis.ModifiedFiles = analysis.appendPath(analysis.ModifiedFiles, path)
	}
	for _, path := range event.newFiles {
		analysis.NewFiles = analysis.appendPath(analysis.NewFiles, path)
	}
	for _, path := range event.deletedFiles {
		analysis.DeletedFiles = analysis.appendPath(analysis.DeletedFiles, path)
	}
	if checkpointRef := analysis.cleanText(event.checkpointRef, maxItemBytes); checkpointRef != "" {
		// Checkpoints are ordered events; the latest one is the best resume
		// evidence for a handoff.
		analysis.CheckpointRef = checkpointRef
	}
	if summary := analysis.cleanText(event.summary, maxIntentBytes); summary != "" {
		analysis.Summary = summary
	}

	switch event.kind {
	case "legacy_user", "user_prompt":
		analysis.Prompts = analysis.appendText(analysis.Prompts, event.text, maxIntentBytes)
	case "legacy_assistant", "agent_response":
		if text := analysis.cleanText(event.text, maxIntentBytes); text != "" {
			analysis.Summary = text
		}
	case "file_changed":
		switch strings.ToLower(strings.TrimSpace(event.change)) {
		case "new", "created", "added":
			analysis.NewFiles = analysis.appendPath(analysis.NewFiles, event.path)
		case "deleted", "removed":
			analysis.DeletedFiles = analysis.appendPath(analysis.DeletedFiles, event.path)
		default:
			analysis.ModifiedFiles = analysis.appendPath(analysis.ModifiedFiles, event.path)
		}
	case "checkpoint_created":
		if summary := analysis.cleanText(event.summary, maxIntentBytes); summary != "" {
			analysis.Summary = summary
		}
		if intent := analysis.cleanText(event.intent, maxIntentBytes); intent != "" {
			analysis.DeveloperIntent = intent
		}
		for _, question := range event.questions {
			analysis.OpenQuestions = analysis.appendText(analysis.OpenQuestions, question, maxItemBytes)
		}
	case "session_started", "session_ended", "tool_call", "tool_result", "file_read", "usage":
		// These records contribute session continuity but not handoff content.
	default:
		analysis.UnknownEventCount++
		analysis.addWarning("unknown event ignored: " + event.kind)
	}
}

func (analysis *TranscriptAnalysis) recordFormat(format TranscriptFormat) {
	if analysis.Format == "" {
		analysis.Format = format
		return
	}
	if analysis.Format != format && analysis.Format != TranscriptFormatMixed {
		analysis.Format = TranscriptFormatMixed
		analysis.Status = TranscriptStatusPartial
		analysis.addWarning("mixed transcript formats require verification")
	}
}

func (analysis *TranscriptAnalysis) recordSessionID(id string) {
	id = analysis.cleanText(id, maxItemBytes)
	if id == "" {
		return
	}
	if analysis.SessionID == "" {
		analysis.SessionID = id
		return
	}
	if analysis.SessionID != id {
		analysis.Status = TranscriptStatusPartial
		analysis.addWarning("multiple session IDs observed")
	}
}

func (analysis *TranscriptAnalysis) cleanText(value string, maxBytes int) string {
	return limit(strings.TrimSpace(Redact(value)), maxBytes)
}

func (analysis *TranscriptAnalysis) appendText(values []string, value string, maxBytes int) []string {
	value = analysis.cleanText(value, maxBytes)
	if value == "" || containsString(values, value) || len(values) >= maxTranscriptEntries {
		return values
	}
	return append(values, value)
}

func (analysis *TranscriptAnalysis) appendPath(values []string, value string) []string {
	value = strings.TrimSpace(Redact(value))
	if value == "" || containsString(values, value) || len(values) >= maxTranscriptEntries {
		return values
	}
	return append(values, limit(value, maxItemBytes))
}

func (analysis *TranscriptAnalysis) addWarning(value string) {
	value = limit(strings.TrimSpace(Redact(value)), maxItemBytes)
	if value == "" || containsString(analysis.Warnings, value) || len(analysis.Warnings) >= maxTranscriptWarnings {
		return
	}
	analysis.Warnings = append(analysis.Warnings, value)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validTranscriptFormat(format TranscriptFormat) bool {
	switch format {
	case TranscriptFormatManual, TranscriptFormatLegacy, TranscriptFormatEvent, TranscriptFormatMixed, TranscriptFormatUnknown:
		return true
	default:
		return false
	}
}
