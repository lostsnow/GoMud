package usercommands

import (
	"fmt"
	"strings"

	"github.com/GoMudEngine/GoMud/internal/buffs"
	"github.com/GoMudEngine/GoMud/internal/events"
	"github.com/GoMudEngine/GoMud/internal/gametime"
	"github.com/GoMudEngine/GoMud/internal/items"
	"github.com/GoMudEngine/GoMud/internal/keywords"
	"github.com/GoMudEngine/GoMud/internal/mapper"
	"github.com/GoMudEngine/GoMud/internal/mobs"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/rooms"
	"github.com/GoMudEngine/GoMud/internal/templates"
	"github.com/GoMudEngine/GoMud/internal/users"
)

func Look(rest string, user *users.UserRecord, room *rooms.Room, flags events.EventFlag) (bool, error) {

	secretLook := flags.Has(events.CmdSecretly)

	visibility := room.GetVisibility()

	if visibility < 1 {
		if !user.Character.HasBuffFlag(buffs.NightVision) {
			user.SendText(`You can't see anything!`)
			return true, nil
		}
	}

	isSneaking := user.Character.HasBuffFlag(buffs.Hidden)

	// trim off some fluff
	if len(rest) > 2 {
		if rest[0:3] == `at ` {
			rest = rest[3:]
		}
	}
	if len(rest) > 3 {
		if rest[0:4] == `the ` {
			rest = rest[4:]
		}
	}

	lookAt := rest

	events.AddToQueue(events.Looking{
		UserId: user.UserId,
		RoomId: room.RoomId,
		Target: lookAt,
		Hidden: isSneaking,
	})

	// Handle an ordinary look with no target
	if len(lookAt) == 0 {

		if !secretLook && !isSneaking {
			room.SendText(
				fmt.Sprintf(`<ansi fg="username">%s</ansi> is looking around.`, user.Character.Name),
				user.UserId,
			)

			// Make it a "secret looks" now because we don't want another look message sent out by the lookRoom() func
			secretLook = true
		}
		lookRoom(user, room.RoomId, secretLook || isSneaking)
		return true, nil
	}

	//
	// look for any mobs, players, npcs
	//

	playerId, mobId := room.FindByName(lookAt)

	if playerId > 0 || mobId > 0 {

		statusTxt := ""
		invTxt := ""

		if playerId > 0 {

			u := *users.GetByUserId(playerId)

			if !isSneaking {
				u.SendText(
					fmt.Sprintf(`<ansi fg="username">%s</ansi> is looking at you.`, user.Character.Name),
				)

				room.SendText(
					fmt.Sprintf(`<ansi fg="username">%s</ansi> is looking at <ansi fg="username">%s</ansi>.`, user.Character.Name, u.Character.Name),
					u.UserId)
			}

			descTxt, _ := templates.Process("character/description", u.Character, user.UserId)
			user.SendText(descTxt)

			itemNames := []string{}
			for _, item := range u.Character.Items {
				itemNames = append(itemNames, item.DisplayName())
			}

			invData := map[string]any{
				`Equipment`: &u.Character.Equipment,
				`ItemNames`: itemNames,
			}

			inventoryTxt, _ := templates.Process("character/inventory-look", invData, user.UserId)
			user.SendText(inventoryTxt)

		} else if mobId > 0 {

			m := mobs.GetInstance(mobId)

			if !isSneaking {
				targetName := m.Character.GetMobName(0).String()
				room.SendText(
					fmt.Sprintf(`<ansi fg="username">%s</ansi> is looking at %s.`, user.Character.Name, targetName),
					user.UserId,
				)
			}

			descTxt, _ := templates.Process("character/description", &m.Character, user.UserId)
			user.SendText(descTxt)

			itemNames := []string{}
			for _, item := range m.Character.Items {
				itemNames = append(itemNames, item.DisplayName())
			}

			invData := map[string]any{
				`Equipment`: &m.Character.Equipment,
				`ItemNames`: itemNames,
			}

			inventoryTxt, _ := templates.Process("character/inventory-look", invData, user.UserId)
			user.SendText(inventoryTxt)
		}

		user.SendText(statusTxt)
		user.SendText(invTxt)

		return true, nil

	}

	containerName := room.FindContainerByName(lookAt)
	if containerName != `` {

		itemNames := []string{}
		itemNamesFormatted := []string{}

		container := room.Containers[containerName]

		if container.Lock.IsLocked() {
			user.SendText(``)
			user.SendText(fmt.Sprintf(`The <ansi fg="container">%s</ansi> is locked.`, containerName))
			user.SendText(``)
			return true, nil
		}

		if container.Gold > 0 {
			itemNames = append(itemNames, fmt.Sprintf(`%d gold`, container.Gold))
			itemNamesFormatted = append(itemNamesFormatted, fmt.Sprintf(`<ansi fg="gold">%d gold</ansi>`, container.Gold))
		}

		for _, item := range container.Items {
			if !item.IsValid() {
				room.RemoveItem(item, false)
				continue
			}

			itemNames = append(itemNames, item.Name())
			itemNamesFormatted = append(itemNamesFormatted, fmt.Sprintf(`<ansi fg="itemname">%s</ansi>`, item.DisplayName()))
		}

		if len(container.Recipes) > 0 {

			user.SendText(``)
			user.SendText(fmt.Sprintf(`You can <ansi fg="command">use</ansi> the <ansi fg="container">%s</ansi> if you put the following objects inside:`, containerName))

			for finalItemId, recipeList := range container.Recipes {

				neededItems := map[int]int{}

				for _, inputItemId := range recipeList {
					neededItems[inputItemId] += 1
				}

				user.SendText(``)

				finalItem := items.New(finalItemId)
				user.SendText(fmt.Sprintf(`    <ansi fg="230">To receive 1 <ansi fg="itemname">%s</ansi>:</ansi> `, finalItem.DisplayName()))

				for inputItemId, qtyNeeded := range neededItems {
					tmpItem := items.New(inputItemId)
					totalContained := container.Count(inputItemId)
					colorClass := "8" // None fulfilled
					if totalContained == qtyNeeded {
						colorClass = "10"
					} else if totalContained > 0 {
						colorClass = "3"
					}
					user.SendText(fmt.Sprintf(`        <ansi fg="%s">[%d/%d]</ansi> <ansi fg="itemname">%s</ansi>`, colorClass, totalContained, qtyNeeded, tmpItem.DisplayName()))
				}

			}

		}

		chestStuff := map[string]any{
			`ItemNames`:          itemNames,
			`ItemNamesFormatted`: itemNamesFormatted,
		}

		textOut, _ := templates.Process("descriptions/insidecontainer", chestStuff, user.UserId)

		user.SendText(``)
		user.SendText(textOut)
		user.SendText(``)

		return true, nil
	}

	//
	// Check room exits
	//
	exitName, lookRoomId := room.FindExitByName(lookAt)
	// If nothing found, consider directional aliases
	if exitName == `` {

		if alias := keywords.TryDirectionAlias(lookAt); alias != lookAt {
			exitName, lookRoomId = room.FindExitByName(alias)
			if exitName != `` {
				lookAt = alias
			}
		}
	}

	if exitName != `` {

		if visibility < 2 {

			if !user.Character.HasBuffFlag(buffs.NightVision) {
				biome := room.GetBiome()
				if !biome.IsLit() {
					user.SendText(`It's too dark to see anything in that direction.`)
					return true, nil
				}
			}

		}

		exitInfo, _ := room.GetExitInfo(exitName)
		if exitInfo.Lock.IsLocked() {
			user.SendText(fmt.Sprintf("The %s exit is locked.", exitName))
			return true, nil
		}

		user.SendText(fmt.Sprintf("You peer toward the %s.", exitName))
		if !isSneaking {
			room.SendText(fmt.Sprintf(`<ansi fg="username">%s</ansi> peers toward the %s.`, user.Character.Name, exitName), user.UserId)
		}

		lookRoom(user, lookRoomId, secretLook || isSneaking)

		return true, nil

	}

	//
	// Check for anything in their backpack they might want to look at
	//
	lookItem, foundItem := user.Character.FindInBackpack(lookAt)
	lookDestination := `in your backpack`
	if !foundItem {
		// Check for any equipment they are wearing they might want to look at
		lookItem, foundItem = user.Character.FindOnBody(lookAt)
		lookDestination = `you are wearing`
	}

	if foundItem {

		user.SendText(``)

		user.SendText(
			fmt.Sprintf(`You look at the <ansi fg="item">%s</ansi> %s:`, lookItem.DisplayName(), lookDestination),
		)

		user.SendText(``)

		if !isSneaking {
			room.SendText(
				fmt.Sprintf(`<ansi fg="username">%s</ansi> is admiring their <ansi fg="item">%s</ansi>.`, user.Character.Name, lookItem.DisplayName()),
				user.UserId,
			)
		}

		user.SendText(
			lookItem.GetLongDescription(),
		)

		user.SendText(``)

		return true, nil
	}

	//
	// Look for any nouns in the room info
	//
	foundNoun, foundDesc := room.FindNoun(lookAt)
	if len(foundNoun) > 0 {

		user.SendText(``)

		user.SendText(
			fmt.Sprintf(`You look at the <ansi fg="noun">%s</ansi>:`, foundNoun),
		)

		user.SendText(``)

		if !isSneaking {

			renderNouns := user.HasRolePermission(`room.nouns`)
			if user.Character.Pet.Exists() && user.Character.HasBuffFlag(buffs.SeeNouns) {
				renderNouns = true
			}

			if renderNouns && len(room.Nouns) > 0 {
				for noun, _ := range room.Nouns {
					foundDesc = strings.Replace(foundDesc, noun, `<ansi fg="noun">`+noun+`</ansi>`, 1)
				}
			}

			room.SendText(
				fmt.Sprintf(`<ansi fg="username">%s</ansi> is examining the <ansi fg="noun">%s</ansi>.`, user.Character.Name, foundNoun),
				user.UserId,
			)
		}

		user.SendText(foundDesc)

		user.SendText(``)

		return true, nil
	}

	//
	// Look for any pets in the room
	//
	petUserId := room.FindByPetName(rest)
	if petUserId == 0 && rest == `pet` && user.Character.Pet.Exists() {
		petUserId = user.UserId
	}
	if petUserId > 0 {
		if petUser := users.GetByUserId(petUserId); petUser != nil {

			user.SendText(fmt.Sprintf(`You look at %s`, petUser.Character.Pet.DisplayName()))

			room.SendText(fmt.Sprintf(`<ansi fg="username">%s</ansi> is looking at %s.`, user.Character.Name, petUser.Character.Pet.DisplayName()), user.UserId)

			textOut, _ := templates.Process("character/pet", petUser, user.UserId)
			user.SendText(textOut)

			return true, nil
		}
	}

	if len(room.Corpses) > 0 {

		mobCorpseLookup := map[string]int{}
		mobCorpses := []string{}

		playerCorpseLookup := map[string]int{}
		playerCorpses := []string{}
		for idx, c := range room.Corpses {
			if c.Prunable {
				continue
			}

			if c.MobId > 0 {
				name := c.Character.Name + ` corpse`
				if _, ok := mobCorpseLookup[name]; !ok {
					mobCorpseLookup[name] = idx
					mobCorpses = append(mobCorpses, name)
				}
			}

			if c.UserId > 0 {
				name := c.Character.Name + ` corpse`
				if _, ok := playerCorpseLookup[name]; !ok {
					playerCorpseLookup[name] = idx
					playerCorpses = append(playerCorpses, name)
				}
			}
		}

		if corpse, corpseFound := room.FindCorpse(rest); corpseFound {

			corpseColor := `mob-corpse`
			if corpse.UserId > 0 {
				corpseColor = `user-corpse`
			}

			user.SendText(fmt.Sprintf(`You look at the <ansi fg="%s">%s corpse</ansi>.`, corpseColor, corpse.Character.Name))
			room.SendText(fmt.Sprintf(`<ansi fg="username">%s</ansi> is looking at the <ansi fg="%s">%s corpse</ansi>.`, user.Character.Name, corpseColor, corpse.Character.Name), user.UserId)

			descTxt, _ := templates.Process("character/description-corpse", &corpse.Character, user.UserId)
			user.SendText(descTxt)

			return true, nil

		}

	}

	// Nothing found
	user.SendText("Look at what???")

	return true, nil

}

