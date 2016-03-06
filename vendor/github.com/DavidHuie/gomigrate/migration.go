// Holds metadata about a migration.

package gomigrate

// Migration statuses.
const (
	Inactive = iota
	Active
)

// Holds configuration information for a given migration.
type Migration struct {
	DownPath string
	Id       uint64
	Name     string
	Status   int
	UpPath   string
}

// Performs a basic validation of a migration.
func (m *Migration) valid() bool {
	if m.Id != 0 && m.Name != "" && m.UpPath != "" && m.DownPath != "" {
		return true
	}
	return false
}
