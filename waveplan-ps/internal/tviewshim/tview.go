package tview

const (
	FlexColumn = iota
	FlexRow
)

// Primitive is the shared marker accepted by container and application APIs.
type Primitive interface{}

// Application is a small compatibility surface for non-interactive tests.
type Application struct {
	root Primitive
}

func NewApplication() *Application {
	return &Application{}
}

func (a *Application) SetRoot(root Primitive, fullscreen bool) *Application {
	a.root = root
	return a
}

func (a *Application) Run() error {
	return nil
}

func (a *Application) Stop() {
}

func (a *Application) QueueUpdateDraw(fn func()) *Application {
	if fn != nil {
		fn()
	}
	return a
}

// Flex lays out child primitives.
type Flex struct {
	direction int
	items     []flexItem
}

type flexItem struct {
	primitive  Primitive
	fixed      int
	proportion int
	focus      bool
}

func NewFlex() *Flex {
	return &Flex{}
}

func (f *Flex) SetDirection(direction int) *Flex {
	f.direction = direction
	return f
}

func (f *Flex) AddItem(primitive Primitive, fixedSize, proportion int, focus bool) *Flex {
	f.items = append(f.items, flexItem{
		primitive:  primitive,
		fixed:      fixedSize,
		proportion: proportion,
		focus:      focus,
	})
	return f
}

// Table stores cells in row/column coordinates.
type Table struct {
	borders    bool
	selectable bool
	cells      map[int]map[int]*TableCell
}

func NewTable() *Table {
	return &Table{cells: map[int]map[int]*TableCell{}}
}

func (t *Table) SetBorders(show bool) *Table {
	t.borders = show
	return t
}

func (t *Table) SetSelectable(rows, columns bool) *Table {
	t.selectable = rows || columns
	return t
}

func (t *Table) SetCell(row, column int, cell *TableCell) *Table {
	if t.cells[row] == nil {
		t.cells[row] = map[int]*TableCell{}
	}
	t.cells[row][column] = cell
	return t
}

func (t *Table) GetCell(row, column int) *TableCell {
	if t.cells[row] == nil {
		return nil
	}
	return t.cells[row][column]
}

// TableCell stores displayed table text and layout weight.
type TableCell struct {
	Text      string
	expansion int
}

func NewTableCell(text string) *TableCell {
	return &TableCell{Text: text}
}

func (c *TableCell) SetExpansion(expansion int) *TableCell {
	c.expansion = expansion
	return c
}

// TextView stores read-only rendered text.
type TextView struct {
	text string
}

func NewTextView() *TextView {
	return &TextView{}
}

func (t *TextView) SetDynamicColors(enabled bool) *TextView {
	return t
}

func (t *TextView) SetWrap(wrap bool) *TextView {
	return t
}

func (t *TextView) SetText(text string) *TextView {
	t.text = text
	return t
}

func (t *TextView) GetText(stripAll bool) string {
	return t.text
}

// Pages stores named primitives and tracks the active page.
type Pages struct {
	pages  map[string]Primitive
	active string
}

func NewPages() *Pages {
	return &Pages{pages: map[string]Primitive{}}
}

func (p *Pages) AddAndSwitchToPage(name string, item Primitive, resize, visible bool) *Pages {
	p.pages[name] = item
	if visible {
		p.active = name
	}
	return p
}

func (p *Pages) RemovePage(name string) *Pages {
	delete(p.pages, name)
	if p.active == name {
		p.active = ""
	}
	return p
}
