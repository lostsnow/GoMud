package rooms

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/GoMudEngine/GoMud/internal/configs"
	"github.com/GoMudEngine/GoMud/internal/events"
	"github.com/GoMudEngine/GoMud/internal/fileloader"
	"github.com/GoMudEngine/GoMud/internal/items"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/util"
	"gopkg.in/yaml.v2"
)

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// RULES FOR SAVING AND LOADING ROOMS
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//
// The following are rules for reading and writing room files.
// This set of rules must be followed or behaviors will break.
// DO NOT CHANGE CODE FOR THE PROCESS unless you understand the implications.
//
// NOTE: in-memory cache contains a RoomId => Room struct map. This Room struct is TEMPLATE data updated on load with INSTANCE data.
// Once loaded into memory, the in-memory cache is the most recent source-of-truth for INSTANCE data.
//
// TEMPLATE data is stored in the /rooms/ folder
// INSTANCE data is stored in the /rooms.instances/ folder (may only be created on running the MUD)
//
// INSTANCE data can be cleared out by deleting the /rooms.instances/ folder, or running the `make clean-instances` to do it.
//
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// A. LOADING ROOMS BLINDLY                                                                                              AKA LoadRoom()
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//
// NOTE: This is the only way outside code should be loading rooms. The other functions are exported for very specialized purposes.
//
// 1. Look for in-memory Room cache record for RoomId. If found, return it.
// 2. Do [B. LOADING ROOMS FROM FILES]
// 3. Save result to in-memory Room cache.
//
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// B. LOADING ROOMS FROM FILES                                                                                  AKA  LoadRoomInstance()
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//
//  1. Load the TEMPLATE data into a Room struct.
//  2. Load the INSTANCE data into the same struct - the result is only the field present in the INSTANCE data will overwrite the
//     TEMPLATE data.
//
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// C. SAVING ROOM TEMPLATES                                                                                      AKA SaveRoomTemplate()
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//
//  1. Anytime a change is made to a TEMPLATE, first load the TEMPLATE ONLY, make any changes, then save the TEMPLATE ONLY.
//  2. There may be data from the OLD in-memory Room struct that needs to be copied over to the new one. (items, users, mobs, etc).
//     If so, do it.
//  3. Update the in-memory Room struct for the RoomId to use the NEW TEMPLATE version.
//  4. Rebuild maps for the Room's Zone.RoomId and then the room.RoomId, in that order. That ensures it's added to the zone map
//     IF it should be, and if not, will generate its own map.
//
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// D. SAVING ROOMS INSTANCES                                                                                     AKA SaveRoomInstance()
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//
//  1. Load the TEMPLATE data into a variable
//  2. Use reflection to iterate through the structure.
//  3. If field isn't readable (unexported/lowercase) or has the struct tag `instance:"skip"`, skip writing it.
//  4. If the field value matches the TEMPLATE version of the field, skip writing it.
//  5. If the final result of the data to write is empty ( "{}\n" ), delete and do not write the new save file.
//
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// See: A. LOADING ROOMS BLINDLY
func LoadRoom(roomId int) *Room {

	// Room 0 aliases to start room
	if roomId == StartRoomIdAlias {
		if roomId = int(configs.GetSpecialRoomsConfig().StartRoom); roomId == 0 {
			roomId = 1
		}
	}

	if room := getRoomFromMemory(roomId); room != nil {
		return room
	}

	if room := LoadRoomInstance(roomId); room != nil {
		addRoomToMemory(room)
		return room
	}

	return nil
}

// See: B. LOADING ROOMS FROM FILES
func LoadRoomInstance(roomId int) *Room {

	room := LoadRoomTemplate(roomId)
	if room == nil {
		return nil
	}

	filename := roomManager.GetFilePath(roomId)

	if len(filename) == 0 {
		return nil
	}

	// Look for specially saved instance data
	filepath := util.FilePath(configs.GetFilePathsConfig().DataFiles.String(), `/rooms.instances/`, filename)

	if bytes, err := os.ReadFile(filepath); err == nil {
		// Unmarshal onto the default template data, overwriting any set fields in the instance save file
		yaml.Unmarshal(bytes, room)
	}

	return room
}

// Only loads the template data, ignores instance data.
func LoadRoomTemplate(roomId int) *Room {

	if roomId >= ephemeralRoomIdMinimum {
		return nil
	}

	filename := roomManager.GetFilePath(roomId)

	if len(filename) == 0 {
		return nil
	}

	filepath := util.FilePath(configs.GetFilePathsConfig().DataFiles.String(), `/rooms/`, filename)

	retRoom, _ := loadRoomFromFile(filepath)

	return retRoom
}

