package pipeline

type Vars map[string]any

func NewVars(targetValue any) Vars {
	v := Vars{
		"target": targetValue,
	}
	return v
}
