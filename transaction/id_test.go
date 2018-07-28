package transaction

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestWithID(t *testing.T) {
	id, err := uuid.Parse("6a652ecd-8285-4f6b-94d6-686bea2d6d7d")
	if err != nil {
		t.Fatalf("uuid.Parse() failed: %v", err)
	}

	oldGenID := genID
	genID = func() uuid.UUID {
		return id
	}
	defer func() {
		genID = oldGenID
	}()

	req := &http.Request{}

	testCases := []struct {
		req *http.Request
		id  uuid.UUID
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
		id := r.Context().Value(IDKey).(uuid.UUID)
		if tc.id != id {
			t.Errorf("(%d) expected: %s, got: %s", i, tc.id, id)
		}
	}
}

func TestID(t *testing.T) {
	r := &http.Request{}
	id, err := uuid.Parse("6a652ecd-8285-4f6b-94d6-686bea2d6d7d")
	if err != nil {
		t.Fatalf("uuid.Parse() failed: %v", err)
	}

	testCases := []struct {
		req *http.Request
		id  *uuid.UUID
	}{
		{
			req: r,
			id:  nil,
		},
		{
			req: r.WithContext(context.WithValue(nil, IDKey, id)),
			id:  &id,
		},
	}

	for i, tc := range testCases {
		id := ID(tc.req)
		if tc.id == nil {
			if id != nil {
				t.Errorf("(%d) expected: nil, got: %s", i, id)
			}
			continue
		} else if id == nil {
			t.Errorf("(%d) expected: %s, got: nil", i, tc.id)
		}
		if *tc.id != *id {
			t.Errorf("(%d) expected: %s, got: %s", i, tc.id, id)
		}
	}
}
