package profileplugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"

	"github.com/wmentor/go-magnetar/internal/config"
	"github.com/wmentor/go-magnetar/internal/plugin"
)

type fakeAgentHandle struct {
	cfg         *config.Config
	messages    int
	reconfigured int
}

func (h *fakeAgentHandle) Messages() []openai.ChatCompletionMessage {
	return make([]openai.ChatCompletionMessage, h.messages)
}
func (h *fakeAgentHandle) SetMessages([]openai.ChatCompletionMessage) {}
func (h *fakeAgentHandle) Config() *config.Config                    { return h.cfg }
func (h *fakeAgentHandle) Reconfigure()                              { h.reconfigured++ }
func (h *fakeAgentHandle) Compact() error                            { return nil }
func (h *fakeAgentHandle) Reset()                                    {}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	cfgYaml := "version: \"1.0\"\nprofile: main\nllm:\n  model: base-model\nprofiles:\n  main:\n    llm:\n      model: main-model\n  fast:\n    llm:\n      model: fast-model\n"
	if err := os.WriteFile(path, []byte(cfgYaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func registeredCommand(t *testing.T) plugin.ChatCommand {
	t.Helper()
	hub := &captureHub{}
	if err := (&Plugin{}).Init(nil, hub); err != nil {
		t.Fatal(err)
	}
	return hub.command
}

type captureHub struct {
	plugin.Hub
	command plugin.ChatCommand
}

func (h *captureHub) RegisterChatCommand(command plugin.ChatCommand) {
	h.command = command
}

func TestProfileRegistration(t *testing.T) {
	t.Parallel()
	command := registeredCommand(t)
	if command.Name != "profile" || command.Execute == nil {
		t.Fatalf("unexpected profile registration: %+v", command)
	}
	if !strings.Contains(command.Help, "profile") {
		t.Fatalf("help should mention profile: %q", command.Help)
	}
}

func TestProfileListingUsesUnifiedOutput(t *testing.T) {
	t.Parallel()
	command := registeredCommand(t)
	handler := &fakeAgentHandle{cfg: testConfig(t), messages: 3}

	response, err := command.Execute(context.Background(), &plugin.ChatCommandInput{
		AgentHandler: handler,
	})
	if err != nil {
		t.Fatal(err)
	}

	wantLines := []string{
		"Available profiles:\n",
		"  `fast`\n",
		"*> `main`\n",
		"\nCurrent profile: `main`\n",
		"LLM model: `main-model`\n",
		"Total messages: 3\n",
	}
	for _, line := range wantLines {
		if !strings.Contains(response.Stdout, line) {
			t.Fatalf("listing missing %q:\n%s", line, response.Stdout)
		}
	}
	if handler.reconfigured != 0 {
		t.Fatalf("listing must not reconfigure: %+v", handler)
	}
}

func TestProfileSwitchReportsThroughUnifiedOutput(t *testing.T) {
	t.Parallel()
	command := registeredCommand(t)
	handler := &fakeAgentHandle{cfg: testConfig(t)}

	response, err := command.Execute(context.Background(), &plugin.ChatCommandInput{
		AgentHandler: handler,
		Args:         "fast",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(response.Stdout, "Switched to profile: `fast`") {
		t.Fatalf("switch confirmation missing from stdout: %q", response.Stdout)
	}
	if handler.cfg.Profile() != "fast" {
		t.Fatalf("profile did not switch: %q", handler.cfg.Profile())
	}
	if handler.reconfigured != 1 {
		t.Fatalf("switch must reconfigure exactly once: %+v", handler)
	}
}

func TestProfileSwitchRejectsUnknownProfile(t *testing.T) {
	t.Parallel()
	command := registeredCommand(t)
	handler := &fakeAgentHandle{cfg: testConfig(t)}

	_, err := command.Execute(context.Background(), &plugin.ChatCommandInput{
		AgentHandler: handler,
		Args:         "nope",
	})
	if err == nil || !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("expected profile-not-found error, got %v", err)
	}
	if handler.reconfigured != 0 {
		t.Fatalf("a failed switch must not reconfigure: %+v", handler)
	}
}

func TestProfileListingWithoutProfiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("version: \"1.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	got := CurrentProfileMarkdown(cfg, nil)
	if !strings.Contains(got, "Current profile: ``") || strings.Contains(got, "*>") {
		t.Fatalf("unexpected empty listing: %q", got)
	}
}

func TestProfileListingKeepsMarkdownForbiddenCharsInert(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	handler := &fakeAgentHandle{cfg: cfg}

	// A profile carrying backticks or a leading `*` could fake the `*>`
	// marker of the active row; the listing renders names fenced in
	// backticks without re-interpreting their content.
	got := CurrentProfileMarkdown(handler.cfg, nil)
	if strings.Contains(got, "*> `fast`") {
		t.Fatalf("inactive row must not carry the active marker: %q", got)
	}
}
