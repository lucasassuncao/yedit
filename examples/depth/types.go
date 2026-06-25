package main

// SharedNode is reused across blocks to demonstrate Metadata-driven Presentation
// overrides: alpha.portal.mid.shared renders it expandable (no Presentation set);
// epsilon.forced renders the same type as overlay (PresentationOverlay forced).
type SharedNode struct {
	Name  string `yaml:"name"`
	Score int    `yaml:"score"`
	Tag   string `yaml:"tag"`
}

// ── alpha: O → E → O → E → E → O ────────────────────────────────────────────
// portal(O→L2) · mid(E) · bridge(O→L3) · row(E) · sub(E) · sink(O→L4)

type AlphaSink struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
	Flag  bool   `yaml:"flag"`
}

type AlphaSub struct {
	Code string    `yaml:"code"`
	Sink AlphaSink `yaml:"sink"` // O via Metadata → L4
}

type AlphaRow struct {
	Name string   `yaml:"name"`
	Sub  AlphaSub `yaml:"sub"` // E: expands inline
}

type AlphaBridge struct {
	Label string   `yaml:"label"`
	Row   AlphaRow `yaml:"row"` // E: expands inline
}

type AlphaMid struct {
	Title  string      `yaml:"title"`
	Bridge AlphaBridge `yaml:"bridge"` // O via Metadata → L3
	Shared SharedNode  `yaml:"shared"` // E: expandable (no Presentation override; contrast with epsilon.forced)
}

type AlphaPortal struct {
	Info string   `yaml:"info"`
	Mid  AlphaMid `yaml:"mid"` // E: expands inline in L2
}

type Alpha struct {
	Desc   string      `yaml:"desc"`
	Portal AlphaPortal `yaml:"portal"` // O via Metadata → L2
}

// ── beta: E → O → O → E → E → O ─────────────────────────────────────────────
// wrap(E) · gate_a(O→L2) · gate_b(O→L3) · row(E) · sub(E) · end(O→L4)

type BetaEnd struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
	Flag  bool   `yaml:"flag"`
}

type BetaSub struct {
	Code string  `yaml:"code"`
	End  BetaEnd `yaml:"end"` // O via Metadata → L4
}

type BetaRow struct {
	Name string  `yaml:"name"`
	Sub  BetaSub `yaml:"sub"` // E: expands inline
}

type BetaGateB struct {
	Label string  `yaml:"label"`
	Row   BetaRow `yaml:"row"` // E: expands inline
}

type BetaGateA struct {
	Code  string    `yaml:"code"`
	GateB BetaGateB `yaml:"gate_b"` // O via Metadata → L3
}

type BetaWrap struct {
	Title string    `yaml:"title"`
	GateA BetaGateA `yaml:"gate_a"` // O via Metadata → L2
}

type Beta struct {
	Desc string   `yaml:"desc"`
	Wrap BetaWrap `yaml:"wrap"` // E: expands inline
}

// ── gamma: O → O → E → E → O → E ────────────────────────────────────────────
// door_a(O→L2) · door_b(O→L3) · span_a(E) · span_b(E) · hop(O→L4) · tail(E)

type GammaTail struct {
	X string `yaml:"x"`
	Y int    `yaml:"y"`
}

type GammaHop struct {
	Key  string    `yaml:"key"`
	Tail GammaTail `yaml:"tail"` // E: expands inline in L4
}

type GammaSpanB struct {
	Code string   `yaml:"code"`
	Hop  GammaHop `yaml:"hop"` // O via Metadata → L4
}

type GammaSpanA struct {
	Name  string     `yaml:"name"`
	SpanB GammaSpanB `yaml:"span_b"` // E: expands inline
}

type GammaDoorB struct {
	Label string     `yaml:"label"`
	SpanA GammaSpanA `yaml:"span_a"` // E: expands inline
}

type GammaDoorA struct {
	Title string     `yaml:"title"`
	DoorB GammaDoorB `yaml:"door_b"` // O via Metadata → L3
}

type Gamma struct {
	Desc  string     `yaml:"desc"`
	DoorA GammaDoorA `yaml:"door_a"` // O via Metadata → L2
}

// ── delta: E → E → O → O → E → O ────────────────────────────────────────────
// row_a(E) · row_b(E) · jump_a(O→L2) · jump_b(O→L3) · after(E) · final(O→L4)

type DeltaFinal struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
	Flag  bool   `yaml:"flag"`
}

type DeltaAfter struct {
	Code  string     `yaml:"code"`
	Final DeltaFinal `yaml:"final"` // O via Metadata → L4
}

type DeltaJumpB struct {
	Label string     `yaml:"label"`
	After DeltaAfter `yaml:"after"` // E: expands inline
}

type DeltaJumpA struct {
	Info  string     `yaml:"info"`
	JumpB DeltaJumpB `yaml:"jump_b"` // O via Metadata → L3
}

type DeltaRowB struct {
	Name  string     `yaml:"name"`
	JumpA DeltaJumpA `yaml:"jump_a"` // O via Metadata → L2
}

type DeltaRowA struct {
	Title string    `yaml:"title"`
	RowB  DeltaRowB `yaml:"row_b"` // E: expands inline
}

type Delta struct {
	Desc string    `yaml:"desc"`
	RowA DeltaRowA `yaml:"row_a"` // E: expands inline
}

// ── epsilon: O → E → E → O → O → E ──────────────────────────────────────────
// entry(O→L2) · stretch(E) · deep(E) · hop_a(O→L3) · hop_b(O→L4) · foot(E)
// + forced: SharedNode forced overlay (same type as alpha.portal.mid.shared but different Presentation)

type EpsilonFoot struct {
	X string `yaml:"x"`
	Y int    `yaml:"y"`
}

type EpsilonHopB struct {
	Key  string      `yaml:"key"`
	Foot EpsilonFoot `yaml:"foot"` // E: expands inline in L4
}

type EpsilonHopA struct {
	Code string      `yaml:"code"`
	HopB EpsilonHopB `yaml:"hop_b"` // O via Metadata → L4
}

type EpsilonDeep struct {
	Label string      `yaml:"label"`
	HopA  EpsilonHopA `yaml:"hop_a"` // O via Metadata → L3
}

type EpsilonStretch struct {
	Name string      `yaml:"name"`
	Deep EpsilonDeep `yaml:"deep"` // E: expands inline
}

type EpsilonEntry struct {
	Info    string         `yaml:"info"`
	Stretch EpsilonStretch `yaml:"stretch"` // E: expands inline in L2
}

type Epsilon struct {
	Desc   string       `yaml:"desc"`
	Entry  EpsilonEntry `yaml:"entry"`  // O via Metadata → L2
	Forced SharedNode   `yaml:"forced"` // O: PresentationOverlay forced via Metadata (same type as alpha.portal.mid.shared, rendered expandable there)
}

// ── Root config ───────────────────────────────────────────────────────────────

type DepthConfig struct {
	Alpha   Alpha   `yaml:"alpha"`
	Beta    Beta    `yaml:"beta"`
	Gamma   Gamma   `yaml:"gamma"`
	Delta   Delta   `yaml:"delta"`
	Epsilon Epsilon `yaml:"epsilon"`
}
