package main

const seedYAML = `alpha:
  desc: "O→E→O→E→E→O"
  portal:
    info: "L2 via overlay"
    mid:
      title: "mid expands inline"
      bridge:
        label: "L3 via overlay"
        row:
          name: "row expands inline"
          sub:
            code: "sub expands inline"
            sink:
              key: "L4 terminal key"
              value: "L4 terminal value"
              flag: true
      shared:
        name: "shared-expandable"
        score: 10
        tag: "alpha-tag"

beta:
  desc: "E→O→O→E→E→O"
  wrap:
    title: "wrap expands inline"
    gate_a:
      code: "L2 via overlay"
      gate_b:
        label: "L3 via overlay"
        row:
          name: "row expands inline"
          sub:
            code: "sub expands inline"
            end:
              key: "L4 terminal key"
              value: "L4 terminal value"
              flag: false

gamma:
  desc: "O→O→E→E→O→E"
  door_a:
    title: "L2 via overlay"
    door_b:
      label: "L3 via overlay"
      span_a:
        name: "span-a expands inline"
        span_b:
          code: "span-b expands inline"
          hop:
            key: "L4 via overlay"
            tail:
              x: "tail expands inline"
              y: 7

delta:
  desc: "E→E→O→O→E→O"
  row_a:
    title: "row-a expands inline"
    row_b:
      name: "row-b expands inline"
      jump_a:
        info: "L2 via overlay"
        jump_b:
          label: "L3 via overlay"
          after:
            code: "after expands inline"
            final:
              key: "L4 terminal key"
              value: "L4 terminal value"
              flag: true

epsilon:
  desc: "O→E→E→O→O→E + SharedNode override"
  entry:
    info: "L2 via overlay"
    stretch:
      name: "stretch expands inline"
      deep:
        label: "deep expands inline"
        hop_a:
          code: "L3 via overlay"
          hop_b:
            key: "L4 via overlay"
            foot:
              x: "foot expands inline"
              y: 3
  forced:
    name: "shared-forced-overlay"
    score: 99
    tag: "epsilon-tag"
`
