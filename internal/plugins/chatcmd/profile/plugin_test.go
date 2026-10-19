package profileplugin

import (
	"context"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"

	"github.com/wmentor/go-magnetar/internal/config"
	"github.com/wmentor/go-magnetar/internal/plugin"
	"github.com/wmentor/go-magnetar/internal/plugin/testdata"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return loadConfig(t, "testdata/config.yml")
}

func loadConfig(t *testing.T, path string) *config.Config {
	t.Helper()
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
	handler := testdata.NewAgentHandle(t)
	handler.EXPECT().Config().Return(testConfig(t))
	handler.EXPECT().Messages().Return(make([]openai.ChatCompletionMessage, 3))
	// No Reconfigure expectation: a listing that reconfigures fails as an
	// unexpected mock call.

	response, err := command.Execute(context.Background(), &plugin.ChatCommandInput{
		AgentHandler: handler,
	})
	if err != nil {
		t.Fatal(err)
	}

	wantLines := []string{
		"Available profiles:\n\n",
		"- `fast`\n",
		"- **`main`**\n",
		"\nCurrent profile: `main`\n",
		"LLM model: `main-model`\n",
		"Total messages: 3\n",
	}
	for _, line := range wantLines {
		if !strings.Contains(response.Stdout, line) {
			t.Fatalf("listing missing %q:\n%s", line, response.Stdout)
		}
	}
}

func TestProfileSwitchReportsThroughUnifiedOutput(t *testing.T) {
	t.Parallel()
	command := registeredCommand(t)
	cfg := testConfig(t)
	handler := testdata.NewAgentHandle(t)
	handler.EXPECT().Config().Return(cfg)
	handler.EXPECT().Reconfigure().Once()

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
	if cfg.Profile() != "fast" {
		t.Fatalf("profile did not switch: %q", cfg.Profile())
	}
}

func TestProfileSwitchRejectsUnknownProfile(t *testing.T) {
	t.Parallel()
	command := registeredCommand(t)
	handler := testdata.NewAgentHandle(t)
	handler.EXPECT().Config().Return(testConfig(t))
	// Reconfigure is deliberately unstubbed: a failed switch must not
	// reconfigure, so any call fails as an unexpected mock call.

	_, err := command.Execute(context.Background(), &plugin.ChatCommandInput{
		AgentHandler: handler,
		Args:         "nope",
	})
	if err == nil || !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("expected profile-not-found error, got %v", err)
	}
}

func TestProfileListingWithoutProfiles(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, "testdata/config-no-profiles.yml")

	got := CurrentProfileMarkdown(cfg, nil)
	if !strings.Contains(got, "Current profile: ``") || strings.Contains(got, "- ") {
		t.Fatalf("unexpected empty listing: %q", got)
	}
}

func TestProfileListingKeepsMarkdownForbiddenCharsInert(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, "testdata/config-markdown.yml")

	// A profile name carrying markdown emphasis could fake the current
	// profile's bold marker; names are fenced in backticks, so only the
	// real current profile may come out bold.
	got := CurrentProfileMarkdown(cfg, nil)
	if strings.Count(got, "- **") != 1 {
		t.Fatalf("only the current profile may carry the bold marker: %q", got)
	}
	if !strings.Contains(got, "- **`plain`**\n") {
		t.Fatalf("current profile must be the emphasised row: %q", got)
	}
	if !strings.Contains(got, "- `**evil**`\n") {
		t.Fatalf("markdown-looking profile names must stay inert: %q", got)
	}
}
