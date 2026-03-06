package dl

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"unsafe"
)

const (
	// ld.so.cache magic strings for different formats
	cacheHeaderMagic    = "glibc-ld.so.cache1.1"
	cacheHeaderMagicLen = 20

	// Default cache file location
	defaultCachePath = "/etc/ld.so.cache"

	// Flags from the cache file (see glibc elf/cache.c)
	flagTypeMask     = 0x00ff
	flagLibc6        = 0x0003 // ELF, libc6
	flagArchMask     = 0xff00
	flagX8664        = 0x0300
	flagRequiredMask = flagTypeMask
)

// ldCache represents a parsed ld.so.cache file.
type ldCache struct {
	entries map[string]string // soname -> absolute path
}

var (
	// Global cache singleton
	globalCache     *ldCache
	globalCacheLock sync.Mutex
	cacheLoadOnce   sync.Once
)

// cacheHeader is the header of the new format ld.so.cache file.
// Corresponds to struct cache_file_new in glibc elf/cache.c.
type cacheHeader struct {
	Magic      [20]byte // "glibc-ld.so.cache1.1"
	NumLibs    uint32   // Number of entries
	StringsLen uint32   // Total size of string table
	_          [5]uint32
	_          uint64
	_          uint32
	Extension  uint64 // Offset to extension area (unused here)
	_          [3]uint32
}

// cacheEntry is a single library entry in the cache.
// Corresponds to struct file_entry_new in glibc elf/cache.c.
type cacheEntry struct {
	Flags       uint32 // Architecture and type flags
	KeyOffset   uint32 // Offset into string table for soname
	ValueOffset uint32 // Offset into string table for path
	OSVersion   uint32
	HWCap       uint64
}

// parseCache reads and parses /etc/ld.so.cache.
func parseCache(path string) (*ldCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := validateCacheData(data); err != nil {
		return nil, err
	}

	header, err := parseCacheHeader(data)
	if err != nil {
		return nil, err
	}

	if header.NumLibs == 0 {
		return &ldCache{entries: make(map[string]string)}, nil
	}

	return parseCacheEntries(data, header)
}

// validateCacheData checks file size and magic header.
func validateCacheData(data []byte) error {
	if len(data) < int(unsafe.Sizeof(cacheHeader{})) {
		return fmt.Errorf("ld.so.cache: file too small")
	}
	if !bytes.HasPrefix(data, []byte(cacheHeaderMagic)) {
		return fmt.Errorf("ld.so.cache: invalid magic (expected %q)", cacheHeaderMagic)
	}
	return nil
}

// parseCacheHeader reads and validates the cache file header.
func parseCacheHeader(data []byte) (*cacheHeader, error) {
	var header cacheHeader
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("ld.so.cache: failed to read header: %w", err)
	}
	return &header, nil
}

// parseCacheEntries reads all cache entries and builds the library map.
func parseCacheEntries(data []byte, header *cacheHeader) (*ldCache, error) {
	headerSize := int(unsafe.Sizeof(cacheHeader{}))
	entriesSize := int(header.NumLibs) * int(unsafe.Sizeof(cacheEntry{}))
	stringsOffset := headerSize + entriesSize

	if stringsOffset > len(data) {
		return nil, fmt.Errorf("ld.so.cache: truncated file (expected strings at offset %d, file size %d)", stringsOffset, len(data))
	}

	stringTable := data[stringsOffset:]
	cache := &ldCache{
		entries: make(map[string]string, int(header.NumLibs)),
	}

	entryData := data[headerSize:stringsOffset]
	reader := bytes.NewReader(entryData)

	for i := uint32(0); i < header.NumLibs; i++ {
		var entry cacheEntry
		if err := binary.Read(reader, binary.LittleEndian, &entry); err != nil {
			return nil, fmt.Errorf("ld.so.cache: failed to read entry %d: %w", i, err)
		}

		if !shouldIncludeEntry(&entry) {
			continue
		}

		soname, err := extractString(stringTable, entry.KeyOffset)
		if err != nil {
			continue
		}

		path, err := extractString(stringTable, entry.ValueOffset)
		if err != nil {
			continue
		}

		cache.entries[soname] = path
	}

	return cache, nil
}

// shouldIncludeEntry filters cache entries by architecture and library type.
func shouldIncludeEntry(entry *cacheEntry) bool {
	if (entry.Flags & flagRequiredMask) != flagLibc6 {
		return false
	}
	arch := entry.Flags & flagArchMask
	return arch == 0 || arch == flagX8664
}

// extractString extracts a null-terminated string from data at the given offset.
func extractString(data []byte, offset uint32) (string, error) {
	if int(offset) >= len(data) {
		return "", fmt.Errorf("offset %d out of bounds (data size %d)", offset, len(data))
	}

	end := bytes.IndexByte(data[offset:], 0)
	if end == -1 {
		return "", fmt.Errorf("no null terminator found at offset %d", offset)
	}

	return string(data[offset : int(offset)+end]), nil
}

// lookupInCache searches the global ld.so.cache for the given soname.
// Returns the absolute path if found, empty string otherwise.
func lookupInCache(soname string) string {
	// Lazy-load the cache on first use
	cacheLoadOnce.Do(func() {
		globalCacheLock.Lock()
		defer globalCacheLock.Unlock()

		cache, err := parseCache(defaultCachePath)
		if err != nil {
			// Cache parse failure is not fatal; we just won't use it.
			// This allows the loader to work on systems without ld.so.cache.
			return
		}
		globalCache = cache
	})

	if globalCache == nil {
		return ""
	}

	globalCacheLock.Lock()
	defer globalCacheLock.Unlock()

	return globalCache.entries[soname]
}
