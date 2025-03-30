package hooks

import (
	"strings"

	"github.com/volte6/gomud/internal/events"
	"github.com/volte6/gomud/internal/mudlog"
	"github.com/volte6/gomud/internal/templates"
)

func ClearSettingCaches(e events.Event) events.ListenerReturn {

	evt, typeOk := e.(events.UserSettingChanged)
	if !typeOk {
		mudlog.Error("Event", "Expected Type", "UserSettingChanged", "Actual Type", e.Type())
		return events.Cancel
	}

	// If this isn't a user changing rooms, just pass it along.
	if evt.UserId == 0 {
		return events.Continue
	}

	if strings.ToLower(evt.Name) == `screenreader` {
		templates.ClearTemplateConfigCache(evt.UserId)
	}

	return events.Continue
}
