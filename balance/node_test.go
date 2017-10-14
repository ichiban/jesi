package balance

import (
	"net"
	"testing"
)

func TestParseNode(t *testing.T) {
	testCases := []struct {
		str string

		node *Node
		err  bool
	}{
		{
			str: "unknown",
			node: &Node{
				ID: "unknown",
			},
		},
		{
			str: "_obfuscated-identifier.",
			node: &Node{
				ID: "_obfuscated-identifier.",
			},
		},
		{
			str: "192.0.2.1",
			node: &Node{
				Addr: net.ParseIP("192.0.2.1"),
			},
		},
		{
			str: "192.0.2.1:12345",
			node: &Node{
				Addr: net.ParseIP("192.0.2.1"),
				Port: port(12345),
			},
		},
		{
			str: "[2001:db8::1]",
			node: &Node{
				Addr: net.ParseIP("2001:db8::1"),
			},
		},
		{
			str: "[2001:db8::1]:12345",
			node: &Node{
				Addr: net.ParseIP("2001:db8::1"),
				Port: port(12345),
			},
		},
		{
			str: "random string",
			err: true,
		},
	}

	for i, tc := range testCases {
		n, err := ParseNode(tc.str)
		if err != nil {
			if tc.err {
				continue
			}
			t.Errorf("(%d) %s", i, err)
		}

		if tc.node.ID != n.ID {
			t.Errorf("(%d) expected: %s, got %s", i, tc.node.ID, n.ID)
		}

		if tc.node.Addr.String() != n.Addr.String() {
			t.Errorf("(%d) expected: %s, got %s", i, tc.node.Addr.String(), n.Addr.String())
		}

		if tc.node.Port == nil && n.Port == nil {
			continue
		}

		if *tc.node.Port != *n.Port {
			t.Errorf("(%d) expected: %d, got %d", i, *tc.node.Port, *n.Port)
		}
	}
}

func TestNode_String(t *testing.T) {
	testCases := []struct {
		node Node

		str string
	}{
		{
			node: Node{
				ID: "unknown",
			},
			str: "unknown",
		},
		{
			node: Node{
				ID: "_obfuscated-identifier.",
			},
			str: "_obfuscated-identifier.",
		},
		{
			node: Node{
				Addr: net.ParseIP("192.0.2.1"),
			},
			str: "192.0.2.1",
		},
		{
			node: Node{
				Addr: net.ParseIP("192.0.2.1"),
				Port: port(12345),
			},
			str: "192.0.2.1:12345",
		},
		{
			node: Node{
				Addr: net.ParseIP("2001:db8::1"),
			},
			str: "[2001:db8::1]",
		},
		{
			node: Node{
				Addr: net.ParseIP("2001:db8::1"),
				Port: port(12345),
			},
			str: "[2001:db8::1]:12345",
		},
	}

	for i, tc := range testCases {
		str := tc.node.String()

		if tc.str != str {
			t.Errorf("(%d) expected: %s, got %s", i, tc.str, str)
		}
	}
}

func port(n int) *int {
	return &n
}
