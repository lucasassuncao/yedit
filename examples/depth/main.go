// Command depth is a yedit example that exercises Presentation combinations
// (PresentationOverlay, inline expandable) with alternating zigzag patterns at
// up to 6 nesting levels. Each block uses a distinct alternation so edge cases
// in overlay/expand transitions surface during manual testing.
//
// Run from the yedit root:
//
//	go run ./examples/depth
//	go run ./examples/depth --theme dracula
//
// # Presentation legend
//
//	O  PresentationOverlay  — field opens a new block editor (drill-in)
//	E  (default)            — field expands inline in the tree panel
//
// # Pattern table
//
//	Block    L1  L2  L3  L4  L5  L6   Notes
//	-------  --  --  --  --  --  --   -----------------------------------------------
//	alpha     O   E   O   E   E   O   portal(O)→mid(E)→bridge(O)→row(E)→sub(E)→sink(O)
//	beta      E   O   O   E   E   O   wrap(E)→gate_a(O)→gate_b(O)→row(E)→sub(E)→end(O)
//	gamma     O   O   E   E   O   E   door_a(O)→door_b(O)→span_a(E)→span_b(E)→hop(O)→tail(E)
//	delta     E   E   O   O   E   O   row_a(E)→row_b(E)→jump_a(O)→jump_b(O)→after(E)→final(O)
//	epsilon   O   E   E   O   O   E   entry(O)→stretch(E)→deep(E)→hop_a(O)→hop_b(O)→foot(E)
//
// # Metadata Presentation override (SharedNode)
//
// SharedNode (name/score/tag) appears twice with opposite Presentation:
//
//	alpha.portal.mid.shared  — no Presentation set → expandable inline (default)
//	epsilon.forced           — PresentationOverlay forced via Metadata → opens overlay
//
// Same Go type, different rendering — the Metadata controls which wins.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/metadata"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

var depthMetadata = mustBuildMetadata()

func mustBuildMetadata() editor.MetadataSource {
	src, err := metadata.NewFromTree(&DepthConfig{}, depthTree())
	if err != nil {
		panic(fmt.Sprintf("depthMetadata: %v", err))
	}
	return src
}

func depthTree() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"alpha":   alphaNode(),
		"beta":    betaNode(),
		"gamma":   gammaNode(),
		"delta":   deltaNode(),
		"epsilon": epsilonNode(),
	}
}

// ── alpha: O → E → O → E → E → O ────────────────────────────────────────────

