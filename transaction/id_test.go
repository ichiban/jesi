package transaction

import (
	"context"
	"net/http"
	"testing"
)

func TestWithID(t *testing.T) {
	id := "6a652ecd-8285-4f6b-94d6-686bea2d6d7d"

	oldGenID := genID
	genID = func() string {
		return id
	}
	defer func() {
		genID = oldGenID
	}()

	req := &http.Request{}

	testCases := []struct {
		req *http.Request
		id  string
	}{
		{
			req: req,
			id:  id,
		},
		{
			req: req.WithContext(context.WithValue(nil, IDKey, "foobar")),
			id:  id,
		},
	}

	for i, tc := range testCases {
		r := WithID(tc.req)
		id := r.Context().Value(IDKey).(string)
		if tc.id != id {
			t.Errorf("(%d) expected: %s, got: %s", i, tc.id, id)
		}
	}
}

func TestID(t *testing.T) {
	r := &http.Request{}
	id := "6a652ecd-8285-4f6b-94d6-686bea2d6d7d"

	testCases := []struct {
		req *http.Request
		id  string
	}{
		{
			req: r,
			id:  "",
		},
		{
			req: r.WithContext(context.WithValue(nil, IDKey, id)),
			id:  id,
		},
	}

	for i, tc := range testCases {
		id := ID(tc.req)
		if tc.id != id {
			t.Errorf("(%d) expected: %s, got: %s", i, tc.id, id)
		}
	}
}
