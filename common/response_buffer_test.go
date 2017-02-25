package common

import (
	"bytes"
	"net/http"
	"testing"
)

func TestResponseBuffer_Header(t *testing.T) {
	h := http.Header{
		"foo": []string{"bar"},
		"baz": []string{"qux", "quux"},
	}
	resp := &ResponseBuffer{
		HeaderMap: h,
	}

	if len(h) != len(resp.Header()) {
		t.Error()
	}

	for k, vs := range h {
		if len(vs) != len(resp.Header()[k]) {
			t.Error()
		}

		for i, v := range vs {
			if v != resp.Header()[k][i] {
				t.Error()
			}
		}
	}
}

func TestResponseBuffer_Write(t *testing.T) {
	var resp ResponseBuffer

	resp.Write([]byte("foo"))
	if "foo" != string(resp.Body) {
		t.Error()
	}
	resp.Write([]byte("bar"))
	if "foobar" != string(resp.Body) {
		t.Error()
	}
}

func TestResponseBuffer_WriteHeader(t *testing.T) {
	var resp ResponseBuffer

	resp.WriteHeader(http.StatusAccepted)

	if http.StatusAccepted != resp.StatusCode {
		t.Error()
	}
}

func TestResponseBuffer_WriteTo(t *testing.T) {
	resp := ResponseBuffer{
		Body: []byte("foobar"),
	}
	var buf bytes.Buffer

	resp.WriteTo(&buf)

	if "foobar" != string(buf.Bytes()) {
		t.Error()
	}
}

func TestResponseBuffer_Successful(t *testing.T) {
	testCases := []struct {
		n int
		b bool
	}{
		{n: 199, b: false},
		{n: 200, b: true},
		{n: 399, b: true},
		{n: 400, b: false},
	}

	for _, tc := range testCases {
		resp := ResponseBuffer{
			StatusCode: tc.n,
		}

		if tc.b != resp.Successful() {
			t.Error()
		}
	}
}
