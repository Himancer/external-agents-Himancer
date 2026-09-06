package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/entireio/external-agents/agents/entire-agent-universal-memory/internal/memory"
)

type hookInput struct {
	SessionID string `json:"session_id"`
}

type agentSession struct {
	SessionID     string   `json:"session_id"`
	AgentName     string   `json:"agent_name"`
	RepoPath      string   `json:"repo_path"`
	SessionRef    string   `json:"session_ref"`
	StartTime     string   `json:"start_time"`
	NativeData    []byte   `json:"native_data"`
	ModifiedFiles []string `json:"modified_files"`
	NewFiles      []string `json:"new_files"`
	DeletedFiles  []string `json:"deleted_files"`
}

func currentStore() memory.Store {
	root := os.Getenv("ENTIRE_REPO_ROOT")
	if root == "" {
		root, _ = os.Getwd()
	}
	return memory.Store{RepoRoot: root}
}

func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func captureCommand(args []string, stdout io.Writer) error {
	fs := flagSet("capture")
	id := fs.String("session-id", "", "handoff id")
	source := fs.String("source-agent", "", "source agent")
	target := fs.String("target-agent", "deployment-cli", "target agent")
	intent := fs.String("intent", "", "developer intent")
	changes := fs.String("changed", "", "comma-separated structural changes")
	risks := fs.String("risk", "", "comma-separated unresolved risks")
	checkpoint := fs.String("checkpoint-ref", "", "checkpoint or commit evidence reference")
	transcript := fs.String("transcript", "", "legacy or event JSONL transcript path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	context := memory.HandoffContext{
		HandoffID:         *id,
		SourceAgent:       *source,
		TargetAgent:       *target,
		DeveloperIntent:   *intent,
		StructuralChanges: memory.ParseList(*changes),
		UnresolvedRisks:   memory.ParseList(*risks),
		CheckpointRef:     *checkpoint,
	}
	if *transcript != "" {
		data, err := readAnalyzerData(*transcript)
		if err != nil {
			return fmt.Errorf("read transcript: %w", err)
		}
		analysis, err := memory.AnalyzeTranscript(data)
		if err != nil {
			return err
		}
		if context.HandoffID == "" {
			context.HandoffID = analysis.SessionID
		}
		if context.SourceAgent == "" {
			context.SourceAgent = analysis.SourceAgent
		}
		if context.DeveloperIntent == "" {
			context.DeveloperIntent = analysis.DeveloperIntent
		}
		if len(context.StructuralChanges) == 0 {
			context.StructuralChanges = analysisFiles(analysis)
		}
		if len(context.UnresolvedRisks) == 0 {
			context.UnresolvedRisks = analysis.OpenQuestions
		}
		if context.CheckpointRef == "" {
			context.CheckpointRef = analysis.CheckpointRef
		}
		context.ContextStatus = analysis.Status
		context.TranscriptFormat = analysis.Format
		context.ContextWarnings = analysis.Warnings
		if context.SourceAgent == "" && analysis.Format == memory.TranscriptFormatLegacy {
			context.SourceAgent = "legacy-workflow"
			context.ContextWarnings = append(context.ContextWarnings, "legacy transcript did not identify a source agent")
		}
	}
	if context.HandoffID == "" {
		return errors.New("session-id is required (or provide a transcript with session_id)")
	}
	context, err := currentStore().Save(context)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(context)
}

