package cursor

import "fmt"

// Speed é a velocidade configurada (--cursor-speed).
type Speed string

const (
	Slow    Speed = "slow"
	Normal  Speed = "normal"
	Fast    Speed = "fast"
	Instant Speed = "instant"
)

// ParseSpeed valida o valor da flag.
func ParseSpeed(s string) (Speed, error) {
	switch v := Speed(s); v {
	case Slow, Normal, Fast, Instant:
		return v, nil
	}
	return "", fmt.Errorf("cursor speed %q: use slow, normal, fast or instant", s)
}

// Factor multiplica as durações (movimento, mira, press, submovimentos e os tetos).
func (s Speed) Factor() float64 {
	switch s {
	case Slow:
		return 1.6
	case Fast:
		return 0.6
	case Instant:
		return 0
	}
	return 1
}

// Profile tem todos os parâmetros do movimento. Os padrões ficam em DefaultProfile.
type Profile struct {
	Speed Speed

	// Ponto de destino (4.1).
	Tries int     // sorteios antes do ponto de repouso
	Sigma float64 // desvio do ponto, fração da largura e da altura
	Inner float64 // fração interna do alvo onde o ponto pode cair

	// Duração pela Lei de Fitts (4.2), em ms.
	FittsA, FittsB   float64
	Jitter           float64 // variação máxima da duração (±)
	MinMove, MaxMove float64
	Still            float64 // px: abaixo disso, sem movimento
	MinWidth         float64 // px: W mínimo

	// Trajetória (4.3).
	ArcMin, ArcMax       float64 // deslocamento do 1º ponto de controle, fração de D
	ArcLimit, ArcLimitPx float64 // desvio máximo da reta
	SameSide             float64 // chance de repetir o lado do arco

	// Velocidade (4.4): fração da duração onde fica o pico.
	PeakMin, PeakMax float64

	// Submovimentos (4.5).
	SubmoveID          float64
	Undershoot         float64 // chance de parar antes (o resto passa do ponto)
	UnderMin, UnderMax float64 // fração de D coberta ao parar antes
	UnderLateral       float64 // erro lateral máximo, fração de W
	OverMin, OverMax   float64 // fração de D além do ponto
	OverPx             float64
	CorrMin, CorrMax   float64 // ms do submovimento corretivo
	CorrCurve          float64 // curvatura máxima do corretivo, fração da sua distância
	GapMin, GapMax     float64 // ms parado entre os dois

	// Tremor (4.6).
	TremorAmp                float64 // px
	TremorHzMin, TremorHzMax float64
	TremorEdge               float64 // fração do começo e do fim sem tremor

	// Clique (4.6), em ms.
	AimMin, AimMax     float64
	PressMin, PressMax float64

	ShapeLead   float64 // ms antes de chegar em que a forma muda
	Frame       float64 // ms entre keyframes
	FadeInstant float64 // ms do fade no destino com speed instant
	FadeReduced float64 // ms do fade com prefers-reduced-motion

	// Coreografia (seção 5), em ms e px.
	AwayMin, AwayMax         float64 // afastar do campo depois do clique (5.2)
	AwayDistMin, AwayDistMax float64
	KeyMin, KeyMax           float64 // intervalo entre teclas no ritmo de digitação (5.2)
	TypeCap                  float64 // teto do ritmo; o resto entra de uma vez
	WheelStep                float64 // px por passo de roda (5.4)
	WheelMin, WheelMax       float64 // ms entre passos
	WheelCap                 float64
	ScrollCap                float64 // rolagem suave até o alvo antes do clique (5.1)
}

// DefaultProfile é o ponto de partida da velocidade normal (seção 4).
func DefaultProfile() Profile {
	return Profile{
		Speed: Normal,
		Tries: 5, Sigma: 0.15, Inner: 0.70,
		FittsA: 100, FittsB: 110, Jitter: 0.08, MinMove: 250, MaxMove: 900, Still: 3, MinWidth: 8,
		ArcMin: 0.04, ArcMax: 0.12, ArcLimit: 0.15, ArcLimitPx: 120, SameSide: 0.65,
		PeakMin: 0.35, PeakMax: 0.45,
		SubmoveID: 3.5, Undershoot: 0.70, UnderMin: 0.92, UnderMax: 0.97, UnderLateral: 0.3,
		OverMin: 0.02, OverMax: 0.06, OverPx: 24, CorrMin: 80, CorrMax: 160, CorrCurve: 0.02,
		GapMin: 20, GapMax: 60,
		TremorAmp: 0.4, TremorHzMin: 8, TremorHzMax: 12, TremorEdge: 0.10,
		AimMin: 70, AimMax: 140, PressMin: 60, PressMax: 110,
		ShapeLead: 80, Frame: 16, FadeInstant: 80, FadeReduced: 120,
		AwayMin: 150, AwayMax: 250, AwayDistMin: 40, AwayDistMax: 80,
		KeyMin: 25, KeyMax: 45, TypeCap: 1500,
		WheelStep: 100, WheelMin: 60, WheelMax: 90, WheelCap: 800,
		ScrollCap: 600,
	}
}
