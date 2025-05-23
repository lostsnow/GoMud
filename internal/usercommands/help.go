package usercommands

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/GoMudEngine/GoMud/internal/events"
	"github.com/GoMudEngine/GoMud/internal/keywords"
	"github.com/GoMudEngine/GoMud/internal/races"
	"github.com/GoMudEngine/GoMud/internal/rooms"
	"github.com/GoMudEngine/GoMud/internal/spells"
	"github.com/GoMudEngine/GoMud/internal/templates"
	"github.com/GoMudEngine/GoMud/internal/users"
	"github.com/GoMudEngine/GoMud/internal/util"
)

func Help(rest string, user *users.UserRecord, room *rooms.Room, flags events.EventFlag) (bool, error) {

	var helpTxt string
	var err error = nil

	args := util.SplitButRespectQuotes(rest)

	if len(args) == 0 {

		type helpCommand struct {
			Command string
			Type    string
			Missing bool
		}

		type commandLists struct {
			Commands map[string][]helpCommand
			Skills   map[string][]helpCommand
			Admin    map[string][]helpCommand
		}

		helpCommandList := commandLists{
			Commands: make(map[string][]helpCommand),
			Skills:   make(map[string][]helpCommand),
			Admin:    make(map[string][]helpCommand),
		}

		for _, command := range keywords.GetAllHelpTopicInfo() {

			category := command.Category
			if category == `all` {
				category = ``
			}

			templateFile := `help/` + keywords.TryHelpAlias(command.Command)

			if command.AdminOnly {
				if user.HasRolePermission(command.Command, true) {
					helpCommandList.Admin[category] = append(
						helpCommandList.Admin[category],
						helpCommand{Command: command.Command, Type: "command-admin", Missing: false},
					)
				}
				continue
			}

			hlpCmd := helpCommand{Command: command.Command, Type: command.Type, Missing: !templates.Exists(templateFile)}

			if command.Type == `skill` {
				helpCommandList.Skills[category] = append(helpCommandList.Skills[category], hlpCmd)
				continue
			}

			helpCommandList.Commands[category] = append(helpCommandList.Commands[category], hlpCmd)

		}

		helpTxt, err = templates.Process("help/help", helpCommandList, user.UserId)
		if err != nil {
			helpTxt = err.Error()
		}
	} else {

		helpTxt, err = GetHelpContents(rest)
		if err != nil {
			user.SendText(fmt.Sprintf(`No help found for "%s"`, rest))
			return true, err
		}

	}

	user.SendText(helpTxt)

	return true, nil
}

func getRaceOptions(raceRequest string) []races.Race {

	allRaces := races.GetRaces()
	sort.Slice(allRaces, func(i, j int) bool {
		return allRaces[i].RaceId < allRaces[j].RaceId
	})

	raceNames := strings.Split(raceRequest, ` `)

	getAllRaces := false
	if raceRequest == `all` {
		getAllRaces = true
	}

	raceOptions := []races.Race{}
	for _, race := range allRaces {

		if len(raceRequest) == 0 {
			if !race.Selectable && !getAllRaces {
				continue
			}
		} else if len(raceNames) > 0 {
			lowerName := strings.ToLower(race.Name)
			found := false
			for _, rName := range raceNames {
				if strings.Contains(lowerName, strings.ToLower(rName)) {
					found = true
					break
				}
			}
			if !getAllRaces && !found {
				continue
			}
		}
		raceOptions = append(raceOptions, race)
	}

	return raceOptions
}

func GetHelpContents(input string) (string, error) {

	args := util.SplitButRespectQuotes(input)

	helpName := args[0]
	helpRest := ``

	args = args[1:]
	if len(args) > 0 {
		helpRest = strings.Join(args, ` `)
	}

	// replace any non alpha/numeric characters in "rest"
	if fullSearchAlias := keywords.TryHelpAlias(input); fullSearchAlias != input {
		helpName = fullSearchAlias
	} else {
		helpName = regexp.MustCompile(`[^a-zA-Z0-9\\-]+`).ReplaceAllString(helpName, ``)
		helpName = keywords.TryHelpAlias(helpName)
	}

	var helpVars any = nil

	if helpName == `emote` {
		helpVars = emoteAliases
	}

	if helpName == `races` {
		helpVars = getRaceOptions(helpRest)
	}

	if helpName == `spell` {
		sData := spells.GetSpell(helpRest)
		if sData == nil {
			sData = spells.FindSpellByName(helpRest)
		}

		if sData == nil {
			helpName = `spells`
		} else {
			helpVars = *sData
		}
	}

	return templates.Process("help/"+helpName, helpVars, 0)
}
