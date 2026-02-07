package ch

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dailyyoga/nexgo/logger"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ClickhouseType represents the category of a ClickHouse column type
type ClickhouseType int

const (
	TypeUnknown ClickhouseType = iota
	TypeString
	TypeInt
	TypeFloat
	TypeDecimal
	TypeBool
	TypeDateTime
	TypeEnum
	TypeIP
	TypeDate
)

// Regex for parsing enum first value (handles spaces)
var enumValueRegex = regexp.MustCompile(`\(\s*'([^']+)'`)

// Regex for parsing function calls (e.g., now64(3), now(), today())
var funcCallRegex = regexp.MustCompile(`^(\w+)\s*\(([^)]*)\)$`)

// DefaultFunc represents a ClickHouse default function that should be evaluated at insert time
type DefaultFunc struct {
	Name string   // Function name (e.g., "now64", "now", "today")
	Args []string // Function arguments
}

// Evaluate evaluates the default function and returns the result
func (f *DefaultFunc) Evaluate() any {
	switch strings.ToLower(f.Name) {
	case "now":
		// now() returns current DateTime (second precision)
		return time.Now().UTC()
	case "now64":
		// now64(precision) returns current DateTime64
		// precision: 3 = milliseconds, 6 = microseconds, 9 = nanoseconds
		return time.Now().UTC()
	case "today":
		// today() returns current Date
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "yesterday":
		// yesterday() returns yesterday's Date
		now := time.Now().UTC().AddDate(0, 0, -1)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "generateuuidv4":
		// generateUUIDv4() returns a new UUID (as string, let ClickHouse handle it)
		// Using a simple UUID v4 generation
		return generateUUID()
	default:
		// Unknown function, return nil and let ClickHouse handle it
		return nil
	}
}

// generateUUID generates a simple UUID v4 string
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Converter singletons to avoid repeated allocations
var (
	stringConverter   = &StringConverter{}
	intConverter      = &IntConverter{}
	floatConverter    = &FloatConverter{}
	decimalConverter  = &DecimalConverter{}
	boolConverter     = &BoolConverter{}
	dateTimeConverter = &DateTimeConverter{}
	ipConverter       = &IPConverter{}
	dateConverter     = &DateConverter{}

	// clickhouseMinTime is the minimum valid time for ClickHouse DateTime/Date (1970-01-01 00:00:00 UTC)
	// Go's time.Time{} zero value (0001-01-01) is before ClickHouse's supported range
	clickhouseMinTime = time.Unix(0, 0).UTC()
)

// TableColumn represents a ClickHouse table column with parsed metadata
type TableColumn struct {
	Name           string
	OriginalType   string
	BaseType       string         // Type without Nullable wrapper
	ParsedType     ClickhouseType // Parsed type category
	IsNullable     bool
	DefaultValue   any    // Pre-parsed default value
	EnumFirstValue string // First enum value (for Enum types)
}

// parseColumnType parses a ClickHouse type string and returns an EnhancedColumn
func parseColumnType(name, colType, defaultValue string) TableColumn {
	col := TableColumn{
		Name:         name,
		OriginalType: colType,
		BaseType:     colType,
	}

	// Check if nullable
	if strings.HasPrefix(colType, "Nullable(") {
		col.IsNullable = true
		col.BaseType = strings.TrimPrefix(colType, "Nullable(")
		col.BaseType = strings.TrimSuffix(col.BaseType, ")")
	}

	// Parse type category - ordered by frequency (most common first)
	// Note: DATETIME must be checked before DATE since DATETIME contains DATE
	baseTypeUpper := strings.ToUpper(col.BaseType)
	switch {
	case strings.Contains(baseTypeUpper, "INT"):
		col.ParsedType = TypeInt
	case strings.Contains(baseTypeUpper, "STRING") || strings.Contains(baseTypeUpper, "FIXEDSTRING"):
		col.ParsedType = TypeString
	case strings.Contains(baseTypeUpper, "FLOAT"):
		col.ParsedType = TypeFloat
	case strings.Contains(baseTypeUpper, "BOOL"):
		col.ParsedType = TypeBool
	case strings.Contains(baseTypeUpper, "DATETIME"):
		col.ParsedType = TypeDateTime
	case strings.Contains(baseTypeUpper, "DATE"):
		col.ParsedType = TypeDate
	case strings.Contains(baseTypeUpper, "DECIMAL"):
		col.ParsedType = TypeDecimal
	case strings.Contains(baseTypeUpper, "ENUM"):
		col.ParsedType = TypeEnum
		col.EnumFirstValue = parseFirstEnumValue(col.BaseType)
	case baseTypeUpper == "IPV4" || baseTypeUpper == "IPV6":
		col.ParsedType = TypeIP
	default:
		col.ParsedType = TypeUnknown
	}

	// Parse default value if provided
	if defaultValue != "" {
		col.DefaultValue = parseDefaultValueForType(defaultValue, col.ParsedType)
	}

	return col
}

