package xpc

import (
	"testing"
	"time"
)

// TestNewDictionary tests basic dictionary creation and operations
func TestNewDictionary(t *testing.T) {
	dict := NewDictionary()
	if dict == nil {
		t.Fatal("NewDictionary returned nil")
	}
	if dict.Count() != 0 {
		t.Fatalf("expected count 0, got %d", dict.Count())
	}
}

// TestDictionarySetAndGetString tests setting and getting string values
func TestDictionarySetAndGetString(t *testing.T) {
	dict := NewDictionary(
		KeyValue("greeting", NewString("hello")),
		KeyValue("name", NewString("world")),
	)
	
	if dict.Count() != 2 {
		t.Fatalf("expected count 2, got %d", dict.Count())
	}
	
	greeting := dict.GetString("greeting")
	if greeting != "hello" {
		t.Fatalf("expected greeting 'hello', got '%s'", greeting)
	}
	
	name := dict.GetString("name")
	if name != "world" {
		t.Fatalf("expected name 'world', got '%s'", name)
	}
}

// TestDictionarySetAndGetInt64 tests setting and getting int64 values
func TestDictionarySetAndGetInt64(t *testing.T) {
	dict := NewDictionary(
		KeyValue("port", NewInt64(8080)),
		KeyValue("count", NewInt64(42)),
	)
	
	port := dict.GetInt64("port")
	if port != 8080 {
		t.Fatalf("expected port 8080, got %d", port)
	}
	
	count := dict.GetInt64("count")
	if count != 42 {
		t.Fatalf("expected count 42, got %d", count)
	}
}

// TestDictionarySetAndGetUInt64 tests setting and getting uint64 values
func TestDictionarySetAndGetUInt64(t *testing.T) {
	dict := NewDictionary(
		KeyValue("id", NewUInt64(12345)),
	)
	
	id := dict.GetUInt64("id")
	if id != 12345 {
		t.Fatalf("expected id 12345, got %d", id)
	}
}

// TestDictionarySetAndGetBool tests setting and getting boolean values
func TestDictionarySetAndGetBool(t *testing.T) {
	dict := NewDictionary(
		KeyValue("enabled", NewBool(true)),
		KeyValue("disabled", NewBool(false)),
	)
	
	enabled := dict.GetBool("enabled")
	if !enabled {
		t.Fatalf("expected enabled true, got %v", enabled)
	}
	
	disabled := dict.GetBool("disabled")
	if disabled {
		t.Fatalf("expected disabled false, got %v", disabled)
	}
}

// TestDictionarySetAndGetDouble tests setting and getting double values
func TestDictionarySetAndGetDouble(t *testing.T) {
	dict := NewDictionary(
		KeyValue("pi", NewDouble(3.14159)),
	)
	
	pi := dict.GetDouble("pi")
	if pi != 3.14159 {
		t.Fatalf("expected pi 3.14159, got %f", pi)
	}
}

// TestDictionarySetAndGetData tests setting and getting data values
func TestDictionarySetAndGetData(t *testing.T) {
	data := []byte("hello binary")
	dict := NewDictionary(
		KeyValue("binary", NewData(data)),
	)
	
	retrieved := dict.GetData("binary")
	if string(retrieved) != string(data) {
		t.Fatalf("expected data '%s', got '%s'", string(data), string(retrieved))
	}
}

// TestDictionarySetAndGetDate tests setting and getting date values
func TestDictionarySetAndGetDate(t *testing.T) {
	now := time.Now()
	dict := NewDictionary(
		KeyValue("timestamp", NewDate(now.UnixNano())),
	)
	
	retrieved := dict.GetDate("timestamp")
	// Compare with second precision due to nanosecond precision differences
	if retrieved.Unix() != now.Unix() {
		t.Fatalf("expected timestamp %v, got %v", now.Unix(), retrieved.Unix())
	}
}

// TestDictionarySetAndGetArray tests setting and getting array values
func TestDictionarySetAndGetArray(t *testing.T) {
	arr := NewArray(
		NewString("foo"),
		NewString("bar"),
	)
	dict := NewDictionary(
		KeyValue("items", arr),
	)
	
	retrieved := dict.GetArray("items")
	if retrieved == nil {
		t.Fatal("expected array, got nil")
	}
	
	if retrieved.Count() != 2 {
		t.Fatalf("expected array count 2, got %d", retrieved.Count())
	}
	
	first := retrieved.GetString(0)
	if first != "foo" {
		t.Fatalf("expected first 'foo', got '%s'", first)
	}
	
	second := retrieved.GetString(1)
	if second != "bar" {
		t.Fatalf("expected second 'bar', got '%s'", second)
	}
}

