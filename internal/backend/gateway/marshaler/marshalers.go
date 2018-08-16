package marshaler

// Type defines the marshaler type.
type Type int

// Marshaler types.
const (
	V2JSON Type = iota
	Protobuf
	JSON
)
