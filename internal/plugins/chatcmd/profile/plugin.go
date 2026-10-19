package profileplugin

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sashabaranov/go-openai"

	"github.com/wmentor/go-magnetar/internal/config"
	"github.com/wmentor/go-magnetar/internal/plugin"
)

func init() {
	plugin.Register("chatcmd.profile", &Plugin{})
}

// Plugin registers the /profile chat command.
type Plugin struct{}

func (p *Plugin) Init(_ *plugin.State, hub plugin.Hub) error {
	hub.RegisterChatCommand(plugin.ChatCommand{
		Name: "profile",
		Help: "Show current profile or switch to a different profile.",
		Execute: func(ctx context.Context, params *plugin.ChatCommandInput) (*plugin.ChatCommandResponse, error) {
			args := params.Args
			if args == "" {
				return &plugin.ChatCommandResponse{
					Stdout: CurrentProfileMarkdown(params.AgentHandler.Config(), params.AgentHandler.Messages()),
				}, nil
			}

			if err := params.AgentHandler.Config().SetProfile(args); err != nil {
				return nil, errors.Wrap(err, "set profile")
			}

			params.AgentHandler.Reconfigure()

			return &plugin.ChatCommandResponse{
				Stdout: fmt.Sprintf("Switched to profile: `%s`\n", args),
			}, nil
		},
	})
	return nil
}

// CurrentProfileMarkdown renders the profile listing as markdown for the
// unified chat-command output (ChatCommandResponse.Stdout), so /profile
// prints through the same channel as /help and every other chat command.
func CurrentProfileMarkdown(cfg *config.Config, messages []openai.ChatCompletionMessage) string {
	profiles := cfg.Profiles()
	current := cfg.String("profile")
	model := cfg.ProfileParamString("llm.model")

	var sb strings.Builder
	fmt.Fprintf(&sb, "Available profiles:\n\n")
	for _, p := range profiles {
		if p == current {
			fmt.Fprintf(&sb, "- **`%s`**\n", p)
			continue
		}
		fmt.Fprintf(&sb, "- `%s`\n", p)
	}

	fmt.Fprintf(&sb, "\nCurrent profile: `%s`\n", current)
	fmt.Fprintf(&sb, "LLM model: `%s`\n", model)
	fmt.Fprintf(&sb, "Total messages: %d\n", len(messages))

	return sb.String()
}
