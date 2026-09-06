package memory

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxIntentBytes = 4096
	maxItemBytes   = 512
	maxChanges     = 20
	maxRisks       = 10
)

var safeSessionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// HandoffContext is the bounded payload transferred from a coding workflow to
// a deployment workflow. CheckpointRef is an evidence link, not a claim that
// Entire supports arbitrary checkpoint metadata.
type HandoffContext struct {
	HandoffID         string   `json:"handoff_id"`
	SourceAgent       string   `json:"source_agent"`
	TargetAgent       string   `json:"target_agent,omitempty"`
	DeveloperIntent   string   `json:"developer_intent"`
	StructuralChanges []string `json:"structural_changes"`
	UnresolvedRisks   []string `json:"unresolved_risks,omitempty"`
	CheckpointRef     string   `json:"checkpoint_ref,omitempty"`
	CreatedAt         string   `json:"created_at"`
}

// HandoffResult is the deployment-side response. It explicitly states that it
// is a recommendation rather than an executed deployment.
type HandoffResult struct {
	Available      bool            `json:"available"`
	Context        *HandoffContext `json:"context,omitempty"`
	DeploymentPlan []string        `json:"deployment_plan,omitempty"`
	Message        string          `json:"message"`
}

// Store owns separate protocol-session and handoff namespaces below one
// repository's .entire directory. They must not share filenames because an
// Entire protocol session and a deployment handoff may legitimately use the
// same session identifier.
type Store struct{ RepoRoot string }

// SessionDir contains opaque protocol-native session envelopes.
func (s Store) SessionDir() string {
	return filepath.Join(s.RepoRoot, ".entire", "universal-memory", "sessions")
}

// HandoffDir contains validated coding-to-deployment handoff packets.
func (s Store) HandoffDir() string {
	return filepath.Join(s.RepoRoot, ".entire", "universal-memory", "handoffs")
}

func (s Store) SessionPath(id string) (string, error) {
	if !safeSessionID.MatchString(id) {
		return "", fmt.Errorf("invalid session id")
	}
	return filepath.Join(s.SessionDir(), id+".json"), nil
}

func (s Store) HandoffPath(id string) (string, error) {
	if !safeSessionID.MatchString(id) {
		return "", fmt.Errorf("invalid handoff id")
	}
	return filepath.Join(s.HandoffDir(), id+".json"), nil
}

func NewHandoffID() (string, error) {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate handoff id: %w", err)
	}
	return "handoff-" + hex.EncodeToString(data), nil
}