// See C. UPDATING EXISTING ROOM TEMPLATES
func SaveRoomTemplate(roomTpl Room) error {

	if roomTpl.IsEphemeral() {
		return errors.New(`ephemeral rooms are not saved`)
	}

	// Do not accept a RoomId of zero.
	if roomTpl.RoomId == 0 {
		roomTpl.RoomId = GetNextRoomId()
		SetNextRoomId(roomTpl.RoomId + 1)
	}

	//
	// It is assumed that `roomTpl` contains empty room data
	// That means no players/mobs/items/gold/etc that aren't intended to be included in the default/empty room template
	//
	data, err := yaml.Marshal(&roomTpl)
	if err != nil {
		return err
	}

	zoneFolder := ZoneToFolder(roomTpl.Zone)

	// First write the empty version to its template file
	roomFilePath := util.FilePath(configs.GetFilePathsConfig().DataFiles.String(), `/rooms/`, fmt.Sprintf("%s%d.yaml", zoneFolder, roomTpl.RoomId))
	if err = os.WriteFile(roomFilePath, data, 0777); err != nil {
		return err
	}

	// Get zone root
	cfg := GetZoneConfig(roomTpl.Zone)

	// Queue rebuild the zone map
	events.AddToQueue(events.RebuildMap{MapRootRoomId: cfg.RoomId})
	// Also queue rebuild the map based on the room, if it's not already present in a map (AKA, SkipIfExists)
	events.AddToQueue(events.RebuildMap{MapRootRoomId: roomTpl.RoomId, SkipIfExists: true})

	//
	// Now we can make changes to the roomTpl as though it is the room (copy over stuff etc)
	//
	roomBeingReplaced := roomManager.rooms[roomTpl.RoomId]

	// Copy container contents (if new vs. old room container names match)
	for containerName, container := range roomBeingReplaced.Containers {

		if newContainer, ok := roomTpl.Containers[containerName]; ok {

			if newContainer.Gold == 0 {
				newContainer.Gold = container.Gold
			}

			if len(newContainer.Items) == 0 && len(container.Items) > 0 {
				newContainer.Items = make([]items.Item, len(container.Items))
				copy(newContainer.Items, container.Items)
			}

			roomTpl.Containers[containerName] = newContainer
		}
	}

	// Copy items and stashed items
	for _, itm := range roomBeingReplaced.GetAllFloorItems(true) {
		if itm.StashedBy > 0 {
			roomTpl.AddItem(itm, true)
		} else {
			roomTpl.AddItem(itm, false)
		}
	}

	// Copy gold on floor
	roomTpl.Gold = roomBeingReplaced.Gold

	// Copy signs
	roomTpl.Signs = make([]Sign, len(roomBeingReplaced.Signs))
	copy(roomTpl.Signs, roomBeingReplaced.Signs)

	// Copy mobs in room
	roomTpl.mobs = make([]int, len(roomBeingReplaced.mobs))
	copy(roomTpl.mobs, roomBeingReplaced.mobs)

	// Copy players in room
	roomTpl.players = make([]int, len(roomBeingReplaced.players))
	copy(roomTpl.players, roomBeingReplaced.players)

	// Add to memory with the force flag true
	// This will clear out the old data and force write the new data.
	addRoomToMemory(&roomTpl, true)

	// Save whatever is in this room as the instance data
	SaveRoomInstance(roomTpl)

	return nil
}

// See: D. SAVING ROOMS INSTANCES
func SaveRoomInstance(r Room) error {

	if r.IsEphemeral() {
		return errors.New(`ephemeral rooms are not saved`)
	}

	rTpl := LoadRoomTemplate(r.RoomId) // This is also a Room{}
	if rTpl == nil {
		return fmt.Errorf(`could not load template for room %d`, r.RoomId)
	}

	rVal := reflect.ValueOf(r)
	tplVal := reflect.ValueOf(*rTpl)
	t := reflect.TypeOf(r)

	instanceSaveData := make(map[string]interface{})

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}

		yamlTag := field.Tag.Get("yaml")
		if yamlTag == `-` {
			continue
		}

		if field.Tag.Get("instance") == "skip" {
			continue
		}

		rVal2 := rVal.Field(i)
		tplVal2 := tplVal.Field(i)

		if reflect.DeepEqual(rVal2.Interface(), tplVal2.Interface()) {
			continue
		}

		tagParts := strings.Split(yamlTag, ",")
		fieldName := tagParts[0]
		if fieldName == `` || fieldName == `omitempty` || fieldName == `flow` {
			fieldName = field.Name
		}

		instanceSaveData[fieldName] = rVal2.Interface()

	}

	zone := ZoneToFolder(r.Zone)
	folderPath := util.FilePath(configs.GetFilePathsConfig().DataFiles.String(), `/rooms.instances/`, zone)
	instanceFilePath := fmt.Sprintf("%s%d.yaml", folderPath, r.RoomId)

	if len(instanceSaveData) == 0 {
		os.Remove(instanceFilePath)
		return nil
	}

	data, err := yaml.Marshal(instanceSaveData)
	if err != nil {
		return err
	}

	if err = os.WriteFile(instanceFilePath, data, 0777); err != nil {
		return err
	}

	return nil
}

