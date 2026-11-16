package canonjson

import (
	"bytes"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

// The expected strings are what Ruby's JSON.generate(Acceptance.canonical(value)) writes
// for the same input (json 2.7.2, Ruby 3.3).
func TestMarshalMatchesRuby(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`{"b": 1, "a": [true, null, "x"]}`, `{"a":[true,null,"x"],"b":1}`},
		{`[0.0, -0.0, 1.0, 0.1, 1e-4, 1e-5, 100.0]`, `[0.0,-0.0,1.0,0.1,0.0001,1.0e-05,100.0]`},
		{`[123456789012345.0, 1e15, 1e16, 1.5e16, 2.5e-7]`, `[123456789012345.0,1.0e+15,1.0e+16,1.5e+16,2.5e-07]`},
		{`[1234567890123456.0, 9007199254740992.0, 12345678901234567.0]`, `[1.234567890123456e+15,9.007199254740992e+15,1.2345678901234568e+16]`},
		{`[0.3333333333333333, 12345678.9, 5e-324, 1.7976931348623157e308]`, `[0.3333333333333333,12345678.9,5.0e-324,1.7976931348623157e+308]`},
		{`[0.00012, 0.00001234, 1.0e2, -2.5, 1E3]`, `[0.00012,1.234e-05,100.0,-2.5,1000.0]`},
		{`[1977127499012779.5, 1234567890123456.5, 999999999999999.0, 9999999999999998.0]`, `[1977127499012779.5,1234567890123456.5,999999999999999.0,9.999999999999998e+15]`},
		{`[12345678901234567890123, -0, 7]`, `[12345678901234567890123,0,7]`},
		{`["a\"b\\c/d\u0001\u001f\b\f\n\r\t\u007f é   <>&"]`, "[\"a\\\"b\\\\c/d\\u0001\\u001f\\b\\f\\n\\r\\t\u007f é   <>&\"]"},
		{`{"é": 1, "e": 2, "Z": 3}`, `{"Z":3,"e":2,"é":1}`},
	}
	for _, tc := range cases {
		value, err := Decode(strings.NewReader(tc.input))
		if err != nil {
			t.Fatalf("Decode(%s): %v", tc.input, err)
		}
		got, err := Marshal(value)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", tc.input, err)
		}
		if string(got) != tc.want {
			t.Errorf("Marshal(%s) = %s, want %s", tc.input, got, tc.want)
		}
	}
}

func TestDecodeRejectsTrailingData(t *testing.T) {
	if _, err := Decode(strings.NewReader(`{} {}`)); err == nil {
		t.Fatal("Decode accepted two values")
	}
}

func TestPrettyMatchesRuby(t *testing.T) {
	value := map[string]any{"b": []any{}, "a": map[string]any{}, "c": []any{int64(1), map[string]any{"z": "x\n", "y": 1.5}}}
	got, err := Pretty(value)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": {\n  },\n  \"b\": [\n\n  ],\n  \"c\": [\n    1,\n    {\n      \"y\": 1.5,\n      \"z\": \"x\\n\"\n    }\n  ]\n}"
	if string(got) != want {
		t.Fatalf("got %q", got)
	}
}

// The expectation is Ruby's JSON.pretty_generate of Command.run's result.
func TestPrettyKeysKeepsRubysOrder(t *testing.T) {
	stage := map[string]any{"command": []any{"mutool", "draw"}, "exit": int64(0), "signal": nil, "timeout": false, "seconds": 0.25}
	got, err := PrettyKeys(stage, []string{"command", "exit", "signal", "timeout", "seconds"})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"command\": [\n    \"mutool\",\n    \"draw\"\n  ],\n  \"exit\": 0,\n  \"signal\": null,\n  \"timeout\": false,\n  \"seconds\": 0.25\n}"
	if string(got) != want {
		t.Fatalf("got %q", got)
	}
}

// referenceShortest returns the shortest round-trip decimal digits of a positive value and the
// position of the decimal point relative to them (value = 0.digits * 10^point).
func referenceShortest(value float64) (string, int) {
	formatted := strconv.FormatFloat(value, 'e', -1, 64)
	mantissa, exponent, _ := strings.Cut(formatted, "e")
	power, _ := strconv.Atoi(exponent)
	return strings.Replace(mantissa, ".", "", 1), power + 1
}

func referenceFixed(digits string, point int) string {
	if point <= 0 {
		return "0." + strings.Repeat("0", -point) + digits
	}
	if len(digits) <= point {
		return digits + strings.Repeat("0", point-len(digits)) + ".0"
	}
	return digits[:point] + "." + digits[point:]
}

// TestFloatDigitsMatchTheStringForms checks the byte forms of the float writer against the string forms
// they replaced, over magnitudes from 1e-9 to 1e17.
func TestFloatDigitsMatchTheStringForms(t *testing.T) {
	random := rand.New(rand.NewSource(7))
	for count := 0; count < 200000; count++ {
		value := random.Float64() * math.Pow(10, float64(random.Intn(27)-9))
		if count%7 == 0 {
			value = math.Round(value)
		}
		if value == 0 {
			continue
		}
		want, wantPoint := referenceShortest(value)
		var scratch [40]byte
		got, gotPoint := shortestDigits(scratch[:0], value)
		if string(got) != want || gotPoint != wantPoint {
			t.Fatalf("%v: digits %q point %d, want %q point %d", value, got, gotPoint, want, wantPoint)
		}
		var buffer bytes.Buffer
		writeFixed(&buffer, got, gotPoint)
		if buffer.String() != referenceFixed(want, wantPoint) {
			t.Fatalf("%v: fixed %q, want %q", value, buffer.String(), referenceFixed(want, wantPoint))
		}
	}
}
