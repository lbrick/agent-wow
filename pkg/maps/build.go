package maps

import "strconv"

// Labels resolves map and zone IDs for one client build.
type Labels struct {
	maps, zones map[uint32]string
}

// ForBuild returns the labels for a client build: 5875 uses the WoW 1.12.1
// tables; any other build uses the 3.3.5a (12340) tables.
func ForBuild(build uint16) Labels {
	if build == 5875 {
		return Labels{classicMapNames, classicZoneNames}
	}
	return Labels{mapNames, zoneNames}
}

// MapName returns the English label for a map ID, or "Unknown map (<id>)".
func (l Labels) MapName(id uint32) string {
	if name, ok := l.maps[id]; ok {
		return name
	}
	return "Unknown map (" + strconv.FormatUint(uint64(id), 10) + ")"
}

// ZoneName returns the English label for a zone or subzone ID, or "Unknown zone (<id>)".
func (l Labels) ZoneName(id uint32) string {
	if name, ok := l.zones[id]; ok {
		return name
	}
	return "Unknown zone (" + strconv.FormatUint(uint64(id), 10) + ")"
}
