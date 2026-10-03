package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/flow"
)

// Device é um perfil simples de tela.
type Device struct {
	Width, Height int
	Scale         float64
	Mobile        bool
	UserAgent     string
}

// Devices são os perfis aceitos por emulate. desktop volta ao normal.
var Devices = map[string]Device{
	"mobile": {Width: 390, Height: 844, Scale: 3, Mobile: true,
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"},
	"tablet": {Width: 820, Height: 1180, Scale: 2, Mobile: true,
		UserAgent: "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"},
	"desktop": {},
}

// NetworkProfile segue os perfis do DevTools (bytes por segundo).
type NetworkProfile struct {
	Offline          bool
	LatencyMs        float64
	Download, Upload float64
}

// NetworkProfiles são os perfis aceitos por emulate. none remove a limitação.
var NetworkProfiles = map[string]NetworkProfile{
	"offline": {Offline: true},
	"slow3g":  {LatencyMs: 2000, Download: 50_000, Upload: 50_000},
	"fast3g":  {LatencyMs: 563, Download: 180_000, Upload: 84_375},
	"none":    {Download: -1, Upload: -1},
}

// Emulation é o estado pedido; é reaplicado se a aba for reaberta.
type Emulation struct {
	Device  string
	Network string
}

// Emulate muda o perfil de tela e/ou de rede da página.
func (s *Session) Emulate(ctx context.Context, device, network string) (string, error) {
	device, network = strings.ToLower(strings.TrimSpace(device)), strings.ToLower(strings.TrimSpace(network))
	if device == "" && network == "" {
		return "", fmt.Errorf("provide device (mobile, tablet, desktop) or network (offline, slow3g, fast3g, none)")
	}
	if _, ok := Devices[device]; device != "" && !ok {
		return "", fmt.Errorf("unknown device %q. Use mobile, tablet or desktop.", device)
	}
	if _, ok := NetworkProfiles[network]; network != "" && !ok {
		return "", fmt.Errorf("unknown network %q. Use offline, slow3g, fast3g or none.", network)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return "", err
	}
	if device != "" {
		if err := applyDevice(p.Context(ctx), device); err != nil {
			return "", err
		}
		s.emulation.Device = device
	}
	if network != "" {
		if err := applyNetwork(p.Context(ctx), network); err != nil {
			return "", err
		}
		s.emulation.Network = network
	}
	// A troca de viewport muda o layout: o próximo diff seria só ruído.
	s.last = nil
	rec := &recording{step: func(*flow.Selector) flow.Step {
		return flow.Step{Emulate: &flow.EmulateStep{Device: device, Network: network}}
	}}
	return joinNonEmpty(describeEmulation(s.emulation), s.record(rec, nil, "")), nil
}

func describeEmulation(e Emulation) string {
	device, network := e.Device, e.Network
	if device == "" {
		device = "desktop"
	}
	if network == "" {
		network = "none"
	}
	d := Devices[device]
	size := ""
	if d.Width > 0 {
		size = fmt.Sprintf(" (%dx%d, touch)", d.Width, d.Height)
	}
	return fmt.Sprintf("ok: device %s%s, network %s. Call snapshot to see the page.", device, size, network)
}

func applyDevice(p *rod.Page, name string) error {
	d := Devices[name]
	if name == "desktop" {
		if err := (proto.EmulationClearDeviceMetricsOverride{}).Call(p); err != nil {
			return fmt.Errorf("reset viewport: %w", err)
		}
		if err := (proto.EmulationSetTouchEmulationEnabled{Enabled: false}).Call(p); err != nil {
			return err
		}
		version, err := proto.BrowserGetVersion{}.Call(p)
		if err != nil {
			return err
		}
		return proto.NetworkSetUserAgentOverride{UserAgent: version.UserAgent}.Call(p)
	}
	metrics := proto.EmulationSetDeviceMetricsOverride{Width: d.Width, Height: d.Height, DeviceScaleFactor: d.Scale, Mobile: d.Mobile}
	if err := metrics.Call(p); err != nil {
		return fmt.Errorf("set viewport: %w", err)
	}
	if err := (proto.EmulationSetTouchEmulationEnabled{Enabled: true, MaxTouchPoints: new(5)}).Call(p); err != nil {
		return err
	}
	return proto.NetworkSetUserAgentOverride{UserAgent: d.UserAgent}.Call(p)
}

func applyNetwork(p *rod.Page, name string) error {
	n := NetworkProfiles[name]
	return proto.NetworkEmulateNetworkConditions{
		Offline:            n.Offline,
		Latency:            n.LatencyMs,
		DownloadThroughput: n.Download,
		UploadThroughput:   n.Upload,
	}.Call(p)
}

// reapplyEmulation restaura a emulação numa aba nova.
func (s *Session) reapplyEmulation(p *rod.Page) error {
	if s.emulation.Device != "" && s.emulation.Device != "desktop" {
		if err := applyDevice(p, s.emulation.Device); err != nil {
			return err
		}
	}
	if s.emulation.Network != "" && s.emulation.Network != "none" {
		return applyNetwork(p, s.emulation.Network)
	}
	return nil
}
