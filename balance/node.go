package balance

import (
	"flag"
	"fmt"
	"github.com/satori/go.uuid"
	"net"
	"strconv"
	"strings"
)

// Node is node identifier which will be appear in Forwarded request header field.
type Node struct {
	ID   string
	Addr net.IP
	Port *int
}

var _ flag.Value = (*Node)(nil)

// ParseNode parses node identifiers.
func ParseNode(s string) (*Node, error) {
	m := nodePattern.FindStringSubmatch(s)
	name := m[1]
	port := m[2]

	var n Node
	if port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		n.Port = &p
	}

	// "unknown" Identifier
	if "unknown" == name {
		n.ID = name
		return &n, nil
	}

	// Obfuscated Identifier
	if obfuscated.MatchString(name) {
		n.ID = name
		return &n, nil
	}

	// strip []
	if strings.HasPrefix(name, `[`) {
		name = name[1 : len(name)-1]
	}

	n.Addr = net.ParseIP(name)
	if n.Addr == nil {
		return nil, fmt.Errorf("failed to parse: %s", name)
	}
	return &n, nil
}

// Set parses node identifiers.
func (n *Node) Set(s string) error {
	p, err := ParseNode(s)
	if err != nil {
		return err
	}
	*n = *p

	return nil
}

func (n *Node) String() string {
	if n.ID == "" && n.Addr == nil {
		n.ID = "_" + uuid.NewV4().String()
	}

	var s string

	if n.ID != "" {
		s = n.ID
	} else {
		s = n.Addr.String()
		if n.Addr.To4() == nil { // v6
			s = fmt.Sprintf("[%s]", s)
		}
	}

	if n.Port != nil {
		s = fmt.Sprintf("%s:%d", s, *n.Port)
	}

	return s
}
