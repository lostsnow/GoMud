package usercommands

import (
	"github.com/volte6/gomud/internal/events"
	"github.com/volte6/gomud/internal/rooms"
	"github.com/volte6/gomud/internal/templates"
	"github.com/volte6/gomud/internal/users"
)

func Cooldowns(rest string, user *users.UserRecord, room *rooms.Room, flags events.EventFlag) (bool, error) {

	cdTxt, _ := templates.Process("character/cooldowns", user.Character.GetAllCooldowns(), user.UserId)
	user.SendText(cdTxt)

	return true, nil
}
