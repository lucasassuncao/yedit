# yedit — Arquitetura Interna

Este documento descreve como o editor funciona do início ao fim: como os dados fluem do disco pela árvore de nós e de volta, como o editor stack habilita edição aninhada, e como o sistema de undo opera em dois níveis independentes.

---

## Camadas em resumo

```
┌─────────────────────────────────────────────────────┐
│  editor.Run (programa bubbletea)                    │
│  ┌───────────────┐  ┌──────────────────────────┐    │
│  │  model (root) │  │  blockEditState (stack)  │    │
│  │  list / alert │  │  tree · YAML pane · node │    │
│  └───────────────┘  └──────────────────────────┘    │
│           │                      │                  │
│      document.Document     *yaml.Node (editRoot)    │
│      (bytes brutos + histórico)  árvore canônica    │
└─────────────────────────────────────────────────────┘
           │
      arquivo no disco
```

Há duas "fontes de verdade" separadas que nunca se sobrepõem:

| Camada | O que ela possui | Mecanismo de undo |
|---|---|---|
| `document.Document` | Bytes YAML brutos como apareceriam no disco | `doc.Undo()` / `doc.Redo()` — snapshots em nível de bytes |
| `blockEditState.node` | `*yaml.Node` parseado enquanto um bloco está aberto para edição | `be.undoStack` / `be.redoStack` — snapshots de nó |

O documento só é mutado quando o usuário faz commit (Ctrl+S dentro de um editor) ou remove explicitamente um bloco. Entre esses momentos, a única coisa que muda é a árvore de nós em memória.

---

## document.Document

`document/document.go` possui os bytes brutos do arquivo YAML.

```
Load(path, knownOrder) → *Document
    raw []byte              ← conteúdo do arquivo (CRLF normalizado)
    blocks []Block          ← lista de blocos parseados (chave + intervalo de linhas)
    history [][]byte        ← pilha de undo (snapshots brutos)
    future  [][]byte        ← pilha de redo
    knownOrder []string     ← ordem canônica de chaves para Insert/Replace
```

**Modelo de blocos.** O documento é dividido no nível superior em blocos nomeados — um por chave YAML de topo. `ParseBlocks` divide os bytes brutos em entradas `Block{Key, StartLine, EndLine}` sem parsing YAML completo. Isso mantém todas as mutações rápidas: `Insert`, `Replace` e `Remove` manipulam linhas brutas e depois re-parseiam o índice de blocos.

**Métodos de mutação:**

| Método | O que faz |
|---|---|
| `Insert(snippet)` | Adiciona um novo bloco, posicionado por `knownOrder` |
| `Replace(key, snippet)` | Remove o bloco e insere a nova versão |
| `Remove(key)` | Remove o bloco inteiramente |

Toda mutação chama `snapshot()` primeiro (salva os bytes atuais em `history`) e define `dirty = true`. `Undo()` / `Redo()` trocam os bytes de volta por esses snapshots e re-parseiam o índice de blocos.

**Round-trip guard.** Após `Insert` e `Replace`, o documento relê o bloco armazenado com `BlockContent` e o compara com o snippet submetido usando `blockSemanticEqual` (parseia ambos como YAML e compara a estrutura dos nós). Se divergirem, a mutação é revertida. Isso captura qualquer quirk de serialização antes de chegar ao disco.

---

## Camada de schema

`schema.Discover(ptr, depth)` percorre a struct Go passada como `Config.Schema` via reflection e constrói uma árvore `[]FieldDef`:

```go
type FieldDef struct {
    YAMLName string
    Kind      Kind       // KindPrimitive | KindObject | KindList | KindDictionary
    Children  []FieldDef // campos de struct aninhados
    // ...
}
```

`Kind` determina como um block editor se comporta:

| Kind | Mapeamento Go | Modo do editor |
|---|---|---|
| `KindObject` | struct | tree + YAML pane (toggles de campos) |
| `KindList` | `[]Struct` | navegador de coleção (sequência) |
| `KindDictionary` | `map[string]Struct` | navegador de coleção (mapa) |
| `KindPrimitive` | escalar / `[]string` / mapa livre | apenas YAML pane |

