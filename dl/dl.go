package dl

import (
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/ebitengine/purego"
	goelf "github.com/opd-ai/pure-go-dl/elf"
	"github.com/opd-ai/pure-go-dl/loader"
	"github.com/opd-ai/pure-go-dl/symbol"
)

// Flag controls library loading behaviour.
type Flag int

const (
	RTLD_LOCAL  Flag = 0 // Symbols are only visible to this library and its dependents (default)
	RTLD_GLOBAL Flag = 1 // Symbols are visible to all subsequently loaded libraries
	RTLD_NOW    Flag = 2 // Immediate symbol binding (always enabled; provided for compatibility)
)

// RTLD_LAZY (deferred symbol binding) is explicitly not supported.
// All libraries are loaded with eager binding semantics (RTLD_NOW behavior).

// Library represents a loaded shared object.
type Library struct {
	obj    *loader.Object
	global bool
}

var (
	mu      sync.Mutex
	cond    *sync.Cond
	loaded  = map[string]*Library{} // soname/path → Library
	loading = map[string]bool{}     // paths currently being loaded (by any goroutine)
	globals []*Library              // RTLD_GLOBAL libraries
)

func init() {
	cond = sync.NewCond(&mu)
}

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
// Transitive DT_NEEDED dependencies are loaded depth-first before the
// requested library, so their symbols are available during relocation.
//
// Flags control symbol visibility:
//   - RTLD_LOCAL (default): symbols visible only to this library and dependents
//   - RTLD_GLOBAL: symbols visible to all subsequently loaded libraries
//   - RTLD_NOW: immediate binding (compatibility flag; all libraries use eager binding)
//
// Note: RTLD_LAZY is not supported; all symbol binding is eager.
func Open(name string, flags ...Flag) (*Library, error) {
	flag := RTLD_LOCAL
	if len(flags) > 0 {
		flag = flags[0]
		// RTLD_NOW is a no-op compatibility flag (eager binding is always enabled)
		if flag == RTLD_NOW {
			flag = RTLD_LOCAL
		}
	}
	// visiting tracks paths on the current call stack to detect cycles.
	return loadLib(name, flag, make(map[string]bool))
}

// loadLib resolves a library by name (soname or path) and loads it.
func loadLib(name string, flag Flag, visiting map[string]bool) (*Library, error) {
	mu.Lock()
	if lib, ok := loaded[name]; ok {
		lib.obj.RefCount++
		mu.Unlock()
		return lib, nil
	}
	mu.Unlock()

	// Initial load has no parent RUNPATH/RPATH
	path, err := findLibrary(name, "", "")
	if err != nil {
		return nil, err
	}
	return loadPath(path, name, flag, visiting)
}

// loadPath loads a library by its resolved absolute file path.
// visiting is per-call-stack and used for cycle detection.
func loadPath(path, soname string, flag Flag, visiting map[string]bool) (*Library, error) {
	// Cycle detection: this path is already on the current goroutine's call stack.
	if visiting[path] {
		mu.Lock()
		lib := loaded[path]
		if lib != nil {
			lib.obj.RefCount++
		}
		mu.Unlock()
		return lib, nil
	}

	mu.Lock()
	// If another goroutine is loading this path, wait for it to finish.
	for loading[path] {
		cond.Wait()
	}
	// Check if it was loaded while we were waiting.
	if lib, ok := loaded[path]; ok {
		lib.obj.RefCount++
		mu.Unlock()
		return lib, nil
	}
	// Claim the slot so no other goroutine starts loading this path.
	loading[path] = true
	mu.Unlock()

	// Mark on this call stack to detect cycles in the dependency graph.
	visiting[path] = true

	// Parse the ELF file to read DT_NEEDED before fully loading it.
	// This lets us load transitive dependencies depth-first.
	parsed, err := goelf.Parse(path)
	if err != nil {
		mu.Lock()
		delete(loading, path)
		cond.Broadcast()
		mu.Unlock()
		delete(visiting, path)
		return nil, err
	}

	// Load transitive dependencies depth-first.
	// They are loaded as RTLD_GLOBAL so their symbols are visible during
	// the relocation phase of the dependent library.
	for _, dep := range parsed.Needed {
		if visiting[dep] {
			continue // already on this call stack → cycle, skip
		}
		// Use parent's RUNPATH/RPATH to resolve dependencies
		depPath, findErr := findLibrary(dep, parsed.Runpath, parsed.Rpath)
		if findErr != nil {
			continue // non-fatal: system library may not be present
		}
		if _, loadErr := loadPath(depPath, dep, RTLD_GLOBAL, visiting); loadErr != nil {
			_ = loadErr // non-fatal: continue with missing dependency
		}
	}

	delete(visiting, path)

	// Now load the library itself (maps segments, applies relocations,
	// runs constructors).
	obj, err := loader.Load(path, globalResolver{})

	mu.Lock()
	delete(loading, path)
	cond.Broadcast()
	if err != nil {
		mu.Unlock()
		return nil, err
	}

	lib := &Library{obj: obj, global: flag == RTLD_GLOBAL}
	if obj.Soname != "" && obj.Soname != path {
		loaded[obj.Soname] = lib
	}
	if soname != "" && soname != path && soname != obj.Soname {
		loaded[soname] = lib
	}
	loaded[path] = lib
	if lib.global {
		globals = append(globals, lib)
	}
	mu.Unlock()

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
	l.obj.Symbols.ForEach(func(name string, s *symbol.Symbol) {
		entries = append(entries, entry{name, s})
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, e := range entries {
		fmt.Fprintf(w, "0x%016x  %s\n", e.sym.Value, e.name)
	}
}
