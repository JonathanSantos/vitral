package session

import (
	"context"
	"fmt"
	"image"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/screenshot"
	"github.com/JonathanSantos/vitral/internal/snapshot"
)

// maxFullPageHeight limita prints de página inteira para não gerar imagens gigantes.
const maxFullPageHeight = 10000

// ShotOptions são as opções da ferramenta screenshot.
type ShotOptions struct {
	Marks    bool
	Ref      string
	FullPage bool
}

// Shot é o PNG final e uma legenda curta.
type Shot struct {
	PNG     []byte
	Caption string
}

// Screenshot captura a tela e desenha as refs do snapshot por cima (em Go, sem mexer no DOM).
func (s *Session) Screenshot(ctx context.Context, opts ShotOptions) (Shot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return Shot{}, err
	}
	if opts.Ref != "" {
		if err := scrollIntoView(ctx, p, opts.Ref); err != nil {
			return Shot{}, err
		}
	}

	// O selo e o destaque do Vitral nunca aparecem no print.
	var (
		snap   *snapshot.Page
		img    image.Image
		offset image.Point
	)
	err = s.ui.Hidden(ctx, p, func() error {
		var err error
		snap, img, offset, err = s.captureImage(ctx, p, opts.FullPage)
		return err
	})
	if err != nil {
		return Shot{}, err
	}
	marks := screenshot.Visible(screenshot.MarksFrom(snap.Nodes, offset), img.Bounds())
	if opts.Marks && len(marks) > 0 {
		img = screenshot.Annotate(img, marks)
	}

	caption := fmt.Sprintf("screenshot %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	if opts.Marks {
		caption += fmt.Sprintf(", %d marks with the same refs as the snapshot", len(marks))
	}
	if opts.Ref != "" {
		var box *[4]int
		if err := evalInto(ctx, p, &box, `(r) => window.__vitral.box(r)`, opts.Ref); err != nil {
			return Shot{}, err
		}
		if box == nil {
			return Shot{}, snapshot.ErrRefNotFound{Ref: opts.Ref}
		}
		r := image.Rect(box[0], box[1], box[0]+box[2], box[1]+box[3]).Add(offset)
		img = screenshot.Crop(img, r, 16)
		caption = fmt.Sprintf("element %s, %dx%d", opts.Ref, img.Bounds().Dx(), img.Bounds().Dy())
	}
	data, err := screenshot.Encode(img)
	if err != nil {
		return Shot{}, err
	}
	return Shot{PNG: data, Caption: caption}, nil
}

func scrollIntoView(ctx context.Context, p *rod.Page, ref string) error {
	var code string
	js := `(r) => { const el = window.__vitral.find(r); if (!el) return 'not_found';
		el.scrollIntoView({block: 'center', inline: 'center', behavior: 'instant'}); return ''; }`
	if err := evalInto(ctx, p, &code, js, ref); err != nil {
		return err
	}
	return refError(ref, code)
}

// captureImage devolve o snapshot usado para as marcas, o print em pixels CSS e o deslocamento das caixas.
func (s *Session) captureImage(ctx context.Context, p *rod.Page, fullPage bool) (*snapshot.Page, image.Image, image.Point, error) {
	snap, err := capture(ctx, p, snapshot.Options{Full: fullPage})
	if err != nil {
		return nil, nil, image.Point{}, err
	}
	if !fullPage {
		s.last = snap
	}

	req := proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng}
	cssWidth := float64(snap.Viewport[0])
	offset := image.Point{}
	if fullPage {
		metrics, err := proto.PageGetLayoutMetrics{}.Call(p)
		if err != nil || metrics.CSSContentSize == nil {
			return nil, nil, offset, fmt.Errorf("read page size: %w", err)
		}
		size := metrics.CSSContentSize
		cssWidth = size.Width
		req.CaptureBeyondViewport = true
		req.Clip = &proto.PageViewport{Width: size.Width, Height: min(size.Height, maxFullPageHeight), Scale: 1}
		// As caixas vêm relativas à viewport; no print inteiro a origem é o topo do documento.
		offset = image.Pt(0, snap.Scroll[0])
	}
	shot, err := req.Call(p.Context(ctx))
	if err != nil {
		return nil, nil, offset, fmt.Errorf("capture screenshot: %w", err)
	}
	img, err := screenshot.Decode(shot.Data)
	if err != nil {
		return nil, nil, offset, err
	}
	scale := float64(img.Bounds().Dx()) / cssWidth
	return snap, screenshot.ToCSSPixels(img, scale), offset, nil
}

// Inspect devolve estilos computados, caixa, estados e atributos aria de um elemento.
func (s *Session) Inspect(ctx context.Context, ref string, props []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return "", err
	}
	if props == nil {
		props = []string{}
	}
	data, err := evalJSON(ctx, p, `(r, props) => window.__vitral.inspect(r, props)`, ref, props)
	if err != nil {
		return "", err
	}
	in, err := snapshot.ParseInspection(data, ref)
	if err != nil {
		return "", err
	}
	return snapshot.FormatInspection(in), nil
}
