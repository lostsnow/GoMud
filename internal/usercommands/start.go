package usercommands

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GoMudEngine/GoMud/internal/characters"
	"github.com/GoMudEngine/GoMud/internal/configs"
	"github.com/GoMudEngine/GoMud/internal/events"
	"github.com/GoMudEngine/GoMud/internal/mobs"
	"github.com/GoMudEngine/GoMud/internal/races"
	"github.com/GoMudEngine/GoMud/internal/rooms"
	"github.com/GoMudEngine/GoMud/internal/scripting"
	"github.com/GoMudEngine/GoMud/internal/templates"
	"github.com/GoMudEngine/GoMud/internal/term"
	"github.com/GoMudEngine/GoMud/internal/users"
)

func Start(rest string, user *users.UserRecord, room *rooms.Room, flags events.EventFlag) (bool, error) {

	if user.Character.RoomId != -1 {
		return false, errors.New(`only allowed in the void`)
	}

	// Get if already exists, otherwise create new
	cmdPrompt, isNew := user.StartPrompt(`start`, rest)

	if isNew {
		user.SendText(``)
		user.SendText(fmt.Sprintf(`You'll need to answer some questions.%s`, term.CRLFStr))
	}

	if user.Character.RaceId == 0 {

		raceOptions := []templates.NameDescription{}

		for _, r := range races.GetRaces() {
			if r.Selectable {
				raceOptions = append(raceOptions, templates.NameDescription{
					Id:          r.RaceId,
					Name:        r.Name,
					Description: r.Description,
				})
			}
		}
		sort.SliceStable(raceOptions, func(i, j int) bool {
			return raceOptions[i].Name < raceOptions[j].Name
		})

		question := cmdPrompt.Ask(`Which race will you be?`, []string{})
		if !question.Done {

			tplTxt, _ := templates.Process("tables/numbered-list", raceOptions, user.UserId)
			user.SendText(tplTxt)
			user.SendText(`  Want to know more details? Type <ansi fg="command">help {racename}</ansi> or <ansi fg="command">help {number}</ansi>`)
			user.SendText(``)
			return true, nil
		}

		respLower := strings.ToLower(question.Response)
		if len(respLower) >= 5 && respLower[0:5] == `help ` {
			helpCmd := `race`
			helpRest := respLower[5:]

			if restNum, err := strconv.Atoi(helpRest); err == nil {
				if restNum > 0 && restNum <= len(raceOptions) {
					helpRest = raceOptions[restNum-1].Name
				} else {
					helpCmd = `races`
					helpRest = ``
				}
			}

			question.RejectResponse()
			return Help(helpCmd+` `+helpRest, user, room, flags)
		}

		raceNameSelection := question.Response
		if restNum, err := strconv.Atoi(raceNameSelection); err == nil {
			if restNum > 0 && restNum <= len(raceOptions) {
				raceNameSelection = raceOptions[restNum-1].Name
			}
		}

		matchFound := false
		for _, r := range races.GetRaces() {
			if strings.EqualFold(r.Name, raceNameSelection) {

				if r.Selectable {
					matchFound = true
					user.Character.RaceId = r.Id()
					user.Character.Alignment = r.DefaultAlignment
					user.Character.Validate()

					user.SendText(``)
					user.SendText(fmt.Sprintf(`  <ansi fg="magenta">*** Your ghostly form materializes into that of a %s ***</ansi>%s`, r.Name, term.CRLFStr))
					break
				}

			}
		}

		if !matchFound {
			question.RejectResponse()

			tplTxt, _ := templates.Process("tables/numbered-list", raceOptions, user.UserId)
			user.SendText(tplTxt)
			user.SendText(`  Want to know more details? Type <ansi fg="command">help {racename}</ansi> or <ansi fg="command">help {number}</ansi>`)
			user.SendText(``)

			return true, nil
		}
	}

	if strings.EqualFold(user.Character.Name, user.Username) || user.Character.Name == user.TempName() || len(user.Character.Name) == 0 || strings.ToLower(user.Character.Name) == `nameless` {

		question := cmdPrompt.Ask(`What will your character be known as (name)?`, []string{})
		if !question.Done {
			return true, nil
		}

		if strings.EqualFold(question.Response, user.Username) {
			user.SendText(`Your username cannot match your character name!`)
			question.RejectResponse()
			return true, nil
		}

		for _, c := range characters.LoadAlts(user.UserId) {
			if strings.EqualFold(question.Response, c.Name) {
				user.SendText(`Your already have a character named that!`)
				question.RejectResponse()
				return true, nil
			}
		}

		if err := users.ValidateName(question.Response); err != nil {
			user.SendText(`that name is not allowed: ` + err.Error())
			question.RejectResponse()
			return true, nil
		}

		if bannedPattern, ok := configs.GetConfig().IsBannedName(question.Response); ok {
			user.SendText(`that username matched the prohibited name pattern: "` + bannedPattern + `"`)
			question.RejectResponse()
			return true, nil
		}

		if foundUserId, _ := users.CharacterNameSearch(question.Response); foundUserId > 0 {
			user.SendText(`that character name is already in use.`)
			question.RejectResponse()
			return true, nil
		}

		for _, name := range mobs.GetAllMobNames() {
			if strings.EqualFold(name, question.Response) {
				user.SendText("that name is in use")
				question.RejectResponse()
				return true, nil
			}
		}

		usernameSelected := question.Response

		question = cmdPrompt.Ask(`Choose the name <ansi fg="username">`+usernameSelected+`</ansi>?`, []string{`yes`, `no`}, `no`)
		if !question.Done {
			return true, nil
		}

		if question.Response == `no` {
			user.ClearPrompt()
			return Start(rest, user, room, flags)
		}

		if err := user.SetCharacterName(usernameSelected); err != nil {
			user.SendText(err.Error())
			question.RejectResponse()
			return true, nil
		}

		user.SendText(fmt.Sprintf(`You will be known as <ansi fg="yellow-bold">%s</ansi>!%s`, user.Character.Name, term.CRLFStr))
	}

	user.Character.ExtraLives = int(configs.GetGamePlayConfig().LivesStart)

	user.EventLog.Add(`char`, fmt.Sprintf(`Created a new character: <ansi fg="username">%s</ansi>`, user.Character.Name))

	events.AddToQueue(events.CharacterCreated{UserId: user.UserId, CharacterName: user.Character.Name})

	duration := time.Now().Sub(user.Joined)
	if duration.Hours() > 1 {

		question := cmdPrompt.Ask(`Skip tutorial?`, []string{`yes`, `no`}, `yes`)
		if !question.Done {
			return true, nil
		}

		if question.Response != `no` {

			user.ClearPrompt()

			user.SendText(fmt.Sprintf(`<ansi fg="magenta">Suddenly, a vortex appears before you, drawing you in before you have any chance to react!</ansi>%s`, term.CRLFStr))

			if destRoom := rooms.LoadRoom(rooms.StartRoomIdAlias); destRoom != nil {

				rooms.MoveToRoom(user.UserId, destRoom.RoomId)

				// Tell the new room they have arrived

				destRoom.SendText(
					fmt.Sprintf(configs.GetTextFormatsConfig().EnterRoomMessageWrapper.String(),
						fmt.Sprintf(`<ansi fg="username">%s</ansi> enters from <ansi fg="exit">somewhere</ansi>.`, user.Character.Name),
					),
					user.UserId,
				)

				if doLook, err := scripting.TryRoomScriptEvent(`onEnter`, user.UserId, destRoom.RoomId); err != nil || doLook {
					Look(``, user, destRoom, events.CmdSecretly) // Do a secret look.
				}

				room.PlaySound(`room-exit`, `movement`, user.UserId)
				destRoom.PlaySound(`room-enter`, `movement`, user.UserId)

				return true, nil
			}

		}

	}

	user.ClearPrompt()

	tutorialRoomIds := []int{}
	startRoom := 0
	for i, roomIdStr := range configs.GetSpecialRoomsConfig().TutorialRooms {
		roomId, _ := strconv.ParseInt(roomIdStr, 10, 64)
		tutorialRoomIds = append(tutorialRoomIds, int(roomId))

		if i == 0 {
			startRoom = int(roomId)
		}
	}

	createdRoomIds, err := rooms.CreateEphemeralRoomIds(tutorialRoomIds...)
	if err != nil {
		user.SendText(`The Tutorial zone is fully occupied right now. Please try again in a few minutes`)
		return true, nil
	}

	ephemeralStartRoomId := createdRoomIds[startRoom]

	user.SendText(fmt.Sprintf(`<ansi fg="magenta">Suddenly, a vortex appears before you, drawing you in before you have any chance to react!</ansi>%s`, term.CRLFStr))

	rooms.MoveToRoom(user.UserId, ephemeralStartRoomId)

	if doLook, err := scripting.TryRoomScriptEvent(`onEnter`, user.UserId, ephemeralStartRoomId); err != nil || doLook {
		if lookRoom := rooms.LoadRoom(ephemeralStartRoomId); lookRoom != nil {
			Look(``, user, lookRoom, events.CmdSecretly)
		}
	}

	return true, nil
}
