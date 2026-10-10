// Command opcodecatalog generates world packet constants and labels from a
// server's public Opcodes.h: AzerothCore's for build 12340 (tmp/client-data,
// package opcode) or cMaNGOS Classic's for build 5875 (tmp/client-data-5875,
// package opcode/classic). It verifies the pinned header's checksum and never
// accesses the network or running server.
//
// Run from the repository root:
//
//	./run client-data:fetch [5875]
//	./run opcodecatalog:generate [5875]
package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/format"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/agent-wow/agent-wow/tools/internal/clientdata"
)

type entry struct {
	name, label string
	value       uint32
}

// catalog describes one build's opcode source and generated package.
type catalog struct {
	build           int
	release, server string
	directory, url  string
	sha256          string
	enum            *regexp.Regexp
	null            string // label of opcode 0
	pkg, constants  string
	names           string
	explicitLimit   bool // the enum ends with NUM_MSG_TYPES
}

var catalogs = map[string]catalog{
	"12340": {
		build: 12340, release: "WoW 3.3.5a", server: "AzerothCore",
		directory: clientdata.Directory, url: clientdata.OpcodesURL, sha256: clientdata.OpcodesSHA256,
		enum: regexp.MustCompile(`(?s)enum Opcodes : uint16\s*\{(.*?)\};`), null: "NULL_OPCODE",
		pkg: "opcode", constants: "pkg/opcode/world_gen.go", names: "pkg/opcode/world_names_gen.go", explicitLimit: true,
	},
	"5875": {
		build: 5875, release: "WoW 1.12.1", server: "cMaNGOS Classic",
		directory: clientdata.ClassicDirectory, url: clientdata.ClassicOpcodesURL, sha256: clientdata.ClassicOpcodesSHA256,
		enum: regexp.MustCompile(`(?s)enum Opcodes\s*\{(.*?)\};`), null: "MSG_NULL_ACTION",
		pkg: "classic", constants: "pkg/opcode/classic/world_gen.go", names: "pkg/opcode/classic/world_names_gen.go",
	},
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println("Usage: ./run opcodecatalog:generate [12340|5875]\nReads Opcodes.h from tmp/client-data (12340, default) or tmp/client-data-5875 (5875) and writes the build's world_gen.go and world_names_gen.go. Run './run client-data:fetch [5875]' first.")
		return
	}
	build := "12340"
	if len(os.Args) == 2 {
		build = os.Args[1]
	}
	cat, ok := catalogs[build]
	if !ok || len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "Usage: ./run opcodecatalog:generate [12340|5875]")
		os.Exit(2)
	}
	if err := run(cat); err != nil {
		fmt.Fprintln(os.Stderr, "generate opcode catalog:", err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s and %s\n", cat.constants, cat.names)
}

func run(cat catalog) error {
	data, err := clientdata.Read(cat.directory, "Opcodes.h")
	if err != nil {
		return err
	}
	constants, names, err := generate(data, cat)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cat.constants, constants, 0o644); err != nil {
		return err
	}
	return os.WriteFile(cat.names, names, 0o644)
}

