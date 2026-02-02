package action

const (
	// Actions are int8 and we reserve positive integers so constants are all <0
	NULL_ACTION Action = 0
	ALL         Action = -1
	ON          Action = -2
	OFF         Action = -3
)

// Action defines which behaviors should be applied to resources
type Action int8

// ToAction turns an int to an Action
func ToAction(i int) Action {
	return Action(i)
}
