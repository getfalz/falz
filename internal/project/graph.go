package project

import (
	"fmt"
	"sync"
)

type Node interface {
	ID() string
	Kind() NodeKind
}

type NodeKind string

const (
	NodeKindEntry    NodeKind = "entry"
	NodeKindLayout   NodeKind = "layout"
	NodeKindFragment NodeKind = "fragment"
	NodeKindTemplate NodeKind = "template"
	NodeKindPipeline NodeKind = "pipeline"
)

type SourceType int

const (
	SourceTypeUnknown SourceType = iota
	SourceTypeSource
	SourceTypeScheme
	SourceTypeOutput
)

type EntryType int

const (
	ContentType EntryType = iota
	AssetType
)

func (t EntryType) String() string {
	switch t {
	case ContentType:
		return "content"
	case AssetType:
		return "asset"
	default:
		return "unknown"
	}
}

type BuildEntry struct {
	ID string // Unique identifier: "content/about.md" or "assets/style.css"
	// TODO simplify fields: ID, type. source from type
	Type       EntryType  // "content" or "asset"
	SourceRoot SourceType // Source root: "source" or "scheme"
	SourcePath string     // Source file path

	// Build information
	LayoutKey LayoutKey
	Pipeline  string // Pipeline name
	TagName   string // Content slot name (empty for assets)
	BaseDir   string // Base directory
	ContentVars map[string]string

	// State
	Hash  string
	Dirty bool  // Needs rebuilding
	Error error // Build error if any
}

type DependencyGraph interface {
	AddNode(node Node)
	RemoveNode(node Node)
	GetNode(id string) (Node, bool)
	AllNodes() []Node

	AllEntries() []*BuildEntry
	EntryByID(id string) (*BuildEntry, bool)
	DirtyEntries() []*BuildEntry
	CleanEntries() []*BuildEntry
	UpdateEntry(id string, fn func(*BuildEntry) error) error
	MarkDirty(id string) error
	MarkClean(id string) error
	MarkAllDirty() error
	ClearDirty() error

	AddEdge(dependent Node, dependency Node)
	RemoveEdgesFrom(node Node)
	RemoveEdgesTo(node Node)

	DependentsOf(node Node) []Node
	DependenciesOf(node Node) []Node
	AffectedBy(node Node) []Node

	Clear()
}

type EntryNode struct {
	entry *BuildEntry
}

func NewEntryNode(entry *BuildEntry) *EntryNode {
	if entry == nil {
		panic("entry cannot be nil")
	}
	return &EntryNode{entry: entry}
}

func (n *EntryNode) ID() string         { return n.entry.ID }
func (n *EntryNode) Kind() NodeKind     { return NodeKindEntry }
func (n *EntryNode) Entry() *BuildEntry { return n.entry }

type LayoutNode struct {
	key LayoutKey
}

func NewLayoutNode(key LayoutKey) *LayoutNode {
	return &LayoutNode{key: key}
}

func (n *LayoutNode) ID() string {
    return fmt.Sprintf("layout:%s", n.key.LayoutPath)
}
func (n *LayoutNode) Kind() NodeKind { return NodeKindLayout }
func (n *LayoutNode) Key() LayoutKey { return n.key }

type FragmentNode struct {
	path string
}

func NewFragmentNode(path string) *FragmentNode {
	if path == "" {
		panic("fragment path cannot be empty")
	}
	return &FragmentNode{path: path}
}

func (n *FragmentNode) ID() string     { return fmt.Sprintf("fragment:%s", n.path) }
func (n *FragmentNode) Kind() NodeKind { return NodeKindFragment }
func (n *FragmentNode) Path() string   { return n.path }

type TemplateNode struct {
	path string
}

func NewTemplateNode(path string) *TemplateNode {
	if path == "" {
		panic("template path cannot be empty")
	}
	return &TemplateNode{path: path}
}

func (n *TemplateNode) ID() string     { return fmt.Sprintf("template:%s", n.path) }
func (n *TemplateNode) Kind() NodeKind { return NodeKindTemplate }
func (n *TemplateNode) Path() string   { return n.path }

type PipelineNode struct {
	name string
}

func NewPipelineNode(name string) *PipelineNode {
	if name == "" {
		panic("pipeline name cannot be empty")
	}
	return &PipelineNode{name: name}
}

