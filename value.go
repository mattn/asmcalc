package mame

const (
	TagInt = 0
	TagStr = 1
)

type Value struct {
	Tag int
	I   int
	S   string
}

func intVal(n int) Value    { return Value{Tag: TagInt, I: n} }
func strVal(s string) Value { return Value{Tag: TagStr, S: s} }
