package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// ============ PRETTY PRINT SYSTEM FOR INTERFACE VALUES ============

// PrintOptions configures the print behavior
type PrintOptions struct {
	Indent            string // Indentation string (default: "  ")
	MaxDepth          int    // Maximum nesting depth (default: 10)
	ShowTypes         bool   // Show type information (default: false)
	CompactArrays     bool   // Print arrays in compact format (default: false)
	ColorOutput       bool   // Enable colored output (default: false)
	TimeFormat        string // Time format (default: RFC3339)
	MaxStringLength   int    // Maximum string length before truncation (default: 100)
	ShowPrivateFields bool   // Show private struct fields (default: true)
}

// DefaultPrintOptions returns sensible defaults
func DefaultPrintOptions() *PrintOptions {
	return &PrintOptions{
		Indent:            "  ",
		MaxDepth:          10,
		ShowTypes:         false,
		CompactArrays:     false,
		ColorOutput:       false,
		TimeFormat:        time.RFC3339,
		MaxStringLength:   100,
		ShowPrivateFields: true,
	}
}

// PrettyPrint prints any interface value in a beautiful, nested format
func PrettyPrint(v interface{}) {
	PrettyPrintWithOptions(v, DefaultPrintOptions())
}

// PrettyPrintWithOptions prints with custom options
func PrettyPrintWithOptions(v interface{}, opts *PrintOptions) {
	if opts == nil {
		opts = DefaultPrintOptions()
	}

	fmt.Println(formatValue(v, 0, opts, make(map[uintptr]bool)))
}

// PrettyPrintJSON prints as formatted JSON (fallback for complex types)
func PrettyPrintJSON(v interface{}) {
	jsonData, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling to JSON: %v\n", err)
		// Fallback to regular pretty print
		PrettyPrint(v)
		return
	}
	fmt.Println(string(jsonData))
}

// PrintLog - Enhanced version of your original function
func PrintLog(v interface{}) {
	if v == nil {
		fmt.Println("nil")
		return
	}

	// Use pretty print for complex types
	switch reflect.TypeOf(v).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct, reflect.Ptr:
		PrettyPrint(v)
	default:
		fmt.Println(v)
	}
}

// ============ INTERNAL FORMATTING FUNCTIONS ============

// formatValue formats any value recursively
func formatValue(v interface{}, depth int, opts *PrintOptions, visited map[uintptr]bool) string {
	if depth > opts.MaxDepth {
		return fmt.Sprintf("... (max depth %d exceeded)", opts.MaxDepth)
	}

	if v == nil {
		return colorize("nil", "gray", opts)
	}

	val := reflect.ValueOf(v)
	typ := reflect.TypeOf(v)

	// Handle circular references for pointers
	if val.Kind() == reflect.Ptr && !val.IsNil() {
		ptr := val.Pointer()
		if visited[ptr] {
			return colorize("... (circular reference)", "yellow", opts)
		}
		visited[ptr] = true
		defer delete(visited, ptr)
	}

	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return colorize("nil", "gray", opts)
		}
		return formatValue(val.Elem().Interface(), depth, opts, visited)

	case reflect.Interface:
		if val.IsNil() {
			return colorize("nil", "gray", opts)
		}
		return formatValue(val.Elem().Interface(), depth, opts, visited)

	case reflect.String:
		str := val.String()
		if len(str) > opts.MaxStringLength {
			str = str[:opts.MaxStringLength] + "..."
		}
		result := fmt.Sprintf(`"%s"`, strings.ReplaceAll(str, "\n", "\\n"))
		return colorize(result, "green", opts)

	case reflect.Bool:
		return colorize(fmt.Sprintf("%v", val.Bool()), "yellow", opts)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return colorize(fmt.Sprintf("%d", val.Int()), "blue", opts)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return colorize(fmt.Sprintf("%d", val.Uint()), "blue", opts)

	case reflect.Float32, reflect.Float64:
		return colorize(fmt.Sprintf("%g", val.Float()), "blue", opts)

	case reflect.Complex64, reflect.Complex128:
		return colorize(fmt.Sprintf("%v", val.Complex()), "blue", opts)

	case reflect.Slice, reflect.Array:
		return formatSlice(val, depth, opts, visited)

	case reflect.Map:
		return formatMap(val, depth, opts, visited)

	case reflect.Struct:
		return formatStruct(val, typ, depth, opts, visited)

	case reflect.Chan:
		return colorize(fmt.Sprintf("chan %s (len: %d, cap: %d)",
			typ.Elem().Name(), val.Len(), val.Cap()), "purple", opts)

	case reflect.Func:
		return colorize(fmt.Sprintf("func %s", typ.String()), "purple", opts)

	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatSlice formats slices and arrays