func generate(data []byte, cat catalog) ([]byte, []byte, error) {
	if fmt.Sprintf("%x", sha256.Sum256(data)) != cat.sha256 {
		return nil, nil, fmt.Errorf("Opcodes.h checksum mismatch; fetch the header from %s", cat.url)
	}
	enum := cat.enum.FindSubmatch(data)
	if enum == nil {
		return nil, nil, errors.New("missing Opcodes enum")
	}
	assignment := regexp.MustCompile(`^([A-Z][A-Z0-9_]+)\s*=\s*(0x[0-9A-Fa-f]+)$`)
	nullName := goName(cat.null)
	entries := []entry{{nullName, cat.null, 0}}
	identifiers := map[string]bool{nullName: true, "NumMsgTypes": true}
	if !cat.explicitLimit {
		// The first enum line defines opcode 0, already listed above.
		first := regexp.MustCompile(`(?m)^\s*` + cat.null + `\s*=\s*0x0+,`)
		if first.FindIndex(enum[1]) == nil {
			return nil, nil, fmt.Errorf("missing %s definition", cat.null)
		}
		enum[1] = first.ReplaceAll(enum[1], nil)
	}
	var limit uint32
	for _, line := range strings.Split(string(enum[1]), "\n") {
		line, _, _ = strings.Cut(line, "//")
		line = strings.TrimSuffix(strings.TrimSpace(line), ",")
		if line == "" {
			continue
		}
		match := assignment.FindStringSubmatch(line)
		if match == nil || limit != 0 {
			return nil, nil, fmt.Errorf("unexpected opcode definition %q", line)
		}
		value, err := strconv.ParseUint(match[2], 0, 16)
		if err != nil {
			return nil, nil, err
		}
		if value != uint64(len(entries)) {
			return nil, nil, fmt.Errorf("expected opcode 0x%03x, got %s", len(entries), match[2])
		}
		if match[1] == "NUM_MSG_TYPES" {
			limit = uint32(value)
			continue
		}
		name := goName(match[1])
		if identifiers[name] {
			return nil, nil, fmt.Errorf("duplicate Go identifier %s", name)
		}
		identifiers[name] = true
		entries = append(entries, entry{name, match[1], uint32(value)})
	}
	if !cat.explicitLimit {
		limit = uint32(len(entries))
	}
	if limit == 0 {
		return nil, nil, errors.New("missing NUM_MSG_TYPES boundary")
	}
	if cat.explicitLimit && !regexp.MustCompile(`\bNULL_OPCODE\s*=\s*0x0000\b`).Match(data) {
		return nil, nil, errors.New("missing NULL_OPCODE definition")
	}
	var constants, names bytes.Buffer
	for _, out := range []*bytes.Buffer{&constants, &names} {
		command := "./run opcodecatalog:generate"
		if !cat.explicitLimit {
			command += fmt.Sprintf(" %d", cat.build)
		}
		fmt.Fprintf(out, "// Code generated by %s; DO NOT EDIT.\n", command)
		fmt.Fprintf(out, "// %s, build %d.\n", cat.release, cat.build)
		if !cat.explicitLimit {
			fmt.Fprintln(out, "// Server: "+cat.server)
		}
		fmt.Fprintln(out, "// Source: "+cat.url)
		fmt.Fprintln(out, "// SHA-256: "+cat.sha256)
		fmt.Fprintln(out, "\npackage "+cat.pkg)
	}
	fmt.Fprintln(&constants, "\n// World packet identifiers, including obsolete and server-specific entries.")
	fmt.Fprintln(&constants, "// An identifier being defined does not imply client or server support for it.")
	fmt.Fprintln(&constants, "const (")
	for _, op := range entries {
		fmt.Fprintf(&constants, "%s = 0x%03x // %s\n", op.name, op.value, op.label)
	}
	fmt.Fprintln(&constants, ")")
	fmt.Fprintln(&constants, "\n// NumMsgTypes is the exclusive upper bound of the catalog, not a packet opcode.")
	fmt.Fprintf(&constants, "const NumMsgTypes = 0x%03x\n", limit)
	fmt.Fprintln(&names, "\nvar worldNames = [...]string{")
	for _, op := range entries {
		fmt.Fprintf(&names, "%s: %q,\n", op.name, op.label)
	}
	fmt.Fprintln(&names, "}")
	world, err := format.Source(constants.Bytes())
	if err != nil {
		return nil, nil, err
	}
	labels, err := format.Source(names.Bytes())
	return world, labels, err
}

func goName(label string) string {
	parts := strings.Split(label, "_")
	for i, part := range parts {
		switch part {
		case "CMSG", "SMSG", "MSG", "UMSG", "TC9", "ID", "GUID", "AI", "GM", "NPC", "UI", "PVP", "PVE", "AFK", "DND", "LFG", "LFD", "LFR", "GMTICKET", "SRP6", "VOIP", "MOTD", "PCT", "XP":
		default:
			parts[i] = part[:1] + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "")
}