func analysisFiles(analysis memory.TranscriptAnalysis) []string {
	files := make([]string, 0, len(analysis.ModifiedFiles)+len(analysis.NewFiles)+len(analysis.DeletedFiles))
	for _, group := range [][]string{analysis.ModifiedFiles, analysis.NewFiles, analysis.DeletedFiles} {
		for _, file := range group {
			if !contains(files, file) {
				files = append(files, file)
			}
		}
	}
	return files
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func handoffCommand(args []string, stdout io.Writer) error {
	fs := flagSet("handoff")
	id := fs.String("session-id", "", "handoff id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store := currentStore()
	var context memory.HandoffContext
	var err error
	if *id == "" {
		context, err = store.Latest()
	} else {
		context, err = store.Load(*id)
	}
	if errors.Is(err, os.ErrNotExist) {
		return json.NewEncoder(stdout).Encode(memory.HandoffResult{Available: false, Message: "No handoff context is available. Capture coding context before requesting a deployment handoff."})
	}
	if err != nil {
		return err
	}
	message := "Deployment recommendations generated from the captured handoff. No deployment has been executed."
	if context.ContextStatus == memory.TranscriptStatusPartial {
		message = "Deployment recommendations generated from PARTIAL captured context. Review context_warnings before rollout. No deployment has been executed."
	}
	return json.NewEncoder(stdout).Encode(memory.HandoffResult{Available: true, Context: &context, DeploymentPlan: memory.DeploymentPlan(context), Message: message})
}

func getSessionID(stdin io.Reader, stdout io.Writer) error {
	var input hookInput
	if err := json.NewDecoder(stdin).Decode(&input); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(struct {
		SessionID string `json:"session_id"`
	}{input.SessionID})
}

func getSessionDir(args []string, stdout io.Writer) error {
	fs := flagSet("get-session-dir")
	root := fs.String("repo-path", currentStore().RepoRoot, "repo path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(struct {
		SessionDir string `json:"session_dir"`
	}{memory.Store{RepoRoot: *root}.SessionDir()})
}

func resolveSessionFile(args []string, stdout io.Writer) error {
	fs := flagSet("resolve-session-file")
	dir := fs.String("session-dir", "", "session directory")
	id := fs.String("session-id", "", "session id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := currentStore().SessionPath(*id)
	if err != nil {
		return err
	}
	if *dir != "" {
		path = filepath.Join(*dir, filepath.Base(path))
	}
	return json.NewEncoder(stdout).Encode(struct {
		SessionFile string `json:"session_file"`
	}{path})
}

func readSession(stdin io.Reader, stdout io.Writer) error {
	var input hookInput
	if err := json.NewDecoder(stdin).Decode(&input); err != nil {
		return err
	}
	store := currentStore()
	path, err := store.SessionPath(input.SessionID)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		var persisted agentSession
		if err := json.Unmarshal(raw, &persisted); err != nil {
			return err
		}
		if persisted.SessionID == "" {
			return errors.New("protocol session is missing session_id")
		}
		return json.NewEncoder(stdout).Encode(normalizeSession(persisted))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	context, err := store.Load(input.SessionID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(context)
	if err != nil {
		return err
	}
	ref, err := store.SessionPath(context.HandoffID)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(normalizeSession(agentSession{SessionID: context.HandoffID, AgentName: "universal-memory", RepoPath: store.RepoRoot, SessionRef: ref, StartTime: context.CreatedAt, NativeData: data, ModifiedFiles: context.StructuralChanges}))
}

func writeSession(stdin io.Reader) error {
	var session agentSession
	if err := json.NewDecoder(stdin).Decode(&session); err != nil {
		return err
	}
	if len(session.NativeData) == 0 {
		return errors.New("native_data is required")
	}
	var context memory.HandoffContext
	if err := json.Unmarshal(session.NativeData, &context); err == nil && context.DeveloperIntent != "" {
		if context.HandoffID == "" {
			context.HandoffID = session.SessionID
		}
		if len(context.StructuralChanges) == 0 {
			context.StructuralChanges = session.ModifiedFiles
		}
		_, err := currentStore().Save(context)
		return err
	}
	if session.SessionID == "" {
		return errors.New("session_id is required")
	}
	store := currentStore()
	_, err := store.SessionPath(session.SessionID)
	if err != nil {
		return err
	}
	session = normalizeSession(session)
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if memory.HasSensitiveMaterial(string(session.NativeData)) {
		return errors.New("native_data contains sensitive material and must be sanitized before persistence")
	}
	return store.WriteRawSession(session.SessionID, data)
}

func normalizeSession(session agentSession) agentSession {
	if session.AgentName == "" {
		session.AgentName = "universal-memory"
	}
	if session.RepoPath == "" {
		session.RepoPath = currentStore().RepoRoot
	}
	if session.SessionRef == "" {
		if path, err := currentStore().SessionPath(session.SessionID); err == nil {
			session.SessionRef = path
		}
	}
	if session.ModifiedFiles == nil {
		session.ModifiedFiles = []string{}
	}
	if session.NewFiles == nil {
		session.NewFiles = []string{}
	}
	if session.DeletedFiles == nil {
		session.DeletedFiles = []string{}
	}
	return session
}

func readTranscript(args []string, stdout io.Writer) error {
	fs := flagSet("read-transcript")
	ref := fs.String("session-ref", "", "session reference")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ref == "" {
		return errors.New("session-ref is required")
	}
	store := currentStore()
	root, err := filepath.Abs(store.SessionDir())
	if err != nil {
		return err
	}
	path, err := filepath.Abs(*ref)
	if err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return errors.New("session-ref is outside the Universal Agent Memory session directory")
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return err
	}
	_, err = stdout.Write(data)
	return err
}

func getTranscriptPosition(args []string, stdout io.Writer) error {
	fs := flagSet("get-transcript-position")
	path := fs.String("path", "", "transcript path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("path is required")
	}
	data, err := readOptionalAnalyzerData(*path)
	if err != nil {
		return err
	}
	position, err := memory.TranscriptPosition(data)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(struct {
		Position int `json:"position"`
	}{Position: position})
}

func extractModifiedFiles(args []string, stdout io.Writer) error {
	fs := flagSet("extract-modified-files")
	path := fs.String("path", "", "transcript path")
	offset := fs.Int("offset", 0, "already consumed physical record count")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("path is required")
	}
	data, err := readOptionalAnalyzerData(*path)
	if err != nil {
		return err
	}
	analysis, position, err := memory.AnalyzeTranscriptAfterOffset(data, *offset)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(struct {
		Files           []string `json:"files"`
		CurrentPosition int      `json:"current_position"`
	}{Files: analysisFiles(analysis), CurrentPosition: position})
}

func extractPrompts(args []string, stdout io.Writer) error {
	fs := flagSet("extract-prompts")
	ref := fs.String("session-ref", "", "session reference")
	offset := fs.Int("offset", 0, "already consumed physical record count")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ref == "" {
		return errors.New("session-ref is required")
	}
	data, err := readOptionalAnalyzerData(*ref)
	if err != nil {
		return err
	}
	analysis, _, err := memory.AnalyzeTranscriptAfterOffset(data, *offset)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(struct {
		Prompts []string `json:"prompts"`
	}{Prompts: analysis.Prompts})
}

func extractSummary(args []string, stdout io.Writer) error {
	fs := flagSet("extract-summary")
	ref := fs.String("session-ref", "", "session reference")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ref == "" {
		return errors.New("session-ref is required")
	}
	data, err := readOptionalAnalyzerData(*ref)
	if err != nil {
		return err
	}
	analysis, err := memory.AnalyzeTranscript(data)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(struct {
		Summary    string `json:"summary"`
		HasSummary bool   `json:"has_summary"`
	}{Summary: analysis.Summary, HasSummary: analysis.Summary != ""})
}

func readAnalyzerData(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var session agentSession
	if err := json.Unmarshal(data, &session); err == nil && len(session.NativeData) > 0 {
		return session.NativeData, nil
	}
	return data, nil
}

// readOptionalAnalyzerData preserves the transcript-analyzer contract for a
// session that has not created its transcript yet. Capture remains strict and
// uses readAnalyzerData directly.
func readOptionalAnalyzerData(path string) ([]byte, error) {
	data, err := readAnalyzerData(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func chunkTranscript(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flagSet("chunk-transcript")
	maxSize := fs.Int("max-size", 0, "maximum chunk size")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *maxSize <= 0 {
		return errors.New("max-size must be positive")
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	chunks := make([][]byte, 0, (len(data)+*maxSize-1) / *maxSize)
	for len(data) > 0 {
		end := *maxSize
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[:end])
		data = data[end:]
	}
	return json.NewEncoder(stdout).Encode(struct {
		Chunks [][]byte `json:"chunks"`
	}{chunks})
}

func reassembleTranscript(stdin io.Reader, stdout io.Writer) error {
	var input struct {
		Chunks [][]byte `json:"chunks"`
	}
	if err := json.NewDecoder(stdin).Decode(&input); err != nil {
		return err
	}
	for _, chunk := range input.Chunks {
		if _, err := stdout.Write(chunk); err != nil {
			return err
		}
	}
	return nil
}
