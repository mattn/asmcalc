package mame

const (
	TagInt = 0
)

type Value struct {
	Tag int
	I   int
}

func intVal(n int) Value { return Value{Tag: TagInt, I: n} }