// Sanitize validates, redacts, and bounds user-controlled handoff data before
// it reaches disk or another agent.
func Sanitize(context HandoffContext) (HandoffContext, error) {
	if containsUnsafe(context) {
		return HandoffContext{}, errors.New("handoff contains unsafe material that must be removed before capture")
	}
	if strings.TrimSpace(context.SourceAgent) == "" {
		return HandoffContext{}, errors.New("source_agent is required")
	}
	if strings.TrimSpace(context.DeveloperIntent) == "" {
		return HandoffContext{}, errors.New("developer_intent is required")
	}
	if context.HandoffID == "" {
		id, err := NewHandoffID()
		if err != nil {
			return HandoffContext{}, err
		}
		context.HandoffID = id
	}
	if !safeSessionID.MatchString(context.HandoffID) {
		return HandoffContext{}, errors.New("invalid handoff_id")
	}

	context.SourceAgent = limit(Redact(strings.TrimSpace(context.SourceAgent)), maxItemBytes)
	context.TargetAgent = limit(Redact(strings.TrimSpace(context.TargetAgent)), maxItemBytes)
	context.DeveloperIntent = limit(Redact(strings.TrimSpace(context.DeveloperIntent)), maxIntentBytes)
	context.CheckpointRef = limit(Redact(strings.TrimSpace(context.CheckpointRef)), maxItemBytes)
	context.StructuralChanges = sanitizeList(context.StructuralChanges, maxChanges)
	context.UnresolvedRisks = sanitizeList(context.UnresolvedRisks, maxRisks)
	if len(context.StructuralChanges) == 0 {
		return HandoffContext{}, errors.New("structural_changes is required")
	}
	if context.CreatedAt == "" {
		context.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return context, nil
}

func containsUnsafe(context HandoffContext) bool {
	values := append([]string{context.SourceAgent, context.TargetAgent, context.DeveloperIntent, context.CheckpointRef}, context.StructuralChanges...)
	values = append(values, context.UnresolvedRisks...)
	for _, value := range values {
		if strings.Contains(value, "\x00") || strings.Contains(value, "-----BEGIN") {
			return true
		}
	}
	return false
}

func sanitizeList(items []string, maximum int) []string {
	result := make([]string, 0, min(len(items), maximum))
	for _, item := range items {
		if len(result) == maximum {
			break
		}
		if cleaned := strings.TrimSpace(Redact(item)); cleaned != "" {
			result = append(result, limit(cleaned, maxItemBytes))
		}
	}
	return result
}

func limit(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	limit := maxBytes - 3
	var builder strings.Builder
	for _, r := range value {
		if builder.Len()+utf8.RuneLen(r) > limit {
			break
		}
		builder.WriteRune(r)
	}
	return builder.String() + "..."
}

func (s Store) Save(context HandoffContext) (HandoffContext, error) {
	clean, err := Sanitize(context)
	if err != nil {
		return HandoffContext{}, err
	}
	if err := os.MkdirAll(s.HandoffDir(), 0o700); err != nil {
		return HandoffContext{}, fmt.Errorf("create handoff directory: %w", err)
	}
	path, err := s.HandoffPath(clean.HandoffID)
	if err != nil {
		return HandoffContext{}, err
	}
	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return HandoffContext{}, fmt.Errorf("encode handoff: %w", err)
	}
	temp, err := os.CreateTemp(s.HandoffDir(), ".handoff-*")
	if err != nil {
		return HandoffContext{}, fmt.Errorf("create temporary handoff: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return HandoffContext{}, fmt.Errorf("write handoff: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return HandoffContext{}, fmt.Errorf("protect handoff: %w", err)
	}
	if err := temp.Close(); err != nil {
		return HandoffContext{}, fmt.Errorf("close handoff: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return HandoffContext{}, fmt.Errorf("save handoff: %w", err)
	}
	return clean, nil
}

func (s Store) Load(id string) (HandoffContext, error) {
	path, err := s.HandoffPath(id)
	if err != nil {
		return HandoffContext{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return HandoffContext{}, fmt.Errorf("read handoff: %w", err)
	}
	var context HandoffContext
	if err := json.Unmarshal(data, &context); err != nil {
		return HandoffContext{}, fmt.Errorf("decode handoff: %w", err)
	}
	return Sanitize(context)
}

func (s Store) Latest() (HandoffContext, error) {
	entries, err := os.ReadDir(s.HandoffDir())
	if err != nil {
		return HandoffContext{}, fmt.Errorf("read session directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		left, leftErr := entries[i].Info()
		right, rightErr := entries[j].Info()
		if leftErr != nil || rightErr != nil {
			return entries[i].Name() > entries[j].Name()
		}
		return left.ModTime().After(right.ModTime())
	})
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		context, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err == nil {
			return context, nil
		}
	}
	return HandoffContext{}, os.ErrNotExist
}

func (s Store) ReadRaw(id string) ([]byte, error) {
	path, err := s.SessionPath(id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// WriteRawSession persists opaque protocol-native data atomically. The caller
// is responsible for ensuring that the data is safe to retain.
func (s Store) WriteRawSession(id string, data []byte) error {
	if _, err := s.SessionPath(id); err != nil {
		return err
	}
	if err := os.MkdirAll(s.SessionDir(), 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}
	path, _ := s.SessionPath(id)
	temp, err := os.CreateTemp(s.SessionDir(), ".session-*")
	if err != nil {
		return fmt.Errorf("create temporary session: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write session: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("protect session: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close session: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func DeploymentPlan(context HandoffContext) []string {
	plan := []string{"Review the handoff and confirm environment configuration."}
	changes := strings.ToLower(strings.Join(context.StructuralChanges, " "))
	if strings.Contains(changes, "redis") {
		plan = append(plan, "Provision or verify a Redis service and configure its connection settings.")
	}
	if strings.Contains(changes, "migration") || strings.Contains(changes, "schema") || strings.Contains(changes, "database") {
		plan = append(plan, "Run the required database migration before rollout.")
	}
	if strings.Contains(changes, "docker") || strings.Contains(changes, "compose") {
		plan = append(plan, "Validate container service definitions before rollout.")
	}
	plan = append(plan, "Run application health checks after the planned rollout.")
	return plan
}

func ParseList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, ",")
}