func lookRoom(user *users.UserRecord, roomId int, secretLook bool) {

	room := rooms.LoadRoom(roomId)

	if room == nil {
		return
	}

	// Make sure to prepare the room before anyone looks in if this is the first time someone has dealt with it in a while.
	if room.PlayerCt() < 1 {
		room.Prepare(true)
	}

	if !secretLook {
		// Find the exit back
		lookFromName := room.FindExitTo(user.Character.RoomId)
		if lookFromName == "" {
			room.SendText(
				fmt.Sprintf(`<ansi fg="username">%s</ansi> is looking into the room from somewhere...`, user.Character.Name),
				user.UserId,
			)
		} else {
			room.SendText(
				fmt.Sprintf(`<ansi fg="username">%s</ansi> is looking into the room from the <ansi fg="exit">%s</ansi> exit`, user.Character.Name, lookFromName),
				user.UserId,
			)
		}
	}

	var details rooms.RoomTemplateDetails

	tinyMapOn := user.GetConfigOption(`tinymap`)
	if tinyMapOn == nil {
		tinyMapOn = true
	}

	if user.ScreenReader {
		tinyMapOn = false
	}

	if tinyMapOn.(bool) && roomId > 0 {

		zMapper := mapper.GetMapper(room.RoomId)
		if zMapper == nil {

			mudlog.Error("Map", "error", "Could not find mapper for zone:"+room.Zone)

			details = rooms.GetDetails(room, user)

		} else {

			c := mapper.Config{
				ZoomLevel: 1,
				Width:     5,
				Height:    5,
				UserId:    user.UserId,
			}

			c.OverrideSymbol(roomId, '@', `You`)

			output := zMapper.GetLimitedMap(room.RoomId, c)
			tinyMap := []string{}
			tinyMap = append(tinyMap, `╔═════╗`)
			for _, mapLine := range output.Render {
				tinyMap = append(tinyMap, `║`+string(mapLine)+`║`)
			}
			tinyMap = append(tinyMap, `╚═════╝`)
			// This additional check is for ephemeral room copies,
			// which can slightly mess with the map render of the @
			if tinyMap[3][3] != '@' {
				youLine := []rune(tinyMap[3])
				youLine[3] = '@'
				tinyMap[3] = string(youLine)
			}

			legend := output.GetLegend(keywords.GetAllLegendAliases(room.Zone))

			for i := 1; i <= c.Height; i++ {
				for sym, txtLegend := range legend {
					txtLc := strings.ToLower(txtLegend)
					tinyMap[i] = strings.Replace(tinyMap[i], string(sym), fmt.Sprintf(`<ansi fg="map-room"><ansi fg="map-%s" bg="mapbg-%s">%c</ansi></ansi>`, txtLc, txtLc, sym), -1)
				}
			}

			details = rooms.GetDetails(room, user, tinyMap)

		}

	} else {
		details = rooms.GetDetails(room, user)
	}

	textOut, _ := templates.Process("descriptions/room-title", details, user.UserId)
	user.SendText(textOut)

	textOut, _ = templates.Process("descriptions/room", details, user.UserId)
	user.SendText(textOut)

	signCt := 0
	privateSigns := room.GetPrivateSigns()
	for _, sign := range privateSigns {
		if sign.VisibleUserId == user.UserId {
			signCt++
			textOut, _ = templates.Process("descriptions/rune", sign, user.UserId)
			user.SendText(textOut)
		}
	}

	publicSigns := room.GetPublicSigns()
	for _, sign := range publicSigns {
		signCt++
		textOut, _ = templates.Process("descriptions/sign", sign, user.UserId)
		user.SendText(textOut)
	}

	if signCt > 0 {
		user.SendText("")
	}

	textOut, _ = templates.Process("descriptions/who", details, user.UserId)
	if len(textOut) > 0 {
		user.SendText(textOut)
	}

	groundStuff := []string{}
	for containerName, container := range room.Containers {

		chestName := fmt.Sprintf(`<ansi fg="container">%s</ansi>`, containerName)

		if container.HasLock() {
			if container.Lock.IsLocked() {
				chestName += ` <ansi fg="white">(locked)</ansi>`
			} else {
				chestName += ` <ansi fg="white">(unlocked)</ansi>`
			}
		}

		groundStuff = append(groundStuff, chestName)

	}

	if room.Gold > 0 {
		groundStuff = append(groundStuff, fmt.Sprintf(`<ansi fg="gold">%d gold</ansi>`, room.Gold))
	}

	for _, item := range room.Items {
		if !item.IsValid() {
			room.RemoveItem(item, false)
			continue
		}
		groundStuff = append(groundStuff, item.DisplayName())
	}

	// Find stashed items
	for _, item := range room.Stash {
		if !item.IsValid() {
			room.RemoveItem(item, true)
		}
		if item.StashedBy != user.UserId {
			continue
		}
		name := item.DisplayName() + ` <ansi fg="item-stashed">(stashed)</ansi>`
		groundStuff = append(groundStuff, name)
	}

	groundStuff = append(groundStuff, details.VisibleCorpses...)

	groundDetails := map[string]any{
		`GroundStuff`: groundStuff,
		`IsDark`:      room.GetBiome().IsDark(),
		`IsNight`:     gametime.IsNight(),
	}
	textOut, _ = templates.Process("descriptions/ontheground", groundDetails, user.UserId)
	if len(textOut) > 0 {
		user.SendText(textOut)
	}

	textOut, _ = templates.Process("descriptions/exits", details, user.UserId)
	user.SendText(textOut)

}
