package scheduler

// Actions are int8 and we reserve positive integers so constants are all <0
const (
	OFF         Action = 0
	ON          Action = -1
	NULL_ACTION Action = -2
)

type Action int8

type Scheduler interface {
	Evaluate(ScheduleInput) Action
}

type ScheduleInput interface {
	Parse() string
}
