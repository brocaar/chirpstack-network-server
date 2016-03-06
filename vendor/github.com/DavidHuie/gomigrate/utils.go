package gomigrate

import (
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	upMigrationFile   = regexp.MustCompile(`(\d+)_([\w-]+)_up\.sql`)
	downMigrationFile = regexp.MustCompile(`(\d+)_([\w-]+)_down\.sql`)
	subMigrationSplit = regexp.MustCompile(`;\s*`)
	allWhitespace     = regexp.MustCompile(`^\s*$`)
)

// Returns the migration number, type and base name, so 1, "up", "migration" from "01_migration_up.sql"
func parseMigrationPath(path string) (uint64, migrationType, string, error) {
	filebase := filepath.Base(path)

	matches := upMigrationFile.FindAllSubmatch([]byte(filebase), -1)
	if matches != nil {
		return parseMatches(matches, upMigration)
	}
	matches = downMigrationFile.FindAllSubmatch([]byte(filebase), -1)
	if matches != nil {
		return parseMatches(matches, downMigration)
	}

	return 0, "", "", InvalidMigrationFile
}

// Parses matches given by a migration file regex.
func parseMatches(matches [][][]byte, mType migrationType) (uint64, migrationType, string, error) {
	num := matches[0][1]
	name := matches[0][2]
	parsedNum, err := strconv.ParseUint(string(num), 10, 64)
	if err != nil {
		return 0, "", "", err
	}
	return parsedNum, mType, string(name), nil
}

// This type is used to sort migration ids.
type uint64slice []uint64

func (u uint64slice) Len() int {
	return len(u)
}

func (u uint64slice) Less(a, b int) bool {
	return u[a] < u[b]
}

func (u uint64slice) Swap(a, b int) {
	tempA := u[a]
	u[a] = u[b]
	u[b] = tempA
}