func formatSlice(val reflect.Value, depth int, opts *PrintOptions, visited map[uintptr]bool) string {
	length := val.Len()
	if length == 0 {
		return "[]"
	}

	if opts.CompactArrays && length <= 5 {
		// Compact format for small arrays
		var elements []string
		for i := 0; i < length; i++ {
			elements = append(elements, formatValue(val.Index(i).Interface(), depth+1, opts, visited))
		}
		return fmt.Sprintf("[%s]", strings.Join(elements, ", "))
	}

	// Multi-line format
	indent := strings.Repeat(opts.Indent, depth)
	nextIndent := strings.Repeat(opts.Indent, depth+1)

	var builder strings.Builder
	builder.WriteString("[\n")

	for i := 0; i < length; i++ {
		builder.WriteString(nextIndent)
		builder.WriteString(fmt.Sprintf("[%d]: ", i))
		builder.WriteString(formatValue(val.Index(i).Interface(), depth+1, opts, visited))
		if i < length-1 {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
	}

	builder.WriteString(indent + "]")
	return builder.String()
}

// formatMap formats maps
func formatMap(val reflect.Value, depth int, opts *PrintOptions, visited map[uintptr]bool) string {
	keys := val.MapKeys()
	if len(keys) == 0 {
		return "{}"
	}

	indent := strings.Repeat(opts.Indent, depth)
	nextIndent := strings.Repeat(opts.Indent, depth+1)

	var builder strings.Builder
	builder.WriteString("{\n")

	for i, key := range keys {
		mapVal := val.MapIndex(key)
		builder.WriteString(nextIndent)
		builder.WriteString(formatValue(key.Interface(), depth+1, opts, visited))
		builder.WriteString(": ")
		builder.WriteString(formatValue(mapVal.Interface(), depth+1, opts, visited))
		if i < len(keys)-1 {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
	}

	builder.WriteString(indent + "}")
	return builder.String()
}

// formatStruct formats structs
func formatStruct(val reflect.Value, typ reflect.Type, depth int, opts *PrintOptions, visited map[uintptr]bool) string {
	// Handle special types
	if t, ok := val.Interface().(time.Time); ok {
		return colorize(fmt.Sprintf(`"%s"`, t.Format(opts.TimeFormat)), "cyan", opts)
	}

	numFields := val.NumField()
	if numFields == 0 {
		typeName := ""
		if opts.ShowTypes {
			typeName = typ.Name() + " "
		}
		return fmt.Sprintf("%s{}", typeName)
	}

	indent := strings.Repeat(opts.Indent, depth)
	nextIndent := strings.Repeat(opts.Indent, depth+1)

	var builder strings.Builder
	if opts.ShowTypes {
		builder.WriteString(colorize(typ.Name(), "cyan", opts))
		builder.WriteString(" ")
	}
	builder.WriteString("{\n")

	fieldCount := 0
	for i := 0; i < numFields; i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// Skip private fields if option is disabled
		if !opts.ShowPrivateFields && !field.IsExported() {
			continue
		}

		// Skip unexported fields that can't be accessed
		if !fieldVal.CanInterface() {
			continue
		}

		if fieldCount > 0 {
			builder.WriteString(",\n")
		}

		builder.WriteString(nextIndent)
		builder.WriteString(colorize(field.Name, "magenta", opts))
		builder.WriteString(": ")
		builder.WriteString(formatValue(fieldVal.Interface(), depth+1, opts, visited))
		fieldCount++
	}

	if fieldCount > 0 {
		builder.WriteString("\n")
	}
	builder.WriteString(indent + "}")
	return builder.String()
}

// colorize adds color to output if enabled
func colorize(text, color string, opts *PrintOptions) string {
	if !opts.ColorOutput {
		return text
	}

	colors := map[string]string{
		"red":     "\033[31m",
		"green":   "\033[32m",
		"yellow":  "\033[33m",
		"blue":    "\033[34m",
		"magenta": "\033[35m",
		"cyan":    "\033[36m",
		"gray":    "\033[37m",
		"purple":  "\033[95m",
		"reset":   "\033[0m",
	}

	if colorCode, exists := colors[color]; exists {
		return colorCode + text + colors["reset"]
	}
	return text
}

// ============ CONVENIENCE FUNCTIONS ============

// PrintWithTitle prints a value with a descriptive title
func PrintWithTitle(title string, v interface{}) {
	fmt.Printf("=== %s ===\n", title)
	PrettyPrint(v)
	fmt.Println()
}

// PrintColored prints with colors enabled
func PrintColored(v interface{}) {
	opts := DefaultPrintOptions()
	opts.ColorOutput = true
	PrettyPrintWithOptions(v, opts)
}

// PrintCompact prints in compact format
func PrintCompact(v interface{}) {
	opts := DefaultPrintOptions()
	opts.CompactArrays = true
	opts.Indent = " "
	PrettyPrintWithOptions(v, opts)
}

// PrintWithTypes prints with type information
func PrintWithTypes(v interface{}) {
	opts := DefaultPrintOptions()
	opts.ShowTypes = true
	PrettyPrintWithOptions(v, opts)
}
