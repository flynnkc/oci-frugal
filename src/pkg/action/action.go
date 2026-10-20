package action

const (
	NULL_ACTION Action = 0      // 00000000
	OFF         Action = 1 << 0 // 00000001
	ON          Action = 1 << 1 // 00000010
	ALL         Action = 255    // 11111111
)

// Action defines which behaviors should be applied to resources
type Action uint8

// Compare checks an execution filter against a concrete power action.
func Compare(filter Action, decision Action) bool {
	return (decision == ON || decision == OFF) && (filter == ALL || filter == decision)
}

func (a Action) String() string {
	switch a {
	case ON:
		return "on"
	case OFF:
		return "off"
	case ALL:
		return "all"
	default:
		return "none"
	}
}