func loadRoomFromFile(roomFilePath string) (*Room, error) {

	roomFilePath = util.FilePath(roomFilePath)

	roomPtr, err := fileloader.LoadFlatFile[*Room](roomFilePath)
	if err != nil {
		mudlog.Error("loadRoomFromFile()", "error", err.Error())
	}

	return roomPtr, err
}

func SaveAllRooms() error {

	start := time.Now()
	saveCt := 0
	errCt := 0
	for _, r := range roomManager.rooms {

		if r.IsEphemeral() {
			continue
		}

		if SaveRoomInstance(*r) != nil {
			errCt++
			continue
		}
		saveCt++

	}

	mudlog.Info("SaveAllRooms()", "savedCount", saveCt, "expectedCt", len(roomManager.rooms), "errorCount", errCt, "Time Taken", time.Since(start))

	return nil
}

// Overwrite file and memory for zoneconfig
func SaveZoneConfig(zoneConfig *ZoneConfig) error {

	zoneFolder := util.FilePath(configs.GetFilePathsConfig().DataFiles.String(), "/", "rooms")
	if err := fileloader.SaveFlatFile(zoneFolder, zoneConfig); err != nil {
		return err
	}

	roomManager.zones[zoneConfig.Name] = zoneConfig

	return nil
}

// Goes through all of the rooms and caches key information
func loadAllRoomZones() error {
	start := time.Now()

	nextRoomId := GetNextRoomId()
	defer func() {
		if nextRoomId != GetNextRoomId() {
			SetNextRoomId(nextRoomId)
		}
	}()

	loadedZones, err := fileloader.LoadAllFlatFiles[string, *ZoneConfig](configs.GetFilePathsConfig().DataFiles.String()+`/rooms`, "zone-config.yaml")
	if err != nil {
		return err
	}

	for zoneName, zoneConfig := range loadedZones {
		roomManager.zones[zoneName] = zoneConfig

		folderPath := util.FilePath(configs.GetFilePathsConfig().DataFiles.String(), `/rooms.instances/`, ZoneNameSanitize(zoneConfig.Name))
		if _, err := os.Stat(folderPath); os.IsNotExist(err) {
			os.MkdirAll(folderPath, 0755)
		}
	}

	loadedRooms, err := fileloader.LoadAllFlatFiles[int, *Room](configs.GetFilePathsConfig().DataFiles.String()+`/rooms`, "[0-9]*.yaml")
	if err != nil {
		return err
	}

	roomsWithoutEntrances := map[int]string{}

	for _, loadedRoom := range loadedRooms {

		// configs.GetConfig().DeathRecoveryRoom is the death/shadow realm and gets a pass
		if loadedRoom.RoomId == int(configs.GetSpecialRoomsConfig().DeathRecoveryRoom) {
			continue
		}

		// If it has never been set, set it to the filepath
		if _, ok := roomsWithoutEntrances[loadedRoom.RoomId]; !ok {
			roomsWithoutEntrances[loadedRoom.RoomId] = loadedRoom.Filepath()
		}

		for _, exit := range loadedRoom.Exits {
			roomsWithoutEntrances[exit.RoomId] = ``
		}

	}

	for roomId, filePath := range roomsWithoutEntrances {

		if filePath == `` {
			delete(roomsWithoutEntrances, roomId)
			continue
		}

		mudlog.Warn("No Entrance", "roomId", roomId, "filePath", filePath)
	}

	for _, loadedRoom := range loadedRooms {
		// Keep track of the highest roomId

		if loadedRoom.RoomId >= nextRoomId {
			nextRoomId = loadedRoom.RoomId + 1
		}

		// Cache the file path for every roomId
		roomManager.roomIdToFileCache[loadedRoom.RoomId] = loadedRoom.Filepath()

		// Update the zone info cache
		if _, ok := roomManager.zones[loadedRoom.Zone]; !ok {
			// Form one?
			return fmt.Errorf("No zone-config.yaml was loaded for roomId: %d zone: %s", loadedRoom.RoomId, loadedRoom.Zone)
		}
	}

	mudlog.Info("rooms.loadAllRoomZones()", "zoneCount", len(loadedZones), "loadedCount", len(loadedRooms), "Time Taken", time.Since(start))

	return nil
}
