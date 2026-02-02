package regresql

// NullGenerator always generates NULL
type NullGenerator struct {
	BaseGenerator
}

func NewNullGenerator() *NullGenerator {
	return &NullGenerator{
		BaseGenerator: BaseGenerator{name: "null"},
	}
}

func (g *NullGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	return nil, nil
}

func (g *NullGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	return nil
}

// ConstantGenerator generates a constant value
type ConstantGenerator struct {
	BaseGenerator
}

func NewConstantGenerator() *ConstantGenerator {
	return &ConstantGenerator{
		BaseGenerator: BaseGenerator{name: "constant"},
	}
}

func (g *ConstantGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	val, ok := params["value"]
	if !ok {
		return nil, nil
	}
	return val, nil
}

func (g *ConstantGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	return nil
}
