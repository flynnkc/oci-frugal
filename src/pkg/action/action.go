package action

const (
	NULL_ACTION Action = 0      // 00000000
	OFF         Action = 1 << 0 // 00000001
	ON          Action = 1 << 1 // 00000010
	ALL         Action = 255    // 11111111
)

// Action defines which behaviors should be applied to resources
type Action uint8

// ToAction turns an int to an Action
func ToAction(i int) Action {
	return Action(i)
}

func Compare(a Action, b Action) bool {
	return (a & b) > 0
}