`schema.KnownChildren(tree)` produz um `map[path]map[key]bool` usado no momento do commit para detectar chaves YAML desconhecidas.

---

## Root model

`editor/root.go` contém o `model` bubbletea:

```go
type model struct {
    cfg         Config
    doc         *document.Document
    schemaTree  []schema.FieldDef

    list        listModel        // lista de blocos no painel esquerdo
    blockEdits  []*blockEditState // stack de editores (não-vazio quando um bloco está aberto)
    editRoot    *yaml.Node       // nó canônico do bloco sendo editado
    editBlockKey string          // chave de topo que editRoot pertence
    alert       *alert.Model
    // ...
}
```

**Exatamente um pane está ativo por vez:** `paneList`, `panePreview`, `paneBlockEdit` ou `paneAlert`. Os quatro métodos `enter*` são os únicos que mudam `m.mode`, portanto os invariantes são mantidos por construção: `alert != nil ⟺ paneAlert`, `len(blockEdits) > 0 ⟺ paneBlockEdit`.

---

## Abrindo um bloco

Quando o usuário pressiona Enter em um item da lista, `handleOpenItem` executa:

1. Lê o conteúdo atual do bloco no documento com `doc.BlockContent(key)` (ou usa um template vazio para um novo bloco).
2. Cria um `blockEditState` com `newBlockEdit(cfg, spec, w, h)`.
3. Define `be.focus = nil` — o editor raiz endereça o bloco inteiro.
4. Empurra na pilha `m.blockEdits`.
5. Inicializa `m.editRoot` como um `MappingNode` vazio (placeholder não-nulo; o primeiro flush o popula).
6. Chama `enterBlockEdit()`.

---

## blockEditState — o editor de bloco

Cada `blockEditState` possui:

```
node        *yaml.Node       ← nó de valor canônico (única fonte de verdade deste editor)
tree        treeModel        ← árvore de checkmarks projetada a partir do node
yamlEditor  textarea.Model   ← buffer de texto (tolerante; pode estar no meio de uma edição)
coll        collectionBuffer ← índice da entrada atual (apenas coleções)
focus       []pathSeg        ← endereço deste editor dentro de editRoot
undoStack   []*blockEditUndoSnap
redoStack   []*blockEditUndoSnap
```

### be.node: a única fonte de verdade

`be.node` é o `*yaml.Node` para o mapeamento de valor do bloco (ou a raiz da sequência/mapa para coleções). É a única representação autoritativa dos dados atuais do bloco. Tudo mais é derivado dele:

- **Painel Tree** — `syncTreeCheckedFromNode(tree, node)` percorre o nó para computar quais campos estão presentes (marcados), depois aplica a seção ADDED/AVAILABLE.
- **Painel YAML** — `nodeToContent(key, node)` serializa o nó para o texto exibido (e editável) à direita.

### Parse gate tolerante

Digitar no painel YAML pode deixar o buffer transitoriamente inválido. Após cada keystroke que altera o conteúdo, `syncParsedNode` tenta parsear o buffer com `valueNodeOfSnippet`:

- **Parse bem-sucedido** → `be.node` avança para o novo nó; a árvore é re-derivada dele.
- **Parse falhou** → `be.node` mantém seu último estado válido; a árvore permanece inalterada.

Teclas de navegação (setas, seleção) não disparam o gate — não há nada a re-projetar.

### Toggles na árvore

Quando o usuário marca ou desmarca um campo no painel tree, `handleTreeToggle` executa:

1. `be.saveUndo()` — snapshot do estado atual antes da mutação.
2. `toggleNodeField(be.node, ctx, node, checked)` — adiciona ou remove o campo de `be.node` estruturalmente usando `applyToggleAt`, depois chama `pruneEmptyMappings(be.node)` para remover mapeamentos ou sequências pai que ficaram vazios.
3. `reorderNestedMappingKeys` — ordena as chaves de volta para a ordem do schema.
4. `syncTreeCheckedFromNode` — re-deriva a árvore a partir do `be.node` atualizado.
5. `nodeToContent(key, be.node)` → `yamlEditor.SetValue(...)` — re-renderiza o painel YAML.

