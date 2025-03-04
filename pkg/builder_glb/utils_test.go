package builder

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// Test MinInt function
func TestMinInt(t *testing.T) {
	// Test case where a < b
	a, b := 3, 5
	expected := 3
	result := MinInt(a, b)
	if result != expected {
		t.Errorf("MinInt(%d, %d) = %d; want %d", a, b, result, expected)
	}

	// Test case where a > b
	a, b = 7, 2
	expected = 2
	result = MinInt(a, b)
	if result != expected {
		t.Errorf("MinInt(%d, %d) = %d; want %d", a, b, result, expected)
	}

	// Test case where a == b
	a, b = 4, 4
	expected = 4
	result = MinInt(a, b)
	if result != expected {
		t.Errorf("MinInt(%d, %d) = %d; want %d", a, b, result, expected)
	}
}

// Test TrimString function
func TestTrimString(t *testing.T) {
	// Test case where length is shorter than string length
	str := "hello world"
	length := 5
	expected := "hello"
	result := TrimString(str, length)
	if result != expected {
		t.Errorf("TrimString(%s, %d) = %s; want %s", str, length, result, expected)
	}

	// Test case where length is longer than string length
	length = 20
	expected = "hello world"
	result = TrimString(str, length)
	if result != expected {
		t.Errorf("TrimString(%s, %d) = %s; want %s", str, length, result, expected)
	}
}

// Test HashString function
func TestHashString(t *testing.T) {
	str := "test"
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte(str)))
	result := HashString(str)

	if result != expected {
		t.Errorf("HashString(%s) = %s; want %s", str, result, expected)
	}
}

// Test StringListToString function
func TestStringListToString(t *testing.T) {
	// Test case with multiple strings
	strList := []string{"apple", "banana", "cherry"}
	expected := "apple,banana,cherry"
	result := StringListToString(strList)

	if result != expected {
		t.Errorf("StringListToString(%v) = %s; want %s", strList, result, expected)
	}

	// Test case with an empty list
	strList = []string{}
	expected = ""
	result = StringListToString(strList)

	if result != expected {
		t.Errorf("StringListToString(%v) = %s; want %s", strList, result, expected)
	}
}

// Test PointerOf function
func TestPointerOf(t *testing.T) {
	// Test with an int
	val := 42
	ptr := PointerOf(val)
	if *ptr != val {
		t.Errorf("PointerOf(%d) = %d; want %d", val, *ptr, val)
	}

	// Test with a string
	str := "hello"
	ptrStr := PointerOf(str)
	if *ptrStr != str {
		t.Errorf("PointerOf(%s) = %s; want %s", str, *ptrStr, str)
	}
}