// TestDictionarySetAndGetDictionary tests setting and getting nested dictionaries
func TestDictionarySetAndGetDictionary(t *testing.T) {
	nested := NewDictionary(
		KeyValue("nested_key", NewString("nested_value")),
	)
	parent := NewDictionary(
		KeyValue("nested", nested),
	)
	
	retrieved := parent.GetDictionary("nested")
	if retrieved == nil {
		t.Fatal("expected dictionary, got nil")
	}
	
	value := retrieved.GetString("nested_key")
	if value != "nested_value" {
		t.Fatalf("expected 'nested_value', got '%s'", value)
	}
}

// TestDictionaryIteration tests iterating over dictionary entries
func TestDictionaryIteration(t *testing.T) {
	dict := NewDictionary(
		KeyValue("key1", NewString("value1")),
		KeyValue("key2", NewString("value2")),
		KeyValue("key3", NewString("value3")),
	)
	
	count := 0
	for key, value := range dict.All() {
		count++
		if value == nil {
			t.Fatalf("expected non-nil value for key %s", key)
		}
	}
	
	if count != 3 {
		t.Fatalf("expected 3 entries, got %d", count)
	}
}

// TestDictionaryKeys tests iterating over dictionary keys
func TestDictionaryKeys(t *testing.T) {
	dict := NewDictionary(
		KeyValue("alpha", NewString("a")),
		KeyValue("beta", NewString("b")),
	)
	
	keys := make(map[string]bool)
	for key := range dict.Keys() {
		keys[key] = true
	}
	
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	
	if !keys["alpha"] || !keys["beta"] {
		t.Fatal("expected keys 'alpha' and 'beta'")
	}
}

// TestDictionaryValues tests iterating over dictionary values
func TestDictionaryValues(t *testing.T) {
	dict := NewDictionary(
		KeyValue("a", NewInt64(1)),
		KeyValue("b", NewInt64(2)),
	)
	
	count := 0
	for value := range dict.Values() {
		if value == nil {
			t.Fatal("expected non-nil value")
		}
		count++
	}
	
	if count != 2 {
		t.Fatalf("expected 2 values, got %d", count)
	}
}

// TestNewArray tests basic array creation and operations
func TestNewArray(t *testing.T) {
	arr := NewArray()
	if arr == nil {
		t.Fatal("NewArray returned nil")
	}
	if arr.Count() != 0 {
		t.Fatalf("expected count 0, got %d", arr.Count())
	}
}

// TestArrayAppendString tests appending string values to an array
func TestArrayAppendString(t *testing.T) {
	arr := NewArray()
	arr.AppendValue(NewString("hello"))
	arr.AppendValue(NewString("world"))
	
	if arr.Count() != 2 {
		t.Fatalf("expected count 2, got %d", arr.Count())
	}
	
	first := arr.GetString(0)
	if first != "hello" {
		t.Fatalf("expected 'hello', got '%s'", first)
	}
	
	second := arr.GetString(1)
	if second != "world" {
		t.Fatalf("expected 'world', got '%s'", second)
	}
}

// TestArrayAppendInt64 tests appending int64 values to an array
func TestArrayAppendInt64(t *testing.T) {
	arr := NewArray(
		NewInt64(10),
		NewInt64(20),
	)
	
	if arr.Count() != 2 {
		t.Fatalf("expected count 2, got %d", arr.Count())
	}
	
	first := arr.GetInt64(0)
	if first != 10 {
		t.Fatalf("expected 10, got %d", first)
	}
	
	second := arr.GetInt64(1)
	if second != 20 {
		t.Fatalf("expected 20, got %d", second)
	}
}

// TestArrayAppendBool tests appending boolean values to an array
func TestArrayAppendBool(t *testing.T) {
	arr := NewArray(
		NewBool(true),
		NewBool(false),
	)
	
	if arr.Count() != 2 {
		t.Fatalf("expected count 2, got %d", arr.Count())
	}
	
	first := arr.GetBool(0)
	if !first {
		t.Fatalf("expected true, got %v", first)
	}
	
	second := arr.GetBool(1)
	if second {
		t.Fatalf("expected false, got %v", second)
	}
}

// TestArraySetValue tests setting values in an array
func TestArraySetValue(t *testing.T) {
	arr := NewArray(
		NewString("initial1"),
		NewString("initial2"),
	)
	
	arr.SetValue(0, NewString("updated1"))
	arr.SetValue(1, NewString("updated2"))
	
	first := arr.GetString(0)
	if first != "updated1" {
		t.Fatalf("expected 'updated1', got '%s'", first)
	}
	
	second := arr.GetString(1)
	if second != "updated2" {
		t.Fatalf("expected 'updated2', got '%s'", second)
	}
}

