package marshaler

// Type defines the marshaler type.
type Type int

// Marshaler types.
const (
	Protobuf Type = iota
	JSON
)