func (n *PipelineNode) ID() string     { return fmt.Sprintf("pipeline:%s", n.name) }
func (n *PipelineNode) Kind() NodeKind { return NodeKindPipeline }
func (n *PipelineNode) Name() string   { return n.name }

type dependencyGraph struct {
	mu      sync.RWMutex
	nodes   map[string]Node
	edges   map[string]map[string]struct{}
	reverse map[string]map[string]struct{}
}

func NewDependencyGraph() DependencyGraph {
	return &dependencyGraph{
		nodes:   make(map[string]Node),
		edges:   make(map[string]map[string]struct{}),
		reverse: make(map[string]map[string]struct{}),
	}
}

// AddNode adds a node to the graph.
// If a node with the same ID already exists, it is replaced.
func (g *dependencyGraph) AddNode(node Node) {
	if node == nil {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	id := node.ID()
	g.nodes[id] = node

	if g.edges[id] == nil {
		g.edges[id] = make(map[string]struct{})
	}
	if g.reverse[id] == nil {
		g.reverse[id] = make(map[string]struct{})
	}
}

// RemoveNode removes a node and all its edges from the graph.
func (g *dependencyGraph) RemoveNode(node Node) {
	if node == nil {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	id := node.ID()

	delete(g.edges, id)

	for otherID := range g.reverse[id] {
		if deps := g.edges[otherID]; deps != nil {
			delete(deps, id)
		}
	}
	delete(g.reverse, id)

	for depID := range g.edges[id] {
		if revDeps := g.reverse[depID]; revDeps != nil {
			delete(revDeps, id)
		}
	}

	delete(g.nodes, id)
}

func (g *dependencyGraph) GetNode(id string) (Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, ok := g.nodes[id]
	return node, ok
}

// AllNodes returns all nodes in the graph.
// The returned slice is a snapshot and can be safely modified by the caller.
func (g *dependencyGraph) AllNodes() []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]Node, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// AllEntries returns all entry nodes in the graph.
// The returned slice contains pointers to the actual BuildEntry objects.
func (g *dependencyGraph) AllEntries() []*BuildEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var entries []*BuildEntry
	for _, node := range g.nodes {
		if node.Kind() == NodeKindEntry {
			if entryNode, ok := node.(*EntryNode); ok {
				entries = append(entries, entryNode.entry)
			}
		}
	}
	return entries
}

// EntryByID retrieves an entry by its ID.
func (g *dependencyGraph) EntryByID(id string) (*BuildEntry, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, ok := g.nodes[id]
	if !ok {
		return nil, false
	}

	if entryNode, ok := node.(*EntryNode); ok {
		return entryNode.entry, true
	}

	return nil, false
}

// DirtyEntries returns all entries marked as dirty.
func (g *dependencyGraph) DirtyEntries() []*BuildEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var dirty []*BuildEntry
	for _, node := range g.nodes {
		if node.Kind() == NodeKindEntry {
			if entryNode, ok := node.(*EntryNode); ok {
				if entryNode.entry.Dirty {
					dirty = append(dirty, entryNode.entry)
				}
			}
		}
	}
	return dirty
}

// CleanEntries returns all entries not marked as dirty.
func (g *dependencyGraph) CleanEntries() []*BuildEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var clean []*BuildEntry
	for _, node := range g.nodes {
		if node.Kind() == NodeKindEntry {
			if entryNode, ok := node.(*EntryNode); ok {
				if !entryNode.entry.Dirty {
					clean = append(clean, entryNode.entry)
				}
			}
		}
	}
	return clean
}

// UpdateEntry atomically updates an entry using the provided function.
// The function receives a pointer to the BuildEntry and can modify it.
// If the function returns an error, the update is aborted and the error is returned.
func (g *dependencyGraph) UpdateEntry(id string, fn func(*BuildEntry) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	node, ok := g.nodes[id]
	if !ok {
		return fmt.Errorf("entry not found: %s", id)
	}

	entryNode, ok := node.(*EntryNode)
	if !ok {
		return fmt.Errorf("node %s is not an entry (kind: %s)", id, node.Kind())
	}

	return fn(entryNode.entry)
}

// MarkDirty marks an entry as dirty.
func (g *dependencyGraph) MarkDirty(id string) error {
	return g.UpdateEntry(id, func(e *BuildEntry) error {
		e.Dirty = true
		return nil
	})
}