// TestArrayIteration tests iterating over array elements
func TestArrayIteration(t *testing.T) {
	arr := NewArray(
		NewString("a"),
		NewString("b"),
		NewString("c"),
	)
	
	count := 0
	for _, value := range arr.All() {
		if value == nil {
			t.Fatal("expected non-nil value")
		}
		count++
	}
	
	if count != 3 {
		t.Fatalf("expected 3 elements, got %d", count)
	}
}

// TestArrayValues tests iterating over array values using Values method
func TestArrayValues(t *testing.T) {
	arr := NewArray(
		NewInt64(1),
		NewInt64(2),
		NewInt64(3),
	)
	
	count := 0
	for value := range arr.Values() {
		if value == nil {
			t.Fatal("expected non-nil value")
		}
		count++
	}
	
	if count != 3 {
		t.Fatalf("expected 3 values, got %d", count)
	}
}

// TestGetType tests getting the type of XPC objects
func TestGetType(t *testing.T) {
	dict := NewDictionary()
	dictType := GetType(dict)
	if dictType != TypeDictionary {
		t.Fatalf("expected TypeDictionary, got %s", dictType.String())
	}
	
	arr := NewArray()
	arrType := GetType(arr)
	if arrType != TypeArray {
		t.Fatalf("expected TypeArray, got %s", arrType.String())
	}
	
	str := NewString("test")
	strType := GetType(str)
	if strType != TypeString {
		t.Fatalf("expected TypeString, got %s", strType.String())
	}
	
	b := NewBool(true)
	bType := GetType(b)
	if bType != TypeBool {
		t.Fatalf("expected TypeBool, got %s", bType.String())
	}
	
	i := NewInt64(42)
	iType := GetType(i)
	if iType != TypeInt64 {
		t.Fatalf("expected TypeInt64, got %s", iType.String())
	}
	
	u := NewUInt64(100)
	uType := GetType(u)
	if uType != TypeUInt64 {
		t.Fatalf("expected TypeUInt64, got %s", uType.String())
	}
	
	d := NewDouble(3.14)
	dType := GetType(d)
	if dType != TypeDouble {
		t.Fatalf("expected TypeDouble, got %s", dType.String())
	}
	
	data := NewData([]byte("test"))
	dataType := GetType(data)
	if dataType != TypeData {
		t.Fatalf("expected TypeData, got %s", dataType.String())
	}
}

// TestStringValue tests string value operations
func TestStringValue(t *testing.T) {
	str := NewString("hello world")
	if str == nil {
		t.Fatal("NewString returned nil")
	}
	
	strType := GetType(str)
	if strType != TypeString {
		t.Fatalf("expected TypeString, got %s", strType.String())
	}
	
	if str.String() != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", str.String())
	}
}

// TestInt64Value tests int64 value operations
func TestInt64Value(t *testing.T) {
	i := NewInt64(12345)
	if i.Int64() != 12345 {
		t.Fatalf("expected 12345, got %d", i.Int64())
	}
}

// TestUInt64Value tests uint64 value operations
func TestUInt64Value(t *testing.T) {
	u := NewUInt64(54321)
	if u.UInt64() != 54321 {
		t.Fatalf("expected 54321, got %d", u.UInt64())
	}
}

// TestDoubleValue tests double value operations
func TestDoubleValue(t *testing.T) {
	d := NewDouble(2.71828)
	if d.Float64() != 2.71828 {
		t.Fatalf("expected 2.71828, got %f", d.Float64())
	}
}

// TestDataValue tests data value operations
func TestDataValue(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	d := NewData(data)
	retrieved := d.Bytes()
	
	if len(retrieved) != len(data) {
		t.Fatalf("expected length %d, got %d", len(data), len(retrieved))
	}
	
	for i, b := range retrieved {
		if b != data[i] {
			t.Fatalf("expected byte %d at index %d, got %d", data[i], i, b)
		}
	}
}

// TestBoolValue tests boolean value operations
func TestBoolValue(t *testing.T) {
	trueValue := NewBool(true)
	if !trueValue.Bool() {
		t.Fatal("expected true")
	}
	
	falseValue := NewBool(false)
	if falseValue.Bool() {
		t.Fatal("expected false")
	}
}

// TestDateValue tests date value operations
func TestDateValue(t *testing.T) {
	now := time.Now()
	d := NewDate(now.UnixNano())
	retrieved := d.Time()
	
	if retrieved.Unix() != now.Unix() {
		t.Fatalf("expected %v, got %v", now.Unix(), retrieved.Unix())
	}
}

// TestNullValue tests null value operations
func TestNullValue(t *testing.T) {
	nullType := GetType(NewNull())
	if nullType != TypeNull {
		t.Fatalf("expected TypeNull, got %s", nullType.String())
	}
}

