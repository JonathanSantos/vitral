package cursor

import "encoding/json"

// wire é o formato compacto que vai para o overlay: keyframes num array plano, sem objeto por ponto.
type wire struct {
	F       []float64 `json:"f"`
	Shape   string    `json:"shape"`
	ShapeAt float64   `json:"shapeAt"`
	Move    float64   `json:"move"`
	Press   float64   `json:"press"`
	Release float64   `json:"release"`
	Fade    float64   `json:"fade"`
}

// MarshalJSON envia só o que o overlay precisa para tocar o plano.
func (p Plan) MarshalJSON() ([]byte, error) {
	return json.Marshal(wire{p.Frames, p.Shape.String(), p.ShapeAt, p.Move, p.Press, p.Release, p.Fade})
}
