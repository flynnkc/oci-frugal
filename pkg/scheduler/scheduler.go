package scheduler

type Scheduler interface {
	Evaluate(string) bool
}