// TestComplexNestedStructure tests creating and accessing complex nested structures
func TestComplexNestedStructure(t *testing.T) {
	config := NewDictionary(
		KeyValue("database", NewDictionary(
			KeyValue("host", NewString("localhost")),
			KeyValue("port", NewInt64(5432)),
			KeyValue("enabled", NewBool(true)),
		)),
		KeyValue("servers", NewArray(
			NewDictionary(
				KeyValue("name", NewString("server1")),
				KeyValue("port", NewInt64(8080)),
			),
			NewDictionary(
				KeyValue("name", NewString("server2")),
				KeyValue("port", NewInt64(8081)),
			),
		)),
	)
	
	// Access nested database config
	db := config.GetDictionary("database")
	if db == nil {
		t.Fatal("expected database dictionary")
	}
	
	host := db.GetString("host")
	if host != "localhost" {
		t.Fatalf("expected 'localhost', got '%s'", host)
	}
	
	dbPort := db.GetInt64("port")
	if dbPort != 5432 {
		t.Fatalf("expected 5432, got %d", dbPort)
	}
	
	enabled := db.GetBool("enabled")
	if !enabled {
		t.Fatal("expected enabled to be true")
	}
	
	// Access servers array
	servers := config.GetArray("servers")
	if servers == nil {
		t.Fatal("expected servers array")
	}
	
	if servers.Count() != 2 {
		t.Fatalf("expected 2 servers, got %d", servers.Count())
	}
	
	server1 := servers.GetDictionary(0)
	if server1 == nil {
		t.Fatal("expected server1 dictionary")
	}
	
	server1Name := server1.GetString("name")
	if server1Name != "server1" {
		t.Fatalf("expected 'server1', got '%s'", server1Name)
	}
	
	server1Port := server1.GetInt64("port")
	if server1Port != 8080 {
		t.Fatalf("expected 8080, got %d", server1Port)
	}
	
	server2 := servers.GetDictionary(1)
	if server2 == nil {
		t.Fatal("expected server2 dictionary")
	}
	
	server2Name := server2.GetString("name")
	if server2Name != "server2" {
		t.Fatalf("expected 'server2', got '%s'", server2Name)
	}
}

// TestTypeString tests the String method for all types
func TestTypeString(t *testing.T) {
	types := map[Type]string{
		TypeArray:      "array",
		TypeBool:       "bool",
		TypeData:       "data",
		TypeDate:       "date",
		TypeDictionary: "dictionary",
		TypeDouble:     "double",
		TypeInt64:      "int64",
		TypeNull:       "null",
		TypeString:     "string",
		TypeUInt64:     "uint64",
	}
	
	for typ, expectedName := range types {
		name := typ.String()
		if name != expectedName {
			t.Fatalf("expected %s, got %s", expectedName, name)
		}
	}
}

// TestDictionarySetValueNil tests that setting nil removes a key
func TestDictionarySetValueNil(t *testing.T) {
	dict := NewDictionary(
		KeyValue("key", NewString("value")),
	)
	
	if dict.Count() != 1 {
		t.Fatalf("expected count 1, got %d", dict.Count())
	}
	
	dict.SetValue("key", nil)
	
	if dict.Count() != 0 {
		t.Fatalf("expected count 0 after nil assignment, got %d", dict.Count())
	}
}

// TestEmptyDataValue tests empty data value
func TestEmptyDataValue(t *testing.T) {
	empty := NewData([]byte{})
	retrieved := empty.Bytes()
	
	if len(retrieved) != 0 {
		t.Fatalf("expected empty data, got %d bytes", len(retrieved))
	}
}

// TestDictionaryGetNonExistentKey tests getting a non-existent key
func TestDictionaryGetNonExistentKey(t *testing.T) {
	dict := NewDictionary(
		KeyValue("existing", NewString("value")),
	)
	
	result := dict.GetString("non_existent")
	if result != "" {
		t.Fatalf("expected empty string, got '%s'", result)
	}
	
	arr := dict.GetArray("non_existent")
	if arr != nil {
		t.Fatal("expected nil for non-existent array")
	}
	
	nested := dict.GetDictionary("non_existent")
	if nested != nil {
		t.Fatal("expected nil for non-existent dictionary")
	}
}

// TestEmptyArray tests creating and using an empty array
func TestEmptyArray(t *testing.T) {
	arr := NewArray()
	if arr == nil {
		t.Fatal("NewArray() returned nil")
	}
	
	if arr.Count() != 0 {
		t.Fatalf("expected empty array count 0, got %d", arr.Count())
	}
	
	// Test appending to empty array
	arr.AppendValue(NewString("first"))
	if arr.Count() != 1 {
		t.Fatalf("expected count 1 after append, got %d", arr.Count())
	}
	
	value := arr.GetString(0)
	if value != "first" {
		t.Fatalf("expected 'first', got '%s'", value)
	}
}