O painel tree e o painel YAML estão sempre consistentes porque ambos são derivados de `be.node` após cada mutação.

---

## Navegação de coleção

Para blocos `KindList` e `KindDictionary` com filhos definidos no schema, o editor se torna um **navegador de coleção**. `be.node` é o nó de sequência ou mapa inteiro (não apenas o valor de uma entrada). Um `collectionBuffer` rastreia qual entrada está sendo exibida:

```
be.node          ← nó de sequência/mapa — possui TODAS as entradas
be.coll.current  ← índice da entrada exibida no yamlEditor
```

A navegação entre entradas é um ciclo flush-load de dois passos:

1. `flushCurrentEntry()` — parseia o texto do editor YAML e o escreve de volta em `be.node` na posição da entrada atual (parse gate: rejeita YAML inválido e bloqueia a navegação).
2. `loadEntry(idx)` — lê `be.node[idx]` e define `yamlEditor` com sua forma serializada.

`collectionDeriveTree()` re-projeta labels e checkmarks de campos para **todas** as entradas a partir de `be.node` após qualquer mudança estrutural (adicionar, deletar, reordenar).

---

## editRoot e o editor stack

`model.editRoot` é o único `*yaml.Node` canônico para o bloco de nível superior sendo editado. Começa como um mapeamento vazio e é populado pela primeira chamada a `flushTopToRoot`.

**Por que uma árvore compartilhada em vez de um nó por editor?**  
Splicing de strings entre editores empilhados corromperia dados aninhados (por exemplo, se o texto de um campo externo coincidisse com um limite de bloco interno). Usando uma árvore de nós compartilhada, o caminho `focus` de cada editor endereça o mesmo objeto vivo, portanto escritas em qualquer profundidade nunca podem corromper caminhos não relacionados.

### flushTopToRoot

Serializa o editor ativo (topo) e escreve seu resultado de volta em `editRoot` em `be.focus`:

```
top.commit() → snippet (texto YAML)
valueNodeOfSnippet(snippet) → val (*yaml.Node)
setNodeAt(editRoot, top.focus, val)
```

`setNodeAt` com `segs = nil` (editor raiz, `focus = nil`) substitui o próprio `editRoot` por `val`.

### Drill-in (Enter em um campo openable)

1. Flush do editor topo atual em `editRoot` (`flushTopToRoot`).
2. Computa `childFocus = parentFocus + relSegs`.
3. Lê `nodeAt(editRoot, childFocus)` — o conteúdo atual do filho a partir da árvore viva.
4. Cria um `blockEditState` para o campo filho com `focus = childFocus`.
5. Empurra em `blockEdits`.

### Drill-out (Esc em um editor aninhado)

1. Registra `childWasDirty = top.dirty`.
2. Flush do filho em `editRoot` (`flushTopToRoot`).
3. Remove o filho de `blockEdits`.
4. Se `childWasDirty`, chama `saveUndo()` no novo topo (editor pai) — isso permite que Ctrl+U no pai desfaça o drill-in inteiro em um único passo.
5. `refreshTopFromRoot(childWasDirty)` — relê o caminho focus do pai a partir de `editRoot` e reconstrói seu painel YAML e árvore a partir do nó atualizado.

Nenhum dado é escrito no documento durante o drill-out. As mudanças se acumulam em `editRoot` até o Ctrl+S.

---

## commitAll — escrevendo de volta no documento

Ctrl+S dentro de qualquer editor de bloco chama `saveAll`, que executa os validators primeiro e depois `commitAll`:

```
commitAll():
    isEdit = blockEdits[0].isEdit   ← true = Replace, false = Insert

    flushTopToRoot()                ← escreve o editor ativo em editRoot

    pruneEmptyMappings(editRoot)    ← remove mapeamentos/sequências vazios deixados por toggles
                                       e itens de coleção vazios deixados por drill-out

    blockIsEmpty = editRoot é um MappingNode vazio

    switch:
    case blockIsEmpty && isEdit:
        doc.Remove(editBlockKey)    ← todos os campos removidos → deleta a chave inteira

    case !blockIsEmpty:
        final = nodeToContent(editBlockKey, editRoot)   ← serializa uma vez
        se isEdit:
            current = doc.BlockContent(editBlockKey)
            se normalizeBlockContent(current) != final: ← pula se conteúdo não mudou
                doc.Replace(editBlockKey, final)
        senão:
            doc.Insert(final)

    syncView(); enterList()
```