// parseFirstEnumValue extracts the first enum value from an Enum type definition
// Example: Enum8('unknown' = 0, 'app' = 1) -> "unknown"
func parseFirstEnumValue(enumType string) string {
	matches := enumValueRegex.FindStringSubmatch(enumType)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseFunctionCall parses a function call string and returns a DefaultFunc
// Example: "now64(3)" -> &DefaultFunc{Name: "now64", Args: []string{"3"}}
// Returns nil if not a supported function call
func parseFunctionCall(value string) *DefaultFunc {
	// Remove leading/trailing whitespace
	value = strings.TrimSpace(value)

	// Check if it matches function call pattern
	matches := funcCallRegex.FindStringSubmatch(value)
	if len(matches) < 2 {
		return nil
	}

	funcName := strings.ToLower(matches[1])

	// Only support known functions
	supportedFuncs := map[string]bool{
		"now":            true,
		"now64":          true,
		"today":          true,
		"yesterday":      true,
		"generateuuidv4": true,
	}

	if !supportedFuncs[funcName] {
		return nil
	}

	// Parse arguments
	var args []string
	if len(matches) > 2 && matches[2] != "" {
		argStr := strings.TrimSpace(matches[2])
		if argStr != "" {
			// Split by comma and trim each argument
			for _, arg := range strings.Split(argStr, ",") {
				args = append(args, strings.TrimSpace(arg))
			}
		}
	}

	return &DefaultFunc{
		Name: funcName,
		Args: args,
	}
}

// parseDefaultValueForType parses a default value string based on the column type
func parseDefaultValueForType(defaultValue string, parsedType ClickhouseType) any {
	// First, check if it's a function call (e.g., now64(3), now(), today())
	if fn := parseFunctionCall(defaultValue); fn != nil {
		return fn
	}

	// Remove surrounding quotes for string/enum types
	if strings.HasPrefix(defaultValue, "'") && strings.HasSuffix(defaultValue, "'") {
		return strings.Trim(defaultValue, "'")
	}

	switch parsedType {
	case TypeUnknown:
		return defaultValue
	case TypeString:
		if strings.HasPrefix(defaultValue, "'") && strings.HasSuffix(defaultValue, "'") {
			return strings.Trim(defaultValue, "'")
		}
		return defaultValue
	case TypeDateTime, TypeDate:
		if strings.HasPrefix(defaultValue, "'") && strings.HasSuffix(defaultValue, "'") {
			return strings.Trim(defaultValue, "'")
		}
		return defaultValue
	case TypeEnum:
		if strings.HasPrefix(defaultValue, "'") && strings.HasSuffix(defaultValue, "'") {
			return strings.Trim(defaultValue, "'")
		}
		return defaultValue
	case TypeIP:
		if strings.HasPrefix(defaultValue, "'") && strings.HasSuffix(defaultValue, "'") {
			return strings.Trim(defaultValue, "'")
		}
		return defaultValue
	case TypeInt:
		if val, err := strconv.ParseInt(defaultValue, 10, 64); err == nil {
			return val
		}
		return int64(0)

	case TypeFloat:
		if val, err := strconv.ParseFloat(defaultValue, 64); err == nil {
			return val
		}
		return 0.0

	case TypeDecimal:
		if d, err := decimal.NewFromString(defaultValue); err == nil {
			return d
		}
		return decimal.Zero

	case TypeBool:
		return defaultValue == "true" || defaultValue == "1"

	default:
		return defaultValue
	}
}

// getZeroValue returns the zero value for a given type
func getZeroValue(col *TableColumn) any {
	// If nullable, return nil
	if col.IsNullable {
		return nil
	}

	switch col.ParsedType {
	case TypeUnknown:
		return nil
	case TypeString:
		return ""
	case TypeInt:
		return int64(0)
	case TypeFloat:
		return 0.0
	case TypeDecimal:
		return decimal.Zero
	case TypeBool:
		return false
	case TypeDateTime:
		// Use Unix epoch (1970-01-01) instead of Go's zero time (0001-01-01)
		// because ClickHouse DateTime only supports 1970-01-01 to 2106-02-07
		return clickhouseMinTime
	case TypeDate:
		// Use Unix epoch (1970-01-01) instead of Go's zero time (0001-01-01)
		// because ClickHouse Date only supports 1970-01-01 to 2149-06-06
		return clickhouseMinTime
	case TypeEnum:
		return col.EnumFirstValue
	case TypeIP:
		// Return IPv6 zero address (::)
		return net.IPv6zero
	default:
		return nil
	}
}

// ValueConverter defines the interface for type-specific value converters
type ValueConverter interface {
	Convert(val any, logger logger.Logger) (any, error)
}

// StringConverter converts values to strings
type StringConverter struct{}

func (c *StringConverter) Convert(val any, log logger.Logger) (any, error) {
	switch v := val.(type) {
	case string:
		return v, nil
	case *string:
		if v == nil {
			return "", nil
		}
		return *v, nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case *int64:
		if v == nil {
			return "", nil
		}
		return strconv.FormatInt(*v, 10), nil
	case int:
		return strconv.Itoa(v), nil
	case *int:
		if v == nil {
			return "", nil
		}
		return strconv.Itoa(*v), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case *int32:
		if v == nil {
			return "", nil
		}
		return strconv.FormatInt(int64(*v), 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case *uint64:
		if v == nil {
			return "", nil
		}
		return strconv.FormatUint(*v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case *float64:
		if v == nil {
			return "", nil
		}
		return strconv.FormatFloat(*v, 'f', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case *float32:
		if v == nil {
			return "", nil
		}
		return strconv.FormatFloat(float64(*v), 'f', -1, 32), nil
	case bool:
		return strconv.FormatBool(v), nil
	case *bool:
		if v == nil {
			return "", nil
		}
		return strconv.FormatBool(*v), nil
	default:
		return "", nil
	}
}

// IntConverter converts values to int64
type IntConverter struct{}

func (c *IntConverter) Convert(val any, log logger.Logger) (any, error) {
	switch v := val.(type) {
	case int64:
		return v, nil
	case *int64:
		if v == nil {
			return int64(0), nil
		}
		return *v, nil
	case int:
		return int64(v), nil
	case *int:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case int32:
		return int64(v), nil
	case *int32:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case int16:
		return int64(v), nil
	case *int16:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case int8:
		return int64(v), nil
	case *int8:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case uint64:
		if v > 9223372036854775807 {
			return int64(9223372036854775807), nil
		}
		return int64(v), nil
	case *uint64:
		if v == nil {
			return int64(0), nil
		}
		if *v > 9223372036854775807 {
			return int64(9223372036854775807), nil
		}
		return int64(*v), nil
	case uint:
		if v > 9223372036854775807 {
			return int64(9223372036854775807), nil
		}
		return int64(v), nil
	case *uint:
		if v == nil {
			return int64(0), nil
		}
		if *v > 9223372036854775807 {
			return int64(9223372036854775807), nil
		}
		return int64(*v), nil
	case uint32:
		return int64(v), nil
	case *uint32:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case uint8:
		return int64(v), nil
	case *uint8:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case *uint16:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case json.Number:
		// json.Number preserves full precision (used with decoder.UseNumber())
		if i, err := v.Int64(); err == nil {
			return i, nil
		}
		if log != nil {
			log.Warn("failed to convert json.Number to int64, using zero", zap.String("value", v.String()))
		}
		return int64(0), nil
	case float64:
		return int64(v), nil
	case *float64:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case float32:
		return int64(v), nil
	case *float32:
		if v == nil {
			return int64(0), nil
		}
		return int64(*v), nil
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i, nil
		}
		if log != nil {
			log.Warn("failed to convert string to int64, using zero", zap.String("value", v))
		}
		return int64(0), nil
	case *string:
		if v == nil {
			return int64(0), nil
		}
		if i, err := strconv.ParseInt(*v, 10, 64); err == nil {
			return i, nil
		}
		if log != nil {
			log.Warn("failed to convert string to int64, using zero", zap.String("value", *v))
		}
		return int64(0), nil
	case bool:
		if v {
			return int64(1), nil
		}
		return int64(0), nil
	case *bool:
		if v == nil {
			return int64(0), nil
		}
		if *v {
			return int64(1), nil
		}
		return int64(0), nil
	default:
		return int64(0), nil
	}
}

// FloatConverter converts values to float64
type FloatConverter struct{}

func (c *FloatConverter) Convert(val any, log logger.Logger) (any, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case *float64:
		if v == nil {
			return 0.0, nil
		}
		return *v, nil
	case float32:
		return float64(v), nil
	case *float32:
		if v == nil {
			return 0.0, nil
		}
		return float64(*v), nil
	case int64:
		return float64(v), nil
	case *int64:
		if v == nil {
			return 0.0, nil
		}
		return float64(*v), nil
	case int:
		return float64(v), nil
	case *int:
		if v == nil {
			return 0.0, nil
		}
		return float64(*v), nil
	case int32:
		return float64(v), nil
	case *int32:
		if v == nil {
			return 0.0, nil
		}
		return float64(*v), nil
	case uint64:
		return float64(v), nil
	case *uint64:
		if v == nil {
			return 0.0, nil
		}
		return float64(*v), nil
	case json.Number:
		// json.Number preserves full precision (used with decoder.UseNumber())
		if f, err := v.Float64(); err == nil {
			return f, nil
		}
		if log != nil {
			log.Warn("failed to convert json.Number to float64, using zero", zap.String("value", v.String()))
		}
		return 0.0, nil
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, nil
		}
		if log != nil {
			log.Warn("failed to convert string to float64, using zero", zap.String("value", v))
		}
		return 0.0, nil
	case *string:
		if v == nil {
			return 0.0, nil
		}
		if f, err := strconv.ParseFloat(*v, 64); err == nil {
			return f, nil
		}
		if log != nil {
			log.Warn("failed to convert string to float64, using zero", zap.String("value", *v))
		}
		return 0.0, nil
	default:
		return 0.0, nil
	}
}

// DecimalConverter converts values to decimal.Decimal
type DecimalConverter struct{}

func (c *DecimalConverter) Convert(val any, log logger.Logger) (any, error) {
	switch v := val.(type) {
	case decimal.Decimal:
		return v, nil
	case *decimal.Decimal:
		if v == nil {
			return decimal.Zero, nil
		}
		return *v, nil
	case float64:
		return decimal.NewFromFloat(v), nil
	case *float64:
		if v == nil {
			return decimal.Zero, nil
		}
		return decimal.NewFromFloat(*v), nil
	case float32:
		return decimal.NewFromFloat32(v), nil
	case *float32:
		if v == nil {
			return decimal.Zero, nil
		}
		return decimal.NewFromFloat32(*v), nil
	case int64:
		return decimal.NewFromInt(v), nil
	case *int64:
		if v == nil {
			return decimal.Zero, nil
		}
		return decimal.NewFromInt(*v), nil
	case int:
		return decimal.NewFromInt(int64(v)), nil
	case *int:
		if v == nil {
			return decimal.Zero, nil
		}
		return decimal.NewFromInt(int64(*v)), nil
	case int32:
		return decimal.NewFromInt32(v), nil
	case *int32:
		if v == nil {
			return decimal.Zero, nil
		}
		return decimal.NewFromInt32(*v), nil
	case uint64:
		// Use string conversion to avoid potential integer overflow
		return decimal.NewFromString(strconv.FormatUint(v, 10))
	case *uint64:
		if v == nil {
			return decimal.Zero, nil
		}
		// Use string conversion to avoid potential integer overflow
		return decimal.NewFromString(strconv.FormatUint(*v, 10))
	case json.Number:
		// json.Number preserves full precision (used with decoder.UseNumber())
		// Use string conversion to preserve precision
		if d, err := decimal.NewFromString(v.String()); err == nil {
			return d, nil
		}
		if log != nil {
			log.Warn("failed to convert json.Number to decimal, using zero", zap.String("value", v.String()))
		}
		return decimal.Zero, nil
	case string:
		if d, err := decimal.NewFromString(v); err == nil {
			return d, nil
		}
		if log != nil {
			log.Warn("failed to convert string to decimal, using zero", zap.String("value", v))
		}
		return decimal.Zero, nil
	case *string:
		if v == nil {
			return decimal.Zero, nil
		}
		if d, err := decimal.NewFromString(*v); err == nil {
			return d, nil
		}
		if log != nil {
			log.Warn("failed to convert string to decimal, using zero", zap.String("value", *v))
		}
		return decimal.Zero, nil
	default:
		return decimal.Zero, nil
	}
}

// BoolConverter converts values to bool
type BoolConverter struct{}

func (c *BoolConverter) Convert(val any, log logger.Logger) (any, error) {
	switch v := val.(type) {
	case bool:
		return v, nil
	case *bool:
		if v == nil {
			return false, nil
		}
		return *v, nil
	case int64:
		return v != 0, nil
	case *int64:
		if v == nil {
			return false, nil
		}
		return *v != 0, nil
	case int:
		return v != 0, nil
	case *int:
		if v == nil {
			return false, nil
		}
		return *v != 0, nil
	case int32:
		return v != 0, nil
	case *int32:
		if v == nil {
			return false, nil
		}
		return *v != 0, nil
	case uint64:
		return v != 0, nil
	case *uint64:
		if v == nil {
			return false, nil
		}
		return *v != 0, nil
	case float64:
		return v != 0, nil
	case *float64:
		if v == nil {
			return false, nil
		}
		return *v != 0, nil
	case float32:
		return v != 0, nil
	case *float32:
		if v == nil {
			return false, nil
		}
		return *v != 0, nil
	case string:
		return v == "true" || v == "1" || v == "TRUE", nil
	case *string:
		if v == nil {
			return false, nil
		}
		return *v == "true" || *v == "1" || *v == "TRUE", nil
	default:
		return false, nil
	}
}

// Common time formats for parsing time strings
var timeFormats = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",           // MySQL/ClickHouse datetime
	"2006-01-02T15:04:05",           // ISO 8601 without timezone
	"2006-01-02 15:04:05.000",       // MySQL datetime with milliseconds
	"2006-01-02T15:04:05.000",       // ISO 8601 with milliseconds
	"2006-01-02 15:04:05.000000",    // MySQL datetime with microseconds
	"2006-01-02T15:04:05.000000",    // ISO 8601 with microseconds
	"2006-01-02 15:04:05.000000000", // MySQL datetime with nanoseconds
	"2006-01-02T15:04:05.000000000", // ISO 8601 with nanoseconds
	"2006-01-02",                    // Date only
}

