package hooks

import (
	"github.com/GoMudEngine/GoMud/internal/events"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/rooms"
	"github.com/GoMudEngine/GoMud/internal/users"
)

//
// RoomChangeHandler waits for RoomChange events
// Also sends music changes out
//

func LocationMusicChange(e events.Event) events.ListenerReturn {

	evt, typeOk := e.(events.RoomChange)
	if !typeOk {
		mudlog.Error("Event", "Expected Type", "RoomChange", "Actual Type", e.Type())
		return events.Cancel
	}

	// If this isn't a user changing rooms, just pass it along.
	if evt.UserId == 0 {
		return events.Continue
	}

	// Get user... Make sure they still exist too.
	user := users.GetByUserId(evt.UserId)
	if user == nil {
		return events.Cancel
	}

	// Get the new room data... abort if doesn't exist.
	newRoom := rooms.LoadRoom(evt.ToRoomId)
	if newRoom == nil {
		return events.Cancel
	}

	// Get the old room data... abort if doesn't exist.
	oldRoom := rooms.LoadRoom(evt.FromRoomId)
	if oldRoom == nil {
		return events.Cancel
	}

	// If this zone has music, play it.
	// Room music takes priority.
	if newRoom.MusicFile != `` {
		user.PlayMusic(newRoom.MusicFile)
	} else {
		zoneInfo := rooms.GetZoneConfig(newRoom.Zone)
		if zoneInfo.MusicFile != `` {
			user.PlayMusic(zoneInfo.MusicFile)
		} else if oldRoom.MusicFile != `` {
			user.PlayMusic(`Off`)
		}
	}

	return events.Continue
}
