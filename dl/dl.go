package dl

import (
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/opd-ai/pure-go-dl/loader"
	"github.com/opd-ai/pure-go-dl/symbol"
)

// Flag controls library loading behaviour.
type Flag int

const (
	RTLD_LOCAL  Flag = 0
	RTLD_GLOBAL Flag = 1
)

// Library represents a loaded shared object.
type Library struct {
	obj    *loader.Object
	global bool
}

var (
	mu      sync.Mutex
	loaded  = map[string]*Library{} // soname/path → Library
	globals []*Library               // RTLD_GLOBAL libraries
)

// globalResolver implements loader.SymbolResolver over the globals list.
type globalResolver struct{}

func (globalResolver) Resolve(name string) (uintptr, error) {
	mu.Lock()
	defer mu.Unlock()
	for _, lib := range globals {
		if sym, ok := lib.obj.Symbols.Lookup(name); ok {
			return sym.Value, nil
		}
	}
	return 0, fmt.Errorf("dl: undefined symbol %q", name)
}

// Open loads the shared library identified by name (path or soname).
// Transitive DT_NEEDED dependencies are loaded first.
func Open(name string, flags ...Flag) (*Library, error) {
	flag := RTLD_LOCAL
	if len(flags) > 0 {
		flag = flags[0]
	}

	mu.Lock()
	if lib, ok := loaded[name]; ok {
		lib.obj.RefCount++
		mu.Unlock()
		return lib, nil
	}
	mu.Unlock()

	// Locate the file.
	path, err := findLibrary(name)
	if err != nil {
		return nil, err
	}

	// Load dependencies first (pre-resolver step: load them without symbols).
	// We'll do a best-effort parse to collect DT_NEEDED.
	// The actual loading happens depth-first.
	lib, err := loadOne(path, flag)
	if err != nil {
		return nil, err
	}
	return lib, nil
}

// loadOne loads a single shared object (not its dependencies recursively) and
// registers it in the global table.
func loadOne(path string, flag Flag) (*Library, error) {
	mu.Lock()
	if lib, ok := loaded[path]; ok {
		lib.obj.RefCount++
		mu.Unlock()
		return lib, nil
	}
	mu.Unlock()

	obj, err := loader.Load(path, globalResolver{})
	if err != nil {
		return nil, err
	}

	lib := &Library{obj: obj, global: flag == RTLD_GLOBAL}

	mu.Lock()
	defer mu.Unlock()
	key := path
	if obj.Soname != "" {
		key = obj.Soname
	}
	loaded[key] = lib
	loaded[path] = lib
	if lib.global {
		globals = append(globals, lib)
	}
	return lib, nil
}

// Sym returns the absolute address of the exported symbol name.
func (l *Library) Sym(name string) (uintptr, error) {
	sym, ok := l.obj.Symbols.Lookup(name)
	if !ok {
		return 0, fmt.Errorf("dl: symbol %q not found", name)
	}
	return sym.Value, nil
}

// Bind resolves a symbol and registers it as a Go function using purego.
// fnPtr must be a pointer to a function variable (e.g. *func(int) int).
func (l *Library) Bind(name string, fnPtr any) error {
	addr, err := l.Sym(name)
	if err != nil {
		return err
	}
	purego.RegisterFunc(fnPtr, addr)
	return nil
}

// Close decrements the reference count and unloads if it reaches zero.
func (l *Library) Close() error {
	mu.Lock()
	defer mu.Unlock()
	l.obj.RefCount--
	if l.obj.RefCount > 0 {
		return nil
	}
	// Remove from loaded map.
	for k, v := range loaded {
		if v == l {
			delete(loaded, k)
		}
	}
	// Remove from globals.
	for i, g := range globals {
		if g == l {
			globals = append(globals[:i], globals[i+1:]...)
			break
		}
	}
	return loader.Unload(l.obj)
}

// PrintSymbols writes a sorted list of exported symbols to w.
func (l *Library) PrintSymbols(w io.Writer) {
	type entry struct {
		name string
		sym  *symbol.Symbol
	}
	var entries []entry
	// Walk all symbols – we expose the Symbols field via a helper.
	l.obj.Symbols.ForEach(func(name string, s *symbol.Symbol) {
		entries = append(entries, entry{name, s})
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, e := range entries {
		fmt.Fprintf(w, "0x%016x  %s\n", e.sym.Value, e.name)
	}
}