`normalizeBlockContent` parseia o conteúdo de bloco existente do documento e o re-serializa via `nodeToContent` para que ambos os lados da comparação passem pelo mesmo pipeline de formatação (block style, 2 espaços de indentação). Isso impede que um commit sem mudanças (por exemplo, um item de coleção vazio que foi podado) crie um snapshot de histórico ou marque o documento como dirty.

---

## Undo/redo — dois níveis independentes

### Dentro de um editor de bloco (Ctrl+U / Ctrl+Y)

Toda operação mutante chama `be.saveUndo()` antes de alterar o estado. `saveUndo` chama `captureSnap()`:

```go
type blockEditUndoSnap struct {
    node       *yaml.Node  // clone profundo de be.node
    yamlValue  string      // buffer de texto no momento do snapshot
    dirty      bool
    preset     string
    treeNodes  []treeNode  // estado da árvore (cursor, expansão)
    // ...
}
```

`restoreUndo()` clona o nó do snapshot de volta em `be.node`, restaura o buffer YAML e a árvore, e empurra o estado desfeito na `redoStack`. `restoreRedo()` é a operação simétrica.

Ctrl+U/Ctrl+Y enquanto um editor de bloco está aberto **nunca** cai para `doc.Undo()`/`doc.Redo()`. Os dois níveis de undo são completamente separados.

### No nível do documento (visão de lista, Ctrl+U / Ctrl+Y)

`doc.Undo()` e `doc.Redo()` trocam snapshots de bytes brutos. Esses cobrem operações `Insert`, `Replace` e `Remove` — ou seja, cada commit Ctrl+S, cada deleção de bloco e cada aplicação de preset.

---

## pruneEmptyMappings

Chamada após cada toggle na árvore e novamente em `commitAll`. Remove:

- Chaves de mapeamento cujo valor é um mapeamento ou sequência vazios (de baixo para cima, para que nós intermediários sejam limpos após seus filhos).
- Itens de mapeamento vazios (`{}`) de sequências — isso trata o caso onde um drill-in é aberto para um novo item de coleção e o usuário faz commit sem adicionar nenhum campo.

---

## Validators

Duas famílias, ambas executadas no momento de salvar via `RunAll(cfg.Validators, raw, blocks)`:

**Família FromMetadata** (`RequiredFromMetadata`, `OneOfFromMetadata`, etc.) — conectada na inicialização em `newModel` com a árvore de schema descoberta e `cfg.Metadata`. Leem `FieldMeta` do `MetadataSource` para cada campo e aplicam a restrição declarada contra o YAML bruto.

**Família explícita** (`Required`, `ValueOneOf`, `ValueInRange`, etc.) — auto-contidas; funcionam apenas com os bytes brutos e a lista de blocos. Usadas para regras pontuais ou cross-field.

---

## MetadataSource e o painel de hints

`Config.Metadata` é uma `MetadataSource`:

```go
type MetadataSource interface {
    FieldMeta(blockKey, fieldPath string) FieldMeta
}
```

`FieldMeta` carrega dados de exibição (Description, Type, Default, OneOf, Example, …) e dados de restrição (Required, Min, Max, Pattern, MinCount, MaxCount, Unique, Deprecated). É a única fonte de verdade tanto para o painel Hint/Example quanto para os validators `FromMetadata` — as restrições são declaradas uma vez e reutilizadas nos dois lugares.

O painel de hints é opt-in via `Config.EnableHints`. Quando habilitado, a coluna direita na visão de lista se divide: Preview em cima, Hint/Example abaixo. O painel renderiza o `FieldMeta` para o bloco selecionado via `selectedHint()`, e `blockEditState.fieldHintFor(path)` faz o mesmo para campos individuais dentro de um editor aberto.