func alphaNode() *metadata.Node {
	return &metadata.Node{
		FieldMeta: editor.FieldMeta{Description: "O→E→O→E→E→O: portal(O) wraps mid(E) which bridges(O) into row(E)→sub(E)→sink(O)."},
		Children: map[string]*metadata.Node{
			"desc": {FieldMeta: editor.FieldMeta{Description: "Block description."}},
			"portal": {
				FieldMeta: editor.FieldMeta{
					Presentation: schema.PresentationOverlay,
					Description:  "O: opens overlay L2 — first step of the chain.",
				},
				Children: map[string]*metadata.Node{
					"info": {FieldMeta: editor.FieldMeta{Description: "Portal info (L2 scalar)."}},
					"mid": {
						FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L2."},
						Children: map[string]*metadata.Node{
							"title": {FieldMeta: editor.FieldMeta{Description: "Mid title (L2 scalar)."}},
							"bridge": {
								FieldMeta: editor.FieldMeta{
									Presentation: schema.PresentationOverlay,
									Description:  "O: opens overlay L3 — second overlay in the chain.",
								},
								Children: map[string]*metadata.Node{
									"label": {FieldMeta: editor.FieldMeta{Description: "Bridge label (L3 scalar)."}},
									"row": {
										FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L3."},
										Children: map[string]*metadata.Node{
											"name": {FieldMeta: editor.FieldMeta{Description: "Row name (L3 scalar)."}},
											"sub": {
												FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L3 (second consecutive expand)."},
												Children: map[string]*metadata.Node{
													"code": {FieldMeta: editor.FieldMeta{Description: "Sub code (L3 scalar)."}},
													"sink": {
														FieldMeta: editor.FieldMeta{
															Presentation: schema.PresentationOverlay,
															Description:  "O: opens overlay L4 — terminal editor.",
														},
														Children: map[string]*metadata.Node{
															"key":   {FieldMeta: editor.FieldMeta{Description: "Sink key."}},
															"value": {FieldMeta: editor.FieldMeta{Description: "Sink value."}},
															"flag":  {FieldMeta: editor.FieldMeta{Description: "Sink flag."}},
														},
													},
												},
											},
										},
									},
								},
							},
							"shared": {
								// No Presentation set: SharedNode expands inline here.
								// The same type is forced overlay in epsilon.forced below.
								FieldMeta: editor.FieldMeta{Description: "E: SharedNode expandable here — same type forced overlay in epsilon.forced."},
								Children: map[string]*metadata.Node{
									"name":  {FieldMeta: editor.FieldMeta{Description: "Shared name."}},
									"score": {FieldMeta: editor.FieldMeta{Description: "Shared score."}},
									"tag":   {FieldMeta: editor.FieldMeta{Description: "Shared tag."}},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ── beta: E → O → O → E → E → O ─────────────────────────────────────────────

func betaNode() *metadata.Node {
	return &metadata.Node{
		FieldMeta: editor.FieldMeta{Description: "E→O→O→E→E→O: wrap(E) leads into two consecutive overlays gate_a→gate_b, then row(E)→sub(E)→end(O)."},
		Children: map[string]*metadata.Node{
			"desc": {FieldMeta: editor.FieldMeta{Description: "Block description."}},
			"wrap": {
				FieldMeta: editor.FieldMeta{Description: "E: expands inline at L1 — single expand before the overlay chain."},
				Children: map[string]*metadata.Node{
					"title": {FieldMeta: editor.FieldMeta{Description: "Wrap title (L1 scalar)."}},
					"gate_a": {
						FieldMeta: editor.FieldMeta{
							Presentation: schema.PresentationOverlay,
							Description:  "O: opens overlay L2 — first of two consecutive overlays.",
						},
						Children: map[string]*metadata.Node{
							"code": {FieldMeta: editor.FieldMeta{Description: "Gate A code (L2 scalar)."}},
							"gate_b": {
								FieldMeta: editor.FieldMeta{
									Presentation: schema.PresentationOverlay,
									Description:  "O: opens overlay L3 — second consecutive overlay.",
								},
								Children: map[string]*metadata.Node{
									"label": {FieldMeta: editor.FieldMeta{Description: "Gate B label (L3 scalar)."}},
									"row": {
										FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L3."},
										Children: map[string]*metadata.Node{
											"name": {FieldMeta: editor.FieldMeta{Description: "Row name (L3 scalar)."}},
											"sub": {
												FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L3 (second consecutive expand)."},
												Children: map[string]*metadata.Node{
													"code": {FieldMeta: editor.FieldMeta{Description: "Sub code (L3 scalar)."}},
													"end": {
														FieldMeta: editor.FieldMeta{
															Presentation: schema.PresentationOverlay,
															Description:  "O: opens overlay L4 — terminal editor.",
														},
														Children: map[string]*metadata.Node{
															"key":   {FieldMeta: editor.FieldMeta{Description: "End key."}},
															"value": {FieldMeta: editor.FieldMeta{Description: "End value."}},
															"flag":  {FieldMeta: editor.FieldMeta{Description: "End flag."}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ── gamma: O → O → E → E → O → E ────────────────────────────────────────────

func gammaNode() *metadata.Node {
	return &metadata.Node{
		FieldMeta: editor.FieldMeta{Description: "O→O→E→E→O→E: door_a(O)→door_b(O), then span_a(E)→span_b(E)→hop(O), terminal tail(E)."},
		Children: map[string]*metadata.Node{
			"desc": {FieldMeta: editor.FieldMeta{Description: "Block description."}},
			"door_a": {
				FieldMeta: editor.FieldMeta{
					Presentation: schema.PresentationOverlay,
					Description:  "O: opens overlay L2 — first of two consecutive overlays.",
				},
				Children: map[string]*metadata.Node{
					"title": {FieldMeta: editor.FieldMeta{Description: "Door A title (L2 scalar)."}},
					"door_b": {
						FieldMeta: editor.FieldMeta{
							Presentation: schema.PresentationOverlay,
							Description:  "O: opens overlay L3 — second consecutive overlay.",
						},
						Children: map[string]*metadata.Node{
							"label": {FieldMeta: editor.FieldMeta{Description: "Door B label (L3 scalar)."}},
							"span_a": {
								FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L3."},
								Children: map[string]*metadata.Node{
									"name": {FieldMeta: editor.FieldMeta{Description: "Span A name (L3 scalar)."}},
									"span_b": {
										FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L3 (second consecutive expand)."},
										Children: map[string]*metadata.Node{
											"code": {FieldMeta: editor.FieldMeta{Description: "Span B code (L3 scalar)."}},
											"hop": {
												FieldMeta: editor.FieldMeta{
													Presentation: schema.PresentationOverlay,
													Description:  "O: opens overlay L4.",
												},
												Children: map[string]*metadata.Node{
													"key": {FieldMeta: editor.FieldMeta{Description: "Hop key (L4 scalar)."}},
													"tail": {
														FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L4 — terminal expand."},
														Children: map[string]*metadata.Node{
															"x": {FieldMeta: editor.FieldMeta{Description: "Tail x."}},
															"y": {FieldMeta: editor.FieldMeta{Description: "Tail y."}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ── delta: E → E → O → O → E → O ────────────────────────────────────────────

func deltaNode() *metadata.Node {
	return &metadata.Node{
		FieldMeta: editor.FieldMeta{Description: "E→E→O→O→E→O: row_a(E)→row_b(E) lead into jump_a(O)→jump_b(O), then after(E)→final(O)."},
		Children: map[string]*metadata.Node{
			"desc": {FieldMeta: editor.FieldMeta{Description: "Block description."}},
			"row_a": {
				FieldMeta: editor.FieldMeta{Description: "E: expands inline at L1 — first of two consecutive expands."},
				Children: map[string]*metadata.Node{
					"title": {FieldMeta: editor.FieldMeta{Description: "Row A title (L1 scalar)."}},
					"row_b": {
						FieldMeta: editor.FieldMeta{Description: "E: expands inline at L1 (second consecutive expand)."},
						Children: map[string]*metadata.Node{
							"name": {FieldMeta: editor.FieldMeta{Description: "Row B name (L1 scalar)."}},
							"jump_a": {
								FieldMeta: editor.FieldMeta{
									Presentation: schema.PresentationOverlay,
									Description:  "O: opens overlay L2 — first of two consecutive overlays.",
								},
								Children: map[string]*metadata.Node{
									"info": {FieldMeta: editor.FieldMeta{Description: "Jump A info (L2 scalar)."}},
									"jump_b": {
										FieldMeta: editor.FieldMeta{
											Presentation: schema.PresentationOverlay,
											Description:  "O: opens overlay L3 — second consecutive overlay.",
										},
										Children: map[string]*metadata.Node{
											"label": {FieldMeta: editor.FieldMeta{Description: "Jump B label (L3 scalar)."}},
											"after": {
												FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L3."},
												Children: map[string]*metadata.Node{
													"code": {FieldMeta: editor.FieldMeta{Description: "After code (L3 scalar)."}},
													"final": {
														FieldMeta: editor.FieldMeta{
															Presentation: schema.PresentationOverlay,
															Description:  "O: opens overlay L4 — terminal editor.",
														},
														Children: map[string]*metadata.Node{
															"key":   {FieldMeta: editor.FieldMeta{Description: "Final key."}},
															"value": {FieldMeta: editor.FieldMeta{Description: "Final value."}},
															"flag":  {FieldMeta: editor.FieldMeta{Description: "Final flag."}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ── epsilon: O → E → E → O → O → E ──────────────────────────────────────────

func epsilonNode() *metadata.Node {
	return &metadata.Node{
		FieldMeta: editor.FieldMeta{Description: "O→E→E→O→O→E: entry(O)→stretch(E)→deep(E)→hop_a(O)→hop_b(O)→foot(E). Plus SharedNode override demo."},
		Children: map[string]*metadata.Node{
			"desc": {FieldMeta: editor.FieldMeta{Description: "Block description."}},
			"entry": {
				FieldMeta: editor.FieldMeta{
					Presentation: schema.PresentationOverlay,
					Description:  "O: opens overlay L2 — entry point of the chain.",
				},
				Children: map[string]*metadata.Node{
					"info": {FieldMeta: editor.FieldMeta{Description: "Entry info (L2 scalar)."}},
					"stretch": {
						FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L2."},
						Children: map[string]*metadata.Node{
							"name": {FieldMeta: editor.FieldMeta{Description: "Stretch name (L2 scalar)."}},
							"deep": {
								FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L2 (second consecutive expand)."},
								Children: map[string]*metadata.Node{
									"label": {FieldMeta: editor.FieldMeta{Description: "Deep label (L2 scalar)."}},
									"hop_a": {
										FieldMeta: editor.FieldMeta{
											Presentation: schema.PresentationOverlay,
											Description:  "O: opens overlay L3 — first of two consecutive overlays.",
										},
										Children: map[string]*metadata.Node{
											"code": {FieldMeta: editor.FieldMeta{Description: "Hop A code (L3 scalar)."}},
											"hop_b": {
												FieldMeta: editor.FieldMeta{
													Presentation: schema.PresentationOverlay,
													Description:  "O: opens overlay L4 — second consecutive overlay.",
												},
												Children: map[string]*metadata.Node{
													"key": {FieldMeta: editor.FieldMeta{Description: "Hop B key (L4 scalar)."}},
													"foot": {
														FieldMeta: editor.FieldMeta{Description: "E: expands inline inside L4 — terminal expand."},
														Children: map[string]*metadata.Node{
															"x": {FieldMeta: editor.FieldMeta{Description: "Foot x."}},
															"y": {FieldMeta: editor.FieldMeta{Description: "Foot y."}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"forced": {
				// PresentationOverlay forced on SharedNode: same type as alpha.portal.mid.shared,
				// but rendered as overlay here instead of expandable.
				FieldMeta: editor.FieldMeta{
					Presentation: schema.PresentationOverlay,
					Description:  "O: SharedNode FORCED overlay via Metadata — same type expands inline in alpha.portal.mid.shared.",
				},
				Children: map[string]*metadata.Node{
					"name":  {FieldMeta: editor.FieldMeta{Description: "Shared name."}},
					"score": {FieldMeta: editor.FieldMeta{Description: "Shared score."}},
					"tag":   {FieldMeta: editor.FieldMeta{Description: "Shared tag."}},
				},
			},
		},
	}
}

// ── Theme helper ──────────────────────────────────────────────────────────────

func appTheme(name string) theme.Theme {
	all := theme.All()
	if t, ok := all[name]; ok {
		return theme.Theme{Base: &t}
	}
	return theme.Theme{}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	var themeName, configPath string

	cmd := &cobra.Command{
		Use:   "depth",
		Short: "yedit depth - exercises alternating Presentation patterns at up to 6 nesting levels",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := configPath
			if path == "" {
				path = "depth.yaml"
				if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
					if err := os.WriteFile(path, []byte(seedYAML), 0600); err != nil {
						return err
					}
				}
			}

			res, err := editor.Run(editor.Config{
				Theme:       appTheme(themeName),
				Path:        path,
				Schema:      &DepthConfig{},
				Title:       "yedit depth — Presentation alternation test",
				EnableHints: true,
				Metadata:    depthMetadata,
			})
			if err != nil {
				return err
			}
			if res.Saved {
				fmt.Println("changes saved to", path)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "YAML file to edit (default: depth.yaml)")
	cmd.Flags().StringVar(&themeName, "theme", "dark", "theme preset (dracula, light, ...)")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