// MarkClean marks an entry as clean.
func (g *dependencyGraph) MarkClean(id string) error {
	return g.UpdateEntry(id, func(e *BuildEntry) error {
		e.Dirty = false
		return nil
	})
}

// MarkAllDirty marks all entries as dirty.
func (g *dependencyGraph) MarkAllDirty() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, node := range g.nodes {
		if node.Kind() == NodeKindEntry {
			if entryNode, ok := node.(*EntryNode); ok {
				entryNode.entry.Dirty = true
			}
		}
	}
	return nil
}

// ClearDirty marks all entries as clean.
func (g *dependencyGraph) ClearDirty() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, node := range g.nodes {
		if node.Kind() == NodeKindEntry {
			if entryNode, ok := node.(*EntryNode); ok {
				entryNode.entry.Dirty = false
			}
		}
	}
	return nil
}

// AddEdge creates a dependency relationship.
// If dependent depends on dependency, then changes to dependency will affect dependent.
func (g *dependencyGraph) AddEdge(dependent Node, dependency Node) {
	if dependent == nil || dependency == nil {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	depID := dependent.ID()
	depsID := dependency.ID()

	// Ensure edge maps exist
	if g.edges[depID] == nil {
		g.edges[depID] = make(map[string]struct{})
	}
	if g.reverse[depsID] == nil {
		g.reverse[depsID] = make(map[string]struct{})
	}

	// Add edge
	g.edges[depID][depsID] = struct{}{}
	g.reverse[depsID][depID] = struct{}{}
}

// RemoveEdgesFrom removes all outgoing edges from a node.
// The node remains in the graph, but no longer depends on anything.
func (g *dependencyGraph) RemoveEdgesFrom(node Node) {
	if node == nil {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	id := node.ID()

	// Remove from reverse index
	if deps := g.edges[id]; deps != nil {
		for depID := range deps {
			if revDeps := g.reverse[depID]; revDeps != nil {
				delete(revDeps, id)
			}
		}
		g.edges[id] = make(map[string]struct{})
	}
}

// RemoveEdgesTo removes all incoming edges to a node.
// The node remains in the graph, but nothing depends on it anymore.
func (g *dependencyGraph) RemoveEdgesTo(node Node) {
	if node == nil {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	id := node.ID()

	// Remove from forward index
	if revDeps := g.reverse[id]; revDeps != nil {
		for depID := range revDeps {
			if deps := g.edges[depID]; deps != nil {
				delete(deps, id)
			}
		}
		g.reverse[id] = make(map[string]struct{})
	}
}

// DependentsOf returns all nodes that directly depend on the given node.
func (g *dependencyGraph) DependentsOf(node Node) []Node {
	if node == nil {
		return nil
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	id := node.ID()
	dependents := make([]Node, 0, len(g.reverse[id]))
	for depID := range g.reverse[id] {
		if n, ok := g.nodes[depID]; ok {
			dependents = append(dependents, n)
		}
	}
	return dependents
}

// DependenciesOf returns all nodes that the given node directly depends on.
func (g *dependencyGraph) DependenciesOf(node Node) []Node {
	if node == nil {
		return nil
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	id := node.ID()
	dependencies := make([]Node, 0, len(g.edges[id]))
	for depID := range g.edges[id] {
		if n, ok := g.nodes[depID]; ok {
			dependencies = append(dependencies, n)
		}
	}
	return dependencies
}

// AffectedBy returns all nodes transitively affected by a change to the given node.
// It performs a breadth-first traversal of the reverse dependency graph.
//
// Example: If A depends on B, and B depends on C, then AffectedBy(C) returns [B, A].
func (g *dependencyGraph) AffectedBy(node Node) []Node {
	if node == nil {
		return nil
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	id := node.ID()
	visited := make(map[string]struct{})
	var result []Node

	// BFS traversal
	queue := []string{id}
	visited[id] = struct{}{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Add all nodes that depend on current
		for dependentID := range g.reverse[current] {
			if _, seen := visited[dependentID]; !seen {
				visited[dependentID] = struct{}{}
				if n, ok := g.nodes[dependentID]; ok {
					result = append(result, n)
				}
				queue = append(queue, dependentID)
			}
		}
	}

	return result
}

// Clear removes all nodes and edges from the graph.
func (g *dependencyGraph) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nodes = make(map[string]Node)
	g.edges = make(map[string]map[string]struct{})
	g.reverse = make(map[string]map[string]struct{})
}
