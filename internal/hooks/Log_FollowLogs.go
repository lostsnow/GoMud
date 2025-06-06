package hooks

import (
	"fmt"

	"github.com/GoMudEngine/GoMud/internal/connections"
	"github.com/GoMudEngine/GoMud/internal/events"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
)

// Tee's log output to admins following
var (
	logFollowConnectionIds = map[connections.ConnectionId]int{}

	sendLists = [4][]connections.ConnectionId{}

	pruneLogCounter = 0

	logLevels = map[string]int{
		`DEBUG`: 0,
		`INFO`:  1,
		`WARN`:  2,
		`ERROR`: 3,
	}
)

func FollowLogs(e events.Event) events.ListenerReturn {

	evt, typeOk := e.(events.Log)
	if !typeOk {
		mudlog.Error("Event", "Expected Type", "Log", "Actual Type", e.Type())
		return events.Cancel
	}

	if evt.FollowAdd > 0 {

		// Easiest way, just remove them first. This is a low frequency operation
		removeFromSendLists(evt.FollowAdd)

		for i := logLevels[evt.Level]; i < 4; i++ {
			sendLists[i] = append(sendLists[i], evt.FollowAdd)
		}

		return events.Continue
	}

	if evt.FollowRemove > 0 {

		removeFromSendLists(evt.FollowRemove)

		return events.Continue
	}

	if len(sendLists[logLevels[evt.Level]]) > 0 {
		// Leaving timestamp out for now
		connections.SendTo([]byte(fmt.Sprintln(evt.Data[1:]...)), sendLists[logLevels[evt.Level]]...)
	}

	pruneLogCounter++
	if pruneLogCounter%1000 == 0 {
		removeFromSendLists(0) // Force a prune.
	}

	return events.Continue
}

func removeFromSendLists(connId connections.ConnectionId) {

	for i := 0; i < 4; i++ {

		for idx := len(sendLists[i]) - 1; idx >= 0; idx-- {

			testConnId := sendLists[i][idx]

			if testConnId == connId {
				sendLists[i] = append(sendLists[i][:idx], sendLists[i][idx+1:]...)
				continue
			}

			// Prune if it's old.
			if connections.Get(testConnId) == nil {
				sendLists[i] = append(sendLists[i][:idx], sendLists[i][idx+1:]...)
			}

		}
	}

}
