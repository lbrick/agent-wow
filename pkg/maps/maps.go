// Package maps resolves map and zone IDs to stock English labels. The package
// functions use WoW 3.3.5a (build 12340) tables; ForBuild selects a build's
// tables, including WoW 1.12.1 (build 5875). Zone IDs refer to AreaTable
// entries, including subzones. The bundled data requires no client files or
// network access at runtime.
package maps

import "strconv"

// MapName returns the English label for a map ID. Map 0 is Eastern Kingdoms.
// Unknown IDs return "Unknown map (<id>)" so custom IDs remain identifiable.
func MapName(id uint32) string {
	if name, ok := mapNames[id]; ok {
		return name
	}
	return "Unknown map (" + strconv.FormatUint(uint64(id), 10) + ")"
}

// ZoneName returns the English label for a zone or subzone ID.
// Unknown IDs, including 0, return "Unknown zone (<id>)".
func ZoneName(id uint32) string {
	if name, ok := zoneNames[id]; ok {
		return name
	}
	return "Unknown zone (" + strconv.FormatUint(uint64(id), 10) + ")"
}