// parseTimeString attempts to parse a time string using common formats
func parseTimeString(s string) (time.Time, bool) {
	for _, format := range timeFormats {
		if t, err := time.Parse(format, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// DateTimeConverter converts values to time.Time
type DateTimeConverter struct{}

func (c *DateTimeConverter) Convert(val any, log logger.Logger) (any, error) {
	switch v := val.(type) {
	case int64:
		// Assume milliseconds timestamp
		return time.UnixMilli(v), nil
	case *int64:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return time.UnixMilli(*v), nil
	case int:
		return time.UnixMilli(int64(v)), nil
	case *int:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return time.UnixMilli(int64(*v)), nil
	case json.Number:
		// json.Number preserves full precision (used with decoder.UseNumber())
		// Assume milliseconds timestamp
		if i, err := v.Int64(); err == nil {
			return time.UnixMilli(i), nil
		}
		if log != nil {
			log.Warn("failed to parse json.Number to int64, using clickhouse min time", zap.String("value", v.String()))
		}
		return clickhouseMinTime, nil
	case float64:
		// JSON numbers are decoded as float64 by default (may lose precision for large integers)
		// Assume milliseconds timestamp
		return time.UnixMilli(int64(v)), nil
	case *float64:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return time.UnixMilli(int64(*v)), nil
	case float32:
		return time.UnixMilli(int64(v)), nil
	case *float32:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return time.UnixMilli(int64(*v)), nil
	case time.Time:
		return v, nil
	case *time.Time:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return *v, nil
	case string:
		if t, ok := parseTimeString(v); ok {
			return t, nil
		}
		if log != nil {
			log.Warn("failed to parse string to time, using clickhouse min time", zap.String("value", v))
		}
		return clickhouseMinTime, nil
	case *string:
		if v == nil {
			return clickhouseMinTime, nil
		}
		if t, ok := parseTimeString(*v); ok {
			return t, nil
		}
		if log != nil {
			log.Warn("failed to parse string to time, using clickhouse min time", zap.String("value", *v))
		}
		return clickhouseMinTime, nil
	default:
		return clickhouseMinTime, nil
	}
}

// EnumConverter handles enum type conversions
type EnumConverter struct {
	FirstValue string
}

func (c *EnumConverter) Convert(val any, log logger.Logger) (any, error) {
	// If value is empty string, return first enum value
	if str, ok := val.(string); ok && str == "" {
		return c.FirstValue, nil
	}
	return val, nil
}

// IPConverter converts values to net.IP for IPv4/IPv6 columns
type IPConverter struct{}

func (c *IPConverter) Convert(val any, log logger.Logger) (any, error) {
	switch v := val.(type) {
	case net.IP:
		if len(v) == 0 {
			return net.IPv6zero, nil
		}
		return v, nil
	case netip.Addr:
		if !v.IsValid() {
			return net.IPv6zero, nil
		}
		return net.IP(v.AsSlice()), nil
	case string:
		if v == "" {
			return net.IPv6zero, nil
		}
		ip := net.ParseIP(v)
		if ip == nil {
			if log != nil {
				log.Warn("failed to parse string to IP, using zero IP", zap.String("value", v))
			}
			return net.IPv6zero, nil
		}
		return ip, nil
	case []byte:
		if len(v) == 0 {
			return net.IPv6zero, nil
		}
		return net.IP(v), nil
	default:
		return net.IPv6zero, nil
	}
}

// DateConverter converts values to time.Time for Date columns
type DateConverter struct{}

func (c *DateConverter) Convert(val any, log logger.Logger) (any, error) {
	switch v := val.(type) {
	case time.Time:
		return v, nil
	case *time.Time:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return *v, nil
	case int64:
		// Assume seconds timestamp for Date (not milliseconds)
		return time.Unix(v, 0), nil
	case *int64:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return time.Unix(*v, 0), nil
	case int:
		return time.Unix(int64(v), 0), nil
	case *int:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return time.Unix(int64(*v), 0), nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return time.Unix(i, 0), nil
		}
		if log != nil {
			log.Warn("failed to parse json.Number to date, using clickhouse min time", zap.String("value", v.String()))
		}
		return clickhouseMinTime, nil
	case float64:
		return time.Unix(int64(v), 0), nil
	case *float64:
		if v == nil {
			return clickhouseMinTime, nil
		}
		return time.Unix(int64(*v), 0), nil
	case string:
		// Try common date formats
		formats := []string{
			"2006-01-02",
			"2006/01/02",
			"20060102",
			time.RFC3339,
		}
		for _, format := range formats {
			if t, err := time.Parse(format, v); err == nil {
				return t, nil
			}
		}
		if log != nil {
			log.Warn("failed to parse string to date, using clickhouse min time", zap.String("value", v))
		}
		return clickhouseMinTime, nil
	case *string:
		if v == nil {
			return clickhouseMinTime, nil
		}
		formats := []string{
			"2006-01-02",
			"2006/01/02",
			"20060102",
			time.RFC3339,
		}
		for _, format := range formats {
			if t, err := time.Parse(format, *v); err == nil {
				return t, nil
			}
		}
		if log != nil {
			log.Warn("failed to parse string to date, using clickhouse min time", zap.String("value", *v))
		}
		return clickhouseMinTime, nil
	default:
		return clickhouseMinTime, nil
	}
}

// getConverter returns the appropriate converter for a column type
// Uses singleton instances for better performance
func getConverter(col *TableColumn) ValueConverter {
	switch col.ParsedType {
	case TypeUnknown:
		return stringConverter // fallback to string for unknown types
	case TypeInt:
		return intConverter
	case TypeString:
		return stringConverter
	case TypeFloat:
		return floatConverter
	case TypeDecimal:
		return decimalConverter
	case TypeBool:
		return boolConverter
	case TypeDateTime:
		return dateTimeConverter
	case TypeDate:
		return dateConverter
	case TypeEnum:
		// Only EnumConverter needs to be created per column (contains state)
		return &EnumConverter{FirstValue: col.EnumFirstValue}
	case TypeIP:
		return ipConverter
	default:
		return stringConverter // fallback to string
	}
}
