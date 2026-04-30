package mame

const (
	TagInt   = 0
	TagStr   = 1
	TagFloat = 2
)

type Value struct {
	Tag int
	I   int
	S   string
	F   float64
}

func intVal(n int) Value       { return Value{Tag: TagInt, I: n} }
func strVal(s string) Value    { return Value{Tag: TagStr, S: s} }
func floatVal(f float64) Value { return Value{Tag: TagFloat, F: f} }
